package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
)

// Message is an outbound email. Body is the plain-text part and is always
// sent; when HTML is set the message is multipart/alternative, and Inline
// images (referenced as cid:<CID>) wrap it in multipart/related.
type Message struct {
	To      []string
	Subject string
	Body    string
	HTML    string
	Inline  []Inline
}

// Inline is an image embedded in the HTML part by Content-ID.
type Inline struct {
	CID         string
	Filename    string
	ContentType string
	Data        []byte
}

// Sender delivers email messages.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

var (
	// ErrNotConfigured: SMTP_HOST is empty. Retrying cannot help.
	ErrNotConfigured = errors.New("mail: smtp not configured")
	// ErrInvalidMessage: the message itself is unusable (no or bad recipient).
	ErrInvalidMessage = errors.New("mail: invalid message")
)

// TLS modes (SMTP_TLS_MODE).
const (
	TLSModeStartTLS = "starttls"
	TLSModeImplicit = "implicit"
)

const (
	defaultDialTimeout = 10 * time.Second
	defaultSendTimeout = 30 * time.Second
)

// SendError is a failed SMTP exchange. Stage is where it failed ("dial",
// "greeting", "starttls", "auth", "mail", "rcpt", "data", "quit").
type SendError struct {
	Stage string
	Addr  string
	Err   error
}

func (e *SendError) Error() string {
	return "mail: send: " + e.Stage + " " + e.Addr + ": " + e.Err.Error()
}

func (e *SendError) Unwrap() error { return e.Err }

// SMTPSender sends mail via SMTP (MailHog in development).
type SMTPSender struct {
	host         string
	port         int
	username     string
	password     string
	from         string
	fromName     string
	tlsMode      string
	fallbackPort int
	dialTimeout  time.Duration
	sendTimeout  time.Duration
}

// NewSMTPSender builds a sender from config.
func NewSMTPSender(cfg config.SMTPConfig) *SMTPSender {
	s := &SMTPSender{
		host:         strings.TrimSpace(cfg.Host),
		port:         cfg.Port,
		username:     cfg.Username,
		password:     cfg.Password,
		from:         cfg.From,
		fromName:     cfg.FromName,
		tlsMode:      cfg.TLSMode,
		fallbackPort: cfg.FallbackPort,
		dialTimeout:  cfg.DialTimeout,
		sendTimeout:  cfg.SendTimeout,
	}
	if s.tlsMode != TLSModeImplicit {
		s.tlsMode = TLSModeStartTLS
	}
	if s.dialTimeout <= 0 {
		s.dialTimeout = defaultDialTimeout
	}
	if s.sendTimeout <= 0 {
		s.sendTimeout = defaultSendTimeout
	}
	return s
}

// Configured reports whether an SMTP host is set.
func (s *SMTPSender) Configured() bool { return s != nil && s.host != "" }

// Transport describes the configured transport for health pages (no secrets).
type Transport struct {
	Configured   bool   `json:"configured"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	TLSMode      string `json:"tls_mode"`
	FallbackPort int    `json:"fallback_port,omitempty"`
}

// Transport returns the non-secret transport settings.
func (s *SMTPSender) Transport() Transport {
	if s == nil {
		return Transport{}
	}
	return Transport{Configured: s.Configured(), Host: s.host, Port: s.port, TLSMode: s.tlsMode, FallbackPort: s.fallbackPort}
}

// Send delivers msg: plain text alone, or text + HTML (+ inline images).
// The TCP connect is bounded by the dial timeout and the whole exchange by
// the send timeout (or ctx, if sooner).
// When the primary port cannot be reached and a fallback port is configured,
// the message is retried once over implicit TLS on that port.
func (s *SMTPSender) Send(ctx context.Context, msg Message) error {
	if s == nil {
		return ErrNotConfigured
	}
	if len(msg.To) == 0 {
		return fmt.Errorf("%w: recipient required", ErrInvalidMessage)
	}
	// Header values must not carry CR/LF: subjects interpolate user-chosen
	// names and a newline would inject extra headers (Bcc, etc.).
	for _, to := range msg.To {
		if strings.ContainsAny(to, "\r\n") {
			return fmt.Errorf("%w: invalid recipient", ErrInvalidMessage)
		}
	}
	if !s.Configured() {
		return ErrNotConfigured
	}
	from := s.from
	if from == "" {
		from = "noreply@localhost"
	}
	body, err := s.payload(from, msg)
	if err != nil {
		return err
	}
	err = s.sendVia(ctx, s.port, s.tlsMode == TLSModeImplicit, from, msg.To, body)
	if err == nil || s.fallbackPort <= 0 || s.fallbackPort == s.port || !isDialError(err) || ctx.Err() != nil {
		return err
	}
	fallbackErr := s.sendVia(ctx, s.fallbackPort, true, from, msg.To, body)
	if fallbackErr == nil {
		return nil
	}
	return errors.Join(err, fallbackErr)
}

// Probe connects and reads the greeting (no mail is sent). It reports
// whether the server is reachable from this host.
func (s *SMTPSender) Probe(ctx context.Context) error {
	if !s.Configured() {
		return ErrNotConfigured
	}
	err := s.probeVia(ctx, s.port, s.tlsMode == TLSModeImplicit)
	if err == nil || s.fallbackPort <= 0 || s.fallbackPort == s.port || !isDialError(err) {
		return err
	}
	if fallbackErr := s.probeVia(ctx, s.fallbackPort, true); fallbackErr != nil {
		return errors.Join(err, fallbackErr)
	}
	return nil
}

func (s *SMTPSender) probeVia(ctx context.Context, port int, implicit bool) error {
	c, addr, done, err := s.open(ctx, port, implicit)
	if err != nil {
		return err
	}
	defer done()
	if err := c.Quit(); err != nil {
		return &SendError{Stage: "quit", Addr: addr, Err: err}
	}
	return nil
}

// open dials, applies deadlines and reads the greeting. done closes everything.
func (s *SMTPSender) open(ctx context.Context, port int, implicit bool) (*smtp.Client, string, func(), error) {
	addr := net.JoinHostPort(s.host, strconv.Itoa(port))
	dialCtx, cancel := context.WithTimeout(ctx, s.dialTimeout)
	defer cancel()
	nd := &net.Dialer{}
	var conn net.Conn
	var err error
	if implicit {
		td := &tls.Dialer{NetDialer: nd, Config: s.tlsConfig()}
		conn, err = td.DialContext(dialCtx, "tcp", addr)
	} else {
		conn, err = nd.DialContext(dialCtx, "tcp", addr)
	}
	if err != nil {
		return nil, addr, nil, &SendError{Stage: "dial", Addr: addr, Err: err}
	}
	deadline := time.Now().Add(s.sendTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	// A cancelled context aborts a blocked read or write at once.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		stop()
		_ = conn.Close()
		return nil, addr, nil, &SendError{Stage: "greeting", Addr: addr, Err: err}
	}
	return c, addr, func() { stop(); _ = c.Close() }, nil
}

func (s *SMTPSender) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}
}

func (s *SMTPSender) sendVia(ctx context.Context, port int, implicit bool, from string, to []string, body []byte) error {
	c, addr, done, err := s.open(ctx, port, implicit)
	if err != nil {
		return err
	}
	defer done()
	fail := func(stage string, err error) error { return &SendError{Stage: stage, Addr: addr, Err: err} }
	if !implicit {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(s.tlsConfig()); err != nil {
				return fail("starttls", err)
			}
		}
	}
	if s.username != "" {
		if ok, _ := c.Extension("AUTH"); !ok {
			return fail("auth", errors.New("server does not offer AUTH"))
		}
		if err := c.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return fail("auth", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return fail("mail", err)
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return fail("rcpt", err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fail("data", err)
	}
	if _, err := w.Write(body); err != nil {
		return fail("data", err)
	}
	if err := w.Close(); err != nil {
		return fail("data", err)
	}
	// The message is accepted once DATA is closed; a failed QUIT is harmless.
	_ = c.Quit()
	return nil
}

func (s *SMTPSender) payload(from string, msg Message) ([]byte, error) {
	fromHeader := from
	if s.fromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", s.fromName, from)
	}
	subject := mime.QEncoding.Encode("utf-8", stripHeaderBreaks(msg.Subject))
	payload := strings.Builder{}
	payload.WriteString("From: ")
	payload.WriteString(fromHeader)
	payload.WriteString("\r\n")
	payload.WriteString("To: ")
	payload.WriteString(strings.Join(msg.To, ", "))
	payload.WriteString("\r\n")
	payload.WriteString("Subject: ")
	payload.WriteString(subject)
	payload.WriteString("\r\n")
	payload.WriteString("MIME-Version: 1.0\r\n")
	if err := writeBody(&payload, msg); err != nil {
		return nil, err
	}
	return []byte(payload.String()), nil
}

// writeBody writes the Content-Type header and body of msg. Text parts are
// quoted-printable so long HTML lines stay under the SMTP 998-octet limit.
func writeBody(w *strings.Builder, msg Message) error {
	if msg.HTML == "" {
		w.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		w.WriteString("\r\n")
		w.WriteString(msg.Body)
		return nil
	}
	alt, err := boundary()
	if err != nil {
		return err
	}
	if len(msg.Inline) == 0 {
		writeAlternative(w, alt, msg)
		return nil
	}
	rel, err := boundary()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Content-Type: multipart/related; boundary=%q; type=\"multipart/alternative\"\r\n\r\n", rel)
	fmt.Fprintf(w, "--%s\r\n", rel)
	writeAlternative(w, alt, msg)
	for _, img := range msg.Inline {
		if strings.ContainsAny(img.CID+img.Filename+img.ContentType, "\r\n\"<>") {
			return fmt.Errorf("%w: invalid inline image", ErrInvalidMessage)
		}
		fmt.Fprintf(w, "\r\n--%s\r\n", rel)
		fmt.Fprintf(w, "Content-Type: %s\r\n", img.ContentType)
		w.WriteString("Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(w, "Content-ID: <%s>\r\n", img.CID)
		fmt.Fprintf(w, "Content-Disposition: inline; filename=%q\r\n\r\n", img.Filename)
		writeBase64(w, img.Data)
	}
	fmt.Fprintf(w, "\r\n--%s--\r\n", rel)
	return nil
}

func writeAlternative(w *strings.Builder, alt string, msg Message) {
	fmt.Fprintf(w, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", alt)
	for _, part := range []struct{ ctype, body string }{
		{"text/plain", msg.Body},
		{"text/html", msg.HTML},
	} {
		fmt.Fprintf(w, "--%s\r\n", alt)
		fmt.Fprintf(w, "Content-Type: %s; charset=UTF-8\r\n", part.ctype)
		w.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		w.WriteString(quotedPrintable(part.body))
		w.WriteString("\r\n")
	}
	fmt.Fprintf(w, "--%s--\r\n", alt)
}

func quotedPrintable(s string) string {
	var buf bytes.Buffer
	qp := quotedprintable.NewWriter(&buf)
	_, _ = qp.Write([]byte(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")))
	_ = qp.Close()
	return buf.String()
}

func writeBase64(w *strings.Builder, data []byte) {
	enc := base64.StdEncoding.EncodeToString(data)
	for len(enc) > 76 {
		w.WriteString(enc[:76])
		w.WriteString("\r\n")
		enc = enc[76:]
	}
	w.WriteString(enc)
	w.WriteString("\r\n")
}

func boundary() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("mail: boundary: %w", err)
	}
	return "cf-" + hex.EncodeToString(b[:]), nil
}

func stripHeaderBreaks(v string) string {
	return strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(v)
}

func isDialError(err error) bool {
	var se *SendError
	return errors.As(err, &se) && se.Stage == "dial"
}

// IsTransient reports whether retrying later may succeed: network failures,
// timeouts and SMTP 4xx replies. Missing configuration, invalid messages,
// certificate problems and SMTP 5xx replies are permanent.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	switch Cause(err) {
	case CauseNotConfigured, CauseInvalidMessage, CauseTLS, CauseAuth, CauseRejected:
		return false
	}
	return true
}

// Failure causes (stable, low-cardinality: used for log fields and error
// tracker fingerprints).
const (
	CauseNotConfigured  = "not_configured"
	CauseInvalidMessage = "invalid_message"
	CauseTimeout        = "timeout"
	CauseRefused        = "connection_refused"
	CauseDNS            = "dns"
	CauseNetwork        = "network"
	CauseTLS            = "tls"
	CauseAuth           = "auth"
	CauseTempRejected   = "smtp_4xx"
	CauseRejected       = "smtp_5xx"
	CauseOther          = "other"
)

// Cause classifies err into one of the Cause* values.
func Cause(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrNotConfigured) {
		return CauseNotConfigured
	}
	if errors.Is(err, ErrInvalidMessage) {
		return CauseInvalidMessage
	}
	var tp *textproto.Error
	if errors.As(err, &tp) {
		switch {
		case tp.Code == 535 || tp.Code == 534 || tp.Code == 530:
			return CauseAuth
		case tp.Code >= 500:
			return CauseRejected
		default:
			return CauseTempRejected
		}
	}
	var se *SendError
	if errors.As(err, &se) && se.Stage == "auth" {
		return CauseAuth
	}
	var certErr *tls.CertificateVerificationError
	var unknownAuth x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var invalidCert x509.CertificateInvalidError
	if errors.As(err, &certErr) || errors.As(err, &unknownAuth) || errors.As(err, &hostErr) || errors.As(err, &invalidCert) {
		return CauseTLS
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return CauseDNS
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return CauseRefused
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, syscall.ETIMEDOUT) {
		return CauseTimeout
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return CauseTimeout
	}
	if errors.As(err, &ne) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) || errors.Is(err, net.ErrClosed) {
		return CauseNetwork
	}
	return CauseOther
}

// NoopSender discards mail (tests).
type NoopSender struct{}

// Send implements Sender.
func (NoopSender) Send(context.Context, Message) error { return nil }

var (
	_ Sender = (*SMTPSender)(nil)
	_ Sender = NoopSender{}
)
