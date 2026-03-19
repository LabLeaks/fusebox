package embed

import (
	"strings"
	"testing"
)

func TestServerBinary_DevBuildArm64(t *testing.T) {
	_, err := ServerBinary("arm64")
	if err == nil {
		t.Fatal("expected error for arm64 on dev build")
	}
	if !strings.Contains(err.Error(), "dev build") {
		t.Errorf("expected dev build message, got: %v", err)
	}
}

func TestServerBinary_DevBuildAmd64(t *testing.T) {
	_, err := ServerBinary("amd64")
	if err == nil {
		t.Fatal("expected error for amd64 on dev build")
	}
	if !strings.Contains(err.Error(), "dev build") {
		t.Errorf("expected dev build message, got: %v", err)
	}
}

func TestServerBinary_Unsupported(t *testing.T) {
	_, err := ServerBinary("mips")
	if err == nil {
		t.Fatal("expected error for unsupported arch")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected unsupported message, got: %v", err)
	}
}
