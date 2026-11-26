package keeper

import (
	"sort"
	"strings"
	"testing"
)

// Der Betreiber allein erschafft keine Menschen (bio_bescheinigung_zwei_menschen.go).

const zmGruender = "0x0000000000000000000000000000000000000abc"

// zmRegister: Gruender plus die Eintraege (Mensch, seit, bis), sortiert.
func zmRegister(e ...verifierEintrag) *verifierRegister {
	sort.Slice(e, func(i, j int) bool { return e[i].Mensch < e[j].Mensch })
	return &verifierRegister{Eintraege: e}
}

// zmFall: Genesis-Schluessel des Betreibers (Coordinator + Vergleichsdienst),
// Quorum 1, Pruefer-Schluessel erlaubt, Regel ab ab (0 = Stichtag im Code).
func zmFall(t *testing.T, ab int64) *b1bFall {
	t.Helper()
	f := neuerB1bFall(t, 1)
	verifierGruender = []string{zmGruender}
	vorher, vorherReg := bescheinigungZweiMenschenOverride.Load(), verifierRegisterOverride.Load()
	bescheinigungZweiMenschenOverride.Store(ab)
	verifierRegisterOverride.Store(bTestZeit - 10_000)
	t.Cleanup(func() {
		bescheinigungZweiMenschenOverride.Store(vorher)
		verifierRegisterOverride.Store(vorherReg)
	})
	return f
}

// nurBetreiber: eine Registrierung, die nur die Schluessel der Genesis tragen.
func nurBetreiber(t *testing.T, f *b1bFall) Transaction {
	return bTestTx(t, bTestNullifier,
		bindung(f.coord, "test", bTestNullifier, bTestWallet, bTestZeit),
		aussage(f.zeuge, "test", bTestNullifier, bTestZeit))
}

// mitZweitem: Betreiber bindet, der zweite Pruefer bezeugt mit seiner Bindung.
func mitZweitem(t *testing.T, f *b1bFall, p b1bPruefer) Transaction {
	return mitBindungen(bTestTx(t, bTestNullifier,
		bindung(f.coord, "test", bTestNullifier, bTestWallet, bTestZeit),
		aussage(p.k, "test", bTestNullifier, bTestZeit)), p.bindung(t))
}

// Missbrauch: Gibt es einen zweiten Pruefer, weist jeder Knoten eine
// Registrierung ab, die nur der Betreiber bescheinigt hat. Mit der Unterschrift
// des zweiten Pruefers geht sie durch.
func TestZweiMenschen_BetreiberAlleinAbgewiesen(t *testing.T) {
	f := zmFall(t, bTestZeit-100)
	p := neuerB1bPruefer(t)
	reg := zmRegister(verifierEintrag{Mensch: p.mensch, Seit: bTestZeit - 500})
	err := pruefeBioBescheinigungMit(nurBetreiber(t, f), bTestZeit+60, reg)
	if err == nil || !strings.Contains(err.Error(), "zweiten Pruefer") {
		t.Fatalf("Betreiber allein bescheinigte trotz zweitem Pruefer: %v", err)
	}
	if err := pruefeBioBescheinigungMit(mitZweitem(t, f, p), bTestZeit+60, reg); err != nil {
		t.Fatalf("Betreiber + zweiter Pruefer abgewiesen: %v", err)
	}
	// Ein Fremder (nicht im Register) zaehlt nicht als zweiter Mensch.
	fremd := neuerB1bPruefer(t)
	if err := pruefeBioBescheinigungMit(mitZweitem(t, f, fremd), bTestZeit+60, reg); err == nil {
		t.Fatal("ein Fremder ersetzte den zweiten Pruefer")
	}
}

// Solange nur der Gruender Pruefer ist, bleibt alles wie bisher -- sonst
// stuende jede Registrierung still.
func TestZweiMenschen_EinPrueferAendertNichts(t *testing.T) {
	f := zmFall(t, bTestZeit-100)
	if err := pruefeBioBescheinigungMit(nurBetreiber(t, f), bTestZeit+60, zmRegister()); err != nil {
		t.Fatalf("nur der Gruender: %v", err)
	}
	// Zwei Gruender zaehlen als ein Mensch.
	verifierGruender = []string{zmGruender, "0x0000000000000000000000000000000000000abd"}
	if n := zmRegister().menschenZu(bTestZeit); n != 1 {
		t.Fatalf("zwei Gruender = %d Menschen, erwartet 1", n)
	}
	if err := pruefeBioBescheinigungMit(nurBetreiber(t, f), bTestZeit+60, zmRegister()); err != nil {
		t.Fatalf("zwei Gruender: %v", err)
	}
}

// Der echte Stichtag (Betreiber, 26.11.2026): 27.11.2026 12:00 UTC, auf die
// Sekunde; davor unveraendert, auch mit zweitem Pruefer; im Status.
func TestZweiMenschen_Stichtag(t *testing.T) {
	const ab int64 = 1795780800
	if bescheinigungZweiMenschenAbUnix != ab {
		t.Fatalf("Stichtag %d, erwartet %d (27.11.2026 12:00 UTC)", bescheinigungZweiMenschenAbUnix, ab)
	}
	if ab <= bescheinigungRegisterAbUnix || ab <= verifierRegisterAbUnix {
		t.Fatal("die Regel braucht Pruefer-Register und Pruefer-Schluessel vorher")
	}
	f := zmFall(t, 0)
	verifierRegisterOverride.Store(ab - 10_000) // zmFall stellt ihn zurueck
	p := neuerB1bPruefer(t)
	reg := zmRegister(verifierEintrag{Mensch: p.mensch, Seit: ab - 5000})
	davor := bTestTx(t, bTestNullifier, bindung(f.coord, "test", bTestNullifier, bTestWallet, ab-100), aussage(f.zeuge, "test", bTestNullifier, ab-100))
	davor.RegAt = ab - 100
	davor.BioBescheinigung.Zeit = ab - 100
	if err := pruefeBioBescheinigungMit(davor, ab-1, reg); err != nil {
		t.Fatalf("eine Sekunde vor dem Stichtag abgewiesen: %v", err)
	}
	if pruefeBioBescheinigungMit(davor, ab, reg) == nil {
		t.Fatal("am Stichtag: Betreiber allein angenommen")
	}
	if v, _ := BescheinigungStand()["zwei_menschen_ab"].(int64); v != ab {
		t.Fatalf("Status: %v", BescheinigungStand()["zwei_menschen_ab"])
	}
}

// Zeitpunkte: der zweite Pruefer zaehlt, wenn er zur Zeit der Bescheinigung
// ODER des Blocks Pruefer ist; ist er zu beiden Zeiten ausgetreten, reicht
// wieder einer. Vor dem Stichtag (Blockzeit) gilt die Regel nicht.
func TestZweiMenschen_Zeitpunkte(t *testing.T) {
	f := zmFall(t, bTestZeit-100)
	p := neuerB1bPruefer(t)
	spaeter := zmRegister(verifierEintrag{Mensch: p.mensch, Seit: bTestZeit + 30})
	if pruefeBioBescheinigungMit(nurBetreiber(t, f), bTestZeit+60, spaeter) == nil {
		t.Fatal("zweiter Pruefer seit dem Block nicht gezaehlt")
	}
	weg := zmRegister(verifierEintrag{Mensch: p.mensch, Seit: bTestZeit - 900, Bis: bTestZeit - 300})
	if err := pruefeBioBescheinigungMit(nurBetreiber(t, f), bTestZeit+60, weg); err != nil {
		t.Fatalf("ausgetretener zweiter Pruefer: %v", err)
	}
	// Bis genau zur Blockzeit noch Pruefer zur Zeit der Bescheinigung: gilt.
	bisBlock := zmRegister(verifierEintrag{Mensch: p.mensch, Seit: bTestZeit - 900, Bis: bTestZeit + 10})
	if pruefeBioBescheinigungMit(nurBetreiber(t, f), bTestZeit+60, bisBlock) == nil {
		t.Fatal("Austritt nach der Bescheinigung schaltete die Regel ab")
	}
	g := zmFall(t, bTestZeit+61)
	reg := zmRegister(verifierEintrag{Mensch: p.mensch, Seit: bTestZeit - 500})
	if err := pruefeBioBescheinigungMit(nurBetreiber(t, g), bTestZeit+60, reg); err != nil {
		t.Fatalf("eine Sekunde vor dem Stichtag: %v", err)
	}
	if pruefeBioBescheinigungMit(nurBetreiber(t, g), bTestZeit+61, reg) == nil {
		t.Fatal("am Stichtag nicht scharf")
	}
}

// Fail-closed: ohne lesbares Register oder mit einer Quelle, die die Zahl nicht
// kennt, gilt die Regel, sobald sie scharf ist.
func TestZweiMenschen_OhneRegisterFailClosed(t *testing.T) {
	f := zmFall(t, bTestZeit-100)
	if pruefeBioBescheinigungMit(nurBetreiber(t, f), bTestZeit+60, nil) == nil {
		t.Fatal("ohne Register: Betreiber allein angenommen")
	}
	if pruefeBioBescheinigungMit(nurBetreiber(t, f), bTestZeit+60, alsPruefer(func(string, int64) bool { return true })) == nil {
		t.Fatal("Quelle ohne Zahl: Betreiber allein angenommen")
	}
	if !zweiMenschenNoetig(bTestZeit, bTestZeit+60, nil) || zweiMenschenNoetig(bTestZeit, 0, nil) {
		t.Fatal("zweiMenschenNoetig: Grenzen")
	}
}
