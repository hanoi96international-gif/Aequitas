package keeper

import (
	"crypto/ecdsa"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// signierterTestblock: ein Block mit gueltigem Hash und gueltiger
// Unterschrift von key.
func signierterTestblock(t *testing.T, key *ecdsa.PrivateKey, hoehe, zeit int64, eltern []string, txs []Transaction) *Block {
	t.Helper()
	b := &Block{Height: hoehe, Timestamp: zeit, ParentHashes: eltern,
		Proposer: strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex()), Humans: 3, Transactions: txs}
	b.Hash = calculateBlockHash(b)
	h := common.HexToHash(b.Hash)
	sig, err := crypto.Sign(h[:], key)
	if err != nil {
		t.Fatal(err)
	}
	b.Signature = hex.EncodeToString(sig)
	return b
}

func slashFall(t *testing.T) (*ecdsa.PrivateKey, *Block, *Block) {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	zeit := int64(equivocationSlashingActivationUnix + 86400)
	eltern := []string{"aa", "bb"}
	a := signierterTestblock(t, key, 10, zeit, eltern, []Transaction{{Type: "transfer", Wallet: "0x1", To: "0x2", Amount: 1}})
	// Dieselben Eltern in anderer Reihenfolge, anderer Inhalt: Doppelsignatur.
	b := signierterTestblock(t, key, 10, zeit+1, []string{"bb", "aa"}, []Transaction{{Type: "transfer", Wallet: "0x1", To: "0x3", Amount: 1}})
	return key, a, b
}

func slashTx(signierer string, a, b *Block, beweis *Doppelbeweis) Transaction {
	return Transaction{Type: "slash_equivocation", Wallet: signierer, BlockAHash: a.Hash, BlockBHash: b.Hash,
		DetectedAt: b.Timestamp, Doppelbeweis: beweis}
}

func slashZaehler(t *testing.T, tx Transaction) map[string]int64 {
	t.Helper()
	regeln := []string{"slash_ohne_beweis", "slash_zeit"}
	vorher := map[string]int64{}
	for _, r := range regeln {
		vorher[r] = erhaltungZaehler(r)
	}
	// Ueber den Eingang des Nachspielens, nicht die Regel direkt.
	cs := newTestState()
	cs.mu.Lock()
	err := cs.nachrechnenTxLocked(&tx, tx.DetectedAt+60)
	cs.mu.Unlock()
	if err != nil {
		t.Fatalf("im Beobachtungsmodus darf nichts abgelehnt werden: %v", err)
	}
	neu := map[string]int64{}
	for _, r := range regeln {
		if d := erhaltungZaehler(r) - vorher[r]; d != 0 {
			neu[r] = d
		}
	}
	return neu
}

// Gutfall: echte Doppelsignatur, auch fuer einen Block ohne Rumpf (nur
// TxRoot, wie nach der Ausduennung).
func TestSlashBeweis_EchteDoppelsignatur(t *testing.T) {
	key, a, b := slashFall(t)
	signierer := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	ohneRumpf := *a
	ohneRumpf.TxRoot = kopfVon(a).TxRoot
	ohneRumpf.Transactions = nil
	beweis := &Doppelbeweis{A: kopfVon(&ohneRumpf), B: kopfVon(b)}
	if err := pruefeDoppelbeweis(beweis, signierer, b.Hash, a.Hash); err != nil {
		t.Fatalf("echte Doppelsignatur abgelehnt: %v", err)
	}
	if got := slashZaehler(t, slashTx(signierer, a, b, beweis)); len(got) != 0 {
		t.Fatalf("echte Doppelsignatur gemeldet: %v", got)
	}
}

// Missbrauch: ohne Beweis, erfundene Hashes, fremder Unterzeichner, ein
// Kopf veraendert, verschiedene Eltern, zweimal derselbe Block, gewaehlter
// Zeitpunkt.
func TestSlashBeweis_Missbrauch(t *testing.T) {
	key, a, b := slashFall(t)
	signierer := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	gut := &Doppelbeweis{A: kopfVon(a), B: kopfVon(b)}

	if got := slashZaehler(t, slashTx(signierer, a, b, nil)); got["slash_ohne_beweis"] != 1 {
		t.Fatalf("ohne Beweis nicht erkannt: %v", got)
	}

	erfunden := slashTx(signierer, a, b, gut)
	erfunden.BlockAHash, erfunden.BlockBHash = "11", "22"
	if got := slashZaehler(t, erfunden); got["slash_ohne_beweis"] != 1 {
		t.Fatalf("erfundene Hashes nicht erkannt: %v", got)
	}

	anderer, _ := crypto.GenerateKey()
	fremd := strings.ToLower(crypto.PubkeyToAddress(anderer.PublicKey).Hex())
	if got := slashZaehler(t, slashTx(fremd, a, b, gut)); got["slash_ohne_beweis"] != 1 {
		t.Fatalf("fremder Beschuldigter nicht erkannt: %v", got)
	}

	veraendert := &Doppelbeweis{A: kopfVon(a), B: kopfVon(b)}
	veraendert.B.Height = 11 // Hash passt nicht mehr
	if got := slashZaehler(t, slashTx(signierer, a, b, veraendert)); got["slash_ohne_beweis"] != 1 {
		t.Fatalf("veraenderter Kopf nicht erkannt: %v", got)
	}

	falscheUnterschrift := &Doppelbeweis{A: kopfVon(a), B: kopfVon(b)}
	vonAnderem := signierterTestblock(t, anderer, 10, b.Timestamp, b.ParentHashes, b.Transactions)
	falscheUnterschrift.B.Signature = vonAnderem.Signature
	if got := slashZaehler(t, slashTx(signierer, a, b, falscheUnterschrift)); got["slash_ohne_beweis"] != 1 {
		t.Fatalf("Unterschrift eines anderen nicht erkannt: %v", got)
	}

	andereEltern := signierterTestblock(t, key, 10, b.Timestamp, []string{"cc"}, b.Transactions)
	if got := slashZaehler(t, slashTx(signierer, a, andereEltern, &Doppelbeweis{A: kopfVon(a), B: kopfVon(andereEltern)})); got["slash_ohne_beweis"] != 1 {
		t.Fatalf("verschiedene Eltern als Doppelsignatur gewertet: %v", got)
	}

	if got := slashZaehler(t, slashTx(signierer, a, a, &Doppelbeweis{A: kopfVon(a), B: kopfVon(a)})); got["slash_ohne_beweis"] != 1 {
		t.Fatalf("zweimal derselbe Block als Doppelsignatur gewertet: %v", got)
	}

	gewaehlt := slashTx(signierer, a, b, gut)
	gewaehlt.DetectedAt = b.Timestamp + 30*86400 // spaeter: Sperre und Fenster verschoben
	if got := slashZaehler(t, gewaehlt); got["slash_zeit"] != 1 {
		t.Fatalf("gewaehlter Zeitpunkt nicht erkannt: %v", got)
	}
}

// Im strengen Modus wird ein Block mit einer unbelegten Strafe abgelehnt,
// bevor jemand gesperrt wird.
func TestSlashBeweis_StrengLehntAb(t *testing.T) {
	_, a, b := slashFall(t)
	nachrechnenStrengOverride.Store(1)
	t.Cleanup(func() { nachrechnenStrengOverride.Store(0) })
	tx := slashTx("0x00000000000000000000000000000000000000aa", a, b, nil)
	cs := newTestState()
	cs.mu.Lock()
	err := cs.nachrechnenTxLocked(&tx, tx.DetectedAt+60)
	cs.mu.Unlock()
	if err == nil {
		t.Fatal("Strafe ohne Beweis im strengen Modus angenommen")
	}
}
