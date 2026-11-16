package keeper

import (
	"encoding/hex"
	"strings"
	"testing"
)

// Validator-Dienst fuer Variante S (quanten_w_validator.go): Bestaetigen der
// Ausgabe, Anteile erst fuer eine FINALE Anmeldung, Offenlegen in Teilen und
// Frist ueber den Hintergrund.

type qsDienste struct {
	*qwDienste
	s *qsWelt
}

func neueQsDienste(t *testing.T, n, probeLaenge int) *qsDienste {
	t.Helper()
	q := neueQwDienste(t, n)
	s := &qsWelt{qwWelt: q.w}
	s.vorbereiten(probeLaenge)
	return &qsDienste{qwDienste: q, s: s}
}

// anfrageS: der Auftrag an Validator x (1-basiert), vom Ausgeber unterschrieben.
func (q *qsDienste) anfrageS(x int) QuantenWBestaetigenAnfrage {
	p, w := q.w.p, q.w
	r := QuantenWBestaetigenAnfrage{K: p.K, N: p.n, T: p.t, Tol: qsTol, X: x, Wurzel: hex.EncodeToString(p.wurzel[:]),
		Validatoren: w.val, Zeit: q.zeit, Art: "s", Proben: q.s.probenHex(), Praeger: w.praeger}
	for i := 0; i < p.n; i++ {
		r.Anteile = append(r.Anteile, p.anteile[i][x-1].Text(16))
		r.Blaetter = append(r.Blaetter, hex.EncodeToString(p.blaetter[i][:]))
	}
	r.PraegerSig = personalSign(w.t, w.keys[w.praeger], quantenWDienstAuftrag(r))
	return r
}

// ausgeben: alle Validatoren bestaetigen ueber den Dienst, dann die Kette.
func (q *qsDienste) ausgeben(t *testing.T) {
	t.Helper()
	tx := q.s.ausgebenTx(q.zeit, 10_000_000)
	for x := range q.w.val {
		anker, sig, err := q.d[x].bestaetigen(q.anfrageS(x + 1))
		if err != nil || anker != "" {
			t.Fatalf("Validator %d bestaetigt die S-Ausgabe nicht: %v (Anker %q)", x+1, err, anker)
		}
		tx.QuantenW.Bestaetigungen[x] = sig
	}
	q.w.annehmen(tx, q.zeit+1)
	q.w.annehmen(q.s.bindenTx(q.w.anna), q.zeit+2)
	q.w.annehmen(q.s.waehlenTx(q.w.anna, "1"), q.zeit+3)
}

func TestQuantenWSDienst_Gutfall(t *testing.T) {
	q := neueQsDienste(t, 128, 80) // 80 Stellen: zwei Teile
	s, w := q.s, q.w
	q.ausgeben(t)
	for _, d := range q.d {
		if _, err := d.anteile(w.p.K, 0, 0); err == nil {
			t.Fatal("Anteile ohne Anmeldung herausgegeben")
		}
	}
	q.zeit += 10
	w.annehmen(s.anmeldenTx(s.stelle[0], 0, s.probe[0], s.vorlage(0, w.keys[w.anna])), q.zeit)
	// Noch nicht final: kein Anteil, keine Offenlegung.
	q.hoehe.Store(10)
	q.final.Store(5)
	q.zeit += 5
	q.takt()
	if a := s.token(w.cs).Anmeldung; a == nil || a.Offen != 0 {
		t.Fatal("vor der Finalitaet offengelegt")
	}
	if _, err := q.d[1].anteile(w.p.K, 0, 0); err == nil || !strings.Contains(err.Error(), "final") {
		t.Fatalf("Anteil vor der Finalitaet: %v", err)
	}
	// Final: ein Validator sammelt und legt in zwei Takten offen. Anteile
	// nur fuer den laufenden Versuch (Nummer = bisherige Fehlversuche).
	q.final.Store(10)
	if _, err := q.d[1].anteile(w.p.K, 1, 0); err == nil {
		t.Fatal("Anteile fuer einen anderen Versuch herausgegeben")
	}
	if _, err := q.d[1].anteile(w.p.K, 0, len(s.probe[0])); err == nil {
		t.Fatal("Anteile hinter der Stichprobe herausgegeben")
	}
	q.takt() // der Zustaendige reicht beide Teile in einem Takt ein
	if s.token(w.cs) != nil || w.stand(w.cs, s.stelle[0]) != 10_000_000 {
		t.Fatalf("Einloesen nicht durch: Stelle %d", w.stand(w.cs, s.stelle[0]))
	}
	// Der eingeloeste Token verschwindet aus dem Tresor.
	q.takt()
	for _, d := range q.d {
		if len(d.eintraege) != 0 {
			t.Fatal("eingeloester Token im Tresor")
		}
	}
}

// Legen die Validatoren nicht in der Frist offen (hier: kein Nachbar
// erreichbar, t = 3), reicht der Dienst die Frist ein; ohne Fehlversuch.
func TestQuantenWSDienst_Frist(t *testing.T) {
	q := neueQsDienste(t, 64, 24)
	s, w := q.s, q.w
	q.ausgeben(t)
	for _, d := range q.d {
		d.peers = nil
	}
	q.zeit += 10
	w.annehmen(s.anmeldenTx(s.stelle[0], 0, s.probe[0], s.vorlage(0, w.keys[w.anna])), q.zeit)
	q.zeit += 5
	q.takt()
	if a := s.token(w.cs).Anmeldung; a == nil || a.Offen != 0 {
		t.Fatal("ohne Nachbarn offengelegt")
	}
	q.zeit += quantenWFristOffen
	q.takt()
	if tok := s.token(w.cs); tok == nil || tok.Anmeldung != nil || tok.Fehl != 0 {
		t.Fatalf("Frist nicht eingereicht: %+v", tok)
	}
	// Lebende Token bleiben im Tresor, auch lange nach der Ausgabe.
	q.zeit += quantenWToteBehalten + 10
	q.takt()
	for x, d := range q.d {
		if len(d.eintraege) != 1 {
			t.Fatalf("Validator %d hat den lebenden Token vergessen", x+1)
		}
	}
}

func TestQuantenWSDienst_BestaetigenMissbrauch(t *testing.T) {
	q := neueQsDienste(t, 64, 24)
	d := q.d[0]
	gut := q.anfrageS(1)
	resign := func(r QuantenWBestaetigenAnfrage) QuantenWBestaetigenAnfrage {
		r.PraegerSig = personalSign(t, q.w.keys[q.w.praeger], quantenWDienstAuftrag(r))
		return r
	}
	for name, r := range map[string]QuantenWBestaetigenAnfrage{
		"Stichprobe ausgetauscht": func() QuantenWBestaetigenAnfrage {
			r := gut
			r.Proben = []string{gut.Proben[1], gut.Proben[0]}
			return r
		}(),
		"als W-Auftrag unterschrieben": func() QuantenWBestaetigenAnfrage {
			r := gut
			r.PraegerSig = personalSign(t, q.w.keys[q.w.praeger], quantenWAuftragNachricht(r.Praeger, r.K, r.N, r.T, r.Tol, r.X, r.Wurzel, r.Validatoren, r.Zeit, quantenWAnteileHash(r.Anteile)))
			return r
		}(),
		"unbekannte Art":      func() QuantenWBestaetigenAnfrage { r := gut; r.Art = "x"; return resign(r) }(),
		"eine Stichprobe":     func() QuantenWBestaetigenAnfrage { r := gut; r.Proben = r.Proben[:1]; return resign(r) }(),
		"W mit Stichproben":   func() QuantenWBestaetigenAnfrage { r := gut; r.Art = ""; return resign(r) }(),
		"Toleranz ausserhalb": func() QuantenWBestaetigenAnfrage { r := gut; r.Tol = 7; return resign(r) }(),
		"Validator doppelt": func() QuantenWBestaetigenAnfrage {
			r := gut
			r.Validatoren = append([]string(nil), gut.Validatoren...)
			r.Validatoren[1] = r.Validatoren[0]
			return resign(r)
		}(),
	} {
		if _, _, err := d.bestaetigen(r); err == nil {
			t.Errorf("%s: bestaetigt", name)
		}
	}
	if len(d.eintraege) != 0 {
		t.Fatal("abgewiesene Bestaetigung abgelegt")
	}
	_, sig1, err := d.bestaetigen(gut)
	if err != nil {
		t.Fatalf("Gegenprobe: %v", err)
	}
	if _, sig2, err := d.bestaetigen(gut); err != nil || sig2 != sig1 {
		t.Fatalf("Wiederholung: %v", err)
	}
	// Dieselbe K als W-Auftrag: anderer Auftrag, abgewiesen.
	w := q.anfrageBestaetigen(1)
	if _, _, err := d.bestaetigen(w); err == nil {
		t.Fatal("W-Auftrag fuer ein schon bestaetigtes S-K")
	}
}
