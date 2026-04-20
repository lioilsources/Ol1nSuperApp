package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	apiKey     string
	apiBase    string
	successURL string
	cancelURL  string
	ipnURL     string
	priceUSD   float64
	http       *http.Client
}

func NewClient(apiKey, apiBase, successURL, cancelURL, ipnURL string, priceUSD float64) *Client {
	return &Client{
		apiKey:     apiKey,
		apiBase:    apiBase,
		successURL: successURL,
		cancelURL:  cancelURL,
		ipnURL:     ipnURL,
		priceUSD:   priceUSD,
		http:       &http.Client{Timeout: 20 * time.Second},
	}
}

type InvoiceRequest struct {
	PriceAmount      float64 `json:"price_amount"`
	PriceCurrency    string  `json:"price_currency"`
	OrderID          string  `json:"order_id"`
	OrderDescription string  `json:"order_description"`
	SuccessURL       string  `json:"success_url"`
	CancelURL        string  `json:"cancel_url"`
	IPNCallbackURL   string  `json:"ipn_callback_url"`
}

type InvoiceResponse struct {
	ID         string `json:"id"`
	InvoiceURL string `json:"invoice_url"`
	Status     string `json:"token_id"`
}

func (c *Client) CreateInvoice(ctx context.Context, jobID, description string) (*InvoiceResponse, error) {
	body, _ := json.Marshal(InvoiceRequest{
		PriceAmount:      c.priceUSD,
		PriceCurrency:    "usd",
		OrderID:          jobID,
		OrderDescription: description,
		SuccessURL:       c.successURL,
		CancelURL:        c.cancelURL,
		IPNCallbackURL:   c.ipnURL,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+"/invoice", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nowpayments invoice: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("nowpayments invoice: status %d: %s", resp.StatusCode, string(raw))
	}

	var out InvoiceResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("nowpayments invoice: decode: %w — body=%s", err, string(raw))
	}
	return &out, nil
}
