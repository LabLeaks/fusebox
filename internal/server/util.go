package server

import (
	"encoding/json"
	"fmt"
	"os"
)

// ExitError writes a JSON error to stderr and exits with code 1.
func ExitError(msg string) {
	data, _ := json.Marshal(map[string]string{"error": msg})
	fmt.Fprintln(os.Stderr, string(data))
	os.Exit(1)
}

// writeJSON marshals v to JSON and writes to stdout (no trailing newline).
func writeJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		ExitError("json marshal: " + err.Error())
	}
	os.Stdout.Write(data)
}
