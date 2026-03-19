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

func TestCheckNested(t *testing.T) {
	m := NewManager("/tmp/fusebox", "user@host")

	// Stub List to return fake existing syncs
	tests := []struct {
		name     string
		existing []SyncSession
		newPath  string
		wantErr  bool
		errMsg   string
	}{
		{"no overlap", []SyncSession{{Local: "/Users/you/projects"}}, "/Users/you/work", false, ""},
		{"exact duplicate", []SyncSession{{Local: "/Users/you/projects"}}, "/Users/you/projects", true, "already syncing"},
		{"child of existing", []SyncSession{{Local: "/Users/you/work"}}, "/Users/you/work/app", true, "inside already-synced"},
		{"parent of existing", []SyncSession{{Local: "/Users/you/work/app"}}, "/Users/you/work", true, "contains already-synced"},
		{"no existing syncs", nil, "/Users/you/anything", false, ""},
		{"similar prefix not nested", []SyncSession{{Local: "/Users/you/work"}}, "/Users/you/work-backup", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Temporarily override checkNested to use our fake sessions
			err := checkNestedAgainst(tt.existing, tt.newPath)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
	_ = m
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
