//go:build embed_server

package embed

import _ "embed"

//go:embed work-linux-arm64
var LinuxArm64 []byte

//go:embed work-linux-amd64
var LinuxAmd64 []byte
