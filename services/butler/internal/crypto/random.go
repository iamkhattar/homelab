package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// GeneratePassword returns a cryptographically random base64url-encoded string
// of the specified byte length (the encoded string will be longer).
func GeneratePassword(byteLength int) (string, error) {
	if byteLength <= 0 {
		return "", fmt.Errorf("byte length must be positive, got %d", byteLength)
	}

	buf := make([]byte, byteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(buf), nil
}
