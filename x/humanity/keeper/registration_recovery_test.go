package keeper

import (
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// Pruefung von #329, INFO-13: ein liegengebliebener Vor-EVM-Intent
// (evm_tx_hash leer) registrierte in RetryRegistrationRecoveries den Menschen
// lokal per RegisterHumanAtomic -- ohne dass der Vertrag den Groth16-Beweis
// je geprueft hatte. Auf einem Folger reichte dafuer ein Intent, dessen
// Loeschen nach der Ablehnung an der EVM-Uebergabe scheiterte.

func recoveryTestKnoten(t *testing.T, datei string) *ChainState {
	t.Helper()
	skipUnlessRealDBBenchEnv(t)
	truncateDistTestTables(t) // Menschen eines frueheren Laufs
	cs := testKnoten(t, datei)
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	if _, err := cs.db.Exec(`DELETE FROM registration_recovery`); err != nil {
		t.Fatal(err)
	}
	return cs
}

// vorEVMIntentAnlegen legt einen Intent mit vollstaendigen (gefaelschten)
// Beweisdaten an und datiert ihn um alterSek zurueck.
func vorEVMIntentAnlegen(t *testing.T, cs *ChainState, wallet, nullifier string, alterSek int64) int64 {
	t.Helper()
	tx := Transaction{Type: "register_human", Wallet: wallet, Nullifier: nullifier,
		ProofA: []string{"1", "2"}, ProofC: []string{"3", "4"}, PubSignals: []string{"5", "6"}}
	id, err := cs.SaveRegistrationIntent(wallet, nullifier, tx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cs.db.Exec(`UPDATE registration_recovery SET created_at=$1 WHERE id=$2`, time.Now().Unix()-alterSek, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func recoveryZeile(t *testing.T, cs *ChainState, id int64) (geschlossen bool, letzterFehler string) {
	t.Helper()
	var rec *int64
	var le *string
	if err := cs.db.QueryRow(`SELECT recovered_at, last_error FROM registration_recovery WHERE id=$1`, id).Scan(&rec, &le); err != nil {
		t.Fatal(err)
	}
	if le != nil {
		letzterFehler = *le
	}
	return rec != nil, letzterFehler
}

// spiegelMenschSetzen setzt im EVM-Spiegel isHuman fuer w, ohne den
// Go-Zustand zu beruehren.
func spiegelMenschSetzen(t *testing.T, cs *ChainState, w string) {
	t.Helper()
	slot := mappingSlot(common.HexToAddress(w).Bytes(), spiegelSlotIsHuman()).Hex()
	if err := cs.SaveStorageSlot(V7_CONTRACT_ADDR, slot, common.HexToHash("0x01").Hex()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.db.Exec(`DELETE FROM evm_storage WHERE slot=$1`, slot) })
}

func pendingTxAnzahl(cs *ChainState) int {
	var n int
	cs.db.QueryRow(`SELECT COUNT(*) FROM pending_txs`).Scan(&n)
	return n
}

// Missbrauch: ein alter Vor-EVM-Intent mit Nullifier und einer ohne werden
// verworfen -- kein Mensch, kein Nullifier, nichts im Ausgang.
func TestRecovery_VorEVMIntentRegistriertNie_RealDB(t *testing.T) {
	cs := recoveryTestKnoten(t, "unused-recovery-vorevm-test.json")
	mit, ohne := "0x00000000000000000000000000000000000000e2", "0x00000000000000000000000000000000000000e3"
	idMit := vorEVMIntentAnlegen(t, cs, mit, "0x"+strings.Repeat("e2", 32), vorEVMMindestAlterSek+60)
	idOhne := vorEVMIntentAnlegen(t, cs, ohne, "", vorEVMMindestAlterSek+60)
	vorher := pendingTxAnzahl(cs)

	if n := cs.RetryRegistrationRecoveries(); n != 0 {
		t.Fatalf("%d Vor-EVM-Intents als nachgeholt gezaehlt", n)
	}
	if cs.IsHuman(mit) || cs.IsHuman(ohne) {
		t.Fatalf("Vor-EVM-Intent registriert: %v/%v", cs.IsHuman(mit), cs.IsHuman(ohne))
	}
	if nachher := pendingTxAnzahl(cs); nachher != vorher {
		t.Fatalf("Ausgang %d -> %d", vorher, nachher)
	}
	var nullifierDa bool
	cs.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM nullifiers WHERE nullifier=$1)`, "0x"+strings.Repeat("e2", 32)).Scan(&nullifierDa)
	if nullifierDa {
		t.Fatal("Nullifier eines nie bestaetigten Intents verbraucht")
	}
	for _, id := range []int64{idMit, idOhne} {
		if zu, le := recoveryZeile(t, cs, id); !zu || !strings.Contains(le, "never confirmed") {
			t.Fatalf("Intent %d: geschlossen=%v last_error=%q", id, zu, le)
		}
	}
	// Ein zweiter Durchlauf fasst nichts mehr an.
	if n := cs.RetryRegistrationRecoveries(); n != 0 || cs.IsHuman(mit) || cs.CountUnrecoveredRegistrations() != 0 {
		t.Fatalf("zweiter Durchlauf: %d, Mensch %v, offen %d", n, cs.IsHuman(mit), cs.CountUnrecoveredRegistrations())
	}
}

// Ein junger Intent kann noch unterwegs sein (register.go, Schritt 2): er
// bleibt offen.
func TestRecovery_JungerVorEVMIntentBleibt_RealDB(t *testing.T) {
	cs := recoveryTestKnoten(t, "unused-recovery-jung-test.json")
	w := "0x00000000000000000000000000000000000000e4"
	id := vorEVMIntentAnlegen(t, cs, w, "0x"+strings.Repeat("e4", 32), 5)
	cs.RetryRegistrationRecoveries()
	if zu, _ := recoveryZeile(t, cs, id); zu || cs.IsHuman(w) {
		t.Fatalf("junger Intent: geschlossen=%v Mensch=%v", zu, cs.IsHuman(w))
	}
}

// Junger Intent, Spiegel zeigt den Menschen schon: das ist eine gerade
// bestaetigte Registrierung, deren Hash register.go gleich vermerkt --
// kein Hinweis fuer den Betreiber, keine Fehlermeldung an der Zeile.
func TestRecovery_JungerIntentMitSpiegelKeinAlarm_RealDB(t *testing.T) {
	cs := recoveryTestKnoten(t, "unused-recovery-jung-spiegel-test.json")
	w := "0x00000000000000000000000000000000000000e9"
	spiegelMenschSetzen(t, cs, w)
	id := vorEVMIntentAnlegen(t, cs, w, "0x"+strings.Repeat("e9", 32), 5)
	cs.SetBootstrapDegraded("")
	cs.RetryRegistrationRecoveries()
	if zu, le := recoveryZeile(t, cs, id); zu || le != "" {
		t.Fatalf("junger Intent: geschlossen=%v last_error=%q", zu, le)
	}
	if r := cs.BootstrapDegradedReason(); r != "" {
		t.Fatalf("Fehlalarm fuer eine laufende Registrierung: %q", r)
	}
}

// Wettlauf beim Schliessen: der Durchlauf hat die Zeile noch ohne Hash
// gelesen, register.go hat ihn inzwischen vermerkt. Die Zeile gehoert jetzt
// der Wiederholung nach der EVM und darf weder verworfen noch markiert werden.
func TestRecovery_VeralteteLesungSchliesstBestaetigtenNicht_RealDB(t *testing.T) {
	cs := recoveryTestKnoten(t, "unused-recovery-veraltet-test.json")
	for i, spiegel := range []bool{false, true} {
		w := distTestAddr(2300 + i)
		if spiegel {
			spiegelMenschSetzen(t, cs, w)
		}
		alt := time.Now().Unix() - vorEVMMindestAlterSek - 60
		id := vorEVMIntentAnlegen(t, cs, w, "", vorEVMMindestAlterSek+60)
		if err := cs.UpdateRegistrationIntentEVMTxHash(id, "0x"+strings.Repeat("ea", 32)); err != nil {
			t.Fatal(err)
		}
		if e := cs.vorEVMIntentAufloesen(id, w, alt, time.Now().Unix()); e != vorEVMOffen {
			t.Fatalf("Spiegel=%v: bestaetigter Intent aus veralteter Lesung = %v", spiegel, e)
		}
		if zu, le := recoveryZeile(t, cs, id); zu || le != "" {
			t.Fatalf("Spiegel=%v: geschlossen=%v last_error=%q", spiegel, zu, le)
		}
	}
}

// Hat der Go-Zustand den Menschen schon (Block eines anderen Knotens), wird
// der Intent geschlossen und gezaehlt.
func TestRecovery_VorEVMIntentSchonMensch_RealDB(t *testing.T) {
	cs := recoveryTestKnoten(t, "unused-recovery-schon-test.json")
	w := "0x00000000000000000000000000000000000000e5"
	if err := cs.SaveRegistrationRecovery(w, "0x"+strings.Repeat("e5", 32), "", Transaction{Type: "register_human", Wallet: w}); err != nil {
		t.Fatal(err)
	}
	if n := cs.RetryRegistrationRecoveries(); n != 1 || !cs.IsHuman(w) {
		t.Fatalf("Vorbedingung: Nachholen nach der EVM = %d, Mensch %v", n, cs.IsHuman(w))
	}
	id := vorEVMIntentAnlegen(t, cs, w, "", vorEVMMindestAlterSek+60)
	if n := cs.RetryRegistrationRecoveries(); n != 1 {
		t.Fatalf("schon Mensch: %d geschlossen", n)
	}
	if zu, le := recoveryZeile(t, cs, id); !zu || !strings.Contains(le, "already human") {
		t.Fatalf("geschlossen=%v last_error=%q", zu, le)
	}
}

// Zeigt der EVM-Spiegel den Menschen, Go aber nicht, registriert die
// Wiederholung trotzdem nicht: ein Spiegelplatz ist kein gepruefter Beweis.
// Die Zeile bleibt fuer den Betreiber offen.
func TestRecovery_VorEVMIntentSpiegelMenschBleibtFuerBetreiber_RealDB(t *testing.T) {
	cs := recoveryTestKnoten(t, "unused-recovery-spiegel-test.json")
	w := "0x00000000000000000000000000000000000000e6"
	spiegelMenschSetzen(t, cs, w)
	id := vorEVMIntentAnlegen(t, cs, w, "0x"+strings.Repeat("e6", 32), vorEVMMindestAlterSek+60)
	vorher := pendingTxAnzahl(cs)
	if n := cs.RetryRegistrationRecoveries(); n != 0 || cs.IsHuman(w) || pendingTxAnzahl(cs) != vorher {
		t.Fatalf("Spiegel: %d nachgeholt, Mensch %v", n, cs.IsHuman(w))
	}
	if zu, le := recoveryZeile(t, cs, id); zu || !strings.Contains(le, "manual review") {
		t.Fatalf("geschlossen=%v last_error=%q", zu, le)
	}
	if !strings.Contains(cs.BootstrapDegradedReason(), "registration_recovery") {
		t.Fatalf("kein Hinweis fuer den Betreiber: %q", cs.BootstrapDegradedReason())
	}
}

// Wettlauf: die Wiederholung verwirft einen Intent, waehrend die
// EVM-Uebergabe noch laeuft; der spaete Hash oeffnet ihn wieder, damit ein
// danach scheiterndes RegisterHumanAtomic nachgeholt wird.
func TestRecovery_SpaeterHashOeffnetVerworfenenIntent_RealDB(t *testing.T) {
	cs := recoveryTestKnoten(t, "unused-recovery-spaet-test.json")
	w := "0x00000000000000000000000000000000000000e7"
	id := vorEVMIntentAnlegen(t, cs, w, "", vorEVMMindestAlterSek+60)
	cs.RetryRegistrationRecoveries()
	if zu, _ := recoveryZeile(t, cs, id); !zu {
		t.Fatal("Vorbedingung: Intent verworfen")
	}
	if err := cs.UpdateRegistrationIntentEVMTxHash(id, "0x"+strings.Repeat("e7", 32)); err != nil {
		t.Fatal(err)
	}
	if zu, _ := recoveryZeile(t, cs, id); zu {
		t.Fatal("bestaetigter Intent bleibt geschlossen -- die Wiederholung verloere ihn")
	}
	if n := cs.RetryRegistrationRecoveries(); n != 1 || !cs.IsHuman(w) {
		t.Fatalf("nach der EVM: %d nachgeholt, Mensch %v", n, cs.IsHuman(w))
	}
}

// Pruefung von #322, INFO-2: solange die Annahme pausiert, schreibt die
// Wiederholung nichts in den Ausgang; danach holt sie nach. Auch auf einem
// Knoten, der nicht annimmt (nur lesend, Folger): was er in seinen Ausgang
// legt, waehrend er nicht erzeugt, kommt ebenso in keinen Block.
func TestRecovery_PauseSperrtNachholen_RealDB(t *testing.T) {
	for _, nurLesend := range []bool{false, true} {
		t.Run(map[bool]string{false: "annehmend", true: "nur_lesend"}[nurLesend], func(t *testing.T) {
			pauseSperrtNachholen(t, nurLesend)
		})
	}
}

func pauseSperrtNachholen(t *testing.T, nurLesend bool) {
	cs := recoveryTestKnoten(t, "unused-recovery-pause-test.json")
	w := "0x00000000000000000000000000000000000000e8"
	if err := cs.SaveRegistrationRecovery(w, "0x"+strings.Repeat("e8", 32), "0x"+strings.Repeat("e8", 32), Transaction{Type: "register_human", Wallet: w, Nullifier: "0x" + strings.Repeat("e8", 32)}); err != nil {
		t.Fatal(err)
	}
	cs.nurLesend.Store(nurLesend)
	if cs.nimmtAnFuer() == nurLesend {
		t.Fatalf("Vorbedingung: nimmtAnFuer = %v bei nur_lesend = %v", cs.nimmtAnFuer(), nurLesend)
	}
	cs.erzeugerSeit.Store(time.Now().Unix() - admissionStallLimit() - 1)
	if cs.annahmePauseGrund() == nil {
		t.Fatal("Vorbedingung: Annahme pausiert")
	}
	vorher := pendingTxAnzahl(cs)
	if n := cs.RetryRegistrationRecoveries(); n != 0 || cs.IsHuman(w) || pendingTxAnzahl(cs) != vorher {
		t.Fatalf("pausiert: %d nachgeholt, Mensch %v", n, cs.IsHuman(w))
	}
	cs.erzeugerSeit.Store(0)
	if n := cs.RetryRegistrationRecoveries(); n != 1 || !cs.IsHuman(w) {
		t.Fatalf("Gegenprobe: %d nachgeholt, Mensch %v", n, cs.IsHuman(w))
	}
}

// Fail-closed: ist der EVM-Spiegel nicht lesbar, bleibt der Intent liegen --
// er darf weder verworfen werden (ein bestaetigter ginge verloren) noch
// sonst etwas ausloesen.
func TestRecovery_SpiegelLesefehlerLaesstLiegen_RealDB(t *testing.T) {
	cs := recoveryTestKnoten(t, "unused-recovery-lesefehler-test.json")
	w := "0x00000000000000000000000000000000000000eb"
	id := vorEVMIntentAnlegen(t, cs, w, "", vorEVMMindestAlterSek+60)
	if _, err := cs.db.Exec(`ALTER TABLE evm_storage RENAME TO evm_storage_weg`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := cs.db.Exec(`ALTER TABLE evm_storage_weg RENAME TO evm_storage`); err != nil {
			t.Errorf("evm_storage nicht zurueckbenannt: %v", err)
		}
	})
	if e := cs.vorEVMIntentAufloesen(id, w, time.Now().Unix()-vorEVMMindestAlterSek-60, time.Now().Unix()); e != vorEVMOffen {
		t.Fatalf("Lesefehler am Spiegel: %v", e)
	}
	if zu, le := recoveryZeile(t, cs, id); zu || le != "" {
		t.Fatalf("geschlossen=%v last_error=%q", zu, le)
	}
}

// Grenze: ein Durchlauf liest hoechstens recoveryHoechstensJeDurchlauf
// Zeilen, der Rest bleibt fuer den naechsten.
func TestRecovery_DurchlaufBegrenzt_RealDB(t *testing.T) {
	cs := recoveryTestKnoten(t, "unused-recovery-grenze-test.json")
	n := recoveryHoechstensJeDurchlauf + 5
	if _, err := cs.db.Exec(`INSERT INTO registration_recovery (wallet, evm_tx_hash, nullifier, pending_tx_json, created_at)
		SELECT '0x' || lpad(to_hex(g), 40, '0'), '', '', '', $1 FROM generate_series(1, $2) g`,
		time.Now().Unix()-vorEVMMindestAlterSek-60, n); err != nil {
		t.Fatal(err)
	}
	cs.RetryRegistrationRecoveries()
	if offen := cs.CountUnrecoveredRegistrations(); offen != 5 {
		t.Fatalf("nach einem Durchlauf offen: %d, erwartet 5", offen)
	}
	cs.RetryRegistrationRecoveries()
	if offen := cs.CountUnrecoveredRegistrations(); offen != 0 {
		t.Fatalf("nach zwei Durchlaeufen offen: %d", offen)
	}
}
