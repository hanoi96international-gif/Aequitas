package keeper

import (
	"context"
	"testing"
)

// Regeln vom 08.11.2026 (umzugCodeRegelnAbUnix): ein Code je Wallet, nicht
// aenderbar; gueltig erst nach umzugCodeReife; ein noch nicht gueltiger Code
// verfaellt, sobald ein Umzug ohne Code angekuendigt wird; solange einer
// offen ist, kein neuer Code. Angriff: Dieb mit der Seed-Phrase und einem
// Deepfake des Gesichts.

const uzDiebCode = "ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ"

func neuerUzFallRegeln(t *testing.T) *uzFall {
	t.Helper()
	f := neuerUzFall(t)
	vorher := umzugCodeRegelnAbOverride.Load()
	umzugCodeRegelnAbOverride.Store(1)
	t.Cleanup(func() { umzugCodeRegelnAbOverride.Store(vorher) })
	return f
}

func (f *uzFall) vollzuege(t int64) int {
	f.t.Helper()
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	txs, err := f.cs.umzugVollzuegeLocked(context.Background(), t)
	if err != nil {
		f.t.Fatal(err)
	}
	return len(txs)
}

// Ein Code je Wallet: ein zweiter (der eines Diebs mit dem Seed) wird nie
// hinterlegt, auch derselbe nicht noch einmal.
func TestUmzugCode_EinCodeNichtAenderbar(t *testing.T) {
	f := neuerUzFallRegeln(t)
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
}

// Ein Code zaehlt erst nach der Reifezeit -- eine Sekunde davor nicht, danach
// sofort und ohne Widerruf.
func TestUmzugCode_ErstNachReife(t *testing.T) {
	f := neuerUzFallRegeln(t)
	f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit)
	frueh := uzZeit + umzugCodeReife - 1
	if b := f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, frueh, uzCode), frueh); b["umzug_code_unreif"] != 1 || f.umgezogen() {
		t.Fatalf("vor der Reife: %v", b)
	}
	reif := uzZeit + umzugCodeReife
	if b := f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, reif, uzCode), reif); len(b) != 0 || !f.umgezogen() {
		t.Fatalf("nach der Reife: %v", b)
	}
}

// Der Angriff: der Dieb (Seed + Deepfake) legt als Erster einen Code an. Der
// Mensch sieht ihn (App) und kuendigt mit dem Gesicht einen Umzug an -- der
// unreife Code verfaellt, und waehrend der Umzug offen ist, legt der Dieb
// keinen neuen an. Auch nach der Reifezeit zieht der Dieb nicht sofort um.
func TestUmzugCode_DiebCodeVerfaelltBeiUmzugOhneCode(t *testing.T) {
	f := neuerUzFallRegeln(t)
	diebKey, dieb := neuerSchluessel(t)
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, uzDiebCode, uzZeit), uzZeit); len(b) != 0 {
		t.Fatalf("Code des Diebs: %v", b)
	}
	z := uzZeit + 3*86400
	if b := f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, z, ""), z); len(b) != 0 {
		t.Fatalf("Umzug des Menschen ohne Code: %v", b)
	}
	if _, ok := f.cs.umzugRegisterJetzt().code(f.alt); ok {
		t.Fatal("der unreife Code des Diebs ist nicht verfallen")
	}
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, uzDiebCode, z+10), z+10); b["umzug_code_offen"] != 1 {
		t.Fatalf("neuer Code waehrend des offenen Umzugs: %v", b)
	}
	spaet := uzZeit + umzugCodeReife + 10
	if b := f.anwenden(f.antrag(diebKey, dieb, f.alt, uzN, spaet, uzDiebCode), spaet); b["umzug_code"] != 1 || f.mensch(dieb) {
		t.Fatalf("Dieb mit verfallenem Code: %v", b)
	}
	if n := f.vollzuege(z + umzugFrist); n != 1 || !f.umgezogen() || f.mensch(dieb) {
		t.Fatalf("Umzug des Menschen nach der Frist: %d Vollzuege, umgezogen %v", n, f.umgezogen())
	}
}

// Der Dieb widerruft den Umzug des Menschen (einmal im Jahr) und legt neu
// einen Code an. Der zweite Umzug des Menschen ist fest -- und laesst auch
// diesen Code verfallen.
func TestUmzugCode_AuchFesterUmzugVerwirftUnreifenCode(t *testing.T) {
	f := neuerUzFallRegeln(t)
	diebKey, dieb := neuerSchluessel(t)
	z := uzZeit + 100
	f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, z, ""), z)
	if b := f.anwenden(f.widerruf(f.altKey, f.alt, f.neu, uzN, z), z+10); len(b) != 0 {
		t.Fatalf("Widerruf: %v", b)
	}
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, uzDiebCode, z+20), z+20); len(b) != 0 {
		t.Fatalf("Code nach dem Widerruf: %v", b)
	}
	z2 := z + 86400
	if b := f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, z2, ""), z2); len(b) != 0 {
		t.Fatalf("zweiter Umzug: %v", b)
	}
	if o, _ := f.cs.umzugRegisterJetzt().offen(uzN); !o.Fest {
		t.Fatalf("zweiter Umzug nicht fest: %+v", o)
	}
	if _, ok := f.cs.umzugRegisterJetzt().code(f.alt); ok {
		t.Fatal("unreifer Code nach festem Umzug noch da")
	}
	spaet := z + 20 + umzugCodeReife
	if b := f.anwenden(f.antrag(diebKey, dieb, f.alt, uzN, spaet, uzDiebCode), spaet); b["umzug_code"] != 1 || f.mensch(dieb) {
		t.Fatalf("Dieb: %v", b)
	}
}

// Ein REIFER Code verfaellt nicht durch einen Umzug ohne Code (etwa den eines
// Angreifers mit Deepfake) -- und schlaegt ihn wie bisher.
func TestUmzugCode_ReiferCodeBleibtUndGehtVor(t *testing.T) {
	f := neuerUzFallRegeln(t)
	f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit)
	angreiferKey, angreifer := neuerSchluessel(t)
	z := uzZeit + umzugCodeReife + 100
	if b := f.anwenden(f.antrag(angreiferKey, angreifer, f.alt, uzN, z, ""), z); len(b) != 0 {
		t.Fatalf("Angreifer ohne Code: %v", b)
	}
	if _, ok := f.cs.umzugRegisterJetzt().code(f.alt); !ok {
		t.Fatal("reifer Code ist verfallen")
	}
	if b := f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, z+60, uzCode), z+60); len(b) != 0 || !f.umgezogen() {
		t.Fatalf("Mensch mit reifem Code: %v", b)
	}
	if f.vollzuege(z+umzugFristFest) != 0 || f.mensch(angreifer) {
		t.Fatal("der Angreifer darf nie Mensch werden")
	}
}

// Ein Umzug, an dem die Wallet nur als NEUE beteiligt ist, sperrt ihr Code-
// Hinterlegen nicht (offenAlsAlt, nicht offenMit).
func TestUmzugCode_OffenNurAlsAlteWalletSperrt(t *testing.T) {
	f := neuerUzFallRegeln(t)
	_, fremd := neuerSchluessel(t)
	reg := f.cs.umzugRegisterJetzt().mitOffen(umzugOffen{Nullifier: uzN2, Alt: fremd, Neu: f.alt, Zeit: uzZeit, FreiAb: uzZeit + umzugFrist})
	f.cs.mu.Lock()
	if err := f.cs.umzugRegisterSetzenLocked(context.Background(), reg); err != nil {
		f.cs.mu.Unlock()
		t.Fatal(err)
	}
	f.cs.mu.Unlock()
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit+10), uzZeit+10); len(b) != 0 {
		t.Fatalf("Code als neue Wallet eines fremden Umzugs: %v", b)
	}
}

// Vor dem Stichtag gilt alles wie bisher (Nachspielen alter Bloecke):
// mehrere Codes, sofort gueltig, kein Verfall. Danach greift die Regel auch
// fuer die schon hinterlegten Codes.
func TestUmzugCode_VorDemStichtagWieBisher(t *testing.T) {
	f := neuerUzFall(t)
	ab := uzZeit + 1000
	vorher := umzugCodeRegelnAbOverride.Load()
	umzugCodeRegelnAbOverride.Store(ab)
	t.Cleanup(func() { umzugCodeRegelnAbOverride.Store(vorher) })
	f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit)
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, uzDiebCode, uzZeit+10), uzZeit+10); len(b) != 0 {
		t.Fatalf("zweiter Code vor dem Stichtag: %v", b)
	}
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, "1111-2222-3333-4444-5555-6666", ab), ab); b["umzug_code_fest"] != 1 {
		t.Fatalf("dritter Code ab dem Stichtag: %v", b)
	}
	// Ab dem Stichtag zaehlt die Reife auch fuer die alten Codes.
	if b := f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, ab+10, uzCode), ab+10); b["umzug_code_unreif"] != 1 {
		t.Fatalf("alter Code nach dem Stichtag vor der Reife: %v", b)
	}
	f2 := neuerUzFall(t)
	f2.anwenden(f2.codeTx(f2.altKey, f2.alt, uzCode, uzZeit), uzZeit)
	if b := f2.anwenden(f2.antrag(f2.neuKey, f2.neu, f2.alt, uzN, uzZeit+100, uzCode), uzZeit+100); len(b) != 0 || !f2.umgezogen() {
		t.Fatalf("vor dem Stichtag sofort: %v", b)
	}
}
