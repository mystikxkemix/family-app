// Command family-api runs the Family app HTTP API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"familyapp/internal/api"
	"familyapp/internal/arr"
	"familyapp/internal/config"
	"familyapp/internal/push"
	"familyapp/internal/store"
)

// version is set at build time: -ldflags "-X main.version=1.42".
var version = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("stopped", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	st, err := store.Open(startCtx, cfg.MongoURI, cfg.MongoDB)
	if err != nil {
		return err
	}
	defer st.Close(context.Background())

	notifier, err := newNotifier(cfg, st, log)
	if err != nil {
		return err
	}
	defer notifier.Close()

	server, err := api.New(startCtx, st, notifier, log, api.Options{
		SessionTTL:     cfg.SessionTTL,
		ClientIPHeader: cfg.ClientIPHeader,
		Version:        version,
		Arr:            arr.New(cfg.Radarr, cfg.Sonarr, cfg.Prowlarr),
	})
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	errs := make(chan error, 1)
	go func() {
		log.Info("listening", "port", cfg.Port, "version", version, "push", notifier.Enabled(),
			"radarr", cfg.Radarr.Enabled(), "sonarr", cfg.Sonarr.Enabled(), "prowlarr", cfg.Prowlarr.Enabled())
		errs <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelShutdown()
	if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// newNotifier loads the APNs keys. Without a config file, push stays off.
func newNotifier(cfg config.Config, st *store.Store, log *slog.Logger) (*push.Notifier, error) {
	if cfg.APNSConfigFile == "" {
		log.Info("push notifications off: APNS_CONFIG_FILE is not set")
		return push.NewNotifier(st, nil, log), nil
	}
	if _, err := os.Stat(cfg.APNSConfigFile); errors.Is(err, os.ErrNotExist) {
		// The app store mounts an empty folder by default: no file means no push yet.
		log.Warn("push notifications off: APNs config file not found", "path", cfg.APNSConfigFile)
		return push.NewNotifier(st, nil, log), nil
	}
	apnsConfig, err := push.LoadConfig(cfg.APNSConfigFile)
	if err != nil {
		return nil, err
	}
	client, err := push.NewClient(apnsConfig)
	if err != nil {
		return nil, err
	}
	return push.NewNotifier(st, client, log), nil
}
