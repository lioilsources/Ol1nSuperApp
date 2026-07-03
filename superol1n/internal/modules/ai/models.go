package ai

import (
	"database/sql"
	"net/http"

	"github.com/ol1ne/superol1n/internal/module"
)

type Module struct {
	handler *Handler
	client  *NIMClient
}

var _ module.Module = (*Module)(nil)

func New(baseURL, apiKey, cfClientID, cfSecret string, db *sql.DB) *Module {
	client := NewNIMClient(baseURL, apiKey, cfClientID, cfSecret)
	return &Module{
		client:  client,
		handler: NewHandler(client, db),
	}
}

func (m *Module) Prefix() string { return "/api/ai" }
func (m *Module) Name() string   { return "ai" }
func (m *Module) IsHealthy() bool { return m.client.IsHealthy() }

func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat", m.handler.Chat)
	mux.HandleFunc("GET /models", m.handler.Models)
	mux.HandleFunc("GET /conversations", m.handler.Conversations)
	mux.HandleFunc("POST /conversations/new", m.handler.NewConversation)
	return mux
}
