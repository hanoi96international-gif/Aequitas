package keeper

import "testing"

// TestCheckpointIsIncoherent covers the invariant that hung Contabo twice:
// a finalized checkpoint can never legitimately sit above the height this
// node has actually synced.
func TestCheckpointIsIncoherent(t *testing.T) {
	cases := []struct {
		name            string
		finalizedHeight int64
		syncedHeight    int64
		want            bool
	}{
		{"stale checkpoint from a pre-a12009d resync (real Contabo values)", 67293, 0, true},
		{"checkpoint behind synced height is coherent", 100, 200, false},
		{"checkpoint equal to synced height is coherent", 100, 100, false},
		{"fresh node, nothing finalized yet", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkpointIsIncoherent(tc.finalizedHeight, tc.syncedHeight); got != tc.want {
				t.Errorf("checkpointIsIncoherent(%d, %d) = %v, want %v", tc.finalizedHeight, tc.syncedHeight, got, tc.want)
			}
		})
	}
}
