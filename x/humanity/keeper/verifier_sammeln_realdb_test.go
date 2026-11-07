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
	"sort"
	"testing"
)

// Die Sammelstelle reicht selbst ein, sobald genug Pruefer gestimmt haben --
// mit derselben Pruefung wie beim Nachspielen. Eine Stimme, die erst beim
// Einreichen durchfaellt, blockiert die Abstimmung nicht.
func TestVerifierSammeln_ReichtSelbstEin_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-verifier-sammeln-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	vsFrisch(t)
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

	// 0 Gruender, 1 Pruefer, 2 Pruefer OHNE Validator-Schluessel (seine Stimme
	// faellt erst beim Einreichen durch), 3 Kandidat.
	var menschen []seBetreiber
	for i := 0; i < 4; i++ {
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
		if i != 2 {
			verlaufEintrag(t, cs, mensch, distTestAddr(3400+i), jetzt-30*86400)
		}
	}
	m := func(i int) string { return menschen[i].bindung.Mensch }
	verifierGruender = []string{m(0)}
	reg := &verifierRegister{}
	for _, i := range []int{1, 2} {
		reg.Eintraege = append(reg.Eintraege, verifierEintrag{Mensch: m(i), Seit: jetzt - 10*86400, Zuletzt: jetzt - 11*86400})
	}
	sort.Slice(reg.Eintraege, func(a, b int) bool { return reg.Eintraege[a].Mensch < reg.Eintraege[b].Mensch })
	cs.verifierReg.Store(reg)

	stimme := func(von int, zeit int64) VerifierStimme {
		b := menschen[von]
		sig := ed25519.Sign(b.priv, []byte(verifierNachricht(verifierArtAufnahme, m(3))("", zeit)))
		return VerifierStimme{Zeit: zeit, Bescheinigung: Lebendigkeitsbescheinigung{PublicKey: b.pubHex,
			Signature: hex.EncodeToString(sig), Mensch: b.bindung.Mensch, MenschSig: b.bindung.MenschSig,
			SchluesselSig: b.bindung.SchluesselSig}}
	}
	a := &APIServer{state: cs}
	senden := func(von int) map[string]interface{} {
		t.Helper()
		ipBurst.Delete("verifier:192.0.2.1")
		body, _ := json.Marshal(map[string]interface{}{"art": verifierArtAufnahme, "wallet": m(3), "stimme": stimme(von, nowUnix())})
		w := httptest.NewRecorder()
		a.handleVerifierStimme(w, httptest.NewRequest(http.MethodPost, "/api/verifier/stimme", bytes.NewReader(body)))
		if w.Code != http.StatusOK {
			t.Fatalf("Stimme von %d: %d %s", von, w.Code, w.Body.String())
		}
		var out map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	verifierSammel.antraege[m(3)] = jetzt

	// Drei Pruefer: noetig sind zwei.
	if out := senden(2); out["eingereicht"] != false || out["noetig"] != float64(2) {
		t.Fatalf("erste Stimme: %v", out)
	}
	// Zweite Stimme: zwei da, aber die von 2 faellt beim Einreichen durch
	// (kein Validator-Schluessel) -- verworfen, nicht eingereicht.
	if out := senden(0); out["eingereicht"] != false {
		t.Fatalf("mit untauglicher Stimme eingereicht: %v", out)
	}
	if s := verifierSammel.offen[verifierArtAufnahme+"|"+m(3)]; s == nil || len(s.stimmen) != 1 {
		t.Fatal("die untaugliche Stimme haette verworfen, die gute behalten werden muessen")
	}
	if cs.verifierRegisterJetzt().mitglied(m(3), jetzt+erzeugerFrist+60) {
		t.Fatal("aufgenommen ohne genug taugliche Stimmen")
	}
	// Die Stimme von 1: jetzt zwei taugliche -- der Knoten reicht selbst ein.
	if out := senden(1); out["eingereicht"] != true {
		t.Fatalf("dritte Stimme: %v", out)
	}
	if !cs.verifierRegisterJetzt().mitglied(m(3), nowUnix()+erzeugerFrist+60) {
		t.Fatal("nicht aufgenommen")
	}
	if len(verifierSammel.offen) != 0 || len(verifierSammel.antraege) != 0 {
		t.Fatal("Abstimmung und Antrag nach der Aufnahme nicht weggeraeumt")
	}

	// Im Ausgang: eine Transaktion mit genau den zwei tauglichen Stimmen, die
	// auf einem frischen Stand dasselbe ergibt.
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
	if len(txs) != 1 || txs[0].Verifier == nil || len(txs[0].Verifier.Stimmen) != 2 {
		t.Fatalf("Ausgang: %+v", txs)
	}
	erwartet := cs.verifierRegisterJetzt().kodiert()
	cs.mu.Lock()
	cs.verifierReg.Store(reg)
	err = cs.applyVerifierZulassungLocked(context.Background(), &txs[0], txs[0].DistributionAt+5)
	nachgespielt := cs.verifierRegisterJetzt().kodiert()
	cs.mu.Unlock()
	if err != nil || nachgespielt != erwartet {
		t.Fatalf("nachgespielt: %v %s, erwartet %s", err, nachgespielt, erwartet)
	}
}
