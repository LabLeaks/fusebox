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
	if !ValidateSecret("", "") {
		t.Error("ValidateSecret returned false for two empty strings")
	}
}
