package gateway

import (
	"net/http"

	"github.com/ol1ne/superol1n/internal/module"
)

func registerModules(mux *http.ServeMux, modules []module.Module) {
	for _, m := range modules {
		prefix := m.Prefix()
		handler := m.Handler()
		mux.Handle(prefix+"/", http.StripPrefix(prefix, handler))
	}
}
