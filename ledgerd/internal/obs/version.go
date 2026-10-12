// Package obs holds ledgerd's logs, metrics, traces and the
// private debug server.
package obs

import (
	"runtime/debug"
	"sync"
)

// Version is stamped at link time with
// -X example.com/ledgerd/internal/obs.Version=... (Technique 5).
var Version = "dev"

// Build is what /version reports (Techniques 6 and 49).
type Build struct {
	Version  string `json:"version"`
	Go       string `json:"go"`
	Revision string `json:"revision,omitzero"`
	Dirty    bool   `json:"dirty"`
}

// BuildInfo reads the binary's build info once (Technique 6).
// An explicit -X value wins over the Go 1.24 VCS version.
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
