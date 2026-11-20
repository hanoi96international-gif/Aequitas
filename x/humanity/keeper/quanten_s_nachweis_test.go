package keeper

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Nachweis nach dem Ende (quanten_s_nachweis.go) gegen die echten Regeln und
// die echten Validator-Dienste (in-process): erst nach dem finalen Ende,
// nachrechenbar gegen Wurzel und Proben der Ausgabe, und die Zaehlung an der
// gewaehlten Stelle ist genau die der Annahmestelle.

func qsnBeenden(t *testing.T) (*qsrWelt, QuantenSPaket, *QuantenSAnnahmeErgebnis) {
	t.Helper()
	w := neueQsrWelt(t)
	p := w.ausgeben(t)
	for _, d := range w.q.d {
		if _, err := d.nachweis(p.K); err == nil || !strings.Contains(err.Error(), "lebt") {
			t.Fatalf("Nachweis, solange der Token lebt: %v", err)
		}
	}
	w.annehmen(t, p)
	vl := w.einloesen(t, 1)
	erg, err := quantenSAnnahmeLauf(context.Background(), w.stelle(1), vl, w.q.w.val, w.netz, &w.log)
	if err != nil || erg.Urteil != "eingeloest" {
		t.Fatalf("Annahme: %+v %v\n%s", erg, err, w.log.String())
	}
	return w, p, erg
}

func qsnSammeln(t *testing.T, w *qsrWelt, K string) []QuantenSNachweis {
	t.Helper()
	var nw []QuantenSNachweis
	for x, d := range w.q.d {
		n, err := d.nachweis(K)
		if err != nil {
			t.Fatalf("Validator %d: %v", x+1, err)
		}
		nw = append(nw, *n)
	}
	return jsonWeg(t, nw)
}

func TestQuantenSNachweis_NachDemEnde(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w, p, erg := qsnBeenden(t)
	w.q.takt()
	nw := qsnSammeln(t, w, p.K)
	for x, d := range w.q.d {
		if e := d.eintrag(p.K); e != nil {
			t.Fatalf("Validator %d haelt den Tresor-Eintrag noch", x+1)
		}
	}
	wurzel, _ := quantenWHex32(p.Wurzel)
	ber, err := quantenSNachweisePruefen(p.K, w.q.w.val, nw, &wurzel, p.Proben[:])
	if err != nil {
		t.Fatal(err)
	}
	if ber.WurzelGegen != "kette" || ber.Stelle == nil || *ber.Stelle != 1 || ber.BasenBekannt != p.N {
		t.Fatalf("Bericht: %+v", ber)
	}
	// An der gewaehlten Stelle genau das, was die Annahmestelle (und die
	// Kette) gezaehlt hat.
	if ber.Stichprobe[1] != erg.Stichprobe || ber.TrefferStelle != erg.Treffer || ber.FehlerStelle != erg.Fehler {
		t.Fatalf("Zaehlung an der Stelle: Nachweis %d/%d/%d, Annahme %d/%d/%d", ber.Stichprobe[1], ber.TrefferStelle, ber.FehlerStelle,
			erg.Stichprobe, erg.Treffer, erg.Fehler)
	}
	// Ueber alle Stellen: etwa die Haelfte trifft die Basis (Zufall der Inhaberin).
	if ber.Treffer < p.N/4 || ber.Treffer > 3*p.N/4 || ber.Fehler > ber.Treffer/10 {
		t.Fatalf("Treffer %d, Fehler %d bei n = %d", ber.Treffer, ber.Fehler, p.N)
	}
	// Nichts Geheimes: der Schluessel der Inhaberin steht nirgends.
	roh, _ := json.Marshal(nw)
	if strings.Contains(string(roh), w.dat.Schluessel) {
		t.Fatal("Schluessel der Inhaberin im Nachweis")
	}
}

func TestQuantenSNachweis_ErstNachFinalitaet(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	// Die Finalitaet laeuft einen Block hinterher (wie auf der Kette): beim
	// Ende ist die Hoehe, in der es gesichtet wurde, noch nicht final.
	w := neueQsrWelt(t)
	w.netz.finalNachlauf = 1
	p := w.ausgeben(t)
	w.annehmen(t, p)
	vl := w.einloesen(t, 1)
	if erg, err := quantenSAnnahmeLauf(context.Background(), w.stelle(1), vl, w.q.w.val, w.netz, &w.log); err != nil || erg.Urteil != "eingeloest" {
		t.Fatalf("Annahme: %+v %v\n%s", erg, err, w.log.String())
	}
	if w.q.w.cs.quantenWJetzt().Token[p.K] != nil {
		t.Fatal("Token lebt noch")
	}
	for x, d := range w.q.d {
		if _, err := d.nachweis(p.K); err == nil {
			t.Fatalf("Validator %d legt offen, bevor das Ende final ist", x+1)
		}
		if d.eintrag(p.K) == nil {
			t.Fatalf("Validator %d hat den Eintrag vor dem Nachweis vergessen", x+1)
		}
	}
	w.netz.warten(context.Background()) // ein Block weiter: das Ende ist final
	w.netz.warten(context.Background())
	qsnSammeln(t, w, p.K)
}

func TestQuantenSNachweis_Faelschungen(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w, p, _ := qsnBeenden(t)
	w.q.takt()
	echt := qsnSammeln(t, w, p.K)
	wurzel, _ := quantenWHex32(p.Wurzel)
	kopie := func() []QuantenSNachweis { return jsonWeg(t, echt) }
	neuSigniert := func(nw []QuantenSNachweis, x int) {
		m, _ := quantenSNachweisNachricht(nw[x])
		nw[x].Sig, _ = quantenSUnterschreiben(w.q.w.valKeys[x], m)
	}
	faelle := []struct {
		was     string
		machen  func(nw []QuantenSNachweis) []QuantenSNachweis
		meldung string
	}{
		{"Wert veraendert", func(nw []QuantenSNachweis) []QuantenSNachweis { nw[0].Werte[3] = "1"; return nw }, "nicht von Validator"},
		{"Wert veraendert und neu unterschrieben", func(nw []QuantenSNachweis) []QuantenSNachweis {
			nw[0].Werte[3] = "5"
			neuSigniert(nw, 0)
			return nw
		}, "nicht die Wurzel"},
		{"Salz veraendert und neu unterschrieben", func(nw []QuantenSNachweis) []QuantenSNachweis {
			nw[1].Salze[0] = strings.Repeat("ab", 32)
			neuSigniert(nw, 1)
			return nw
		}, "Proben"},
		{"alle Validatoren erfinden eine andere Praeparation", func(nw []QuantenSNachweis) []QuantenSNachweis {
			// Die Werte aller Abschnitte geaendert, Wurzel und Proben passend
			// neu gerechnet und von jedem neu unterschrieben -- nur die Wurzel
			// der Ausgabe auf der Kette verraet es.
			for x := range nw {
				nw[x].Werte[0] = "4"
			}
			ber, err := qsnOhneSoll(p.K, w.q.w.val, nw)
			if err == nil {
				t.Fatal("ohne neue Wurzel angenommen")
			}
			_ = ber
			var blaetter [][32]byte
			for x := range nw {
				for i, wh := range nw[x].Werte {
					wv, _ := new(big.Int).SetString(wh, 16)
					blaetter = append(blaetter, quantenWPruefsumme(p.K, nw[x].Von+i, wv))
				}
			}
			neu := quantenWMerkleWurzel(blaetter)
			for x := range nw {
				nw[x].Wurzel = hex.EncodeToString(neu[:])
			}
			proben, _ := qsnProbenAus(p.K, nw)
			for x := range nw {
				nw[x].Proben = proben
				neuSigniert(nw, x)
			}
			return nw
		}, "BEFUND"},
		{"Unterschrift eines anderen Validators", func(nw []QuantenSNachweis) []QuantenSNachweis {
			m, _ := quantenSNachweisNachricht(nw[0])
			nw[0].Sig, _ = quantenSUnterschreiben(w.q.w.valKeys[1], m)
			return nw
		}, "nicht von Validator"},
		{"Reihenfolge vertauscht", func(nw []QuantenSNachweis) []QuantenSNachweis { nw[0], nw[1] = nw[1], nw[0]; return nw }, "Abschnitt"},
		{"ein Nachweis fehlt", func(nw []QuantenSNachweis) []QuantenSNachweis { return nw[:len(nw)-1] }, "Nachweise fuer"},
		{"andere Wahl in einem Nachweis", func(nw []QuantenSNachweis) []QuantenSNachweis {
			c := 1 - *nw[2].C
			nw[2].C = &c
			neuSigniert(nw, 2)
			return nw
		}, "verschiedene Wahlen"},
		{"Bindung zu kurz", func(nw []QuantenSNachweis) []QuantenSNachweis {
			nw[1].D = nw[1].D[1:]
			neuSigniert(nw, 1)
			return nw
		}, "Bindung oder Wahl"},
		{"x der Inhaberin zu kurz", func(nw []QuantenSNachweis) []QuantenSNachweis {
			nw[0].XInhaberin = nw[0].XInhaberin[1:]
			neuSigniert(nw, 0)
			return nw
		}, "Laengen"},
	}
	for _, f := range faelle {
		if _, err := quantenSNachweisePruefen(p.K, w.q.w.val, f.machen(kopie()), &wurzel, p.Proben[:]); err == nil || !strings.Contains(err.Error(), f.meldung) {
			t.Errorf("%s: %v (erwartet %q)", f.was, err, f.meldung)
		}
	}
	// Proben der Ausgabe anders: BEFUND, auch wenn alles in sich stimmt.
	if _, err := quantenSNachweisePruefen(p.K, w.q.w.val, kopie(), &wurzel, []string{p.Proben[1], p.Proben[0]}); err == nil || !strings.Contains(err.Error(), "BEFUND") {
		t.Errorf("fremde Proben: %v", err)
	}
	// Nennen die Validatoren verschiedene Stellen: keine Zaehlung nach Basen
	// (statt einer falschen), und so gekennzeichnet.
	uneins := kopie()
	null := 0
	uneins[2].Stelle = &null
	neuSigniert(uneins, 2)
	if ber, err := quantenSNachweisePruefen(p.K, w.q.w.val, uneins, &wurzel, p.Proben[:]); err != nil || !ber.StellenUneinheitlich || ber.Stelle != nil || ber.BasenBekannt != 0 {
		t.Errorf("verschiedene Stellen: %+v %v", ber, err)
	}
	// Auch ohne Soll von der Kette: Salze, die nicht die Proben ergeben, die
	// der Validator selbst nennt, fallen auf.
	salz := kopie()
	salz[1].Salze[1] = strings.Repeat("cd", 32)
	neuSigniert(salz, 1)
	if _, err := quantenSNachweisePruefen(p.K, w.q.w.val, salz, nil, nil); err == nil || !strings.Contains(err.Error(), "die der Validator nennt") {
		t.Errorf("Salz ohne Soll: %v", err)
	}
	// Ohne Wurzel der Ausgabe: nur in sich geprueft, und so gekennzeichnet.
	if ber, err := quantenSNachweisePruefen(p.K, w.q.w.val, kopie(), nil, nil); err != nil || ber.WurzelGegen != "" {
		t.Errorf("ohne Soll: %+v %v", ber, err)
	}
}

func TestQuantenSNachweis_GrenzeUndEndpunkt(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w, p, _ := qsnBeenden(t)
	w.q.takt()
	d := w.q.d[0]
	n, err := d.nachweis(p.K)
	if err != nil {
		t.Fatal(err)
	}
	// Hoechstens quantenSNachweiseMax: die aeltesten fallen heraus.
	for i := 0; i < quantenSNachweiseMax; i++ {
		c := *n
		c.K = strings.Repeat("0", 63) + string("0123456789abcdef"[i%16])
		if i >= 16 {
			c.K = strings.Repeat("1", 62) + string("0123456789abcdef"[i/16]) + string("0123456789abcdef"[i%16])
		}
		d.nachweisMerken(&c)
	}
	if _, err := d.nachweis(p.K); err == nil {
		t.Fatal("der aelteste Nachweis ist nicht herausgefallen")
	}
	d.mu.Lock()
	if len(d.nachweise) != quantenSNachweiseMax || len(d.nachweisReihe) != quantenSNachweiseMax {
		t.Fatalf("%d Nachweise, %d in der Reihe", len(d.nachweise), len(d.nachweisReihe))
	}
	d.nachweise[p.K] = n // fuer den Endpunkt wieder hinein
	d.mu.Unlock()

	alt := w.q.w.cs.quantenWDienstZeiger.Load()
	w.q.w.cs.quantenWDienstZeiger.Store(d)
	defer w.q.w.cs.quantenWDienstZeiger.Store(alt)
	altAb := quantenWAbOverride.Load()
	quantenWAbOverride.Store(1)
	defer quantenWAbOverride.Store(altAb)
	api := &APIServer{state: w.q.w.cs}
	frage := func(methode, k, von string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(methode, "/api/quanten-w/validator/nachweis?k="+k, nil)
		req.RemoteAddr = von
		rec := httptest.NewRecorder()
		api.handleQuantenSNachweis(rec, req)
		return rec
	}
	if rec := frage(http.MethodPost, p.K, "198.51.100.60:1"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", rec.Code)
	}
	if rec := frage(http.MethodGet, "zz", "198.51.100.61:1"); rec.Code != http.StatusBadRequest {
		t.Fatalf("K ohne Form: %d", rec.Code)
	}
	if rec := frage(http.MethodGet, strings.Repeat("9", 64), "198.51.100.62:1"); rec.Code != http.StatusNotFound {
		t.Fatalf("unbekanntes K: %d", rec.Code)
	}
	rec := frage(http.MethodGet, p.K, "198.51.100.63:1")
	var got QuantenSNachweis
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.Sig != n.Sig {
		t.Fatalf("Nachweis ueber den Endpunkt: %d %s", rec.Code, rec.Body.String())
	}
	// Begrenzt wie die anderen Anfragen an den Dienst.
	begrenzt := false
	for i := 0; i < 200 && !begrenzt; i++ {
		begrenzt = frage(http.MethodGet, p.K, "198.51.100.64:1").Code == http.StatusTooManyRequests
	}
	if !begrenzt {
		t.Fatal("keine Grenze je Absender")
	}
	// Ein Nachweis zu einem Token, der (wieder) lebt, geht nie heraus.
	w2 := neueQsrWelt(t)
	p2 := w2.ausgeben(t)
	c := *n
	c.K = p2.K
	w2.q.d[0].nachweisMerken(&c)
	if _, err := w2.q.d[0].nachweis(p2.K); err == nil || !strings.Contains(err.Error(), "lebt") {
		t.Fatalf("Nachweis zu einem lebenden Token: %v", err)
	}
}

func qsnOhneSoll(K string, val []string, nw []QuantenSNachweis) (*QuantenSNachweisBericht, error) {
	return quantenSNachweisePruefen(K, val, nw, nil, nil)
}

// qsnProbenAus: die Proben aus Werten und Salzen (wie der Pruefer).
func qsnProbenAus(K string, nw []QuantenSNachweis) ([]string, error) {
	var teile [2][][32]byte
	for x, a := range nw {
		var stellen [2][]int
		for i, wh := range a.Werte {
			w, _ := new(big.Int).SetString(wh, 16)
			stellen[quantenSStellenBit(w)] = append(stellen[quantenSStellenBit(w)], a.Von+i)
		}
		for s := 0; s < 2; s++ {
			salz, _ := quantenWHex32(a.Salze[s])
			teile[s] = append(teile[s], quantenSProbeTeilHash(K, s, x+1, salz, stellen[s]))
		}
	}
	var out []string
	for s := 0; s < 2; s++ {
		h, err := quantenSProbeHashV2(K, s, teile[s])
		if err != nil {
			return nil, err
		}
		out = append(out, hex.EncodeToString(h[:]))
	}
	return out, nil
}

// Ueber HTTP wie im Betrieb: je Validator ein Server (der Endpunkt des
// Dienstes), die Annahmestelle wartet, bis alle offenlegen, und prueft.
func TestQuantenSNachweis_AbwartenUndBefehl(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	alt := quantenSNachweisTakt
	quantenSNachweisTakt = time.Millisecond
	defer func() { quantenSNachweisTakt = alt }()
	w, p, erg := qsnBeenden(t)
	var noch atomic.Int32
	noch.Store(3) // die ersten Anfragen: noch nicht final
	vertauscht := false
	var vs []QuantenSValidator
	for x, d := range w.q.d {
		d := d
		x := x
		srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if noch.Add(-1) >= 0 {
				http.Error(rw, `{"error":"kein Nachweis"}`, http.StatusNotFound)
				return
			}
			if r.URL.Path != "/api/quanten-w/validator/nachweis" {
				http.NotFound(rw, r)
				return
			}
			dd := d
			if vertauscht && x == 1 {
				dd = w.q.d[2] // Validator 2 liefert den Nachweis von Validator 3
			}
			n, err := dd.nachweis(r.URL.Query().Get("k"))
			if err != nil {
				http.Error(rw, err.Error(), http.StatusNotFound)
				return
			}
			json.NewEncoder(rw).Encode(n)
		}))
		defer srv.Close()
		vs = append(vs, QuantenSValidator{Adresse: w.q.w.val[x], API: srv.URL})
	}
	adressen, netze, err := quantenSValidatorenPruefen(vs, 1)
	if err != nil {
		t.Fatal(err)
	}
	netz := &quantenSMehrNetz{validatoren: netze}
	w.q.takt() // das Ende ist final: jeder legt offen
	var log bytes.Buffer
	ber, err := quantenSNachweisAbwarten(context.Background(), netz, p.K, adressen, erg.Wurzel, erg.Proben, time.Now().Add(time.Minute), &log)
	if err != nil || ber.WurzelGegen != "kette" || ber.TrefferStelle != erg.Treffer || ber.FehlerStelle != erg.Fehler {
		t.Fatalf("Abwarten: %+v %v", ber, err)
	}
	if !strings.Contains(log.String(), "passt zur Wurzel der Ausgabe") {
		t.Fatalf("Protokoll: %s", log.String())
	}
	// Der Befehl fuer jeden: mit Wurzel und Proben, als JSON.
	var aus bytes.Buffer
	if err := QuantenSNachweisPruefenBefehl(context.Background(), p.K, vs, erg.Wurzel, erg.Proben, &aus); err != nil || !strings.Contains(aus.String(), `"wurzel_gegen": "kette"`) {
		t.Fatalf("Befehl: %v %s", err, aus.String())
	}
	// Eine andere Wurzel: BEFUND.
	if err := QuantenSNachweisPruefenBefehl(context.Background(), p.K, vs, strings.Repeat("00", 32), nil, &aus); err == nil || !strings.Contains(err.Error(), "BEFUND") {
		t.Fatalf("fremde Wurzel: %v", err)
	}
	// Ein Validator liefert den Nachweis eines anderen: abgewiesen.
	vertauscht = true
	if _, err := quantenSNachweisAbwarten(context.Background(), netz, p.K, adressen, erg.Wurzel, erg.Proben, time.Now(), &log); err == nil {
		t.Fatal("vertauschter Nachweis angenommen")
	}
	// Ohne Nachweis bis zur Frist: Fehler, kein Bericht.
	vertauscht = false
	noch.Store(1 << 20)
	if ber, err := quantenSNachweisAbwarten(context.Background(), netz, p.K, adressen, erg.Wurzel, erg.Proben, time.Now().Add(20*time.Millisecond), &log); err == nil || ber != nil {
		t.Fatal("ohne Nachweis ein Bericht")
	}
}

// Steht die Finalitaet still, legt niemand offen -- und der Eintrag wird nach
// dem Praegefenster trotzdem vergessen (kein Tresor, der vollaeuft).
func TestQuantenSNachweis_FinalitaetSteht(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w := neueQsrWelt(t)
	w.netz.finalNachlauf = 1
	p := w.ausgeben(t)
	w.annehmen(t, p)
	vl := w.einloesen(t, 1)
	if erg, err := quantenSAnnahmeLauf(context.Background(), w.stelle(1), vl, w.q.w.val, w.netz, &w.log); err != nil || erg.Urteil != "eingeloest" {
		t.Fatalf("Annahme: %+v %v", erg, err)
	}
	w.q.hoehe.Store(w.q.hoehe.Load() + 100) // weiter, aber nichts mehr final
	w.q.takt()
	for x, d := range w.q.d {
		if _, err := d.nachweis(p.K); err == nil || d.eintrag(p.K) == nil {
			t.Fatalf("Validator %d: offengelegt oder vergessen, obwohl das Ende nicht final ist", x+1)
		}
	}
	w.q.zeit += quantenWToteBehalten + 1
	w.q.takt()
	for x, d := range w.q.d {
		if _, err := d.nachweis(p.K); err == nil {
			t.Fatalf("Validator %d legt ohne Finalitaet offen", x+1)
		}
		if d.eintrag(p.K) != nil {
			t.Fatalf("Validator %d haelt den Eintrag nach dem Praegefenster noch", x+1)
		}
	}
}
