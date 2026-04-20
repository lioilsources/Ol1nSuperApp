package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	vdb "github.com/lioilsources/ol1n-vault/internal/db"
	"github.com/lioilsources/ol1n-vault/internal/jobs"
	"github.com/lioilsources/ol1n-vault/internal/nntp"
	"github.com/lioilsources/ol1n-vault/internal/payment"
	"github.com/lioilsources/ol1n-vault/internal/sse"
)

const maxUploadSize = 4 << 30 // 4 GiB

type UploadDeps struct {
	Queries   *vdb.Queries
	Broker    *sse.Broker
	Worker    *jobs.Worker
	Payments  *payment.Client
	IPNSecret string
	NNTP      *nntp.Client
	TmpDir    string
	NZBDir    string
	Poster    string
}

type UploadHandler struct {
	deps UploadDeps
}

func NewUploadHandler(deps UploadDeps) *UploadHandler {
	return &UploadHandler{deps: deps}
}

func (h *UploadHandler) Mount(r chi.Router) {
	r.Post("/upload/init", h.initUpload)
	r.Post("/upload/payment-confirm", h.paymentConfirm)
	r.Get("/upload/status/{job_id}", h.status)
}

type initResponse struct {
	JobID      string `json:"job_id"`
	PaymentID  string `json:"payment_id"`
	PaymentURL string `json:"payment_url"`
}

func (h *UploadHandler) initUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "bad multipart", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if header.Size > maxUploadSize {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}

	if err := os.MkdirAll(h.deps.TmpDir, 0o755); err != nil {
		http.Error(w, "tmp dir", http.StatusInternalServerError)
		return
	}
	jobID := uuid.New().String()
	tmpPath := filepath.Join(h.deps.TmpDir, jobID)

	out, err := os.Create(tmpPath)
	if err != nil {
		http.Error(w, "tmp create", http.StatusInternalServerError)
		return
	}
	size, err := io.Copy(out, io.LimitReader(file, maxUploadSize+1))
	out.Close()
	if err != nil || size > maxUploadSize {
		os.Remove(tmpPath)
		http.Error(w, "write failed", http.StatusInternalServerError)
		return
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = sniffContentType(tmpPath)
	}

	filename := sanitizeFilename(header.Filename)

	invoice, err := h.deps.Payments.CreateInvoice(r.Context(), jobID,
		fmt.Sprintf("Ol1n Vault upload: %s", filename))
	if err != nil {
		slog.Error("create invoice", "err", err)
		os.Remove(tmpPath)
		http.Error(w, "payment init failed", http.StatusBadGateway)
		return
	}

	// Use jobID as the placeholder artifact ID; it will be replaced with the
	// content hash of the final NZB once the upload completes.
	if err := h.deps.Queries.InsertArtifactPending(r.Context(),
		jobID, filename, contentType, size, invoice.ID); err != nil {
		slog.Error("insert artifact", "err", err)
		os.Remove(tmpPath)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if err := h.deps.Queries.InsertJob(r.Context(), jobID, jobID, vdb.JobTypeUpload, tmpPath); err != nil {
		slog.Error("insert job", "err", err)
		os.Remove(tmpPath)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, initResponse{
		JobID:      jobID,
		PaymentID:  invoice.ID,
		PaymentURL: invoice.InvoiceURL,
	})
}

func (h *UploadHandler) paymentConfirm(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	sig := r.Header.Get(payment.SignatureHeader)
	if err := payment.Verify(h.deps.IPNSecret, sig, raw); err != nil {
		slog.Warn("webhook verify failed", "err", err)
		http.Error(w, "bad signature", http.StatusUnauthorized)
		return
	}

	var p payment.IPNPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	slog.Info("nowpayments ipn", "payment_id", p.PaymentID, "status", p.PaymentStatus, "order_id", p.OrderID)

	if !payment.IsTerminalConfirmed(p.PaymentStatus) {
		// Not yet confirmed — record status and return 200 so NOWPayments stops retrying.
		_ = h.deps.Queries.UpdateArtifactPaymentStatus(r.Context(), p.PaymentID, p.PaymentStatus)
		w.WriteHeader(http.StatusOK)
		return
	}

	advanced, err := h.deps.Queries.MarkPaymentConfirmed(r.Context(), p.PaymentID, p.PaymentStatus)
	if err != nil {
		slog.Error("mark confirmed", "err", err)
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if !advanced {
		// Either already confirmed (duplicate webhook) or artifact missing — safe no-op.
		w.WriteHeader(http.StatusOK)
		return
	}

	// Use a detached context so the job survives this HTTP request.
	go func(paymentID string) {
		if err := jobs.StartFromPaymentConfirm(context.Background(),
			h.deps.Queries, h.deps.Worker, h.deps.NNTP, paymentID, h.deps.NZBDir, h.deps.Poster); err != nil {
			slog.Error("start upload job", "err", err)
		}
	}(p.PaymentID)

	w.WriteHeader(http.StatusOK)
}

func (h *UploadHandler) status(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "job_id")
	streamJobStatus(w, r, h.deps.Queries, h.deps.Broker, jobID)
}
