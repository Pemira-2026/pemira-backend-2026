package crypto

import (
	"strings"
	"testing"
)

func TestGenerateReceiptTokenContainsOnlyPublicSignature(t *testing.T) {
	token, hash := GenerateReceiptToken("voter-123", "test-secret", "test-salt")

	parts := strings.Split(token, "-")
	if len(parts) != 3 || parts[0] != "VOTE" || parts[1] != "RCPT" {
		t.Fatalf("unexpected receipt token format: %q", token)
	}
	if parts[2] != strings.ToUpper(hash) {
		t.Fatalf("receipt token signature does not match stored hash")
	}
	if strings.Contains(token, "voter-123") {
		t.Fatalf("receipt token must not contain voter identity")
	}
}
