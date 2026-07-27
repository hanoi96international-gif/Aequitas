package keeper

import (
	"os"
	"testing"
)

// The database pool size now comes from here. A malformed value silently
// becoming zero would mean a pool of zero connections — a node that cannot
// reach its own database, failing in a way that looks nothing like a typo.
func TestIntFromEnv_BadValuesKeepTheDefault(t *testing.T) {
	const key = "AEQUITAS_TEST_INT_FROM_ENV"
	for _, bad := range []string{"", "abc", "0", "-5", "12.5", " "} {
		t.Setenv(key, bad)
		if got := intFromEnv(key, 20); got != 20 {
			t.Fatalf("%q yielded %d; anything but the default risks a pool of zero connections", bad, got)
		}
	}
	os.Unsetenv(key)
	if got := intFromEnv(key, 20); got != 20 {
		t.Fatalf("an unset variable must yield the default, got %d", got)
	}
}

func TestIntFromEnv_GoodValueIsUsed(t *testing.T) {
	const key = "AEQUITAS_TEST_INT_FROM_ENV"
	t.Setenv(key, "64")
	if got := intFromEnv(key, 20); got != 64 {
		t.Fatalf("expected 64, got %d", got)
	}
}
