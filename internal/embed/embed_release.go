//go:build embed_server

package embed

import _ "embed"

//go:embed fusebox-linux-arm64
var LinuxArm64 []byte

//go:embed fusebox-linux-amd64
var LinuxAmd64 []byte
