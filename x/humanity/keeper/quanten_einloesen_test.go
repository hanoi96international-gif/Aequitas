package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strings"
	"testing"
)

// Test-Pruefer statt eines echten Einmal-Unterschriftsverfahrens: gilt, wenn
// sig = SHA-256(serie | nachricht). Nur fuer Tests -- er beweist nicht, dass
// jemand die Note hat, sondern nur, dass die Kette alles DRUMHERUM richtig
// prueft. Die Produktionsliste bleibt leer (TestQuantenEinloesen_HeuteUnmoeglich).
func qgTestEinmal(serie, nachricht []byte) string {
	h := sha256.Sum256(append(append([]byte{}, serie...), nachricht...))
	return hex.EncodeToString(h[:])
}

func qgTestPrueferSetzen(t *testing.T, satz string) {
	t.Helper()
	quantenEinmalPruefer[satz] = func(serie, nachricht, sig []byte) bool {
		return qgTestEinmal(serie, nachricht) == hex.EncodeToString(sig)
	}
	t.Cleanup(func() { delete(quantenEinmalPruefer, satz) })
}

// qgEinloeseFall: eine angemeldete Note (100 AEQ) zum Einloesen.
type qgEinloeseFall struct {
	*qgPraegeFall
	k, serieHex string
	serie       []byte
}

func neuerQgEinloeseFall(t *testing.T) *qgEinloeseFall {
	t.Helper()
	f := &qgEinloeseFall{qgPraegeFall: neuerQgPraegeFall(t), serieHex: "04d2", serie: []byte{0x04, 0xd2}}
	f.k = quantenSerieSchluessel("isogenie-8192", f.wallet, f.serie)
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	qgTopfGutschreiben(f.cs, 100)
	if _, err := f.cs.quantenNoteEintragenLocked(context.Background(),
		quantenNoteEintrag{K: f.k, Satz: "isogenie-8192", Von: f.wallet}, 0); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *qgEinloeseFall) tx(empf, einmal string) Transaction {
	return Transaction{Type: quantenEinloesenTxArt, Wallet: empf, TxHash: quantenEinloesenTxHash(f.k, empf),
		Quanten: &QuantenAngaben{Satz: "isogenie-8192", Serie: f.serieHex, K: f.k, Einmal: einmal}}
}

func (f *qgEinloeseFall) einmal(empf string) string {
	return qgTestEinmal(f.serie, []byte(quantenEinloesenNachricht(f.k, empf)))
}

func (f *qgEinloeseFall) einloesen(tx Transaction, erwartet int64) (quantenEinloesung, error) {
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	return f.cs.applyQuantenEinloesenLocked(context.Background(), &tx, qgAb+200, erwartet)
}

// Heute gibt es fuer keinen Satz eine Pruefung einer Einmal-Unterschrift:
// jede Einloesung wird abgewiesen, auch mit einer "passenden" Unterschrift.
func TestQuantenEinloesen_HeuteUnmoeglich(t *testing.T) {
	if len(quantenEinmalPruefer) != 0 {
		t.Fatalf("Produktionsliste der Einmal-Pruefer nicht leer: %d -- ein Pruefer kommt nur mit eigener Sicherheitspruefung", len(quantenEinmalPruefer))
	}
	f := neuerQgEinloeseFall(t)
	empf := "0x" + strings.Repeat("e", 40)
	if _, err := f.einloesen(f.tx(empf, f.einmal(empf)), 0); err == nil || !strings.Contains(err.Error(), "quanten_einloesen_unmoeglich") {
		t.Fatalf("Einloesung ohne Pruefer: %v", err)
	}
	if len(f.cs.quantenGeldJetzt().Noten) != 1 || f.cs.GetBalance(empf) != 0 {
		t.Fatal("abgewiesene Einloesung hat etwas veraendert")
	}
}

// Gutfall: der Empfaenger bekommt den Wert der Note NACH der Umlaufabgabe,
// die Note ist danach weg, ein zweites Einloesen findet sie nicht.
func TestQuantenEinloesen_ZahltWertNachAbgabeEinmal(t *testing.T) {
	qgTestPrueferSetzen(t, "isogenie-8192")
	f := neuerQgEinloeseFall(t)
	f.cs.mu.Lock()
	topf, _ := f.cs.accounts.Get(quantenTopfAddr)
	topf.Balance = topf.Balance.Sub(NewDecimal(7.5)) // Abgabe seit der Anmeldung
	f.cs.mu.Unlock()

	empf := "0x" + strings.Repeat("e", 40)
	u, err := f.einloesen(f.tx(empf, f.einmal(empf)), 0)
	if err != nil {
		t.Fatal(err)
	}
	if u.wertMikro != 92_500_000 || math.Abs(f.cs.GetBalance(empf)-92.5) > 1e-9 || f.cs.GetBalance(quantenTopfAddr) != 0 {
		t.Fatalf("ausgezahlt %d, Empfaenger %.6f, Topf %.6f", u.wertMikro, f.cs.GetBalance(empf), f.cs.GetBalance(quantenTopfAddr))
	}
	if len(f.cs.quantenGeldJetzt().Noten) != 0 {
		t.Fatal("Note nach dem Einloesen noch angemeldet")
	}
	if _, err := f.einloesen(f.tx(empf, f.einmal(empf)), 0); err == nil || !strings.Contains(err.Error(), "quanten_unbekannt") {
		t.Fatalf("zweites Einloesen: %v", err)
	}
}

// Missbrauch: nichts davon zahlt aus, nichts veraendert den Zustand.
func TestQuantenEinloesen_Angriffe(t *testing.T) {
	qgTestPrueferSetzen(t, "isogenie-8192")
	f := neuerQgEinloeseFall(t)
	empf := "0x" + strings.Repeat("e", 40)
	dieb := "0x" + strings.Repeat("d", 40)
	gut := f.tx(empf, f.einmal(empf))
	faelle := map[string]struct {
		tx       Transaction
		erwartet int64
		regel    string
	}{
		"Unterschrift fuer anderen Empfaenger": {f.tx(dieb, f.einmal(empf)), 0, "quanten_unterschrift"},
		"falsche Unterschrift":                 {f.tx(empf, strings.Repeat("ab", 32)), 0, "quanten_unterschrift"},
		"ohne Unterschrift":                    {f.tx(empf, ""), 0, "quanten_form"},
		"Seriennummer gehoert nicht zu K": {func() Transaction {
			x := f.tx(empf, f.einmal(empf))
			x.Quanten.Serie = "04d3"
			return x
		}(), 0, "quanten_form"},
		"unbekannte Note": {func() Transaction {
			x := f.tx(empf, f.einmal(empf))
			x.Quanten.K = strings.Repeat("9", 64)
			return x
		}(), 0, "quanten_unbekannt"},
		"an eine Systemadresse":       {f.tx(ubiPoolAddr, qgTestEinmal(f.serie, []byte(quantenEinloesenNachricht(f.k, ubiPoolAddr)))), 0, "quanten_form"},
		"Block zahlt zu viel aus":     {gut, 100_000_001, "Block zahlt"},
		"Block nennt keinen Betrag":   {gut, -1, "Block zahlt"},
		"Block nennt falsche Anteile": {func() Transaction { x := gut; q := *x.Quanten; q.Anteile = 5; x.Quanten = &q; return x }(), 0, "Anteile"},
	}
	for name, fall := range faelle {
		if _, err := f.einloesen(fall.tx, fall.erwartet); err == nil || !strings.Contains(err.Error(), fall.regel) {
			t.Errorf("%s: %v (erwartet %s)", name, err, fall.regel)
		}
	}
	if len(f.cs.quantenGeldJetzt().Noten) != 1 || f.cs.GetBalance(empf) != 0 || f.cs.GetBalance(dieb) != 0 || math.Abs(f.cs.GetBalance(quantenTopfAddr)-100) > 1e-9 {
		t.Fatal("ein abgewiesener Versuch hat den Zustand veraendert")
	}
	// Danach geht der ehrliche Weg noch -- mit genau dem Betrag aus dem Block.
	if _, err := f.einloesen(gut, 100_000_000); err != nil {
		t.Fatalf("ehrliche Einloesung nach den Angriffen: %v", err)
	}
}
