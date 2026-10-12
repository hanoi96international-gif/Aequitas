package keeper

import (
	"strings"
	"testing"
	"time"
)

// Pruefung #330, LOW-3 (Kopf-Blockade): Zeilen, die nie geschlossen werden
// (hier: korruptes pending_tx_json), verdraengten juengere fuer immer, weil
// der Durchlauf die 1.000 aeltesten las. Jetzt rueckt eine versuchte Zeile ans
// Ende: im zweiten Durchlauf kommt die juengere an die Reihe.
func TestRecovery_KeineKopfBlockade_RealDB(t *testing.T) {
	cs := recoveryTestKnoten(t, "unused-recovery-kopf-test.json")
	alt := time.Now().Unix() - 3600
	if _, err := cs.db.Exec(`INSERT INTO registration_recovery (wallet, evm_tx_hash, nullifier, pending_tx_json, created_at)
		SELECT '0x' || lpad(to_hex(g), 40, '0'), '0xhash' || g, '', '{', $1 FROM generate_series(1, $2) g`,
		alt, recoveryHoechstensJeDurchlauf); err != nil {
		t.Fatal(err)
	}
	var jung int64
	if err := cs.db.QueryRow(`INSERT INTO registration_recovery (wallet, evm_tx_hash, nullifier, pending_tx_json, created_at)
		VALUES ($1, '0xjung', '', '{', $2) RETURNING id`, distTestAddr(2399), time.Now().Unix()).Scan(&jung); err != nil {
		t.Fatal(err)
	}
	versuche := func() int {
		var n int
		cs.db.QueryRow(`SELECT attempt_count FROM registration_recovery WHERE id=$1`, jung).Scan(&n)
		return n
	}
	cs.RetryRegistrationRecoveries()
	if n := versuche(); n != 0 {
		t.Fatalf("Vorbedingung: im ersten Durchlauf die aelteren (%d Versuche an der juengeren)", n)
	}
	cs.RetryRegistrationRecoveries()
	if n := versuche(); n != 1 {
		t.Fatalf("die juengere Zeile kam auch im zweiten Durchlauf nicht dran (%d Versuche) -- Kopf-Blockade", n)
	}
}

// Bestaetigte Registrierungen (mit Hash) kommen vor liegengebliebenen
// Vor-EVM-Intents an die Reihe, auch wenn diese aelter sind.
func TestRecovery_HashZeilenZuerst_RealDB(t *testing.T) {
	cs := recoveryTestKnoten(t, "unused-recovery-hash-zuerst-test.json")
	alt := time.Now().Unix() - 3600
	// Juenger als vorEVMMindestAlterSek waeren sie "unterwegs"; hier alt und
	// ohne Beleg -- sie wuerden verworfen, wenn sie drankaemen.
	if _, err := cs.db.Exec(`INSERT INTO registration_recovery (wallet, evm_tx_hash, nullifier, pending_tx_json, created_at)
		SELECT '0x' || lpad(to_hex(g + 5000), 40, '0'), '', '', '', $1 FROM generate_series(1, $2) g`,
		alt, recoveryHoechstensJeDurchlauf); err != nil {
		t.Fatal(err)
	}
	var mitHash int64
	if err := cs.db.QueryRow(`INSERT INTO registration_recovery (wallet, evm_tx_hash, nullifier, pending_tx_json, created_at)
		VALUES ($1, '0xbestaetigt', '', '{', $2) RETURNING id`, distTestAddr(2398), time.Now().Unix()).Scan(&mitHash); err != nil {
		t.Fatal(err)
	}
	cs.RetryRegistrationRecoveries()
	var n int
	cs.db.QueryRow(`SELECT attempt_count FROM registration_recovery WHERE id=$1`, mitHash).Scan(&n)
	if n != 1 {
		t.Fatalf("Zeile mit Hash im ersten Durchlauf %d-mal versucht, erwartet 1", n)
	}
}

// INFO-7: ist die Tabelle nicht lesbar, meldet die Zaehlung -1, nicht 0.
func TestRecovery_ZaehlungMeldetLesefehler_RealDB(t *testing.T) {
	cs := recoveryTestKnoten(t, "unused-recovery-zaehlung-test.json")
	if n := cs.CountUnrecoveredRegistrations(); n != 0 {
		t.Fatalf("Vorbedingung: leer (%d)", n)
	}
	cs.db.Close()
	if n := cs.CountUnrecoveredRegistrations(); n != -1 {
		t.Fatalf("Zaehlung bei Lesefehler %d, erwartet -1", n)
	}
}

// INFO-3: CLEAR_REGISTRATIONS und RESET_DB_STATE leeren registration_recovery.
func TestRecovery_LoeschwegeLeerenDieTabelle(t *testing.T) {
	if b := functionBodyFromSource(t, "state.go", "func (cs *ChainState) clearRegistrationsFromDB("); !strings.Contains(b, "`DELETE FROM registration_recovery`") {
		t.Fatal("clearRegistrationsFromDB leert registration_recovery nicht -- die Wiederholung registrierte geloeschte Menschen nach")
	}
	if b := functionBodyFromSource(t, "state.go", "func (cs *ChainState) resetDBStateForBootstrap("); !strings.Contains(b, `"registration_recovery",`) {
		t.Fatal("resetDBStateForBootstrap leert registration_recovery nicht")
	}
}

// INFO-4: mit Datenbank keine EVM-Transaktion ohne vorgemerkten Intent --
// registerOnV7 legt ihn ueber intentVorEVM an und kehrt bei Fehler vorher um.
func TestRegister_OhneIntentKeineEVMTransaktion(t *testing.T) {
	b := functionBodyFromSource(t, "register.go", "func (a *APIServer) registerOnV7(")
	intent := strings.Index(b, "a.state.intentVorEVM(")
	abbruch := strings.Index(b, "if intentErr != nil {\n\t\treturn \"\", intentErr")
	senden := strings.Index(b, "evmRPC.sendRawTransaction(")
	if intent < 0 || abbruch < 0 || senden < 0 || !(intent < abbruch && abbruch < senden) {
		t.Fatalf("registerOnV7: Intent %d, Abbruch %d, Senden %d -- der Intent muss vor der EVM-Transaktion stehen und sein Fehler abbrechen", intent, abbruch, senden)
	}
}

// Pruefung #335, INFO-3 (M1): scheitert das Schreiben des Intents mit
// Datenbank, lehnt intentVorEVM ab; ohne Datenbank bleibt es beim Hinweis.
func TestRegister_IntentFehlerLehntAb_RealDB(t *testing.T) {
	cs := recoveryTestKnoten(t, "unused-recovery-intent-test.json")
	for _, q := range []string{
		`CREATE OR REPLACE FUNCTION aeq_test_intent_fehler() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'Platte voll (Test)'; END $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS aeq_test_intent_fehler ON registration_recovery`,
		`CREATE TRIGGER aeq_test_intent_fehler BEFORE INSERT ON registration_recovery FOR EACH ROW EXECUTE FUNCTION aeq_test_intent_fehler()`,
	} {
		if _, err := cs.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	weg := func() {
		cs.db.Exec(`DROP TRIGGER IF EXISTS aeq_test_intent_fehler ON registration_recovery`)
		cs.db.Exec(`DROP FUNCTION IF EXISTS aeq_test_intent_fehler()`)
	}
	t.Cleanup(weg)
	w := distTestAddr(2395)
	if id, err := cs.intentVorEVM(w, "", Transaction{Type: "register_human", Wallet: w}); err == nil || id != 0 {
		t.Fatalf("Intent scheitert, trotzdem weiter: id=%d err=%v", id, err)
	}
	weg()
	if id, err := cs.intentVorEVM(w, "", Transaction{Type: "register_human", Wallet: w}); err != nil || id <= 0 {
		t.Fatalf("Intent geschrieben: id=%d err=%v", id, err)
	}
	ohneDB := &ChainState{}
	if id, err := ohneDB.intentVorEVM(w, "", Transaction{}); err != nil || id != 0 {
		t.Fatalf("ohne Datenbank: id=%d err=%v, erwartet 0 und kein Fehler", id, err)
	}
}

// Pruefung #335, INFO-3 (M2, M3): -1 (nicht lesbar) heisst weder "nichts
// offen" noch "gesund".
func TestRecovery_LesefehlerIstKeinGutfall(t *testing.T) {
	if recoveryNichtsOffen(-1) || !recoveryNichtsOffen(0) || recoveryNichtsOffen(2) {
		t.Fatal("recoveryNichtsOffen: nur eine gelesene Null ist nichts offen")
	}
	if w, h := registrationRecoveryHinweis(-1); !w || !strings.Contains(h, "not readable") {
		t.Fatalf("-1: warnen=%v %q", w, h)
	}
	if w, _ := registrationRecoveryHinweis(0); w {
		t.Fatal("0 offen warnt")
	}
	if w, h := registrationRecoveryHinweis(3); !w || !strings.Contains(h, "3 registration(s)") {
		t.Fatalf("3 offen: warnen=%v %q", w, h)
	}
}

// Der Degraded-Grund faellt nur ueber recoveryNichtsOffen (M2 an der
// Aufrufstelle).
func TestRecovery_DegradedNurBeiGelesenerNull(t *testing.T) {
	b := functionBodyFromSource(t, "evm_storage.go", "func (cs *ChainState) RetryRegistrationRecoveries(")
	if !strings.Contains(b, "recoveryNichtsOffen(cs.CountUnrecoveredRegistrations())") {
		t.Fatal("RetryRegistrationRecoveries loescht den Degraded-Grund nicht ueber recoveryNichtsOffen -- ein Lesefehler (-1) liesse ihn verschwinden")
	}
}
