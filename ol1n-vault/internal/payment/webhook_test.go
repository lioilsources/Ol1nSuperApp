package payment

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"testing"
)

func TestVerify_SortedJSONMatchesHMAC(t *testing.T) {
	secret := "test-secret"
	// NOWPayments canonicalizes keys alphabetically at every level.
	// The body we sign is the sorted form — the raw body sent to the
	// endpoint can have any key order; our canonicalizer normalizes it.
	canonical := `{"order_id":"abc","payment_id":"42","payment_status":"finished"}`
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(canonical))
	sig := hex.EncodeToString(mac.Sum(nil))

	raw := []byte(`{"payment_status":"finished","payment_id":"42","order_id":"abc"}`)
	if err := Verify(secret, sig, raw); err != nil {
		t.Fatalf("expected signature to verify, got %v", err)
	}
}

func TestVerify_RejectsTamperedBody(t *testing.T) {
	secret := "test-secret"
	canonical := `{"order_id":"abc","payment_id":"42","payment_status":"finished"}`
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(canonical))
	sig := hex.EncodeToString(mac.Sum(nil))

	raw := []byte(`{"payment_status":"failed","payment_id":"42","order_id":"abc"}`)
	if err := Verify(secret, sig, raw); err == nil {
		t.Fatal("expected tampered body to fail verification")
	}
}

func TestVerify_MissingSignature(t *testing.T) {
	if err := Verify("s", "", []byte(`{}`)); err != ErrMissingSignature {
		t.Fatalf("want ErrMissingSignature, got %v", err)
	}
}

func TestVerify_NestedObjectSorted(t *testing.T) {
	secret := "s"
	canonical := `{"a":{"x":1,"y":2},"z":[{"k":"v"}]}`
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(canonical))
	sig := hex.EncodeToString(mac.Sum(nil))

	raw := []byte(`{"z":[{"k":"v"}],"a":{"y":2,"x":1}}`)
	if err := Verify(secret, sig, raw); err != nil {
		t.Fatalf("nested canonicalization failed: %v", err)
	}
}
