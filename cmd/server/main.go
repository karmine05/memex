package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"memex/internal/api"
	"memex/internal/auth"
	"memex/internal/config"
	"memex/internal/feed"
	"memex/internal/search"
	"memex/internal/store"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "config file (yaml or json)")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := run(configPath); err != nil {
		slog.Error("shutdown", "err", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if cfg.Mode == "public" && cfg.Registration == "open" {
		slog.Warn("open registration is enabled on a public memex")
	}
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return err
	}
	adminKey, err := loadAdminKey(cfg.DataDir)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	st, err := store.Open(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return err
	}
	emb := newEmbedder(cfg)
	hub := feed.New()
	go hub.Serve(ctx, cfg.DB)
	if cfg.Embed.Provider != "none" {
		go runEmbeddings(ctx, st, emb)
	}
	go maintain(ctx, st)
	srv := &api.Server{
		Cfg: cfg, Store: st, Hub: hub, Embed: emb,
		AdminHash: auth.Hash(adminKey), Limit: api.NewLimiter(),
		AuditPath: cfg.DataDir + "/audit.log",
		WebsiteHandler: api.WebsiteHandler(),
	}
	errCh := make(chan error, 2)
	go func() { errCh <- listen(ctx, cfg.Listen, srv.Handler("agent")) }()
	go func() { errCh <- listen(ctx, cfg.AdminListen, srv.Handler("admin")) }()
	slog.Info("memex listening", "agent", cfg.Listen, "admin", cfg.AdminListen, "mode", cfg.Mode)
	var first error
	for i := 0; i < 2; i++ {
		err := <-errCh
		if i == 0 {
			stop()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) && first == nil {
			first = err
		}
	}
	return first
}

func listen(ctx context.Context, addr string, h http.Handler) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
		shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
		return nil
	case err := <-errCh:
		return err
	}
}

func newEmbedder(cfg config.Config) search.Embedder {
	switch cfg.Embed.Provider {
	case "none":
		return search.None{}
	case "openai":
		return &search.OpenAI{URL: cfg.Embed.URL, Model: cfg.Embed.Model, APIKey: cfg.Embed.APIKey, Dim: cfg.Embed.Dim}
	default:
		return &search.Ollama{URL: cfg.Embed.URL, Model: cfg.Embed.Model, Dim: cfg.Embed.Dim}
	}
}
