package embed

import "fmt"

// ServerBinary returns the embedded Linux server binary for the given architecture.
func ServerBinary(goarch string) ([]byte, error) {
	switch goarch {
	case "arm64":
		if len(LinuxArm64) == 0 {
			return nil, fmt.Errorf("no embedded arm64 binary (dev build — use a release build or run `make release`)")
		}
		return LinuxArm64, nil
	case "amd64":
		if len(LinuxAmd64) == 0 {
			return nil, fmt.Errorf("no embedded amd64 binary (dev build — use a release build or run `make release`)")
		}
		return LinuxAmd64, nil
	default:
		return nil, fmt.Errorf("unsupported server architecture: %s", goarch)
	}
}
