package keeper

import (
	"context"
	"strings"
	"testing"
)

// Schritt 3 mit echter Datenbank: Strafkonto, Erzeugerstand und Leitung aus
// dem Kettenregister (validator_register_leser.go).

// registerZeile: eine Bindung direkt in validator_register (die Pruefung der
// Unterschriften ist Schritt 1 und dort getestet).
func registerZeile(t *testing.T, cs *ChainState, betreiber, signing string, zeit int64, ueberholt bool) {
	t.Helper()
	if _, err := cs.db.Exec(`INSERT INTO validator_register (operator_wallet, signing_address, bindung_ts, sig_operator, sig_signing, ueberholt)
		VALUES ($1, $2, $3, 'x', 'y', $4)
		ON CONFLICT (operator_wallet) DO UPDATE SET signing_address = $2, bindung_ts = $3, ueberholt = $4`,
		strings.ToLower(betreiber), strings.ToLower(signing), zeit, ueberholt); err != nil {
		t.Fatal(err)
	}
}

// konto: ein Konto mit 100 AEQ, Mensch oder nicht.
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

// strafFall: registered_nodes nennt k.op (100 AEQ, wie bisher), das Register
// einen anderen Betreiber reg (Mensch, 100 AEQ) -- beide fuer denselben
// Signierschluessel. Das erste Vergehen steht schon im Block.
func strafFall(t *testing.T) (*strafKnoten, string) {
	t.Helper()
	k := neuerStrafKnoten(t)
	if _, err := k.cs.db.Exec(`DELETE FROM validator_register`); err != nil {
		t.Fatal(err)
	}
	reg := distTestAddr(1701)
	registerKonto(t, k.cs, reg, true)
	registerZeile(t, k.cs, reg, k.signer, nowUnix()-10*86400, false)
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

// Vor dem Stichtag zahlt, wen registered_nodes nennt -- byte-gleich wie
// bisher, auch wenn das Register etwas anderes sagt.
func TestStrafkonto_VorDemStichtagWieBisher_RealDB(t *testing.T) {
	k, reg := strafFall(t)
	if !k.block(k.doppelsignatur(nowUnix()-3600, "cc", "dd")) {
		t.Fatal("zweites Vergehen abgelehnt")
	}
	if got := standVon(t, k.cs, k.op); got != 100-equivocationSecondOffensePenaltyAEQ {
		t.Fatalf("registered_nodes-Betreiber %.2f", got)
	}
	if got := standVon(t, k.cs, reg); got != 100 {
		t.Fatalf("Register-Betreiber vor dem Stichtag belastet: %.2f", got)
	}
}

// Missbrauch: ab dem Stichtag zahlt der Betreiber aus dem Register -- ein
// Eintrag in registered_nodes (jeder Knoten fuehrt ihn selbst) lenkt die
// Strafe nicht mehr um. Beim Nachspielen wie bei der Erkennung.
func TestStrafkonto_AbStichtagAusDemRegister_RealDB(t *testing.T) {
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
		if got := standVon(t, k.cs, reg); got != 100-equivocationSecondOffensePenaltyAEQ {
			t.Fatalf("%s: Register-Betreiber %.2f", weg, got)
		}
		if got := standVon(t, k.cs, k.op); got != 100 {
			t.Fatalf("%s: registered_nodes-Betreiber belastet: %.2f", weg, got)
		}
	}
}

// Kein eindeutiger Betreiber: umstritten (zwei mit demselben Zeitpunkt) oder
// kein Mensch -- keine Geldstrafe, die Sperre bleibt.
func TestStrafkonto_UmstrittenOderKeinMensch_RealDB(t *testing.T) {
	for _, fall := range []string{"umstritten", "kein Mensch", "keine Bindung"} {
		k, reg := strafFall(t)
		switch fall {
		case "umstritten":
			zweiter := distTestAddr(1702)
			registerKonto(t, k.cs, zweiter, true)
			registerZeile(t, k.cs, zweiter, k.signer, nowUnix()-10*86400, false)
		case "kein Mensch":
			registerKonto(t, k.cs, reg, false)
		case "keine Bindung":
			k.cs.db.Exec(`DELETE FROM validator_register`)
		}
		registerLeserOverride.Store(1)
		ok := k.block(k.doppelsignatur(nowUnix()-3600, "cc", "dd"))
		registerLeserOverride.Store(0)
		if !ok {
			t.Fatalf("%s: Block abgelehnt", fall)
		}
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

// Missbrauch: wer den Schluessel nach der Tat uebernimmt, zahlt nicht -- es
// zahlt, wer ihn zur Tat hielt (seine Bindung ist inzwischen ueberholt).
func TestStrafkonto_UebernahmeNachDerTat_RealDB(t *testing.T) {
	k, reg := strafFall(t)
	spaeter := distTestAddr(1703)
	registerKonto(t, k.cs, spaeter, true)
	registerZeile(t, k.cs, spaeter, k.signer, nowUnix()+60, false)
	if _, err := k.cs.db.Exec(`UPDATE validator_register SET ueberholt = true WHERE operator_wallet = $1`, reg); err != nil {
		t.Fatal(err)
	}
	registerLeserOverride.Store(1)
	ok := k.block(k.doppelsignatur(nowUnix()-3600, "cc", "dd"))
	registerLeserOverride.Store(0)
	if !ok {
		t.Fatal("Block abgelehnt")
	}
	if got := standVon(t, k.cs, reg); got != 100-equivocationSecondOffensePenaltyAEQ {
		t.Fatalf("Halter zur Tat %.2f", got)
	}
	if got := standVon(t, k.cs, spaeter); got != 100 {
		t.Fatalf("spaeterer Halter belastet: %.2f", got)
	}
}

// Fail-closed: ist das Register nicht lesbar, wird der Block abgewiesen --
// nie "keine Strafe".
func TestStrafkonto_RegisterNichtLesbarWeistAb_RealDB(t *testing.T) {
	k, reg := strafFall(t)
	if _, err := k.cs.db.Exec(`ALTER TABLE validator_register RENAME TO validator_register_weg`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.cs.db.Exec(`ALTER TABLE validator_register_weg RENAME TO validator_register`) })
	registerLeserOverride.Store(1)
	ok := k.block(k.doppelsignatur(nowUnix()-3600, "cc", "dd"))
	registerLeserOverride.Store(0)
	if ok {
		t.Fatal("Block trotz unlesbarem Register angenommen")
	}
	if _, err := k.cs.db.Exec(`ALTER TABLE validator_register_weg RENAME TO validator_register`); err != nil {
		t.Fatal(err)
	}
	if got := standVon(t, k.cs, reg); got != 100 {
		t.Fatalf("nach der Abweisung belastet: %.2f", got)
	}
	if v := k.vergehen(); v != 1 {
		t.Fatalf("%d Vergehen nach der Abweisung (erwartet 1)", v)
	}
}

// Der Erzeugerstand: nur nicht ueberholte, unumstrittene Bindungen von
// Menschen.
func TestErzeugerAusRegister_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	if _, err := f.cs.db.Exec(`DELETE FROM validator_register`); err != nil {
		t.Fatal(err)
	}
	m := func(n int) string { return distTestAddr(1800 + n) }
	s := func(n int) string { return distTestAddr(1850 + n) }
	for i, mensch := range []bool{true, true, true, true, false} {
		registerKonto(t, f.cs, m(i), mensch)
	}
	jetzt := nowUnix()
	registerZeile(t, f.cs, m(0), s(0), jetzt-100, false) // gilt
	registerZeile(t, f.cs, m(1), s(1), jetzt-100, true)  // ueberholt
	registerZeile(t, f.cs, m(2), s(2), jetzt-100, false) // umstritten ...
	registerZeile(t, f.cs, m(3), s(2), jetzt-100, false) // ... mit diesem
	registerZeile(t, f.cs, m(4), s(4), jetzt-100, false) // kein Mensch
	got, err := f.cs.validatorErzeugerAusRegister(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[s(0)] {
		t.Fatalf("Erzeuger aus dem Register: %v, erwartet nur %s", got, s(0))
	}
	f.cs.erzeugerRegisterAuffrischen()
	if st := f.cs.erzeugerRegister.Load(); st == nil || st.fehler != nil || !st.adressen[s(0)] || len(st.adressen) != 1 {
		t.Fatalf("Stand nach dem Auffrischen: %+v", st)
	}
	// Ein Lesefehler ersetzt den Stand -- die Pruefung schliesst ab.
	if _, err := f.cs.db.Exec(`ALTER TABLE validator_register RENAME TO validator_register_weg`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.cs.db.Exec(`ALTER TABLE validator_register_weg RENAME TO validator_register`) })
	f.cs.erzeugerRegisterAuffrischen()
	if st := f.cs.erzeugerRegister.Load(); st == nil || st.fehler == nil {
		t.Fatalf("Lesefehler nicht im Stand: %+v", st)
	}
	f.cs.db.Exec(`ALTER TABLE validator_register_weg RENAME TO validator_register`)

	// Die Leitung: derselbe Massstab.
	dag := newOrphanTestDAG()
	dag.state = f.cs
	for addr, want := range map[string]string{s(0): m(0), s(1): "", s(2): "", s(4): ""} {
		if got := dag.menschAusRegister(addr); got != want {
			t.Fatalf("Mensch zu %s: %q, erwartet %q", addr, got, want)
		}
	}
}

// Die eigene Annahme einer Bindung frischt den Erzeugerstand auf.
func TestValidatorBinden_FrischtErzeugerAuf_RealDB(t *testing.T) {
	f := neuerRegisterFall(t)
	f.cs.annehmendAusdruecklich.Store(true)
	t.Cleanup(func() { f.cs.annehmendAusdruecklich.Store(false) })
	betreiber := f.betreiber()
	knoten, signing := neuerSchluessel(t)
	if err := f.cs.ValidatorBinden(bindungUnterschrieben(t, betreiber, knoten, nowUnix())); err != nil {
		t.Fatal(err)
	}
	if st := f.cs.erzeugerRegister.Load(); st == nil || st.fehler != nil || !st.adressen[signing] {
		t.Fatalf("Erzeugerstand nach der Annahme: %+v", st)
	}
}
