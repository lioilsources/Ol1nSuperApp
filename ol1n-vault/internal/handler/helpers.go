package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	vdb "github.com/lioilsources/ol1n-vault/internal/db"
	"github.com/lioilsources/ol1n-vault/internal/jobs"
	"github.com/lioilsources/ol1n-vault/internal/sse"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// sanitizeFilename strips any directory components and hostile characters so
// the name is safe to embed in subjects, filesystem paths, and URLs.
func sanitizeFilename(s string) string {
	s = filepath.Base(s)
	// Replace characters that would break yEnc =ybegin line or NZB parsing.
	replacer := strings.NewReplacer(
		"\n", "_", "\r", "_", "\t", "_",
		"\x00", "_", "/", "_", "\\", "_",
	)
	s = replacer.Replace(s)
	s = strings.TrimSpace(s)
	if s == "" || s == "." || s == ".." {
		return "upload.bin"
	}
	return s
}

// sniffContentType reads the first 512 bytes to guess a MIME type.
func sniffContentType(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	var buf [512]byte
	n, _ := f.Read(buf[:])
	return http.DetectContentType(buf[:n])
}

// streamJobStatus emits SSE events for a job, starting with its current DB
// state and then forwarding broker updates until the client disconnects or
// the job reaches a terminal state.
func streamJobStatus(w http.ResponseWriter, r *http.Request, q *vdb.Queries, broker *sse.Broker, jobID string) {
	f, ok := sseHeaders(w)
	if !ok {
		return
	}

	ch, cancel := broker.Subscribe(jobID)
	defer cancel()

	// Emit current state so clients reconnecting mid-job get the latest picture.
	if job, err := q.GetJob(r.Context(), jobID); err == nil {
		ev := jobs.Event{Status: job.Status, Progress: job.Progress}
		if job.ResultURL.Valid {
			ev.DownloadURL = job.ResultURL.String
		}
		if job.ErrorMsg.Valid {
			ev.Error = job.ErrorMsg.String
		}
		_ = writeSSEJSON(w, f, ev)
		if isTerminal(job.Status) {
			return
		}
	} else if errors.Is(err, vdb.ErrNotFound) {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSE(w, f, data); err != nil {
				return
			}
			// Peek at status field to terminate on terminal states.
			var peek struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(data, &peek); err == nil && isTerminal(peek.Status) {
				return
			}
		}
	}
}

func isTerminal(status string) bool {
	return status == vdb.JobDone || status == vdb.JobFailed
}
