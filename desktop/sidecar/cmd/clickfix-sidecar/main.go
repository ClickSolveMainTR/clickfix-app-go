// Komut clickfix-sidecar, ClickFix masaüstü uygulamalarının Go yardımcı
// sürecidir. Kabuk (Tauri) onu başlatır ve stdin/stdout üzerinden satır
// sınırlı JSON-RPC ile konuşur (bkz. internal/ipc, IPC.md). Günlükler
// maskelenerek stderr'e ve dönen bir dosyaya yazılır; stdout yalnız
// protokol içindir.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/api"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/capture"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/input"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/ipc"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/logging"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/remote"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/support"
	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/sysinfo"
)

// version, derlemede -ldflags "-X main.version=..." ile desktop/VERSION'dan gelir.
var version = "dev"

// defaultAppURL, CLICKFIX_APP_URL verilmediğindeki API köküdür.
const defaultAppURL = "https://getclickfix.com"

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "clickfix-sidecar:", err)
		os.Exit(2)
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("clickfix-sidecar", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mode := fs.String("mode", "", "çalışma modu: agent | support")
	showVersion := fs.Bool("version", false, "sürümü yaz ve çık")
	logDir := fs.String("log-dir", logging.DefaultDir(), "günlük dizini (boş: yalnız stderr)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		_, err := fmt.Fprintln(stdout, version)
		return err
	}
	switch *mode {
	case "agent", "support":
	default:
		return fmt.Errorf("--mode agent ya da support olmalı (verilen %q)", *mode)
	}

	logger, closer, lerr := logging.New(logging.Options{Dir: *logDir, Stderr: stderr})
	defer func() { _ = closer.Close() }()
	logger = logger.With("mode", *mode)
	if lerr != nil {
		logger.Warn("günlük dosyası açılamadı, yalnız stderr", "err", lerr)
	}
	// Standart log paketi ve LiveKit SDK da maskeleyen işleyiciye yazar.
	log.SetOutput(slogWriter{logger})
	log.SetFlags(0)
	remote.SetLogger(logger)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := ipc.NewServer(stdout)
	ipc.RegisterBuiltins(srv, ipc.NewInfo(version, *mode))

	var svc *support.Service
	if *mode == "support" {
		var err error
		svc, err = newSupport(srv, logger)
		if err != nil {
			return err
		}
	}

	if err := srv.Notify("ready", map[string]string{"mode": *mode, "version": version}); err != nil {
		return fmt.Errorf("ready olayı yazılamadı: %w", err)
	}
	logger.Info("sidecar başladı", "version", version)

	err := srv.Serve(ctx, stdin)
	if errors.Is(err, context.Canceled) {
		err = nil
	}
	if svc != nil {
		// Kabuk kapandı (stdin EOF) ya da sinyal geldi: süren oturum bitirilir.
		sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		svc.Shutdown(sctx)
		cancel()
	}
	logger.Info("sidecar durdu")
	return err
}

func newSupport(srv *ipc.Server, logger *slog.Logger) (*support.Service, error) {
	appURL := os.Getenv("CLICKFIX_APP_URL")
	if appURL == "" {
		appURL = defaultAppURL
	}
	osName := sysinfo.ShortOS()
	client, err := api.New(appURL, api.UserAgent(version, osName))
	if err != nil {
		return nil, fmt.Errorf("CLICKFIX_APP_URL: %w", err)
	}
	svc := support.New(support.Config{
		API:     client,
		Version: version,
		Logger:  logger,
		Notify:  func(event string, params any) { _ = srv.Notify(event, params) },
		Platform: support.Platform{
			Name:                    runtime.GOOS,
			ScreenPermission:        capture.ScreenPermission,
			RequestScreenPermission: capture.RequestScreenPermission,
			Accessibility:           input.Accessibility,
			RequestAccessibility:    input.RequestAccessibility,
			MainDisplay:             capture.MainDisplay,
			StartCapture:            capture.Start,
			Injector:                input.NewInjector(nil),
		},
	})
	svc.Register(srv)
	return svc, nil
}

// slogWriter, standart log satırlarını slog'a aktarır.
type slogWriter struct{ l *slog.Logger }

func (w slogWriter) Write(p []byte) (int, error) {
	msg := string(p)
	for len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	w.l.Info(msg, "source", "log")
	return len(p), nil
}
