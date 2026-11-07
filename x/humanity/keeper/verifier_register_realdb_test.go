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

// Der Endpunkt /api/verifier und das Register in der Datenbank: Annahme,
// Ausgang mit Bindungen, Nachspielen auf einem frischen Stand, Laden nach
// einem Neustart -- und Missbrauch ueber HTTP.
func TestVerifierRegister_AnnahmeNachspielenLaden_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-verifier-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	jetzt := nowUnix()
	vorherLeser := registerLeserOverride.Load()
	registerLeserOverride.Store(1)
	vorherAb := verifierRegisterOverride.Load()
	verifierRegisterOverride.Store(jetzt - 30*86400)
	vorherGruender := verifierGruender
	t.Cleanup(func() {
		registerLeserOverride.Store(vorherLeser)
		verifierRegisterOverride.Store(vorherAb)
		verifierGruender = vorherGruender
	})
	cs.annehmendAusdruecklich.Store(true)
	if _, err := cs.db.Exec(`DELETE FROM validator_verlauf`); err != nil {
		t.Fatal(err)
	}
	cs.setConfigValueCtx(context.Background(), verifierKonfigSchluessel, "")
	cs.verifierReg.Store(&verifierRegister{})

	var menschen []seBetreiber
	for i := 0; i < 3; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		pubHex := hex.EncodeToString(pub)
		mk, mensch := neuerSchluessel(t)
		registerKonto(t, cs, mensch, true)
		menschen = append(menschen, seBetreiber{priv: priv, pubHex: pubHex, bindung: CoordinatorBindung{Mensch: mensch,
			MenschSig:     personalSign(t, mk, coordinatorFreigabeNachricht(pubHex)),
			SchluesselSig: hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachricht(mensch))))}})
		// Alle drei halten einen Validator-Schluessel; nur 0 ist Gruender.
		verlaufEintrag(t, cs, mensch, distTestAddr(3300+i), jetzt-30*86400)
	}
	verifierGruender = []string{menschen[0].bindung.Mensch}

	koerper := func(art string, betroffener string, issued int64, signiertFuer string, von ...int) []byte {
		var stimmen []map[string]interface{}
		for _, i := range von {
			b := menschen[i]
			sig := ed25519.Sign(b.priv, []byte(verifierNachricht(art, signiertFuer)("", issued)))
			stimmen = append(stimmen, map[string]interface{}{"zeit": issued, "bescheinigung": map[string]string{
				"public_key": b.pubHex, "signature": hex.EncodeToString(sig), "mensch": b.bindung.Mensch,
				"mensch_sig": b.bindung.MenschSig, "schluessel_sig": b.bindung.SchluesselSig}})
		}
		out, _ := json.Marshal(map[string]interface{}{"art": art, "wallet": betroffener, "issued_at": issued, "stimmen": stimmen})
		return out
	}
	reiche := func(body []byte) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		ipBurst.Delete("verifier:192.0.2.1")
		(&APIServer{state: cs}).handleVerifier(w, httptest.NewRequest(http.MethodPost, "/api/verifier", bytes.NewReader(body)))
		return w
	}
	a, b := menschen[1].bindung.Mensch, menschen[2].bindung.Mensch

	// Missbrauch ueber HTTP, vor jeder Annahme.
	for _, m := range []struct {
		name string
		body []byte
		code int
	}{
		{"Faelschung (Stimme fuer einen anderen)", koerper(verifierArtAufnahme, a, jetzt, b, 0), http.StatusForbidden},
		{"Stimme eines Nicht-Pruefers", koerper(verifierArtAufnahme, a, jetzt, a, 2), http.StatusConflict},
		{"Selbststimme", koerper(verifierArtAufnahme, a, jetzt, a, 1), http.StatusConflict},
		{"ohne Stimmen", koerper(verifierArtAufnahme, a, jetzt, a), http.StatusBadRequest},
		{"zu alt", koerper(verifierArtAufnahme, a, jetzt-2*nachweisHoechstensAlt, a, 0), http.StatusBadRequest},
		{"kaputt", []byte(`{"art":`), http.StatusBadRequest},
		{"unbekanntes Feld", []byte(`{"art":"aufnahme","x":1}`), http.StatusBadRequest},
	} {
		if w := reiche(m.body); w.Code != m.code {
			t.Fatalf("%s: %d %s, erwartet %d", m.name, w.Code, w.Body.String(), m.code)
		}
		if !cs.verifierRegisterJetzt().leer() {
			t.Fatalf("%s: Register veraendert", m.name)
		}
	}

	// Gutfall: der Gruender nimmt a auf.
	gut := koerper(verifierArtAufnahme, a, jetzt, a, 0)
	if w := reiche(gut); w.Code != http.StatusOK {
		t.Fatalf("gueltige Aufnahme: %d %s", w.Code, w.Body.String())
	}
	if !cs.verifierRegisterJetzt().mitglied(a, jetzt+erzeugerFrist) {
		t.Fatal("aufgenommen, ab issued_at + erzeugerFrist")
	}
	erwartet := cs.verifierRegisterJetzt().kodiert()
	if g := cs.getConfigValueDB(verifierKonfigSchluessel); g != erwartet {
		t.Fatalf("Register in chain_config %q, im Speicher %q", g, erwartet)
	}
	if w := reiche(gut); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "verifier_") {
		t.Fatalf("Wiederholung: %d %s", w.Code, w.Body.String())
	}

	// Der Ausgang traegt alles: auf einem leeren Register nachgespielt,
	// entsteht dasselbe.
	rows, err := cs.db.Query(`SELECT tx_json FROM pending_txs WHERE included_at = 0 AND tx_json LIKE '%verifier_zulassung%'`)
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
	if len(txs) != 1 || txs[0].Verifier == nil || txs[0].Verifier.Stimmen[0].Bescheinigung.Mensch == "" {
		t.Fatalf("Ausgang: %+v", txs)
	}
	cs.mu.Lock()
	cs.verifierReg.Store(&verifierRegister{})
	err = cs.applyVerifierZulassungLocked(context.Background(), &txs[0], jetzt+5)
	nachgespielt := cs.verifierRegisterJetzt().kodiert()
	cs.mu.Unlock()
	if err != nil || nachgespielt != erwartet {
		t.Fatalf("nachgespielt: %v %s, erwartet %s", err, nachgespielt, erwartet)
	}

	// Nach einem Neustart steht dasselbe Register im Speicher.
	neu := testKnoten(t, "unused-verifier-realdb-test-2.json")
	if neu.verifierRegisterJetzt().kodiert() != erwartet {
		t.Fatalf("nach dem Neustart %s, erwartet %s", neu.verifierRegisterJetzt().kodiert(), erwartet)
	}

	// Die Coordinator-Zulassung der Annahme: b haelt einen Schluessel, ist
	// aber kein Pruefer.
	if ok, err := cs.coordinatorZugelassen(b, jetzt); err != nil || ok {
		t.Fatalf("Schluessel ohne Aufnahme: %v %v", ok, err)
	}
	if ok, err := cs.coordinatorZugelassen(menschen[0].bindung.Mensch, jetzt); err != nil || !ok {
		t.Fatalf("Gruender: %v %v", ok, err)
	}
}
