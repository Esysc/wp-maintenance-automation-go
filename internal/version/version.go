// Package version exposes the application version embedded at build time.
package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var rawVersion string

// Version is the application version read from the VERSION file. Release
// automation updates that file so the API health endpoint and CLI banner stay
// in sync with the git tag.
var Version = strings.TrimSpace(rawVersion)
