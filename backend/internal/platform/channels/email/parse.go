package email

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/htmlindex"
)

// Address is one mailbox of a header.
type Address struct {
	Name    string `json:"name,omitempty"`
	Address string `json:"address"`
}

func (a Address) String() string {
	return (&mail.Address{Name: a.Name, Address: a.Address}).String()
}

// Part is a file of a parsed message (attachment or inline image).
type Part struct {
	Filename    string
	ContentType string
	ContentID   string
	Inline      bool
	Data        []byte
}

// Parsed is a decoded RFC 5322 message.
type Parsed struct {
	Header     textproto.MIMEHeader
	From       Address
	ReplyTo    []Address
	To         []Address
	Cc         []Address
	Subject    string
	MessageID  string
	InReplyTo  []string
	References []string
	Date       time.Time
	Text       string
	HTML       string
	Parts      []Part
	// Report is set for a delivery status notification (bounce).
	Report *Report
}

// Report is the machine-readable part of a bounce (RFC 3464).
type Report struct {
	// Action is failed, delayed, delivered, relayed or expanded.
	Action            string
	Status            string
	Diagnostic        string
	Recipient         string
	OriginalMessageID string
}

// Limits of the parser: a message bigger than MaxMessageBytes is rejected,
// nesting deeper than maxDepth is not walked.
const (
	MaxMessageBytes = 30 << 20
	maxDepth        = 8
)

var wordDecoder = &mime.WordDecoder{CharsetReader: charsetReader}

// Parse decodes a raw message: headers, the text and HTML bodies (converted
// to UTF-8), files, and a delivery report when the message is a bounce.
func Parse(raw []byte) (*Parsed, error) {
	if len(raw) > MaxMessageBytes {
		return nil, errors.New("email: message too large")
	}
	msg, err := mail.ReadMessage(bufio.NewReader(bytes.NewReader(raw)))
	if err != nil {
		return nil, fmt.Errorf("email: read message: %w", err)
	}
	h := textproto.MIMEHeader(msg.Header)
	p := &Parsed{Header: h}
	p.Subject = decodeHeader(h.Get("Subject"))
	p.From = firstAddress(h.Get("From"))
	p.ReplyTo = addressList(h.Get("Reply-To"))
	p.To = addressList(h.Get("To"))
	p.Cc = addressList(h.Get("Cc"))
	p.MessageID = firstID(h.Get("Message-Id"))
	p.InReplyTo = msgIDs(h.Get("In-Reply-To"))
	p.References = msgIDs(h.Get("References"))
	if d, err := mail.ParseDate(h.Get("Date")); err == nil {
		p.Date = d
	}
	if err := p.walk(h, msg.Body, 0); err != nil {
		return nil, err
	}
	if p.Text == "" && p.HTML != "" {
		p.Text = HTMLToText(p.HTML)
	}
	p.Text = strings.TrimSpace(normalizeNewlines(p.Text))
	return p, nil
}

func (p *Parsed) walk(h textproto.MIMEHeader, body io.Reader, depth int) error {
	if depth > maxDepth {
		return nil
	}
	ctype, params, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil || ctype == "" {
		ctype, params = "text/plain", map[string]string{"charset": "us-ascii"}
	}
	if strings.HasPrefix(ctype, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return nil
		}
		mr := multipart.NewReader(body, boundary)
		for {
			part, err := mr.NextRawPart()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				// A truncated multipart keeps what was read so far.
				return nil
			}
			if err := p.walk(part.Header, part, depth+1); err != nil {
				return err
			}
		}
	}
	data, err := io.ReadAll(decodeTransfer(h.Get("Content-Transfer-Encoding"), body))
	if err != nil {
		return fmt.Errorf("email: read part: %w", err)
	}
	disposition, dparams, _ := mime.ParseMediaType(h.Get("Content-Disposition"))
	filename := decodeHeader(dparams["filename"])
	if filename == "" {
		filename = decodeHeader(params["name"])
	}
	attached := disposition == "attachment" || (filename != "" && disposition != "inline" && !strings.HasPrefix(ctype, "text/"))
	switch {
	case ctype == "message/delivery-status":
		p.Report = parseReport(data, p.Report)
		return nil
	case ctype == "text/rfc822-headers" || (ctype == "message/rfc822" && p.Report != nil):
		if hdr, err := textproto.NewReader(bufio.NewReader(io.MultiReader(bytes.NewReader(data), strings.NewReader("\r\n\r\n")))).ReadMIMEHeader(); err == nil {
			if p.Report == nil {
				p.Report = &Report{}
			}
			if id := firstID(hdr.Get("Message-Id")); id != "" {
				p.Report.OriginalMessageID = id
			}
		}
		return nil
	case !attached && ctype == "text/plain" && p.Text == "":
		p.Text = toUTF8(data, params["charset"])
		return nil
	case !attached && ctype == "text/html" && p.HTML == "":
		p.HTML = toUTF8(data, params["charset"])
		return nil
	case !attached && strings.HasPrefix(ctype, "text/"):
		return nil
	}
	if len(data) == 0 {
		return nil
	}
	if filename == "" {
		filename = "attachment"
		if exts, _ := mime.ExtensionsByType(ctype); len(exts) > 0 {
			filename += exts[0]
		}
	}
	p.Parts = append(p.Parts, Part{
		Filename: sanitizeFilename(filename), ContentType: ctype, ContentID: strings.Trim(h.Get("Content-Id"), "<> "),
		Inline: disposition == "inline", Data: data,
	})
	return nil
}

func decodeTransfer(enc string, r io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(enc)) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, &base64Cleaner{r: r})
	case "quoted-printable":
		return quotedprintable.NewReader(r)
	}
	return r
}

// base64Cleaner drops the line breaks and spaces mailers put into base64.
type base64Cleaner struct{ r io.Reader }

func (c *base64Cleaner) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	j := 0
	for i := 0; i < n; i++ {
		switch p[i] {
		case '\r', '\n', ' ', '\t':
			continue
		}
		p[j] = p[i]
		j++
	}
	if j == 0 && n > 0 && err == nil {
		return c.Read(p)
	}
	return j, err
}

func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	enc, err := htmlindex.Get(charset)
	if err != nil {
		return nil, err
	}
	return enc.NewDecoder().Reader(input), nil
}

// toUTF8 converts a body from its declared charset; undeclared non-UTF-8
// bytes are read as windows-1254 (the usual Turkish mail charset).
func toUTF8(data []byte, charset string) string {
	charset = strings.ToLower(strings.TrimSpace(charset))
	if charset == "" || charset == "utf-8" || charset == "us-ascii" || charset == "utf8" {
		if utf8.Valid(data) {
			return string(data)
		}
		charset = "windows-1254"
	}
	enc, err := htmlindex.Get(charset)
	if err != nil {
		return strings.ToValidUTF8(string(data), "�")
	}
	out, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		return strings.ToValidUTF8(string(data), "�")
	}
	return string(out)
}

func decodeHeader(v string) string {
	if v == "" {
		return ""
	}
	out, err := wordDecoder.DecodeHeader(v)
	if err != nil {
		return strings.TrimSpace(v)
	}
	return strings.TrimSpace(out)
}

func addressList(v string) []Address {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parser := mail.AddressParser{WordDecoder: wordDecoder}
	list, err := parser.ParseList(v)
	if err != nil {
		// Lenient fallback: pick anything that looks like an address.
		var out []Address
		for _, m := range looseAddrRE.FindAllString(v, -1) {
			out = append(out, Address{Address: strings.ToLower(m)})
		}
		return out
	}
	out := make([]Address, 0, len(list))
	for _, a := range list {
		out = append(out, Address{Name: a.Name, Address: strings.ToLower(a.Address)})
	}
	return out
}

func firstAddress(v string) Address {
	if l := addressList(v); len(l) > 0 {
		return l[0]
	}
	return Address{}
}

var (
	looseAddrRE = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	msgIDRE     = regexp.MustCompile(`<[^<>\s]+>`)
)

// msgIDs returns the <ids> of a Message-ID list header, in order.
func msgIDs(v string) []string {
	ids := msgIDRE.FindAllString(v, -1)
	if len(ids) == 0 {
		if f := strings.Fields(v); len(f) == 1 && strings.Contains(f[0], "@") {
			return []string{"<" + strings.Trim(f[0], "<>") + ">"}
		}
	}
	return ids
}

func firstID(v string) string {
	if ids := msgIDs(v); len(ids) > 0 {
		return ids[0]
	}
	return ""
}

func parseReport(data []byte, into *Report) *Report {
	r := into
	if r == nil {
		r = &Report{}
	}
	// A delivery-status body is a series of header blocks separated by
	// blank lines (per-message, then one per recipient).
	for _, block := range strings.Split(normalizeNewlines(string(data)), "\n\n") {
		hdr, err := textproto.NewReader(bufio.NewReader(strings.NewReader(block + "\n\n"))).ReadMIMEHeader()
		if err != nil && len(hdr) == 0 {
			continue
		}
		if v := hdr.Get("Action"); v != "" && r.Action == "" {
			r.Action = strings.ToLower(strings.TrimSpace(v))
			r.Status = strings.TrimSpace(hdr.Get("Status"))
			r.Diagnostic = strings.TrimSpace(hdr.Get("Diagnostic-Code"))
			r.Recipient = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(hdr.Get("Final-Recipient")), "rfc822;"))
		}
		if v := firstID(hdr.Get("Original-Message-Id")); v != "" {
			r.OriginalMessageID = v
		}
	}
	return r
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if len([]rune(name)) > 200 {
		name = string([]rune(name)[:200])
	}
	if name == "" || name == "." || name == ".." {
		return "attachment"
	}
	return name
}
