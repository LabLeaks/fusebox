package orchestrator

import (
	"testing"
)

func TestUpConfigDefaults(t *testing.T) {
	cfg := UpConfig{}
	if cfg.Log != nil {
		t.Error("default Log should be nil")
	}
	if cfg.FuseboxBinary != "" {
		t.Error("default FuseboxBinary should be empty")
	}
}

func TestConstants(t *testing.T) {
	if rpcPort != 9600 {
		t.Errorf("rpcPort = %d, want 9600", rpcPort)
	}
	if remoteBinPath != "/usr/local/bin/fusebox" {
		t.Errorf("remoteBinPath = %q, want /usr/local/bin/fusebox", remoteBinPath)
	}
}
