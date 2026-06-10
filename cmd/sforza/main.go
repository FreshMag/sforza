// Command sforza runs the SFBAC authorization service.
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

	"github.com/francesco/sforza/internal/api"
	"github.com/francesco/sforza/internal/auth"
	"github.com/francesco/sforza/internal/config"
	"github.com/francesco/sforza/internal/service"
	"github.com/francesco/sforza/internal/store"
)

func main() {
	configPath := flag.String("config", defaultConfigPath(), "path to the configuration file")
	flag.Parse()

	if err := run(*configPath); err != nil {
		slog.Error("sforza terminated", "error", err)
		os.Exit(1)
	}
}

func defaultConfigPath() string {
	if p := os.Getenv("SFORZA_CONFIG"); p != "" {
		return p
	}
	return "sforza.yaml"
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	stores, err := store.Open(cfg.Storage)
	if err != nil {
		return err
	}

	// Bootstrap: meta model + administrator, then microservice YAML files.
	if err := service.BootstrapMeta(stores, cfg.Bootstrap.AdminSub); err != nil {
		return err
	}
	files, err := service.LoadBootstrapFiles(cfg.Bootstrap.Files)
	if err != nil {
		return err
	}
	if err := service.Sync(stores, files); err != nil {
		return err
	}
	slog.Info("bootstrap complete", "files", len(files), "tenants", stores.TenantIDs())

	var authn auth.Authenticator
	if cfg.Auth.IsEnabled() {
		oidcAuthn, err := auth.NewOIDC(context.Background(), cfg.Auth.Issuer, cfg.Auth.Audience)
		if err != nil {
			return err
		}
		authn = oidcAuthn
		slog.Info("authentication enabled", "issuer", cfg.Auth.Issuer)
	} else {
		authn = auth.Static{DefaultSub: cfg.Auth.DefaultSub}
		slog.Warn("authentication DISABLED — development mode", "default-sub", cfg.Auth.DefaultSub)
	}

	srv := &http.Server{
		Addr:              cfg.Server.Address,
		Handler:           api.New(cfg, stores, authn).Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "address", cfg.Server.Address)
		errCh <- srv.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case sig := <-stop:
		slog.Info("shutting down", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	return nil
}
