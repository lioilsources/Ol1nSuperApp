package sabnzbd

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/ol1ne/superol1n/internal/module"
)

type Module struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

var _ module.Module = (*Module)(nil)

func New(baseURL, apiKey string) *Module {
	return &Module{baseURL: baseURL, apiKey: apiKey, client: &http.Client{}}
}

func (m *Module) Prefix() string  { return "/api/sabnzbd" }
func (m *Module) Name() string    { return "sabnzbd" }
func (m *Module) IsHealthy() bool {
	resp, err := m.client.Get(m.apiURL("mode=version"))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (m *Module) apiURL(params string) string {
	return fmt.Sprintf("%s/api?output=json&apikey=%s&%s", m.baseURL, m.apiKey, params)
}

func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /queue", func(w http.ResponseWriter, r *http.Request) {
		m.proxy(w, m.apiURL("mode=queue"))
	})
	mux.HandleFunc("GET /history", func(w http.ResponseWriter, r *http.Request) {
		m.proxy(w, m.apiURL("mode=history"))
	})
	mux.HandleFunc("POST /pause", func(w http.ResponseWriter, r *http.Request) {
		m.proxy(w, m.apiURL("mode=pause"))
	})
	mux.HandleFunc("POST /resume", func(w http.ResponseWriter, r *http.Request) {
		m.proxy(w, m.apiURL("mode=resume"))
	})
	mux.HandleFunc("DELETE /queue/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		m.proxy(w, m.apiURL(fmt.Sprintf("mode=queue&name=delete&value=%s", url.QueryEscape(id))))
	})
	return mux
}

func (m *Module) proxy(w http.ResponseWriter, upstreamURL string) {
	resp, err := m.client.Get(upstreamURL)
	if err != nil {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
