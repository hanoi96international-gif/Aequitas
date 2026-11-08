package keeper

import (
	"math"
	"testing"
)

// Regeln vom 08.11.2026 (umzugCodeRegelnAbUnix): EIN Code je Wallet, nicht
// aenderbar, gilt sofort; ein neuer nur nach einem Umzug auf eine neue
// Wallet. Angriff: Dieb mit der Seed-Phrase legt einen eigenen Code dazu.
// Gemessen an der unterschriebenen Zeit, nicht an der Blockzeit.

const uzDiebCode = "ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ"

func codeRegelnAb(t *testing.T, ab int64) {
	t.Helper()
	vorher := umzugCodeRegelnAbOverride.Load()
	umzugCodeRegelnAbOverride.Store(ab)
	t.Cleanup(func() { umzugCodeRegelnAbOverride.Store(vorher) })
}

// Der Dieb mit dem Seed legt keinen zweiten Code dazu -- der Mensch zieht
// mit Gesicht und SEINEM Code sofort um.
func TestUmzugCode_DiebLegtKeinenZweitenDazu(t *testing.T) {
	f := neuerUzFall(t)
	codeRegelnAb(t, 1)
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit); len(b) != 0 {
		t.Fatalf("erster Code: %v", b)
	}
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, uzDiebCode, uzZeit+10), uzZeit+10); b["umzug_code_fest"] != 1 {
		t.Fatalf("zweiter Code: %v", b)
	}
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit+20), uzZeit+20); b["umzug_code_fest"] != 1 {
		t.Fatalf("derselbe Code noch einmal: %v", b)
	}
	if c, _ := f.cs.umzugRegisterJetzt().code(f.alt); len(c.Hashes) != 1 || c.Zeit != uzZeit {
		t.Fatalf("Register: %+v", c)
	}
	diebKey, dieb := neuerSchluessel(t)
	if b := f.anwenden(f.antrag(diebKey, dieb, f.alt, uzN, uzZeit+30, uzDiebCode), uzZeit+30); b["umzug_code"] != 1 || f.mensch(dieb) {
		t.Fatalf("Dieb mit seinem Code: %v", b)
	}
	// Der erste Code gilt sofort.
	if b := f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, uzZeit+40, uzCode), uzZeit+40); len(b) != 0 || !f.umgezogen() {
		t.Fatalf("Mensch mit seinem Code: %v", b)
	}
}

// Ein neuer Code nur nach einem Umzug: die neue Wallet ist ein anderer
// Eintrag und legt ihren eigenen an.
func TestUmzugCode_NeuerCodeNurNachUmzug(t *testing.T) {
	f := neuerUzFall(t)
	codeRegelnAb(t, 1)
	f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit)
	f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, uzZeit+10, uzCode), uzZeit+10)
	if !f.umgezogen() {
		t.Fatal("Vorbedingung: umgezogen")
	}
	if b := f.anwenden(f.codeTx(f.neuKey, f.neu, uzDiebCode, uzZeit+20), uzZeit+20); len(b) != 0 {
		t.Fatalf("Code der neuen Wallet: %v", b)
	}
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, uzDiebCode, uzZeit+30), uzZeit+30); b["umzug_code_kein_mensch"] != 1 {
		t.Fatalf("alte Wallet nach dem Umzug: %v", b)
	}
}

// Gemessen an der unterschriebenen Zeit: Annahme (Uhrzeit) und Nachspielen
// (spaetere Blockzeit) entscheiden gleich -- auch genau an der Grenze.
func TestUmzugCode_AnnahmeUndNachspielenGleich(t *testing.T) {
	ab := uzZeit + 1000
	for _, z := range []int64{ab, ab + 1} {
		for _, block := range []int64{z, z + 1, z + 3600, z + umzugHoechstensAlt} {
			f := neuerUzFall(t)
			codeRegelnAb(t, ab)
			f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit)
			if b := f.anwenden(f.codeTx(f.altKey, f.alt, uzDiebCode, z), block); b["umzug_code_fest"] != 1 {
				t.Fatalf("z %d, Block %d: %v", z, block, b)
			}
		}
	}
}

// Zurueckdatiert: ein Auftrag von vor dem Stichtag kommt in keinen Block
// danach (Urteil, blockTauglich, auftragsFenster) -- sonst legte der Dieb
// nach dem Stichtag mit alter Zeit noch einen zweiten Code dazu.
func TestUmzugCode_ZurueckdatiertNichtNachDemStichtag(t *testing.T) {
	ab := uzZeit + 1000
	f := neuerUzFall(t)
	codeRegelnAb(t, ab)
	f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit)
	alt := f.codeTx(f.altKey, f.alt, uzDiebCode, ab-10)
	if b := f.anwenden(alt, ab); b["umzug_zeit"] != 1 {
		t.Fatalf("zurueckdatiert nach dem Stichtag: %v", b)
	}
	if c, _ := f.cs.umzugRegisterJetzt().code(f.alt); len(c.Hashes) != 1 {
		t.Fatalf("Register: %+v", c)
	}
	if err := blockTauglich(&alt, ab); err == nil {
		t.Fatal("blockTauglich nimmt den zurueckdatierten Auftrag nach dem Stichtag")
	}
	if err := blockTauglich(&alt, ab-1); err != nil {
		t.Fatalf("blockTauglich vor dem Stichtag: %v", err)
	}
	fen := auftragsFenster(&alt, ab)
	if fen.bis != ab-1 {
		t.Fatalf("auftragsFenster: %+v, erwartet bis %d", fen, ab-1)
	}
	neu := f.codeTx(f.altKey, f.alt, uzDiebCode, ab)
	if fen := auftragsFenster(&neu, ab); fen.bis != math.MaxInt64 && fen.bis < ab {
		t.Fatalf("auftragsFenster nach dem Stichtag: %+v", fen)
	}
	// Vor dem Stichtag mit alter Zeit wie bisher: der zweite Code kommt dazu.
	if b := f.anwenden(alt, ab-1); len(b) != 0 {
		t.Fatalf("vor dem Stichtag: %v", b)
	}
}

// Vor dem Stichtag gilt alles wie bisher (Nachspielen alter Bloecke).
func TestUmzugCode_VorDemStichtagWieBisher(t *testing.T) {
	f := neuerUzFall(t)
	codeRegelnAb(t, uzZeit+1000)
	f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit)
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, uzDiebCode, uzZeit+10), uzZeit+10); len(b) != 0 {
		t.Fatalf("zweiter Code vor dem Stichtag: %v", b)
	}
}

// Annahme-Pause (Sicherheitspruefung #472, M1): ab 600 s vor dem Stichtag
// nimmt der Knoten keinen Code-Auftrag mit einer Zeit vor dem Stichtag mehr
// an -- sonst koennte er im Ausgang stehen, waehrend schon ein Block nach dem
// Stichtag entstand, und kein Block passte mehr fuer ihn.
func TestUmzugCode_AnnahmePauseVorDemStichtag(t *testing.T) {
	ab := uzZeit + 10000
	codeRegelnAb(t, ab)
	pause := ab - umzugCodeRegelnPause
	fall := []struct {
		z, jetzt int64
		pausiert bool
	}{
		{pause - 100, pause - 1, false}, // vor der Pause: wie bisher
		{pause - 100, pause, true},      // Pause beginnt
		{ab - 1, ab - 1, true},          // kurz vor dem Stichtag
		{ab - 10, ab + 5, true},         // zurueckdatiert danach
		{ab, ab - 300, false},           // neue Zeit (bis 300 s voraus): neue Regel, keine Pause
		{ab, ab, false},
		{ab + 50, ab + 60, false},
	}
	for _, c := range fall {
		if got := umzugCodeAnnahmePausiert(c.z, c.jetzt); got != c.pausiert {
			t.Errorf("z %d, jetzt %d: pausiert %v, erwartet %v", c.z-ab, c.jetzt-ab, got, c.pausiert)
		}
	}
}
