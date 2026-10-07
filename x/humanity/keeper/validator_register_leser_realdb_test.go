package keeper

import (
	"strings"
	"testing"
	"time"
)

// Schritt 3 mit echter Datenbank: Strafkonto, Erzeugerstand und Leitung aus
// dem Verlauf des Kettenregisters (validator_register_leser.go).

// verlaufEintrag: eine Bindung direkt in validator_verlauf (die Pruefung der
// Unterschriften ist Schritt 1 und dort getestet).
func verlaufEintrag(t *testing.T, cs *ChainState, betreiber, signing string, zeit int64) {
	t.Helper()
	if _, err := cs.db.Exec(`INSERT INTO validator_verlauf (operator_wallet, signing_address, bindung_ts, sig_operator, sig_signing)
		VALUES ($1, $2, $3, 'x', 'y') ON CONFLICT DO NOTHING`,
		strings.ToLower(betreiber), strings.ToLower(signing), zeit); err != nil {
		t.Fatal(err)
	}
}

// registerKonto: ein Konto mit 100 AEQ, Mensch oder nicht.
func registerKonto(t *testing.T, cs *ChainState, adresse string, mensch bool) {
	t.Helper()
	cs.mu.Lock()
	acc := &AccountState{Address: adresse, IsHuman: mensch, Balance: NewDecimal(100)}
	cs.accounts.Set(adresse, acc)
	err := cs.saveAccountToDB(acc)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
}

// strafFall: registered_nodes nennt k.op (100 AEQ, wie bisher), der Verlauf
// einen anderen Betreiber reg (Mensch, 100 AEQ, seit zehn Tagen) -- beide
// fuer denselben Signierschluessel. Das erste Vergehen steht schon im Block.
func strafFall(t *testing.T) (*strafKnoten, string) {
	t.Helper()
	k := neuerStrafKnoten(t)
	for _, q := range []string{`DELETE FROM validator_register`, `DELETE FROM validator_verlauf`} {
		if _, err := k.cs.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	reg := distTestAddr(1701)
	registerKonto(t, k.cs, reg, true)
	verlaufEintrag(t, k.cs, reg, k.signer, nowUnix()-10*86400)
	if !k.block(k.doppelsignatur(nowUnix()-5*86400, "aa", "bb")) {
		t.Fatal("erstes Vergehen abgelehnt")
	}
	return k, reg
}

func standVon(t *testing.T, cs *ChainState, adresse string) float64 {
	t.Helper()
	var db float64
	if err := cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, adresse).Scan(&db); err != nil {
		t.Fatal(err)
	}
	return db
}

// zweitesVergehen: das zweite Vergehen zur Zeit tat, im Block nachgespielt,
// mit registerLeserAb = 1.
func zweitesVergehen(t *testing.T, k *strafKnoten, tat int64) {
	t.Helper()
	registerLeserOverride.Store(1)
	ok := k.block(k.doppelsignatur(tat, "cc", "dd"))
	registerLeserOverride.Store(0)
	if !ok {
		t.Fatal("zweites Vergehen abgelehnt")
	}
}

const strafe = 100 - equivocationSecondOffensePenaltyAEQ

// Vor dem Stichtag zahlt, wen registered_nodes nennt -- byte-gleich wie
// bisher, auch wenn der Verlauf etwas anderes sagt.
func TestStrafkonto_VorDemStichtagWieBisher_RealDB(t *testing.T) {
	k, reg := strafFall(t)
	if !k.block(k.doppelsignatur(nowUnix()-3600, "cc", "dd")) {
		t.Fatal("zweites Vergehen abgelehnt")
	}
	if got := standVon(t, k.cs, k.op); got != strafe {
		t.Fatalf("registered_nodes-Betreiber %.2f", got)
	}
	if got := standVon(t, k.cs, reg); got != 100 {
		t.Fatalf("Register-Betreiber vor dem Stichtag belastet: %.2f", got)
	}
}

// Missbrauch: ab dem Stichtag zahlt der Betreiber aus dem Verlauf -- ein
// Eintrag in registered_nodes (jeder Knoten fuehrt ihn selbst) lenkt die
// Strafe nicht mehr um. Beim Nachspielen wie bei der Erkennung.
func TestStrafkonto_AbStichtagAusDemVerlauf_RealDB(t *testing.T) {
	for _, weg := range []string{"nachspielen", "erkennen"} {
		k, reg := strafFall(t)
		registerLeserOverride.Store(1)
		zweites := k.doppelsignatur(nowUnix()-3600, "cc", "dd")
		switch weg {
		case "nachspielen":
			if !k.block(zweites) {
				t.Fatal("zweites Vergehen abgelehnt")
			}
		case "erkennen":
			if _, _, err := k.cs.DoppelsignaturErkannt(k.signer, zweites.BlockAHash, zweites.BlockBHash, zweites.DetectedAt, zweites.Doppelbeweis); err != nil {
				t.Fatal(err)
			}
		}
		registerLeserOverride.Store(0)
		if got := standVon(t, k.cs, reg); got != strafe {
			t.Fatalf("%s: Register-Betreiber %.2f", weg, got)
		}
		if got := standVon(t, k.cs, k.op); got != 100 {
			t.Fatalf("%s: registered_nodes-Betreiber belastet: %.2f", weg, got)
		}
	}
}

// Missbrauch (MEDIUM 1, #303): DetectedAt waehlt, wer den Schluessel haelt.
// Datiert er den Beweis vor den Stichtag, bleibt es trotzdem beim Verlauf --
// geschaltet wird an der Blockzeit (beim Erkennen an der Uhr), wenn sie
// spaeter liegt.
func TestStrafkonto_RueckdatierterBeweisUmgehtDenStichtagNicht_RealDB(t *testing.T) {
	for _, weg := range []string{"nachspielen", "erkennen"} {
		k, reg := strafFall(t)
		registerLeserOverride.Store(nowUnix() - 1800)
		zweites := k.doppelsignatur(nowUnix()-3600, "cc", "dd") // vor dem Stichtag
		switch weg {
		case "nachspielen":
			if !k.block(zweites) { // Blockzeit: jetzt, nach dem Stichtag
				t.Fatal("zweites Vergehen abgelehnt")
			}
		case "erkennen":
			if _, _, err := k.cs.DoppelsignaturErkannt(k.signer, zweites.BlockAHash, zweites.BlockBHash, zweites.DetectedAt, zweites.Doppelbeweis); err != nil {
				t.Fatal(err)
			}
		}
		registerLeserOverride.Store(0)
		if got := standVon(t, k.cs, k.op); got != 100 {
			t.Fatalf("%s: rueckdatierter Beweis nahm den alten Weg (registered_nodes %.2f)", weg, got)
		}
		if got := standVon(t, k.cs, reg); got != strafe {
			t.Fatalf("%s: Halter aus dem Verlauf %.2f", weg, got)
		}
	}
}

// Kein eindeutiger Halter: umstritten (zwei mit demselben Zeitpunkt), kein
// Mensch, oder keine Bindung vor der Tat -- keine Geldstrafe, die Sperre
// bleibt.
func TestStrafkonto_UmstrittenOderKeinMensch_RealDB(t *testing.T) {
	for _, fall := range []string{"umstritten", "kein Mensch", "keine Bindung", "erst nach der Tat gebunden"} {
		k, reg := strafFall(t)
		switch fall {
		case "umstritten":
			zweiter := distTestAddr(1702)
			registerKonto(t, k.cs, zweiter, true)
			verlaufEintrag(t, k.cs, zweiter, k.signer, nowUnix()-10*86400)
		case "kein Mensch":
			registerKonto(t, k.cs, reg, false)
		case "keine Bindung":
			k.cs.db.Exec(`DELETE FROM validator_verlauf`)
		case "erst nach der Tat gebunden":
			k.cs.db.Exec(`DELETE FROM validator_verlauf`)
			verlaufEintrag(t, k.cs, reg, k.signer, nowUnix()-1800)
		}
		zweitesVergehen(t, k, nowUnix()-3600)
		if got := standVon(t, k.cs, reg); got != 100 {
			t.Fatalf("%s: Register-Betreiber belastet: %.2f", fall, got)
		}
		if got := standVon(t, k.cs, k.op); got != 100 {
			t.Fatalf("%s: registered_nodes-Betreiber belastet: %.2f", fall, got)
		}
		if gesperrt, _ := k.cs.IsValidatorSuspended(k.signer, 0); !gesperrt {
			t.Fatalf("%s: Sperre fehlt", fall)
		}
	}
}

// Missbrauch (MEDIUM 1, #303): wer den Schluessel nach der Tat uebernimmt,
// besitzt ihn und haette den Beweis selbst unterschreiben koennen -- mit
// einem Zeitpunkt, zu dem ihn der fruehere Halter hielt. Darum zahlt dann
// KEINER: weder der spaetere Halter noch der fruehere. Die Sperre bleibt.
func TestStrafkonto_UebernahmeNachDerTatZahltKeiner_RealDB(t *testing.T) {
	k, reg := strafFall(t)
	spaeter := distTestAddr(1703)
	registerKonto(t, k.cs, spaeter, true)
	verlaufEintrag(t, k.cs, spaeter, k.signer, nowUnix()-1800)
	// Vor der Tat hielt der Spaetere einen anderen Schluessel -- der zaehlt
	// fuer K nicht.
	verlaufEintrag(t, k.cs, spaeter, distTestAddr(1709), nowUnix()-2*86400)
	zweitesVergehen(t, k, nowUnix()-3600)
	if got := standVon(t, k.cs, reg); got != 100 {
		t.Fatalf("frueherer Halter belastet: %.2f -- der spaetere haette ihm den Beweis anhaengen koennen", got)
	}
	if got := standVon(t, k.cs, spaeter); got != 100 {
		t.Fatalf("spaeterer Halter belastet: %.2f", got)
	}
	if gesperrt, _ := k.cs.IsValidatorSuspended(k.signer, 0); !gesperrt {
		t.Fatal("Sperre fehlt")
	}
}

// Der Ablauf aus MEDIUM 1 (#303): V bindet K, B uebernimmt K, B zieht zu K3
// weiter. Fuer eine Tat in B's Zeit zahlt B -- vorher fand das Register nur
// noch V's (ueberholte) Zeile, und V zahlte fuer B.
func TestStrafkonto_UebernahmeUndWechsel_RealDB(t *testing.T) {
	k, v := strafFall(t) // V bindet K vor zehn Tagen
	b := distTestAddr(1704)
	registerKonto(t, k.cs, b, true)
	verlaufEintrag(t, k.cs, b, k.signer, nowUnix()-2*86400)
	verlaufEintrag(t, k.cs, b, distTestAddr(1705), nowUnix()-86400)
	zweitesVergehen(t, k, nowUnix()-36*3600)
	if got := standVon(t, k.cs, b); got != strafe {
		t.Fatalf("Halter zur Tat (B) %.2f", got)
	}
	if got := standVon(t, k.cs, v); got != 100 {
		t.Fatalf("V zahlte fuer B: %.2f", got)
	}
}

// Wer nur den eigenen Schluessel wechselt, bleibt fuer den alten haftbar --
// er kennt ihn weiter, und in der Frist darf der alte noch erzeugen.
func TestStrafkonto_EigenerWechselBefreitNicht_RealDB(t *testing.T) {
	k, v := strafFall(t)
	verlaufEintrag(t, k.cs, v, distTestAddr(1706), nowUnix()-7200)
	zweitesVergehen(t, k, nowUnix()-3600)
	if got := standVon(t, k.cs, v); got != strafe {
		t.Fatalf("nach eigenem Wechsel nicht belastet: %.2f", got)
	}
}

// Missbrauch: B bindet K und im selben Augenblick einen anderen Schluessel,
// sodass B's Zeitraum fuer K leer ist. Die Zeile zaehlt trotzdem -- B
// besitzt K und haette V den Beweis anhaengen koennen.
func TestStrafkonto_LeererZeitraumZaehltAlsUebernahme_RealDB(t *testing.T) {
	k, v := strafFall(t)
	b := distTestAddr(1707)
	registerKonto(t, k.cs, b, true)
	zeit := nowUnix() - 1800
	verlaufEintrag(t, k.cs, b, k.signer, zeit)
	verlaufEintrag(t, k.cs, b, "0x"+strings.Repeat("f", 40), zeit) // ordnet nach K
	if iv := bindungsIntervalle([]bindungsZeile{z(b, k.signer, zeit), z(b, "0x"+strings.Repeat("f", 40), zeit)}); len(iv) != 1 {
		t.Fatalf("Voraussetzung: B's Zeitraum fuer K leer, erwartet ein Zeitraum, %+v", iv)
	}
	zweitesVergehen(t, k, nowUnix()-3600)
	if got := standVon(t, k.cs, v); got != 100 {
		t.Fatalf("V belastet: %.2f", got)
	}
}

// Fail-closed: ist der Verlauf nicht lesbar, wird der Block abgewiesen --
// nie "keine Strafe".
func TestStrafkonto_VerlaufNichtLesbarWeistAb_RealDB(t *testing.T) {
	k, reg := strafFall(t)
	if _, err := k.cs.db.Exec(`ALTER TABLE validator_verlauf RENAME TO validator_verlauf_weg`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.cs.db.Exec(`ALTER TABLE validator_verlauf_weg RENAME TO validator_verlauf`) })
	registerLeserOverride.Store(1)
	ok := k.block(k.doppelsignatur(nowUnix()-3600, "cc", "dd"))
	registerLeserOverride.Store(0)
	if ok {
		t.Fatal("Block trotz unlesbarem Verlauf angenommen")
	}
	if _, err := k.cs.db.Exec(`ALTER TABLE validator_verlauf_weg RENAME TO validator_verlauf`); err != nil {
		t.Fatal(err)
	}
	if got := standVon(t, k.cs, reg); got != 100 {
		t.Fatalf("nach der Abweisung belastet: %.2f", got)
	}
	if v := k.vergehen(); v != 1 {
		t.Fatalf("%d Vergehen nach der Abweisung (erwartet 1)", v)
	}
}

// LOW 1 (#303): aendert sich das Strafkonto zwischen der Vorab-Lesung und
// der Transaktion (ein Block mit einer Bindung wird nachgespielt, waehrend
// der Erkennende auf cs.mu wartet), versucht er es einmal neu -- vorher fiel
// die ganze Erkennung weg: Sperre, Zaehler und Beweis-Transaktion.
func TestDoppelsignaturErkannt_StrafkontoAendertSichWaehrenddessen_RealDB(t *testing.T) {
	k, _ := strafFall(t)
	k.cs.db.Exec(`DELETE FROM validator_verlauf`) // vorab: kein Halter
	neu := distTestAddr(1708)
	registerKonto(t, k.cs, neu, true)
	aufrufe := 0
	strafkontoVorabGelesen = func() {
		aufrufe++
		if aufrufe == 1 {
			verlaufEintrag(t, k.cs, neu, k.signer, nowUnix()-7200)
		}
	}
	t.Cleanup(func() { strafkontoVorabGelesen = nil })
	registerLeserOverride.Store(1)
	zweites := k.doppelsignatur(nowUnix()-3600, "cc", "dd")
	_, betrag, err := k.cs.DoppelsignaturErkannt(k.signer, zweites.BlockAHash, zweites.BlockBHash, zweites.DetectedAt, zweites.Doppelbeweis)
	registerLeserOverride.Store(0)
	if err != nil {
		t.Fatalf("Erkennung verworfen: %v", err)
	}
	if aufrufe != 2 {
		t.Fatalf("%d Versuche, erwartet 2", aufrufe)
	}
	if betrag != equivocationSecondOffensePenaltyAEQ || standVon(t, k.cs, neu) != strafe {
		t.Fatalf("Strafe %.2f, Halter %.2f", betrag, standVon(t, k.cs, neu))
	}
	if v := k.vergehen(); v != 2 {
		t.Fatalf("%d Vergehen, erwartet 2", v)
	}
	if n := k.beweise(zweites); n != 1 {
		t.Fatalf("%d Beweise, erwartet 1", n)
	}
}

// Der Erzeugerstand aus dem Verlauf: Zeitraeume menschlicher Betreiber mit
// Frist; geschlossen nur die Schluessel der Liste. Ein Lesefehler schliesst
// die Erzeugerpruefung, die Leitung behaelt den letzten Stand.
func TestErzeugerAusVerlauf_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	for _, q := range []string{`DELETE FROM validator_register`, `DELETE FROM validator_verlauf`} {
		if _, err := f.cs.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	m := func(n int) string { return distTestAddr(1800 + n) }
	s := func(n int) string { return distTestAddr(1850 + n) }
	for i, mensch := range []bool{true, true, true, true, false} {
		registerKonto(t, f.cs, m(i), mensch)
	}
	jetzt := nowUnix()
	lange := jetzt - 30*86400
	verlaufEintrag(t, f.cs, m(0), s(0), lange) // gilt
	verlaufEintrag(t, f.cs, m(1), s(1), lange) // gewechselt ...
	verlaufEintrag(t, f.cs, m(1), s(5), jetzt) // ... gerade eben (Frist laeuft)
	verlaufEintrag(t, f.cs, m(2), s(2), lange) // umstritten ...
	verlaufEintrag(t, f.cs, m(3), s(2), lange) // ... mit diesem
	verlaufEintrag(t, f.cs, m(4), s(4), lange) // kein Mensch

	dag := newOrphanTestDAG()
	dag.state = f.cs
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	f.cs.erzeugerRegisterAuffrischen()
	for addr, want := range map[string]bool{s(0): true, s(1): true, s(5): false, s(2): false, s(4): false} {
		if got := dag.erzeugerNachRegister(addr, jetzt); got != want {
			t.Fatalf("offen, %s jetzt: %v, erwartet %v", addr, got, want)
		}
	}
	if !dag.erzeugerNachRegister(s(5), jetzt+erzeugerFrist) || dag.erzeugerNachRegister(s(1), jetzt+erzeugerFrist) {
		t.Fatal("nach der Frist: Wechsel von s1 zu s5 nicht vollzogen")
	}
	for addr, want := range map[string]string{s(0): m(0), s(1): m(1), s(5): "", s(2): "", s(4): ""} {
		if got := dag.menschAusRegister(addr); got != want {
			t.Fatalf("Mensch zu %s: %q, erwartet %q", addr, got, want)
		}
	}

	// Geschlossen: nur die Schluessel der Liste. s5 wird mitgelesen (sein
	// Betreiber m1 haelt s1, das auf der Liste steht), bekommt aber kein
	// Fenster.
	fest := []string{s(0), s(1), s(4)}
	f.cs.erzeugerFest.Store(&fest)
	t.Cleanup(func() { f.cs.erzeugerFest.Store(nil) })
	f.cs.erzeugerRegisterAuffrischen()
	st := f.cs.erzeugerRegister.Load()
	if st == nil || st.fehler != nil || len(st.fenster) != 2 || len(st.fenster[s(0)]) != 1 || len(st.fenster[s(1)]) != 1 || st.fenster[s(5)] != nil {
		t.Fatalf("geschlossen: %+v", st)
	}

	// Ein Lesefehler: die Erzeugerpruefung schliesst ab, die Leitung behaelt
	// den letzten Stand.
	if _, err := f.cs.db.Exec(`ALTER TABLE validator_verlauf RENAME TO validator_verlauf_weg`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.cs.db.Exec(`ALTER TABLE validator_verlauf_weg RENAME TO validator_verlauf`) })
	f.cs.erzeugerRegisterAuffrischen()
	if st := f.cs.erzeugerRegister.Load(); st == nil || st.fehler == nil {
		t.Fatalf("Lesefehler nicht im Stand: %+v", st)
	}
	if dag.erzeugerNachRegister(s(0), jetzt) {
		t.Fatal("Erzeugerpruefung mit Lesefehler offen")
	}
	if got := dag.menschAusRegister(s(0)); got != m(0) {
		t.Fatalf("Leitung nach dem Lesefehler: %q", got)
	}
}

// Die eigene Annahme einer Bindung frischt den Erzeugerstand auf.
func TestValidatorBinden_FrischtErzeugerAuf_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	f.cs.annehmendAusdruecklich.Store(true)
	t.Cleanup(func() { f.cs.annehmendAusdruecklich.Store(false) })
	betreiber := f.betreiber()
	knoten, signing := neuerSchluessel(t)
	zeit := nowUnix()
	if err := f.cs.ValidatorBinden(bindungUnterschrieben(t, betreiber, knoten, zeit)); err != nil {
		t.Fatal(err)
	}
	st := f.cs.erzeugerRegister.Load()
	if st == nil || st.fehler != nil || st.erzeugerFenster(signing, zeit+erzeugerFrist) != adrVon(betreiber) {
		t.Fatalf("Erzeugerstand nach der Annahme: %+v", st)
	}
	if f.cs.registerGeaendert.Load() {
		t.Fatal("Merker nach dem Auffrischen nicht zurueckgesetzt")
	}
}

// LOW 4 (#303): das Nachspielen frischt den Stand nur auf, wenn der Block
// den Verlauf erweitert hat -- eine Bindung, die als Zustandsablehnung
// uebersprungen wird, loest keine Abfrage aus.
func TestReplay_FrischtNurBeiNeuerZeileAuf_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	merker := &erzeugerStand{zeit: time.Unix(1, 0)}
	f.cs.erzeugerRegister.Store(merker)
	f.cs.registerGeaendert.Store(false)
	keinMensch, _ := neuerSchluessel(t)
	k1, _ := neuerSchluessel(t)
	if !f.block(f.jetzt, bindungUnterschrieben(t, keinMensch, k1, f.jetzt-60)) {
		t.Fatal("Block abgewiesen")
	}
	if f.cs.erzeugerRegister.Load() != merker {
		t.Fatal("uebersprungene Bindung hat den Stand neu lesen lassen")
	}
	betreiber := f.betreiber()
	k2, signing := neuerSchluessel(t)
	if !f.block(f.jetzt, bindungUnterschrieben(t, betreiber, k2, f.jetzt-60)) {
		t.Fatal("Block abgewiesen")
	}
	st := f.cs.erzeugerRegister.Load()
	if st == merker || st.erzeugerFenster(signing, f.jetzt-60+erzeugerFrist) != adrVon(betreiber) {
		t.Fatalf("neue Bindung nicht im Stand: %+v", st)
	}
}
