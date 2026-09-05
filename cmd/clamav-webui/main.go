// Command clamav-webui runs a web UI to install, update, and operate ClamAV on a
// headless Debian/Ubuntu server: on-demand and scheduled scans, signature
// updates, quarantine, daemon and on-access configuration, and notifications.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FlightlessWeasel/clamav-webui/internal/api"
	"github.com/FlightlessWeasel/clamav-webui/internal/config"
	"github.com/FlightlessWeasel/clamav-webui/internal/db"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", os.Getenv("CLAMWEB_CONFIG_FILE"), "path to JSON config file (optional)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		os.Stdout.WriteString(version + "\n")
		return nil
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	setupLogging(cfg.LogLevel)
	slog.Info("starting", "version", version, "config", cfg.String())

	if err := cfg.EnsureDirs(); err != nil {
		return err
	}

	database, err := db.Open(cfg.DBPath())
	if err != nil {
		return err
	}
	defer database.Close()

	apiSrv, err := api.New(cfg, database, version)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           apiSrv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Addr, "tls", cfg.TLSEnabled())
		var err error
		if cfg.TLSEnabled() {
			err = srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func setupLogging(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})))
}
