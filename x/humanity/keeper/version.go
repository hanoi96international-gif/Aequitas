package keeper

import "runtime/debug"

// buildGitCommit is the short git commit SHA the running binary was built
// from, read from Go's automatic VCS build-info stamping (available since
// Go 1.18 whenever `go build` runs inside a git checkout — true for both
// Railway's Docker build and a plain `go build` on a Contabo host pulling
// this repo, since neither strips .git from the build context). Computed
// once at startup; "unknown" if the binary was built without VCS info
// (e.g. `go build` outside a git checkout, or GOFLAGS=-buildvcs=false).
//
// This exists so every node in the fleet (Railway primary, Contabo 1,
// Contabo 2) can be checked against the others from its own /api/status —
// see handleStatus in api.go — without needing SSH or CI access to each one
// individually, which was the actual blocker behind repeatedly having to
// ask "are all nodes really on the same commit?" instead of just checking.
var buildGitCommit = readBuildGitCommit()

func readBuildGitCommit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	var revision string
	var modified bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if revision == "" {
		return "unknown"
	}
	if len(revision) > 8 {
		revision = revision[:8]
	}
	if modified {
		revision += "-dirty"
	}
	return revision
}
