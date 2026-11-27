package keeper

import (
	"encoding/hex"
	"math/rand"
	"slices"
	"sort"
	"strings"
	"testing"
)

// Weg C, Ausgabe ueber die gezogene Teilmenge (quanten_s3_ausgabe.go): ein
// Token, dessen Stelle i Puls teil[i] der Vormerkung ist, mit der Wurzel ueber
// ALLE Pulse. 20.000 Pulse: die Pfade sind 15 Glieder lang, laenger als
// quantenWPfadMax (14) der bisherigen Token.

const (
	s3AN    = 20_000 // Pulse der Vormerkung
	s3ASend = 5      // Sender = Validatoren der Welt (je ein Abschnitt von 4.000 Pulsen)
	s3AMeld = 4_500  // gemeldet: je Abschnitt 900 (gamma 20 % verlangt 800); teil = Meldung
	s3AK    = 32     // Pruefgroesse im Ablauf (quantenSKLeitungOverride)
	s3AProb = 40     // Stellen je Stichprobe
)

type s3AWelt struct {
	*s3Welt
	teil []int
}

// neueS3AWelt: vormerken (mit der echten Wurzel), melden, ziehen. Danach
// stehen Stichproben, Messbasen und Vorlage der Inhaberin je STELLE des
// Tokens (nicht je Puls) in der qsWelt.
func neueS3AWelt(t *testing.T) *s3AWelt {
	t.Helper()
	w := &s3AWelt{s3Welt: neueS3WeltN(t, s3AN)}
	w.wurzel, w.sender = hex.EncodeToString(w.p.wurzel[:]), s3ASend
	quantenSKLeitungOverride.Store(s3AK)
	t.Cleanup(func() { quantenSKLeitungOverride.Store(0) })
	w.bisGezogenN(s3LambdaJe(11, s3AN, s3ASend, s3AMeld/s3ASend))
	w.teil = slices.Clone(w.vs(w.cs, w.p.K).Teil)
	if len(w.teil) != s3AMeld {
		t.Fatalf("Teilmenge %d", len(w.teil))
	}
	w.stellenVorbereiten(s3AProb)
	return w
}

// s3LambdaJe: je Abschnitt von v Sendern genau je zufaellige Pulse.
func s3LambdaJe(seed int64, n, v, je int) []int {
	rng := rand.New(rand.NewSource(seed))
	var lam []int
	for x := 1; x <= v; x++ {
		von, bis, _ := quantenS3AbschnittPulse(n, v, x)
		for _, p := range rng.Perm(bis - von)[:je] {
			lam = append(lam, von+p)
		}
	}
	sort.Ints(lam)
	return lam
}

// bisGezogenN: wie bisGezogen, mit N = Pulse der Praegung.
func (w *s3AWelt) bisGezogenN(lam []int) {
	text := s3Text(w.t, lam)
	K := w.p.K
	w.annehmen(w.vormerkenTx(w.praeger, w.keys[w.praeger], K, w.anna, s3AN, s3Deckung, w.z), w.z)
	w.annehmen(w.meldenTx(w.anna, w.keys[w.anna], K, text), w.z+10)
	r, _ := quantenS3ZiehRunde(w.z + 10)
	w.annehmen(w.ziehenTx(K, text, r, w.sig(r)), w.z+10+quantenS3ZiehAbstand+5)
}

// stellenVorbereiten: Stichproben ueber die n Stellen, Messung der Inhaberin
// an Stelle j gegen Puls teil[j].
func (w *s3AWelt) stellenVorbereiten(probe int) {
	n := len(w.teil)
	r := rand.New(rand.NewSource(77))
	for b := 0; b < 2; b++ {
		w.probe[b] = r.Perm(n)[:probe]
		sort.Ints(w.probe[b])
	}
	w.messbasen, w.x = nil, nil
	for j := 0; j < n; j++ {
		m, puls := r.Intn(2), w.teil[j]
		w.messbasen = append(w.messbasen, m)
		if m == w.p.basen[puls] {
			w.x = append(w.x, w.p.bits[puls])
		} else {
			w.x = append(w.x, r.Intn(2))
		}
	}
}

// ausgebenS3Tx: Ausgabe ueber die Teilmenge; aendern passt die Angaben vor
// den Unterschriften an (Angriffe), von unterschreibt als Ausgeber.
func (w *s3AWelt) ausgebenS3Tx(zeit int64, von string, aendern func(q *QuantenWAngaben)) Transaction {
	p := w.p
	q := &QuantenWAngaben{K: p.K, N: len(w.teil), T: p.t, Wurzel: hex.EncodeToString(p.wurzel[:]), Validatoren: w.val, Deckung: s3Deckung,
		Tol: qsTol, Inhaber: w.anna, Annahme: []string{w.stelle[0], w.stelle[1]}, Proben: w.probenHex()}
	if aendern != nil {
		aendern(q)
	}
	wurzel, _ := quantenWHex32(q.Wurzel)
	var proben [2][32]byte
	for b := 0; b < 2; b++ {
		proben[b], _ = quantenWHex32(q.Proben[b])
	}
	for x := range q.Validatoren {
		q.Bestaetigungen = append(q.Bestaetigungen, personalSign(w.t, w.valKeys[x], quantenWSBestaetigungNachricht(p.K, wurzel, q.N, q.T, q.Validatoren, x+1, proben, zeit, q.Tol)))
	}
	q.WalletSig = personalSign(w.t, w.keys[von], quantenWSAusgabeNachricht(von, p.K, q.N, q.T, q.Wurzel, q.Validatoren, q.Proben, q.Annahme, q.Inhaber, q.Deckung, zeit, q.Tol))
	return Transaction{Type: quantenWSAusgebenTxArt, Wallet: von, DistributionAt: zeit, QuantenW: q}
}

// offenS3Tx: Stellen der Stichprobe mit Anteilen und Pfaden. blatt bildet
// Stelle j auf das Blatt ab (ehrlich: teil[j]).
func (w *s3AWelt) offenS3Tx(ab int, stellen []int, blatt func(j int) int, blaetter [][32]byte) Transaction {
	q := &QuantenWAngaben{K: w.p.K, Ab: ab}
	for _, j := range stellen {
		i := blatt(j)
		var of QuantenWOffen
		for _, x := range []int{1, 3, 5} {
			of.Anteile = append(of.Anteile, QuantenWAnteilAngabe{X: x, Y: w.p.anteile[i][x-1].Text(16)})
		}
		for _, h := range quantenWMerklePfad(blaetter, i) {
			of.Pfad = append(of.Pfad, hex.EncodeToString(h[:]))
		}
		q.Offen = append(q.Offen, of)
	}
	return Transaction{Type: quantenWSOffenlegenTxArt, QuantenW: q}
}

func (w *s3AWelt) ehrlich(j int) int { return w.teil[j] }

const s3ATeil = 16 // Stellen je Offenlegung (Pfade sind lang)

// bisAngemeldet: ausgeben, binden, waehlen, an Stelle b anmelden.
func (w *s3AWelt) bisAngemeldet(z int64, b int) {
	w.t.Helper()
	w.annehmen(w.ausgebenS3Tx(z, w.praeger, nil), z)
	w.annehmen(w.bindenTx(w.anna), z+1)
	w.annehmen(w.waehlenTx(w.anna, string(rune('0'+w.c))), z+2)
	w.annehmen(w.anmeldenTx(w.stelle[b], b, w.probe[b], w.vorlage(b, w.keys[w.anna])), z+3)
}

// Gutfall: Ausgabe ueber die Teilmenge, Geld genau einmal, dann Einloesen mit
// Pfaden zu teil[j] im Baum ueber alle Pulse (15 Glieder).
func TestQuantenS3Ausgabe_Gutfall(t *testing.T) {
	w := neueS3AWelt(t)
	K := w.p.K
	summe := w.summe(w.cs)
	ausgeber := w.stand(w.cs, w.praeger)
	pfandTopf := w.stand(w.cs, quantenWPfandAddr)
	topf := w.stand(w.cs, quantenWTopfAddr)
	z := w.z + quantenS3ZiehAbstand + 100

	tx := w.annehmen(w.ausgebenS3Tx(z, w.praeger, nil), z)
	tok := w.token(w.cs)
	if tok == nil || !slices.Equal(tok.Pulse, w.teil) || tok.NPuls != s3AN || tok.KMin != s3AK || tok.N != s3AMeld || w.vs(w.cs, K) != nil {
		t.Fatalf("Token: %+v", tok)
	}
	if tx.QuantenW.Pfand != s3Pfand || tx.QuantenW.PfandAn != w.praeger || tx.QuantenW.Anteile <= 0 {
		t.Fatalf("Felder: %+v", tx.QuantenW.gerechnet())
	}
	if w.stand(w.cs, w.praeger) != ausgeber+s3Pfand || w.stand(w.cs, quantenWPfandAddr) != pfandTopf-s3Deckung-s3Pfand ||
		w.stand(w.cs, quantenWTopfAddr) != topf+s3Deckung || w.summe(w.cs) != summe {
		t.Fatal("Deckung nicht genau einmal im W-Topf oder Pfand nicht zurueck")
	}
	if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() {
		t.Fatal("Knoten weichen ab")
	}
	if tok.pfadMax() != 15 || quantenWMerkleTiefe(s3AN) != 15 {
		t.Fatalf("Pfadgrenze %d", tok.pfadMax())
	}
	a := w.cs.quantenWSTokenAnsichtLocked(t.Context(), w.cs.quantenWJetzt(), K)
	if a["n_puls"] != s3AN || a["k_min"] != s3AK || a["pulse"] == nil {
		t.Fatalf("Ansicht: %v", a)
	}

	w.annehmen(w.bindenTx(w.anna), z+1)
	w.annehmen(w.waehlenTx(w.anna, string(rune('0'+w.c))), z+2)
	w.annehmen(w.anmeldenTx(w.stelle[0], 0, w.probe[0], w.vorlage(0, w.keys[w.anna])), z+3)
	stelle := w.stand(w.cs, w.stelle[0])
	var g QuantenWAngaben
	for ab := 0; ab < s3AProb; ab += s3ATeil {
		bis := min(ab+s3ATeil, s3AProb)
		g = w.annehmen(w.offenS3Tx(ab, w.probe[0][ab:bis], w.ehrlich, w.p.blaetter), z+4).QuantenW.gerechnet()
	}
	if g.Urteil != "echt" || g.WertAn != w.stelle[0] || w.stand(w.cs, w.stelle[0]) <= stelle || w.token(w.cs) != nil {
		t.Fatalf("Einloesen: %+v", g)
	}
	// summe zaehlt die Annahmestellen nicht mit: die Deckung ist dorthin gegangen.
	if w.summe(w.cs)+w.stand(w.cs, w.stelle[0])-stelle != summe {
		t.Fatalf("Erhaltung nach dem Einloesen: %d != %d", w.summe(w.cs)+w.stand(w.cs, w.stelle[0])-stelle, summe)
	}
	if a, b := w.cs.quantenWJetzt().kodiert(), w.nach.quantenWJetzt().kodiert(); a != b {
		t.Fatalf("Gleichschritt nach dem Einloesen:\n%s\n%s", a, b)
	}
}

// Angriffe auf die Ausgabe: jede Abweichung von der Vormerkung, vor dem
// Ziehen, nach der Frist, unsichere Pruefgroesse, falsche Felder im Block.
func TestQuantenS3Ausgabe_Angriffe(t *testing.T) {
	w := neueS3AWelt(t)
	z := w.z + quantenS3ZiehAbstand + 100
	ab := func(name string, tx Transaction, zeit int64, regel string) {
		t.Helper()
		w.abgewiesen(name, tx, zeit, regel)
	}
	ab("fremder Ausgeber", w.ausgebenS3Tx(z, w.bert, nil), z, "quanten_s3_ausgabe")
	ab("andere Inhaberin", w.ausgebenS3Tx(z, w.praeger, func(q *QuantenWAngaben) { q.Inhaber = w.bert }), z, "quanten_s3_ausgabe")
	ab("andere Deckung", w.ausgebenS3Tx(z, w.praeger, func(q *QuantenWAngaben) { q.Deckung = s3Deckung - 1 }), z, "quanten_s3_ausgabe")
	ab("andere Wurzel (Werte nach dem Ziehen gewaehlt)", w.ausgebenS3Tx(z, w.praeger, func(q *QuantenWAngaben) { q.Wurzel = strings.Repeat("ab", 32) }), z, "quanten_s3_ausgabe")
	ab("n kleiner als die Teilmenge", w.ausgebenS3Tx(z, w.praeger, func(q *QuantenWAngaben) { q.N = s3AMeld - 1 }), z, "quanten_s3_ausgabe")
	ab("nach der Frist", w.ausgebenS3Tx(w.z+quantenS3Frist+1, w.praeger, nil), w.z+quantenS3Frist+1, "quanten_w_frist")
	quantenSKLeitungOverride.Store(s3AMeld/2 + 1)
	ab("2k > n", w.ausgebenS3Tx(z, w.praeger, nil), z, "quanten_s3_pruefgroesse")
	quantenSKLeitungOverride.Store(0)
	// Echte Tabelle: 2 Grad, 5 % frei, t 3 von 5 -> f16 8 -> k 7320 > n/2.
	ab("echte Pruefgroesse der Leitung zu gross fuer n", w.ausgebenS3Tx(z, w.praeger, nil), z, "quanten_s3_pruefgroesse")
	quantenSKLeitungOverride.Store(s3AK)

	// Der Block nennt das Pfand nicht (der Erzeuger behaelt es im Topf) oder
	// gibt es einem anderen: der Nachspielende weist ab.
	for name, f := range map[string]func(q *QuantenWAngaben){
		"ohne Pfand":             func(q *QuantenWAngaben) { q.Pfand, q.PfandAn = 0, "" },
		"Pfand an einen Dritten": func(q *QuantenWAngaben) { q.PfandAn = w.bert },
	} {
		u := w.bescheid(w.cs, w.ausgebenS3Tx(z, w.praeger, nil), z)
		tx := w.ausgebenS3Tx(z, w.praeger, nil)
		tx.QuantenW.Anteile, tx.QuantenW.Pfand, tx.QuantenW.PfandAn = u.felder.Anteile, u.felder.Pfand, u.felder.PfandAn
		f(tx.QuantenW)
		tx.BuchAt = z
		if err := w.nachspielen(tx, z+7); err == nil {
			t.Fatalf("%s: im Block angenommen", name)
		}
	}
	// Ein gewoehnliches S-Ausgabe-K bleibt unberuehrt; Frist nach der Ausgabe gibt es nicht.
	w.annehmen(w.ausgebenS3Tx(z, w.praeger, nil), z)
	ab("zweite Ausgabe", w.ausgebenS3Tx(z+1, w.praeger, nil), z+1, "quanten_w_doppelt")
	ab("Frist nach der Ausgabe", s3FristTx(w.p.K), w.z+quantenS3Frist+1, "quanten_w_unbekannt")
}

// Liegt der Stichtag (wieder) in der Zukunft, gibt auch eine stehende
// Vormerkung nichts aus.
func TestQuantenS3Ausgabe_VorDemStichtag(t *testing.T) {
	w := neueS3AWelt(t)
	z := w.z + quantenS3ZiehAbstand + 100
	quantenS3AbOverride.Store(z + 86400)
	w.abgewiesen("Ausgabe vor dem Stichtag", w.ausgebenS3Tx(z, w.praeger, nil), z, "quanten_w_doppelt")
	quantenS3AbOverride.Store(qwT0 - 30*86400)
	w.annehmen(w.ausgebenS3Tx(z, w.praeger, nil), z)
}

// Vor dem Ziehen keine Ausgabe.
func TestQuantenS3Ausgabe_VorDemZiehen(t *testing.T) {
	w := &s3AWelt{s3Welt: neueS3WeltN(t, s3AN)}
	w.wurzel, w.sender = hex.EncodeToString(w.p.wurzel[:]), s3ASend
	quantenSKLeitungOverride.Store(s3AK)
	t.Cleanup(func() { quantenSKLeitungOverride.Store(0) })
	lam := s3LambdaJe(12, s3AN, s3ASend, s3AMeld/s3ASend)
	w.annehmen(w.vormerkenTx(w.praeger, w.keys[w.praeger], w.p.K, w.anna, s3AN, s3Deckung, w.z), w.z)
	w.annehmen(w.meldenTx(w.anna, w.keys[w.anna], w.p.K, s3Text(t, lam)), w.z+10)
	w.teil = lam
	w.stellenVorbereiten(s3AProb)
	w.abgewiesen("gemeldet, nicht gezogen", w.ausgebenS3Tx(w.z+20, w.praeger, nil), w.z+20, "quanten_s3_nicht_gezogen")
}

// Einloesen: Pfade muessen zu teil[j] im Baum ueber ALLE Pulse gehoeren; eine
// Stichprobe unter der Pruefgroesse der Leitung wird nicht angemeldet.
func TestQuantenS3Ausgabe_EinloesenAngriffe(t *testing.T) {
	w := neueS3AWelt(t)
	z := w.z + quantenS3ZiehAbstand + 100
	w.bisAngemeldet(z, 0)
	st := w.probe[0][:s3ATeil]
	// Blatt j statt teil[j] im selben Baum.
	w.abgewiesen("Blatt der Stelle statt des Pulses", w.offenS3Tx(0, st, func(j int) int { return j }, w.p.blaetter), z+4, "quanten_w_regel")
	// Ein Baum nur ueber die gezogenen Pulse (andere Wurzel, kuerzere Pfade).
	var klein [][32]byte
	for _, i := range w.teil {
		klein = append(klein, w.p.blaetter[i])
	}
	tx := w.offenS3Tx(0, st, func(j int) int { return j }, klein)
	w.abgewiesen("Baum nur ueber die Teilmenge", tx, z+4, "quanten_w_regel")
	// Pfade laenger als der Baum tief ist.
	lang := w.offenS3Tx(0, st, w.ehrlich, w.p.blaetter)
	for i := range lang.QuantenW.Offen {
		lang.QuantenW.Offen[i].Pfad = append(lang.QuantenW.Offen[i].Pfad, strings.Repeat("00", 32))
	}
	w.abgewiesen("Pfad zu lang", lang, z+4, "quanten_w_form")
	w.annehmen(w.offenS3Tx(0, st, w.ehrlich, w.p.blaetter), z+4)

	// Stichprobe unter KMin: die Stelle 1 hat bei der Ausgabe eine zu kleine
	// Probe gebunden -- anmelden wird abgewiesen.
	w2 := neueS3AWelt(t)
	w2.probe[1] = w2.probe[1][:s3AK-1]
	w2.annehmen(w2.ausgebenS3Tx(z, w2.praeger, nil), z)
	w2.annehmen(w2.bindenTx(w2.anna), z+1)
	w2.annehmen(w2.waehlenTx(w2.anna, string(rune('0'+w2.c))), z+2)
	w2.abgewiesen("Stichprobe unter der Pruefgroesse der Leitung",
		w2.anmeldenTx(w2.stelle[1], 1, w2.probe[1], w2.vorlage(1, w2.keys[w2.anna])), z+3, "quanten_w_regel")
}

// Das Register von aussen: die Felder der Leitung streng.
func TestQuantenS3Ausgabe_RegisterStreng(t *testing.T) {
	w := neueS3AWelt(t)
	z := w.z + quantenS3ZiehAbstand + 100
	w.bisAngemeldet(z, 0)
	gut := w.cs.quantenWJetzt().form()
	if len(gut.Token) != 1 || gut.Token[0].Pulse == "" {
		t.Fatal("Token ohne Pulse in der Form")
	}
	if _, err := quantenWRegisterLesen(gut.kodiert()); err != nil {
		t.Fatalf("gute Form: %v", err)
	}
	pruefen := func(name string, f func(tf *quantenWSTokenForm)) {
		t.Helper()
		k := gut
		k.Token = append([]quantenWSTokenForm{}, gut.Token...)
		tf := k.Token[0]
		if tf.Anmeldung != nil {
			an := *tf.Anmeldung
			tf.Anmeldung = &an
		}
		f(&tf)
		k.Token[0] = tf
		if _, err := quantenWRegisterLesen(k.kodiert()); err == nil {
			t.Fatalf("%s: gelesen", name)
		}
	}
	pruefen("Pulse fehlen", func(tf *quantenWSTokenForm) { tf.Pulse = "" })
	pruefen("NPuls fehlt", func(tf *quantenWSTokenForm) { tf.NPuls = 0 })
	pruefen("KMin fehlt", func(tf *quantenWSTokenForm) { tf.KMin = 0 })
	pruefen("2 KMin > N", func(tf *quantenWSTokenForm) { tf.KMin, tf.Anmeldung = tf.N/2+1, nil })
	ohneAnmeldung := gut
	ohneAnmeldung.Token = append([]quantenWSTokenForm{}, gut.Token...)
	ohneAnmeldung.Token[0].Anmeldung, ohneAnmeldung.Token[0].KMin = nil, gut.Token[0].N/2
	if _, err := quantenWRegisterLesen(ohneAnmeldung.kodiert()); err != nil {
		t.Fatalf("2 KMin = N ist erlaubt: %v", err)
	}
	pruefen("Puls ausserhalb NPuls", func(tf *quantenWSTokenForm) { tf.NPuls = w.teil[len(w.teil)-1] })
	pruefen("ein Puls zu wenig", func(tf *quantenWSTokenForm) { tf.Pulse, _ = quantenS3Kodieren(w.teil[1:]) })
	pruefen("NPuls zu gross", func(tf *quantenWSTokenForm) { tf.NPuls = quantenS3PulsMax + 1 })
	pruefen("Anmeldung unter KMin", func(tf *quantenWSTokenForm) { tf.KMin = len(w.probe[0]) + 1 })
	pruefen("nur KMin bei gewoehnlichem Token", func(tf *quantenWSTokenForm) { tf.Pulse, tf.NPuls = "", 0 })
}

// Bisherige Token: keine neuen Felder in der Form (StateRoot gleich), Pfade
// wie immer hoechstens quantenWPfadMax, Blatt = Stelle.
func TestQuantenS3Ausgabe_BisherigeTokenUnveraendert(t *testing.T) {
	w := neueQsWelt(t, 64, 24)
	w.bereit(qwT0)
	f := w.cs.quantenWJetzt().form()
	b := string(f.kodiert())
	if strings.Contains(b, "pulse") || strings.Contains(b, "n_puls") || strings.Contains(b, "k_min") {
		t.Fatal("bisheriger Token mit Feldern der Leitung")
	}
	tok := w.token(w.cs)
	if tok.pfadMax() != quantenWPfadMax || tok.baum() != 64 || !tok.probeGross(1) {
		t.Fatalf("bisheriger Token: pfadMax %d baum %d", tok.pfadMax(), tok.baum())
	}
	if _, ok := quantenWBlatt(nil, 5); !ok {
		t.Fatal("Blatt ohne Pulse")
	}
	if _, ok := quantenWBlatt([]int{7}, 1); ok {
		t.Fatal("Stelle ausserhalb der Pulse angenommen")
	}
}

// Die Deckung liegt seit der Vormerkung im Pfandtopf: ein Ausgeber ohne
// weiteres Guthaben gibt trotzdem aus (und zahlt nicht zweimal).
func TestQuantenS3Ausgabe_OhneGuthaben(t *testing.T) {
	w := neueS3AWelt(t)
	for _, cs := range []*ChainState{w.cs, w.nach} {
		acc, _ := cs.accounts.Get(w.praeger)
		acc.Balance = NewDecimal(0)
		cs.accounts.Set(w.praeger, acc)
	}
	z := w.z + quantenS3ZiehAbstand + 100
	w.annehmen(w.ausgebenS3Tx(z, w.praeger, nil), z)
	if w.stand(w.cs, w.praeger) != s3Pfand || w.stand(w.nach, w.praeger) != s3Pfand {
		t.Fatalf("Ausgeber: %d (nur das Pfand zurueck)", w.stand(w.cs, w.praeger))
	}
}

// Nachweis einer Faelschung (P4) bei einem Token ueber eine Leitung: Werte
// und Pfade gehoeren ebenso zu teil[j] im Baum ueber alle Pulse.
func TestQuantenS3Ausgabe_NachweisMitPulsen(t *testing.T) {
	w := neueS3AWelt(t)
	z := w.z + quantenS3ZiehAbstand + 100
	w.bisAngemeldet(z, 0)
	werte := func(blatt func(j int) int) []QuantenWOffen {
		var out []QuantenWOffen
		for _, j := range w.probe[0][:s3ATeil] {
			i := blatt(j)
			var as []quantenWAnteil
			for _, x := range []int{1, 3, 5} {
				as = append(as, quantenWAnteil{X: x, Y: w.p.anteile[i][x-1]})
			}
			o := QuantenWOffen{W: quantenWZusammensetzen(as).Text(16)}
			for _, h := range quantenWMerklePfad(w.p.blaetter, i) {
				o.Pfad = append(o.Pfad, hex.EncodeToString(h[:]))
			}
			out = append(out, o)
		}
		return out
	}
	kopie := func() *quantenSToken {
		tk := *w.token(w.cs)
		an := *tk.Anmeldung
		tk.Anmeldung = &an
		return &tk
	}
	tok := kopie()
	if _, ok := quantenSWerteLesen(werte(w.ehrlich)); ok {
		t.Fatal("15 Pfadglieder mit der bisherigen Grenze gelesen")
	}
	gut, ok := quantenSWerteLesenMax(werte(w.ehrlich), tok.pfadMax())
	if !ok {
		t.Fatal("Werte mit der Grenze des Tokens nicht gelesen")
	}
	if _, _, err := tok.nachweisOffenlegenTeil(w.p.K, 0, gut, z+4); err != nil {
		t.Fatalf("ehrliche Werte: %v", err)
	}
	falsch, _ := quantenSWerteLesenMax(werte(func(j int) int { return j }), tok.pfadMax())
	if _, _, err := kopie().nachweisOffenlegenTeil(w.p.K, 0, falsch, z+4); err == nil {
		t.Fatal("Werte zum Blatt der Stelle statt des Pulses angenommen")
	}
}
