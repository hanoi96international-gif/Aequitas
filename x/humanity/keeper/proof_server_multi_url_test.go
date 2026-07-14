package keeper

import (
	"errors"
	"net/http"
	"os"
	"testing"
)

// setEnvForTest sets an env var and returns a cleanup func, since this
// package's tests don't have a shared test-env helper for this.
func setEnvForTest(t *testing.T, key, value string) {
	t.Helper()
	old, hadOld := os.LookupEnv(key)
	os.Setenv(key, value)
	t.Cleanup(func() {
		if hadOld {
			os.Setenv(key, old)
		} else {
			os.Unsetenv(key)
		}
	})
}

func TestProofServerURLs_ParsesCommaSeparatedList(t *testing.T) {
	setEnvForTest(t, "PROOF_SERVER_URLS", " http://a.example/ , http://b.example, http://c.example/ ")
	setEnvForTest(t, "PROOF_SERVER_URL", "")
	got := proofServerURLs()
	want := []string{"http://a.example", "http://b.example", "http://c.example"}
	if len(got) != len(want) {
		t.Fatalf("got %d URLs, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("URL %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestProofServerURLs_FallsBackToSingularForBackwardCompat(t *testing.T) {
	setEnvForTest(t, "PROOF_SERVER_URLS", "")
	setEnvForTest(t, "PROOF_SERVER_URL", "http://solo.example/")
	got := proofServerURLs()
	if len(got) != 1 || got[0] != "http://solo.example" {
		t.Fatalf("expected fallback to single trimmed URL, got %v", got)
	}
}

func TestProofServerURLs_EmptyWhenNeitherSet(t *testing.T) {
	setEnvForTest(t, "PROOF_SERVER_URLS", "")
	setEnvForTest(t, "PROOF_SERVER_URL", "")
	if got := proofServerURLs(); len(got) != 0 {
		t.Fatalf("expected no URLs configured, got %v", got)
	}
}

// --- attemptURLsInOrder: pure failover-decision logic, no real network I/O ---

func TestAttemptURLsInOrder_FallsOverOnError(t *testing.T) {
	var called []string
	resp, err := attemptURLsInOrder([]string{"a", "b", "c"}, func(url string) (*http.Response, error) {
		called = append(called, url)
		if url == "a" {
			return nil, errors.New("simulated transport error")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"X-Url": []string{url}}}, nil
	})
	if err != nil {
		t.Fatalf("expected success via failover, got err: %v", err)
	}
	if resp.Header.Get("X-Url") != "b" {
		t.Fatalf("expected the second URL's response, got response tagged %q", resp.Header.Get("X-Url"))
	}
	if len(called) != 2 || called[0] != "a" || called[1] != "b" {
		t.Fatalf("expected exactly a, b to be tried (not c, since b succeeded), got %v", called)
	}
}

func TestAttemptURLsInOrder_DoesNotRetryOnSuccessfulButErrorStatusResponse(t *testing.T) {
	// fn returning a nil error (a response was actually received, regardless
	// of its HTTP status) must stop the loop immediately -- a 409
	// "already registered" from the first instance is meaningful and must
	// not trigger asking a second instance.
	var called []string
	_, err := attemptURLsInOrder([]string{"first", "second"}, func(url string) (*http.Response, error) {
		called = append(called, url)
		return &http.Response{StatusCode: 409}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(called) != 1 || called[0] != "first" {
		t.Fatalf("expected only the first URL to be tried, got %v", called)
	}
}

func TestAttemptURLsInOrder_ReturnsLastErrorWhenAllFail(t *testing.T) {
	_, err := attemptURLsInOrder([]string{"a", "b"}, func(url string) (*http.Response, error) {
		return nil, errors.New(url + " failed")
	})
	if err == nil || err.Error() != "b failed" {
		t.Fatalf("expected the LAST url's error, got: %v", err)
	}
}

// --- aggregateBroadcastResults: pure broadcast-decision logic ---

func TestAggregateBroadcastResults_NilWhenAllSucceed(t *testing.T) {
	err := aggregateBroadcastResults(map[string]error{
		"http://a": nil,
		"http://b": nil,
	})
	if err != nil {
		t.Fatalf("expected nil error when every instance succeeded, got: %v", err)
	}
}

func TestAggregateBroadcastResults_NilOnPartialSuccess(t *testing.T) {
	// Correctness doesn't depend on every instance being in sync -- the
	// chain's own is_human check is authoritative regardless -- so a
	// partial failure must not be treated as an overall failure.
	err := aggregateBroadcastResults(map[string]error{
		"http://a": nil,
		"http://b": errors.New("unreachable"),
	})
	if err != nil {
		t.Fatalf("expected nil error on partial success, got: %v", err)
	}
}

func TestAggregateBroadcastResults_ErrorWhenAllFail(t *testing.T) {
	err := aggregateBroadcastResults(map[string]error{
		"http://a": errors.New("unreachable"),
		"http://b": errors.New("timeout"),
	})
	if err == nil {
		t.Fatal("expected an error when every instance failed")
	}
}

// --- notifyProofServer: the configured/skip gating (no network needed) ---

func TestNotifyProofServer_SkippedWhenNoURLConfigured(t *testing.T) {
	setEnvForTest(t, "PROOF_SERVER_URLS", "")
	setEnvForTest(t, "PROOF_SERVER_URL", "")
	setEnvForTest(t, "CHAIN_SERVICE_TOKEN", "test-token")

	attempted, err := notifyProofServer("0xabc", "0xwallet")
	if attempted {
		t.Fatal("expected attempted=false when no proof-server URL is configured")
	}
	if err != nil {
		t.Fatalf("expected no error for a deliberate skip, got: %v", err)
	}
}

func TestNotifyProofServer_SkippedWhenNoToken(t *testing.T) {
	setEnvForTest(t, "PROOF_SERVER_URLS", "http://a.example,http://b.example")
	setEnvForTest(t, "PROOF_SERVER_URL", "")
	setEnvForTest(t, "CHAIN_SERVICE_TOKEN", "")

	attempted, err := notifyProofServer("0xabc", "0xwallet")
	if attempted {
		t.Fatal("expected attempted=false when CHAIN_SERVICE_TOKEN is unset")
	}
	if err != nil {
		t.Fatalf("expected no error for a deliberate skip, got: %v", err)
	}
}
