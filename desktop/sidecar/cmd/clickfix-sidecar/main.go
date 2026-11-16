// Komut clickfix-sidecar, ClickFix masaüstü uygulamalarının Go yardımcı
// sürecidir. Kabuk (Tauri) onu başlatır ve stdin/stdout üzerinden satır
// sınırlı JSON-RPC ile konuşur (bkz. internal/ipc). Günlükler stderr'e
// yazılır; stdout yalnız protokol içindir.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/furkanmeclis/clickfix-app-go/desktop/sidecar/internal/ipc"
)

// version, derlemede -ldflags "-X main.version=..." ile desktop/VERSION'dan gelir.
var version = "dev"

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

	logger := slog.New(slog.NewTextHandler(stderr, nil)).With("mode", *mode)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := ipc.NewServer(stdout)
	ipc.RegisterBuiltins(srv, ipc.NewInfo(version, *mode))

	if err := srv.Notify("ready", map[string]string{"mode": *mode, "version": version}); err != nil {
		return fmt.Errorf("ready olayı yazılamadı: %w", err)
	}
	logger.Info("sidecar başladı", "version", version)

	err := srv.Serve(ctx, stdin)
	if errors.Is(err, context.Canceled) {
		err = nil
	}
	logger.Info("sidecar durdu")
	return err
}
