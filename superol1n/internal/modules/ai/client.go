package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type NIMClient struct {
	baseURL    string
	apiKey     string
	cfClientID string
	cfSecret   string
	httpClient *http.Client
}

func NewNIMClient(baseURL, apiKey, cfClientID, cfSecret string) *NIMClient {
	return &NIMClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		cfClientID: cfClientID,
		cfSecret:   cfSecret,
		httpClient: &http.Client{},
	}
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (c *NIMClient) do(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if c.cfClientID != "" {
		req.Header.Set("CF-Access-Client-Id", c.cfClientID)
		req.Header.Set("CF-Access-Client-Secret", c.cfSecret)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpClient.Do(req)
}

func (c *NIMClient) ChatStream(req ChatRequest) (io.ReadCloser, error) {
	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("nim: marshal: %w", err)
	}

	resp, err := c.do(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("nim: post: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("nim: status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// Models returns available models normalized to Ollama-compatible format
// so the Flutter client does not need changes.
func (c *NIMClient) Models() (json.RawMessage, error) {
	resp, err := c.do(http.MethodGet, "/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("nim: models: %w", err)
	}
	defer resp.Body.Close()

	var nimResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&nimResp); err != nil {
		return nil, fmt.Errorf("nim: models decode: %w", err)
	}

	type ollamaModel struct {
		Name string `json:"name"`
	}
	type ollamaResp struct {
		Models []ollamaModel `json:"models"`
	}
	out := ollamaResp{}
	for _, m := range nimResp.Data {
		out.Models = append(out.Models, ollamaModel{Name: m.ID})
	}
	return json.Marshal(out)
}

func (c *NIMClient) IsHealthy() bool {
	resp, err := c.do(http.MethodGet, "/v1/models", nil)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
