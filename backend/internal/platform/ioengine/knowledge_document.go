package ioengine

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxKnowledgeFileBytes = 8 << 20
const maxKnowledgeTextBytes = 180000

type cappedText struct{ bytes.Buffer }

func (b *cappedText) Write(p []byte) (int, error) {
	if b.Len()+len(p) > maxKnowledgeTextBytes {
		return 0, errors.New("ioengine: extracted document too large")
	}
	return b.Buffer.Write(p)
}

// ExtractKnowledgeDocument extends the existing import engine for unstructured
// documents. Bounds cover compressed bytes, expanded XML, extracted text and PDF
// execution time. No shell command or user-controlled path is executed.
func ExtractKnowledgeDocument(ctx context.Context, name string, data []byte) (string, error) {
	if len(data) == 0 || len(data) > MaxKnowledgeFileBytes {
		return "", errors.New("ioengine: invalid document size")
	}
	var text string
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".txt":
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return "", errors.New("ioengine: invalid UTF-8 document")
		}
		text = string(data)
	case ".docx":
		archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return "", err
		}
		found := false
		for _, f := range archive.File {
			if f.Name != "word/document.xml" {
				continue
			}
			if f.UncompressedSize64 > 4<<20 {
				return "", errors.New("ioengine: expanded document too large")
			}
			r, err := f.Open()
			if err != nil {
				return "", err
			}
			raw, err := io.ReadAll(io.LimitReader(r, (4<<20)+1))
			_ = r.Close()
			if err != nil || len(raw) > 4<<20 {
				return "", errors.New("ioengine: invalid document XML")
			}
			decoder := xml.NewDecoder(bytes.NewReader(raw))
			var out strings.Builder
			inText := false
			for {
				tok, e := decoder.Token()
				if errors.Is(e, io.EOF) {
					break
				}
				if e != nil {
					return "", e
				}
				switch t := tok.(type) {
				case xml.StartElement:
					if t.Name.Local == "t" {
						inText = true
					}
				case xml.EndElement:
					if t.Name.Local == "t" {
						inText = false
					}
					if t.Name.Local == "p" {
						out.WriteByte('\n')
					}
				case xml.CharData:
					if inText {
						out.Write(t)
					}
				}
				if out.Len() > maxKnowledgeTextBytes {
					return "", errors.New("ioengine: document too large")
				}
			}
			text = out.String()
			found = true
			break
		}
		if !found {
			return "", errors.New("ioengine: missing DOCX document")
		}
	case ".pdf":
		if !bytes.HasPrefix(data, []byte("%PDF-")) {
			return "", errors.New("ioengine: invalid PDF")
		}
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "pdftotext", "-enc", "UTF-8", "-", "-")
		cmd.Stdin = bytes.NewReader(data)
		out := &cappedText{}
		cmd.Stdout = out
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			return "", errors.New("ioengine: PDF extraction failed (pdftotext required)")
		}
		text = out.String()
	default:
		return "", errors.New("ioengine: unsupported knowledge format")
	}
	text = strings.TrimSpace(text)
	if text == "" || len(text) > maxKnowledgeTextBytes || !utf8.ValidString(text) {
		return "", errors.New("ioengine: empty or oversized document; scanned PDFs require OCR")
	}
	return text, nil
}
