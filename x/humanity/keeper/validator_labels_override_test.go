package keeper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestLoadValidatorLabelOverrides_Parsing checks the VALIDATOR_LABELS env
// var parser directly: valid pairs, whitespace tolerance, and that
// malformed entries (bad address shape, missing colon) are skipped rather
// than corrupting the whole map or panicking.
func TestLoadValidatorLabelOverrides_Parsing(t *testing.T) {
	t.Setenv("VALIDATOR_LABELS", "Primary:0xB67100000000000000000000000000000000B671, Validator 1 : 0xD01600000000000000000000000000000000D016,Validator 2:0xa4E300000000000000000000000000000000a4E3,garbage-no-colon,BadAddr:0x123")
	got := loadValidatorLabelOverrides()

	want := map[string]string{
		"0xb67100000000000000000000000000000000b671": "Primary",
		"0xd01600000000000000000000000000000000d016": "Validator 1",
		"0xa4e300000000000000000000000000000000a4e3": "Validator 2",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(got), len(want), got)
	}
	for addr, label := range want {
		if got[addr] != label {
			t.Errorf("addr %s: got label %q, want %q", addr, got[addr], label)
		}
	}
}

// TestLoadValidatorLabelOverrides_Empty confirms the unset-env-var default
// (empty map, not nil-panic) — the fallback-to-ordinals path in
// handleValidatorLabels depends on this being a safe, iterable empty map.
func TestLoadValidatorLabelOverrides_Empty(t *testing.T) {
	t.Setenv("VALIDATOR_LABELS", "")
	got := loadValidatorLabelOverrides()
	if got == nil {
		t.Fatal("expected empty map, got nil")
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 entries, got %d: %v", len(got), got)
	}
}

// TestHandleValidatorLabels_OverrideTakesPrecedence exercises the actual
// HTTP handler: an address covered by VALIDATOR_LABELS must return that
// exact string, not a "Validator #N" computed from GetValidatorOrdinals —
// this is the launch-night fix itself (B671=Primary, D016=Validator 1,
// a4E3=Validator 2), verified end-to-end through the same code path the
// explorer actually calls.
func TestHandleValidatorLabels_OverrideTakesPrecedence(t *testing.T) {
	primary := "0xB67100000000000000000000000000000000B671"
	v1 := "0xD01600000000000000000000000000000000D016"
	v2 := "0xa4E300000000000000000000000000000000a4E3"

	// t.Setenv auto-restores the env var when the test ends; the package var
	// derived from it needs its own matching restore, registered AFTER (so
	// it runs BEFORE, deferred cleanups are LIFO) t.Setenv's implicit
	// restore has taken effect — otherwise this reloads validatorLabelOverrides
	// from the already-restored (i.e. wrong, still-old) env var and leaves
	// the NEXT test in this package running with THIS test's override map.
	oldOverrides := validatorLabelOverrides
	t.Cleanup(func() { validatorLabelOverrides = oldOverrides })
	t.Setenv("VALIDATOR_LABELS", "Primary:"+primary+",Validator 1:"+v1+",Validator 2:"+v2)
	validatorLabelOverrides = loadValidatorLabelOverrides()

	a := &APIServer{} // blockchain/state nil — handler must degrade to overrides-only, not panic
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/validator-labels", nil)
	a.handleValidatorLabels(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var resp struct {
		Labels map[string]string `json:"labels"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("could not decode response: %v (body=%s)", err, rr.Body.String())
	}

	cases := map[string]string{
		"0xb67100000000000000000000000000000000b671": "Primary",
		"0xd01600000000000000000000000000000000d016": "Validator 1",
		"0xa4e300000000000000000000000000000000a4e3": "Validator 2",
	}
	for addr, want := range cases {
		if got := resp.Labels[addr]; got != want {
			t.Errorf("addr %s: got label %q, want %q (full response: %v)", addr, got, want, resp.Labels)
		}
	}
}
