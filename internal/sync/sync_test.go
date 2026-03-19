package sync

import (
	"testing"
)

func TestParseMutagenList(t *testing.T) {
	input := `Name: fusebox-projects
Identifier: abc123def456
Alpha:
	URL: /Users/you/projects
	Connected: Yes
Beta:
	URL: user@server:~/.fusebox/sync/projects
	Connected: Yes
Status: Watching for changes
`
	sessions := parseMutagenList(input)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	s := sessions[0]
	if s.Name != "fusebox-projects" {
		t.Errorf("name = %q, want %q", s.Name, "fusebox-projects")
	}
	if s.Local != "/Users/you/projects" {
		t.Errorf("local = %q, want %q", s.Local, "/Users/you/projects")
	}
	if s.Remote != "user@server:~/.fusebox/sync/projects" {
		t.Errorf("remote = %q, want %q", s.Remote, "user@server:~/.fusebox/sync/projects")
	}
	if s.Status != "Watching for changes" {
		t.Errorf("status = %q, want %q", s.Status, "Watching for changes")
	}
}

func TestParseMutagenListMultiple(t *testing.T) {
	input := `Name: fusebox-projects
Identifier: abc123
Alpha:
	URL: /Users/you/projects
Beta:
	URL: user@server:~/.fusebox/sync/projects
Status: Watching for changes

Name: fusebox-work
Identifier: def456
Alpha:
	URL: /Users/you/work
Beta:
	URL: user@server:~/.fusebox/sync/work
Status: Staging files
`
	sessions := parseMutagenList(input)
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].Name != "fusebox-projects" {
		t.Errorf("session[0].name = %q, want %q", sessions[0].Name, "fusebox-projects")
	}
	if sessions[1].Name != "fusebox-work" {
		t.Errorf("session[1].name = %q, want %q", sessions[1].Name, "fusebox-work")
	}
	if sessions[1].Status != "Staging files" {
		t.Errorf("session[1].status = %q, want %q", sessions[1].Status, "Staging files")
	}
}

func TestParseMutagenListEmpty(t *testing.T) {
	sessions := parseMutagenList("")
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestSyncName(t *testing.T) {
	m := NewManager("/tmp/fusebox", "user@host")
	if got := m.syncName("/Users/you/projects"); got != "fusebox-projects" {
		t.Errorf("syncName = %q, want %q", got, "fusebox-projects")
	}
}

func TestRemotePath(t *testing.T) {
	m := NewManager("/tmp/fusebox", "user@host")
	got := m.remotePath("/Users/you/projects")
	want := "user@host:~/.fusebox/sync/projects"
	if got != want {
		t.Errorf("remotePath = %q, want %q", got, want)
	}
}
