package keeper

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Jaehrlicher Lebensnachweis gegen eine echte Datenbank: Annahme ueber den
// Handler (Register, Zulassung aus validator_verlauf, Ausgang), Nachspielen
// bei einem Knoten ohne Register, Spalte, Laden und StateRoot.

type lnDBFall struct {
	t       *testing.T
	cs      *ChainState
	dag     *BlockDAG
	priv    ed25519.PrivateKey
	pubHex  string
	bindung CoordinatorBindung
	wallet  string
	jetzt   int64
}

// neuerLnDBFall: Stichtag vor 400 Tagen (wer nicht nachgewiesen hat, ist
// faellig und pausiert), ein eingetragener, zugelassener Coordinator und eine
// registrierte Wallet ohne Nachweis.
func neuerLnDBFall(t *testing.T) *lnDBFall {
	t.Helper()
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-lebensnachweis-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}
	jetzt := nowUnix()
	lnAktiv(t, jetzt-400*lnTag, 1)
	ipBurst.Delete("proof-of-life:192.0.2.1")
	cs.EnsureCoordinatorRegistry()
	if _, err := cs.db.Exec(`DELETE FROM coordinator_keys`); err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubHex := hex.EncodeToString(pub)
	mk, mensch := neuerSchluessel(t)
	registerKonto(t, cs, mensch, true)
	bindung := CoordinatorBindung{Mensch: mensch,
		MenschSig:     personalSign(t, mk, coordinatorFreigabeNachricht(pubHex)),
		SchluesselSig: hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachricht(mensch))))}
	if err := cs.RegisterCoordinatorKey(pubHex, mensch, "", bindung.MenschSig, bindung.SchluesselSig, true); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.db.Exec(`DELETE FROM validator_verlauf`); err != nil {
		t.Fatal(err)
	}
	verlaufEintrag(t, cs, mensch, distTestAddr(1950), jetzt-30*lnTag)
	f := &lnDBFall{t: t, cs: cs, priv: priv, pubHex: pubHex, bindung: bindung, wallet: distTestAddr(1960), jetzt: jetzt}
	registerKonto(t, cs, f.wallet, true)
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

func (f *lnDBFall) koerper(wallet string, issued int64, satz string) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"wallet": wallet, "issued_at": issued, "public_key": f.pubHex,
		"signature": hex.EncodeToString(ed25519.Sign(f.priv, []byte(satz))),
	})
	return b
}

func (f *lnDBFall) reiche(body []byte) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	(&APIServer{state: f.cs}).handleProofOfLife(w, httptest.NewRequest(http.MethodPost, "/api/proof-of-life", bytes.NewReader(body)))
	return w
}

func (f *lnDBFall) amInDB(w string) int64 {
	var am int64
	if err := f.cs.db.QueryRow(`SELECT lebensnachweis_am FROM chain_accounts WHERE lower(address) = $1`, w).Scan(&am); err != nil {
		f.t.Fatal(err)
	}
	return am
}

func (f *lnDBFall) imAusgang() []Transaction {
	rows, err := f.cs.db.Query(`SELECT tx_json FROM pending_txs WHERE included_at = 0 AND tx_json LIKE '%proof_of_life%' ORDER BY id`)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	var txs []Transaction
	for rows.Next() {
		var s string
		var tx Transaction
		if rows.Scan(&s) != nil || json.Unmarshal([]byte(s), &tx) != nil {
			f.t.Fatal("Ausgang unlesbar")
		}
		txs = append(txs, tx)
	}
	return txs
}

// Gutfall: angenommen, im Zustand und in der Datenbank, einmal im Ausgang
// (mit Bindung); dieselbe Bescheinigung noch einmal: schon_nachgewiesen.
// Ein Knoten ohne Coordinator-Register spielt den Ausgang nach und kommt
// zum selben Ergebnis.
func TestLebensnachweis_AnnahmeUndNachspielen_RealDB(t *testing.T) {
	f := neuerLnDBFall(t)
	issued := nowUnix()
	body := f.koerper(f.wallet, issued, lebensnachweisNachricht(f.wallet, issued))
	w := f.reiche(body)
	if w.Code != http.StatusOK {
		t.Fatalf("gueltiger Nachweis: %d %s", w.Code, w.Body.String())
	}
	var antwort struct {
		OK             bool                       `json:"ok"`
		Am             int64                      `json:"lebensnachweis_am"`
		Lebensnachweis map[string]json.RawMessage `json:"lebensnachweis"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &antwort); err != nil || !antwort.OK || antwort.Am != issued ||
		string(antwort.Lebensnachweis["pausiert"]) != "false" || string(antwort.Lebensnachweis["am"]) != jsonInt(issued) {
		t.Fatalf("Antwort: %s", w.Body.String())
	}
	if got := f.amInDB(f.wallet); got != issued {
		t.Fatalf("lebensnachweis_am in der Datenbank %d, erwartet %d", got, issued)
	}
	for i := 0; i < 2; i++ {
		if w := f.reiche(body); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "schon_nachgewiesen") {
			t.Fatalf("Wiederholung %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	txs := f.imAusgang()
	if len(txs) != 1 {
		t.Fatalf("%d Lebensnachweise im Ausgang statt 1", len(txs))
	}
	tx := txs[0]
	if tx.Type != "proof_of_life" || tx.DistributionAt != issued || tx.Bescheinigung == nil || tx.Bescheinigung.Mensch != f.bindung.Mensch ||
		tx.Bescheinigung.MenschSig == "" || tx.Bescheinigung.SchluesselSig == "" {
		t.Fatalf("Ausgang: %+v %+v", tx, tx.Bescheinigung)
	}

	// Nachspielen ohne Register: das Feld zuruecksetzen, Register leeren,
	// den Block nachspielen.
	f.cs.mu.Lock()
	acc := acct(f.cs, f.wallet)
	acc.LebensnachweisAm = 0
	err := f.cs.saveAccountToDB(acc)
	f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	f.cs.db.Exec(`DELETE FROM coordinator_keys`)
	vorher := erhaltungZaehler(lnRegelOhne)
	if !f.dag.replayTransactions(&Block{Height: 1, Hash: "ln-db-1", Timestamp: issued + 5, Transactions: []Transaction{tx}}, true) {
		t.Fatal("Block mit gueltigem Lebensnachweis abgelehnt")
	}
	if got := f.amInDB(f.wallet); got != issued || erhaltungZaehler(lnRegelOhne) != vorher {
		t.Fatalf("nachgespielt: am=%d, Befunde +%d", got, erhaltungZaehler(lnRegelOhne)-vorher)
	}
	// Derselbe Block noch einmal (anderer Hash): Leerlauf.
	if !f.dag.replayTransactions(&Block{Height: 2, Hash: "ln-db-2", Timestamp: issued + 6, Transactions: []Transaction{tx}}, true) ||
		f.amInDB(f.wallet) != issued {
		t.Fatal("Wiederholung desselben Lebensnachweises nicht idempotent")
	}
}

func jsonInt(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// Missbrauch bei der Annahme -- und dieselben Faelle beim Nachspielen.
func TestLebensnachweis_AnnahmeMissbrauch_RealDB(t *testing.T) {
	f := neuerLnDBFall(t)
	issued := nowUnix()
	gut := f.koerper(f.wallet, issued, lebensnachweisNachricht(f.wallet, issued))

	// Erneuerungs-Domaene: 403, bevor das Register gefragt wird.
	if w := f.reiche(f.koerper(f.wallet, issued, erneuerungsNachricht(f.wallet, issued))); w.Code != http.StatusForbidden {
		t.Fatalf("Erneuerungs-Unterschrift: %d %s", w.Code, w.Body.String())
	}
	// Kein Mensch: 409.
	fremd := distTestAddr(1961)
	registerKonto(t, f.cs, fremd, false)
	if w := f.reiche(f.koerper(fremd, issued, lebensnachweisNachricht(fremd, issued))); w.Code != http.StatusConflict {
		t.Fatalf("kein Mensch: %d %s", w.Code, w.Body.String())
	}
	// Zu frueh: 409 mit faellig_ab als ganzer Zahl.
	f.cs.mu.Lock()
	acc := acct(f.cs, f.wallet)
	acc.LebensnachweisAm = issued - 10*lnTag
	err := f.cs.saveAccountToDB(acc)
	f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	w := f.reiche(gut)
	var frueh struct {
		FaelligAb json.RawMessage `json:"faellig_ab"`
	}
	if w.Code != http.StatusConflict || json.Unmarshal(w.Body.Bytes(), &frueh) != nil || string(frueh.FaelligAb) != jsonInt(issued-10*lnTag+300*lnTag) {
		t.Fatalf("zu frueh: %d %s", w.Code, w.Body.String())
	}
	f.cs.mu.Lock()
	acc.LebensnachweisAm = 0
	err = f.cs.saveAccountToDB(acc)
	f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	// Folger: nimmt nicht an.
	f.cs.leitung.Store(&Leitung{})
	if w := f.reiche(gut); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("Folger: %d %s", w.Code, w.Body.String())
	}
	f.cs.leitung.Store(nil)
	// Coordinator nicht zugelassen (keine Validator-Bindung): 403 bei der
	// Annahme, Befund und kein Anwenden beim Nachspielen.
	f.cs.db.Exec(`DELETE FROM validator_verlauf`)
	if w := f.reiche(gut); w.Code != http.StatusForbidden {
		t.Fatalf("nicht zugelassen: %d %s", w.Code, w.Body.String())
	}
	tx := lebensnachweisTransaktion(f.wallet, issued, f.pubHex,
		hex.EncodeToString(ed25519.Sign(f.priv, []byte(lebensnachweisNachricht(f.wallet, issued)))), f.bindung)
	vorher := erhaltungZaehler(lnRegelOhne)
	f.dag.replayTransactions(&Block{Height: 1, Hash: "ln-db-nz", Timestamp: issued + 5, Transactions: []Transaction{tx}}, true)
	if erhaltungZaehler(lnRegelOhne) != vorher+1 || f.amInDB(f.wallet) != 0 {
		t.Fatalf("nicht zugelassener Coordinator beim Nachspielen: Befunde +%d, am=%d", erhaltungZaehler(lnRegelOhne)-vorher, f.amInDB(f.wallet))
	}
	// Schluessel nicht eingetragen: 403.
	verlaufEintrag(t, f.cs, f.bindung.Mensch, distTestAddr(1950), issued-30*lnTag)
	f.cs.db.Exec(`DELETE FROM coordinator_keys`)
	if w := f.reiche(gut); w.Code != http.StatusForbidden {
		t.Fatalf("Schluessel nicht eingetragen: %d %s", w.Code, w.Body.String())
	}
	if len(f.imAusgang()) != 0 {
		t.Fatal("abgewiesener Lebensnachweis im Ausgang")
	}
}

// Spalte, Laden und StateRoot: ein Konto ohne Nachweis hat lebensnachweis_am
// = 0 und denselben Beitrag zur StateRoot wie vorher; kalt geladen traegt
// ein Konto mit Nachweis sein Feld, und der Neuaufbau der Akkumulatoren aus
// der Datenbank stimmt mit dem Speicher ueberein. Die Zaehlung in SQL ist
// die Auswahl des Erzeugers.
func TestLebensnachweis_SpalteLadenZaehlen_RealDB(t *testing.T) {
	f := neuerLnDBFall(t)
	ubiAt := f.jetzt
	mit := distTestAddr(1962)
	f.cs.mu.Lock()
	acc := &AccountState{Address: mit, IsHuman: true, Balance: NewDecimal(1000), LebensnachweisAm: ubiAt - 10*lnTag}
	f.cs.accounts.Set(mit, acc)
	err := f.cs.saveAccountToDB(acc)
	f.cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if f.amInDB(f.wallet) != 0 || f.amInDB(mit) != ubiAt-10*lnTag {
		t.Fatal("Spalte lebensnachweis_am falsch geschrieben")
	}
	// Kalt laden.
	f.cs.mu.Lock()
	f.cs.accounts.Delete(mit)
	f.cs.ensureAccountLoadedCtx(context.Background(), mit)
	geladen := acct(f.cs, mit)
	f.cs.mu.Unlock()
	if geladen == nil || geladen.LebensnachweisAm != ubiAt-10*lnTag {
		t.Fatalf("kalt geladen ohne Lebensnachweis: %+v", geladen)
	}
	// Neuaufbau aus der Datenbank (wie beim Start) == Speicher.
	vorher := f.cs.StateRootComponentBreakdown()
	f.cs.mu.Lock()
	f.cs.rebuildStateAccumulators()
	f.cs.mu.Unlock()
	// Nur die Konten: validator_verlauf hat der Test direkt beschrieben
	// (verlaufEintrag), am Speicher vorbei -- das ist nicht Gegenstand hier.
	if nachher := f.cs.StateRootComponentBreakdown(); vorher.AccountSetXOR != nachher.AccountSetXOR {
		t.Fatalf("account_set_xor vor/nach Neuaufbau verschieden: %s / %s", vorher.AccountSetXOR, nachher.AccountSetXOR)
	}
	// Nur EIN zugelassener Betreiber: niemand wird pausiert -- sein eigener
	// Mensch koennte nie nachweisen (kein Coordinator bescheinigt sich selbst).
	f.cs.mu.Lock()
	moeglich, err := f.cs.lebensnachweisPauseMoeglichLocked(context.Background(), ubiAt)
	f.cs.mu.Unlock()
	if err != nil || moeglich {
		t.Fatalf("ein Betreiber: Pause moeglich=%v (%v)", moeglich, err)
	}
	// Ein zweiter Mensch haelt einen Validator-Schluessel: jetzt pausiert die Runde.
	zweiter := distTestAddr(1963)
	registerKonto(t, f.cs, zweiter, true)
	verlaufEintrag(t, f.cs, zweiter, distTestAddr(1951), f.jetzt-30*lnTag)
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	moeglich, err = f.cs.lebensnachweisPauseMoeglichLocked(context.Background(), ubiAt)
	if err != nil || !moeglich {
		t.Fatalf("zwei Betreiber: Pause moeglich=%v (%v)", moeglich, err)
	}
	// Zaehlung (SQL) und Auswahl (Speicher): f.wallet und beide Betreiber
	// ohne Nachweis sind pausiert, mit nicht.
	n, err := f.cs.countUBIEmpfaengerLocked(context.Background(), ubiAt, moeglich)
	if err != nil {
		t.Fatal(err)
	}
	var menschen []string
	f.cs.accounts.Range(func(a string, acc *AccountState) bool {
		if acc.IsHuman {
			menschen = append(menschen, a)
		}
		return true
	})
	empf, pausiert := f.cs.lebensnachweisEmpfaengerLocked(menschen, ubiAt, moeglich)
	if n != int64(len(empf)) || n != 1 || pausiert != 3 || empf[0] != mit {
		t.Fatalf("SQL zaehlt %d, Auswahl %v (%d pausiert)", n, empf, pausiert)
	}
}
