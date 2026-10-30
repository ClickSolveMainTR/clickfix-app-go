package ioengine

import (
	"archive/zip"
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestExtractKnowledgeDocumentsAndBounds(t *testing.T) {
	ctx := context.Background()
	text, err := ExtractKnowledgeDocument(ctx, "guide.md", []byte("# Guide\nRefund within 14 days"))
	if err != nil || !strings.Contains(text, "Refund") {
		t.Fatalf("markdown %q %v", text, err)
	}
	var data bytes.Buffer
	archive := zip.NewWriter(&data)
	file, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Write([]byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Support answer</w:t></w:r></w:p></w:body></w:document>`))
	if err != nil {
		t.Fatal(err)
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	text, err = ExtractKnowledgeDocument(ctx, "guide.docx", data.Bytes())
	if err != nil || text != "Support answer" {
		t.Fatalf("docx %q %v", text, err)
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{{"x.exe", []byte("malicious")}, {"x.md", []byte{0xff}}, {"x.pdf", []byte("not PDF")}, {"x.md", bytes.Repeat([]byte("x"), MaxKnowledgeFileBytes+1)}, {"x.docx", []byte("invalid zip")}} {
		if _, err = ExtractKnowledgeDocument(ctx, tc.name, tc.data); err == nil {
			t.Errorf("accepted invalid %s", tc.name)
		}
	}
}
func TestExtractPDFUsesBoundedTextProcess(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed")
	}
	// Use the engine's PDF encoder to produce an actual PDF fixture locally.
	pdf, err := EncodePDF(Dataset{Columns: []Column{{Key: "answer", LabelKey: "answer", Type: ColumnTypeString}}, Rows: []map[string]any{{"answer": "Refund"}}}, "en", nil, "Support")
	if err != nil {
		t.Fatal(err)
	}
	text, err := ExtractKnowledgeDocument(context.Background(), "guide.pdf", pdf)
	if err != nil || !strings.Contains(text, "Refund") {
		t.Fatalf("pdf %q %v", text, err)
	}
}
