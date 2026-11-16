package logging

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// Varsayılan döndürme sınırları.
const (
	DefaultMaxBytes = 5 << 20
	DefaultBackups  = 3
)

// RotatingFile, boyutu MaxBytes'ı aşınca dosyayı name.1, name.2 ... olarak
// kaydıran bir io.WriteCloser'dır. En çok Backups eski dosya tutulur.
type RotatingFile struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	backups  int
	f        *os.File
	size     int64
}

// OpenRotating, dizini oluşturur ve dosyayı ekleme kipinde açar.
func OpenRotating(path string, maxBytes int64, backups int) (*RotatingFile, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if backups < 0 {
		backups = 0
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	r := &RotatingFile{path: path, maxBytes: maxBytes, backups: backups}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *RotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	r.f, r.size = f, st.Size()
	return nil
}

func (r *RotatingFile) rotate() error {
	if err := r.f.Close(); err != nil {
		return err
	}
	if r.backups == 0 {
		if err := os.Remove(r.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return r.open()
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", r.path, r.backups))
	for i := r.backups - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", r.path, i), fmt.Sprintf("%s.%d", r.path, i+1))
	}
	if err := os.Rename(r.path, r.path+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return r.open()
}

// Write tek bir günlük kaydını yazar; kayıt sınırı aşacaksa önce döndürür.
func (r *RotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return 0, os.ErrClosed
	}
	if r.size > 0 && r.size+int64(len(p)) > r.maxBytes {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

// Close dosyayı kapatır.
func (r *RotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

// Options günlük kurulumudur.
type Options struct {
	// Dir boşsa yalnız Stderr'e yazılır.
	Dir      string
	Stderr   io.Writer
	Level    slog.Leveler
	MaxBytes int64
	Backups  int
}

// DefaultDir, --log-dir verilmediğinde kullanılan dizindir.
func DefaultDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "ClickFix", "sidecar-logs")
}

// New, maskeleyen bir *slog.Logger ve kapatıcı döndürür. Dosya
// açılamazsa günlük yalnız stderr'e gider ve hata döner (sidecar yine çalışır).
func New(o Options) (*slog.Logger, io.Closer, error) {
	ho := &slog.HandlerOptions{Level: o.Level, ReplaceAttr: ReplaceAttr}
	w := io.Discard
	if o.Stderr != nil {
		w = o.Stderr
	}
	var closer io.Closer = nopCloser{}
	var ferr error
	if o.Dir != "" {
		backups := o.Backups
		if backups == 0 {
			backups = DefaultBackups
		}
		f, err := OpenRotating(filepath.Join(o.Dir, "sidecar.log"), o.MaxBytes, backups)
		if err != nil {
			ferr = err
		} else {
			w = io.MultiWriter(w, f)
			closer = f
		}
	}
	return slog.New(slog.NewTextHandler(w, ho)), closer, ferr
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }
