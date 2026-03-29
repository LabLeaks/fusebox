package rpc

import (
	"testing"
)

func TestGenerateSecretLength(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}

	// 32 bytes hex-encoded = 64 characters
	if len(secret) != 64 {
		t.Errorf("secret length = %d, want 64", len(secret))
	}
}

func TestGenerateSecretHex(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}

	for _, c := range secret {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("non-hex character %q in secret", c)
		}
	}
}

func TestGenerateSecretUniqueness(t *testing.T) {
	secrets := make(map[string]bool)
	for i := 0; i < 100; i++ {
		s, err := GenerateSecret()
		if err != nil {
			t.Fatalf("GenerateSecret %d: %v", i, err)
		}
		if secrets[s] {
			t.Fatalf("duplicate secret on iteration %d", i)
		}
		secrets[s] = true
	}
}

func TestValidateSecretMatch(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}

	if !ValidateSecret(secret, secret) {
		t.Error("ValidateSecret returned false for matching secrets")
	}
}

func TestValidateSecretMismatch(t *testing.T) {
	if ValidateSecret("aaa", "bbb") {
		t.Error("ValidateSecret returned true for mismatched secrets")
	}
}

func TestValidateSecretEmpty(t *testing.T) {
	if ValidateSecret("", "notempty") {
		t.Error("ValidateSecret returned true for empty got")
	}
	if ValidateSecret("notempty", "") {
		t.Error("ValidateSecret returned true for empty expected")
	}
}

func TestValidateSecretBothEmpty(t *testing.T) {
	// subtle.ConstantTimeCompare([]byte(""), []byte("")) == 1, so two empty
	// strings are considered equal. This is safe in practice because the daemon
	// server always generates a 64-char hex secret via GenerateSecret — empty
	// secrets never appear in production. Documenting this behavior explicitly.
	if !ValidateSecret("", "") {
		t.Error("ValidateSecret returned false for two empty strings")
	}
}
