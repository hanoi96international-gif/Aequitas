package keeper

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// Oeffnung aus t Anteilen (R9, quanten_s_anteil_oeffnung.go) gegen die echten
// Regeln und den echten Dienst.

// qsv2Netz schreibt mit, ob ein Paket vor Bindung und Wahl herausging.
func (n *qsv2Netz) anteilOeffnung(ctx context.Context, j, x int, K string, b int) (string, error) {
	c, err := n.qsaNetz.anteilOeffnung(ctx, j, x, K, b)
	if tok := n.q.w.cs.quantenWJetzt().Token[K]; err == nil && (tok == nil || tok.token.D == nil || tok.token.C == -1) {
		n.vorBindung++
	}
	return c, err
}

// Gutfall: ein Praeparator ist nach der Bindung weg. Die Stellen laden seinen
// Abschnitt aus den Paketen der anderen, und die Ausgabe laeuft durch.
func TestQuantenSAnteilOeffnung_PraeparatorAusgefallen(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	for _, weg := range []int{1, 3} {
		q, a, bs, _ := qsaWerkzeug(t, 256)
		a.ProbeV2 = true
		netz := &qsv2Netz{qsaNetz: &qsaNetz{qsNetz: &qsNetz{q: q}, ausgefallen: weg}}
		var log bytes.Buffer
		erg, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, netz, &log)
		if err != nil {
			t.Fatalf("Praeparator %d weg: %v\n%s", weg, err, log.String())
		}
		if erg.Vorlage != "eingeloest" || erg.Doppelvorlage != "abgewiesen" {
			t.Fatalf("Praeparator %d weg: %+v", weg, erg)
		}
		if netz.pakete == 0 {
			t.Fatal("keine Pakete abgefragt")
		}
		if netz.vorBindung != 0 {
			t.Fatalf("%d Pakete oder Oeffnungen vor Bindung und Wahl herausgegeben", netz.vorBindung)
		}
	}
}

// Ohne Ausfall fragt niemand nach Paketen (der Weg wie bisher).
func TestQuantenSAnteilOeffnung_OhneAusfallKeinePakete(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	q, a, bs, _ := qsaWerkzeug(t, 256)
	a.ProbeV2 = true
	netz := &qsv2Netz{qsaNetz: &qsaNetz{qsNetz: &qsNetz{q: q}}}
	if _, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, netz, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if netz.pakete != 0 {
		t.Fatalf("%d Pakete abgefragt, obwohl jeder Praeparator da war", netz.pakete)
	}
}

// Missbrauch: Praeparator weg und ein anderer gibt das Paket fuer die andere
// Stelle heraus. Dann bleiben weniger als t lesbare Pakete -- kein Laden,
// keine Vorlage.
func TestQuantenSAnteilOeffnung_ZuWenigeEchtePakete(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	q, a, bs, _ := qsaWerkzeug(t, 256)
	a.ProbeV2, a.Frist = true, 50e6
	// v Validatoren, einer weg, t = 3: alle bis auf zwei der anderen geben
	// falsche Pakete heraus -- es bleiben t - 1 lesbare.
	falsch := map[int]bool{}
	for j := 5; j <= len(q.w.val); j++ {
		falsch[j] = true
	}
	falsch[3] = true
	netz := &qsv2Netz{qsaNetz: &qsaNetz{qsNetz: &qsNetz{q: q}, ausgefallen: 2, paketFalsch: falsch}}
	var log bytes.Buffer
	_, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, netz, &log)
	if err == nil || !strings.Contains(err.Error(), "aus Anteilen") || strings.Contains(log.String(), "Doppelvorlage:") {
		t.Fatalf("falsches Paket: %v\n%s", err, log.String())
	}
}

// Der Dienst gibt kein Paket heraus, solange der Token nicht gebunden ist,
// fuer sich selbst als Praeparator, oder fuer einen fremden Praeparator.
func TestQuantenSAnteilOeffnung_DienstGrenzen(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	q, _, _, _ := qsaWerkzeug(t, 256)
	K := strings.Repeat("ab", 32)
	for _, f := range []struct{ x, b int }{{1, 0}, {2, 1}, {0, 0}, {99, 0}, {2, 2}, {2, -1}} {
		if _, _, err := q.d[0].anteilOeffnungFuerStelle(K, f.x, f.b); err == nil {
			t.Fatalf("Paket ohne Token herausgegeben (x=%d, b=%d)", f.x, f.b)
		}
	}
}

func qaoSchluessel(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// Lesen: jede Abweichung wird abgewiesen.
func TestQuantenSAnteilOeffnungLesen_Abweisungen(t *testing.T) {
	stelle, fremd := qaoSchluessel(t), qaoSchluessel(t)
	K := strings.Repeat("cd", 32)
	salz := strings.Repeat("11", 32)
	gut := quantenSAnteilOeffnungInhalt{K: K, B: 1, Praeparator: 2, Empfaenger: 3, Von: 10, Bis: 20, Anteile: map[string]string{"12": "abc", "15": "1"}, Salz: salz}
	versiegeln := func(in quantenSAnteilOeffnungInhalt, an *ecdsa.PrivateKey) string {
		c, err := quantenSAnteilOeffnungVersiegeln(&an.PublicKey, in)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	c := versiegeln(gut, stelle)
	an, s, err := quantenSAnteilOeffnungLesen(stelle, c, K, 1, 2, 3, 10, 20)
	if err != nil || len(an) != 2 || an[12].Cmp(big.NewInt(0xabc)) != 0 || hexOf(s) != salz {
		t.Fatalf("Gutfall: %v %v", an, err)
	}
	mit := func(f func(*quantenSAnteilOeffnungInhalt)) string {
		in := gut
		in.Anteile = map[string]string{"12": "abc", "15": "1"}
		f(&in)
		return versiegeln(in, stelle)
	}
	faelle := map[string]func() error{
		"fremde Stelle": func() error { _, _, e := quantenSAnteilOeffnungLesen(fremd, c, K, 1, 2, 3, 10, 20); return e },
		"anderes K": func() error {
			_, _, e := quantenSAnteilOeffnungLesen(stelle, c, strings.Repeat("ef", 32), 1, 2, 3, 10, 20)
			return e
		},
		"andere Stelle b":   func() error { _, _, e := quantenSAnteilOeffnungLesen(stelle, c, K, 0, 2, 3, 10, 20); return e },
		"anderer Praep.":    func() error { _, _, e := quantenSAnteilOeffnungLesen(stelle, c, K, 1, 1, 3, 10, 20); return e },
		"anderer Validator": func() error { _, _, e := quantenSAnteilOeffnungLesen(stelle, c, K, 1, 2, 4, 10, 20); return e },
		"anderer Abschnitt": func() error { _, _, e := quantenSAnteilOeffnungLesen(stelle, c, K, 1, 2, 3, 11, 20); return e },
		"Stelle ausserhalb": func() error {
			_, _, e := quantenSAnteilOeffnungLesen(stelle, mit(func(i *quantenSAnteilOeffnungInhalt) { i.Anteile["20"] = "1" }), K, 1, 2, 3, 10, 20)
			return e
		},
		"Stelle mit Null": func() error {
			_, _, e := quantenSAnteilOeffnungLesen(stelle, mit(func(i *quantenSAnteilOeffnungInhalt) { i.Anteile["012"] = "1" }), K, 1, 2, 3, 10, 20)
			return e
		},
		"Anteil >= P": func() error {
			_, _, e := quantenSAnteilOeffnungLesen(stelle, mit(func(i *quantenSAnteilOeffnungInhalt) { i.Anteile["12"] = quantenWP.Text(16) }), K, 1, 2, 3, 10, 20)
			return e
		},
		"Grossbuchstaben": func() error {
			_, _, e := quantenSAnteilOeffnungLesen(stelle, mit(func(i *quantenSAnteilOeffnungInhalt) { i.Anteile["12"] = "ABC" }), K, 1, 2, 3, 10, 20)
			return e
		},
		"Salz kaputt": func() error {
			_, _, e := quantenSAnteilOeffnungLesen(stelle, mit(func(i *quantenSAnteilOeffnungInhalt) { i.Salz = "zz" }), K, 1, 2, 3, 10, 20)
			return e
		},
		"Uebergroesse": func() error {
			_, _, e := quantenSAnteilOeffnungLesen(stelle, strings.Repeat("a", quantenSAnteilOeffnungHoechstens+2), K, 1, 2, 3, 10, 20)
			return e
		},
		"Grossschrift Chiffrat": func() error {
			_, _, e := quantenSAnteilOeffnungLesen(stelle, strings.ToUpper(c), K, 1, 2, 3, 10, 20)
			return e
		},
	}
	for name, f := range faelle {
		if f() == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
}

func hexOf(b [32]byte) string { return new(big.Int).SetBytes(b[:]).Text(16) }

// Zusammensetzen: ein falscher Anteil unter vier Paketen bei t = 3 -- die
// Stelle findet die ehrliche Teilmenge; doppelte Validatoren und zu wenige
// Pakete werden abgewiesen.
func TestQuantenSAusAnteilenZusammensetzen(t *testing.T) {
	w := []*big.Int{big.NewInt(12345), big.NewInt(678)}
	stellen := []int{4, 9}
	anteile := make([][]*big.Int, len(w))
	for i, x := range w {
		a, err := quantenSTeilen(x, 3, 5)
		if err != nil {
			t.Fatal(err)
		}
		anteile[i] = a
	}
	paket := func(j int) quantenSAnteilPaket {
		p := quantenSAnteilPaket{J: j, Anteile: map[int]*big.Int{}}
		for i, s := range stellen {
			p.Anteile[s] = anteile[i][j-1]
		}
		return p
	}
	richtig := func(werte map[int]*big.Int, _ [32]byte) error {
		for i, s := range stellen {
			if werte[s] == nil || werte[s].Cmp(w[i]) != 0 {
				return quantenWFehler("falsch")
			}
		}
		return nil
	}
	falsch := paket(2)
	falsch.Anteile[9] = new(big.Int).Add(falsch.Anteile[9], big.NewInt(1))
	if _, _, err := quantenSAusAnteilenZusammensetzen([]quantenSAnteilPaket{paket(1), falsch, paket(4), paket(5)}, 3, richtig); err != nil {
		t.Fatalf("ehrliche Teilmenge nicht gefunden: %v", err)
	}
	if _, _, err := quantenSAusAnteilenZusammensetzen([]quantenSAnteilPaket{paket(1), falsch, paket(4)}, 3, richtig); err == nil {
		t.Fatal("mit einem falschen von drei Paketen angenommen")
	}
	if _, _, err := quantenSAusAnteilenZusammensetzen([]quantenSAnteilPaket{paket(1), paket(1), paket(4)}, 3, richtig); err == nil {
		t.Fatal("doppelter Validator angenommen")
	}
	if _, _, err := quantenSAusAnteilenZusammensetzen([]quantenSAnteilPaket{paket(1), paket(4)}, 3, richtig); err == nil {
		t.Fatal("zwei Pakete bei t = 3 angenommen")
	}
	andereStelle := paket(5)
	delete(andereStelle.Anteile, 9)
	andereStelle.Anteile[10] = big.NewInt(1)
	if _, _, err := quantenSAusAnteilenZusammensetzen([]quantenSAnteilPaket{paket(1), paket(4), andereStelle}, 3, func(map[int]*big.Int, [32]byte) error { return nil }); err == nil {
		t.Fatal("Pakete mit verschiedenen Stellen angenommen")
	}
	anderesSalz := paket(5)
	anderesSalz.Salz[0] = 1
	if _, _, err := quantenSAusAnteilenZusammensetzen([]quantenSAnteilPaket{paket(1), paket(4), anderesSalz}, 3, func(map[int]*big.Int, [32]byte) error { return nil }); err == nil {
		t.Fatal("Pakete mit verschiedenem Salz angenommen")
	}
}

// qaoProbeNetz: nach jedem Einreichen fragt es JEDEN Validator nach JEDEM
// Paket und zaehlt, was vor Bindung, Wahl oder Finalitaet herausging.
type qaoProbeNetz struct {
	*qsv2Netz
	zuFrueh, spaeter int
	// Blaetter: vor dem Token auf der Kette gibt keiner sie heraus; danach
	// ergeben sie auch ohne Validator 2 die Wurzel (R9).
	blaetterZuFrueh, blaetterGeprueft int
	selbst                            int // Pakete "fuer sich selbst" nicht ausdruecklich abgewiesen
}

func (n *qaoProbeNetz) einreichen(ctx context.Context, art, wallet string, zeit int64, a QuantenWAngaben) error {
	n.proben(a.K) // auch vor dem Schritt: zwischen Bestaetigung und Ausgabe
	err := n.qsv2Netz.einreichen(ctx, art, wallet, zeit, a)
	n.proben(a.K)
	return err
}

func (n *qaoProbeNetz) proben(K string) {
	tok := n.q.w.cs.quantenWJetzt().Token[K]
	for _, d := range n.q.d {
		if _, err := d.alleBlaetter(K); err == nil && (tok == nil || tok.token == nil) {
			n.blaetterZuFrueh++
		}
	}
	if tok != nil && tok.token != nil && n.blaetterGeprueft == 0 {
		ohne := &qsaNetz{qsNetz: n.qsaNetz.qsNetz, ausgefallen: 2}
		bl, err := quantenSBlaetterHolen(context.Background(), ohne, K, tok.token.N, len(n.q.w.val), n.q.w.val)
		if err != nil || quantenWMerkleWurzel(bl) != tok.token.Wurzel {
			n.blaetterGeprueft = -1
		} else {
			n.blaetterGeprueft = 1
		}
	}
	for _, d := range n.q.d {
		e, _ := d.eintragVon(K)
		if e == nil {
			continue
		}
		for x := 1; x <= len(e.Validatoren); x++ {
			for b := 0; b < 2; b++ {
				_, _, err := d.anteilOeffnungFuerStelle(K, x, b)
				if x == e.X && tok != nil && tok.token != nil && tok.token.D != nil && tok.token.C != -1 && !e.Wartend && d.reif(quantenSBindungSicht(K, tok.token)) {
					// Fuer sich selbst als Praeparator nie, und ausdruecklich so benannt.
					if err == nil || !strings.Contains(err.Error(), "kein anderer Validator") {
						n.selbst++
					}
					continue
				}
				if err == nil {
					if tok == nil || tok.token.D == nil || tok.token.C == -1 || !d.reif(quantenSBindungSicht(K, tok.token)) {
						n.zuFrueh++
					} else {
						n.spaeter++
					}
				}
			}
		}
	}
}

// Kein Paket vor Bindung und Wahl: nach jedem Schritt der Ausgabe fragt das
// Netz jeden Validator nach jedem Paket.
func TestQuantenSAnteilOeffnung_NichtVorDerBindung(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	q, a, bs, _ := qsaWerkzeug(t, 256)
	a.ProbeV2 = true
	netz := &qaoProbeNetz{qsv2Netz: &qsv2Netz{qsaNetz: &qsaNetz{qsNetz: &qsNetz{q: q}}}}
	if _, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, netz, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if netz.zuFrueh != 0 {
		t.Fatalf("%d Pakete vor Bindung und Wahl herausgegeben", netz.zuFrueh)
	}
	if netz.spaeter == 0 {
		t.Fatal("nach der Bindung kein einziges Paket -- die Probe misst nichts")
	}
	if netz.selbst != 0 {
		t.Fatalf("%d mal ein Paket fuer sich selbst als Praeparator nicht ausdruecklich abgewiesen", netz.selbst)
	}
	if netz.blaetterZuFrueh != 0 {
		t.Fatalf("%d mal Blaetter vor dem Token auf der Kette herausgegeben", netz.blaetterZuFrueh)
	}
	if netz.blaetterGeprueft != 1 {
		t.Fatal("ohne Validator 2 ergeben die Blaetter nicht die Wurzel")
	}
}

// Kein Paket vor der Finalitaet: die Bindung steht, ist aber nie final. Der
// Lauf bricht ab, und kein Validator gab ein Paket heraus.
func TestQuantenSAnteilOeffnung_NichtVorDerFinalitaet(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	q, a, bs, _ := qsaWerkzeug(t, 256)
	a.ProbeV2, a.Frist = true, 50e6
	q.hoehe.Store(10)
	q.final.Store(5)
	netz := &qaoProbeNetz{qsv2Netz: &qsv2Netz{qsaNetz: &qsaNetz{qsNetz: &qsNetz{q: q}, ausgefallen: 2}}}
	_, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, netz, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "nach der Bindung") {
		t.Fatalf("ohne Finalitaet: %v", err)
	}
	if netz.zuFrueh != 0 || netz.spaeter != 0 {
		t.Fatalf("Pakete ohne Finalitaet: %d zu frueh, %d", netz.zuFrueh, netz.spaeter)
	}
}

// Ein Buendel mit einem oder drei Paketen, oder einem zu grossen, wird
// abgewiesen; zwei passende werden durchgereicht.
func TestQuantenSBuendelOeffnenPakete_Form(t *testing.T) {
	praep, empf := qaoSchluessel(t), qaoSchluessel(t)
	praepAdr := crypto.PubkeyToAddress(praep.PublicKey).Hex()
	K := strings.Repeat("cd", 32)
	paket := strings.Repeat("ab", 100)
	for _, f := range []struct {
		pakete []string
		gut    bool
	}{
		{nil, true}, {[]string{paket, paket}, true}, {[]string{paket}, false}, {[]string{paket, paket, paket}, false},
		{[]string{paket, strings.Repeat("a", quantenSAnteilOeffnungHoechstens+2)}, false}, {[]string{paket, "XY"}, false},
	} {
		c, sig, err := quantenSBuendelVersiegeln(praep, &empf.PublicKey, quantenSBuendelInhalt{K: K, Von: 10, Bis: 12, Praeparator: 2, Empfaenger: 3, Anteile: []string{"1", "2"}, Oeffnung: f.pakete})
		if err != nil {
			t.Fatal(err)
		}
		_, p, err := quantenSBuendelOeffnenPakete(empf, c, sig, praepAdr, K, 2, 3, 10, 12)
		if (err == nil) != f.gut || (f.gut && len(p) != len(f.pakete)) {
			t.Errorf("%d Pakete: err=%v, %d zurueck", len(f.pakete), err, len(p))
		}
	}
}

// Ein Praeparator laesst bei Stichprobe v2 die Pakete weg (selbst
// unterschrieben): die anderen bestaetigen nicht -- sonst hinge das Laden
// wieder allein an ihm.
func TestQuantenSAnteilOeffnung_BuendelOhnePakete(t *testing.T) {
	defer qsv2Vorbereiten(t)()
	q, a, bs, _ := qsaWerkzeug(t, 256)
	a.ProbeV2 = true
	netz := &qsv2Netz{qsaNetz: &qsaNetz{qsNetz: &qsNetz{q: q}, ohnePakete: 2}}
	_, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, netz, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "ohne Pakete") {
		t.Fatalf("Buendel ohne Pakete: %v", err)
	}
}

// Bei Stichprobe v1 bleiben die Buendel ohne Pakete (wie bisher).
func TestQuantenSAnteilOeffnung_V1OhnePakete(t *testing.T) {
	q, a, bs, _ := qsaWerkzeug(t, 256)
	netz := &qsaNetz{qsNetz: &qsNetz{q: q}, ohnePakete: 2}
	if _, err := quantenSAusgabeLaufAbschnitte(context.Background(), a, bs, q.w.val, 3, netz, &bytes.Buffer{}); err != nil {
		t.Fatalf("v1 ohne Pakete: %v", err)
	}
}
