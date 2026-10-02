package web

import (
	"embed"
)

// AssetsFS embeds the web dashboard static assets into the Go binary.
//
//go:embed index.html
var AssetsFS embed.FS
