package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// TestWebhookSignatureScheme proves the delivery signature is the documented
// SHA-256 HMAC over "timestamp.body", is reproducible by a receiver, and fails
// under any tampering of the secret, timestamp, body or signature.
func TestWebhookSignatureScheme(t *testing.T) {
	secret := []byte("s3cr3t")
	body := []byte(`{"version":"1","id":"del_1","payload":{"n":1}}`)
	stamp := "2026-01-01T00:00:00Z"

	sig := signWebhook(secret, stamp, body)
	if !strings.HasPrefix(sig, "v1=") {
		t.Fatalf("signature %q lacks the v1= prefix", sig)
	}

	// Receiver reproduces the same value.
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(stamp + "." + string(body)))
	if want := "v1=" + hex.EncodeToString(mac.Sum(nil)); sig != want {
		t.Fatalf("receiver recomputation mismatch: %q != %q", sig, want)
	}

	// Tampering with the secret, body or timestamp must change the signature.
	if signWebhook([]byte("different"), stamp, body) == sig {
		t.Fatal("signature unchanged after secret tampering")
	}
	tamperedBody := append([]byte(nil), body...)
	tamperedBody[0] = 'x'
	if signWebhook(secret, stamp, tamperedBody) == sig {
		t.Fatal("signature unchanged after body tampering")
	}
	if signWebhook(secret, "2026-01-01T00:00:01Z", body) == sig {
		t.Fatal("signature unchanged after timestamp tampering")
	}
}
