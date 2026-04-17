package main

import (
	"log/slog"
	"os"

	"github.com/ol1ne/superol1n/db"
	"github.com/ol1ne/superol1n/internal/config"
	"github.com/ol1ne/superol1n/internal/gateway"
	"github.com/ol1ne/superol1n/internal/module"
	aimod "github.com/ol1ne/superol1n/internal/modules/ai"
	"github.com/ol1ne/superol1n/internal/modules/events"
	"github.com/ol1ne/superol1n/internal/modules/radarr"
	"github.com/ol1ne/superol1n/internal/modules/sabnzbd"
	"github.com/ol1ne/superol1n/internal/modules/sonarr"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load", "err", err)
		os.Exit(1)
	}

	database, err := db.Open(cfg.DB.Path)
	if err != nil {
		slog.Warn("db open failed, chat history disabled", "err", err)
	}

	modules := []module.Module{
		aimod.New(cfg.Ollama.LANURL, database),
		sonarr.New(cfg.Sonarr.URL, cfg.Sonarr.APIKey),
		radarr.New(cfg.Radarr.URL, cfg.Radarr.APIKey),
		sabnzbd.New(cfg.SABnzbd.URL, cfg.SABnzbd.APIKey),
		events.New(),
	}

	srv := gateway.NewServer(cfg, modules)
	slog.Info("starting superol1n", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
