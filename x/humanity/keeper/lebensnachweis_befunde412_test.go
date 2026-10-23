package keeper

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// Aequitas#412: Befund 2 (nur Betreiber ohne offene Staffel zaehlen, die
// Runde wartet auf Betreiber-Aenderungen im Ausgang) und Befund 4 (die
// Annahme kurz anhalten, statt die Runde rutschen zu lassen).

// Befund 2: zwei Betreiber, einer mit offener Staffel -- der kann nicht
// bescheinigen, also ist keine Pause moeglich (der andere saesse sonst ohne
// Ausweg fest). Ist die Staffel freigegeben, zaehlt er.
func TestLebensnachweis_BetreiberMitStaffelZaehltNicht_RealDB(t *testing.T) {
	f := neuerLnDBFall(t)
	ubiAt := f.jetzt
	zweiter := distTestAddr(1964)
	registerKonto(t, f.cs, zweiter, true)
	verlaufEintrag(t, f.cs, zweiter, distTestAddr(1952), f.jetzt-30*lnTag)

	staffel := func(adresse string, rest float64) {
		t.Helper()
		f.cs.mu.Lock()
		acc := acct(f.cs, adresse)
		acc.GrantStagedRest = NewDecimal(rest)
		err := f.cs.saveAccountToDB(acc)
		f.cs.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	}
	moeglich := func() bool {
		t.Helper()
		f.cs.mu.Lock()
		defer f.cs.mu.Unlock()
		ok, err := f.cs.lebensnachweisPauseMoeglichLocked(context.Background(), ubiAt)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}

	// Missbrauch / Randfall: der zweite Betreiber hat eine offene Staffel.
	staffel(zweiter, 800)
	if moeglich() {
		t.Fatal("Betreiber mit offener Staffel mitgezaehlt -- der andere waere ohne Ausweg pausiert")
	}
	// Auch umgekehrt: der Betreiber des eingetragenen Coordinators mit Staffel.
	staffel(zweiter, 0)
	staffel(f.bindung.Mensch, 800)
	if moeglich() {
		t.Fatal("Coordinator-Betreiber mit offener Staffel mitgezaehlt")
	}
	// Beide ohne Staffel: Pause moeglich.
	staffel(f.bindung.Mensch, 0)
	if !moeglich() {
		t.Fatal("zwei Betreiber ohne Staffel: keine Pause moeglich")
	}
}

// Befund 2, Waechter: offene grant_release (jede Wallet) und register_human
// eines Betreibers halten die Runde an -- sie aendern die Betreiberzahl beim
// Erzeuger vor dem Block. Eine gewoehnliche Registrierung nicht.
func TestLebensnachweisRunde_WartetAufBetreiberAenderungen_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-lebensnachweis-runde-betreiber-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	lnAktiv(t, 1_000, 1_000)
	ctx := context.Background()
	if _, err := cs.db.Exec(`DELETE FROM validator_verlauf`); err != nil {
		t.Fatal(err)
	}
	betreiber := distTestAddr(1970)
	verlaufEintrag(t, cs, betreiber, distTestAddr(1971), 500)
	verblocken := func() {
		t.Helper()
		if _, err := cs.db.Exec(`UPDATE pending_txs SET included_at = 1`); err != nil {
			t.Fatal(err)
		}
	}

	// Eine gewoehnliche Registrierung haelt nichts an.
	if err := savePendingTxExec(cs.db, Transaction{Type: "register_human", Wallet: distTestAddr(1972)}); err != nil {
		t.Fatal(err)
	}
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 2_000); err != nil {
		t.Fatalf("gewoehnliche Registrierung haelt die Runde an: %v", err)
	}
	if g, _ := cs.LebensnachweiseOffenGrund(2_000); g != "" {
		t.Fatalf("Vorpruefung haelt bei gewoehnlicher Registrierung an: %q", g)
	}

	// Missbrauch / Randfall: ein Betreiber wird Mensch, noch in keinem Block --
	// auch in anderer Schreibweise der Wallet.
	if err := savePendingTxExec(cs.db, Transaction{Type: "register_human", Wallet: "0x" + strings.ToUpper(betreiber[2:])}); err != nil {
		t.Fatal(err)
	}
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 2_000); !errors.Is(err, ErrLebensnachweiseOffen) {
		t.Fatalf("offene Registrierung eines Betreibers haelt die Runde nicht an: %v", err)
	}
	g, nachweiseOffen := cs.LebensnachweiseOffenGrund(2_000)
	if g == "" {
		t.Fatal("Vorpruefung laesst die Runde trotz offener Betreiber-Registrierung starten")
	}
	// Ohne offenen Lebensnachweis hilft kein Halt der Annahme.
	if nachweiseOffen {
		t.Fatal("Betreiber-Registrierung als offener Lebensnachweis gemeldet -- die Annahme wuerde ohne Nutzen angehalten")
	}
	verblocken()

	// Eine offene Bindung kann die Betreiberzahl auch SENKEN (Uebernahme,
	// umstrittene Bindung): auch sie haelt an.
	if err := savePendingTxExec(cs.db, Transaction{Type: "validator_bindung", Wallet: distTestAddr(1974), To: distTestAddr(1971)}); err != nil {
		t.Fatal(err)
	}
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 2_000); !errors.Is(err, ErrLebensnachweiseOffen) {
		t.Fatalf("offene validator_bindung haelt die Runde nicht an: %v", err)
	}
	verblocken()

	// Eine offene Staffel-Freigabe (haengender Ausgang) haelt an.
	if err := savePendingTxExec(cs.db, Transaction{Type: "grant_release", Wallet: distTestAddr(1973), Amount: 1}); err != nil {
		t.Fatal(err)
	}
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 2_000); !errors.Is(err, ErrLebensnachweiseOffen) {
		t.Fatalf("offene grant_release haelt die Runde nicht an: %v", err)
	}
	verblocken()
	if err := cs.lebensnachweisRundeBereitLocked(ctx, 2_000); err != nil {
		t.Fatalf("alles verblockt, Runde trotzdem angehalten: %v", err)
	}
}

// Befund 4: der Halt ist begrenzt -- er endet nach lebensnachweisHaltDauer
// von selbst, ein neuer beginnt fruehestens lebensnachweisHaltAbstand nach
// dem letzten, Freigeben oeffnet sofort, und eine zurueckgesprungene Uhr
// verlaengert ihn nicht.
func TestLebensnachweis_AnnahmeHaltBegrenzt(t *testing.T) {
	cs := newTestState()
	if cs.lebensnachweisAngehalten(1_000) != 0 {
		t.Fatal("ohne Halt angehalten")
	}
	if !cs.LebensnachweisAnnahmeHalten(1_000) {
		t.Fatal("erster Halt nicht gesetzt")
	}
	if r := cs.lebensnachweisAngehalten(1_000); r != lebensnachweisHaltDauer {
		t.Fatalf("Rest %d, erwartet %d", r, lebensnachweisHaltDauer)
	}
	if r := cs.lebensnachweisAngehalten(1_000 + lebensnachweisHaltDauer - 1); r != 1 {
		t.Fatalf("Rest kurz vor Ende %d", r)
	}
	if r := cs.lebensnachweisAngehalten(1_000 + lebensnachweisHaltDauer); r != 0 {
		t.Fatalf("Halt laeuft ueber seine Dauer hinaus: %d", r)
	}
	// Missbrauch / Dauerlauf: ein haengender Ausgang laesst die Runde
	// immer wieder warten -- ein neuer Halt erst nach dem Abstand.
	if cs.LebensnachweisAnnahmeHalten(1_000 + lebensnachweisHaltAbstand - 1) {
		t.Fatal("neuer Halt vor Ablauf des Abstands")
	}
	if cs.lebensnachweisAngehalten(1_000+lebensnachweisHaltAbstand-1) != 0 {
		t.Fatal("abgelehnter Halt haelt trotzdem an")
	}
	if !cs.LebensnachweisAnnahmeHalten(1_000 + lebensnachweisHaltAbstand) {
		t.Fatal("kein neuer Halt nach dem Abstand")
	}
	cs.LebensnachweisAnnahmeFreigeben()
	if cs.lebensnachweisAngehalten(1_000+lebensnachweisHaltAbstand) != 0 {
		t.Fatal("Freigeben oeffnet die Annahme nicht")
	}
	// Uhr zurueck: kein Halt, der laenger dauert als vorgesehen.
	if !cs.LebensnachweisAnnahmeHalten(10_000) {
		t.Fatal("Halt nicht gesetzt")
	}
	if r := cs.lebensnachweisAngehalten(5_000); r != 0 {
		t.Fatalf("zurueckgesprungene Uhr haelt %d s an", r)
	}
}

// Befund 4, Annahme: waehrend des Halts 503 mit Retry-After, nichts im
// Zustand oder Ausgang; nach dem Freigeben wird dieselbe Bescheinigung
// angenommen.
func TestLebensnachweis_AnnahmeAngehalten_RealDB(t *testing.T) {
	f := neuerLnDBFall(t)
	issued := nowUnix()
	body := f.koerper(f.wallet, issued, lebensnachweisNachricht(f.wallet, issued))
	if !f.cs.LebensnachweisAnnahmeHalten(nowUnix()) {
		t.Fatal("Halt nicht gesetzt")
	}
	w := f.reiche(body)
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" {
		t.Fatalf("waehrend des Halts: %d %q %s", w.Code, w.Header().Get("Retry-After"), w.Body.String())
	}
	if f.amInDB(f.wallet) != 0 || len(f.imAusgang()) != 0 {
		t.Fatal("waehrend des Halts angewandt oder im Ausgang")
	}
	f.cs.LebensnachweisAnnahmeFreigeben()
	if w := f.reiche(body); w.Code != http.StatusOK {
		t.Fatalf("nach dem Freigeben: %d %s", w.Code, w.Body.String())
	}
	if f.amInDB(f.wallet) != issued || len(f.imAusgang()) != 1 {
		t.Fatal("nach dem Freigeben nicht angenommen")
	}
}
