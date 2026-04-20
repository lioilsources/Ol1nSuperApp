package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func sseHeaders(w http.ResponseWriter) (http.Flusher, bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	f.Flush()
	return f, true
}

func writeSSE(w http.ResponseWriter, f http.Flusher, data []byte) error {
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	f.Flush()
	return nil
}

func writeSSEJSON(w http.ResponseWriter, f http.Flusher, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeSSE(w, f, raw)
}
