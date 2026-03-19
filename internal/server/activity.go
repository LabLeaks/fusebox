package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// readActivityDir reads tool activity files from a directory and returns
// entries whose timestamp is within maxAge seconds of now.
func readActivityDir(statusDir string, maxAge int64) map[string]json.RawMessage {
	now := time.Now().Unix()
	result := map[string]json.RawMessage{}

	entries, err := os.ReadDir(statusDir)
	if err != nil {
		return result
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		data, err := os.ReadFile(filepath.Join(statusDir, entry.Name()))
		if err != nil {
			continue
		}

		// Parse just enough to check timestamp
		var activity struct {
			TS int64 `json:"ts"`
		}
		if err := json.Unmarshal(data, &activity); err != nil {
			continue
		}
		if now-activity.TS > maxAge {
			continue
		}

		// Preserve original JSON verbatim
		result[name] = json.RawMessage(data)
	}

	return result
}

// CmdActivity reads tool activity files and outputs recent activity as JSON.
func CmdActivity() {
	writeJSON(readActivityDir("/tmp/work-cli", 60))
}
