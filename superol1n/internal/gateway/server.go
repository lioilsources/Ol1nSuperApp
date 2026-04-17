package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ol1ne/superol1n/internal/config"
	"github.com/ol1ne/superol1n/internal/gateway/middleware"
	"github.com/ol1ne/superol1n/internal/module"
)

func NewServer(cfg *config.Config, modules []module.Module) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		status := map[string]string{}
		for _, m := range modules {
			if m.IsHealthy() {
				status[m.Name()] = "ok"
			} else {
				status[m.Name()] = "down"
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	registerModules(mux, modules)

	handler := middleware.Logger(
		middleware.CORS(
			middleware.Auth(cfg.Cloudflare.AccessTeamDomain, cfg.Server.LANOnlyKey)(mux),
		),
	)

	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: handler,
	}
}
