package keeper

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
)

// Getrennte Rollen (quanten_s_rollen.go) gegen die echten Regeln und den
// echten Validator-Dienst (in-process, alles durch JSON): Inhaberin,
// Ausgeber und zwei Annahmestellen haben je ihren eigenen Schluessel.

type qsrWelt struct {
	q       *qwDienste
	a       QuantenSAusgabeAuftrag // Ausgeber (Validator 1)
	netz    *qsv2Netz
	dat     *QuantenSInhaberinDatei
	stellen [2]*ecdsa.PrivateKey
	log     bytes.Buffer
}

func neueQsrWelt(t *testing.T) *qsrWelt {
	t.Helper()
	q, a, _, _ := qsaWerkzeug(t, 256)
	a.ProbeV2 = true
	w := &qsrWelt{q: q, a: a, netz: &qsv2Netz{qsaNetz: &qsaNetz{qsNetz: &qsNetz{q: q}}}}
	var st [2]QuantenSStelle
	for s := 0; s < 2; s++ {
		k, adr := neuerSchluessel(t)
		w.stellen[s] = k
		st[s] = QuantenSStelle{Adresse: adr, Schluessel: "0x" + strings.ToUpper(pubHex(k))} // Form wird vereinheitlicht
	}
	dat, err := quantenSInhaberinNeu(256, len(q.w.val), st)
	if err != nil {
		t.Fatal(err)
	}
	w.dat = dat
	return w
}

// jsonWeg: wie ueber eine Datei (strenges Lesen, keine unbekannten Felder).
func jsonWeg[T any](t *testing.T, v T) T {
	t.Helper()
	roh, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := quantenSJSONLesen(bytes.NewReader(roh), "Test", &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (w *qsrWelt) ausgeben(t *testing.T) QuantenSPaket {
	t.Helper()
	p, err := quantenSAusgabeFuerInhaberin(context.Background(), w.a, w.dat.Bestellung, w.q.w.val, 3, w.netz, &w.log)
	if err != nil {
		t.Fatalf("Ausgabe: %v\n%s", err, w.log.String())
	}
	return jsonWeg(t, *p)
}

func (w *qsrWelt) annehmen(t *testing.T, p QuantenSPaket) {
	t.Helper()
	if err := quantenSInhaberinAnnehmen(context.Background(), w.netz, w.dat, w.q.w.val, p, &w.log); err != nil {
		t.Fatalf("Annehmen: %v\n%s", err, w.log.String())
	}
}

func (w *qsrWelt) einloesen(t *testing.T, s int) QuantenSVorlage {
	t.Helper()
	vl, err := quantenSInhaberinEinloesen(context.Background(), w.netz, time.Minute, w.dat, func() error { return nil }, s, &w.log)
	if err != nil {
		t.Fatalf("Einloesen: %v\n%s", err, w.log.String())
	}
	return jsonWeg(t, *vl)
}

func (w *qsrWelt) stelle(s int) QuantenSAusgabeAuftrag {
	return QuantenSAusgabeAuftrag{Schluessel: w.stellen[s], Frist: time.Minute}
}

func TestQuantenSRollen_Gutfall(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	for s := 0; s < 2; s++ {
		t.Run(fmt.Sprintf("Stelle%d", s), func(t *testing.T) {
			w := neueQsrWelt(t)
			p := w.ausgeben(t)
			// Der Ausgeber kann x nicht lesen: fuer die Inhaberin verschluesselt.
			von, bis, _ := quantenSAbschnitt(p.N, len(w.q.w.val), 1)
			if _, err := quantenSFuerInhaberinOeffnen(w.a.Schluessel, p.Abschnitte[0].FuerInhaberin, p.K, von, bis); err == nil {
				t.Fatal("der Ausgeber liest x der Inhaberin")
			}
			if p.Inhaberin == quantenSAdresse(w.a.Schluessel) || p.Stellen[0] == p.Ausgeber || p.Stellen[1] == p.Ausgeber {
				t.Fatal("Rollen nicht getrennt")
			}
			w.annehmen(t, p)
			vl := w.einloesen(t, s)
			// Kompakt: keine Blaetter, die holt die Stelle selbst.
			if roh, _ := json.Marshal(vl); len(vl.Blaetter) != 0 || len(roh) > p.N+400 {
				t.Fatalf("Vorlage nicht kompakt: %d Blaetter, %d Byte", len(vl.Blaetter), len(roh))
			}
			wert := w.q.w.stand(w.q.w.cs, quantenSAdresse(w.stellen[s]))
			erg, err := quantenSAnnahmeLauf(context.Background(), w.stelle(s), vl, w.q.w.val, w.netz, &w.log)
			if err != nil {
				t.Fatalf("Annahme: %v\n%s", err, w.log.String())
			}
			if erg.Urteil != "eingeloest" || erg.Vorab != "besteht" || erg.B != s || erg.Stichprobe == 0 || erg.Treffer == 0 {
				t.Fatalf("Ergebnis: %+v", erg)
			}
			if w.q.w.stand(w.q.w.cs, quantenSAdresse(w.stellen[s])) <= wert {
				t.Fatal("die Annahmestelle hat nichts bekommen")
			}
			if w.q.w.stand(w.q.w.cs, quantenSAdresse(w.stellen[1-s])) != 0 {
				t.Fatal("die andere Stelle hat etwas bekommen")
			}
			if w.netz.vorBindung != 0 {
				t.Fatalf("%d Oeffnungen vor Bindung und Wahl", w.netz.vorBindung)
			}
			if w.q.w.cs.quantenWJetzt().kodiert() != w.q.w.nach.quantenWJetzt().kodiert() {
				t.Fatal("Register weichen ab")
			}
			for _, muss := range []string{"das Paket geht an die Inhaberin", "angenommen", "gebunden", "gewaehlt", "vorab: besteht", "Urteil der Kette: eingeloest"} {
				if !strings.Contains(w.log.String(), muss) {
					t.Fatalf("Protokoll ohne %q:\n%s", muss, w.log.String())
				}
			}
			// Nichts Geheimes in den Ausgaben.
			geheim := w.dat.Schluessel
			for _, v := range []interface{}{p, vl, erg} {
				roh, _ := json.Marshal(v)
				if strings.Contains(string(roh), geheim) {
					t.Fatal("Schluessel der Inhaberin in einer Ausgabe")
				}
			}
			if strings.Contains(w.log.String(), geheim) {
				t.Fatal("Schluessel der Inhaberin im Protokoll")
			}
		})
	}
}

// Die Ansicht des Tokens nennt Wurzel und Proben, d erst nach der Bindung,
// c erst nach der Wahl.
func TestQuantenSRollen_AnsichtOeffentlich(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w := neueQsrWelt(t)
	p := w.ausgeben(t)
	st, _ := w.netz.stand(context.Background(), p.K)
	if qsText(st, "wurzel") != p.Wurzel || fmt.Sprint(qsTexte(st, "proben")) != fmt.Sprint(p.Proben[:]) {
		t.Fatalf("Ansicht: %v", st)
	}
	if _, da := st["d"]; da {
		t.Fatal("d vor der Bindung")
	}
	if _, da := st["c"]; da {
		t.Fatal("c vor der Wahl")
	}
	w.annehmen(t, p)
	w.einloesen(t, 1)
	st, _ = w.netz.stand(context.Background(), p.K)
	y, _ := w.dat.y()
	for i := range y {
		y[i] ^= *w.dat.Z
	}
	if qsText(st, "d") != qsBitsText(y) || quantenSZahl(st["c"]) != 1^*w.dat.Z {
		t.Fatalf("d oder c in der Ansicht: %v %v", st["d"], st["c"])
	}
}

// Ein Ausgeber faelscht das Paket: die Inhaberin nimmt nichts an.
func TestQuantenSRollen_PaketGefaelscht(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w := neueQsrWelt(t)
	gut := w.ausgeben(t)
	_, fremdAdr := neuerSchluessel(t)
	falle := map[string]struct {
		f    func(p *QuantenSPaket, val []string) []string
		muss string
	}{
		"x neu verschluesselt": {func(p *QuantenSPaket, val []string) []string {
			// Der Ausgeber kennt den oeffentlichen Schluessel der Inhaberin und
			// verschluesselt ein falsches x selbst: nicht vom Validator unterschrieben.
			von, bis := p.Abschnitte[0].Von, p.Abschnitte[0].Bis
			pub, _ := quantenSSchluesselPruefen(w.dat.Bestellung.InhaberinSchluessel, w.dat.Bestellung.Inhaberin)
			c, err := quantenSFuerInhaberinFuer(pub, p.K, von, bis, strings.Repeat("0", bis-von))
			if err != nil {
				t.Fatal(err)
			}
			p.Abschnitte[0].FuerInhaberin = c
			return val
		}, "nicht von Validator 1 unterschrieben"},
		"Blatt getauscht": {func(p *QuantenSPaket, val []string) []string {
			p.Abschnitte[2].Blaetter[0] = strings.Repeat("ab", 32)
			return val
		}, "nicht von Validator 3"},
		"Teil getauscht": {func(p *QuantenSPaket, val []string) []string {
			p.Abschnitte[1].Teile[0] = strings.Repeat("cd", 32)
			return val
		}, "nicht von Validator 2"},
		"Zaehlung geschoent": {func(p *QuantenSPaket, val []string) []string { p.Abschnitte[4].Zaehlung[1][0]++; return val }, "nicht von Validator 5"},
		"Wurzel anders":      {func(p *QuantenSPaket, val []string) []string { p.Wurzel = strings.Repeat("11", 32); return val }, "ergeben nicht die Wurzel"},
		"Probe anders":       {func(p *QuantenSPaket, val []string) []string { p.Proben[1] = strings.Repeat("22", 32); return val }, "Probe 1 passt nicht"},
		"andere Validatoren": {func(p *QuantenSPaket, val []string) []string { p.Validatoren[3] = fremdAdr; return val }, "Validator 4 einen anderen"},
		"fremde Inhaberin":   {func(p *QuantenSPaket, val []string) []string { p.Inhaberin = fremdAdr; return val }, "nicht fuer diese Bestellung"},
		"Stellen getauscht": {func(p *QuantenSPaket, val []string) []string {
			p.Stellen[0], p.Stellen[1] = p.Stellen[1], p.Stellen[0]
			return val
		}, "nicht fuer diese Bestellung"},
		"Abschnitt fehlt": {func(p *QuantenSPaket, val []string) []string { p.Abschnitte = p.Abschnitte[:4]; return val }, "4 Abschnitte"},
		"Abschnitte vertauscht": {func(p *QuantenSPaket, val []string) []string {
			p.Abschnitte[0], p.Abschnitte[1] = p.Abschnitte[1], p.Abschnitte[0]
			return val
		}, "Abschnitt 1 hat nicht die Form"},
		"v1 statt v2":      {func(p *QuantenSPaket, val []string) []string { p.ProbeArt = 0; return val }, "nur Stichprobe v2"},
		"Schwelle ueber v": {func(p *QuantenSPaket, val []string) []string { p.T = 6; return val }, "Schwelle 6"},
		"Inhaberin vertraut wem anders": {func(p *QuantenSPaket, val []string) []string {
			// Validator 2 unterschreibt; die Inhaberin hat einen anderen genannt.
			v := append([]string(nil), val...)
			v[1] = fremdAdr
			p.Validatoren[1] = fremdAdr
			return v
		}, "nicht von Validator 2"},
	}
	for name, fall := range falle {
		t.Run(name, func(t *testing.T) {
			p := jsonWeg(t, gut)
			val := fall.f(&p, append([]string(nil), w.q.w.val...))
			d := *w.dat
			err := quantenSInhaberinAnnehmen(context.Background(), w.netz, &d, val, p, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), fall.muss) {
				t.Fatalf("%v (erwartet %q)", err, fall.muss)
			}
			if d.K != "" || d.X != "" {
				t.Fatal("trotz Faelschung angenommen")
			}
		})
	}
	// Das ungefaelschte Paket geht danach noch.
	w.annehmen(t, jsonWeg(t, gut))
}

// quantenSFuerInhaberinFuer: (nur Test) verschluesselt ein x fuer die
// Inhaberin, wie es ein faelschender Ausgeber koennte.
func quantenSFuerInhaberinFuer(pub *ecdsa.PublicKey, K string, von, bis int, x string) (string, error) {
	roh, _ := json.Marshal(map[string]interface{}{"dom": quantenSDomFuerInhaberin, "k": K, "von": von, "bis": bis, "x": x})
	c, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(pub), roh, nil, nil)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(c), nil
}

// Die Kette sagt etwas anderes als das Paket: nicht angenommen.
func TestQuantenSRollen_KetteEntscheidet(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w := neueQsrWelt(t)
	p := w.ausgeben(t)
	d := *w.dat
	err := quantenSInhaberinAnnehmen(context.Background(), qsKeinStand{w.netz}, &d, w.q.w.val, p, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "nicht auf der Kette") {
		t.Fatalf("Token nicht auf der Kette: %v", err)
	}
	// Schon gebunden, bevor die Inhaberin angenommen hat (unmoeglich ohne
	// ihren Schluessel -- hier geloggt durch einen luegenden Knoten).
	err = quantenSInhaberinAnnehmen(context.Background(), qsStandMit{w.netz, map[string]interface{}{"gebunden": true}}, &d, w.q.w.val, p, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "BEFUND") {
		t.Fatalf("gebunden vor der Annahme: %v", err)
	}
	err = quantenSInhaberinAnnehmen(context.Background(), qsStandMit{w.netz, map[string]interface{}{"wurzel": strings.Repeat("00", 32)}}, &d, w.q.w.val, p, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "Wurzel oder Proben auf der Kette") {
		t.Fatalf("andere Wurzel auf der Kette: %v", err)
	}
	if d.K != "" {
		t.Fatal("trotzdem angenommen")
	}
}

type qsKeinStand struct{ quantenSAbschnittNetz }

func (qsKeinStand) stand(context.Context, string) (map[string]interface{}, error) { return nil, nil }

// qsStandMit: der Stand mit ueberschriebenen Feldern (ein luegender Knoten).
type qsStandMit struct {
	quantenSAbschnittNetz
	mit map[string]interface{}
}

func (n qsStandMit) stand(ctx context.Context, K string) (map[string]interface{}, error) {
	st, err := n.quantenSAbschnittNetz.stand(ctx, K)
	if st != nil {
		for k, v := range n.mit {
			st[k] = v
		}
	}
	return st, err
}

// Die Inhaberin unterschreibt keine Vorlage fuer die nicht gewaehlte Stelle.
func TestQuantenSRollen_KeineDoppelvorlage(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w := neueQsrWelt(t)
	w.annehmen(t, w.ausgeben(t))
	w.einloesen(t, 0)
	_, err := quantenSInhaberinEinloesen(context.Background(), w.netz, time.Minute, w.dat, func() error { return nil }, 1, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "Doppelvorlage") {
		t.Fatalf("Vorlage fuer die andere Stelle: %v", err)
	}
	// Dieselbe Stelle noch einmal: dieselbe Vorlage, nichts Neues auf der Kette.
	vl := w.einloesen(t, 0)
	if vl.B != 0 {
		t.Fatal(vl.B)
	}
}

// Die Maske wird gespeichert, BEVOR die Bindung eingereicht wird; scheitert
// das Speichern, geht nichts auf die Kette.
func TestQuantenSRollen_MaskeZuerst(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w := neueQsrWelt(t)
	p := w.ausgeben(t)
	w.annehmen(t, p)
	_, err := quantenSInhaberinEinloesen(context.Background(), w.netz, time.Minute, w.dat, func() error { return errors.New("Platte voll") }, 0, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "nichts eingereicht") {
		t.Fatalf("Speichern gescheitert: %v", err)
	}
	if st, _ := w.netz.stand(context.Background(), p.K); st["gebunden"] == true {
		t.Fatal("trotzdem gebunden")
	}
}

// Eine boese Inhaberin unterschreibt selbst eine Vorlage fuer die NICHT
// gewaehlte Stelle: die Stelle rechnet vorab "abgewiesen", die Kette zaehlt
// einen Fehlversuch; danach loest die gewaehlte Stelle ehrlich ein.
func TestQuantenSRollen_BoeseInhaberin(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w := neueQsrWelt(t)
	w.annehmen(t, w.ausgeben(t))
	ehrlich := w.einloesen(t, 0)
	key, _ := w.dat.schluessel()
	x, _ := quantenSBitsText(w.dat.X, w.dat.Bestellung.N)
	boese := ehrlich
	boese.B = 1
	boese.Vorlage = personalSign(t, key, quantenSVorlageText(ehrlich.K, 1, x))
	erg, err := quantenSAnnahmeLauf(context.Background(), w.stelle(1), boese, w.q.w.val, w.netz, &bytes.Buffer{})
	if err == nil || erg == nil || erg.Vorab != "abgewiesen" || erg.Urteil != "abgewiesen" {
		t.Fatalf("Doppelvorlage: %+v %v", erg, err)
	}
	if w.q.w.stand(w.q.w.cs, quantenSAdresse(w.stellen[1])) != 0 {
		t.Fatal("Stelle 1 hat etwas bekommen")
	}
	erg, err = quantenSAnnahmeLauf(context.Background(), w.stelle(0), ehrlich, w.q.w.val, w.netz, &w.log)
	if err != nil || erg.Urteil != "eingeloest" {
		t.Fatalf("ehrlich nach der Doppelvorlage: %+v %v\n%s", erg, err, w.log.String())
	}
}

// Die Annahmestelle glaubt der Vorlage nichts.
func TestQuantenSRollen_VorlageGefaelscht(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w := neueQsrWelt(t)
	w.annehmen(t, w.ausgeben(t))
	gut := w.einloesen(t, 1)
	fremd, _ := neuerSchluessel(t)
	falle := map[string]struct {
		stelle int
		f      func(v *QuantenSVorlage)
		muss   string
		holt   bool // darf die Blaetter bei den Validatoren holen (sonst: keine Anfrage)
	}{
		"fremde Unterschrift": {1, func(v *QuantenSVorlage) {
			x, _ := quantenSBitsText(v.X, len(v.X))
			v.Vorlage = personalSign(t, fremd, quantenSVorlageText(v.K, 1, x))
		}, "nicht von der Inhaberin", false},
		"x veraendert": {1, func(v *QuantenSVorlage) {
			b := []byte(v.X)
			b[7] ^= 1
			v.X = string(b)
		}, "nicht von der Inhaberin", false},
		// Mit Blaettern in der Vorlage: gegen die Wurzel auf der Kette, ohne
		// einen Validator zu fragen.
		"Blatt veraendert": {1, func(v *QuantenSVorlage) {
			v.Blaetter = append([]string(nil), w.dat.Blaetter...)
			v.Blaetter[9] = strings.Repeat("ef", 32)
		}, "ergeben nicht die Wurzel", false},
		"Blatt fehlt":           {1, func(v *QuantenSVorlage) { v.Blaetter = w.dat.Blaetter[1:] }, "Blaetter, n = 256", false},
		"an die falsche Stelle": {0, func(v *QuantenSVorlage) {}, "nicht diese Stelle", false},
		"Stelle 2":              {1, func(v *QuantenSVorlage) { v.B = 2 }, "K oder Stelle", false},
		"K unbekannt":           {1, func(v *QuantenSVorlage) { v.K = strings.Repeat("0", 64) }, "nicht (mehr) auf der Kette", false},
	}
	for name, fall := range falle {
		t.Run(name, func(t *testing.T) {
			v := jsonWeg(t, gut)
			fall.f(&v)
			vorher, vorherAb := w.netz.oeffnungen, w.netz.abholungen
			erg, err := quantenSAnnahmeLauf(context.Background(), w.stelle(fall.stelle), v, w.q.w.val, w.netz, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), fall.muss) || (erg != nil && erg.Urteil != "") {
				t.Fatalf("%+v %v (erwartet %q)", erg, err, fall.muss)
			}
			// Geprueft wird VOR jeder Anfrage an die Validatoren: eine
			// gefaelschte Vorlage holt keine Oeffnung (und erst recht keine
			// Anmeldung -- die Kette wuerde sie auch abweisen).
			if w.netz.oeffnungen != vorher {
				t.Fatalf("%d Oeffnungen fuer eine gefaelschte Vorlage", w.netz.oeffnungen-vorher)
			}
			if !fall.holt && w.netz.abholungen != vorherAb {
				t.Fatalf("%d Abholungen fuer eine gefaelschte Vorlage", w.netz.abholungen-vorherAb)
			}
			if st, _ := w.netz.stand(context.Background(), gut.K); st == nil || st["anmeldung"] != nil || quantenSZahl(st["fehl"]) != 0 {
				t.Fatalf("trotzdem angemeldet: %v", st)
			}
		})
	}
	// Kompakte Vorlage, aber ein Validator luegt ueber ein Blatt: seine
	// Unterschrift passt nicht -- keine Oeffnung, keine Anmeldung.
	w.netz.blattFalsch = 2
	vorher := w.netz.oeffnungen
	if _, err := quantenSAnnahmeLauf(context.Background(), w.stelle(1), gut, w.q.w.val, w.netz, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "Abschnitt 2 ist nicht von Validator 2") || w.netz.oeffnungen != vorher {
		t.Fatalf("falsches Blatt vom Validator: %v (%d Oeffnungen)", err, w.netz.oeffnungen-vorher)
	}
	w.netz.blattFalsch = 0
	// Die Stelle mit falscher Zahl von Validatoren fragt gar nicht erst.
	if _, err := quantenSAnnahmeLauf(context.Background(), w.stelle(1), gut, w.q.w.val[:4], w.netz, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "angegeben 4 Validatoren") {
		t.Fatalf("4 statt 5 Validatoren: %v", err)
	}
	erg, err := quantenSAnnahmeLauf(context.Background(), w.stelle(1), gut, w.q.w.val, w.netz, &bytes.Buffer{})
	if err != nil || erg.Urteil != "eingeloest" {
		t.Fatalf("die ungefaelschte Vorlage: %+v %v", erg, err)
	}
}

// Vor Bindung und Wahl meldet keine Stelle an; ein Validator mit falschem
// Salz fliegt beim Laden auf, bevor etwas auf die Kette geht.
func TestQuantenSRollen_AnnahmeFruehOderFalsch(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w := neueQsrWelt(t)
	p := w.ausgeben(t)
	w.annehmen(t, p)
	x, _ := quantenSBitsText(w.dat.X, p.N)
	key, _ := w.dat.schluessel()
	frueh := QuantenSVorlage{K: p.K, B: 0, X: w.dat.X, Vorlage: personalSign(t, key, quantenSVorlageText(p.K, 0, x)), Blaetter: w.dat.Blaetter}
	if _, err := quantenSAnnahmeLauf(context.Background(), w.stelle(0), frueh, w.q.w.val, w.netz, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "erst nach Bindung und Wahl") {
		t.Fatalf("vor der Bindung: %v", err)
	}
	vl := w.einloesen(t, 0)
	w.netz.salzFalsch = 4
	if _, err := quantenSAnnahmeLauf(context.Background(), w.stelle(0), vl, w.q.w.val, w.netz, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "passen nicht zur Bindung") {
		t.Fatalf("falsches Salz: %v", err)
	}
	if st, _ := w.netz.stand(context.Background(), p.K); st["anmeldung"] != nil {
		t.Fatal("trotz falschem Salz angemeldet")
	}
}

// Ein luegender Knoten meldet nach der Anmeldung "weg" (wie eingeloest), die
// Stelle hatte vorab "abgewiesen" gerechnet: BEFUND, kein Erfolg.
func TestQuantenSRollen_VorabWiderspruchIstBefund(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w := neueQsrWelt(t)
	w.annehmen(t, w.ausgeben(t))
	ehrlich := w.einloesen(t, 0)
	key, _ := w.dat.schluessel()
	x, _ := quantenSBitsText(w.dat.X, w.dat.Bestellung.N)
	boese := ehrlich
	boese.B = 1
	boese.Vorlage = personalSign(t, key, quantenSVorlageText(ehrlich.K, 1, x))
	w.netz.luegeNachAnmeldung = true
	erg, err := quantenSAnnahmeLauf(context.Background(), w.stelle(1), boese, w.q.w.val, w.netz, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "BEFUND: vorab abgewiesen, die Kette urteilt eingeloest") || erg.Urteil != "eingeloest" {
		t.Fatalf("%+v %v", erg, err)
	}
}

// Der Ausgeber: nur v2, nicht fuer sich selbst, und die Bestellung streng.
func TestQuantenSRollen_AusgeberStreng(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w := neueQsrWelt(t)
	a := w.a
	a.ProbeV2 = false
	if _, err := quantenSAusgabeFuerInhaberin(context.Background(), a, w.dat.Bestellung, w.q.w.val, 3, w.netz, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "nur mit Stichprobe v2") {
		t.Fatalf("v1: %v", err)
	}
	selbst := w.dat.Bestellung
	selbst.Inhaberin, selbst.InhaberinSchluessel = quantenSAdresse(w.a.Schluessel), pubHex(w.a.Schluessel)
	if _, err := quantenSAusgabeFuerInhaberin(context.Background(), w.a, selbst, w.q.w.val, 3, w.netz, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "Ausgeber selbst") {
		t.Fatalf("fuer sich selbst: %v", err)
	}
	fremd, _ := neuerSchluessel(t)
	for name, f := range map[string]func(b *QuantenSBestellung){
		"Schluessel passt nicht": func(b *QuantenSBestellung) { b.InhaberinSchluessel = pubHex(fremd) },
		"Stelle ist Inhaberin": func(b *QuantenSBestellung) {
			b.Stellen[1] = QuantenSStelle{Adresse: b.Inhaberin, Schluessel: b.InhaberinSchluessel}
		},
		"Stellen gleich":     func(b *QuantenSBestellung) { b.Stellen[1] = b.Stellen[0] },
		"Stellen-Schluessel": func(b *QuantenSBestellung) { b.Stellen[0].Schluessel = pubHex(fremd) },
		"Basen zu kurz":      func(b *QuantenSBestellung) { b.Basen[2] = b.Basen[2][1:] },
		"Abschnitt fehlt":    func(b *QuantenSBestellung) { b.Basen = b.Basen[:4] },
		"n zu gross":         func(b *QuantenSBestellung) { b.N = quantenWNMax + 1 },
	} {
		b := jsonWeg(t, w.dat.Bestellung)
		f(&b)
		if _, err := quantenSAusgabeFuerInhaberin(context.Background(), w.a, b, w.q.w.val, 3, w.netz, &bytes.Buffer{}); err == nil {
			t.Fatalf("%s angenommen", name)
		}
	}
	if w.netz.anfragen != 0 || len(w.q.w.cs.quantenWJetzt().Token) != 0 {
		t.Fatal("trotz falscher Bestellung gefragt oder ausgegeben")
	}
}

// Die Antwort-Nachricht aus den Hashes ist byte-gleich (v1 und v2).
func TestQuantenSAbschnittAntwortNachricht_AusHashes(t *testing.T) {
	for _, a := range []QuantenSAbschnittAntwort{
		{K: strings.Repeat("a", 64), X: 2, Von: 51, Bis: 102, Blaetter: []string{"aa", "bb"}, FuerInhaberin: "cc", Buendel: []string{"", "dd"}, BuendelSig: []string{"", "ee"}},
		{K: strings.Repeat("b", 64), X: 1, Von: 0, Bis: 51, Blaetter: []string{"aa"}, FuerInhaberin: "cc", Buendel: []string{""}, BuendelSig: []string{""},
			Teile: []string{"11", "22"}, Zaehlung: [][2]int{{3, 1}, {4, 0}}},
	} {
		alt := quantenSAbschnittAntwortNachricht(a)
		neu := quantenSAbschnittAntwortNachrichtAus(a.K, a.X, a.Von, a.Bis, quantenSHashListe(a.Blaetter), a.FuerInhaberin, quantenSHashListe(a.Buendel), quantenSHashListe(a.BuendelSig), a.Teile, a.Zaehlung)
		if alt != neu {
			t.Fatalf("%q != %q", alt, neu)
		}
	}
	// Festgehalten: so unterschreiben die laufenden Dienste (v1 ohne Teile).
	a := QuantenSAbschnittAntwort{K: strings.Repeat("a", 64), X: 1, Von: 0, Bis: 2, Blaetter: []string{"aa"}, FuerInhaberin: "cc", Buendel: []string{""}, BuendelSig: []string{""}}
	if m := quantenSAbschnittAntwortNachricht(a); !strings.HasPrefix(m, quantenSDomAbschnittAntwort+"|chain:") || strings.Contains(m, "teile:") {
		t.Fatal(m)
	}
}

// Die Datei der Inhaberin: nur 0600, kein Verweis, nie ueberschrieben beim
// Anlegen, keine unbekannten Felder; atomar ersetzt.
func TestQuantenSInhaberinDatei(t *testing.T) {
	dir := t.TempDir()
	pfad := filepath.Join(dir, "inhaberin.json")
	var st [2]QuantenSStelle
	for s := range st {
		k, adr := neuerSchluessel(t)
		st[s] = QuantenSStelle{Adresse: adr, Schluessel: pubHex(k)}
	}
	var aus bytes.Buffer
	if err := QuantenSInhaberinNeu(pfad, 256, 2, st, &aus); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(pfad)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("Rechte %v", fi.Mode().Perm())
	}
	d, err := quantenSInhaberinLesen(pfad)
	if err != nil {
		t.Fatal(err)
	}
	var best QuantenSBestellung
	if err := quantenSJSONLesen(&aus, "Bestellung", &best); err != nil || best.Inhaberin != d.Bestellung.Inhaberin {
		t.Fatalf("Bestellung: %v", err)
	}
	if strings.Contains(fmt.Sprint(best), d.Schluessel) {
		t.Fatal("Schluessel in der Bestellung")
	}
	if err := QuantenSInhaberinNeu(pfad, 256, 2, st, &bytes.Buffer{}); err == nil {
		t.Fatal("bestehende Datei ueberschrieben")
	}
	if d2, _ := quantenSInhaberinLesen(pfad); d2 == nil || d2.Schluessel != d.Schluessel {
		t.Fatal("Datei veraendert")
	}
	// Ersetzen: atomar, 0600, kein Rest daneben.
	z := 1
	d.Z = &z
	if err := quantenSInhaberinSchreiben(pfad, d, false); err != nil {
		t.Fatal(err)
	}
	if d2, err := quantenSInhaberinLesen(pfad); err != nil || d2.Z == nil || *d2.Z != 1 {
		t.Fatalf("ersetzt: %v", err)
	}
	if e, _ := os.ReadDir(dir); len(e) != 1 {
		t.Fatalf("Reste: %v", e)
	}
	os.Chmod(pfad, 0o644)
	if _, err := quantenSInhaberinLesen(pfad); err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("0644: %v", err)
	}
	os.Chmod(pfad, 0o600)
	verweis := filepath.Join(dir, "verweis.json")
	os.Symlink(pfad, verweis)
	if _, err := quantenSInhaberinLesen(verweis); err == nil {
		t.Fatal("Verweis gelesen")
	}
	if err := quantenSInhaberinSchreiben(verweis, d, false); err == nil {
		t.Fatal("durch einen Verweis geschrieben")
	}
	// Zwischen Pruefen und Oeffnen vertauscht: die geoeffnete Datei ist nicht
	// die unter dem Pfad -- abgewiesen.
	andere := filepath.Join(dir, "andere.json")
	if err := quantenSInhaberinSchreiben(andere, d, true); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(andere)
	if err != nil {
		t.Fatal(err)
	}
	if err := quantenSInhaberinDateiPasst(f, pfad); err == nil {
		t.Fatal("vertauschte Datei angenommen")
	}
	if err := quantenSInhaberinDateiPasst(f, andere); err != nil {
		t.Fatalf("dieselbe Datei abgewiesen: %v", err)
	}
	f.Close()
	os.Remove(andere)
	roh, _ := os.ReadFile(pfad)
	for name, inhalt := range map[string]string{
		"unbekanntes Feld": strings.Replace(string(roh), `"v":`, `"fremd": 1, "v":`, 1),
		"zwei Werte":       string(roh) + "{}",
		"fremder Schluessel": strings.Replace(string(roh), d.Schluessel, hex.EncodeToString(crypto.FromECDSA(func() *ecdsa.PrivateKey {
			k, _ := neuerSchluessel(t)
			return k
		}())), 1),
	} {
		p := filepath.Join(dir, "f.json")
		os.WriteFile(p, []byte(inhalt), 0o600)
		_, err := quantenSInhaberinLesen(p)
		if err == nil {
			t.Fatalf("%s gelesen", name)
		}
		if strings.Contains(err.Error(), d.Schluessel) {
			t.Fatalf("%s: Schluessel in der Fehlermeldung", name)
		}
		os.Remove(p)
	}
}

func TestQuantenSRollen_Lesen(t *testing.T) {
	k, adr := neuerSchluessel(t)
	if st, err := QuantenSStellenLesen(adr + "=" + pubHex(k) + ", " + adr + "=x"); err != nil || st[1].Schluessel != "x" {
		t.Fatalf("%v %v", st, err)
	}
	for _, roh := range []string{"", adr + "=" + pubHex(k), "a=b,c=d,e=f", adr + "," + adr} {
		if _, err := QuantenSStellenLesen(roh); err == nil {
			t.Fatalf("%q gelesen", roh)
		}
	}
	if v, err := QuantenSAdressenLesen("0x" + strings.ToUpper(adr[2:])); err != nil || v[0] != adr {
		t.Fatalf("Grossbuchstaben: %v %v", v, err)
	}
	if v, err := QuantenSAdressenLesen(" " + adr + " "); err != nil || len(v) != 1 {
		t.Fatalf("%v %v", v, err)
	}
	for _, roh := range []string{"", adr + "," + adr, "0x12", strings.Repeat(adr+",", quantenWVMax) + adr} {
		if _, err := QuantenSAdressenLesen(roh); err == nil {
			t.Fatalf("%q gelesen", roh)
		}
	}
}

// Ein boeser Ausgeber ueberspringt seine Probe der ehrlichen Vorlage (die
// Kette prueft die Zaehlungen nicht): die Inhaberin rechnet selbst aus den
// unterschriebenen Zaehlungen nach und nimmt den Token nicht an.
func TestQuantenSRollen_AusgeberOhneEhrlichProbe(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w := neueQsrWelt(t)
	for _, m := range w.q.d {
		m.messquelle.(*qsaMessung).verrauscht = 25
	}
	// Der ehrliche Ausgeber gibt so nicht aus.
	if _, err := quantenSAusgabeFuerInhaberin(context.Background(), w.a, w.dat.Bestellung, w.q.w.val, 3, w.netz, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "bestuende") {
		t.Fatalf("ehrlicher Ausgeber, verrauscht: %v", err)
	}
	// Der boese: dieselben Schritte ohne quantenSEhrlichBesteht.
	ctx, a, best := context.Background(), w.a, w.dat.Bestellung
	basen := QuantenSBasen{N: best.N, Basen: best.Basen}
	r, err := quantenSRundeBeginnen(a, basen, w.q.w.val, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.schluesselHolen(ctx, w.netz); err != nil {
		t.Fatal(err)
	}
	stellen := best.stellenAdressen()
	if err := r.abschnitteHolen(ctx, a, basen, w.netz, best.Inhaberin, best.InhaberinSchluessel, stellen, [2]string{best.Stellen[0].Schluessel, best.Stellen[1].Schluessel}); err != nil {
		t.Fatal(err)
	}
	proben, probenHex, _, _, err := r.probenV2()
	if err != nil {
		t.Fatal(err)
	}
	bst, err := r.bestaetigungenHolen(ctx, a, w.netz, proben, probenHex)
	if err != nil {
		t.Fatal(err)
	}
	q := QuantenWAngaben{K: r.K, N: r.n, T: 3, Wurzel: hex.EncodeToString(r.wurzel[:]), Validatoren: w.q.w.val, Bestaetigungen: bst,
		Deckung: a.Deckung, Tol: a.Tol, Inhaber: best.Inhaberin, Annahme: stellen[:], Proben: probenHex, ProbeArt: 2}
	if err := quantenSAusgabeEinreichen(ctx, a, w.netz, q, r.ausgeber, r.zeit); err != nil {
		t.Fatalf("die Kette nimmt die Ausgabe (sie zaehlt nicht): %v", err)
	}
	p := jsonWeg(t, *r.paket(a, best.Inhaberin, stellen, probenHex))
	err = quantenSInhaberinAnnehmen(ctx, w.netz, w.dat, w.q.w.val, p, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "BEFUND: nach den Zaehlungen der Validatoren bestuende") {
		t.Fatalf("Inhaberin nimmt einen Token an, der ehrlich nicht bestuende: %v", err)
	}
	if w.dat.K != "" {
		t.Fatal("trotzdem angenommen")
	}
}

// Die Inhaberin holt ihren Token nur mit K: der Stand von der Kette, jeder
// Abschnitt direkt vom Validator -- kein Paket des Ausgebers.
func TestQuantenSRollen_AbholenMitK(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w := neueQsrWelt(t)
	paket := w.ausgeben(t)
	ctx := context.Background()
	// Falsche Zahl von Validatoren, unbekanntes K, ein luegender Validator.
	if _, err := quantenSPaketVonKette(ctx, w.netz, paket.K, w.q.w.val[:3]); err == nil || !strings.Contains(err.Error(), "angegeben 3") {
		t.Fatalf("3 statt 5 Validatoren: %v", err)
	}
	if _, err := quantenSPaketVonKette(ctx, w.netz, strings.Repeat("0", 64), w.q.w.val); err == nil || !strings.Contains(err.Error(), "nicht auf der Kette") {
		t.Fatalf("unbekanntes K: %v", err)
	}
	if _, err := quantenSPaketVonKette(ctx, w.netz, "kein K", w.q.w.val); err == nil {
		t.Fatal("K ohne Form angenommen")
	}
	w.netz.blattFalsch = 4
	p, err := quantenSPaketVonKette(ctx, w.netz, paket.K, w.q.w.val)
	if err != nil {
		t.Fatal(err)
	}
	d := *w.dat
	if err := quantenSInhaberinAnnehmen(ctx, w.netz, &d, w.q.w.val, p, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "Abschnitt 4 ist nicht von Validator 4") || d.K != "" {
		t.Fatalf("luegender Validator: %v", err)
	}
	w.netz.blattFalsch = 0
	// Ehrlich: dasselbe x wie aus dem Paket des Ausgebers.
	p, err = quantenSPaketVonKette(ctx, w.netz, paket.K, w.q.w.val)
	if err != nil {
		t.Fatal(err)
	}
	if p.Wurzel != paket.Wurzel || p.Proben != paket.Proben || p.Stellen != paket.Stellen || p.Inhaberin != paket.Inhaberin || p.T != paket.T || p.Tol != paket.Tol {
		t.Fatalf("Paket von der Kette weicht ab: %+v", p)
	}
	ausPaket := *w.dat
	if err := quantenSInhaberinAnnehmen(ctx, w.netz, &ausPaket, w.q.w.val, paket, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	w.annehmen(t, p)
	if w.dat.X != ausPaket.X || w.dat.K != paket.K {
		t.Fatal("x aus den Abschnitten der Validatoren weicht vom Paket ab")
	}
	vl := w.einloesen(t, 0)
	erg, err := quantenSAnnahmeLauf(ctx, w.stelle(0), vl, w.q.w.val, w.netz, &w.log)
	if err != nil || erg.Urteil != "eingeloest" {
		t.Fatalf("%+v %v\n%s", erg, err, w.log.String())
	}
}

// Der Endpunkt des Dienstes: nur zu Tokens auf der Kette, nur v2, nur GET,
// begrenzt; die Antwort ist vom Validator unterschrieben.
func TestQuantenSFuerInhaberin_Dienst(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	w := neueQsrWelt(t)
	d := w.q.d[1]
	if _, err := d.abschnittFuerInhaberin(strings.Repeat("0", 64)); err == nil || !strings.Contains(err.Error(), "kein Abschnitt") {
		t.Fatalf("unbekanntes K: %v", err)
	}
	// Vorbereitet, aber noch nicht ausgegeben: nichts heraus.
	ctx, a, best := context.Background(), w.a, w.dat.Bestellung
	basen := QuantenSBasen{N: best.N, Basen: best.Basen}
	r, err := quantenSRundeBeginnen(a, basen, w.q.w.val, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.schluesselHolen(ctx, w.netz); err != nil {
		t.Fatal(err)
	}
	if err := r.abschnitteHolen(ctx, a, basen, w.netz, best.Inhaberin, best.InhaberinSchluessel, best.stellenAdressen(), [2]string{best.Stellen[0].Schluessel, best.Stellen[1].Schluessel}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.abschnittFuerInhaberin(r.K); err == nil || !strings.Contains(err.Error(), "erst, wenn der Token auf der Kette steht") {
		t.Fatalf("vor der Ausgabe: %v", err)
	}
	// Ausgegeben (ueber das Werkzeug): jetzt ja, unterschrieben von Validator 2.
	w2 := neueQsrWelt(t)
	p := w2.ausgeben(t)
	d2 := w2.q.d[1]
	ab, err := d2.abschnittFuerInhaberin(p.K)
	if err != nil {
		t.Fatal(err)
	}
	if err := quantenSPaketAbschnittPruefen(ab, p.K, p.N, len(w2.q.w.val), 2, w2.q.w.val[1]); err != nil {
		t.Fatal(err)
	}
	if err := quantenSPaketAbschnittPruefen(ab, p.K, p.N, len(w2.q.w.val), 2, w2.q.w.val[0]); err == nil {
		t.Fatal("Unterschrift eines anderen Validators angenommen")
	}
	// Ein Abschnitt im bisherigen Weg (ohne v2) gibt nichts heraus.
	e, _ := d2.eintragVon(p.K)
	e.Abschnitt.ProbeArt = 0
	if _, err := d2.abschnittFuerInhaberin(p.K); err == nil || !strings.Contains(err.Error(), "nur bei Stichprobe v2") {
		t.Fatalf("v1: %v", err)
	}
	e.Abschnitt.ProbeArt = 2

	// HTTP: Methode, K, Begrenzung, strenges JSON.
	alt := w2.q.w.cs.quantenWDienstZeiger.Swap(d2)
	defer w2.q.w.cs.quantenWDienstZeiger.Store(alt)
	altAb := quantenWAbOverride.Load()
	quantenWAbOverride.Store(1)
	defer quantenWAbOverride.Store(altAb)
	api := &APIServer{state: w2.q.w.cs}
	frage := func(methode, k, von string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(methode, "/api/quanten-w/validator/fuer-inhaberin?k="+k, nil)
		req.RemoteAddr = von
		rec := httptest.NewRecorder()
		api.handleQuantenSFuerInhaberin(rec, req)
		return rec
	}
	if rec := frage(http.MethodPost, p.K, "198.51.100.40:1"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", rec.Code)
	}
	if rec := frage(http.MethodGet, "zz", "198.51.100.41:1"); rec.Code != http.StatusBadRequest {
		t.Fatalf("K ohne Form: %d", rec.Code)
	}
	if rec := frage(http.MethodGet, strings.Repeat("0", 64), "198.51.100.42:1"); rec.Code != http.StatusConflict {
		t.Fatalf("unbekanntes K: %d", rec.Code)
	}
	rec := frage(http.MethodGet, strings.ToUpper(p.K), "198.51.100.43:1")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
	}
	var ab2 QuantenSPaketAbschnitt
	if err := quantenSJSONLesen(rec.Body, "Antwort", &ab2); err != nil {
		t.Fatal(err)
	}
	if err := quantenSPaketAbschnittPruefen(ab2, p.K, p.N, len(w2.q.w.val), 2, w2.q.w.val[1]); err != nil {
		t.Fatal(err)
	}
	gesperrt := false
	for i := 0; i < burstQuantenWDienstJeIP+1; i++ {
		if frage(http.MethodGet, p.K, "198.51.100.44:1").Code == http.StatusTooManyRequests {
			gesperrt = true
			break
		}
	}
	if !gesperrt {
		t.Fatalf("nach %d Anfragen von einer Adresse nicht begrenzt", burstQuantenWDienstJeIP+1)
	}
}
