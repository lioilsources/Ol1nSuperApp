package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/lioilsources/ol1n-vault/internal/config"
	vdb "github.com/lioilsources/ol1n-vault/internal/db"
	"github.com/lioilsources/ol1n-vault/internal/handler"
	"github.com/lioilsources/ol1n-vault/internal/jobs"
	"github.com/lioilsources/ol1n-vault/internal/nntp"
	"github.com/lioilsources/ol1n-vault/internal/payment"
	"github.com/lioilsources/ol1n-vault/internal/sse"
	"github.com/lioilsources/ol1n-vault/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	for _, dir := range []string{cfg.Vault.NZBDir, cfg.Vault.FilesDir, cfg.Vault.TmpDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			slog.Error("mkdir", "dir", dir, "err", err)
			os.Exit(1)
		}
	}

	db, err := vdb.Open(cfg.Vault.DBPath)
	if err != nil {
		slog.Error("db open", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	queries := vdb.New(db)

	nntpClient := nntp.NewClient(nntp.Config{
		Host:        cfg.NNTP.Host,
		Port:        cfg.NNTP.Port,
		TLS:         cfg.NNTP.TLS,
		User:        cfg.NNTP.User,
		Pass:        cfg.NNTP.Pass,
		Connections: cfg.NNTP.Connections,
		Newsgroup:   cfg.NNTP.Newsgroup,
	})
	defer nntpClient.Close()

	ipnURL := cfg.Vault.PublicBaseURL + "/upload/payment-confirm"
	payClient := payment.NewClient(
		cfg.NOWPayments.APIKey,
		cfg.NOWPayments.APIBase,
		cfg.NOWPayments.SuccessURL,
		cfg.NOWPayments.CancelURL,
		ipnURL,
		cfg.NOWPayments.PriceUSD,
	)

	broker := sse.NewBroker()
	worker := jobs.NewWorker(queries, broker, cfg.NNTP.Connections/2+1)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cleaner := &storage.Cleaner{
		Queries:  queries,
		FilesDir: cfg.Vault.FilesDir,
		Interval: time.Hour,
	}
	cleaner.Start(ctx)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(10 * time.Minute))

	r.Get("/health", handler.Health)

	// API key gate — skip the public webhook and file-serving endpoints.
	gate := handler.SkipPrefixes(handler.APIKey(cfg.Vault.APIKey),
		"/upload/payment-confirm", "/files/", "/health")

	r.Group(func(pr chi.Router) {
		pr.Use(gate)

		uh := handler.NewUploadHandler(handler.UploadDeps{
			Queries:   queries,
			Broker:    broker,
			Worker:    worker,
			Payments:  payClient,
			IPNSecret: cfg.NOWPayments.IPNSecret,
			NNTP:      nntpClient,
			TmpDir:    cfg.Vault.TmpDir,
			NZBDir:    cfg.Vault.NZBDir,
			Poster:    cfg.NNTP.User,
		})
		uh.Mount(pr)

		dh := handler.NewDownloadHandler(handler.DownloadDeps{
			Queries:    queries,
			Broker:     broker,
			Worker:     worker,
			NNTP:       nntpClient,
			NZBDir:     cfg.Vault.NZBDir,
			FilesDir:   cfg.Vault.FilesDir,
			PublicBase: cfg.Vault.PublicBaseURL,
			TokenTTL:   cfg.Vault.FilesTTL,
		})
		dh.Mount(pr)
	})

	addr := fmt.Sprintf(":%d", cfg.Vault.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 30 * time.Second,
	}

	go func() {
		slog.Info("ol1n-vault listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http serve", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	worker.Wait()
}
