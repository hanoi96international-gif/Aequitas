package keeper

import (
	"runtime/debug"
	"strings"
)

// buildGitCommit is the short git commit SHA the running binary was built
// from, read from Go's automatic VCS build-info stamping (available since
// Go 1.18 whenever `go build` runs inside a git checkout).
//
// CORRECTION (audit 2026-08-15): this comment used to assert that the Docker
// build "does not strip .git from the build context". That stopped being true
// on 2026-08-14, when .dockerignore was introduced to cut a 1.22 GB build
// context down — .git alone was 1079 MB of it, and excluding it was the right
// call. The side effect is that the builder sees no repository, so Go stamps
// nothing and every node reports "unknown". Confirmed live on both validators.
//
// The consequence is not cosmetic: the explorer's Network tab compares this
// value across nodes to answer "is the whole fleet on the same build?", which
// is exactly the question a rollout needs and which it can therefore never
// answer. Closing it needs the commit to reach the build from outside the
// image — a `--build-arg` stamped through -ldflags, or an env var read at
// runtime — which means changing the box-side deploy script, not this file.
// Recorded in docs/LAUNCH_2026-08-18.md rather than half-done here.
//
// Computed once at startup; "unknown" when no VCS info is present.
//
// This exists so every node in the fleet (Railway primary, Contabo 1,
// Contabo 2) can be checked against the others from its own /api/status —
// see handleStatus in api.go — without needing SSH or CI access to each one
// individually, which was the actual blocker behind repeatedly having to
// ask "are all nodes really on the same commit?" instead of just checking.
// buildGitCommitStamp is set at link time by the Docker build
// (-ldflags -X, see Dockerfile). Empty for a plain `go build`, which then
// falls back to Go's own VCS stamping — correct for a developer building
// inside the checkout.
var buildGitCommitStamp string

var buildGitCommit = resolveBuildGitCommit()

// resolveBuildGitCommit prefers the explicit link-time stamp, because the
// image build cannot see .git and Go therefore stamps nothing there.
func resolveBuildGitCommit() string {
	if s := strings.TrimSpace(buildGitCommitStamp); s != "" && s != "unknown" {
		if len(s) > 8 {
			s = s[:8]
		}
		return s
	}
	return readBuildGitCommit()
}

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
