package tmhi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// Base64urlEscape converts base64 to URL-safe encoding.
func Base64urlEscape(b64 string) string {
	r := strings.NewReplacer("+", "-", "/", "_", "=", ".")

	return r.Replace(b64)
}

// Sha256Hash computes SHA256 hash of val1:val2 and returns base64 encoding.
func Sha256Hash(val1, val2 string) string {
	h := sha256.New()
	h.Write(fmt.Appendf(nil, "%s:%s", val1, val2))

	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// Sha256Url computes SHA256 hash and returns URL-safe base64 encoding.
func Sha256Url(val1, val2 string) string {
	return Base64urlEscape(Sha256Hash(val1, val2))
}

// Random16bytes generates 16 random bytes encoded as URL-safe base64.
func Random16bytes() string {
	const length = 16

	bytes := make([]byte, length)

	_, err := rand.Read(bytes)
	if err != nil {
		return ""
	}

	return Base64urlEscape(base64.StdEncoding.EncodeToString(bytes))
}
