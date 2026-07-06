package keeper

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// Regression tests for the beta-launch audit (2026-07-05) G5 fix:
// EVMEngine.newStateDB() used to unconditionally load every registered
// human's balance (GetAllAccounts()) on every single CallContract
// invocation, including a read-only balanceOf(address) that only ever
// needs one account. addressLikeWordsInCalldata/GetAccountsForAddresses
// let CallContract load just the handful of accounts a given call could
// plausibly touch instead.

func mustEncodeAddress(t *testing.T, addr string) []byte {
	t.Helper()
	a := common.HexToAddress(addr)
	word := make([]byte, 32)
	copy(word[12:], a.Bytes())
	return word
}

func TestAddressLikeWordsInCalldata_ExtractsBalanceOfArgument(t *testing.T) {
	selector := []byte{0x70, 0xa0, 0x82, 0x31} // balanceOf(address)
	addrWord := mustEncodeAddress(t, "0x1234567890123456789012345678901234567890")
	data := append(append([]byte{}, selector...), addrWord...)

	got := addressLikeWordsInCalldata(data)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 address extracted, got %d: %v", len(got), got)
	}
	want := common.HexToAddress("0x1234567890123456789012345678901234567890")
	if got[0] != want {
		t.Errorf("want %s, got %s", want.Hex(), got[0].Hex())
	}
}

func TestAddressLikeWordsInCalldata_ExtractsBothAllowanceArguments(t *testing.T) {
	selector := []byte{0xdd, 0x62, 0xed, 0x3e} // allowance(address,address)
	addr1 := mustEncodeAddress(t, "0x1111111111111111111111111111111111111111")
	addr2 := mustEncodeAddress(t, "0x2222222222222222222222222222222222222222")
	data := append(append(append([]byte{}, selector...), addr1...), addr2...)

	got := addressLikeWordsInCalldata(data)
	if len(got) != 2 {
		t.Fatalf("expected 2 addresses extracted, got %d: %v", len(got), got)
	}
}

func TestAddressLikeWordsInCalldata_NoArgumentsReturnsNil(t *testing.T) {
	selector := []byte{0x18, 0x16, 0x0d, 0xdd} // totalSupply() — no args
	got := addressLikeWordsInCalldata(selector)
	if len(got) != 0 {
		t.Fatalf("expected no addresses for a no-argument call, got %d: %v", len(got), got)
	}
}

func TestAddressLikeWordsInCalldata_ShortCalldataReturnsNil(t *testing.T) {
	got := addressLikeWordsInCalldata([]byte{0x01, 0x02})
	if len(got) != 0 {
		t.Fatalf("expected no addresses for calldata shorter than a selector, got %d", len(got))
	}
}

func TestAddressLikeWordsInCalldata_NonAddressWordSkipped(t *testing.T) {
	selector := []byte{0x70, 0xa0, 0x82, 0x31}
	// A 32-byte word that is NOT address-shaped (nonzero in the top 12 bytes) —
	// e.g. a raw uint256 amount, not an address. Must not be misidentified.
	nonAddrWord := make([]byte, 32)
	nonAddrWord[0] = 0xff // top byte nonzero — not a valid ABI-encoded address
	data := append(append([]byte{}, selector...), nonAddrWord...)

	got := addressLikeWordsInCalldata(data)
	if len(got) != 0 {
		t.Fatalf("expected the non-address-shaped word to be skipped, got %d: %v", len(got), got)
	}
}

func TestGetAccountsForAddresses_ReturnsRequestedAccountsOnly(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0xaaaa000000000000000000000000000000aaaa", 100)
	addHuman(cs, "0xbbbb000000000000000000000000000000bbbb", 200)
	addHuman(cs, "0xcccc000000000000000000000000000000cccc", 300)

	got := cs.GetAccountsForAddresses([]string{
		"0xaaaa000000000000000000000000000000aaaa",
		"0xcccc000000000000000000000000000000cccc",
	})
	if len(got) != 2 {
		t.Fatalf("expected exactly 2 accounts (not all 3 in cs.accounts), got %d", len(got))
	}
	byAddr := map[string]float64{}
	for _, a := range got {
		byAddr[a.Address] = a.Balance.Float()
	}
	if byAddr["0xaaaa000000000000000000000000000000aaaa"] != 100 {
		t.Errorf("wrong balance for aaaa: %v", byAddr["0xaaaa000000000000000000000000000000aaaa"])
	}
	if byAddr["0xcccc000000000000000000000000000000cccc"] != 300 {
		t.Errorf("wrong balance for cccc: %v", byAddr["0xcccc000000000000000000000000000000cccc"])
	}
	if _, present := byAddr["0xbbbb000000000000000000000000000000bbbb"]; present {
		t.Error("bbbb was not requested and must not appear in the result")
	}
}

func TestGetAccountsForAddresses_UnknownAddressOmittedNotErrored(t *testing.T) {
	cs := newTestState()
	got := cs.GetAccountsForAddresses([]string{"0xdead000000000000000000000000000000dead"})
	if len(got) != 0 {
		t.Fatalf("expected an unknown address to be silently omitted, got %d results", len(got))
	}
}

func TestGetAccountsForAddresses_DeduplicatesRepeatedAddress(t *testing.T) {
	cs := newTestState()
	addHuman(cs, "0xaaaa000000000000000000000000000000aaaa", 100)
	got := cs.GetAccountsForAddresses([]string{
		"0xaaaa000000000000000000000000000000aaaa",
		"0xAAAA000000000000000000000000000000AAAA", // same address, different case
	})
	if len(got) != 1 {
		t.Fatalf("expected the repeated (case-insensitive) address to be deduplicated, got %d results", len(got))
	}
}
