package keeper

import (
	"bytes"
	"context"
	"math/rand"
	"strings"
	"testing"
)

// Die Probe zaehlt wie die Kette (quantenWUrteil): je Stelle bestehen,
// beide Stellen, Grenze genau auf der Toleranz, unter 4 Treffern nicht.
func TestQuantenSEhrlichBesteht(t *testing.T) {
	for name, f := range map[string]struct {
		treffer, fehler [2]int
		tol             int
		ok              bool
	}{
		"beide ohne Fehler":             {[2]int{880, 870}, [2]int{0, 0}, 2, true},
		"genau auf der Grenze":          {[2]int{100, 100}, [2]int{2, 2}, 2, true},
		"Stelle 0 einen Fehler zu viel": {[2]int{100, 100}, [2]int{3, 2}, 2, false},
		"Stelle 1 einen Fehler zu viel": {[2]int{100, 100}, [2]int{2, 3}, 2, false},
		"4 % lassen 4 von 100 zu":       {[2]int{100, 100}, [2]int{4, 4}, 4, true},
		"4 % lassen 5 von 100 nicht zu": {[2]int{100, 100}, [2]int{0, 5}, 4, false},
		"zu wenige Treffer":             {[2]int{3, 100}, [2]int{0, 0}, 4, false},
	} {
		err := quantenSEhrlichBesteht(f.treffer, f.fehler, f.tol)
		if (err == nil) != f.ok {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

// qsVerrauschen: ein Anteil prozent der x bei passender Basis wird falsch.
func qsVerrauschen(m *QuantenSMessung, prozent int, seed int64) {
	u, tw, y, x := []byte(m.U), []byte(m.T), []byte(m.Y), []byte(m.X)
	r := rand.New(rand.NewSource(seed))
	for i := range x {
		if y[i] == u[i] && r.Intn(100) < prozent {
			x[i] = '0' + '1' - tw[i]
		}
	}
	m.X = string(x)
}

// Missbrauch im Werkzeug: eine Messung, deren ehrliche Vorlage nicht bestuende
// -- Abbruch, bevor ein Validator bestaetigt oder etwas auf die Kette geht.
// Bei 2 % und auch beim Standard (4 %).
func TestQuantenSAusgabe_EhrlichNichtBestanden(t *testing.T) {
	for _, tol := range []int{2, QuantenSToleranzStandard} {
		q, a := qsWerkzeugWelt(t, 256)
		a.Tol = tol
		qsVerrauschen(&a.Messung, 20, 9)
		w := q.w
		vorher := w.stand(w.cs, w.val[0])
		var log bytes.Buffer
		_, err := quantenSAusgabeLauf(context.Background(), a, w.val, w.p.t, &qsNetz{q: q}, &log)
		if err == nil || !strings.Contains(err.Error(), "ehrliche Vorlage bestuende") {
			t.Fatalf("Toleranz %d: ausgegeben trotz Messfehler: %v\n%s", tol, err, log.String())
		}
		if len(w.cs.quantenWJetzt().Token) != 0 || w.stand(w.cs, w.val[0]) != vorher {
			t.Fatalf("Toleranz %d: trotz Abbruch etwas auf der Kette", tol)
		}
		for _, d := range q.d {
			if n := len(d.eintraege); n != 0 {
				t.Fatalf("Toleranz %d: ein Validator hat trotz Abbruch %d Eintraege", tol, n)
			}
		}
	}
}

// Dasselbe im Abschnitts-Modus: Treffer und Fehler aus den geladenen
// Stichproben.
func TestQuantenSAusgabeAbschnitte_EhrlichNichtBestanden(t *testing.T) {
	for _, tol := range []int{2, QuantenSToleranzStandard} {
		q, a, bs, mess := qsaWerkzeug(t, 256)
		a.Tol = tol
		for _, m := range mess {
			m.verrauscht = 20
		}
		_, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, &qsaNetz{qsNetz: &qsNetz{q: q}}, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "ehrliche Vorlage bestuende") {
			t.Fatalf("Toleranz %d: ausgegeben trotz Messfehler: %v", tol, err)
		}
		if len(q.w.cs.quantenWJetzt().Token) != 0 {
			t.Fatalf("Toleranz %d: trotz Abbruch ausgegeben", tol)
		}
	}
}

// Gutfall mit 2 % und der ECHTEN Tabelle (3 von 5: 2 % verlangt 1.408,
// 4 % verlangt 3.472 Stellen je Annahmestelle): mit 1.760 Stellen gibt das
// Werkzeug bei 2 % aus -- Doppelvorlage abgewiesen, ehrliche Vorlage
// eingeloest --, bei 4 % lehnt es dieselbe Groesse ab. Zwei falsche x (wie
// der Rest mit der Mehrheit aus 5 Schuessen auf dem Chip) bestehen.
func TestQuantenSAusgabe_Toleranz2Gutfall(t *testing.T) {
	if testing.Short() {
		t.Skip("3.520 Stellen")
	}
	q, a := qsWerkzeugWelt(t, 3520)
	quantenWKNoteOverride.Store(0) // die echte Tabelle (qwLabor raeumt auf)
	if quantenWKNote(2, q.w.p.t, len(q.w.val)) != 1408 || quantenWKNote(4, q.w.p.t, len(q.w.val)) != 3472 {
		t.Fatal("Tabelle: andere Pruefgroessen als erwartet")
	}
	m := &a.Messung
	x := []byte(m.X)
	for i, gekippt := 0, 0; i < len(x) && gekippt < 2; i++ {
		if m.Y[i] == m.U[i] {
			x[i] = '0' + '1' - m.T[i]
			gekippt++
		}
	}
	m.X = string(x)
	w := q.w
	if _, err := quantenSAusgabeLauf(context.Background(), a, w.val, w.p.t, &qsNetz{q: q}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "Pruefgroesse") {
		t.Fatalf("4 %% mit 1.760 Stellen angenommen: %v", err)
	}
	a.Tol = 2
	bz := [2]int{1, 0}
	a.bz = &bz
	var log bytes.Buffer
	erg, err := quantenSAusgabeLauf(context.Background(), a, w.val, w.p.t, &qsNetz{q: q}, &log)
	if err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if erg.Doppelvorlage != "abgewiesen" || erg.Vorlage != "eingeloest" || erg.KJeStelle != 1760 || erg.Toleranz != 2 {
		t.Fatalf("Ergebnis: %+v", erg)
	}
	if _, tot := w.cs.quantenWJetzt().Tote[erg.K]; !tot {
		t.Fatal("eingeloester Token nicht tot")
	}
}
