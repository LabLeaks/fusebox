package sync

import (
	"fmt"
	"io"
	"time"
)

// WaitForSyncWithLog waits for sync and logs progress to the writer.
// This is the high-level function used by both the local daemon (pre-exec)
// and the remote CLI (pre-request).
func WaitForSyncWithLog(w SyncWaiter, sessionName string, timeout time.Duration, log io.Writer) error {
	state, err := w.State(sessionName)
	if err != nil {
		return fmt.Errorf("checking sync state: %w", err)
	}

	if state == StateWatching {
		return nil
	}

	fmt.Fprintf(log, "[sync] waiting for sync to complete...\n")

	state, err = w.WaitForWatching(sessionName, timeout)
	if err != nil {
		fmt.Fprintf(log, "[sync] warning: sync not confirmed after %v, proceeding anyway\n", timeout)
		return nil
	}

	fmt.Fprintf(log, "[sync] synced, proceeding\n")
	return nil
}
