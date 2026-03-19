package tui_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/lableaks/fusebox/internal/config"
	"github.com/lableaks/fusebox/internal/testutil"
	"github.com/lableaks/fusebox/internal/tui"
)

const waitTimeout = 3 * time.Second

// Test constants — no user-specific values.
const (
	testHost       = "test-server"
	testUser       = "testuser"
	testHome       = "/home/testuser"
	testServerPath = testHome + "/bin/fusebox"
)

func testConfig() config.Config {
	return config.Config{
		Server: config.Server{Host: testHost, User: testUser},
		Claude: config.Claude{Flags: "--dangerously-skip-permissions --remote-control"},
		BrowseRoots: []string{"~/work/projects", "~/work/research"},
		ServerPath:  testServerPath,
	}
}

func serverCmd(sub string) string { return testServerPath + " " + sub }

// newTestApp creates a test model wired to a mock SSH backend.
// Auto-registers an empty activity response if not already set.
func newTestApp(t *testing.T, mock *testutil.MockSSH) *teatest.TestModel {
	t.Helper()
	actCmd := serverCmd("activity")
	if !mock.HasResponse(actCmd) {
		mock.OnJSON(actCmd, `{}`)
	}
	teamsCmd := serverCmd("teams")
	if !mock.HasResponse(teamsCmd) {
		mock.OnJSON(teamsCmd, `[]`)
	}
	// loadDirsCmd checks for synced folders first
	if !mock.HasResponse("echo $HOME") {
		mock.On("echo $HOME", []byte(testHome+"\n"), nil)
	}
	syncSubdirs := serverCmd("subdirs " + testHome + "/.fusebox/sync")
	if !mock.HasResponse(syncSubdirs) {
		mock.On(syncSubdirs, nil, fmt.Errorf("no sync dir"))
	}
	cfg := testConfig()
	model := tui.NewWithRunner(cfg, mock)
	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(100, 30))
	t.Cleanup(func() { tm.Quit() })
	return tm
}

// waitFor waits for text to appear in the program output.
func waitFor(t *testing.T, tm *teatest.TestModel, text string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte(text))
	}, teatest.WithDuration(waitTimeout))
}

// waitForAll waits for all texts to appear in a single output snapshot.
func waitForAll(t *testing.T, tm *teatest.TestModel, texts ...string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		for _, text := range texts {
			if !bytes.Contains(bts, []byte(text)) {
				return false
			}
		}
		return true
	}, teatest.WithDuration(waitTimeout))
}

// sendKey sends a special key press to the program.
func sendKey(tm *teatest.TestModel, code rune) {
	tm.Send(tea.KeyPressMsg{Code: code})
}

const sessionsJSON = `[
	{"name":"project-a","dir":"/home/testuser/work/projects/project-a","created":1710000000,"activity":9999999999},
	{"name":"project-b","dir":"/home/testuser/work/research/project-b","created":1709900000,"activity":1709900000}
]`

const dirsJSON = `[
	"/home/testuser/work/research/project-b",
	"/home/testuser/work/research/project-c",
	"/home/testuser/work/projects/project-a",
	"/home/testuser/work/projects/project-d"
]`

// --- Dashboard ---

func TestDashboard_RendersSessions(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	tm := newTestApp(t, mock)

	waitForAll(t, tm,
		"project-a", "project-b",
		"FUSEBOX", "test-server",
		"[n] new", "[q] quit",
	)
}

func TestDashboard_EmptyState(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), `[]`)
	tm := newTestApp(t, mock)

	waitForAll(t, tm, "No active sessions", "[n]")
}

func TestDashboard_SessionCount(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "2 active sessions")
}

func TestDashboard_Quit(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), `[]`)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "No active sessions")
	tm.Type("q")
	tm.WaitFinished(t, teatest.WithFinalTimeout(waitTimeout))
}

func TestDashboard_CtrlCQuits(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), `[]`)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "No active sessions")
	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	tm.WaitFinished(t, teatest.WithFinalTimeout(waitTimeout))
}

// --- Stop flow ---

func TestDashboard_StopConfirmAndExecute(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	mock.On(serverCmd("stop project-a"), nil, nil)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "project-a")
	tm.Type("d")
	waitFor(t, tm, "[y/n]")
	tm.Type("y")

	time.Sleep(500 * time.Millisecond)
	if !mock.Called(serverCmd("stop project-a")) {
		t.Errorf("expected stop command, got calls: %v", mock.Calls())
	}
}

func TestDashboard_StopCancel(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "project-a")
	tm.Type("d")
	waitFor(t, tm, "[y/n]")
	tm.Type("n")

	time.Sleep(200 * time.Millisecond)
	if mock.Called(serverCmd("stop project-a")) {
		t.Error("stop command should not have been called after cancel")
	}
}

// --- Create view ---

// subdirs JSON responses for drill-down
const workSubdirsJSON = `[{"path":"projects","count":2},{"path":"research","count":2}]`
const researchSubdirsJSON = `[{"path":"project-b","count":0},{"path":"project-c","count":0}]`
const projectsSubdirsJSON = `[{"path":"project-a","count":0},{"path":"project-d","count":0}]`

func TestCreate_ShowsRootDirs(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), `[]`)
	mock.OnJSON(serverCmd("dirs"), dirsJSON)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "No active sessions")
	tm.Type("n")
	waitForAll(t, tm, "Create Session", "work")
}

func TestCreate_DrillDown(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), `[]`)
	mock.OnJSON(serverCmd("dirs"), dirsJSON)
	mock.OnJSON(serverCmd("subdirs "+testHome+"/work"), workSubdirsJSON)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "No active sessions")
	tm.Type("n")
	waitFor(t, tm, "Create Session")

	// Enter on "work" drills down
	sendKey(tm, tea.KeyEnter)
	waitForAll(t, tm, "projects", "research")
}

func TestCreate_EscReturns(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), `[]`)
	mock.OnJSON(serverCmd("dirs"), dirsJSON)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "No active sessions")
	tm.Type("n")
	waitFor(t, tm, "Create Session")

	// Esc at root → back to dashboard
	sendKey(tm, tea.KeyEscape)
	waitFor(t, tm, "No active sessions")
}

func TestCreate_SpaceCreatesSession(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), `[]`)
	mock.OnJSON(serverCmd("dirs"), dirsJSON)
	mock.OnJSON(serverCmd("subdirs "+testHome+"/work"), workSubdirsJSON)
	mock.OnJSON(serverCmd("subdirs "+testHome+"/work/research"), researchSubdirsJSON)
	mock.On(serverCmd("create project-b "+testHome+"/work/research/project-b"), nil, nil)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "No active sessions")
	tm.Type("n")
	waitFor(t, tm, "Create Session")

	// Drill: work → research (mock returns instantly, short sleep to let async complete)
	sendKey(tm, tea.KeyEnter) // into work
	time.Sleep(200 * time.Millisecond)
	sendKey(tm, tea.KeyDown) // cursor to research (index 1)
	sendKey(tm, tea.KeyEnter) // into research
	time.Sleep(200 * time.Millisecond)

	// Space creates session at cursor (project-b, index 0)
	tm.Send(tea.KeyPressMsg{Code: ' ', Text: " "})

	time.Sleep(500 * time.Millisecond)
	if !mock.Called(serverCmd("create project-b " + testHome + "/work/research/project-b")) {
		t.Errorf("expected create command, got calls: %v", mock.Calls())
	}
}

func TestCreate_EscGoesUpThenDashboard(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), `[]`)
	mock.OnJSON(serverCmd("dirs"), dirsJSON)
	mock.OnJSON(serverCmd("subdirs "+testHome+"/work"), workSubdirsJSON)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "No active sessions")
	tm.Type("n")
	waitFor(t, tm, "Create Session")

	// Drill into work
	sendKey(tm, tea.KeyEnter)
	waitFor(t, tm, "research")

	// Esc goes back up to root
	sendKey(tm, tea.KeyEscape)
	waitFor(t, tm, "work")

	// Esc again at root → back to dashboard
	sendKey(tm, tea.KeyEscape)
	waitFor(t, tm, "No active sessions")
}

// --- Create error handling ---

func TestCreate_ErrorShown(t *testing.T) {
	var createCmd = serverCmd("create work " + testHome + "/work")
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), `[]`)
	mock.OnJSON(serverCmd("dirs"), dirsJSON)
	mock.OnDelayed(createCmd, nil, fmt.Errorf("session already exists: work"), 200*time.Millisecond)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "No active sessions")
	tm.Type("n")
	waitFor(t, tm, "Create Session")

	// Space on root "work" creates session
	tm.Send(tea.KeyPressMsg{Code: ' ', Text: " "})

	waitFor(t, tm, "FUSEBOX")
	waitFor(t, tm, "session already exists")
}

func TestCreate_OptimisticAdd(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), `[]`)
	mock.OnJSON(serverCmd("dirs"), dirsJSON)
	mock.OnDelayed(serverCmd("create work "+testHome+"/work"), nil, nil, 2*time.Second)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "No active sessions")
	tm.Type("n")
	waitFor(t, tm, "Create Session")

	// Space creates; session should appear immediately with "creating..." status
	tm.Send(tea.KeyPressMsg{Code: ' ', Text: " "})
	waitForAll(t, tm, "work", "creating")
}

// --- Error handling ---

func TestDashboard_SSHError(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnError(serverCmd("list"), fmt.Errorf("connection refused"))
	tm := newTestApp(t, mock)

	waitFor(t, tm, "Error")
}

func TestDashboard_ListExitErrorShowsError(t *testing.T) {
	mock := testutil.NewMockSSH()
	// Simulate fusebox-helper list failing with exit status (e.g., broken awk output).
	// Previously this was silently swallowed, hiding existing sessions.
	mock.OnError(serverCmd("list"), fmt.Errorf("exit status 1"))
	tm := newTestApp(t, mock)

	waitFor(t, tm, "Error")
}

// --- Preview pane ---

func TestPreview_ToggleOnOff(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	mock.On(serverCmd("preview project-a 30"), []byte("preview output\n"), nil)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "project-a")
	tm.Type("p")
	waitFor(t, tm, "Preview:")

	// Toggle off
	tm.Type("p")
	waitForAbsent(t, tm, "Preview:")
}

func TestPreview_ShowsContent(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	mock.On(serverCmd("preview project-a 30"), []byte("I've updated the test file.\n$ go test ./...\nPASS\n"), nil)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "project-a")
	tm.Type("p")
	waitForAll(t, tm, "Preview:", "go test", "PASS")
}

func TestPreview_ChangesWithSelection(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	mock.On(serverCmd("preview project-a 30"), []byte("ethics output\n"), nil)
	mock.On(serverCmd("preview project-b 30"), []byte("4d output\n"), nil)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "project-a")
	tm.Type("p")
	waitFor(t, tm, "ethics output")

	// Move cursor down to project-b
	sendKey(tm, tea.KeyDown)
	waitFor(t, tm, "4d output")
}

func TestPreview_NoSessionsNoPreview(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), `[]`)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "No active sessions")
	tm.Type("p")

	// Small delay to ensure no preview appears
	time.Sleep(200 * time.Millisecond)
	waitForAbsent(t, tm, "Preview:")
}

func TestPreview_SSHError(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	mock.OnError(serverCmd("preview project-a 30"), fmt.Errorf("connection lost"))
	tm := newTestApp(t, mock)

	waitFor(t, tm, "project-a")
	tm.Type("p")
	waitForAll(t, tm, "Preview:", "Error")
}

// --- Loading & optimistic UI ---

func TestDashboard_ShowsSpinnerOnStartup(t *testing.T) {
	mock := testutil.NewMockSSH()
	// Delay the list response so we can see the spinner
	mock.OnJSONDelayed(serverCmd("list"), sessionsJSON, 500*time.Millisecond)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "Connecting to test-server")
	waitFor(t, tm, "project-a")
}

func TestDashboard_OptimisticStop(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	mock.OnDelayed(serverCmd("stop project-a"), nil, nil, 2*time.Second)
	tm := newTestApp(t, mock)

	waitForAll(t, tm, "project-a", "project-b")
	tm.Type("d")
	waitFor(t, tm, "[y/n]")
	tm.Type("y")

	// Session should show "stopping..." immediately without waiting for SSH
	waitFor(t, tm, "stopping")
}

// waitForAbsent waits until text is NOT present in the latest output.
func waitForAbsent(t *testing.T, tm *teatest.TestModel, text string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return !bytes.Contains(bts, []byte(text))
	}, teatest.WithDuration(waitTimeout))
}

// --- Tool Activity ---

func TestDashboard_ShowsToolActivity(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	mock.OnJSON(serverCmd("activity"), `{"project-a":{"tool":"Edit","detail":"app.go","ts":9999999999}}`)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "Edit app.go")
}

func TestDashboard_FallsBackWithoutActivity(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	// Empty activity — default registered by newTestApp
	tm := newTestApp(t, mock)

	waitForAll(t, tm, "running", "idle")
}

func TestDashboard_ActivityErrorSilentlyIgnored(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	mock.OnError(serverCmd("activity"), fmt.Errorf("connection lost"))
	tm := newTestApp(t, mock)

	// Dashboard should still render sessions despite activity error
	waitForAll(t, tm, "project-a", "project-b")
}

func TestDashboard_ActivityRefreshes(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	mock.OnJSON(serverCmd("activity"), `{}`)
	tm := newTestApp(t, mock)

	waitForAll(t, tm, "project-a", "running")

	// Update activity mock — next tick (5s) will pick it up
	mock.OnJSON(serverCmd("activity"), `{"project-a":{"tool":"Bash","detail":"go test ./...","ts":9999999999}}`)

	// Longer timeout to wait for 5s activity tick
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return bytes.Contains(bts, []byte("Bash: go test ./..."))
	}, teatest.WithDuration(8*time.Second))
}

// --- Teams ---

const teamsJSON = `[{"name":"auth-review","members":[{"name":"lead"},{"name":"researcher"}],"tasks":[{"id":"t1","title":"Review auth","state":"completed"},{"id":"t2","title":"Write tests","state":"in_progress","assigned_to":"researcher"},{"id":"t3","title":"Update docs","state":"pending"}],"pending":1,"in_progress":1,"completed":1,"total":3}]`

const panesJSON = `[{"index":0,"title":"lead","command":"claude","active":true},{"index":1,"title":"researcher","command":"claude","active":false}]`
const singlePaneJSON = `[{"index":0,"title":"","command":"claude","active":true}]`

func TestDashboard_TeamStatusShown(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	mock.OnJSON(serverCmd("teams"), teamsJSON)
	mock.OnJSON(serverCmd("panes project-a"), panesJSON)
	mock.OnJSON(serverCmd("panes project-b"), singlePaneJSON)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "Team: auth-review 1/3")
}

func TestDashboard_TeamKeyOpensDetail(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	mock.OnJSON(serverCmd("teams"), teamsJSON)
	mock.OnJSON(serverCmd("panes project-a"), panesJSON)
	mock.OnJSON(serverCmd("panes project-b"), singlePaneJSON)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "Team: auth-review")
	tm.Type("t")
	waitForAll(t, tm, "Teammates", "lead", "researcher")
}

func TestDashboard_TeamDetailShowsTasks(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	mock.OnJSON(serverCmd("teams"), teamsJSON)
	mock.OnJSON(serverCmd("panes project-a"), panesJSON)
	mock.OnJSON(serverCmd("panes project-b"), singlePaneJSON)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "Team: auth-review")
	tm.Type("t")
	waitForAll(t, tm, "Tasks (1/3)", "Review auth", "Write tests", "Update docs")
}

func TestDashboard_TeamDetailEscReturns(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	mock.OnJSON(serverCmd("teams"), teamsJSON)
	mock.OnJSON(serverCmd("panes project-a"), panesJSON)
	mock.OnJSON(serverCmd("panes project-b"), singlePaneJSON)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "Team: auth-review")
	tm.Type("t")
	waitFor(t, tm, "Teammates")

	sendKey(tm, tea.KeyEscape)
	// Check for dashboard elements — "2 active sessions" can be split by terminal escape codes
	waitForAll(t, tm, "project-a", "project-b", "[n] new")
}

func TestDashboard_NoTeams_TKeyNoop(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	tm := newTestApp(t, mock)

	waitForAll(t, tm, "project-a", "project-b")
	tm.Type("t")

	// Should still be on dashboard — just check no team detail
	time.Sleep(300 * time.Millisecond)
	waitForAbsent(t, tm, "Teammates")
}

func TestDashboard_HelpBarShowsTeamKey(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), sessionsJSON)
	mock.OnJSON(serverCmd("teams"), teamsJSON)
	mock.OnJSON(serverCmd("panes project-a"), panesJSON)
	mock.OnJSON(serverCmd("panes project-b"), singlePaneJSON)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "[t] team")
}

func TestCreate_TeamsToggle(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), `[]`)
	mock.OnJSON(serverCmd("dirs"), dirsJSON)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "No active sessions")
	tm.Type("n")
	waitForAll(t, tm, "Create Session", "work")

	// Toggle teams on, then force full repaint via resize
	tm.Send(tea.KeyPressMsg{Code: 't', Text: "t"})
	tm.Send(tea.WindowSizeMsg{Width: 101, Height: 30})
	waitFor(t, tm, "Teams: ON")

	// Toggle back off
	tm.Send(tea.KeyPressMsg{Code: 't', Text: "t"})
	tm.Send(tea.WindowSizeMsg{Width: 100, Height: 30})
	waitFor(t, tm, "Teams: OFF")
}

func TestCreate_TeamSessionUsesCreateTeam(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnJSON(serverCmd("list"), `[]`)
	mock.OnJSON(serverCmd("dirs"), dirsJSON)
	mock.On(serverCmd("create-team work "+testHome+"/work"), nil, nil)
	tm := newTestApp(t, mock)

	waitFor(t, tm, "No active sessions")
	tm.Type("n")
	waitForAll(t, tm, "Create Session", "work")

	// Toggle teams on, force repaint
	tm.Send(tea.KeyPressMsg{Code: 't', Text: "t"})
	tm.Send(tea.WindowSizeMsg{Width: 101, Height: 30})
	waitFor(t, tm, "Teams: ON")

	// Space creates session with teams
	tm.Send(tea.KeyPressMsg{Code: ' ', Text: " "})

	time.Sleep(500 * time.Millisecond)
	if !mock.Called(serverCmd("create-team work " + testHome + "/work")) {
		t.Errorf("expected create-team command, got calls: %v", mock.Calls())
	}
}
