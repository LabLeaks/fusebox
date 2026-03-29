package sync

import "time"

// SyncState represents the current state of a Mutagen sync session.
type SyncState string

const (
	StateSyncing  SyncState = "syncing"
	StateWatching SyncState = "watching"
	StateError    SyncState = "error"
	StatePaused   SyncState = "paused"
	StateUnknown  SyncState = "unknown"
)

// classifyStatus maps Mutagen's status string to a SyncState.
func classifyStatus(status string) SyncState {
	switch status {
	case "Watching for changes":
		return StateWatching
	case "Paused":
		return StatePaused
	case "Unknown":
		return StateUnknown
	default:
		// Scanning files, Staging files, Reconciling, Transitioning, etc.
		return StateSyncing
	}
}

// SyncWaiter abstracts sync-wait so both sides (local daemon + remote exec)
// can wait for sync without depending on MutagenManager directly.
type SyncWaiter interface {
	WaitForWatching(sessionName string, timeout time.Duration) (SyncState, error)
	State(sessionName string) (SyncState, error)
}

// mutagenSyncWaiter implements SyncWaiter using MutagenManager.
type mutagenSyncWaiter struct {
	mgr *MutagenManager
}

// NewSyncWaiter creates a SyncWaiter backed by the Mutagen CLI.
func NewSyncWaiter(mgr *MutagenManager) SyncWaiter {
	return &mutagenSyncWaiter{mgr: mgr}
}

// State returns the current sync state for the named session.
func (w *mutagenSyncWaiter) State(sessionName string) (SyncState, error) {
	status, err := w.mgr.SessionStatus(sessionName)
	if err != nil {
		return StateError, err
	}
	return classifyStatus(status), nil
}

// WaitForWatching blocks until the session reaches the Watching state or
// the timeout expires. Returns the last observed state.
func (w *mutagenSyncWaiter) WaitForWatching(sessionName string, timeout time.Duration) (SyncState, error) {
	err := w.mgr.WaitForSync(sessionName, timeout)
	if err != nil {
		// On timeout, return the last state for the caller to decide
		state, stateErr := w.State(sessionName)
		if stateErr != nil {
			return StateUnknown, err
		}
		return state, err
	}
	return StateWatching, nil
}
