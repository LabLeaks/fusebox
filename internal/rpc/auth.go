package rpc

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// GenerateSecret returns a cryptographically random 32-byte hex-encoded string (64 chars).
func GenerateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ValidateSecret performs a constant-time comparison of two secret strings.
func ValidateSecret(got, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}
