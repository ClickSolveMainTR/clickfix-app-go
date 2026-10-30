package email

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
)

const (
	FieldIMAPHost     = "imap_host"
	FieldIMAPPort     = "imap_port"
	FieldIMAPUsername = "imap_username"
	FieldIMAPPassword = "imap_password"
	FieldIMAPTLS      = "imap_tls"
	FieldIMAPFolder   = "imap_folder"
	FieldIMAPAuth     = "imap_auth"
	SettingIMAPDays   = "imap_sync_days"
	SettingIMAPRead   = "imap_mark_read"
	SettingIMAPMove   = "imap_move_folder"
)

// MailboxAuth leaves room for a future XOAUTH2 implementation. Only password
// authentication is supported today; tokens must never be used as passwords.
type MailboxAuth struct{ Method, Username, Password string }

// IMAPConfig describes one mailbox. Passwords are stored only in the Vault.
type IMAPConfig struct {
	Host, Port, TLS, Folder string
	Auth                    MailboxAuth
	Days                    int
	MarkRead                bool
	MoveFolder              string
}

// IMAPCursor tracks a UID in a specific mailbox incarnation.
type IMAPCursor struct {
	MailboxKey                       string
	UIDValidity, LastUID, InitialUID uint32
}

// IMAPConfiguration validates credentials and import settings before dialing.
func IMAPConfiguration(inst channels.Instance) (IMAPConfig, error) {
	c := inst.Credentials
	cfg := IMAPConfig{Host: strings.TrimSpace(c[FieldIMAPHost]), Port: c[FieldIMAPPort], TLS: c[FieldIMAPTLS], Folder: c[FieldIMAPFolder],
		Auth: MailboxAuth{Method: c[FieldIMAPAuth], Username: c[FieldIMAPUsername], Password: c[FieldIMAPPassword]}, Days: 3}
	if cfg.Host == "" || strings.ContainsAny(cfg.Host, "/\r\n") {
		return cfg, fmt.Errorf("IMAP sunucu adresi gerekli veya geçersiz")
	}
	if cfg.TLS == "" {
		cfg.TLS = "implicit"
	}
	if cfg.TLS != "implicit" && cfg.TLS != "starttls" {
		return cfg, fmt.Errorf("imap_tls implicit veya starttls olmalıdır")
	}
	if cfg.Port == "" {
		cfg.Port = "993"
		if cfg.TLS == "starttls" {
			cfg.Port = "143"
		}
	}
	port, err := strconv.Atoi(cfg.Port)
	if err != nil || port < 1 || port > 65535 {
		return cfg, fmt.Errorf("IMAP portu 1-65535 arasında olmalıdır")
	}
	if cfg.Folder == "" {
		cfg.Folder = "INBOX"
	}
	if cfg.Auth.Method == "" {
		cfg.Auth.Method = "password"
	}
	if cfg.Auth.Method != "password" {
		return cfg, fmt.Errorf("IMAP yalnızca parola ile bağlanmayı destekler; OAuth2 henüz desteklenmiyor")
	}
	if cfg.Auth.Username == "" || cfg.Auth.Password == "" {
		return cfg, fmt.Errorf("IMAP kullanıcı adı ve parolası gereklidir")
	}
	if v, ok := inst.Settings[SettingIMAPDays]; ok {
		n, err := strconv.Atoi(fmt.Sprint(v))
		if err != nil || n < 1 || n > 365 {
			return cfg, fmt.Errorf("imap_sync_days 1-365 arasında tam sayı olmalıdır")
		}
		cfg.Days = n
	}
	if v, ok := inst.Settings[SettingIMAPRead]; ok {
		b, ok := v.(bool)
		if !ok {
			return cfg, fmt.Errorf("imap_mark_read boolean olmalıdır")
		}
		cfg.MarkRead = b
	}
	if v, ok := inst.Settings[SettingIMAPMove]; ok {
		s, ok := v.(string)
		if !ok {
			return cfg, fmt.Errorf("imap_move_folder metin olmalıdır")
		}
		cfg.MoveFolder = strings.TrimSpace(s)
	}
	if cfg.MoveFolder != "" && strings.EqualFold(cfg.MoveFolder, cfg.Folder) {
		return cfg, fmt.Errorf("taşıma klasörü kaynak klasörden farklı olmalıdır")
	}
	return cfg, nil
}

// Key invalidates the cursor after switching account or folder, without storing
// mailbox credentials in the cursor table.
func (c IMAPConfig) Key() string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join([]string{c.Host, c.Port, c.Auth.Username, c.Folder}, "\x00"))))
}

// IMAPError deliberately discards provider text: servers may echo credentials
// or literal mail data in errors. This text is safe for logs and health views.
type IMAPError struct{ Step string }

func (e *IMAPError) Error() string {
	switch e.Step {
	case "auth":
		return "IMAP kimlik doğrulaması başarısız. Kullanıcı adı ve parolayı kontrol edin. Gmail, Outlook ve Yandex için uygulama parolası gerekebilir; Microsoft 365 temel kimlik doğrulamasını kapatmış olabilir (OAuth2 henüz desteklenmiyor)."
	case "tls":
		return "IMAP TLS bağlantısı kurulamadı. TLS modunu, portu ve sunucu sertifikasını kontrol edin."
	case "host":
		return "IMAP sunucusuna ulaşılamadı. Sunucu adresini, portu ve ağ erişimini kontrol edin."
	case "folder":
		return "IMAP klasörü açılamadı. Klasör adını ve erişim izinlerini kontrol edin."
	default:
		return "IMAP eşitlemesi tamamlanamadı. Sunucu bağlantısını ve klasör izinlerini kontrol edin."
	}
}

func dialIMAP(ctx context.Context, cfg IMAPConfig, tc *tls.Config) (*client.Client, func(), error) {
	if tc == nil {
		tc = &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	}
	d := &net.Dialer{Timeout: 15 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(cfg.Host, cfg.Port))
	if err != nil {
		return nil, nil, &IMAPError{Step: "host"}
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	closeConn := func() { stop(); _ = conn.Close() }
	deadline := time.Now().Add(90 * time.Second)
	if t, ok := ctx.Deadline(); ok && t.Before(deadline) {
		deadline = t
	}
	_ = conn.SetDeadline(deadline)
	transport := conn
	if cfg.TLS == "implicit" {
		t := tls.Client(conn, tc)
		if err := t.HandshakeContext(ctx); err != nil {
			closeConn()
			return nil, nil, &IMAPError{Step: "tls"}
		}
		transport = t
	}
	c, err := client.New(transport)
	if err != nil {
		closeConn()
		return nil, nil, &IMAPError{Step: "host"}
	}
	// Provider response text can contain credentials or mail literals.
	c.ErrorLog = log.New(io.Discard, "", 0)
	c.Timeout = 30 * time.Second
	if cfg.TLS == "starttls" {
		if err := c.StartTLS(tc); err != nil {
			closeConn()
			return nil, nil, &IMAPError{Step: "tls"}
		}
	}
	if err := c.Login(cfg.Auth.Username, cfg.Auth.Password); err != nil {
		closeConn()
		return nil, nil, &IMAPError{Step: "auth"}
	}
	return c, closeConn, nil
}

// CheckIMAP logs in and selects the configured folder without fetching messages.
func CheckIMAP(ctx context.Context, cfg IMAPConfig, tc *tls.Config) error {
	c, closeConn, err := dialIMAP(ctx, cfg, tc)
	if err != nil {
		return err
	}
	defer closeConn()
	if _, err := c.Select(cfg.Folder, true); err != nil {
		return &IMAPError{Step: "folder"}
	}
	return nil
}

// PollIMAP imports at most 100 messages per pass. Checkpoint runs only after
// Receive and mailbox actions succeed. Retries reuse Message-ID deduplication.
// BODY.PEEK never marks mail read unless the organization explicitly asks.
func (a *Adapter) PollIMAP(ctx context.Context, inst channels.Instance, cursor IMAPCursor, receive func(channels.WebhookResult) error, checkpoint func(IMAPCursor) error, tc *tls.Config) error {
	if tc == nil {
		tc = a.cfg.IMAPTLSConfig
	}
	cfg, err := IMAPConfiguration(inst)
	if err != nil {
		return err
	}
	c, closeConn, err := dialIMAP(ctx, cfg, tc)
	if err != nil {
		return err
	}
	defer closeConn()
	box, err := c.Select(cfg.Folder, !cfg.MarkRead && cfg.MoveFolder == "")
	if err != nil {
		return &IMAPError{Step: "folder"}
	}
	reset := cursor.MailboxKey != cfg.Key() || cursor.UIDValidity != box.UidValidity
	if reset {
		cursor = IMAPCursor{MailboxKey: cfg.Key(), UIDValidity: box.UidValidity, InitialUID: box.UidNext - 1}
	}
	criteria := imap.NewSearchCriteria()
	if cursor.LastUID < cursor.InitialUID {
		criteria.Since = a.now().AddDate(0, 0, -cfg.Days)
	}
	criteria.Uid = new(imap.SeqSet)
	if cursor.LastUID == ^uint32(0) {
		return nil
	}
	criteria.Uid.AddRange(cursor.LastUID+1, 0)
	uids, err := c.UidSearch(criteria)
	if err != nil {
		return &IMAPError{Step: "search"}
	}
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
	// Persist the incarnation even for an empty mailbox.
	if err := checkpoint(cursor); err != nil {
		return err
	}
	imported := 0
	for _, uid := range uids {
		// IMAP n:* includes the highest UID when n exceeds it.
		if uid <= cursor.LastUID {
			continue
		}
		if imported >= 100 {
			break
		}
		set := new(imap.SeqSet)
		set.AddNum(uid)
		section := &imap.BodySectionName{Peek: true, Partial: []int{0, (25 << 20) + 1}}
		// Ask for size first so a large literal is never buffered by the library.
		sizes := make(chan *imap.Message, 1)
		if err := c.UidFetch(set, []imap.FetchItem{imap.FetchRFC822Size}, sizes); err != nil {
			return &IMAPError{Step: "fetch"}
		}
		for msg := range sizes {
			if msg.Size > 25<<20 {
				return &IMAPError{Step: "size"}
			}
		}
		messages := make(chan *imap.Message, 1)
		if err := c.UidFetch(set, []imap.FetchItem{imap.FetchUid, section.FetchItem()}, messages); err != nil {
			return &IMAPError{Step: "fetch"}
		}
		for msg := range messages {
			body := msg.GetBody(section)
			if body == nil {
				return &IMAPError{Step: "fetch"}
			}
			raw, err := io.ReadAll(io.LimitReader(body, (25<<20)+1))
			if err != nil || len(raw) > 25<<20 {
				return &IMAPError{Step: "fetch"}
			}
			res, err := a.ParseRaw(raw, inst, fmt.Sprintf("imap:%s:%d:%d", cfg.Key(), box.UidValidity, uid))
			if err != nil {
				return &IMAPError{Step: "parse"}
			}
			if err := receive(res); err != nil {
				return err
			}
		}
		if cfg.MarkRead {
			if err := c.UidStore(set, imap.FormatFlagsOp(imap.AddFlags, true), []interface{}{imap.SeenFlag}, nil); err != nil {
				return &IMAPError{Step: "store"}
			}
		}
		if cfg.MoveFolder != "" {
			// Require MOVE. No COPY/EXPUNGE fallback: EXPUNGE could delete unrelated mail.
			supported, err := c.Support("MOVE")
			if err != nil || !supported {
				return &IMAPError{Step: "move"}
			}
			if err := c.UidMove(set, cfg.MoveFolder); err != nil {
				return &IMAPError{Step: "move"}
			}
		}
		cursor.LastUID = uid
		if err := checkpoint(cursor); err != nil {
			return err
		}
		imported++
	}
	if imported < 100 && cursor.LastUID < cursor.InitialUID {
		cursor.LastUID = cursor.InitialUID
		return checkpoint(cursor)
	}
	return nil
}

// IsIMAP reports the normalized inbound mode.
func IsIMAP(c channels.Credentials) bool { return provider(c) == ProviderIMAP }
