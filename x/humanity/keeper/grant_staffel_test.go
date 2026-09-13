package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// Jedes Konto ohne Staffel behaelt EXAKT den Blattwert von vor WP 2 -- das
// ist die Bedingung, unter der die Aenderung schlafend ausgerollt werden
// darf: kein StateRoot, kein account_set_xor aendert sich.
func TestAccountLeafUnveraendertOhneStaffel(t *testing.T) {
	acc := &AccountState{Address: "0xABCDEF0000000000000000000000000000000001", Balance: NewDecimal(1049.286126),
		TUsdBalance: NewDecimal(3), LPShares: NewDecimal(0.5), IsHuman: true, FaucetClaimed: true}
	// Die Formel von vor WP 2, hier bewusst nachgebaut statt aufgerufen.
	alt := "acct:" + strings.ToLower(acc.Address) + ":" + strconv.FormatInt(acc.Balance.Micro(), 10) +
		":" + strconv.FormatInt(acc.TUsdBalance.Micro(), 10) + ":" + strconv.FormatInt(acc.LPShares.Micro(), 10) +
		":h=true:fc=true"
	want := sha256.Sum256([]byte(alt))
	if got := accountLeaf(acc); got != want {
		t.Fatalf("Blatt ohne Staffel hat sich geaendert -- das liesse beide Boxen auseinanderlaufen")
	}
	// Mit Staffel MUSS es sich aendern (sonst waere der Rest kein Konsenszustand).
	acc.GrantStagedRest = NewDecimal(800)
	if got := accountLeaf(acc); got == want {
		t.Fatal("Staffelrest muss ins Blatt eingehen")
	}
}

func TestGrantBeiRegistrierungSchlaeftVorAktivierung(t *testing.T) {
	stagedGrantActivationOverride.Store(0)
	sofort, staffel := grantBeiRegistrierung("gestaffelt", 1_800_000_000)
	if sofort != registrationGrant || staffel != 0 {
		t.Fatalf("vor 2100 muss alles sofort kommen: %v/%v", sofort, staffel)
	}
	stagedGrantActivationOverride.Store(1_000)
	defer stagedGrantActivationOverride.Store(0)
	sofort, staffel = grantBeiRegistrierung("gestaffelt", 2_000)
	if sofort != 200 || staffel != 800 {
		t.Fatalf("aktiv + gestaffelt: %v/%v", sofort, staffel)
	}
	for _, k := range []string{"", "sofort", "SOFORT", "unsinn"} {
		if s, st := grantBeiRegistrierung(k, 2_000); s != registrationGrant || st != 0 {
			t.Fatalf("Klasse %q darf niemanden schlechter stellen: %v/%v", k, s, st)
		}
	}
	if s, st := grantBeiRegistrierung("gestaffelt", 999); s != registrationGrant || st != 0 {
		t.Fatalf("eine Sekunde vor der Aktivierung: %v/%v", s, st)
	}
}

// Der ganze Lebenszyklus im Speicher: Registrierung gestaffelt, keine Freigabe
// ohne Erneuerung, Erneuerung, dann 30 Tagesdurchlaeufe bis der Rest 0 ist --
// und die Geldmenge bleibt die ganze Zeit 1.000.
func TestStaffelLebenszyklus(t *testing.T) {
	stagedGrantActivationOverride.Store(1_000)
	defer stagedGrantActivationOverride.Store(0)
	cs := newTestState()
	ctx := context.Background()
	w := "0x00000000000000000000000000000000000000aa"
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if err := cs.registerHumanMitKlasseLocked(ctx, w, 5_000, "gestaffelt"); err != nil {
		t.Fatal(err)
	}
	acc := acct(cs, w)
	if acc.Balance.Float() != 200 || acc.GrantStagedRest.Float() != 800 || acc.GrantStagedUntil != 5_000+30*86400 {
		t.Fatalf("nach Registrierung: %+v", acc)
	}
	if st := StaffelStand(acc); st == nil || st["laeuft"] != false {
		t.Fatalf("Staffel darf ohne Erneuerung nicht laufen: %v", st)
	}

	// Tagesdurchlauf ohne Erneuerung: nichts.
	txs, err := cs.grantReleasesLocked(ctx, 6_000)
	if err != nil || len(txs) != 0 {
		t.Fatalf("ohne Erneuerung keine Freigabe: %v %v", txs, err)
	}
	// Erneuerung -- zweimal nachgespielt aendert nichts.
	for i := 0; i < 2; i++ {
		if err := cs.applyLivenessRenewalDeltaLocked(ctx, w, 7_000); err != nil {
			t.Fatal(err)
		}
	}
	if acc.LivenessRenewedAt != 7_000 {
		t.Fatalf("erneuert_am: %d", acc.LivenessRenewedAt)
	}
	// 30 Tagesdurchlaeufe.
	summe := 0.0
	tage := 0
	for tag := 1; tag <= 40 && acc.GrantStagedRest > 0; tag++ {
		txs, err := cs.grantReleasesLocked(ctx, 7_000+int64(tag)*86400)
		if err != nil {
			t.Fatal(err)
		}
		if len(txs) != 1 || txs[0].Type != "grant_release" || txs[0].Wallet != w {
			t.Fatalf("Tag %d: %v", tag, txs)
		}
		summe += txs[0].Amount
		tage++
		if acc.Balance.Float()+acc.GrantStagedRest.Float() != 1000 {
			t.Fatalf("Tag %d: Geldmenge %v + %v", tag, acc.Balance.Float(), acc.GrantStagedRest.Float())
		}
	}
	if tage != 30 {
		t.Fatalf("Staffel muss in 30 Tagen durch sein, brauchte %d", tage)
	}
	if acc.Balance.Float() != 1000 || acc.GrantStagedRest != 0 || acc.GrantStagedUntil != 0 {
		t.Fatalf("am Ende: %+v", acc)
	}
	if round6(summe) != 800 {
		t.Fatalf("freigegeben: %v", summe)
	}
	// Eine weitere Freigabe (Replay) ist ein No-op, kein Fehler.
	if err := cs.applyGrantReleaseDeltaLocked(ctx, w, 26.666667, 9_000_000); err != nil || acc.Balance.Float() != 1000 {
		t.Fatalf("Replay nach Ende: %v %v", err, acc.Balance.Float())
	}
}

// Vor der Aktivierung sind liveness_renewal und grant_release Leerlauf --
// auch wenn ein Block sie traegt.
func TestStaffelLeerlaufVorAktivierung(t *testing.T) {
	stagedGrantActivationOverride.Store(0)
	cs := newTestState()
	ctx := context.Background()
	w := "0x00000000000000000000000000000000000000bb"
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if err := cs.registerHumanMitKlasseLocked(ctx, w, 1_800_000_000, "gestaffelt"); err != nil {
		t.Fatal(err)
	}
	acc := acct(cs, w)
	if acc.Balance.Float() != 1000 || acc.GrantStagedRest != 0 {
		t.Fatalf("vor Aktivierung voller Zuschuss: %+v", acc)
	}
	if err := cs.applyLivenessRenewalDeltaLocked(ctx, w, 1_800_000_000); err != nil || acc.LivenessRenewedAt != 0 {
		t.Fatalf("Erneuerung vor Aktivierung: %v %d", err, acc.LivenessRenewedAt)
	}
	if err := cs.applyGrantReleaseDeltaLocked(ctx, w, 26.666667, 1_800_000_000); err != nil || acc.Balance.Float() != 1000 {
		t.Fatalf("Freigabe vor Aktivierung: %v %v", err, acc.Balance.Float())
	}
	if txs, err := cs.grantReleasesLocked(ctx, 1_800_000_000); err != nil || txs != nil {
		t.Fatalf("Tagesdurchlauf vor Aktivierung: %v %v", txs, err)
	}
}

// Snapshot: ohne Staffel keine neuen Schluessel im JSON (alte Snapshots und
// neue bleiben byte-gleich), mit Staffel kommen sie durch die Rundreise.
func TestStaffelImSnapshot(t *testing.T) {
	ohne := &AccountState{Address: "0x1", Balance: NewDecimal(1), IsHuman: true}
	j, _ := json.Marshal(ohne)
	for _, k := range []string{"grant_staged_rest", "grant_staged_until", "liveness_renewed_at"} {
		if strings.Contains(string(j), k) {
			t.Fatalf("leeres Feld %s darf nicht im JSON stehen: %s", k, j)
		}
	}
	mit := &AccountState{Address: "0x2", Balance: NewDecimal(200), IsHuman: true, GrantStagedRest: NewDecimal(800), GrantStagedUntil: 123, LivenessRenewedAt: 77}
	j, _ = json.Marshal(mit)
	var zurueck AccountState
	if err := json.Unmarshal(j, &zurueck); err != nil {
		t.Fatal(err)
	}
	if zurueck.GrantStagedRest != mit.GrantStagedRest || zurueck.GrantStagedUntil != 123 || zurueck.LivenessRenewedAt != 77 {
		t.Fatalf("Rundreise: %+v", zurueck)
	}
	if accountLeaf(&zurueck) != accountLeaf(mit) {
		t.Fatal("Blatt nach Rundreise verschieden")
	}
}

func TestGrantKlasseAusHerkunft(t *testing.T) {
	merkeProveKlasse([]byte(`{"zkNullifier":"0xABC","grantClass":"GESTAFFELT"}`))
	if k := grantKlasseAusHerkunft("0xabc"); k != "gestaffelt" {
		t.Fatalf("Klasse aus Herkunft: %q", k)
	}
	merkeProveKlasse([]byte(`{"zkNullifier":"0xDEF"}`)) // ohne Klasse: nichts gemerkt
	if k := grantKlasseAusHerkunft("0xdef"); k != "" {
		t.Fatalf("ohne Klasse: %q", k)
	}
	if grantStaffelTagesrate() != 26.666667 {
		t.Fatalf("Tagesrate %v", grantStaffelTagesrate())
	}
}
