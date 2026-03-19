package main

import (
	"os"
	"testing"
)

func TestIsLocalHost_MatchesHostname(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		t.Skip("cannot get hostname")
	}
	if !isLocalHost(hostname) {
		t.Errorf("expected isLocalHost(%q) to be true", hostname)
	}
}

func TestIsLocalHost_NoMatchDifferentHost(t *testing.T) {
	if isLocalHost("definitely-not-this-machine-xyz") {
		t.Error("expected isLocalHost to be false for non-matching host")
	}
}

func TestIsLocalHost_EmptyString(t *testing.T) {
	if isLocalHost("") {
		t.Error("expected isLocalHost to be false for empty string")
	}
}
