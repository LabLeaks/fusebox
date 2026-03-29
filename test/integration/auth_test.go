package integration

import (
	"io"
	"strings"
	"testing"

	"github.com/lableaks/fusebox/internal/rpc"
)

func TestExecWrongSecret(t *testing.T) {
	cfg := loadFixtureConfig(t)
	addr, _, cleanup := startDaemon(t, cfg)
	defer cleanup()

	client, err := rpc.Dial(rpc.ClientConfig{
		Address: addr,
		Secret:  "wrong-secret",
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	rec := &recorder{}
	err = client.ExecStream("build", nil, io.Discard, rec)
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
	if !strings.Contains(err.Error(), "AUTH_ERROR") {
		t.Errorf("error = %q, want to contain 'AUTH_ERROR'", err.Error())
	}
}

func TestActionsWrongSecret(t *testing.T) {
	cfg := loadFixtureConfig(t)
	addr, _, cleanup := startDaemon(t, cfg)
	defer cleanup()

	client, err := rpc.Dial(rpc.ClientConfig{
		Address: addr,
		Secret:  "wrong-secret",
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	_, err = client.RequestActions()
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
	if !strings.Contains(err.Error(), "AUTH_ERROR") {
		t.Errorf("error = %q, want to contain 'AUTH_ERROR'", err.Error())
	}
}
