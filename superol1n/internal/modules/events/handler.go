package events

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ol1ne/superol1n/internal/module"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Module struct {
	hub *Hub
}

var _ module.Module = (*Module)(nil)

func New() *Module {
	return &Module{hub: NewHub()}
}

func (m *Module) Prefix() string   { return "/ws" }
func (m *Module) Name() string     { return "events" }
func (m *Module) IsHealthy() bool  { return true }

func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/events", m.wsHandler)
	mux.HandleFunc("POST /webhooks/sonarr", m.webhookHandler("sonarr"))
	mux.HandleFunc("POST /webhooks/radarr", m.webhookHandler("radarr"))
	return mux
}

func (m *Module) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade", "err", err)
		return
	}
	defer conn.Close()

	ch := m.hub.Subscribe()
	defer m.hub.Unsubscribe(ch)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-done:
			return
		case <-ping.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case e, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(e)
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	}
}

func (m *Module) webhookHandler(source string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		eventType := r.Header.Get("X-Sonarr-Event")
		if eventType == "" {
			eventType = r.Header.Get("X-Radarr-Event")
		}
		if eventType == "" {
			eventType = "unknown"
		}

		m.hub.Broadcast(Event{
			Type:    source + "." + eventType,
			Payload: json.RawMessage(body),
			Time:    time.Now(),
		})
		w.WriteHeader(http.StatusNoContent)
	}
}
