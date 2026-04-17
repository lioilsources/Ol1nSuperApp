package module

import "net/http"

type Module interface {
	Prefix() string
	Handler() http.Handler
	Name() string
	IsHealthy() bool
}
