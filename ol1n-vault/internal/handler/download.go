package handler

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	vdb "github.com/lioilsources/ol1n-vault/internal/db"
	"github.com/lioilsources/ol1n-vault/internal/jobs"
	"github.com/lioilsources/ol1n-vault/internal/nntp"
	"github.com/lioilsources/ol1n-vault/internal/sse"
)

type DownloadDeps struct {
	Queries    *vdb.Queries
	Broker     *sse.Broker
	Worker     *jobs.Worker
	NNTP       *nntp.Client
	NZBDir     string
	FilesDir   string
	PublicBase string
	TokenTTL   time.Duration
}

type DownloadHandler struct {
	deps DownloadDeps
}

func NewDownloadHandler(deps DownloadDeps) *DownloadHandler {
	return &DownloadHandler{deps: deps}
}

func (h *DownloadHandler) Mount(r chi.Router) {
	r.Post("/download/{artifact_id}", h.request)
	r.Get("/download/{job_id}/status", h.status)
	r.Get("/files/{token}/{filename}", h.serveFile)
}

func (h *DownloadHandler) request(w http.ResponseWriter, r *http.Request) {
	artifactID := chi.URLParam(r, "artifact_id")
	artifact, err := h.deps.Queries.GetArtifact(r.Context(), artifactID)
	if errors.Is(err, vdb.ErrNotFound) {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if artifact.Status != vdb.StatusReady && artifact.Status != vdb.StatusNotarized {
		http.Error(w, "artifact not ready", http.StatusConflict)
		return
	}
	if !artifact.NZBPath.Valid {
		http.Error(w, "nzb missing", http.StatusInternalServerError)
		return
	}

	jobID := uuid.New().String()
	if err := h.deps.Queries.InsertJob(r.Context(), jobID, artifact.ID, vdb.JobTypeDownload, ""); err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	j := &jobs.DownloadJob{
		JobID:       jobID,
		ArtifactID:  artifact.ID,
		Filename:    artifact.Filename,
		ContentType: artifact.ContentType,
		NZBPath:     filepath.Join(h.deps.NZBDir, artifact.NZBPath.String),
		FilesDir:    h.deps.FilesDir,
		TokenTTL:    h.deps.TokenTTL,
		PublicBase:  h.deps.PublicBase,
		Client:      h.deps.NNTP,
		Worker:      h.deps.Worker,
	}
	h.deps.Worker.Submit(r.Context(), j)

	writeJSON(w, http.StatusOK, map[string]string{"job_id": jobID})
}

func (h *DownloadHandler) status(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "job_id")
	streamJobStatus(w, r, h.deps.Queries, h.deps.Broker, jobID)
}

func (h *DownloadHandler) serveFile(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	filename := chi.URLParam(r, "filename")

	tok, err := h.deps.Queries.GetDownloadToken(r.Context(), token)
	if errors.Is(err, vdb.ErrNotFound) {
		http.Error(w, "token not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if tok.ExpiresAt < time.Now().Unix() {
		http.Error(w, "token expired", http.StatusGone)
		return
	}

	// Prevent path traversal: only allow simple filenames.
	if strings.ContainsAny(filename, "/\\") || filename == "" || filename == "." || filename == ".." {
		http.Error(w, "bad filename", http.StatusBadRequest)
		return
	}

	path := filepath.Join(h.deps.FilesDir, tok.ArtifactID, filename)
	artifact, err := h.deps.Queries.GetArtifact(r.Context(), tok.ArtifactID)
	if err == nil && artifact.ContentType != "" {
		w.Header().Set("Content-Type", artifact.ContentType)
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
	http.ServeFile(w, r, path)
}
