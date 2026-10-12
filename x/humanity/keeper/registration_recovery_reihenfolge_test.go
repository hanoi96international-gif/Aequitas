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

// INFO-4: mit Datenbank keine EVM-Transaktion ohne vorgemerkten Intent.
func TestRegister_OhneIntentKeineEVMTransaktion(t *testing.T) {
	b := functionBodyFromSource(t, "register.go", "func (a *APIServer) registerOnV7(")
	ablehnung := strings.Index(b, "registration temporarily unavailable: could not record the registration intent")
	senden := strings.Index(b, "evmRPC.sendRawTransaction(")
	if ablehnung < 0 || senden < 0 || ablehnung > senden {
		t.Fatalf("registerOnV7 lehnt bei fehlgeschlagenem Intent nicht vor der EVM-Transaktion ab (Ablehnung %d, Senden %d)", ablehnung, senden)
	}
}

// Dasselbe fuer Vor-EVM-Intents, die liegen bleiben (hier: unlesbarer
// Nullifier): sie ruecken nach einem Versuch ans Ende, ein juengerer, der
// sich aufloesen laesst, kommt im zweiten Durchlauf dran.
func TestRecovery_LiegendeVorEVMIntentsBlockierenNicht_RealDB(t *testing.T) {
	cs := recoveryTestKnoten(t, "unused-recovery-vorevm-kopf-test.json")
	alt := time.Now().Unix() - 7200
	if _, err := cs.db.Exec(`INSERT INTO registration_recovery (wallet, evm_tx_hash, nullifier, pending_tx_json, created_at)
		SELECT '0x' || lpad(to_hex(g + 9000), 40, '0'), '', 'kein-nullifier', '', $1 FROM generate_series(1, $2) g`,
		alt, recoveryHoechstensJeDurchlauf); err != nil {
		t.Fatal(err)
	}
	id := vorEVMIntentAnlegen(t, cs, distTestAddr(2397), "", vorEVMMindestAlterSek+60)
	cs.RetryRegistrationRecoveries()
	if zu, _ := recoveryZeile(t, cs, id); zu {
		t.Fatal("Vorbedingung: im ersten Durchlauf die aelteren")
	}
	cs.RetryRegistrationRecoveries()
	if zu, _ := recoveryZeile(t, cs, id); !zu {
		t.Fatal("der juengere Vor-EVM-Intent kam auch im zweiten Durchlauf nicht dran -- Kopf-Blockade")
	}
}

// 3. Durchgang von #330, INFO-1: Zeilen mit NULL in nullifier und
// pending_tx_json werden gelesen und versucht, nicht still uebersprungen.
func TestRecovery_NullSpaltenWerdenVersucht_RealDB(t *testing.T) {
	cs := recoveryTestKnoten(t, "unused-recovery-null-test.json")
	var id int64
	if err := cs.db.QueryRow(`INSERT INTO registration_recovery (wallet, evm_tx_hash, nullifier, pending_tx_json, created_at)
		VALUES ($1, '0xnull', NULL, NULL, $2) RETURNING id`, distTestAddr(2396), time.Now().Unix()-3600).Scan(&id); err != nil {
		t.Fatal(err)
	}
	cs.RetryRegistrationRecoveries()
	var n int
	cs.db.QueryRow(`SELECT attempt_count FROM registration_recovery WHERE id=$1`, id).Scan(&n)
	if n != 1 {
		t.Fatalf("Zeile mit NULL-Spalten %d-mal versucht, erwartet 1", n)
	}
}
