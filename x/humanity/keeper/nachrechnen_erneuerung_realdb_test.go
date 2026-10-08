package keeper

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
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
	// bindung: der Mensch hinter dem Coordinator-Schluessel und die beiden
	// Unterschriften der Eintragung -- sie gehen in jede Bescheinigung.
	bindung CoordinatorBindung
	// menschSchluessel: der secp256k1-Schluessel des Coordinator-Menschen.
	menschSchluessel *ecdsa.PrivateKey
	jetzt            int64
	n                int
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
	// Der Coordinator gehoert einem registrierten Menschen, der ihn
	// freigegeben hat; der Schluessel weist den Besitz nach.
	mk, mensch := neuerSchluessel(t)
	cs.mu.Lock()
	macc := &AccountState{Address: mensch, IsHuman: true, Balance: NewDecimal(10)}
	cs.accounts.Set(mensch, macc)
	err = cs.saveAccountToDB(macc)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	bindung := CoordinatorBindung{Mensch: mensch,
		MenschSig:     personalSign(t, mk, coordinatorFreigabeNachricht(hex.EncodeToString(pub))),
		SchluesselSig: hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachricht(mensch))))}
	if err := cs.RegisterCoordinatorKey(hex.EncodeToString(pub), mensch, "", bindung.MenschSig, bindung.SchluesselSig, true); err != nil {
		t.Fatal(err)
	}
	// Zugelassen (coordinator_zulassung.go): der Mensch haelt seit 30 Tagen
	// einen Validator-Schluessel im Kettenregister.
	registerLeserOverride.Store(1)
	t.Cleanup(func() { registerLeserOverride.Store(0) })
	if _, err := cs.db.Exec(`DELETE FROM validator_verlauf`); err != nil {
		t.Fatal(err)
	}
	verlaufEintrag(t, cs, mensch, distTestAddr(1950), nowUnix()-30*86400)
	f := &erneuerungsFall{t: t, cs: cs, pub: pub, priv: priv, wallet: distTestAddr(1900), jetzt: nowUnix(), bindung: bindung, menschSchluessel: mk}
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
	return hex.EncodeToString(ed25519.Sign(priv, []byte(erneuerungsNachricht(wallet, issuedAt))))
}

func (f *erneuerungsFall) gueltig(issuedAt int64) Transaction {
	return erneuerungsTransaktion(f.wallet, issuedAt, hex.EncodeToString(f.pub), f.unterschreibe(f.priv, f.wallet, issuedAt), f.bindung)
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
	fremd := erneuerungsTransaktion(f.wallet, issued, hex.EncodeToString(fremdPriv.Public().(ed25519.PublicKey)), f.unterschreibe(fremdPriv, f.wallet, issued), f.bindung)
	if got := f.pruefe(fremd, f.jetzt); got["erneuerung_ohne_bescheinigung"] != 1 {
		t.Fatalf("fremder Schluessel ohne passende Bindung nicht erkannt: %v", got)
	}

	andereWallet := erneuerungsTransaktion(f.wallet, issued, hex.EncodeToString(f.pub), f.unterschreibe(f.priv, distTestAddr(1999), issued), f.bindung)
	if got := f.pruefe(andereWallet, f.jetzt); got["erneuerung_ohne_bescheinigung"] != 1 {
		t.Fatalf("Bescheinigung fuer eine andere Wallet nicht erkannt: %v", got)
	}

	verschoben := f.gueltig(issued)
	verschoben.DistributionAt = issued + 1
	if got := f.pruefe(verschoben, f.jetzt); got["erneuerung_ohne_bescheinigung"] != 1 {
		t.Fatalf("veraenderter Zeitpunkt nicht erkannt: %v", got)
	}

	alt := f.gueltig(f.jetzt - 8*86400)
	if got := f.pruefe(alt, f.jetzt); got["erneuerung_zeit"] != 1 {
		t.Fatalf("Bescheinigung von vor 8 Tagen nicht erkannt: %v", got)
	}
	// Ein Tag im Ausgang des Erzeugers ist kein Fehler.
	if got := f.pruefe(f.gueltig(f.jetzt-86400), f.jetzt); len(got) != 0 {
		t.Fatalf("Bescheinigung von gestern gemeldet: %v", got)
	}
	// Ohne Zeitpunkt greift die Zeitregel, bevor die Bescheinigung etwas
	// kostet (Sicherheitsdurchgang #312, LOW-2) -- genau eine Meldung.
	nullZeit := erneuerungsTransaktion(f.wallet, 0, hex.EncodeToString(f.pub), f.unterschreibe(f.priv, f.wallet, 0), f.bindung)
	if got := f.pruefe(nullZeit, f.jetzt); got["erneuerung_zeit"] != 1 || len(got) != 1 {
		t.Fatalf("Bescheinigung ohne Zeitpunkt nicht erkannt: %v", got)
	}
	// Die Bescheinigung selbst taugte auch nicht: vor registerLeserAb
	// (Zeitpunkt 0) ist kein Coordinator zugelassen (coordinator_zulassung.go).
	if err := bescheinigungPruefen(f.wallet, 0, nullZeit.Bescheinigung, f.cs.coordinatorMenschStand, f.cs.coordinatorZugelassen); err == nil {
		t.Fatal("Bescheinigung ohne Zeitpunkt angenommen")
	}

	voraus := f.gueltig(f.jetzt + 3600)
	if got := f.pruefe(voraus, f.jetzt); got["erneuerung_zeit"] != 1 {
		t.Fatalf("Bescheinigung aus der Zukunft nicht erkannt: %v", got)
	}
	// Missbrauch (#312, LOW-2): eine Flut alter Erneuerungen mit
	// Muell-Unterschrift scheitert an der Zeitregel, vor der Bescheinigung --
	// gemeldet wird nur sie.
	muellAlt := erneuerungsTransaktion(f.wallet, f.jetzt-8*86400, hex.EncodeToString(f.pub), strings.Repeat("ab", 64), f.bindung)
	if got := f.pruefe(muellAlt, f.jetzt); got["erneuerung_zeit"] != 1 || len(got) != 1 {
		t.Fatalf("alte Muell-Erneuerung: %v (erwartet nur erneuerung_zeit)", got)
	}
	muellFrueh := neuerErneuerungsFall(t, 3)
	muellTx := erneuerungsTransaktion(muellFrueh.wallet, muellFrueh.jetzt-60, hex.EncodeToString(muellFrueh.pub), strings.Repeat("ab", 64), muellFrueh.bindung)
	// Mit kaltem Speicher: die Regel laedt das Konto selbst.
	muellFrueh.cs.accounts.Delete(muellFrueh.wallet)
	if got := muellFrueh.pruefe(muellTx, muellFrueh.jetzt); got["erneuerung_zu_frueh"] != 1 || len(got) != 1 {
		t.Fatalf("fruehe Muell-Erneuerung: %v (erwartet nur erneuerung_zu_frueh)", got)
	}
	// Ohne Konto wird die Bescheinigung trotzdem geprueft (#314).
	ohneKonto := distTestAddr(1997)
	ohneKontoTx := erneuerungsTransaktion(ohneKonto, issued, hex.EncodeToString(f.pub), strings.Repeat("ab", 64), f.bindung)
	if got := f.pruefe(ohneKontoTx, f.jetzt); got["erneuerung_ohne_bescheinigung"] != 1 || len(got) != 1 {
		t.Fatalf("Muell-Erneuerung fuer ein Konto, das es nicht gibt: %v", got)
	}

	frueh := neuerErneuerungsFall(t, 3)
	if got := frueh.pruefe(frueh.gueltig(frueh.jetzt-60), frueh.jetzt); got["erneuerung_zu_frueh"] != 1 {
		t.Fatalf("Erneuerung an Tag 3 nicht erkannt: %v", got)
	}
	// An Tag 3 ausgestellt, an Tag 8 in einen Block gelegt: zaehlt nicht.
	spaet := neuerErneuerungsFall(t, 8)
	if got := spaet.pruefe(spaet.gueltig(spaet.jetzt-5*86400), spaet.jetzt); got["erneuerung_zu_frueh"] != 1 {
		t.Fatalf("an Tag 3 ausgestellte Bescheinigung an Tag 8 nicht erkannt: %v", got)
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
	// Der bescheinigte Zeitpunkt, nicht die Blockzeit (grant_staffel.go,
	// "EIN ZEITPUNKT") -- derselbe, den der annehmende Knoten setzt.
	if got := erneuert(); got != f.jetzt-60 {
		t.Fatalf("echte Erneuerung nicht mit dem bescheinigten Zeitpunkt angewendet (%d)", got)
	}
}

// Missbrauch (zweiter Sicherheitsdurchgang zu #295): vor der Aktivierung
// setzt ein Block mit einer Erneuerung, deren issued_at NACH dem Stichtag
// liegt, nichts -- ob die Staffel gilt, entscheidet die Blockzeit.
func TestErneuerung_VorDerAktivierungMitZukunftsZeitpunktLeerlauf_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	// Stichtag in 100 s, issued_at in 200 s -- innerhalb der 5 min, die ein
	// Zeitpunkt nach dem Block liegen darf. Nur die Blockzeit (jetzt, vor dem
	// Stichtag) haelt die Erneuerung auf.
	stagedGrantActivationOverride.Store(f.jetzt + 100)
	tx := f.gueltig(f.jetzt + 200)
	b := &Block{Height: 1, Hash: "erneuerung-zukunft-" + t.Name(), Timestamp: f.jetzt, Transactions: []Transaction{tx}}
	f.dag.replayTransactions(b, true)
	f.cs.mu.Lock()
	acc, _ := f.cs.accounts.Get(f.wallet)
	got := acc.LivenessRenewedAt
	f.cs.mu.Unlock()
	if got != 0 {
		t.Fatalf("Erneuerung vor der Aktivierung gesetzt (%d) -- die Staffel schlaeft nicht mehr", got)
	}
}

// Die Annahme prueft Tag 7 auch fuer den Zeitpunkt der Bescheinigung: eine
// an Tag 6 ausgestellte, an Tag 7 eingereichte wird abgewiesen -- jeder
// Nachspielende wiese sie im strengen Modus ab (erneuerung_zu_frueh).
func TestErneuerung_AnnahmeTagSiebenAuchFuerDieBescheinigung_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 7) // Tag 7 beginnt genau jetzt
	a := &APIServer{state: f.cs}
	reiche := func(issued int64) int {
		body, _ := json.Marshal(map[string]interface{}{
			"wallet": f.wallet, "issued_at": issued,
			"public_key": hex.EncodeToString(f.pub), "signature": f.unterschreibe(f.priv, f.wallet, issued),
		})
		w := httptest.NewRecorder()
		a.handleLivenessRenewal(w, httptest.NewRequest(http.MethodPost, "/api/liveness-renewal", bytes.NewReader(body)))
		return w.Code
	}
	if code := reiche(f.jetzt - 600); code != http.StatusConflict {
		t.Fatalf("an Tag 6 ausgestellte Bescheinigung angenommen (%d)", code)
	}
	if code := reiche(nowUnix()); code != http.StatusOK {
		t.Fatalf("an Tag 7 ausgestellte Bescheinigung abgewiesen (%d)", code)
	}
}

// Die Bescheinigung traegt ihre Bindung -- jeder Knoten prueft sie selbst,
// ohne Coordinator-Register. Missbrauch: Bindung fehlt (alte Form, obwohl der
// Schluessel im lokalen Register steht), Besitznachweis fuer einen anderen
// Menschen, Freigabe von einem anderen Schluessel, Mensch nicht registriert,
// der Coordinator bescheinigt sich selbst.
func TestErneuerung_BindungInDerBescheinigung_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	issued := f.jetzt - 60
	if got := f.pruefe(f.gueltig(issued), f.jetzt); len(got) != 0 {
		t.Fatalf("gueltige Erneuerung gemeldet: %v", got)
	}
	// Das lokale Register entscheidet nicht mehr: leer -- gleiches Urteil.
	if _, err := f.cs.db.Exec(`DELETE FROM coordinator_keys`); err != nil {
		t.Fatal(err)
	}
	if got := f.pruefe(f.gueltig(issued), f.jetzt); len(got) != 0 {
		t.Fatalf("ohne lokales Register anders beurteilt: %v", got)
	}
	altForm := f.gueltig(issued)
	altForm.Bescheinigung.Mensch, altForm.Bescheinigung.MenschSig, altForm.Bescheinigung.SchluesselSig = "", "", ""
	k2, m2 := neuerSchluessel(t)
	fremderBesitz := f.gueltig(issued)
	fremderBesitz.Bescheinigung.SchluesselSig = hex.EncodeToString(ed25519.Sign(f.priv, []byte(coordinatorBesitzNachricht(m2))))
	fremdeFreigabe := f.gueltig(issued)
	fremdeFreigabe.Bescheinigung.MenschSig = personalSign(t, k2, coordinatorFreigabeNachricht(hex.EncodeToString(f.pub)))
	// Ein Mensch, den es auf der Kette nicht gibt, mit gueltigen Unterschriften.
	keinMensch := erneuerungsTransaktion(f.wallet, issued, hex.EncodeToString(f.pub), f.unterschreibe(f.priv, f.wallet, issued),
		CoordinatorBindung{Mensch: m2, MenschSig: personalSign(t, k2, coordinatorFreigabeNachricht(hex.EncodeToString(f.pub))),
			SchluesselSig: hex.EncodeToString(ed25519.Sign(f.priv, []byte(coordinatorBesitzNachricht(m2))))})
	for name, tx := range map[string]Transaction{
		"alte Form": altForm, "fremder Besitz": fremderBesitz, "fremde Freigabe": fremdeFreigabe, "kein Mensch": keinMensch,
	} {
		if got := f.pruefe(tx, f.jetzt); got["erneuerung_ohne_bescheinigung"] != 1 {
			t.Fatalf("%s nicht erkannt: %v", name, got)
		}
	}
	// Der Coordinator-Mensch erneuert sich selbst.
	selbst := neuerErneuerungsFall(t, 10)
	selbst.cs.mu.Lock()
	acc := &AccountState{Address: selbst.bindung.Mensch, IsHuman: true, Balance: NewDecimal(200), GrantStagedRest: NewDecimal(800),
		GrantStagedUntil: selbst.jetzt - 10*86400 + grantStaffelTage*86400}
	selbst.cs.accounts.Set(acc.Address, acc)
	err := selbst.cs.saveAccountToDB(acc)
	selbst.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	eigen := erneuerungsTransaktion(selbst.bindung.Mensch, issued, hex.EncodeToString(selbst.pub),
		selbst.unterschreibe(selbst.priv, selbst.bindung.Mensch, issued), selbst.bindung)
	if got := selbst.pruefe(eigen, selbst.jetzt); got["erneuerung_ohne_bescheinigung"] != 1 {
		t.Fatalf("Selbstbescheinigung nicht erkannt: %v", got)
	}
}

// Die Annahme: Bindung aus dem lokalen Register in die Transaktion; ein
// Eintrag ohne Unterschriften (vor dem 06.10.2026) reicht nicht; ein Folger
// nimmt nicht an.
func TestErneuerung_AnnahmeMitBindung_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	a := &APIServer{state: f.cs}
	reiche := func(issued int64) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]interface{}{
			"wallet": f.wallet, "issued_at": issued,
			"public_key": hex.EncodeToString(f.pub), "signature": f.unterschreibe(f.priv, f.wallet, issued),
		})
		w := httptest.NewRecorder()
		a.handleLivenessRenewal(w, httptest.NewRequest(http.MethodPost, "/api/liveness-renewal", bytes.NewReader(body)))
		return w
	}
	if _, err := f.cs.db.Exec(`UPDATE coordinator_keys SET human_signature = NULL`); err != nil {
		t.Fatal(err)
	}
	if w := reiche(nowUnix()); w.Code != http.StatusForbidden || !bytes.Contains(w.Body.Bytes(), []byte("register the coordinator key again")) {
		t.Fatalf("Eintrag ohne Unterschriften: %d %s", w.Code, w.Body.String())
	}
	if err := f.cs.RegisterCoordinatorKey(hex.EncodeToString(f.pub), f.bindung.Mensch, "", f.bindung.MenschSig, f.bindung.SchluesselSig, true); err != nil {
		t.Fatal(err)
	}
	f.cs.leitung.Store(&Leitung{}) // Folger
	if w := reiche(nowUnix()); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("Folger hat angenommen: %d %s", w.Code, w.Body.String())
	}
	f.cs.leitung.Store(nil)
	if w := reiche(nowUnix()); w.Code != http.StatusOK {
		t.Fatalf("gueltige Erneuerung abgewiesen: %d %s", w.Code, w.Body.String())
	}
	var tj string
	if err := f.cs.db.QueryRow(`SELECT tx_json FROM pending_txs WHERE included_at = 0 ORDER BY id DESC LIMIT 1`).Scan(&tj); err != nil {
		t.Fatal(err)
	}
	var tx Transaction
	if err := json.Unmarshal([]byte(tj), &tx); err != nil {
		t.Fatal(err)
	}
	if tx.Bescheinigung == nil || tx.Bescheinigung.Mensch != f.bindung.Mensch || tx.Bescheinigung.MenschSig == "" || tx.Bescheinigung.SchluesselSig == "" {
		t.Fatalf("Ausgang traegt die Bindung nicht: %+v", tx.Bescheinigung)
	}
	// Was im Ausgang liegt, besteht die Pruefung jedes Knotens -- auch ohne
	// dessen Register.
	f.cs.db.Exec(`DELETE FROM coordinator_keys`)
	if got := f.pruefe(tx, f.jetzt); len(got) != 0 {
		t.Fatalf("ausgesandte Erneuerung beim Nachrechnen gemeldet: %v", got)
	}
}

// Die Eintragung ueber HTTP speichert beide Unterschriften -- sonst fehlte
// der annehmenden Stelle spaeter die Bindung fuer die Bescheinigung.
func TestCoordinatorEintragung_SpeichertUnterschriften_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)
	k, mensch := neuerSchluessel(t)
	f.cs.mu.Lock()
	acc := &AccountState{Address: mensch, IsHuman: true, Balance: NewDecimal(10)}
	f.cs.accounts.Set(mensch, acc)
	err := f.cs.saveAccountToDB(acc)
	f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	verlaufEintrag(t, f.cs, mensch, distTestAddr(1952), nowUnix()-30*86400) // als Validator zugelassen
	body, _ := json.Marshal(map[string]string{
		"public_key": pubHex, "human_wallet": mensch,
		"human_signature": personalSign(t, k, coordinatorFreigabeNachricht(pubHex)),
		"key_signature":   hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachricht(mensch)))),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/register-coordinator-key", bytes.NewReader(body))
	req.Header.Set("X-Aequitas-Forwarded", "1") // nicht weiterreichen
	w := httptest.NewRecorder()
	(&APIServer{state: f.cs}).handleRegisterCoordinatorKey(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Eintragung: %d %s", w.Code, w.Body.String())
	}
	b, ok := f.cs.CoordinatorBindungLokal(pubHex)
	if !ok || b.Mensch != mensch {
		t.Fatalf("Bindung nicht gespeichert: %+v %v", b, ok)
	}
	tx := erneuerungsTransaktion(f.wallet, f.jetzt-60, pubHex,
		hex.EncodeToString(ed25519.Sign(priv, []byte(erneuerungsNachricht(f.wallet, f.jetzt-60)))), b)
	if got := f.pruefe(tx, f.jetzt); len(got) != 0 {
		t.Fatalf("Bescheinigung mit gespeicherter Bindung gemeldet: %v", got)
	}
}

// eintragen: die Eintragung ueber HTTP (ohne Weiterreichen).
func (f *erneuerungsFall) eintragen(body map[string]string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/register-coordinator-key", bytes.NewReader(b))
	req.Header.Set("X-Aequitas-Forwarded", "1")
	w := httptest.NewRecorder()
	(&APIServer{state: f.cs}).handleRegisterCoordinatorKey(w, req)
	return w
}

// neuerMensch: ein registrierter Mensch (mit oder ohne offene Staffel).
func (f *erneuerungsFall) neuerMensch(staffelOffen bool) (*ecdsa.PrivateKey, string) {
	f.t.Helper()
	k, m := neuerSchluessel(f.t)
	acc := &AccountState{Address: m, IsHuman: true, Balance: NewDecimal(10)}
	if staffelOffen {
		acc.GrantStagedRest, acc.GrantStagedUntil = NewDecimal(800), f.jetzt+20*86400
	}
	f.cs.mu.Lock()
	f.cs.accounts.Set(m, acc)
	err := f.cs.saveAccountToDB(acc)
	f.cs.mu.Unlock()
	if err != nil {
		f.t.Fatal(err)
	}
	return k, m
}

// Missbrauch (Sicherheitspruefung #300, MEDIUM-2): der Schluessel kleiner
// Ordnung -- das neutrale Element. Mit ihm gilt eine Unterschrift fuer jede
// Nachricht; ein Mensch koennte damit jede Erneuerung bescheinigen. Weder die
// Eintragung noch die Bescheinigung nimmt ihn.
func TestErneuerung_SchluesselKleinerOrdnung_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	k, mensch := f.neuerMensch(false)
	freigabe := personalSign(t, k, coordinatorFreigabeNachricht(ed25519NeutralSchluessel))
	if w := f.eintragen(map[string]string{"public_key": ed25519NeutralSchluessel, "human_wallet": mensch,
		"human_signature": freigabe, "key_signature": ed25519NeutralUnterschr}); w.Code != http.StatusBadRequest {
		t.Fatalf("Eintragung mit Schluessel kleiner Ordnung: %d %s", w.Code, w.Body.String())
	}
	if _, ok := f.cs.CoordinatorBindungLokal(ed25519NeutralSchluessel); ok {
		t.Fatal("Schluessel kleiner Ordnung steht im Register")
	}
	issued := f.jetzt - 60
	tx := erneuerungsTransaktion(f.wallet, issued, ed25519NeutralSchluessel, ed25519NeutralUnterschr,
		CoordinatorBindung{Mensch: mensch, MenschSig: freigabe, SchluesselSig: ed25519NeutralUnterschr})
	if got := f.pruefe(tx, f.jetzt); got["erneuerung_ohne_bescheinigung"] != 1 {
		t.Fatalf("Bescheinigung mit Universalunterschrift nicht erkannt: %v", got)
	}
}

// HIGH-1 (Mindestschutz): ein Mensch mit offener Staffel bescheinigt
// nicht -- sonst bescheinigte eine frische Kunstfigur der naechsten. Beim
// Nachrechnen wie bei der Annahme.
func TestErneuerung_CoordinatorMitOffenerStaffel_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	issued := f.jetzt - 60
	if got := f.pruefe(f.gueltig(issued), f.jetzt); len(got) != 0 {
		t.Fatalf("gueltige Erneuerung gemeldet: %v", got)
	}
	f.cs.mu.Lock()
	macc, _ := f.cs.accounts.Get(f.bindung.Mensch)
	macc.GrantStagedRest, macc.GrantStagedUntil = NewDecimal(800), f.jetzt+20*86400
	err := f.cs.saveAccountToDB(macc)
	f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if got := f.pruefe(f.gueltig(issued), f.jetzt); got["erneuerung_ohne_bescheinigung"] != 1 {
		t.Fatalf("Coordinator mit offener Staffel nicht erkannt: %v", got)
	}
	body, _ := json.Marshal(map[string]interface{}{
		"wallet": f.wallet, "issued_at": nowUnix(),
		"public_key": hex.EncodeToString(f.pub), "signature": f.unterschreibe(f.priv, f.wallet, nowUnix()),
	})
	w := httptest.NewRecorder()
	(&APIServer{state: f.cs}).handleLivenessRenewal(w, httptest.NewRequest(http.MethodPost, "/api/liveness-renewal", bytes.NewReader(body)))
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "offene Staffel") {
		t.Fatalf("Annahme mit Coordinator in offener Staffel: %d %s", w.Code, w.Body.String())
	}
}

// LOW-2: ein eingetragener Schluessel wandert nicht zu einem anderen
// Menschen, auch mit gueltigen Unterschriften; neu eintragen fuer denselben
// Menschen bleibt moeglich.
func TestCoordinatorEintragung_SchluesselWandertNicht_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	pubHex := hex.EncodeToString(f.pub)
	k2, m2 := f.neuerMensch(false)
	w := f.eintragen(map[string]string{"public_key": pubHex, "human_wallet": m2,
		"human_signature": personalSign(t, k2, coordinatorFreigabeNachricht(pubHex)),
		"key_signature":   hex.EncodeToString(ed25519.Sign(f.priv, []byte(coordinatorBesitzNachricht(m2))))})
	if w.Code != http.StatusConflict {
		t.Fatalf("Schluessel zu einem anderen Menschen verschoben: %d %s", w.Code, w.Body.String())
	}
	if b, ok := f.cs.CoordinatorBindungLokal(pubHex); !ok || b.Mensch != f.bindung.Mensch {
		t.Fatalf("Bindung geaendert: %+v %v", b, ok)
	}
	w = f.eintragen(map[string]string{"public_key": pubHex, "human_wallet": f.bindung.Mensch,
		"human_signature": f.bindung.MenschSig, "key_signature": f.bindung.SchluesselSig, "url": "https://coordinator.example.org"})
	if w.Code != http.StatusOK {
		t.Fatalf("Neueintragung fuer denselben Menschen: %d %s", w.Code, w.Body.String())
	}
}

// MEDIUM-1 (#311): wer eine alte Eintragung (v1, oder v1-Besitz mit der
// oeffentlichen v2-Freigabe) wieder einspielt, kippt die gespeicherte
// v2-Bindung nicht -- sonst bekaeme jede Erneuerung dieses Coordinators auf
// allen Knoten 403. Eine neue Eintragung im alten Satz wird eingetragen, aber
// ohne Unterschriften, und sagt das.
func TestCoordinatorEintragung_V1UeberschreibtNicht_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	pubHex := hex.EncodeToString(f.pub)
	besitzV1 := hex.EncodeToString(ed25519.Sign(f.priv, []byte(coordinatorBesitzNachrichtV1(f.bindung.Mensch))))
	freigabeV1 := personalSign(t, f.menschSchluessel, coordinatorFreigabeNachrichtV1(pubHex))
	for name, body := range map[string]map[string]string{
		"v2-Freigabe, v1-Besitz": {"public_key": pubHex, "human_wallet": f.bindung.Mensch,
			"human_signature": f.bindung.MenschSig, "key_signature": besitzV1},
		"v1-Freigabe, v2-Besitz": {"public_key": pubHex, "human_wallet": f.bindung.Mensch,
			"human_signature": freigabeV1, "key_signature": f.bindung.SchluesselSig},
		"ganz v1": {"public_key": pubHex, "human_wallet": f.bindung.Mensch,
			"human_signature": freigabeV1, "key_signature": besitzV1},
	} {
		w := f.eintragen(body)
		// Die Antwort meldet den gespeicherten Stand: die v2-Bindung bleibt
		// und taugt -- keine Aufforderung, sich neu einzutragen.
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"bescheinigungstauglich":true`) ||
			strings.Contains(w.Body.String(), "hinweis") {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body.String())
		}
		b, ok := f.cs.CoordinatorBindungLokal(pubHex)
		if !ok || b != f.bindung {
			t.Fatalf("%s: Bindung gekippt: %+v %v", name, b, ok)
		}
		// Die Bescheinigung mit der GESPEICHERTEN Bindung, wie die API sie baut.
		tx := erneuerungsTransaktion(f.wallet, f.jetzt-60, pubHex, f.unterschreibe(f.priv, f.wallet, f.jetzt-60), b)
		if got := f.pruefe(tx, f.jetzt); len(got) != 0 {
			t.Fatalf("%s: Erneuerung danach gemeldet: %v", name, got)
		}
	}
	// v2 wieder eintragen: tauglich, Bindung bleibt dieselbe.
	if w := f.eintragen(map[string]string{"public_key": pubHex, "human_wallet": f.bindung.Mensch,
		"human_signature": f.bindung.MenschSig, "key_signature": f.bindung.SchluesselSig}); w.Code != http.StatusOK ||
		!strings.Contains(w.Body.String(), `"bescheinigungstauglich":true`) {
		t.Fatalf("v2: %d %s", w.Code, w.Body.String())
	}

	// Ein NEUER Schluessel nur im alten Satz: eingetragen, ohne Bindung.
	pub2, priv2, _ := ed25519.GenerateKey(rand.Reader)
	pub2Hex := hex.EncodeToString(pub2)
	k, mensch := f.neuerMensch(false)
	w := f.eintragen(map[string]string{"public_key": pub2Hex, "human_wallet": mensch,
		"human_signature": personalSign(t, k, coordinatorFreigabeNachrichtV1(pub2Hex)),
		"key_signature":   hex.EncodeToString(ed25519.Sign(priv2, []byte(coordinatorBesitzNachrichtV1(mensch))))})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"bescheinigungstauglich":false`) {
		t.Fatalf("neue v1-Eintragung: %d %s", w.Code, w.Body.String())
	}
	if b, ok := f.cs.CoordinatorBindungLokal(pub2Hex); ok {
		t.Fatalf("v1-Eintragung traegt eine Bindung: %+v", b)
	}
}

// #312, LOW-3: die Antwort meldet den gespeicherten Stand nur, wenn dessen
// Unterschriften selbst v2 sind. Eine Zeile mit gespeicherten
// v1-Unterschriften (vor #311 eingetragen) taugt nicht -- die Antwort darf
// nicht "tauglich" sagen, nur weil eine Zeile da ist, sonst erfuehre der
// Coordinator nie, dass er sich neu eintragen muss, und jede Erneuerung
// scheiterte auf allen Knoten.
func TestCoordinatorEintragung_GespeicherteV1TaugtNicht_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	f.cs.EnsureCoordinatorRegistry()
	// Jede Haelfte allein: ist nur eine der beiden gespeicherten
	// Unterschriften v1, taugt die Bindung genauso wenig.
	for i, fall := range []struct {
		name                 string
		freigabeV2, besitzV2 bool
		grund                string
	}{{"ganz v1", false, false, "Besitznachweis"}, {"v2-Freigabe, v1-Besitz", true, false, "Besitznachweis"},
		{"v1-Freigabe, v2-Besitz", false, true, "Freigabe des Menschen"}} {
		pub, priv, _ := ed25519.GenerateKey(rand.Reader)
		pubHex := hex.EncodeToString(pub)
		k, mensch := f.neuerMensch(false)
		// Zugelassen (Validator-Schluessel im Register): sonst scheiterte die
		// Erneuerung unten schon an der Zulassung, und der Test bewiese
		// nichts ueber v1.
		verlaufEintrag(t, f.cs, mensch, distTestAddr(1960+i), nowUnix()-30*86400)
		freigabeV1 := kanonischeSignaturVersuch(personalSign(t, k, coordinatorFreigabeNachrichtV1(pubHex)))
		besitzV1 := hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachrichtV1(mensch))))
		freigabe, besitz := freigabeV1, besitzV1
		if fall.freigabeV2 {
			freigabe = kanonischeSignaturVersuch(personalSign(t, k, coordinatorFreigabeNachricht(pubHex)))
		}
		if fall.besitzV2 {
			besitz = hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachricht(mensch))))
		}
		if _, err := f.cs.db.Exec(`INSERT INTO coordinator_keys (public_key, human_wallet, human_signature, key_signature)
			VALUES ($1, $2, $3, $4)`, pubHex, mensch, freigabe, besitz); err != nil {
			t.Fatal(err)
		}
		// Vorbedingung: die Zeile wird als Bindung gelesen -- sonst pruefte
		// der Test nur den Fall "keine Zeile".
		gespeichert, ok := f.cs.CoordinatorBindungLokal(pubHex)
		if !ok || gespeichert.Mensch != mensch {
			t.Fatalf("%s: gespeicherte Zeile nicht gelesen: %+v %v", fall.name, gespeichert, ok)
		}
		w := f.eintragen(map[string]string{"public_key": pubHex, "human_wallet": mensch,
			"human_signature": freigabeV1, "key_signature": besitzV1})
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"bescheinigungstauglich":false`) ||
			!strings.Contains(w.Body.String(), "hinweis") {
			t.Fatalf("%s: gespeicherte Zeile als tauglich gemeldet: %d %s", fall.name, w.Code, w.Body.String())
		}
		// Und tatsaechlich: eine Erneuerung mit dieser Bindung besteht bei
		// keinem Knoten.
		tx := erneuerungsTransaktion(f.wallet, f.jetzt-60, pubHex,
			hex.EncodeToString(ed25519.Sign(priv, []byte(erneuerungsNachricht(f.wallet, f.jetzt-60)))), gespeichert)
		if got := f.pruefe(tx, f.jetzt); got["erneuerung_ohne_bescheinigung"] != 1 {
			t.Fatalf("%s: Erneuerung mit dieser Bindung nicht gemeldet: %v", fall.name, got)
		}
		if err := bescheinigungPruefen(f.wallet, f.jetzt-60, tx.Bescheinigung, f.cs.coordinatorMenschStand, f.cs.coordinatorZugelassen); err == nil ||
			!strings.Contains(err.Error(), fall.grund) {
			t.Fatalf("%s: Grund %v, erwartet %q", fall.name, err, fall.grund)
		}
	}
}

// LOW-1: die Eintragung gleicht die Schreibweise an (v 0/1, Grossbuchstaben,
// 0x) und speichert die eine -- die Bescheinigung daraus besteht beim
// Nachrechnen. Eine Freigabe mit hohem s bleibt abgewiesen.
func TestCoordinatorEintragung_SchreibweiseAngeglichen_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)
	k, mensch := f.neuerMensch(false)
	verlaufEintrag(t, f.cs, mensch, distTestAddr(1953), nowUnix()-30*86400) // als Validator zugelassen
	freigabe := personalSign(t, k, coordinatorFreigabeNachricht(pubHex))
	v0 := "0x" + strings.ToUpper(freigabe[2:130]) + map[string]string{"1b": "00", "1c": "01"}[freigabe[130:]]
	besitz := "0x" + strings.ToUpper(hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachricht(mensch)))))
	if w := f.eintragen(map[string]string{"public_key": pubHex, "human_wallet": mensch, "human_signature": v0, "key_signature": besitz}); w.Code != http.StatusOK {
		t.Fatalf("Eintragung mit v=0/1 und Grossbuchstaben: %d %s", w.Code, w.Body.String())
	}
	b, ok := f.cs.CoordinatorBindungLokal(pubHex)
	if !ok || b.MenschSig != freigabe || !kanonischeSignatur(b.MenschSig) || b.SchluesselSig != ed25519SigNormal(besitz) {
		t.Fatalf("nicht in der einen Schreibweise gespeichert: %+v", b)
	}
	tx := erneuerungsTransaktion(f.wallet, f.jetzt-60, pubHex,
		hex.EncodeToString(ed25519.Sign(priv, []byte(erneuerungsNachricht(f.wallet, f.jetzt-60)))), b)
	if got := f.pruefe(tx, f.jetzt); len(got) != 0 {
		t.Fatalf("Bescheinigung aus angeglichener Eintragung gemeldet: %v", got)
	}
	// Hohes s: dieselbe Unterschrift in ihrer zweiten Form.
	roh, _ := hex.DecodeString(freigabe[2:])
	hoch := hohesS(roh)
	if w := f.eintragen(map[string]string{"public_key": pubHex, "human_wallet": mensch, "human_signature": "0x" + hex.EncodeToString(hoch),
		"key_signature": ed25519SigNormal(besitz)}); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "canonical form") {
		t.Fatalf("Freigabe mit hohem s: %d %s", w.Code, w.Body.String())
	}
}

// HIGH-2: das Register wird einmal angelegt, nicht je Anfrage. Haelt eine
// lange Lesung die Tabelle, wartet keine Anfrage auf ein ALTER TABLE -- sind
// die Spalten da, gibt es gar keins (information_schema). Fehlen sie und
// haengt das Anlegen an einer Sperre, wartet hoechstens EIN Aufrufer die
// Sperrfrist ab (die anderen gehen gleich weiter), nach dem Fehlschlag gibt
// es 10 s keinen neuen Versuch, und danach gelingt er.
func TestCoordinatorRegister_DDLEinmalUndBegrenzt_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	pubHex := hex.EncodeToString(f.pub)
	sperre, err := f.cs.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer sperre.Rollback()
	if _, err := sperre.Exec(`LOCK TABLE coordinator_keys IN ACCESS SHARE MODE`); err != nil {
		t.Fatal(err)
	}
	fertig := make(chan bool, 1)
	go func() {
		_, ok := f.cs.CoordinatorBindungLokal(pubHex)
		fertig <- ok
	}()
	select {
	case ok := <-fertig:
		if !ok {
			t.Fatal("Bindung unter einer Lesesperre nicht gefunden")
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("Anfrage wartet auf die Tabelle -- DDL im Anfragepfad")
	}
	// Riegel zurueck, Spalten da: kein ALTER TABLE, auch unter der Sperre
	// sofort fertig.
	f.cs.coordinatorRegisterDa.Store(false)
	f.cs.coordinatorRegisterVersuch.Store(0)
	start := time.Now()
	f.cs.EnsureCoordinatorRegistry()
	if d := time.Since(start); d > time.Second || !f.cs.coordinatorRegisterDa.Load() {
		t.Fatalf("mit vorhandenen Spalten: %v, Riegel %v -- erwartet sofort und gesetzt", d, f.cs.coordinatorRegisterDa.Load())
	}
	sperre.Rollback()

	// Spalte fehlt, die Tabelle ist gesperrt: vier Aufrufer gleichzeitig.
	if _, err := f.cs.db.Exec(`ALTER TABLE coordinator_keys DROP COLUMN key_signature`); err != nil {
		t.Fatal(err)
	}
	sperre2, err := f.cs.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer sperre2.Rollback()
	if _, err := sperre2.Exec(`LOCK TABLE coordinator_keys IN ACCESS SHARE MODE`); err != nil {
		t.Fatal(err)
	}
	f.cs.coordinatorRegisterDa.Store(false)
	f.cs.coordinatorRegisterVersuch.Store(0)
	dauer := make(chan time.Duration, 4)
	for i := 0; i < 4; i++ {
		go func() {
			s := time.Now()
			f.cs.EnsureCoordinatorRegistry()
			dauer <- time.Since(s)
		}()
	}
	lange := 0
	for i := 0; i < 4; i++ {
		d := <-dauer
		if d > 4*time.Second {
			t.Fatalf("ein Aufrufer wartete %v -- keine Sperrfrist", d)
		}
		if d > time.Second {
			lange++
		}
	}
	if lange > 1 {
		t.Fatalf("%d Aufrufer warteten die Sperrfrist ab -- hoechstens einer darf", lange)
	}
	if f.cs.coordinatorRegisterDa.Load() {
		t.Fatal("Riegel gesetzt, obwohl das Anlegen an der Sperre scheiterte")
	}
	// Binnen 10 s kein neuer Versuch -- auch nicht nach Freigabe der Sperre.
	sperre2.Rollback()
	f.cs.EnsureCoordinatorRegistry()
	if f.cs.coordinatorRegisterDa.Load() {
		t.Fatal("neuer Versuch binnen 10 s")
	}
	f.cs.coordinatorRegisterVersuch.Store(time.Now().Unix() - 11)
	f.cs.EnsureCoordinatorRegistry()
	if !f.cs.coordinatorRegisterDa.Load() {
		t.Fatal("Riegel nach dem naechsten Versuch nicht gesetzt")
	}
	var spalte int
	f.cs.db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'coordinator_keys' AND column_name = 'key_signature'`).Scan(&spalte)
	if spalte != 1 {
		t.Fatal("Spalte key_signature nicht wieder angelegt")
	}
}

// Missbrauch (zweiter Sicherheitsdurchgang #300): ein vor dem 06.10. mit
// einem Schluessel kleiner Ordnung eingetragener Coordinator oder
// Personhood-Schluessel geht nicht mehr hinaus -- Proof-Server und
// Vergleichsdienst lesen diese Listen und haetten die Universalunterschrift
// angenommen.
func TestGespeicherteSchluesselKleinerOrdnung_NichtAusgeliefert_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	if _, err := f.cs.db.Exec(`INSERT INTO coordinator_keys (public_key, human_wallet, human_signature, key_signature)
		VALUES ($1, $2, 'x', $3)`, ed25519NeutralSchluessel, f.bindung.Mensch, ed25519NeutralUnterschr); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.cs.Coordinators() {
		if c.PublicKey == ed25519NeutralSchluessel {
			t.Fatal("Coordinator mit Schluessel kleiner Ordnung in der Liste")
		}
	}
	if len(f.cs.Coordinators()) != 1 {
		t.Fatalf("echter Coordinator fehlt in der Liste: %+v", f.cs.Coordinators())
	}
	if _, ok := f.cs.CoordinatorBindungLokal(ed25519NeutralSchluessel); ok {
		t.Fatal("Bindung zu einem Schluessel kleiner Ordnung geliefert")
	}
	f.cs.InitValidatorKeysTable()
	f.cs.db.Exec(`DELETE FROM validator_keys`)
	_, signing := neuerSchluessel(t)
	if _, err := f.cs.db.Exec(`INSERT INTO validator_keys (signing_address, human_wallet, personhood_key) VALUES ($1, $2, $3)`,
		signing, f.bindung.Mensch, ed25519NeutralSchluessel); err != nil {
		t.Fatal(err)
	}
	for _, p := range f.cs.GetValidatorKeyPairsForSync() {
		if p.SigningAddress == signing && p.PersonhoodKey != "" {
			t.Fatalf("Personhood-Schluessel kleiner Ordnung ausgeliefert: %+v", p)
		}
	}
}

// Wer selbst eine offene Staffel hat, traegt keinen Coordinator ein -- sonst
// besetzte eine frische Kunstfigur einen Schluessel, den danach niemand mehr
// eintragen kann, und jede Erneuerung dieses Coordinators scheiterte.
func TestCoordinatorEintragung_OffeneStaffelAbgewiesen_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	pubHex := hex.EncodeToString(pub)
	k, mensch := f.neuerMensch(true)
	w := f.eintragen(map[string]string{"public_key": pubHex, "human_wallet": mensch,
		"human_signature": personalSign(t, k, coordinatorFreigabeNachricht(pubHex)),
		"key_signature":   hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachricht(mensch))))})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "open staged grant") {
		t.Fatalf("Eintragung mit offener Staffel: %d %s", w.Code, w.Body.String())
	}
	if _, ok := f.cs.CoordinatorBindungLokal(pubHex); ok {
		t.Fatal("Schluessel trotzdem eingetragen")
	}
}

// Dieselbe Erneuerung zweimal geschickt: einmal im Ausgang, die zweite
// Antwort sagt "schon erneuert" (zweiter Sicherheitsdurchgang #300).
func TestErneuerung_ZweimalGeschicktEinmalImAusgang_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	a := &APIServer{state: f.cs}
	issued := nowUnix()
	body, _ := json.Marshal(map[string]interface{}{
		"wallet": f.wallet, "issued_at": issued,
		"public_key": hex.EncodeToString(f.pub), "signature": f.unterschreibe(f.priv, f.wallet, issued),
	})
	var antworten []string
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		a.handleLivenessRenewal(w, httptest.NewRequest(http.MethodPost, "/api/liveness-renewal", bytes.NewReader(body)))
		if w.Code != http.StatusOK {
			t.Fatalf("Anfrage %d: %d %s", i, w.Code, w.Body.String())
		}
		antworten = append(antworten, w.Body.String())
	}
	if !strings.Contains(antworten[1], "schon_erneuert") || !strings.Contains(antworten[2], "schon_erneuert") {
		t.Fatalf("Wiederholung nicht als schon erneuert beantwortet: %v", antworten)
	}
	var n int
	f.cs.db.QueryRow(`SELECT COUNT(*) FROM pending_txs WHERE included_at = 0 AND tx_json LIKE '%liveness_renewal%'`).Scan(&n)
	if n != 1 {
		t.Fatalf("%d Erneuerungen im Ausgang statt 1", n)
	}
}

// Eine Wallet, eine Schreibweise: dieselbe Erneuerung mit gross geschriebener
// Adresse wird gemeldet (sonst laege sie unter mehreren Hashes in Bloecken).
func TestErneuerung_WalletNichtKanonisch_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 10)
	tx := f.gueltig(f.jetzt - 60)
	tx.Wallet = "0x" + strings.ToUpper(f.wallet[2:])
	if got := f.pruefe(tx, f.jetzt); got["erneuerung_ohne_bescheinigung"] != 1 {
		t.Fatalf("nicht kanonische Wallet nicht erkannt: %v", got)
	}
}

// hohesS: dieselbe secp256k1-Unterschrift in der oberen Haelfte (s -> n-s,
// v gekippt) -- gueltig fuer ecrecover, aber nicht kanonisch.
func hohesS(roh []byte) []byte {
	hoch := append([]byte(nil), roh...)
	s := new(big.Int).SetBytes(hoch[32:64])
	s.Sub(crypto.S256().Params().N, s)
	s.FillBytes(hoch[32:64])
	hoch[64] = 55 - hoch[64]
	return hoch
}

// Zulassung im Konsens (coordinator_zulassung.go): nur, wer zur Zeit der
// Bescheinigung einen Validator-Schluessel haelt, bescheinigt. Missbrauch:
// ein registrierter Mensch ohne Bindung (die Farm mit dem alten Konto), ein
// Schluessel, den ein anderer uebernommen hat, und eine Bindung, die noch in
// ihrer Frist steht -- bei der Annahme wie beim Nachspielen.
func TestErneuerung_CoordinatorZulassung_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 8)
	mensch := f.bindung.Mensch
	schluessel := distTestAddr(1950)
	issued := nowUnix()
	a := &APIServer{state: f.cs}
	annahme := func() int {
		body, _ := json.Marshal(map[string]interface{}{
			"wallet": f.wallet, "issued_at": issued,
			"public_key": hex.EncodeToString(f.pub), "signature": f.unterschreibe(f.priv, f.wallet, issued),
		})
		w := httptest.NewRecorder()
		a.handleLivenessRenewal(w, httptest.NewRequest(http.MethodPost, "/api/liveness-renewal", bytes.NewReader(body)))
		return w.Code
	}
	abgewiesen := func(fall string) {
		t.Helper()
		if n := f.pruefe(f.gueltig(issued), issued+60)["erneuerung_ohne_bescheinigung"]; n != 1 {
			t.Fatalf("%s: beim Nachspielen nicht erkannt (%d)", fall, n)
		}
		if code := annahme(); code != http.StatusForbidden {
			t.Fatalf("%s: bei der Annahme %d statt 403", fall, code)
		}
	}
	if ok, err := coordinatorZugelassenIn(f.cs.db, mensch, issued); err != nil || !ok {
		t.Fatalf("Voraussetzung: gebundener Mensch nicht zugelassen (%v, %v)", ok, err)
	}

	f.cs.db.Exec(`DELETE FROM validator_verlauf`)
	abgewiesen("ohne Validator-Bindung")

	// Ein anderer Betreiber hat den Schluessel uebernommen (Frist vorbei).
	anderer := distTestAddr(1951)
	f.cs.mu.Lock()
	acc := &AccountState{Address: anderer, IsHuman: true}
	f.cs.accounts.Set(anderer, acc)
	err := f.cs.saveAccountToDB(acc)
	f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	verlaufEintrag(t, f.cs, mensch, schluessel, issued-30*86400)
	verlaufEintrag(t, f.cs, anderer, schluessel, issued-erzeugerFrist-60)
	abgewiesen("Schluessel uebernommen")

	// Frisch gebunden: wirkt erst nach erzeugerFrist.
	f.cs.db.Exec(`DELETE FROM validator_verlauf`)
	verlaufEintrag(t, f.cs, mensch, schluessel, issued-erzeugerFrist+60)
	abgewiesen("Bindung in der Frist")

	f.cs.db.Exec(`DELETE FROM validator_verlauf`)
	verlaufEintrag(t, f.cs, mensch, schluessel, issued-erzeugerFrist)
	if n := f.pruefe(f.gueltig(issued), issued+60)["erneuerung_ohne_bescheinigung"]; n != 0 {
		t.Fatalf("Bindung genau nach der Frist abgewiesen (%d)", n)
	}
	if code := annahme(); code != http.StatusOK {
		t.Fatalf("zugelassener Coordinator bei der Annahme %d", code)
	}
}

// Nur Fenster menschlicher Betreiber zaehlen (Sicherheitsdurchgang zu den
// Korrekturen in #312, Testluecke): bindet ein Nicht-Mensch denselben
// Schluessel zum selben Zeitpunkt, ist das kein Streit -- bindet ihn ein
// zweiter Mensch, schon (umstritten: keiner zugelassen).
func TestErneuerung_ZulassungNurMenschlicheBetreiber_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 8)
	mensch := f.bindung.Mensch
	schluessel := distTestAddr(1950)
	issued := nowUnix()
	seit := issued - 30*86400
	f.cs.db.Exec(`DELETE FROM validator_verlauf`)
	verlaufEintrag(t, f.cs, mensch, schluessel, seit)
	keinMensch := distTestAddr(1954)
	registerKonto(t, f.cs, keinMensch, false)
	verlaufEintrag(t, f.cs, keinMensch, schluessel, seit)
	if ok, err := coordinatorZugelassenIn(f.cs.db, mensch, issued); err != nil || !ok {
		t.Fatalf("Nicht-Mensch zum selben Zeitpunkt macht den Schluessel streitig: %v, %v", ok, err)
	}
	zweiter := distTestAddr(1955)
	registerKonto(t, f.cs, zweiter, true)
	verlaufEintrag(t, f.cs, zweiter, schluessel, seit)
	if ok, err := coordinatorZugelassenIn(f.cs.db, mensch, issued); err != nil || ok {
		t.Fatalf("zwei Menschen zum selben Zeitpunkt: zugelassen=%v, %v -- erwartet umstritten", ok, err)
	}
}

// Die Grenze der Betreiber eines Schluessels (Testluecke): bis zu
// coordinatorSchluesselGrenze gelesen, darueber ein Fehler -- abgewiesen,
// nicht abgeschnitten. Nur der Inhaber des Schluessels kann Betreiber
// hinzufuegen (jede Bindung traegt seine Unterschrift).
func TestErneuerung_ZulassungBetreiberGrenze_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 8)
	mensch := f.bindung.Mensch
	schluessel := distTestAddr(1950)
	issued := nowUnix()
	weitere := func(n int) {
		t.Helper()
		f.cs.db.Exec(`DELETE FROM validator_verlauf`)
		// n fruehere Betreiber, danach der Mensch selbst.
		if _, err := f.cs.db.Exec(`INSERT INTO validator_verlauf (operator_wallet, signing_address, bindung_ts, sig_operator, sig_signing)
			SELECT '0x' || lpad(to_hex(g), 40, '0'), $1, $2::bigint - g, 'x', 'y' FROM generate_series(1, $3::bigint) g`,
			strings.ToLower(schluessel), issued-40*86400, n); err != nil {
			t.Fatal(err)
		}
		verlaufEintrag(t, f.cs, mensch, schluessel, issued-30*86400)
	}
	weitere(coordinatorSchluesselGrenze - 1)
	if ok, err := coordinatorZugelassenIn(f.cs.db, mensch, issued); err != nil || !ok {
		t.Fatalf("%d Betreiber: %v, %v -- erwartet zugelassen", coordinatorSchluesselGrenze, ok, err)
	}
	weitere(coordinatorSchluesselGrenze)
	if _, err := coordinatorZugelassenIn(f.cs.db, mensch, issued); err == nil {
		t.Fatalf("%d Betreiber: kein Fehler -- die Grenze schneidet ab statt abzuweisen", coordinatorSchluesselGrenze+1)
	}
	if n := f.pruefe(f.gueltig(issued), issued+60)["erneuerung_ohne_bescheinigung"]; n != 1 {
		t.Fatalf("ueber der Grenze beim Nachspielen nicht abgewiesen (%d)", n)
	}
}

// #312: ein Datenbankfehler bei der Zulassung geht nicht an den Aufrufer
// (Treiber- und Tabellennamen, bei einem Verbindungsfehler der Host) --
// die Antwort ist 500 mit festem Text, die Erneuerung abgewiesen
// (fail-closed), der Ausgang leer.
func TestErneuerung_AnnahmeVerraetKeineInterna_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 8)
	if _, err := f.cs.db.Exec(`ALTER TABLE validator_verlauf RENAME TO validator_verlauf_weg`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.cs.db.Exec(`ALTER TABLE validator_verlauf_weg RENAME TO validator_verlauf`) })
	issued := nowUnix()
	body, _ := json.Marshal(map[string]interface{}{
		"wallet": f.wallet, "issued_at": issued,
		"public_key": hex.EncodeToString(f.pub), "signature": f.unterschreibe(f.priv, f.wallet, issued),
	})
	w := httptest.NewRecorder()
	(&APIServer{state: f.cs}).handleLivenessRenewal(w, httptest.NewRequest(http.MethodPost, "/api/liveness-renewal", bytes.NewReader(body)))
	if _, err := f.cs.db.Exec(`ALTER TABLE validator_verlauf_weg RENAME TO validator_verlauf`); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("unlesbare Zulassung: %d %s (erwartet 500)", w.Code, w.Body.String())
	}
	for _, intern := range []string{"pq", "validator_verlauf", "relation", "Zulassung"} {
		if strings.Contains(w.Body.String(), intern) {
			t.Fatalf("Antwort verraet %q: %s", intern, w.Body.String())
		}
	}
	var n int
	if err := f.cs.db.QueryRow(`SELECT COUNT(*) FROM pending_txs WHERE included_at = 0`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("Erneuerung trotz unlesbarer Zulassung im Ausgang (%d)", n)
	}

	// Dasselbe beim Schreiben: scheitert der Ausgang an der Datenbank, steht
	// in der Antwort nur der feste Text.
	if _, err := f.cs.db.Exec(`ALTER TABLE pending_txs RENAME TO pending_txs_weg`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.cs.db.Exec(`ALTER TABLE pending_txs_weg RENAME TO pending_txs`) })
	w = httptest.NewRecorder()
	(&APIServer{state: f.cs}).handleLivenessRenewal(w, httptest.NewRequest(http.MethodPost, "/api/liveness-renewal", bytes.NewReader(body)))
	if _, err := f.cs.db.Exec(`ALTER TABLE pending_txs_weg RENAME TO pending_txs`); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("Ausgang nicht schreibbar: %d %s (erwartet 500)", w.Code, w.Body.String())
	}
	for _, intern := range []string{"pq", "pending_txs", "relation"} {
		if strings.Contains(w.Body.String(), intern) {
			t.Fatalf("Antwort verraet %q: %s", intern, w.Body.String())
		}
	}
}

// Fail-closed: ist der Verlauf nicht lesbar, gilt niemand als zugelassen.
func TestErneuerung_ZulassungUnlesbar_RealDB(t *testing.T) {
	f := neuerErneuerungsFall(t, 8)
	if _, err := f.cs.db.Exec(`ALTER TABLE validator_verlauf RENAME TO validator_verlauf_weg`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.cs.db.Exec(`ALTER TABLE validator_verlauf_weg RENAME TO validator_verlauf`) })
	issued := nowUnix()
	n := f.pruefe(f.gueltig(issued), issued+60)["erneuerung_ohne_bescheinigung"]
	if _, err := f.cs.db.Exec(`ALTER TABLE validator_verlauf_weg RENAME TO validator_verlauf`); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("unlesbare Zulassung nicht gemeldet (%d)", n)
	}
}
