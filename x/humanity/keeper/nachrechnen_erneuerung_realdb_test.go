package keeper

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
)

// liveness_renewal traegt die Bescheinigung des Coordinators, und jeder
// Knoten prueft sie selbst (nachrechnen_erneuerung.go). Das
// Coordinator-Register steht in der Datenbank (coordinator_keys), also gegen
// eine echte.

type erneuerungsFall struct {
	t      *testing.T
	cs     *ChainState
	dag    *BlockDAG
	pub    ed25519.PublicKey
	priv   ed25519.PrivateKey
	wallet string
	jetzt  int64
	n      int
}

func neuerErneuerungsFall(t *testing.T, registriertVorTagen int64) *erneuerungsFall {
	t.Helper()
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-erneuerung-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(0) })
	cs.EnsureCoordinatorRegistry()
	if _, err := cs.db.Exec(`DELETE FROM coordinator_keys`); err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cs.db.Exec(`INSERT INTO coordinator_keys (public_key, human_wallet) VALUES ($1, $2)`,
		hex.EncodeToString(pub), distTestAddr(1901)); err != nil {
		t.Fatal(err)
	}
	f := &erneuerungsFall{t: t, cs: cs, pub: pub, priv: priv, wallet: distTestAddr(1900), jetzt: nowUnix()}
	// Gestaffeltes Konto: GrantStagedUntil = Registrierung + 30 Tage.
	reg := f.jetzt - registriertVorTagen*86400
	cs.mu.Lock()
	acc := &AccountState{Address: f.wallet, IsHuman: true, Balance: NewDecimal(200), GrantStagedRest: NewDecimal(800),
		GrantStagedUntil: reg + grantStaffelTage*86400}
	cs.accounts.Set(f.wallet, acc)
	err = cs.saveAccountToDB(acc)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	dag := newOrphanTestDAG()
	dag.state = cs
	dag.bootHeight = 0
	dag.replayedBlocks = make(map[string]bool)
	dag.replayFailures = make(map[string]replayFailureState)
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}
	f.dag = dag
	return f
}

func (f *erneuerungsFall) unterschreibe(priv ed25519.PrivateKey, wallet string, issuedAt int64) string {
	return hex.EncodeToString(ed25519.Sign(priv, []byte(fmt.Sprintf("%s|%s|%d", livenessRenewalDomain, wallet, issuedAt))))
}

func (f *erneuerungsFall) gueltig(issuedAt int64) Transaction {
	return erneuerungsTransaktion(f.wallet, issuedAt, hex.EncodeToString(f.pub), f.unterschreibe(f.priv, f.wallet, issuedAt))
}

// pruefe: nur nachrechnen, Zaehler-Differenz zurueck.
func (f *erneuerungsFall) pruefe(tx Transaction, blockZeit int64) map[string]int64 {
	f.t.Helper()
	regeln := []string{"erneuerung_ohne_bescheinigung", "erneuerung_zeit", "erneuerung_zu_frueh"}
	vorher := map[string]int64{}
	for _, r := range regeln {
		vorher[r] = erhaltungZaehler(r)
	}
	f.cs.mu.Lock()
	err := f.cs.nachrechnenTxLocked(&tx, blockZeit)
	f.cs.mu.Unlock()
	if err != nil {
		f.t.Fatalf("im Beobachtungsmodus darf nichts abgelehnt werden: %v", err)
	}
	neu := map[string]int64{}
	for _, r := range regeln {
		if d := erhaltungZaehler(r) - vorher[r]; d != 0 {
			neu[r] = d
		}
	}
	return neu
}

func TestErneuerung_GueltigeBescheinigung_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	tx := f.gueltig(f.jetzt - 60)
	if tx.Bescheinigung == nil || tx.Bescheinigung.PublicKey != hex.EncodeToString(f.pub) {
		t.Fatalf("die Transaktion traegt die Bescheinigung nicht: %+v", tx)
	}
	if got := f.pruefe(tx, f.jetzt); len(got) != 0 {
		t.Fatalf("gueltige Erneuerung gemeldet: %v", got)
	}
}

// Missbrauch: ohne Bescheinigung (wie bisher), fremder Schluessel, fremde
// Wallet, veraenderter Zeitpunkt, alte Bescheinigung, Tag 3 statt Tag 7.
func TestErneuerung_Missbrauch_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	issued := f.jetzt - 60

	ohne := Transaction{Type: "liveness_renewal", Wallet: f.wallet, DistributionAt: issued}
	if got := f.pruefe(ohne, f.jetzt); got["erneuerung_ohne_bescheinigung"] != 1 {
		t.Fatalf("ohne Bescheinigung nicht erkannt: %v", got)
	}

	_, fremdPriv, _ := ed25519.GenerateKey(rand.Reader)
	fremd := erneuerungsTransaktion(f.wallet, issued, hex.EncodeToString(fremdPriv.Public().(ed25519.PublicKey)), f.unterschreibe(fremdPriv, f.wallet, issued))
	if got := f.pruefe(fremd, f.jetzt); got["erneuerung_ohne_bescheinigung"] != 1 {
		t.Fatalf("Schluessel ausserhalb des Registers nicht erkannt: %v", got)
	}

	andereWallet := erneuerungsTransaktion(f.wallet, issued, hex.EncodeToString(f.pub), f.unterschreibe(f.priv, distTestAddr(1999), issued))
	if got := f.pruefe(andereWallet, f.jetzt); got["erneuerung_ohne_bescheinigung"] != 1 {
		t.Fatalf("Bescheinigung fuer eine andere Wallet nicht erkannt: %v", got)
	}

	verschoben := f.gueltig(issued)
	verschoben.DistributionAt = issued + 1
	if got := f.pruefe(verschoben, f.jetzt); got["erneuerung_ohne_bescheinigung"] != 1 {
		t.Fatalf("veraenderter Zeitpunkt nicht erkannt: %v", got)
	}

	alt := f.gueltig(f.jetzt - 2*3600)
	if got := f.pruefe(alt, f.jetzt); got["erneuerung_zeit"] != 1 {
		t.Fatalf("wiederverwendete alte Bescheinigung nicht erkannt: %v", got)
	}

	voraus := f.gueltig(f.jetzt + 3600)
	if got := f.pruefe(voraus, f.jetzt); got["erneuerung_zeit"] != 1 {
		t.Fatalf("Bescheinigung aus der Zukunft nicht erkannt: %v", got)
	}

	frueh := neuerErneuerungsFall(t, 3)
	if got := frueh.pruefe(frueh.gueltig(frueh.jetzt-60), frueh.jetzt); got["erneuerung_zu_frueh"] != 1 {
		t.Fatalf("Erneuerung an Tag 3 nicht erkannt: %v", got)
	}
}

// Vor der Aktivierung ist liveness_renewal Leerlauf und wird nicht geprueft
// -- bestehende Bloecke bleiben, wie sie sind.
func TestErneuerung_VorDerAktivierungUngeprueft_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	stagedGrantActivationOverride.Store(f.jetzt + 86400)
	ohne := Transaction{Type: "liveness_renewal", Wallet: f.wallet, DistributionAt: f.jetzt}
	if got := f.pruefe(ohne, f.jetzt); len(got) != 0 {
		t.Fatalf("vor der Aktivierung gemeldet: %v", got)
	}
}

// Im strengen Modus lehnt jeder Knoten den Block mit einer erfundenen
// Erneuerung ab, und die Staffel bleibt gesperrt; die echte geht durch.
func TestErneuerung_StrengLehntErfundeneAb_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	nachrechnenStrengOverride.Store(1)
	t.Cleanup(func() { nachrechnenStrengOverride.Store(0) })
	block := func(tx Transaction) bool {
		f.n++
		b := &Block{Height: int64(f.n), Hash: fmt.Sprintf("erneuerung-%s-%d", t.Name(), f.n), Timestamp: f.jetzt, Transactions: []Transaction{tx}}
		return f.dag.replayTransactions(b, true)
	}
	erneuert := func() int64 {
		f.cs.mu.Lock()
		defer f.cs.mu.Unlock()
		acc, _ := f.cs.accounts.Get(f.wallet)
		return acc.LivenessRenewedAt
	}
	if block(Transaction{Type: "liveness_renewal", Wallet: f.wallet, DistributionAt: f.jetzt}) {
		t.Fatal("Block mit erfundener Erneuerung angenommen")
	}
	if got := erneuert(); got != 0 {
		t.Fatalf("erfundene Erneuerung hat die Staffel freigeschaltet (%d)", got)
	}
	if !block(f.gueltig(f.jetzt - 60)) {
		t.Fatal("Block mit echter Erneuerung abgelehnt")
	}
	if got := erneuert(); got != f.jetzt {
		t.Fatalf("echte Erneuerung nicht angewendet (%d)", got)
	}
}
