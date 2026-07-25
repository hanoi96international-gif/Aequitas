package keeper

import (
	"errors"
	"testing"
)

// This file is the regression guard for the 2026-07-25 "es merged nix"
// incident: both secondaries got stuck retrying an identical failing
// 500-block /api/blocks page against the primary forever, unable to
// advance at all, right after the primary itself restarted under load.
// Root cause (see api.go's handleBlocks and sync_blocks.go's
// fetchBlocksSince/fetchWithSmallerPageFallback comments for the full
// mechanism): a large page response got cut off mid-write, and the client
// silently discarded the read error, turning "the connection was
// interrupted" into a permanently-reproducible "unexpected end of JSON
// input" with no way to make progress.
//
// Tests fetchWithSmallerPageFallback directly, against a fake attempt
// function, rather than through fetchBlocksSince/fetchBlocksSinceWithFallback
// — those always dial through httpSyncClient's pinningDialer, which
// deliberately rejects loopback/private addresses (an SSRF/DNS-rebinding
// guard this test must not weaken just for testability), so they can't be
// exercised against an httptest.Server here. fetchWithSmallerPageFallback is
// the actual retry POLICY under test; the HTTP plumbing around it is
// unchanged, ordinary code.

func TestFetchWithSmallerPageFallback_LargeFails_SmallSucceeds(t *testing.T) {
	want := []*Block{{Height: 1, Hash: "aaaa"}}
	var calls []int
	attempt := func(size int) ([]*Block, error) {
		calls = append(calls, size)
		if size == 500 {
			return nil, errors.New("decoding response body (47 bytes): unexpected end of JSON input")
		}
		return want, nil
	}

	blocks, usedFallback, err := fetchWithSmallerPageFallback("http://peer.example", 0, 500, 25, attempt)
	if err != nil {
		t.Fatalf("expected the fallback to succeed, got error: %v", err)
	}
	if !usedFallback {
		t.Fatalf("expected usedFallback=true (the large page must have failed first)")
	}
	if len(blocks) != 1 || blocks[0].Hash != want[0].Hash {
		t.Fatalf("got %+v, want %+v", blocks, want)
	}
	if len(calls) != 2 || calls[0] != 500 || calls[1] != 25 {
		t.Fatalf("expected attempt(500) then attempt(25), got calls=%v", calls)
	}
}

func TestFetchWithSmallerPageFallback_BothSizesFail_ReturnsWrappedError(t *testing.T) {
	bigErr := errors.New("unexpected end of JSON input")
	smallErr := errors.New("peer returned HTTP 503")
	attempt := func(size int) ([]*Block, error) {
		if size == 500 {
			return nil, bigErr
		}
		return nil, smallErr
	}

	blocks, usedFallback, err := fetchWithSmallerPageFallback("http://peer.example", 0, 500, 25, attempt)
	if err == nil {
		t.Fatal("expected an error when both the full page and the fallback page fail")
	}
	if usedFallback {
		t.Fatal("usedFallback must be false when the fallback attempt itself failed")
	}
	if blocks != nil {
		t.Fatalf("expected nil blocks on total failure, got %+v", blocks)
	}
}

func TestFetchWithSmallerPageFallback_FirstAttemptSucceeds_NoFallbackNeeded(t *testing.T) {
	want := []*Block{{Height: 1, Hash: "aaaa"}, {Height: 2, Hash: "bbbb"}}
	calls := 0
	attempt := func(size int) ([]*Block, error) {
		calls++
		if size != 500 {
			t.Fatalf("expected only the full page size to be requested, got %d", size)
		}
		return want, nil
	}

	blocks, usedFallback, err := fetchWithSmallerPageFallback("http://peer.example", 0, 500, 25, attempt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usedFallback {
		t.Fatal("usedFallback must be false when the first attempt already succeeded")
	}
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 attempt when the first one succeeds, got %d", calls)
	}
}

func TestFetchWithSmallerPageFallback_AlreadySmall_NoRetryAttempted(t *testing.T) {
	origErr := errors.New("peer unreachable")
	calls := 0
	attempt := func(size int) ([]*Block, error) {
		calls++
		return nil, origErr
	}

	_, usedFallback, err := fetchWithSmallerPageFallback("http://peer.example", 0, 25, 25, attempt)
	if !errors.Is(err, origErr) {
		t.Fatalf("expected the original error to propagate unwrapped, got: %v", err)
	}
	if usedFallback {
		t.Fatal("usedFallback must be false when there is no smaller size to fall back to")
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 attempt when pageSize is already at/below the fallback size, got %d", calls)
	}
}
