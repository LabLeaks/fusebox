package sandbox

import (
	"testing"
)

func TestNew(t *testing.T) {
	s := New("/home/user/.fusebox")
	if s.DataDir != "/home/user/.fusebox" {
		t.Errorf("DataDir = %q, want %q", s.DataDir, "/home/user/.fusebox")
	}
}

func TestTmuxSocket(t *testing.T) {
	s := New("/home/user/.fusebox")
	want := "/home/user/.fusebox/tmux.sock"
	if got := s.TmuxSocket(); got != want {
		t.Errorf("TmuxSocket = %q, want %q", got, want)
	}
}

func TestRootfsDir(t *testing.T) {
	s := New("/home/user/.fusebox")
	want := "/home/user/.fusebox/rootfs"
	if got := s.RootfsDir(); got != want {
		t.Errorf("RootfsDir = %q, want %q", got, want)
	}
}

func TestMergedDir(t *testing.T) {
	s := New("/home/user/.fusebox")
	want := "/home/user/.fusebox/overlay/merged"
	if got := s.MergedDir(); got != want {
		t.Errorf("MergedDir = %q, want %q", got, want)
	}
}
