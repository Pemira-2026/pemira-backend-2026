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

	// Token publik hanya memuat signature agar dapat dicocokkan dengan hash
	// receipt yang disimpan, tanpa memuat voter ID, timestamp, atau pilihan.
	receiptToken := fmt.Sprintf("VOTE-RCPT-%s", strings.ToUpper(signature))

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
