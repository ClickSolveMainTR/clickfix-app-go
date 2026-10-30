package email

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/textproto"
	"sort"
	"strings"
	"time"
)

// SMTPConfig is an organization's own outgoing server.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	// TLS is "starttls" (587) or "implicit" (465); TLS is required.
	TLS string
}

// Outgoing is a composed message for SMTP.
type Outgoing struct {
	From    Address
	To      []string
	Cc      []string
	Bcc     []string
	Subject string
	Text    string
	HTML    string
	Headers map[string]string
	Files   []File
}

// File is an attachment of an outgoing message.
type File struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Build renders an outgoing message as RFC 5322 bytes:
// multipart/mixed(multipart/alternative(text, html), files…). Bcc is not
// written into the headers.
func Build(m Outgoing) ([]byte, error) {
	for _, v := range append([]string{m.From.Address, m.Subject}, append(m.To, m.Cc...)...) {
		if strings.ContainsAny(v, "\r\n") {
			return nil, errors.New("email: header value contains a line break")
		}
	}
	var b bytes.Buffer
	h := textproto.MIMEHeader{}
	h.Set("From", m.From.String())
	h.Set("To", strings.Join(m.To, ", "))
	if len(m.Cc) > 0 {
		h.Set("Cc", strings.Join(m.Cc, ", "))
	}
	h.Set("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	h.Set("Date", time.Now().UTC().Format(time.RFC1123Z))
	h.Set("MIME-Version", "1.0")
	for k, v := range m.Headers {
		if strings.ContainsAny(k+v, "\r\n") {
			return nil, errors.New("email: header value contains a line break")
		}
		h.Set(k, v)
	}
	alt := boundary()
	body := func(w *bytes.Buffer) {
		fmt.Fprintf(w, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", alt)
		writeQP(w, alt, "text/plain; charset=utf-8", m.Text)
		if m.HTML != "" {
			writeQP(w, alt, "text/html; charset=utf-8", m.HTML)
		}
		fmt.Fprintf(w, "--%s--\r\n", alt)
	}
	writeHeader(&b, h)
	if len(m.Files) == 0 {
		body(&b)
		return b.Bytes(), nil
	}
	mixed := boundary()
	fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", mixed)
	fmt.Fprintf(&b, "--%s\r\n", mixed)
	body(&b)
	for _, f := range m.Files {
		ct := f.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		name := mime.QEncoding.Encode("utf-8", sanitizeFilename(f.Filename))
		fmt.Fprintf(&b, "--%s\r\nContent-Type: %s; name=%q\r\nContent-Disposition: attachment; filename=%q\r\nContent-Transfer-Encoding: base64\r\n\r\n", mixed, ct, name, name)
		enc := base64.StdEncoding.EncodeToString(f.Data)
		for len(enc) > 76 {
			b.WriteString(enc[:76] + "\r\n")
			enc = enc[76:]
		}
		b.WriteString(enc + "\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", mixed)
	return b.Bytes(), nil
}

func writeHeader(b *bytes.Buffer, h textproto.MIMEHeader) {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range h[k] {
			fmt.Fprintf(b, "%s: %s\r\n", k, v)
		}
	}
}

func writeQP(b *bytes.Buffer, boundary, ctype, content string) {
	fmt.Fprintf(b, "--%s\r\nContent-Type: %s\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", boundary, ctype)
	w := quotedprintable.NewWriter(b)
	_, _ = w.Write([]byte(strings.ReplaceAll(normalizeNewlines(content), "\n", "\r\n")))
	_ = w.Close()
	b.WriteString("\r\n")
}

func boundary() string {
	var r [12]byte
	_, _ = rand.Read(r[:])
	return "cf-" + hex.EncodeToString(r[:])
}

// SendSMTP delivers a built message (ported from 216detay-mail-system
// sendSMTP): TLS is mandatory, DATA acceptance is success and a failed QUIT
// never causes a resend.
func SendSMTP(ctx context.Context, c SMTPConfig, from string, rcpt []string, msg []byte, tlsConfig *tls.Config) error {
	if tlsConfig == nil {
		tlsConfig = &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	}
	client, closeConn, err := dialSMTP(ctx, c, tlsConfig)
	if err != nil {
		return err
	}
	defer closeConn()
	if err := client.Mail(from); err != nil {
		return smtpErr("mail from", err)
	}
	for _, r := range rcpt {
		if err := client.Rcpt(r); err != nil {
			return smtpErr("recipient", err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return smtpErr("data", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("email: write smtp message: %w", err)
	}
	if err := w.Close(); err != nil {
		return smtpErr("acceptance", err)
	}
	return nil
}

// CheckSMTP connects and authenticates without sending (connect form).
func CheckSMTP(ctx context.Context, c SMTPConfig, tlsConfig *tls.Config) error {
	if tlsConfig == nil {
		tlsConfig = &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	}
	client, closeConn, err := dialSMTP(ctx, c, tlsConfig)
	if err != nil {
		return err
	}
	defer closeConn()
	return client.Noop()
}

// SMTPError is a rejection by the server; 5xx codes are permanent.
type SMTPError struct {
	Step string
	Code int
	Err  error
}

func (e *SMTPError) Error() string { return "email: smtp " + e.Step + ": " + e.Err.Error() }
func (e *SMTPError) Unwrap() error { return e.Err }

func smtpErr(step string, err error) error {
	var tp *textproto.Error
	if errors.As(err, &tp) {
		return &SMTPError{Step: step, Code: tp.Code, Err: err}
	}
	return &SMTPError{Step: step, Err: err}
}

func dialSMTP(ctx context.Context, c SMTPConfig, tlsConfig *tls.Config) (*smtp.Client, func(), error) {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	address := net.JoinHostPort(c.Host, c.Port)
	var conn net.Conn
	var err error
	if c.TLS == "implicit" {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", address)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("email: connect smtp: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	closeConn := func() { stop(); _ = conn.Close() }
	deadline := time.Now().Add(45 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	client, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		closeConn()
		return nil, nil, smtpErr("greeting", err)
	}
	if c.TLS != "implicit" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			closeConn()
			return nil, nil, errors.New("email: smtp server does not support required STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			closeConn()
			return nil, nil, smtpErr("starttls", err)
		}
	}
	if c.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); err != nil {
			closeConn()
			return nil, nil, smtpErr("authentication", err)
		}
	}
	return client, closeConn, nil
}
