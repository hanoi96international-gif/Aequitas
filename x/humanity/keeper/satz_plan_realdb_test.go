package keeper

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Der Endpunkt /api/wirtschaft/satz und der Plan in der Datenbank: Annahme,
// Ausgang mit Bindungen, Nachspielen ohne Coordinator-Register, Laden nach
// einem Neustart -- und Missbrauch ueber HTTP.
func TestSatz_AnnahmeNachspielenLaden_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-satz-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	gleichgewichtAn(t)
	vorherLeser := registerLeserOverride.Load()
	registerLeserOverride.Store(1)
	t.Cleanup(func() { registerLeserOverride.Store(vorherLeser) })
	cs.annehmendAusdruecklich.Store(true)
	jetzt := nowUnix()
	cs.EnsureCoordinatorRegistry()
	for _, q := range []string{`DELETE FROM coordinator_keys`, `DELETE FROM validator_verlauf`} {
		if _, err := cs.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	var betreiber []seBetreiber
	for i := 0; i < 2; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		pubHex := hex.EncodeToString(pub)
		mk, mensch := neuerSchluessel(t)
		registerKonto(t, cs, mensch, true)
		b := seBetreiber{priv: priv, pubHex: pubHex, bindung: CoordinatorBindung{Mensch: mensch,
			MenschSig:     personalSign(t, mk, coordinatorFreigabeNachricht(pubHex)),
			SchluesselSig: hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachricht(mensch))))}}
		if err := cs.RegisterCoordinatorKey(pubHex, mensch, "", b.bindung.MenschSig, b.bindung.SchluesselSig, true); err != nil {
			t.Fatal(err)
		}
		verlaufEintrag(t, cs, mensch, distTestAddr(3200+i), jetzt-30*spTag)
		betreiber = append(betreiber, b)
	}
	ab := jetzt + 35*spTag
	koerper := func(bp, giltAb, issued int64, signiertBp int64) []byte {
		var bs []map[string]string
		for _, b := range betreiber {
			sig := ed25519.Sign(b.priv, []byte(satzNachricht(signiertBp, giltAb)("", issued)))
			bs = append(bs, map[string]string{"public_key": b.pubHex, "signature": hex.EncodeToString(sig)})
		}
		out, _ := json.Marshal(map[string]interface{}{"bp": bp, "gilt_ab": giltAb, "issued_at": issued, "bescheinigungen": bs})
		return out
	}
	reiche := func(body []byte) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		ipBurst.Delete("satz:192.0.2.1")
		(&APIServer{state: cs}).handleSatzAnkuendigung(w, httptest.NewRequest(http.MethodPost, "/api/wirtschaft/satz", bytes.NewReader(body)))
		return w
	}

	// Missbrauch ueber HTTP, vor jeder Annahme.
	for _, m := range []struct {
		name string
		body []byte
		code int
	}{
		{"kein Schritt", koerper(52, ab, jetzt, 52), http.StatusBadRequest},
		{"Faelschung", koerper(60, ab, jetzt, 70), http.StatusForbidden},
		{"Sprung", koerper(90, ab, jetzt, 90), http.StatusConflict},
		{"zu kurz vorher", koerper(60, jetzt+10*spTag, jetzt, 60), http.StatusConflict},
		{"kaputt", []byte(`{"bp":`), http.StatusBadRequest},
	} {
		if w := reiche(m.body); w.Code != m.code {
			t.Fatalf("%s: %d %s, erwartet %d", m.name, w.Code, w.Body.String(), m.code)
		}
		if !cs.satzPlan.leer() {
			t.Fatalf("%s: Plan veraendert", m.name)
		}
	}

	// Gutfall.
	gut := koerper(60, ab, jetzt, 60)
	if w := reiche(gut); w.Code != http.StatusOK {
		t.Fatalf("gueltige Ankuendigung: %d %s", w.Code, w.Body.String())
	}
	if s := cs.satzPlan.bpZu(ab); s != 60 {
		t.Fatalf("Plan im Speicher: %+v", cs.satzPlan)
	}
	gespeichert := cs.getConfigValueDB(satzKonfigSchluessel)
	if gespeichert != cs.satzPlan.kodiert() {
		t.Fatalf("Plan in chain_config %q, im Speicher %q", gespeichert, cs.satzPlan.kodiert())
	}
	// Dieselbe Ankuendigung noch einmal: Wiederholung.
	if w := reiche(gut); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "satz_wiederholt") {
		t.Fatalf("Wiederholung: %d %s", w.Code, w.Body.String())
	}

	// Der Ausgang traegt die Bindungen: ein Knoten ohne Coordinator-Register
	// spielt ihn nach und kommt zum selben Plan.
	rows, err := cs.db.Query(`SELECT tx_json FROM pending_txs WHERE included_at = 0 AND tx_json LIKE '%satz_ankuendigung%'`)
	if err != nil {
		t.Fatal(err)
	}
	var txs []Transaction
	for rows.Next() {
		var s string
		var tx Transaction
		if rows.Scan(&s) != nil || json.Unmarshal([]byte(s), &tx) != nil {
			t.Fatal("Ausgang unlesbar")
		}
		txs = append(txs, tx)
	}
	rows.Close()
	if len(txs) != 1 || txs[0].Satz == nil || txs[0].Satz.Bescheinigungen[0].Mensch == "" {
		t.Fatalf("Ausgang: %+v", txs)
	}
	erwartet := cs.satzPlan
	cs.db.Exec(`DELETE FROM coordinator_keys`)
	cs.mu.Lock()
	cs.satzPlan = satzPlan{}
	err = cs.applySatzAnkuendigungLocked(context.Background(), &txs[0], jetzt+5)
	nachgespielt := cs.satzPlan
	cs.mu.Unlock()
	if err != nil || nachgespielt.kodiert() != erwartet.kodiert() {
		t.Fatalf("nachgespielt: %v %+v, erwartet %+v", err, nachgespielt, erwartet)
	}

	// Nach einem Neustart steht derselbe Plan im Speicher.
	neu := testKnoten(t, "unused-satz-realdb-test-2.json")
	if neu.satzPlan.kodiert() != erwartet.kodiert() {
		t.Fatalf("nach dem Neustart %+v, erwartet %+v", neu.satzPlan, erwartet)
	}
	if s := neu.gleichgewichtSatzZuLocked(ab); math.Abs(s-0.006) > 1e-12 {
		t.Fatalf("Satz nach dem Neustart %v", s)
	}

	// Vor dem Gleichgewicht: 409.
	gleichgewichtAktivOverride.Store(math.MaxInt64)
	if w := reiche(koerper(55, ab, jetzt+1, 55)); w.Code != http.StatusConflict {
		t.Fatalf("vor dem Gleichgewicht: %d", w.Code)
	}
}
