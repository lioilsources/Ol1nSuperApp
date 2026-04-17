package radarr

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/ol1ne/superol1n/internal/module"
)

type Module struct {
	upstream *url.URL
	apiKey   string
}

var _ module.Module = (*Module)(nil)

func New(upstreamURL, apiKey string) *Module {
	u, _ := url.Parse(upstreamURL)
	return &Module{upstream: u, apiKey: apiKey}
}

func (m *Module) Prefix() string  { return "/api/radarr" }
func (m *Module) Name() string    { return "radarr" }
func (m *Module) IsHealthy() bool {
	resp, err := http.Get(m.upstream.String() + "/api/v3/system/status?apikey=" + m.apiKey)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (m *Module) Handler() http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(m.upstream)
	original := proxy.Director
	proxy.Director = func(r *http.Request) {
		original(r)
		r.URL.Path = "/api/v3" + r.URL.Path
		r.URL.Host = m.upstream.Host
		r.URL.Scheme = m.upstream.Scheme
		r.Host = m.upstream.Host
		r.Header.Set("X-Api-Key", m.apiKey)
	}
	return proxy
}
