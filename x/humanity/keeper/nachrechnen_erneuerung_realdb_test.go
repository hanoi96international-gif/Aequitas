package keeper

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	// bindung: der Mensch hinter dem Coordinator-Schluessel und die beiden
	// Unterschriften der Eintragung -- sie gehen in jede Bescheinigung.
	bindung CoordinatorBindung
	jetzt   int64
	n       int
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
		SchluesselSig: hex.EncodeToString(ed25519.Sign(priv, []byte("Aequitas: coordinator key for human "+mensch)))}
	if err := cs.RegisterCoordinatorKey(hex.EncodeToString(pub), mensch, "", bindung.MenschSig, bindung.SchluesselSig); err != nil {
		t.Fatal(err)
	}
	f := &erneuerungsFall{t: t, cs: cs, pub: pub, priv: priv, wallet: distTestAddr(1900), jetzt: nowUnix(), bindung: bindung}
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
		t.Fatalf("Schluessel ausserhalb des Registers nicht erkannt: %v", got)
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
	nullZeit := erneuerungsTransaktion(f.wallet, 0, hex.EncodeToString(f.pub), f.unterschreibe(f.priv, f.wallet, 0), f.bindung)
	if got := f.pruefe(nullZeit, f.jetzt); got["erneuerung_zeit"] != 1 {
		t.Fatalf("Bescheinigung ohne Zeitpunkt nicht erkannt: %v", got)
	}

	voraus := f.gueltig(f.jetzt + 3600)
	if got := f.pruefe(voraus, f.jetzt); got["erneuerung_zeit"] != 1 {
		t.Fatalf("Bescheinigung aus der Zukunft nicht erkannt: %v", got)
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
	fremderBesitz.Bescheinigung.SchluesselSig = hex.EncodeToString(ed25519.Sign(f.priv, []byte("Aequitas: coordinator key for human "+m2)))
	fremdeFreigabe := f.gueltig(issued)
	fremdeFreigabe.Bescheinigung.MenschSig = personalSign(t, k2, coordinatorFreigabeNachricht(hex.EncodeToString(f.pub)))
	// Ein Mensch, den es auf der Kette nicht gibt, mit gueltigen Unterschriften.
	keinMensch := erneuerungsTransaktion(f.wallet, issued, hex.EncodeToString(f.pub), f.unterschreibe(f.priv, f.wallet, issued),
		CoordinatorBindung{Mensch: m2, MenschSig: personalSign(t, k2, coordinatorFreigabeNachricht(hex.EncodeToString(f.pub))),
			SchluesselSig: hex.EncodeToString(ed25519.Sign(f.priv, []byte("Aequitas: coordinator key for human "+m2)))})
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
	if err := f.cs.RegisterCoordinatorKey(hex.EncodeToString(f.pub), f.bindung.Mensch, "", f.bindung.MenschSig, f.bindung.SchluesselSig); err != nil {
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
	body, _ := json.Marshal(map[string]string{
		"public_key": pubHex, "human_wallet": mensch,
		"human_signature": personalSign(t, k, coordinatorFreigabeNachricht(pubHex)),
		"key_signature":   hex.EncodeToString(ed25519.Sign(priv, []byte("Aequitas: coordinator key for human "+mensch))),
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
		hex.EncodeToString(ed25519.Sign(priv, []byte(fmt.Sprintf("%s|%s|%d", livenessRenewalDomain, f.wallet, f.jetzt-60)))), b)
	if got := f.pruefe(tx, f.jetzt); len(got) != 0 {
		t.Fatalf("Bescheinigung mit gespeicherter Bindung gemeldet: %v", got)
	}
}
