package ai

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Handler struct {
	client *OllamaClient
	db     *sql.DB
}

func NewHandler(client *OllamaClient, db *sql.DB) *Handler {
	return &Handler{client: client, db: db}
}

type chatRequest struct {
	Model          string    `json:"model"`
	Messages       []Message `json:"messages"`
	ConversationID string    `json:"conversation_id"`
}

func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Model == "" {
		http.Error(w, "model required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	stream, err := h.client.ChatStream(ChatRequest{
		Model:    req.Model,
		Messages: req.Messages,
	})
	if err != nil {
		slog.Error("ollama chat stream", "err", err)
		fmt.Fprintf(w, "data: {\"error\":%q}\n\n", err.Error())
		flusher.Flush()
		return
	}
	defer stream.Close()

	var fullResponse strings.Builder
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()

		var chunk struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done bool `json:"done"`
		}
		if err := json.Unmarshal([]byte(line), &chunk); err == nil {
			fullResponse.WriteString(chunk.Message.Content)
		}
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	if req.ConversationID != "" && h.db != nil {
		h.saveConversation(req.ConversationID, req.Messages, fullResponse.String())
	}
}

func (h *Handler) saveConversation(convID string, messages []Message, response string) {
	_, err := h.db.Exec(`INSERT OR IGNORE INTO conversations(id, created_at) VALUES(?, ?)`,
		convID, time.Now())
	if err != nil {
		slog.Error("save conversation", "err", err)
		return
	}
	for _, m := range messages {
		_, err = h.db.Exec(`INSERT INTO messages(conversation_id, role, content) VALUES(?, ?, ?)`,
			convID, m.Role, m.Content)
		if err != nil {
			slog.Error("save message", "err", err)
		}
	}
	_, err = h.db.Exec(`INSERT INTO messages(conversation_id, role, content) VALUES(?, ?, ?)`,
		convID, "assistant", response)
	if err != nil {
		slog.Error("save assistant message", "err", err)
	}
}

func (h *Handler) Models(w http.ResponseWriter, r *http.Request) {
	data, err := h.client.Models()
	if err != nil {
		slog.Error("models", "err", err)
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (h *Handler) Conversations(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		http.Error(w, "db not configured", http.StatusServiceUnavailable)
		return
	}
	rows, err := h.db.Query(`SELECT id, created_at FROM conversations ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type conv struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
	}
	var convs []conv
	for rows.Next() {
		var c conv
		rows.Scan(&c.ID, &c.CreatedAt)
		convs = append(convs, c)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(convs)
}

func (h *Handler) NewConversation(w http.ResponseWriter, r *http.Request) {
	id := uuid.New().String()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"conversation_id": id})
}
