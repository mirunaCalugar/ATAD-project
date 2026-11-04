// Package version reports what this binary is and which commit it was built
// from. Do not change the JSON field names: the marking script reads them and
// compares Commit against your repository.
//
// The three values below are empty in a local build and are filled in at image
// build time by the linker. You never edit them by hand.
package version

import (
	"encoding/json"
	"io"
	"runtime/debug"
)

// Set by the linker via -ldflags in the Dockerfile; see the ARG lines there.
var (
	App     = "atad-project"
	Version = "0.0.0-dev"
	Commit  = ""
	BuiltAt = ""
)

// Info is the payload reported by --version and, for a service, by /version.
type Info struct {
	App     string `json:"app"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"built_at"`
}

// Current returns the build information. When the binary was built outside the
// container (go run, go build), Commit falls back to the revision Go records in
// the binary itself, so a local build still reports something truthful.
func Current() Info {
	commit := Commit
	if commit == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				if s.Key == "vcs.revision" && len(s.Value) >= 7 {
					commit = s.Value[:7]
				}
			}
		}
	}
	if commit == "" {
		commit = "unknown"
	}
	return Info{App: App, Version: Version, Commit: commit, BuiltAt: BuiltAt}
}

// WriteJSON writes the build information as one line of JSON.
func WriteJSON(w io.Writer) error {
	return json.NewEncoder(w).Encode(Current())
}
