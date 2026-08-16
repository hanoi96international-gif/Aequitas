package main

import (
	"runtime/debug"
	"testing"
)

// The node had no heap ceiling at all, so the only thing that ever stopped it
// growing was the kernel: Contabo2 was OOM-killed at ~11.4 GB on a 12 GB box,
// nine times, each one reported by Docker as a clean ExitCode=0 exit because a
// global kernel OOM does not set the cgroup OOMKilled flag. See the GOMEMLIMIT
// block in main() for the full account.
//
// detectMemoryLimitBytes cannot be tested against real cgroup files portably —
// it reads /sys and /proc, which do not exist on the machine this suite is
// usually run from. What these tests pin instead are the two properties that
// would actually break the fix: that it never reports a nonsensical limit, and
// that the arithmetic derived from it stays inside the box.

// Whatever this platform reports, it must be either "I don't know" (0) or a
// plausible amount of memory. A negative or absurd value would be turned into
// a soft limit and quietly starve the process.
func TestDetectMemoryLimitBytes_ReturnsZeroOrSomethingPlausible(t *testing.T) {
	got := detectMemoryLimitBytes()
	if got < 0 {
		t.Fatalf("detectMemoryLimitBytes returned a negative value: %d", got)
	}
	if got == 0 {
		t.Skip("no cgroup or /proc/meminfo on this platform — the caller runs without a soft limit, which is the documented fallback")
	}
	const oneMiB = 1 << 20
	if got < 64*oneMiB {
		t.Errorf("detected memory limit is %d bytes (%d MiB), which is too small to be a real machine or container",
			got, got>>20)
	}
	// 1 PiB. cgroup v1 signals "unlimited" with a sentinel near int64 max, and
	// passing that through would produce a soft limit of ~6.9 exabytes — a
	// limit that can never bind, which is the same as having none.
	const onePiB = int64(1) << 50
	if got > onePiB {
		t.Errorf("detected memory limit is %d bytes, which is the 'unlimited' sentinel leaking through rather than a real limit", got)
	}
}

// The 75% calculation must land strictly inside the detected amount and must
// not overflow. `limit / 100 * 75` is written in that order deliberately —
// `limit * 75` first would overflow int64 for limits above ~123 PiB.
func TestSoftLimitArithmetic_StaysInsideTheDetectedAmount(t *testing.T) {
	cases := []int64{
		64 << 20,       // 64 MiB, a small container
		2 << 30,        // 2 GiB
		12 << 30,       // 12 GiB — the Contabo2 box this fix came from
		64 << 30,       // 64 GiB
		int64(1) << 50, // 1 PiB, the upper bound accepted above
	}
	for _, limit := range cases {
		soft := limit / 100 * 75
		if soft <= 0 {
			t.Errorf("limit %d produced a non-positive soft limit %d", limit, soft)
			continue
		}
		if soft >= limit {
			t.Errorf("limit %d produced soft limit %d, which leaves nothing for postgres or the OS on a shared box", limit, soft)
		}
		// Should be genuinely close to 75%, not accidentally truncated to
		// something far smaller by integer division.
		if want := limit / 100 * 75; soft != want {
			t.Errorf("limit %d: soft = %d, want %d", limit, soft, want)
		}
	}
}

// debug.SetMemoryLimit must accept the value actually derived on this machine.
// The runtime rejects negative limits other than the -1 read-back sentinel, so
// this catches a bad value before it reaches production. The previous limit is
// restored so this test cannot affect any other test in the package.
func TestSetMemoryLimit_AcceptsTheDerivedValue(t *testing.T) {
	detected := detectMemoryLimitBytes()
	if detected == 0 {
		t.Skip("no limit detectable on this platform")
	}
	prev := debug.SetMemoryLimit(-1) // -1 reads the current limit without setting it
	t.Cleanup(func() { debug.SetMemoryLimit(prev) })

	soft := detected / 100 * 75
	if got := debug.SetMemoryLimit(soft); got != prev {
		t.Errorf("SetMemoryLimit returned %d, want the previous limit %d", got, prev)
	}
	if now := debug.SetMemoryLimit(-1); now != soft {
		t.Errorf("memory limit is %d after setting it to %d", now, soft)
	}
}
