package keeper

import (
	"encoding/hex"
	"strings"
	"testing"
)

// One biometric must be one registration. Before canonicalNullifier, nine
// different spellings of the SAME nullifier each claimed their own — measured,
// through the public /api/register path, each one granting another 1,000 AEQ.
// The V7 contract keys usedNullifiers by the NUMERIC value while Go and
// Postgres keyed it by the raw string, and nothing normalised in between.
func TestCanonicalNullifier_AllSpellingsOfOneValueCollapse(t *testing.T) {
	const decimal = "12857392037461937299283746192837461928374619283746192837461928374"
	canon, err := canonicalNullifier(decimal)
	if err != nil {
		t.Fatalf("canonical form of the honest value: %v", err)
	}

	variants := map[string]string{
		"as sent":          decimal,
		"one leading zero": "0" + decimal,
		"two leading zeros": "00" + decimal,
		"leading space":    " " + decimal,
		"trailing space":   decimal + " ",
		"trailing newline": decimal + "\n",
		"trailing tab":     decimal + "\t",
	}
	for name, v := range variants {
		got, err := canonicalNullifier(v)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got != canon {
			t.Errorf("%s: canonical form %q != %q — this spelling would claim a second registration", name, got, canon)
		}
	}
}

// Hex spellings must collapse onto each other too. (A hex string and a decimal
// string of the same digits are different VALUES, and correctly stay distinct —
// what must not differ is the same value written two ways.)
func TestCanonicalNullifier_HexSpellingsCollapse(t *testing.T) {
	base := "1f4a9c3e5b7d"
	forms := []string{"0x" + base, "0X" + base, base, "0x" + strings.ToUpper(base), "  0x" + base + "  "}
	var first string
	for i, f := range forms {
		got, err := canonicalNullifier(f)
		if err != nil {
			t.Fatalf("%q: %v", f, err)
		}
		if i == 0 {
			first = got
			continue
		}
		if got != first {
			t.Errorf("%q -> %q, want %q", f, got, first)
		}
	}
}

// A value we cannot parse must never become a fresh key — that is precisely how
// the padded variants got in.
func TestCanonicalNullifier_RejectsUnusableValues(t *testing.T) {
	for _, bad := range []string{"", "   ", "0x", "not-a-number", "-5", "0", "0x0", "12 34"} {
		if got, err := canonicalNullifier(bad); err == nil {
			t.Errorf("%q was accepted as %q — it must be rejected", bad, got)
		}
	}
}

// IsNullifierUsed must fail CLOSED on something it cannot parse: reporting
// "free" for an unreadable value hands out a registration slot.
func TestIsNullifierUsed_UnparseableCountsAsUsed(t *testing.T) {
	cs := newTestState()
	if !cs.IsNullifierUsed("not-a-number") {
		t.Error("an unparseable nullifier must read as USED, not as free")
	}
}

// The end-to-end shape of the original finding, at the state layer that grants
// the money: claim the honest value, then try every padded spelling.
func TestNullifierClaim_PaddedSpellingCannotClaimASecondTime(t *testing.T) {
	cs := newTestState()
	cs.nullifiers = map[string]string{}
	const n = "12857392037461937299283746192837461928374619283746192837461928374"

	ok, err := cs.tryClaimNullifierLocked(t.Context(), n, "0xfirst")
	if err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v", ok, err)
	}
	for _, variant := range []string{"0" + n, "00" + n, " " + n, n + "\n", n + "\t"} {
		ok, err := cs.tryClaimNullifierLocked(t.Context(), variant, "0xsecond")
		if err != nil {
			continue // refused outright is also correct
		}
		if ok {
			t.Errorf("%q claimed a SECOND registration for the same biometric", variant)
		}
	}
}

// The no-DB branch of SaveNullifier had no owner check at all, while the
// Postgres branch fails closed on exactly this case. A DB-less node is a
// supported configuration, and there the identical nullifier registered a
// second wallet.
func TestSaveNullifier_NoDBRejectsASecondOwner(t *testing.T) {
	cs := newTestState()
	cs.nullifiers = map[string]string{}
	const n = "999888777666555444333222111"
	if err := cs.SaveNullifier(t.Context(), n, "0xowner"); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := cs.SaveNullifier(t.Context(), n, "0xowner"); err != nil {
		t.Errorf("re-saving for the SAME wallet must be idempotent, got %v", err)
	}
	if err := cs.SaveNullifier(t.Context(), n, "0xattacker"); err == nil {
		t.Error("a second wallet claiming the same nullifier must be refused")
	}
}

// MigrateEVMFromGoState used common.HexToHash on a DECIMAL nullifier. Every
// decimal digit is a valid hex digit, so the parse quietly succeeded at a
// different number and the rebuilt mapping landed at a slot AequitasV7 never
// reads — after every snapshot import, resync and contract upgrade.
func TestNullifierBytes32_MatchesTheContractsNumericKey(t *testing.T) {
	const decimal = "500"
	got, err := nullifierBytes32(decimal)
	if err != nil {
		t.Fatalf("nullifierBytes32: %v", err)
	}
	// 500 == 0x1f4, right-aligned in 32 bytes.
	want := strings.Repeat("00", 30) + "01f4"
	if hex.EncodeToString(got[:]) != want {
		t.Errorf("bytes32(%s) = %s, want %s", decimal, hex.EncodeToString(got[:]), want)
	}
	// The old behaviour parsed "500" as hex 0x500 — a different slot entirely.
	if hex.EncodeToString(got[:]) == strings.Repeat("00", 30)+"0500" {
		t.Error("still parsing the decimal nullifier as hex")
	}
}

// The nullifier in a block must be the one the ZK proof commits to. The replay
// path verified the proof and separately claimed tx.Nullifier without ever
// checking they described the same thing — and the proof is published in the
// block and is not wallet-bound.
func TestNullifierMatchesProof(t *testing.T) {
	const n = "17579322874185"
	ok, err := nullifierMatchesProof(n, []string{"commitment", n})
	if err != nil || !ok {
		t.Fatalf("honest nullifier must match its proof: ok=%v err=%v", ok, err)
	}
	ok, err = nullifierMatchesProof("0"+n, []string{"commitment", n})
	if err != nil || !ok {
		t.Errorf("a padded spelling of the same value still matches: ok=%v err=%v", ok, err)
	}
	ok, _ = nullifierMatchesProof("99999999999", []string{"commitment", n})
	if ok {
		t.Error("a nullifier that is not the proof's must NOT match")
	}
}
