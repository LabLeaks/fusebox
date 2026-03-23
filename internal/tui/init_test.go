package tui_test

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/lableaks/fusebox/internal/ssh"
	"github.com/lableaks/fusebox/internal/testutil"
	"github.com/lableaks/fusebox/internal/tui"
)

// newInitApp creates a test init model wired to a mock SSH factory.
// Sets XDG_CONFIG_HOME to a temp dir to prevent loading real config.
// When no hostArg is given, selects remote mode ('r') to skip the mode screen.
func newInitApp(t *testing.T, hostArg string, mock *testutil.MockSSH) *teatest.TestModel {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	factory := func(host, user string) ssh.Runner { return mock }
	model := tui.NewInitWithSSH(hostArg, factory)
	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(100, 30))
	t.Cleanup(func() { tm.Quit() })
	// If no host arg, we land on mode selection — pick remote
	if hostArg == "" {
		waitFor(t, tm, "Local")
		tm.Type("r")
	}
	return tm
}

func TestInit_ShowsSetupHeader(t *testing.T) {
	mock := testutil.NewMockSSH()
	tm := newInitApp(t, "", mock)

	waitFor(t, tm, "Server")
}

func TestInit_HostToUser(t *testing.T) {
	mock := testutil.NewMockSSH()
	tm := newInitApp(t, "", mock)

	// Type hostname + enter, then check for username input (no intermediate waitFor)
	time.Sleep(200 * time.Millisecond) // let Init()/Focus() settle
	tm.Type("myserver")
	sendKey(tm, tea.KeyEnter)

	// "Username:" only appears in the input area at stepUser
	waitFor(t, tm, "Username:")
}

func TestInit_BackFromUser(t *testing.T) {
	mock := testutil.NewMockSSH()
	tm := newInitApp(t, "", mock)

	time.Sleep(200 * time.Millisecond)
	tm.Type("myserver")
	sendKey(tm, tea.KeyEnter)

	// Wait until we're at user step, then go back
	waitFor(t, tm, "Username:")
	sendKey(tm, tea.KeyEscape)

	// "Server:" re-appears in the input area when back at host step
	waitFor(t, tm, "Server:")
}

func TestInit_SSHFailure(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnDelayed("uname -m && echo $HOME", nil, fmt.Errorf("connection refused"), 100*time.Millisecond)
	tm := newInitApp(t, "testuser@badhost", mock)

	waitFor(t, tm, "SSH connection failed")
}

func TestInit_DeployFailureOnDevBuild(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnDelayed("uname -m && echo $HOME", []byte("aarch64\n/home/testuser\n"), nil, 100*time.Millisecond)
	tm := newInitApp(t, "testuser@myserver", mock)

	// SSH succeeds, then deploy fails (no embedded binary in dev build)
	waitFor(t, tm, "dev build")
}

func TestInit_ConnectShowsProgress(t *testing.T) {
	mock := testutil.NewMockSSH()
	mock.OnDelayed("uname -m && echo $HOME", []byte("aarch64\n/home/testuser\n"), nil, 1*time.Second)
	tm := newInitApp(t, "testuser@myserver", mock)

	// Both appear in the first render at stepConnect
	waitForAll(t, tm, "Connecting", "Testing SSH")
}

func TestInit_CtrlCQuits(t *testing.T) {
	mock := testutil.NewMockSSH()
	tm := newInitApp(t, "", mock)

	waitFor(t, tm, "Server")
	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	tm.WaitFinished(t, teatest.WithFinalTimeout(waitTimeout))
}

func TestInit_EmptyHostRejected(t *testing.T) {
	mock := testutil.NewMockSSH()
	tm := newInitApp(t, "", mock)

	// Press enter with empty host — no transition, "Username:" never appears
	time.Sleep(200 * time.Millisecond)
	sendKey(tm, tea.KeyEnter)
	time.Sleep(200 * time.Millisecond)
	waitForAbsent(t, tm, "Username:")
}

func TestMapArch(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"aarch64", "arm64"},
		{"arm64", "arm64"},
		{"x86_64", "amd64"},
		{"aarch64\n", "arm64"},
		{"  x86_64  ", "amd64"},
		{"riscv64", "riscv64"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := tui.MapArch(tt.input)
			if got != tt.want {
				t.Errorf("MapArch(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestInit_NoReconfigWithoutConfig(t *testing.T) {
	mock := testutil.NewMockSSH()
	tm := newInitApp(t, "", mock)

	waitForAbsent(t, tm, "reconfiguring")
}
