// Package ipc, masaüstü kabuğu (Tauri/Rust) ile sidecar arasındaki
// stdin/stdout satır sınırlı JSON-RPC protokolünü uygular.
//
// Her satır tek bir JSON nesnesidir:
//
//	istek: {"id": 1, "method": "ping", "params": {...}}
//	yanıt: {"id": 1, "result": ...} ya da {"id": 1, "error": {"code": -32601, "message": "..."}}
//	olay:  {"event": "ready", "params": {...}}
//
// İstekler eşzamanlı işlenir; yanıtlar bitiş sırasına göre yazılır ve
// istemci onları id ile eşler. Sidecar ağ portu açmaz, yalnız ebeveyninden
// stdin üzerinden komut alır.
package ipc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// JSON-RPC 2.0 ile uyumlu hata kodları.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternal       = -32603
)

// MaxLineBytes, tek bir satırın en büyük boyutudur (1 MiB).
const MaxLineBytes = 1 << 20

// Request, kabuktan gelen bir komuttur.
type Request struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Error, yanıttaki hata nesnesidir.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	// Data, isteğe bağlı makine okunur ayrıntıdır (ör. {"kind": "NETWORK"}).
	Data any `json:"data,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("ipc: %d %s", e.Code, e.Message) }

// Errorf, belirli bir kodla hata üretir; işleyiciler bunu döndürebilir.
func Errorf(code int, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Response, bir isteğin yanıtıdır. ID, istekteki değerin aynısıdır;
// istek çözülemediyse null olur.
type Response struct {
	ID     json.RawMessage `json:"id"`
	Result any             `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Event, kabuğa gönderilen uzun ömürlü bir bildirimdir.
type Event struct {
	Event  string `json:"event"`
	Params any    `json:"params,omitempty"`
}

// Handler bir yöntemi uygular. Dönen *Error kodu korunur; başka hatalar
// CodeInternal olarak yazılır.
type Handler func(ctx context.Context, params json.RawMessage) (any, error)

// Server, yöntem kaydını ve çıktı yazıcısını tutar.
type Server struct {
	mu       sync.Mutex // çıktıyı korur
	out      io.Writer
	handlers map[string]Handler
}

// NewServer, out'a yazan bir sunucu döndürür.
func NewServer(out io.Writer) *Server {
	return &Server{out: out, handlers: map[string]Handler{}}
}

// Handle bir yöntemi kaydeder. Serve başlamadan önce çağrılmalıdır.
func (s *Server) Handle(method string, h Handler) {
	s.handlers[method] = h
}

// Notify bir olay satırı yazar.
func (s *Server) Notify(event string, params any) error {
	return s.write(Event{Event: event, Params: params})
}

func (s *Server) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.out.Write(b)
	return err
}

// Serve, in kapanana (EOF) ya da ctx bitene kadar istekleri okur. Dönmeden
// önce süren işleyicilerin bitmesini bekler. EOF normal kapanıştır ve nil
// döner.
func (s *Server) Serve(ctx context.Context, in io.Reader) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	defer wg.Wait()

	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), MaxLineBytes)

	lines := make(chan []byte)
	scanErr := make(chan error, 1)
	go func() {
		defer close(lines)
		for sc.Scan() {
			line := bytes.Clone(sc.Bytes())
			select {
			case lines <- line:
			case <-ctx.Done():
				return
			}
		}
		scanErr <- sc.Err()
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case line, ok := <-lines:
			if !ok {
				err := <-scanErr
				if errors.Is(err, bufio.ErrTooLong) {
					_ = s.write(Response{ID: nullID, Error: Errorf(CodeParseError, "satır %d baytı aşıyor", MaxLineBytes)})
				}
				return err
			}
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			req, rerr := decode(line)
			if rerr != nil {
				_ = s.write(Response{ID: idOrNull(req), Error: rerr})
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.dispatch(ctx, req)
			}()
		}
	}
}

var nullID = json.RawMessage("null")

func idOrNull(req Request) json.RawMessage {
	if len(req.ID) == 0 {
		return nullID
	}
	return req.ID
}

func decode(line []byte) (Request, *Error) {
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return Request{}, Errorf(CodeParseError, "geçersiz JSON: %v", err)
	}
	if len(req.ID) == 0 || bytes.Equal(req.ID, nullID) {
		return req, Errorf(CodeInvalidRequest, "id zorunlu")
	}
	if req.ID[0] != '"' && (req.ID[0] < '0' || req.ID[0] > '9') && req.ID[0] != '-' {
		return Request{}, Errorf(CodeInvalidRequest, "id sayı ya da metin olmalı")
	}
	if req.Method == "" {
		return req, Errorf(CodeInvalidRequest, "method zorunlu")
	}
	return req, nil
}

func (s *Server) dispatch(ctx context.Context, req Request) {
	h, ok := s.handlers[req.Method]
	if !ok {
		_ = s.write(Response{ID: req.ID, Error: Errorf(CodeMethodNotFound, "bilinmeyen yöntem: %s", req.Method)})
		return
	}
	result, err := safeCall(ctx, h, req.Params)
	if err != nil {
		var e *Error
		if !errors.As(err, &e) {
			e = Errorf(CodeInternal, "%v", err)
		}
		_ = s.write(Response{ID: req.ID, Error: e})
		return
	}
	if result == nil {
		result = struct{}{}
	}
	_ = s.write(Response{ID: req.ID, Result: result})
}

// safeCall, bir işleyicideki panik sidecar'ı düşürmesin diye onu hataya çevirir.
func safeCall(ctx context.Context, h Handler, params json.RawMessage) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = Errorf(CodeInternal, "işleyici paniği: %v", r)
		}
	}()
	return h(ctx, params)
}
