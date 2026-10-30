package ioengine

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
)

// Archive streams hierarchical subject exports to a private temporary file.
// Names are server-owned UUID paths; user filenames never become archive paths.
type Archive struct {
	writer    *zip.Writer
	remaining int64
}

func NewArchive(w io.Writer, maxBytes int64) *Archive {
	return &Archive{writer: zip.NewWriter(w), remaining: maxBytes}
}
func (a *Archive) Add(name string, r io.Reader) error {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || path.Clean(name) != name || strings.HasPrefix(name, "../") {
		return fmt.Errorf("archive: invalid path")
	}
	w, err := a.writer.Create(name)
	if err != nil {
		return err
	}
	n, err := io.Copy(w, io.LimitReader(r, a.remaining+1))
	a.remaining -= n
	if a.remaining < 0 {
		return fmt.Errorf("archive: export exceeds size limit")
	}
	return err
}
func (a *Archive) JSON(name string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return a.Add(name, strings.NewReader(string(b)))
}
func (a *Archive) Close() error { return a.writer.Close() }
