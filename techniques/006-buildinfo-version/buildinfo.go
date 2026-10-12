// Package buildinfo is Technique 6: a version endpoint for free,
// read from the build info every module-mode binary carries.
package buildinfo

import (
	"runtime/debug"
	"sync"
)

// Version is a "dev" string that -X can override.
var Version = "dev"

// Build is what the endpoint reports.
type Build struct {
	Version  string `json:"version"`
	Go       string `json:"go"`
	Revision string `json:"revision"`
	Dirty    bool   `json:"dirty"`
}

// BuildInfo runs once and caches the result: build info never
// changes while the process lives.
var BuildInfo = sync.OnceValue(func() Build {
	b := Build{Version: Version}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return b
	}
	b.Go = bi.GoVersion
	if b.Version == "dev" {
		b.Version = bi.Main.Version
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			b.Revision = s.Value
		case "vcs.modified":
			b.Dirty = s.Value == "true"
		}
	}
	return b
})
