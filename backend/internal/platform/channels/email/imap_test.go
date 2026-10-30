package email

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	"github.com/emersion/go-imap/backend/memory"
	"github.com/emersion/go-imap/server"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/google/uuid"
)

// statusBackend overrides UIDVALIDITY so reset behavior is exercised on the wire.
type statusBackend struct {
	*memory.Backend
	validity uint32
}
type statusUser struct {
	backend.User
	be *statusBackend
}
type statusMailbox struct {
	backend.Mailbox
	be *statusBackend
}

func (b *statusBackend) Login(info *imap.ConnInfo, u, p string) (backend.User, error) {
	v, e := b.Backend.Login(info, u, p)
	if e != nil {
		return nil, e
	}
	return &statusUser{User: v, be: b}, nil
}
func (u *statusUser) GetMailbox(name string) (backend.Mailbox, error) {
	v, e := u.User.GetMailbox(name)
	if e != nil {
		return nil, e
	}
	return &statusMailbox{Mailbox: v, be: u.be}, nil
}
func (m *statusMailbox) Status(items []imap.StatusItem) (*imap.MailboxStatus, error) {
	s, e := m.Mailbox.Status(items)
	if e == nil {
		s.UidValidity = m.be.validity
	}
	return s, e
}

func fakeMailbox(t *testing.T, starttls bool) (channels.Instance, *tls.Config, *statusBackend, *memory.Mailbox) {
	t.Helper()
	b := &statusBackend{Backend: memory.New(), validity: 10}
	u, _ := b.Backend.Login(nil, "username", "password")
	mb, _ := u.GetMailbox("INBOX")
	box := mb.(*memory.Mailbox)
	box.Messages = nil
	cert := httptest.NewTLSServer(nil)
	tc := cert.TLS.Clone()
	roots := x509.NewCertPool()
	roots.AddCert(cert.Certificate())
	cert.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(b)
	srv.TLSConfig = tc
	srv.ErrorLog = log.New(io.Discard, "", 0)
	l := listener
	mode := "starttls"
	if !starttls {
		l = tls.NewListener(listener, tc)
		mode = "implicit"
	}
	done := make(chan struct{})
	go func() { _ = srv.Serve(l); close(done) }()
	t.Cleanup(func() { _ = srv.Close(); <-done })
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	inst := channels.Instance{UUID: uuid.New(), Type: channels.TypeEmail, Credentials: channels.Credentials{FieldProvider: ProviderIMAP, FieldIMAPHost: host, FieldIMAPPort: port, FieldIMAPUsername: "username", FieldIMAPPassword: "password", FieldIMAPTLS: mode}, Settings: map[string]any{}}
	return inst, &tls.Config{RootCAs: roots, ServerName: host, MinVersion: tls.VersionTLS12}, b, box
}
func imapMessage(uid uint32, at time.Time, id, reply string) *memory.Message {
	body := fmt.Sprintf("From: user@example.test\r\nTo: support@company.test\r\nMessage-ID: <%s@example.test>\r\nSubject: Help\r\n", id)
	if reply != "" {
		body += "In-Reply-To: <" + reply + "@example.test>\r\nReferences: <" + reply + "@example.test>\r\n"
	}
	body += "Content-Type: text/plain; charset=utf-8\r\n\r\nNew answer\r\n\r\nOn Tuesday someone wrote:\r\n> old answer"
	return &memory.Message{Uid: uid, Date: at, Size: uint32(len(body)), Body: []byte(body)}
}
func TestIMAPWindowCursorAndReset(t *testing.T) {
	for _, starttls := range []bool{false, true} {
		t.Run(fmt.Sprint(starttls), func(t *testing.T) {
			inst, tc, b, box := fakeMailbox(t, starttls)
			a := New(Config{})
			now := time.Now()
			a.now = func() time.Time { return now }
			box.Messages = []*memory.Message{imapMessage(1, now.AddDate(0, 0, -9), "old", ""), imapMessage(2, now, "root", ""), imapMessage(3, now, "reply", "root"), imapMessage(4, now.AddDate(0, 0, -9), "old-high", "")}
			var cursor IMAPCursor
			var messages []channels.InboundMessage
			receive := func(r channels.WebhookResult) error { messages = append(messages, r.Messages...); return nil }
			save := func(c IMAPCursor) error { cursor = c; return nil }
			poll := func() {
				t.Helper()
				if err := a.PollIMAP(context.Background(), inst, cursor, receive, save, tc); err != nil {
					t.Fatal(err)
				}
			}
			poll()
			if len(messages) != 2 || cursor.LastUID != 4 || cursor.UIDValidity != 10 {
				t.Fatalf("initial messages=%d cursor=%+v", len(messages), cursor)
			}
			if messages[1].ThreadID != "<root@example.test>" || strings.Contains(messages[1].Text, "old answer") {
				t.Fatalf("shared parse path not used: %+v", messages[1])
			}
			poll()
			if len(messages) != 2 {
				t.Fatal("cursor replayed messages")
			}
			// Incremental polling must import new UIDs even with an old internal date.
			box.Messages = append(box.Messages, imapMessage(5, now.AddDate(0, 0, -8), "late", "root"))
			poll()
			if len(messages) != 3 {
				t.Fatal("incremental UID missing")
			}
			b.validity = 11
			box.Messages = []*memory.Message{imapMessage(1, now, "reset", "")}
			poll()
			if cursor.LastUID != 1 || cursor.UIDValidity != 11 || len(messages) != 4 {
				t.Fatalf("reset %+v count %d", cursor, len(messages))
			}
		})
	}
}
func TestIMAPAuthTLSAndFolderErrors(t *testing.T) {
	inst, tc, _, _ := fakeMailbox(t, false)
	cfg, err := IMAPConfiguration(inst)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckIMAP(context.Background(), cfg, tc); err != nil {
		t.Fatal(err)
	}
	cfg.Auth.Password = "secret-that-must-not-leak"
	err = CheckIMAP(context.Background(), cfg, tc)
	var ie *IMAPError
	if !errors.As(err, &ie) || ie.Step != "auth" || !strings.Contains(err.Error(), "Gmail") || strings.Contains(err.Error(), cfg.Auth.Password) {
		t.Fatalf("auth error: %v", err)
	}
	cfg.Auth.Password = "password"
	cfg.Folder = "missing"
	err = CheckIMAP(context.Background(), cfg, tc)
	if !errors.As(err, &ie) || ie.Step != "folder" {
		t.Fatalf("folder %v", err)
	}
	cfg.Folder = "INBOX"
	err = CheckIMAP(context.Background(), cfg, nil)
	if !errors.As(err, &ie) || ie.Step != "tls" {
		t.Fatalf("TLS %v", err)
	}
}
func TestIMAPCheckpointAfterReceiveAndRead(t *testing.T) {
	inst, tc, _, box := fakeMailbox(t, false)
	inst.Settings[SettingIMAPRead] = true
	box.Messages = []*memory.Message{imapMessage(1, time.Now(), "one", "")}
	a := New(Config{})
	var cursor IMAPCursor
	save := func(c IMAPCursor) error { cursor = c; return nil }
	err := a.PollIMAP(context.Background(), inst, cursor, func(channels.WebhookResult) error { return errors.New("storage failed") }, save, tc)
	if err == nil || cursor.LastUID != 0 {
		t.Fatal("advanced past receive failure")
	}
	if err := a.PollIMAP(context.Background(), inst, cursor, func(channels.WebhookResult) error { return nil }, save, tc); err != nil {
		t.Fatal(err)
	}
	if cursor.LastUID != 1 || len(box.Messages[0].Flags) != 1 || box.Messages[0].Flags[0] != imap.SeenFlag {
		t.Fatalf("read/cursor %+v %+v", cursor, box.Messages[0].Flags)
	}
}

func TestIMAPConnectAndOutboundUseMailbox(t *testing.T) {
	inst := instance()
	inst.Credentials = channels.Credentials{FieldProvider: ProviderIMAP, FieldIMAPHost: "imap.example.test", FieldIMAPUsername: "support", FieldIMAPPassword: "secret", FieldSMTPHost: "smtp.example.test", FieldFromAddress: "support@example.test"}
	a := New(Config{}) // No Resend config required.
	imapChecked, smtpChecked := false, false
	a.imapTest = func(context.Context, IMAPConfig) error { imapChecked = true; return nil }
	a.smtpTest = func(context.Context, SMTPConfig) error { smtpChecked = true; return nil }
	if _, err := a.Connect(context.Background(), inst); err != nil {
		t.Fatal(err)
	}
	if !imapChecked || !smtpChecked {
		t.Fatal("connect did not test both servers")
	}
	a.smtpSend = func(_ context.Context, _ SMTPConfig, _ string, _ []string, raw []byte) error {
		if !strings.Contains(string(raw), "Reply-To: support@example.test") {
			t.Fatalf("wrong reply mailbox: %s", raw)
		}
		return nil
	}
	_, err := a.Send(context.Background(), inst, channels.OutboundMessage{MessageUUID: uuid.New(), To: channels.Recipient{Email: "customer@example.test"}, Text: "Answer"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIMAPPagesKeepInitialWindow(t *testing.T) {
	inst, tc, _, box := fakeMailbox(t, false)
	a := New(Config{})
	now := time.Now()
	for uid := uint32(1); uid <= 101; uid++ {
		box.Messages = append(box.Messages, imapMessage(uid, now, fmt.Sprintf("m%d", uid), ""))
	}
	// A higher UID outside the first sync window must stay excluded on page two.
	box.Messages = append(box.Messages, imapMessage(102, now.AddDate(0, 0, -10), "old-high", ""))
	var cursor IMAPCursor
	count := 0
	save := func(c IMAPCursor) error { cursor = c; return nil }
	receive := func(r channels.WebhookResult) error { count += len(r.Messages); return nil }
	for i := 0; i < 2; i++ {
		if err := a.PollIMAP(context.Background(), inst, cursor, receive, save, tc); err != nil {
			t.Fatal(err)
		}
	}
	if count != 101 || cursor.LastUID != 102 {
		t.Fatalf("window count=%d cursor=%+v", count, cursor)
	}
}
