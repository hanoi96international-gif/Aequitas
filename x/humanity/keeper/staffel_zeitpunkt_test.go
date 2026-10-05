package keeper

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"
)

// EIN ZEITPUNKT (grant_staffel.go): Annahme und Nachspielen setzen
// GrantStagedUntil und LivenessRenewedAt -- beide im Blatt der StateRoot --
// auf denselben Wert aus der Transaktion, nicht jeder auf seine Uhr.

func TestStaffelRegZeit(t *testing.T) {
	const block = 10_000_000
	if got := staffelRegZeit(0, block); got != block {
		t.Fatalf("ohne RegAt: %d statt Blockzeit", got)
	}
	if got := staffelRegZeit(block-30, block); got != block-30 {
		t.Fatalf("RegAt kurz vor dem Block: %d", got)
	}
	// Rueckdatiert: hoechstens einen Tag vor dem Block.
	if got := staffelRegZeit(block-10*86400, block); got != block-86400 {
		t.Fatalf("rueckdatierte Registrierung nicht begrenzt: %d", got)
	}
	// Vordatiert (V7 prueft RegAt nicht): hoechstens 5 min nach dem Block,
	// sonst der Block -- auch bei MaxInt64 kein Ueberlauf.
	if got := staffelRegZeit(block+v8NachspielKarenz, block); got != block+v8NachspielKarenz {
		t.Fatalf("RegAt knapp nach dem Block: %d", got)
	}
	for _, vor := range []int64{block + v8NachspielKarenz + 1, 4_102_444_800, math.MaxInt64} {
		if got := staffelRegZeit(vor, block); got != block {
			t.Fatalf("vordatierte Registrierung %d nicht auf die Blockzeit gesetzt: %d", vor, got)
		}
	}
}

// Missbrauch: eine vordatierte Registrierung (RegAt = MaxInt64) laeuft nicht
// ueber und haelt die Staffel nicht ewig.
func TestStaffelZeit_VordatiertLaeuftNichtUeber(t *testing.T) {
	stagedGrantActivationOverride.Store(1_000)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	w := "0x00000000000000000000000000000000000000c5"
	blockZeit := int64(5_000_000)
	cs := newTestState()
	cs.mu.Lock()
	err := cs.registerHumanMitZeitenLocked(context.Background(), w, blockZeit, staffelRegZeit(math.MaxInt64, blockZeit), grantKlasseGestaffelt)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if got := acct(cs, w).GrantStagedUntil; got != blockZeit+grantStaffelTage*86400 {
		t.Fatalf("GrantStagedUntil %d nach vordatierter Registrierung", got)
	}
}

// Wiederanlauf: eine Registrierung, die Tage nach ihrer Annahme erneut
// angewendet wird (RetryRegistrationRecoveries), begrenzt der Erzeuger wie
// jeder Nachspielende -- die Staffel laeuft hoechstens einen Tag vor jetzt los.
func TestStaffelZeit_WiederanlaufBegrenztWieNachspielen(t *testing.T) {
	stagedGrantActivationOverride.Store(1_000)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	w := "0x00000000000000000000000000000000000000c6"
	jetzt := nowUnix()
	tx := Transaction{Type: "register_human", Wallet: w, GrantClass: grantKlasseGestaffelt, RegAt: jetzt - 3*86400}
	cs := newTestState()
	if err := cs.RegisterHumanAtomic(w, tx); err != nil {
		t.Fatal(err)
	}
	got := acct(cs, w).GrantStagedUntil
	erwartet := jetzt - 86400 + grantStaffelTage*86400
	if got < erwartet || got > erwartet+5 {
		t.Fatalf("GrantStagedUntil %d nach Wiederanlauf, erwartet etwa %d (ein Tag vor jetzt)", got, erwartet)
	}
}

// V7: auch ohne Vertrag V8 traegt die Registrierung ihren Annahmezeitpunkt.
func TestStaffelZeit_RegistrierungTraegtRegAtAuchAufV7(t *testing.T) {
	src, err := os.ReadFile("register.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "pendingRegTx.RegAt = annahmeZeit\n\tif v8 {") {
		t.Fatal("register.go setzt RegAt nicht mehr fuer jede Registrierung (vor dem V8-Zweig)")
	}
}

// Der Erzeuger nimmt eine gestaffelte Registrierung an (RegisterHumanAtomic,
// mit seiner Uhr spaeter als RegAt), ein anderer Knoten spielt sie Sekunden
// spaeter nach (Blockzeit) -- das Blatt muss gleich sein.
func TestStaffelZeit_AnnahmeUndNachspielenGleichesBlatt(t *testing.T) {
	stagedGrantActivationOverride.Store(1_000)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	w := "0x00000000000000000000000000000000000000c1"
	regAt := nowUnix() - 40 // angenommen vor 40 s
	tx := Transaction{Type: "register_human", Wallet: w, GrantClass: grantKlasseGestaffelt, RegAt: regAt}

	erzeuger := newTestState()
	if err := erzeuger.RegisterHumanAtomic(w, tx); err != nil {
		t.Fatal(err)
	}
	nachspieler := newTestState()
	blockZeit := regAt + 7
	nachspieler.mu.Lock()
	err := nachspieler.registerHumanMitZeitenLocked(context.Background(), w, blockZeit, staffelRegZeit(tx.RegAt, blockZeit), tx.GrantClass)
	nachspieler.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	e, n := acct(erzeuger, w), acct(nachspieler, w)
	if e.GrantStagedUntil != regAt+grantStaffelTage*86400 || n.GrantStagedUntil != e.GrantStagedUntil {
		t.Fatalf("GrantStagedUntil: Erzeuger %d, Nachspielender %d, erwartet %d", e.GrantStagedUntil, n.GrantStagedUntil, regAt+grantStaffelTage*86400)
	}
	if accountLeaf(e) != accountLeaf(n) {
		t.Fatal("Erzeuger und Nachspielender haben verschiedene Blaetter")
	}
}

// Missbrauch: eine rueckdatierte Registrierung (RegAt vor dem Stichtag) darf
// beim Nachspielen nicht den vollen Zuschuss sofort bekommen, wenn der Block
// mehr als einen Tag nach dem Stichtag liegt.
func TestStaffelZeit_RueckdatiertVorDenStichtag(t *testing.T) {
	const ab = 1_000_000
	stagedGrantActivationOverride.Store(ab)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	w := "0x00000000000000000000000000000000000000c2"
	blockZeit := int64(ab + 10*86400)
	cs := newTestState()
	cs.mu.Lock()
	err := cs.registerHumanMitZeitenLocked(context.Background(), w, blockZeit, staffelRegZeit(ab-100, blockZeit), grantKlasseGestaffelt)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	acc := acct(cs, w)
	if acc.Balance.Float() != grantSofortAnteil || acc.GrantStagedRest.Float() != grantStaffelAnteil {
		t.Fatalf("rueckdatierte Registrierung bekam %v sofort / %v gestaffelt", acc.Balance.Float(), acc.GrantStagedRest.Float())
	}
	if acc.GrantStagedUntil != blockZeit-86400+grantStaffelTage*86400 {
		t.Fatalf("Staffel bis %d -- die Rueckdatierung hat sie verkuerzt", acc.GrantStagedUntil)
	}
}

// Erneuerung: der bescheinigte Zeitpunkt zaehlt, und ohne ihn schaltet sie
// nichts frei.
func TestStaffelZeit_ErneuerungNachBescheinigung(t *testing.T) {
	stagedGrantActivationOverride.Store(1_000)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	w := "0x00000000000000000000000000000000000000c3"
	cs := newTestState()
	ctx := context.Background()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if err := cs.registerHumanMitKlasseLocked(ctx, w, 5_000, grantKlasseGestaffelt); err != nil {
		t.Fatal(err)
	}
	if err := cs.applyLivenessRenewalDeltaLocked(ctx, w, 0, 700_100); err != nil || acct(cs, w).LivenessRenewedAt != 0 {
		t.Fatalf("Erneuerung ohne Zeitpunkt hat freigeschaltet: %v %d", err, acct(cs, w).LivenessRenewedAt)
	}
	// Mehr als 5 min nach dem Block: schaltet nichts frei.
	if err := cs.applyLivenessRenewalDeltaLocked(ctx, w, 700_100+nachweisHoechstensVoraus+1, 700_100); err != nil || acct(cs, w).LivenessRenewedAt != 0 {
		t.Fatalf("Erneuerung aus der Zukunft des Blocks hat freigeschaltet: %v %d", err, acct(cs, w).LivenessRenewedAt)
	}
	if err := cs.applyLivenessRenewalDeltaLocked(ctx, w, 700_000, 700_100); err != nil || acct(cs, w).LivenessRenewedAt != 700_000 {
		t.Fatalf("erneuert am %d statt am bescheinigten Zeitpunkt", acct(cs, w).LivenessRenewedAt)
	}
}

// Das Nachspielen nimmt dieselben Zeitpunkte wie die Annahme: RegAt (ueber
// staffelRegZeit) fuer die Registrierung, DistributionAt (issued_at) fuer
// die Erneuerung -- nicht die Blockzeit. Ein Block mit gueltigem
// ZK-Beweis laesst sich hier nicht bauen, also am Quelltext.
func TestStaffelZeit_NachspielenNimmtDieZeitAusDerTransaktion(t *testing.T) {
	body := functionBodyFromSource(t, "block.go", "func (dag *BlockDAG) replayTransactions(")
	for _, muss := range []string{
		"registerHumanMitZeitenLocked(context.Background(), wallet, block.Timestamp,\n\t\t\t\tstaffelRegZeit(tx.RegAt, block.Timestamp), tx.GrantClass)",
		"applyLivenessRenewalDeltaLocked(context.Background(), wallet, tx.DistributionAt, block.Timestamp)",
	} {
		if !strings.Contains(body, muss) {
			t.Fatalf("replayTransactions enthaelt nicht mehr:\n%s", muss)
		}
	}
	if strings.Contains(body, "applyLivenessRenewalDeltaLocked(context.Background(), wallet, block.Timestamp") {
		t.Fatal("die Erneuerung nimmt beim Nachspielen wieder die Blockzeit")
	}
}

// An der Grenze: angenommen kurz VOR dem Stichtag, im Block kurz DANACH.
// Erzeuger und Nachspielender entscheiden beide nach RegAt -- voller
// Zuschuss sofort, bei beiden. Nach der eigenen Uhr bzw. der Blockzeit
// bekaeme der eine 1000 sofort, der andere 200 + 800 Staffel.
func TestStaffelZeit_StichtagZwischenAnnahmeUndBlock(t *testing.T) {
	regAt := nowUnix() - 40
	stagedGrantActivationOverride.Store(regAt + 20)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	w := "0x00000000000000000000000000000000000000c4"
	tx := Transaction{Type: "register_human", Wallet: w, GrantClass: grantKlasseGestaffelt, RegAt: regAt}

	erzeuger := newTestState()
	if err := erzeuger.RegisterHumanAtomic(w, tx); err != nil {
		t.Fatal(err)
	}
	nachspieler := newTestState()
	blockZeit := regAt + 30 // nach dem Stichtag
	nachspieler.mu.Lock()
	err := nachspieler.registerHumanMitZeitenLocked(context.Background(), w, blockZeit, staffelRegZeit(tx.RegAt, blockZeit), tx.GrantClass)
	nachspieler.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	e, n := acct(erzeuger, w), acct(nachspieler, w)
	if e.Balance.Float() != registrationGrant || n.Balance.Float() != registrationGrant || e.GrantStagedRest != 0 || n.GrantStagedRest != 0 {
		t.Fatalf("an der Grenze: Erzeuger %v/%v, Nachspielender %v/%v -- erwartet beide voller Zuschuss",
			e.Balance.Float(), e.GrantStagedRest.Float(), n.Balance.Float(), n.GrantStagedRest.Float())
	}
	if accountLeaf(e) != accountLeaf(n) {
		t.Fatal("Erzeuger und Nachspielender haben verschiedene Blaetter")
	}
}
