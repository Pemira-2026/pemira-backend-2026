package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// GenerateReceiptToken membuat token tanda terima digital menggunakan HMAC-SHA256.
// Mengembalikan (ReceiptToken publik untuk mahasiswa, ReceiptHash untuk disimpan di database voters).
func GenerateReceiptToken(voterID string, secretKey string, salt string) (string, string) {
	timestamp := time.Now().UnixNano()
	payload := fmt.Sprintf("%s:%s:%d", voterID, salt, timestamp)

	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))

	// Format token publik yang ramah pengguna: VOTE-RCPT-<16-karakter-signature>-<8-karakter-timestamp>
	shortSig := strings.ToUpper(signature[:16])
	timeHex := strings.ToUpper(fmt.Sprintf("%X", timestamp)[:8])
	receiptToken := fmt.Sprintf("VOTE-RCPT-%s-%s", shortSig, timeHex)

	// Hash 64 karakter lengkap yang disimpan di DB untuk verifikasi audit
	receiptHash := signature

	return receiptToken, receiptHash
}

// HashString menghasilkan SHA256 biasa untuk hashing data umum
func HashString(data string) string {
	h := sha256.New()
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}
