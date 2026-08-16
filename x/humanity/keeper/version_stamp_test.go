package keeper

import "testing"

// git_commit read "unknown" on every live node, so the explorer's Network tab
// could not answer "is the whole fleet on the same build?" — the one question a
// rollout asks. Cause: Go stamps vcs.revision only when the build can see a
// repository, and .dockerignore excludes .git (correctly — it was 1079 MB of a
// 1.22 GB build context). The commit now arrives as a link-time stamp instead.
func TestResolveBuildGitCommit_PrefersTheLinkTimeStamp(t *testing.T) {
	orig := buildGitCommitStamp
	defer func() { buildGitCommitStamp = orig }()

	cases := []struct {
		stamp string
		want  string
		note  string
	}{
		{"abc12345", "abc12345", "a short sha is used as-is"},
		{"abc12345def67890", "abc12345", "a full sha is trimmed to eight"},
		{"  abc12345  ", "abc12345", "whitespace from the build arg is stripped"},
	}
	for _, tc := range cases {
		buildGitCommitStamp = tc.stamp
		if got := resolveBuildGitCommit(); got != tc.want {
			t.Errorf("%s: stamp %q -> %q, want %q", tc.note, tc.stamp, got, tc.want)
		}
	}
}

// Without a stamp — a plain `go build` in a checkout — it must fall back to Go's
// own VCS info rather than reporting a hardcoded value.
func TestResolveBuildGitCommit_FallsBackWhenUnstamped(t *testing.T) {
	orig := buildGitCommitStamp
	defer func() { buildGitCommitStamp = orig }()

	for _, empty := range []string{"", "   ", "unknown"} {
		buildGitCommitStamp = empty
		if got := resolveBuildGitCommit(); got != readBuildGitCommit() {
			t.Errorf("stamp %q: got %q, want the VCS fallback %q", empty, got, readBuildGitCommit())
		}
	}
}
