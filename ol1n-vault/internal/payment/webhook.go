package payment

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

const SignatureHeader = "x-nowpayments-sig"

var (
	ErrMissingSignature = errors.New("payment: missing signature header")
	ErrInvalidSignature = errors.New("payment: invalid signature")
)

// IPNPayload is the subset of NOWPayments IPN fields we care about.
// The full payload is also kept as a map in order to re-serialize with
// canonical key ordering for HMAC verification.
type IPNPayload struct {
	PaymentID     string  `json:"payment_id"`
	PaymentStatus string  `json:"payment_status"`
	OrderID       string  `json:"order_id"`
	PriceAmount   float64 `json:"price_amount"`
	PriceCurrency string  `json:"price_currency"`
	PayAmount     float64 `json:"pay_amount"`
	PayCurrency   string  `json:"pay_currency"`
}

// Verify verifies the HMAC-SHA512 signature of a NOWPayments IPN webhook body.
//
// NOWPayments computes the signature over a canonical JSON representation where
// all keys at every nesting level are sorted alphabetically. We therefore must:
//  1. read the raw request body (don't rely on json.Decoder which may buffer),
//  2. unmarshal into a generic map,
//  3. re-marshal with sorted keys,
//  4. compute HMAC-SHA512 with the IPN secret,
//  5. hex-encode and constant-time compare with the header.
func Verify(secret, signatureHex string, rawBody []byte) error {
	if signatureHex == "" {
		return ErrMissingSignature
	}

	var generic any
	if err := json.Unmarshal(rawBody, &generic); err != nil {
		return fmt.Errorf("payment: decode body: %w", err)
	}

	var buf strings.Builder
	if err := writeSorted(&buf, generic); err != nil {
		return fmt.Errorf("payment: canonicalize: %w", err)
	}

	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(buf.String()))
	expected := hex.EncodeToString(mac.Sum(nil))

	got, err := hex.DecodeString(signatureHex)
	if err != nil {
		return fmt.Errorf("payment: decode signature: %w", err)
	}
	want, _ := hex.DecodeString(expected)
	if !hmac.Equal(got, want) {
		return ErrInvalidSignature
	}
	return nil
}

// ParsePayload decodes the known fields of an IPN payload.
func ParsePayload(r io.Reader) (*IPNPayload, []byte, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	var p IPNPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, raw, err
	}
	return &p, raw, nil
}

// writeSorted emits v as JSON with all object keys sorted alphabetically.
// Matches the canonical form NOWPayments uses to compute the IPN HMAC.
func writeSorted(w *strings.Builder, v any) error {
	switch t := v.(type) {
	case nil:
		w.WriteString("null")
	case bool:
		if t {
			w.WriteString("true")
		} else {
			w.WriteString("false")
		}
	case float64:
		// NOWPayments payloads may include integers and decimals; json.Unmarshal
		// gives us float64 for both. Emit in the shortest round-trip form.
		w.WriteString(strconv.FormatFloat(t, 'f', -1, 64))
	case string:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		w.Write(b)
	case []any:
		w.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				w.WriteByte(',')
			}
			if err := writeSorted(w, item); err != nil {
				return err
			}
		}
		w.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		w.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				w.WriteByte(',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return err
			}
			w.Write(kb)
			w.WriteByte(':')
			if err := writeSorted(w, t[k]); err != nil {
				return err
			}
		}
		w.WriteByte('}')
	default:
		return fmt.Errorf("unsupported json value type %T", v)
	}
	return nil
}

// IsTerminalConfirmed reports whether a payment_status indicates that funds
// are secured and the upload should be released.
func IsTerminalConfirmed(status string) bool {
	switch status {
	case "confirmed", "finished", "sending":
		return true
	}
	return false
}
