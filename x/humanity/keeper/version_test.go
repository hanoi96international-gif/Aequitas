package keeper

import "testing"

// TestReadBuildGitCommit_NeverPanicsOrEmpty is the regression guard for
// /api/status's git_commit field (api.go): whatever the build environment
// (a real git checkout, `go test` itself, a stripped GOFLAGS=-buildvcs=false
// build), this must return a non-empty, sane placeholder rather than
// panicking or leaving the JSON field blank — a node that can't report its
// own commit should say so explicitly ("unknown"), not silently omit the
// one field the fleet-sync check (loadTopology, explorer.js) depends on.
func TestReadBuildGitCommit_NeverPanicsOrEmpty(t *testing.T) {
	got := readBuildGitCommit()
	if got == "" {
		t.Fatal("readBuildGitCommit() returned an empty string — /api/status would ship git_commit:\"\" instead of a diagnosable value")
	}
	if len(got) > 17 { // 8 hex chars + "-dirty" (6) + 1 slack, generous
		t.Fatalf("readBuildGitCommit() = %q, suspiciously long for a short hash (+ optional -dirty suffix)", got)
	}
}

// TestBuildGitCommit_IsPackageLevelAndStable pins that the package-level
// var (what handleStatus actually reads) is computed once at init and
// doesn't require a fresh call per request.
func TestBuildGitCommit_IsPackageLevelAndStable(t *testing.T) {
	if buildGitCommit == "" {
		t.Fatal("buildGitCommit package var is empty")
	}
	if buildGitCommit != readBuildGitCommit() {
		t.Fatalf("buildGitCommit (%q) diverged from a fresh readBuildGitCommit() call (%q) — should be identical within one build", buildGitCommit, readBuildGitCommit())
	}
}
