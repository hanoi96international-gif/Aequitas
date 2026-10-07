package keeper

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
)

// Alle drei Saetze tragen die Chain-ID.
func TestCoordinatorNachrichten_TragenChainID(t *testing.T) {
	for _, n := range []string{coordinatorBesitzNachricht("0xabc"), coordinatorFreigabeNachricht("ab"), erneuerungsNachricht("0xdef", 1)} {
		if !strings.Contains(n, "chain:1926") {
			t.Fatalf("ohne Chain-ID: %q", n)
		}
	}
	if erneuerungsNachricht("0xdef", 7) != "aequitas-liveness-renewal-v2|chain:1926|0xdef|7" {
		t.Fatalf("Erneuerung: %q", erneuerungsNachricht("0xdef", 7))
	}
}

// Missbrauch: eine Bescheinigung oder Bindung ohne Chain-ID (v1) oder mit
// fremder Chain-ID -- etwa aus einem Testnetz mit derselben Domaene -- gilt im
// Konsens nicht.
func TestCoordinatorNachrichten_ImKonsensNurV2(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)
	mk, mensch := neuerSchluessel(t)
	wallet := "0x" + strings.Repeat("34", 20)
	issued := int64(1_800_000_000)
	gut := func(m string) (bool, bool) { return m == mensch, false }
	zugelassen := func(m string, t int64) (bool, error) { return m == mensch && t == issued, nil }
	registerLeserOverride.Store(1)
	t.Cleanup(func() { registerLeserOverride.Store(0) })
	signiere := func(msg string) string { return hex.EncodeToString(ed25519.Sign(priv, []byte(msg))) }
	fremdeKette := strings.Replace

	v2 := CoordinatorBindung{Mensch: mensch,
		MenschSig:     personalSign(t, mk, coordinatorFreigabeNachricht(pubHex)),
		SchluesselSig: signiere(coordinatorBesitzNachricht(mensch))}
	pruefe := func(bindung CoordinatorBindung, bescheinigung string) error {
		tx := erneuerungsTransaktion(wallet, issued, pubHex, bescheinigung, bindung)
		return bescheinigungPruefen(wallet, issued, tx.Bescheinigung, gut, zugelassen)
	}
	if err := pruefe(v2, signiere(erneuerungsNachricht(wallet, issued))); err != nil {
		t.Fatalf("v2 abgewiesen: %v", err)
	}
	for name, f := range map[string]struct {
		bindung       CoordinatorBindung
		bescheinigung string
		grund         string
	}{
		"Bescheinigung v1": {v2, signiere("aequitas-liveness-renewal-v1|" + wallet + "|1800000000"), "Bescheinigung passt nicht"},
		"Bescheinigung fremde Kette": {v2,
			signiere(fremdeKette(erneuerungsNachricht(wallet, issued), "chain:1926", "chain:1", 1)), "Bescheinigung passt nicht"},
		"Besitznachweis v1": {CoordinatorBindung{Mensch: mensch, MenschSig: v2.MenschSig,
			SchluesselSig: signiere(coordinatorBesitzNachrichtV1(mensch))}, signiere(erneuerungsNachricht(wallet, issued)), "Besitznachweis"},
		"Freigabe v1": {CoordinatorBindung{Mensch: mensch, MenschSig: personalSign(t, mk, coordinatorFreigabeNachrichtV1(pubHex)),
			SchluesselSig: v2.SchluesselSig}, signiere(erneuerungsNachricht(wallet, issued)), "Freigabe"},
		"Freigabe fremde Kette": {CoordinatorBindung{Mensch: mensch,
			MenschSig:     personalSign(t, mk, fremdeKette(coordinatorFreigabeNachricht(pubHex), "chain:1926", "chain:1", 1)),
			SchluesselSig: v2.SchluesselSig}, signiere(erneuerungsNachricht(wallet, issued)), "Freigabe"},
	} {
		if err := pruefe(f.bindung, f.bescheinigung); err == nil || !strings.Contains(err.Error(), f.grund) {
			t.Fatalf("%s: %v (erwartet %q)", name, err, f.grund)
		}
	}
}

// Die Eintragung nimmt im Uebergang beide Besitznachweise, den Konsens
// erreicht nur v2.
func TestCoordinatorNachrichten_EintragungImUebergang(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)
	mensch := "0x" + strings.Repeat("56", 20)
	v1 := hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachrichtV1(mensch))))
	v2 := hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachricht(mensch))))
	if !verifyCoordinatorPossessionEintragung(pubHex, v1, mensch) || !verifyCoordinatorPossessionEintragung(pubHex, v2, mensch) {
		t.Fatal("Eintragung nimmt nicht beide")
	}
	if verifyCoordinatorPossession(pubHex, v1, mensch) || !verifyCoordinatorPossession(pubHex, v2, mensch) {
		t.Fatal("im Konsens nicht nur v2")
	}
	fremd := hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachrichtV1("0x"+strings.Repeat("78", 20)))))
	if verifyCoordinatorPossessionEintragung(pubHex, fremd, mensch) {
		t.Fatal("Besitznachweis fuer einen anderen Menschen angenommen")
	}
}
