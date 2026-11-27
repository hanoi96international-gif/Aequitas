package keeper

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// qsMerkNetz: nimmt die eingereichten Angaben entgegen und meldet "gebunden".
type qsMerkNetz struct{ q QuantenWAngaben }

func (n *qsMerkNetz) bestaetigen(context.Context, QuantenWBestaetigenAnfrage) (string, error) {
	return "", nil
}
func (n *qsMerkNetz) einreichen(_ context.Context, _, _ string, _ int64, q QuantenWAngaben) error {
	n.q = q
	return nil
}
func (n *qsMerkNetz) stand(context.Context, string) (map[string]interface{}, error) {
	return map[string]interface{}{"gebunden": true}, nil
}
func (n *qsMerkNetz) warten(context.Context) error { return nil }

// Der Pfandgeber unterschreibt genau, was die Kette prueft
// (quantenSIPfandBindenLocked): sein Konto, K, Inhaberin, Deckung + 10 %.
func TestQuantenSPfandgeber_UnterschriftWieDieKette(t *testing.T) {
	inhaberKey, _ := crypto.GenerateKey()
	geberKey, _ := crypto.GenerateKey()
	inhaber, geber := quantenSAdresse(inhaberKey), quantenSAdresse(geberKey)
	K := strings.Repeat("ab", 32)
	n := &qsMerkNetz{}
	pg := &quantenSPfandgeber{key: geberKey, deckung: 2_000_000}
	if err := quantenSBinden(context.Background(), n, time.Second, K, inhaber, inhaberKey, []int{0, 1, 1, 0}, pg); err != nil {
		t.Fatal(err)
	}
	if n.q.PfandVon != geber || !kanonischeSignatur(n.q.PfandSig) {
		t.Fatalf("Pfandgeber %q, Unterschrift %q", n.q.PfandVon, n.q.PfandSig)
	}
	if err := pruefePersonalSignGemerkt(quantenSIPfandNachricht(geber, K, inhaber, quantenSIPfandFuer(2_000_000)), n.q.PfandSig, geber); err != nil {
		t.Fatalf("die Kette naehme die Unterschrift nicht an: %v", err)
	}
	// Missbrauch: dieselbe Unterschrift taugt fuer keinen anderen Betrag, Token
	// oder keine andere Inhaberin.
	for _, msg := range []string{
		quantenSIPfandNachricht(geber, K, inhaber, quantenSIPfandFuer(2_000_000)-1),
		quantenSIPfandNachricht(geber, strings.Repeat("cd", 32), inhaber, quantenSIPfandFuer(2_000_000)),
		quantenSIPfandNachricht(geber, K, geber, quantenSIPfandFuer(2_000_000)),
	} {
		if pruefePersonalSignGemerkt(msg, n.q.PfandSig, geber) == nil {
			t.Fatalf("Unterschrift passt auch auf %q", msg)
		}
	}
	// Ohne Pfandgeber: keine Felder (vor dem Stichtag weist die Kette sie ab).
	if err := quantenSBinden(context.Background(), n, time.Second, K, inhaber, inhaberKey, []int{0, 1}, nil); err != nil || n.q.PfandVon != "" || n.q.PfandSig != "" {
		t.Fatalf("ohne Pfandgeber: %+v %v", n.q, err)
	}
}

func TestQuantenSPfandgeber_Missbrauch(t *testing.T) {
	inhaberKey, _ := crypto.GenerateKey()
	inhaber := quantenSAdresse(inhaberKey)
	K := strings.Repeat("ab", 32)
	n := &qsMerkNetz{}
	// Die Inhaberin als ihr eigener Pfandgeber: die Kette wiese es ab, das
	// Werkzeug reicht es gar nicht erst ein.
	if err := quantenSBinden(context.Background(), n, time.Second, K, inhaber, inhaberKey, []int{1}, &quantenSPfandgeber{key: inhaberKey, deckung: 1}); err == nil {
		t.Fatal("Inhaberin als eigener Pfandgeber eingereicht")
	}
	geberKey, _ := crypto.GenerateKey()
	if err := quantenSBinden(context.Background(), n, time.Second, K, inhaber, inhaberKey, []int{1}, &quantenSPfandgeber{key: geberKey, deckung: 0}); err == nil {
		t.Fatal("Pfand ohne Deckung eingereicht")
	}
}

// Vor dem Stichtag der Pfaender gibt es keinen Pfandgeber, danach schon.
func TestQuantenSPfandgeber_Stichtag(t *testing.T) {
	t.Cleanup(func() { quantenSPfaenderAbOverride.Store(0) })
	key, _ := crypto.GenerateKey()
	ab := quantenSLosBehaltenAb() + 1000
	quantenSPfaenderAbOverride.Store(ab)
	if quantenSPfandgeberZu(key, 5, ab-1) != nil {
		t.Fatal("Pfandgeber vor dem Stichtag")
	}
	if pg := quantenSPfandgeberZu(key, 5, ab); pg == nil || pg.deckung != 5 {
		t.Fatal("kein Pfandgeber ab dem Stichtag")
	}
	if quantenSPfandgeberZu(nil, 5, ab) != nil {
		t.Fatal("Pfandgeber ohne Schluessel")
	}
	quantenSPfaenderAbOverride.Store(math.MaxInt64)
	if quantenSPfandgeberZu(key, 5, ab) != nil {
		t.Fatal("Pfandgeber bei schlafenden Pfaendern")
	}
}

func TestQuantenSDeckungAus(t *testing.T) {
	for _, f := range []struct {
		v    interface{}
		soll int64
	}{
		{float64(2_000_000), 2_000_000}, {float64(1), 1}, {float64(1e15), 1e15},
		{float64(0), -1}, {float64(-5), -1}, {1.5, -1}, {float64(1e15 + 1), -1}, {"2000000", -1}, {nil, -1},
	} {
		if got := quantenSDeckungAus(map[string]interface{}{"deckung": f.v}); got != f.soll {
			t.Fatalf("%v: %d, soll %d", f.v, got, f.soll)
		}
	}
}

// Annahme und Nachspielen sperren bzw. laden den Pfandgeber einer Bindung
// mit. Ohne ihn wies die Annahme jede Bindung mit Pfandgeber als
// "wiederholen" ab (Befund 27.11.2026, TestVierKnotenQuantenSPfand_RealDB).
func TestQuantenWKonten_Pfandgeber(t *testing.T) {
	geber := "0x" + strings.Repeat("1a", 20)
	tx := Transaction{Type: quantenWSBindenTxArt, Wallet: "0x" + strings.Repeat("2b", 20),
		QuantenW: &QuantenWAngaben{K: strings.Repeat("ab", 32), PfandVon: strings.ToUpper(geber[:2]) + geber[2:]}}
	k := quantenWKonten(&tx)
	da := false
	for _, a := range k {
		da = da || a == geber
	}
	if !da {
		t.Fatalf("Pfandgeber fehlt in %v", k)
	}
	tx.QuantenW.PfandVon = ""
	for _, a := range quantenWKonten(&tx) {
		if a == "" {
			t.Fatal("leeres Konto in der Liste")
		}
	}
}
