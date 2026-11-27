package keeper

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"math"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
)

// Weg C, Vormerken, Melden, Ziehen und Frist (quanten_s3_vormerkung.go): ein
// annehmender und ein nachspielender Knoten im Gleichschritt (qwWelt). drand
// ist ein Testverbund mit bekanntem Schluessel (drandTestNetz).

const (
	s3Leitung       = "leitung-test"
	s3N             = 100_000
	s3Gamma         = 200_000 // 20 %
	s3Deckung int64 = 2_000_000
)

// s3Pfand: das Pfand zur Deckung (C2b-2b-3a): max(1 AEQ, 10 %) -- wie s_kette.s3_pfand.
var s3Pfand = quantenS3PfandFuer(s3Deckung)

type s3Welt struct {
	*qsWelt
	sig    func(r uint64) string
	z      int64 // Zeit der Vormerkung
	wurzel string
}

func neueS3Welt(t *testing.T) *s3Welt {
	t.Helper()
	return neueS3WeltN(t, 64)
}

// neueS3WeltN: mit einer Praegung ueber n Stellen (fuer die Ausgabe: n = Pulse
// der Vormerkung, quanten_s3_ausgabe_test.go).
func neueS3WeltN(t *testing.T, n int) *s3Welt {
	t.Helper()
	w := &s3Welt{qsWelt: neueQsWelt(t, n, 24), z: qwT0, wurzel: strings.Repeat("5a", 32)}
	quantenLeitungAbOverride.Store(qwT0 - 30*86400)
	quantenS3AbOverride.Store(qwT0 - 30*86400)
	netz, sig := drandTestNetz(t, 0x53a11e)
	alt := drandQuicknet
	drandQuicknet, w.sig = netz, sig
	t.Cleanup(func() {
		quantenLeitungAbOverride.Store(0)
		quantenS3AbOverride.Store(0)
		drandQuicknet = alt
	})
	w.leitungSetzen(20, 50_000, s3Gamma, qwT0-86400)
	return w
}

// leitungSetzen: die Leitung im Register beider Knoten (wie nach einer
// Freischaltung mit zwei Dritteln der Pruefer).
func (w *s3Welt) leitungSetzen(theta, frei, gamma int, ab int64) {
	for _, cs := range []*ChainState{w.cs, w.nach} {
		reg := cs.quantenGeldJetzt().kopie()
		reg.Leitungen = nil
		cs.quantenGeld.Store(reg.mitLeitung(quantenLeitungEintrag{Name: s3Leitung, ThetaZehntel: theta, FreiPpm: frei, GammaPpm: gamma, Ab: ab, Seit: ab}))
	}
}

func (w *s3Welt) vormerkenTx(von string, key *ecdsa.PrivateKey, K, inhaber string, n int, deckung, zeit int64) Transaction {
	return Transaction{Type: quantenS3VormerkenTxArt, Wallet: von, DistributionAt: zeit, QuantenW: &QuantenWAngaben{K: K, Inhaber: inhaber, Leitung: s3Leitung,
		NPuls: n, Wurzel: w.wurzel, Deckung: deckung,
		WalletSig: personalSign(w.t, key, quantenS3VormerkenNachricht(von, K, inhaber, s3Leitung, n, w.wurzel, deckung, zeit))}}
}

func (w *s3Welt) meldenTx(von string, key *ecdsa.PrivateKey, K, text string) Transaction {
	h := quantenS3MeldungHash(text)
	return Transaction{Type: quantenS3MeldenTxArt, Wallet: von, QuantenW: &QuantenWAngaben{K: K, Meldung: text,
		WalletSig: personalSign(w.t, key, quantenS3MeldenNachricht(von, K, hex.EncodeToString(h[:])))}}
}

func (w *s3Welt) ziehenTx(K, text string, r uint64, sig string) Transaction {
	return Transaction{Type: quantenS3ZiehenTxArt, QuantenW: &QuantenWAngaben{K: K, Meldung: text, Runde: r, RundeSig: sig}}
}

func s3FristTx(K string) Transaction {
	return Transaction{Type: quantenS3FristTxArt, QuantenW: &QuantenWAngaben{K: K}}
}

func (w *s3Welt) regel(tx Transaction, zeit int64) string {
	w.t.Helper()
	return w.bescheid(w.cs, tx, zeit).regel
}

// abgewiesen: der Bescheid nennt genau diese Regel, und der nachspielende
// Knoten weist denselben Block ab.
func (w *s3Welt) abgewiesen(name string, tx Transaction, zeit int64, regel string) {
	w.t.Helper()
	if r := w.regel(tx, zeit); r != regel {
		w.t.Fatalf("%s: Regel %q, erwartet %q", name, r, regel)
	}
	tx.BuchAt = zeit
	if err := w.nachspielen(tx, zeit+7); err == nil {
		w.t.Fatalf("%s: im Block angenommen", name)
	}
}

func s3Lambda(seed int64, n, m int) []int {
	rng := rand.New(rand.NewSource(seed))
	lam := rng.Perm(n)[:m]
	sort.Ints(lam)
	return lam
}

func s3Text(t *testing.T, lam []int) string {
	t.Helper()
	text, err := quantenS3Kodieren(lam)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func (w *s3Welt) vs(cs *ChainState, K string) *quantenS3Vormerkung {
	return cs.quantenWJetzt().S3[K]
}

// bisGezogen: vormerken (praeger fuer anna), melden zu z+10, ziehen.
func (w *s3Welt) bisGezogen(K string, lam []int) (uint64, string) {
	text := s3Text(w.t, lam)
	w.annehmen(w.vormerkenTx(w.praeger, w.keys[w.praeger], K, w.anna, s3N, s3Deckung, w.z), w.z)
	w.annehmen(w.meldenTx(w.anna, w.keys[w.anna], K, text), w.z+10)
	r, _ := quantenS3ZiehRunde(w.z + 10)
	w.annehmen(w.ziehenTx(K, text, r, w.sig(r)), w.z+10+quantenS3ZiehAbstand+5)
	return r, text
}

// Gutfall wie s_kette.KetteS3: Deckung in den Pfandtopf, Meldung als Hash,
// Teilmenge aus der festen Runde, beide Knoten gleich, Register lesbar.
func TestQuantenS3_Gutfall(t *testing.T) {
	w := neueS3Welt(t)
	K := w.p.K
	summe := w.summe(w.cs)
	vorher := w.stand(w.cs, w.praeger)
	pfandVorher := w.stand(w.cs, quantenWPfandAddr)
	lam := s3Lambda(1, s3N, 20_000)
	text := s3Text(t, lam)

	tx := w.annehmen(w.vormerkenTx(w.praeger, w.keys[w.praeger], K, w.anna, s3N, s3Deckung, w.z), w.z)
	v := w.vs(w.cs, K)
	if v == nil || v.Von != w.praeger || v.Inhaber != w.anna || v.Gamma != s3Gamma || v.Theta != 20 || v.Frei != 50_000 || v.NPuls != s3N || v.Gemeldet != 0 || v.Zeit != w.z {
		t.Fatalf("Vormerkung: %+v", v)
	}
	if s3Pfand != quantenWPfand || quantenS3PfandFuer(20_000_000) != 2_000_000 || v.Pfand != s3Pfand || tx.QuantenW.Pfand != s3Deckung+s3Pfand || w.stand(w.cs, w.praeger) != vorher-s3Deckung-s3Pfand ||
		w.stand(w.cs, quantenWPfandAddr) != pfandVorher+s3Deckung+s3Pfand {
		t.Fatal("Deckung und Pfand nicht im Pfandtopf")
	}

	w.annehmen(w.meldenTx(w.anna, w.keys[w.anna], K, text), w.z+10)
	v = w.vs(w.cs, K)
	if v.Gemeldet != len(lam) || v.Meldung != quantenS3MeldungHash(text) || v.MeldungZeit != w.z+10 || v.Runde != 0 {
		t.Fatalf("Meldung: %+v", v)
	}
	if roh := w.cs.quantenWJetzt().kodiert(); strings.Contains(roh, text[:64]) || len(roh) > 4096 {
		t.Fatalf("die Meldung steht im Register (%d Bytes)", len(roh))
	}

	r, ok := quantenS3ZiehRunde(w.z + 10)
	zr, _ := drandQuicknet.zeitDerRunde(r)
	if !ok || zr < w.z+10+quantenS3ZiehAbstand || zr-3 >= w.z+10+quantenS3ZiehAbstand {
		t.Fatalf("Runde %d (Zeit %d) ist nicht die erste nach Meldung + Abstand", r, zr)
	}
	w.annehmen(w.ziehenTx(K, text, r, w.sig(r)), w.z+10+quantenS3ZiehAbstand+5)
	v = w.vs(w.cs, K)
	zufall, _ := drandQuicknet.pruefen(r, w.sig(r))
	soll, _ := quantenS3Teilmenge(K, lam, zufall[:], w.z+10, zr, quantenWNMax)
	if v.Runde != r || !reflect.DeepEqual(v.Teil, soll) || len(v.Teil) != quantenWNMax || v.Zufall != zufall {
		t.Fatalf("Teilmenge: %d Pulse, Runde %d", len(v.Teil), v.Runde)
	}
	// Register streng lesbar, dieselben Bytes; die Teilmenge steht kodiert darin.
	kod := w.cs.quantenWJetzt().kodiert()
	gelesen, err := quantenWRegisterLesen(kod)
	if err != nil || gelesen.kodiert() != kod || !reflect.DeepEqual(gelesen.S3[K].Teil, soll) {
		t.Fatalf("Register nicht lesbar: %v", err)
	}
	if z := w.cs.quantenWJetzt().form().zeilen(); z[quantenWZeile{quantenWArtS3, K}] == "" {
		t.Fatal("keine Zeile s3")
	}
	if w.summe(w.cs) != summe {
		t.Fatal("Erhaltung verletzt")
	}
	// Ansicht: Runde und Groesse der Teilmenge, nicht die Teilmenge selbst.
	a := w.cs.quantenWNoteAnsicht(context.Background(), K)
	if a["art"] != "s3" || a["runde"] != r || a["gezogen"] != quantenWNMax || a["teil"] != nil {
		t.Fatalf("Ansicht: %v", a)
	}
}

// Ohne Meldung gehen Deckung und Pfand mit der Frist zurueck, erst dann ist
// der Platz frei; Melden und Ziehen danach sind abgewiesen.
func TestQuantenS3_Frist(t *testing.T) {
	w := neueS3Welt(t)
	K := w.p.K
	vorher := w.stand(w.cs, w.praeger)
	lam := s3Lambda(2, s3N, 20_000)
	text := s3Text(t, lam)
	w.annehmen(w.vormerkenTx(w.praeger, w.keys[w.praeger], K, w.anna, s3N, s3Deckung, w.z), w.z)
	ende := w.z + quantenS3Frist
	w.abgewiesen("Frist vor dem Verfall", s3FristTx(K), ende, "quanten_w_frist")
	w.abgewiesen("Melden nach dem Verfall", w.meldenTx(w.anna, w.keys[w.anna], K, text), ende+1, "quanten_w_frist")
	K2 := strings.Repeat("c4", 32)
	w.abgewiesen("verfallene Vormerkung belegt den Ausgeber bis zur Frist",
		w.vormerkenTx(w.praeger, w.keys[w.praeger], K2, w.anna, s3N, s3Deckung, ende+1), ende+1, "quanten_s3_offen")
	tx := w.annehmen(s3FristTx(K), ende+1)
	if tx.QuantenW.Pfand != s3Deckung+s3Pfand || tx.QuantenW.PfandAn != w.praeger || tx.QuantenW.Wert != 0 || w.stand(w.cs, w.praeger) != vorher || w.vs(w.cs, K) != nil {
		t.Fatalf("Frist: %+v", tx.QuantenW)
	}
	w.abgewiesen("Frist zweimal", s3FristTx(K), ende+2, "quanten_w_unbekannt")
	// Nachspielen mit einem anderen Empfaenger im Block: abgewiesen.
	w2 := neueS3Welt(t)
	w2.annehmen(w2.vormerkenTx(w2.praeger, w2.keys[w2.praeger], K, w2.anna, s3N, s3Deckung, w2.z), w2.z)
	falsch := s3FristTx(K)
	falsch.QuantenW.Pfand, falsch.QuantenW.PfandAn, falsch.BuchAt = s3Deckung+s3Pfand, w2.bert, ende+1
	if err := w2.nachspielen(falsch, ende+8); err == nil {
		t.Fatal("Frist mit fremdem Empfaenger im Block angenommen")
	}
	// Ziehen nach dem Verfall.
	w3 := neueS3Welt(t)
	w3.annehmen(w3.vormerkenTx(w3.praeger, w3.keys[w3.praeger], K, w3.anna, s3N, s3Deckung, w3.z), w3.z)
	w3.annehmen(w3.meldenTx(w3.anna, w3.keys[w3.anna], K, text), w3.z+10)
	r, _ := quantenS3ZiehRunde(w3.z + 10)
	w3.abgewiesen("Ziehen nach dem Verfall", w3.ziehenTx(K, text, r, w3.sig(r)), ende+1, "quanten_w_frist")
}

// C2b-2b-3a, der Angriff aus C2b-2b-2: nach der Meldung steht die Runde fest,
// ihr Zufall wird oeffentlich, und jeder kann die Teilmenge selbst rechnen.
// Ein Abbruch danach (Frist statt Ausgabe) kostet das Pfand -- ob gezogen
// wurde oder nicht. Die Deckung geht zurueck, das Pfand ans Grundeinkommen.
func TestQuantenS3_PfandVerfaelltNachMeldung(t *testing.T) {
	for _, gezogen := range []bool{false, true} {
		w := neueS3Welt(t)
		K := w.p.K
		vorher := w.stand(w.cs, w.praeger)
		ubiVorher := w.stand(w.cs, ubiPoolAddr)
		pfandVorher := w.stand(w.cs, quantenWPfandAddr)
		summe := w.summe(w.cs)
		lam := s3Lambda(9, s3N, 20_000)
		text := s3Text(t, lam)
		w.annehmen(w.vormerkenTx(w.praeger, w.keys[w.praeger], K, w.anna, s3N, s3Deckung, w.z), w.z)
		w.annehmen(w.meldenTx(w.anna, w.keys[w.anna], K, text), w.z+10)
		if gezogen {
			r, _ := quantenS3ZiehRunde(w.z + 10)
			w.annehmen(w.ziehenTx(K, text, r, w.sig(r)), w.z+10+quantenS3ZiehAbstand+5)
		}
		ende := w.z + quantenS3Frist + 1
		// Ein Erzeuger, der das Pfand trotzdem zurueckgibt (oder gar nichts
		// verfallen laesst), faellt beim Nachspielen.
		for name, f := range map[string]func(q *QuantenWAngaben){
			"Pfand zurueck an den Ausgeber": func(q *QuantenWAngaben) { q.Pfand, q.PfandAn = s3Deckung+s3Pfand, w.praeger },
			"nichts verfaellt":              func(q *QuantenWAngaben) { q.Pfand, q.PfandAn = s3Deckung, w.praeger },
			"Pfand an den Ausgeber als Wert": func(q *QuantenWAngaben) {
				q.Pfand, q.PfandAn, q.Wert, q.WertAn = s3Deckung, w.praeger, s3Pfand, w.praeger
			},
			"weniger verfaellt": func(q *QuantenWAngaben) { q.Pfand, q.PfandAn, q.Wert = s3Deckung, w.praeger, s3Pfand-1 },
		} {
			falsch := s3FristTx(K)
			f(falsch.QuantenW)
			falsch.BuchAt = ende
			if err := w.nachspielen(falsch, ende+7); err == nil {
				t.Fatalf("gezogen=%v, %s: im Block angenommen", gezogen, name)
			}
		}
		tx := w.annehmen(s3FristTx(K), ende)
		q := tx.QuantenW
		if q.Pfand != s3Deckung || q.PfandAn != w.praeger || q.Wert != s3Pfand || q.WertAn != "" {
			t.Fatalf("gezogen=%v, Frist: %+v", gezogen, q)
		}
		if w.stand(w.cs, w.praeger) != vorher-s3Pfand || w.stand(w.cs, ubiPoolAddr) != ubiVorher+s3Pfand ||
			w.stand(w.cs, quantenWPfandAddr) != pfandVorher || w.summe(w.cs) != summe || w.vs(w.cs, K) != nil {
			t.Fatalf("gezogen=%v: Pfand nicht verfallen", gezogen)
		}
		if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() || w.stand(w.nach, ubiPoolAddr) != ubiVorher+s3Pfand {
			t.Fatalf("gezogen=%v: Knoten weichen ab", gezogen)
		}
	}
}

// Missbrauch beim Vormerken: jede Form, fremde Unterschrift, kein Mensch,
// K doppelt, zweite Vormerkung, Grenze, fremde Ausgabe eines vorgemerkten K.
func TestQuantenS3_AngriffeVormerken(t *testing.T) {
	w := neueS3Welt(t)
	K, z := w.p.K, w.z
	pk := w.keys[w.praeger]
	vm := func(f func(q *QuantenWAngaben)) Transaction {
		tx := w.vormerkenTx(w.praeger, pk, K, w.anna, s3N, s3Deckung, z)
		f(tx.QuantenW)
		return tx
	}
	w.abgewiesen("Ausgeber als Inhaberin", w.vormerkenTx(w.praeger, pk, K, w.praeger, s3N, s3Deckung, z), z, "quanten_w_form")
	w.abgewiesen("Inhaberin Systemadresse", w.vormerkenTx(w.praeger, pk, K, quantenWPfandAddr, s3N, s3Deckung, z), z, "quanten_w_form")
	w.abgewiesen("N = 0", w.vormerkenTx(w.praeger, pk, K, w.anna, 0, s3Deckung, z), z, "quanten_w_form")
	w.abgewiesen("N zu gross", w.vormerkenTx(w.praeger, pk, K, w.anna, quantenS3PulsMax+1, s3Deckung, z), z, "quanten_w_form")
	w.abgewiesen("Deckung unter dem Mindestmass", w.vormerkenTx(w.praeger, pk, K, w.anna, s3N, quantenMindestDeckungMik-1, z), z, "quanten_w_form")
	w.abgewiesen("Deckung fehlt", w.vormerkenTx(w.praeger, pk, K, w.anna, s3N, 1_000_000_000, z), z, "quanten_w_deckung")
	// Genau die Deckung da, das Pfand nicht: abgewiesen (C2b-2b-3a).
	alles := w.stand(w.cs, w.praeger)
	w.abgewiesen("Deckung da, Pfand fehlt", w.vormerkenTx(w.praeger, pk, K, w.anna, s3N, alles, z), z, "quanten_w_deckung")
	w.abgewiesen("Wurzel kein Hash", vm(func(q *QuantenWAngaben) { q.Wurzel = "zz" }), z, "quanten_w_form")
	w.abgewiesen("Satz statt Leitung", vm(func(q *QuantenWAngaben) { q.Leitung = "satz-test" }), z, "quanten_w_form")
	w.abgewiesen("unbekannte Leitung", vm(func(q *QuantenWAngaben) { q.Leitung = "leitung-fremd" }), z, "quanten_s3_leitung")
	w.abgewiesen("fremde Unterschrift", w.vormerkenTx(w.praeger, w.keys[w.bert], K, w.anna, s3N, s3Deckung, z), z, "quanten_w_unterschrift")
	w.abgewiesen("Unterschrift fuer kleinere Deckung", vm(func(q *QuantenWAngaben) { q.Deckung++ }), z, "quanten_w_unterschrift")
	w.abgewiesen("Unterschrift fuer andere Inhaberin", vm(func(q *QuantenWAngaben) { q.Inhaber = w.bert }), z, "quanten_w_unterschrift")
	w.abgewiesen("Unterschrift fuer anderes N", vm(func(q *QuantenWAngaben) { q.NPuls-- }), z, "quanten_w_unterschrift")
	w.abgewiesen("fremdes Feld (t)", vm(func(q *QuantenWAngaben) { q.T = 3 }), z, "quanten_w_form")
	// Pfand ist gerechnet: ein Block, der nur die Deckung (ohne Pfand) oder
	// weniger nennt, faellt beim Nachspielen.
	for _, p := range []int64{s3Deckung, s3Deckung + s3Pfand - 1} {
		falsch := vm(func(q *QuantenWAngaben) { q.Pfand = p })
		falsch.BuchAt = z
		if err := w.nachspielen(falsch, z+7); err == nil || !strings.Contains(err.Error(), "gerechnet") {
			t.Fatalf("falsches Pfand %d im Block: %v", p, err)
		}
	}
	keinMensch, a := neuerSchluessel(t)
	for _, cs := range []*ChainState{w.cs, w.nach} {
		cs.accounts.Set(a, &AccountState{Address: a, Balance: NewDecimal(100)})
	}
	w.abgewiesen("Ausgeber kein Mensch", w.vormerkenTx(a, keinMensch, K, w.anna, s3N, s3Deckung, z), z, "quanten_s_praep_mensch")
	w.nullifier(pfNAnna, w.praeger)
	w.nullifier(pfNBert, w.praeger)
	w.abgewiesen("zwei Nullifier fuer den Ausgeber", w.vormerkenTx(w.praeger, pk, K, w.anna, s3N, s3Deckung, z), z, "quanten_s_pfand_mensch")
	w.nullifier(pfNBert, "")
	w.nullifier(pfNAnna, "")
	tx := w.vormerkenTx(w.praeger, pk, K, w.anna, s3N, s3Deckung, z)
	tx.DistributionAt = 0
	w.abgewiesen("ohne Zeit", tx, z, "quanten_w_zeit")
	// Die Leitung gilt erst ab ihrem ab.
	w.leitungSetzen(20, 50_000, s3Gamma, z+1)
	w.abgewiesen("Leitung noch nicht gueltig", w.vormerkenTx(w.praeger, pk, K, w.anna, s3N, s3Deckung, z), z, "quanten_s3_leitung")
	w.leitungSetzen(20, 50_000, s3Gamma, z-86400)

	w.annehmen(w.vormerkenTx(w.praeger, pk, K, w.anna, s3N, s3Deckung, z), z)
	w.abgewiesen("K doppelt (anderer Ausgeber)", w.vormerkenTx(w.bert, w.keys[w.bert], K, w.anna, s3N, s3Deckung, z+1), z+1, "quanten_w_doppelt")
	w.abgewiesen("zweite offene Vormerkung", w.vormerkenTx(w.praeger, pk, strings.Repeat("c4", 32), w.anna, s3N, s3Deckung, z+1), z+1, "quanten_s3_offen")
	// Eine andere Ausgabe des vorgemerkten K: Praegen (W) und Ausgabe (S).
	w.abgewiesen("W-Praegung mit vorgemerktem K", w.praegenTx(z+1, s3Deckung), z+1, "quanten_w_doppelt")
	w.abgewiesen("S-Ausgabe mit vorgemerktem K (vor dem Ziehen)", w.ausgebenTx(z+1, s3Deckung), z+1, "quanten_s3_nicht_gezogen")

	// Grenze: hoechstens quantenS3VormerkungenHoechstens offene zugleich.
	for i := 1; i < quantenS3VormerkungenHoechstens; i++ {
		k, a := neuerSchluessel(t)
		addHuman(w.cs, a, 100)
		addHuman(w.nach, a, 100)
		w.annehmen(w.vormerkenTx(a, k, strings.Repeat(hex.EncodeToString([]byte{byte(i)}), 32), w.anna, s3N, s3Deckung, z+2), z+2)
	}
	k, a := neuerSchluessel(t)
	addHuman(w.cs, a, 100)
	addHuman(w.nach, a, 100)
	w.abgewiesen("mehr als die Grenze", w.vormerkenTx(a, k, strings.Repeat("ee", 32), w.anna, s3N, s3Deckung, z+3), z+3, "quanten_w_voll")
}

// Missbrauch beim Melden und Ziehen: fremde Meldung, gefaelschte
// Unterschrift, zu wenig, fremder Puls, zu lang, neu wuerfeln, fremde oder
// gefaelschte Runde, andere Meldung beim Ziehen, zweites Ziehen.
func TestQuantenS3_AngriffeMeldenZiehen(t *testing.T) {
	w := neueS3Welt(t)
	K, z := w.p.K, w.z
	lam := s3Lambda(3, s3N, 20_000)
	text := s3Text(t, lam)
	ak := w.keys[w.anna]
	w.abgewiesen("Melden ohne Vormerkung", w.meldenTx(w.anna, ak, K, text), z+10, "quanten_w_unbekannt")
	w.annehmen(w.vormerkenTx(w.praeger, w.keys[w.praeger], K, w.anna, s3N, s3Deckung, z), z)
	r, _ := quantenS3ZiehRunde(z + 10)
	zieh := z + 10 + quantenS3ZiehAbstand + 5
	w.abgewiesen("Ziehen vor der Meldung", w.ziehenTx(K, text, r, w.sig(r)), zieh, "quanten_s3_nicht_gemeldet")
	w.abgewiesen("fremde Meldung", w.meldenTx(w.bert, w.keys[w.bert], K, text), z+10, "quanten_s3_inhaberin")
	w.abgewiesen("Meldung mit fremder Unterschrift", w.meldenTx(w.anna, w.keys[w.bert], K, text), z+10, "quanten_w_unterschrift")
	gefaelscht := w.meldenTx(w.anna, ak, K, text)
	gefaelscht.QuantenW.Meldung = s3Text(t, s3Lambda(4, s3N, 20_000))
	w.abgewiesen("Unterschrift ueber eine andere Meldung", gefaelscht, z+10, "quanten_w_unterschrift")
	w.abgewiesen("Meldung im Moment der Vormerkung", w.meldenTx(w.anna, ak, K, text), z, "quanten_w_zeit")
	w.abgewiesen("Meldung unter gamma", w.meldenTx(w.anna, ak, K, s3Text(t, lam[:19_999])), z+10, "quanten_s3_meldung")
	w.abgewiesen("Puls ausserhalb N", w.meldenTx(w.anna, ak, K, s3Text(t, append(append([]int{}, lam[:19_999]...), s3N))), z+10, "quanten_s3_meldung")
	w.abgewiesen("zweite Form (Grossbuchstaben)", w.meldenTx(w.anna, ak, K, strings.ToUpper(text)), z+10, "quanten_s3_meldung")
	// quantenS3MeldungMax Pulse in N, 265 Luecken mit zwei Gruppen: gueltig
	// kodiert und genug, aber laenger als eine Transaktion (s_kette.py).
	var lang []int
	for j := 0; j < 265; j++ {
		lang = append(lang, 129*j)
	}
	for i := 129 * 265; len(lang) < quantenS3MeldungMax; i++ {
		lang = append(lang, i)
	}
	langText := s3Text(t, lang)
	if len(langText) <= quantenS3MeldungTxHexMax || quantenS3MeldungPruefen(s3N, s3Gamma, lang) != nil {
		t.Fatalf("lange Meldung: %d Zeichen", len(langText))
	}
	w.abgewiesen("Meldung laenger als eine Transaktion", w.meldenTx(w.anna, ak, K, langText), z+10, "quanten_w_form")
	tx := w.meldenTx(w.anna, ak, K, text)
	tx.DistributionAt = z + 10
	w.abgewiesen("Melden mit Zeit", tx, z+10, "quanten_w_form")
	tx = w.meldenTx(w.anna, ak, K, text)
	tx.QuantenW.Runde = r
	w.abgewiesen("Melden mit Runde", tx, z+10, "quanten_w_form")

	w.annehmen(w.meldenTx(w.anna, ak, K, text), z+10)
	anders := s3Text(t, s3Lambda(5, s3N, 20_000))
	w.abgewiesen("zweite, gueltige Meldung (neu wuerfeln)", w.meldenTx(w.anna, ak, K, anders), z+11, "quanten_s3_doppelt")
	w.abgewiesen("Ziehen mit einer anderen Meldung", w.ziehenTx(K, anders, r, w.sig(r)), zieh, "quanten_s3_meldung")
	w.abgewiesen("Ziehen mit zu langer Meldung", w.ziehenTx(K, langText, r, w.sig(r)), zieh, "quanten_s3_meldung")
	w.abgewiesen("fruehere Runde (schon bekannt)", w.ziehenTx(K, text, r-1, w.sig(r-1)), zieh, "quanten_s_los_runde")
	w.abgewiesen("spaetere Runde", w.ziehenTx(K, text, r+1, w.sig(r+1)), zieh, "quanten_s_los_runde")
	w.abgewiesen("Signatur einer anderen Runde", w.ziehenTx(K, text, r, w.sig(r+1)), zieh, "quanten_s_los_drand")
	tx = w.ziehenTx(K, text, r, w.sig(r))
	tx.Wallet = w.anna
	w.abgewiesen("Ziehen mit Wallet", tx, zieh, "quanten_w_form")
	w.annehmen(w.ziehenTx(K, text, r, w.sig(r)), zieh)
	w.abgewiesen("zweites Ziehen", w.ziehenTx(K, text, r, w.sig(r)), zieh+1, "quanten_s3_doppelt")
	w.abgewiesen("Frist mit Meldung", func() Transaction { f := s3FristTx(K); f.QuantenW.Meldung = text; return f }(), z+quantenS3Frist+1, "quanten_w_form")
}

// Die Vormerkung haelt die Werte der Leitung fest: ein spaeter anderes
// gamma (oder eine fehlende Leitung) aendert weder Melden noch Ziehen.
func TestQuantenS3_LeitungFest(t *testing.T) {
	w := neueS3Welt(t)
	K := w.p.K
	w.annehmen(w.vormerkenTx(w.praeger, w.keys[w.praeger], K, w.anna, s3N, s3Deckung, w.z), w.z)
	w.leitungSetzen(0, 0, 1, w.z-86400) // gamma 1 ppm: ein Puls genuegte
	w.abgewiesen("Meldung unter dem festgehaltenen gamma", w.meldenTx(w.anna, w.keys[w.anna], K, s3Text(t, []int{7})), w.z+10, "quanten_s3_meldung")
	for _, cs := range []*ChainState{w.cs, w.nach} {
		reg := cs.quantenGeldJetzt().kopie()
		reg.Leitungen = nil
		cs.quantenGeld.Store(reg)
	}
	lam := s3Lambda(6, s3N, 20_000)
	r, _ := w.meldenUndRunde(K, lam)
	if v := w.vs(w.cs, K); v.Gamma != s3Gamma || v.Runde != r {
		t.Fatalf("Vormerkung: %+v", v)
	}
}

func (w *s3Welt) meldenUndRunde(K string, lam []int) (uint64, string) {
	text := s3Text(w.t, lam)
	w.annehmen(w.meldenTx(w.anna, w.keys[w.anna], K, text), w.z+10)
	r, _ := quantenS3ZiehRunde(w.z + 10)
	w.annehmen(w.ziehenTx(K, text, r, w.sig(r)), w.z+10+quantenS3ZiehAbstand+5)
	return r, text
}

// Vor dem Stichtag (oder ohne aktives Register der Leitungen): Befund, und im
// Block unbekannt.
func TestQuantenS3_Schlafend(t *testing.T) {
	w := neueS3Welt(t)
	tx := w.vormerkenTx(w.praeger, w.keys[w.praeger], w.p.K, w.anna, s3N, s3Deckung, w.z)
	for _, f := range []func(){
		func() { quantenS3AbOverride.Store(math.MaxInt64) },
		func() { quantenS3AbOverride.Store(qwT0 - 86400); quantenLeitungAbOverride.Store(math.MaxInt64) },
	} {
		f()
		w.abgewiesen("schlafend", tx, w.z, "quanten_s3_inaktiv")
		for _, art := range []string{quantenS3VormerkenTxArt, quantenS3MeldenTxArt, quantenS3ZiehenTxArt, quantenS3FristTxArt} {
			if bekannteTxArt(art, w.z) {
				t.Fatalf("%s vor dem Stichtag bekannt", art)
			}
		}
		m := map[string]interface{}{}
		quantenS3Status(m)
		if _, da := m["s3_ab"]; da != (quantenS3Ab() != math.MaxInt64) {
			t.Fatalf("Status: %v", m)
		}
	}
	quantenS3AbOverride.Store(qwT0 - 86400)
	quantenLeitungAbOverride.Store(qwT0 - 86400)
	for _, art := range []string{quantenS3VormerkenTxArt, quantenS3MeldenTxArt, quantenS3ZiehenTxArt, quantenS3FristTxArt} {
		if !bekannteTxArt(art, w.z) || !istQuantenWTxArt(art) {
			t.Fatalf("%s ab dem Stichtag unbekannt", art)
		}
	}
	m := map[string]interface{}{}
	quantenS3Status(m)
	if m["s3_ab"] != qwT0-86400 {
		t.Fatalf("Status: %v", m)
	}
}

// Das Register von aussen (Snapshot, Tabelle): jede Abweichung faellt auf.
func TestQuantenS3_RegisterStreng(t *testing.T) {
	w := neueS3Welt(t)
	K := w.p.K
	lam := s3Lambda(7, s3N, 20_000)
	w.bisGezogen(K, lam)
	gut := w.cs.quantenWJetzt().form()
	if len(gut.S3) != 1 {
		t.Fatal("keine Vormerkung in der Form")
	}
	pruefen := func(name string, f func(fs *quantenWRegisterForm)) {
		t.Helper()
		k := gut
		k.S3 = append([]quantenS3VormerkungForm{}, gut.S3...)
		f(&k)
		if _, err := quantenWRegisterLesen(k.kodiert()); err == nil {
			t.Fatalf("%s: gelesen", name)
		}
	}
	teilOhne := func(v *quantenS3VormerkungForm, f func(teil []int) []int) {
		teil, _ := quantenS3Dekodieren(v.Teil)
		v.Teil, _ = quantenS3Kodieren(f(teil))
	}
	pruefen("Inhaberin = Ausgeber", func(fs *quantenWRegisterForm) { fs.S3[0].Inhaber = fs.S3[0].Von })
	pruefen("Nullifier nicht kanonisch", func(fs *quantenWRegisterForm) { fs.S3[0].N = "0123" })
	pruefen("gamma 0", func(fs *quantenWRegisterForm) { fs.S3[0].Gamma = 0 })
	pruefen("theta zu gross", func(fs *quantenWRegisterForm) { fs.S3[0].Theta = quantenSThetaZehntelMax + 1 })
	pruefen("kein Leitungsname", func(fs *quantenWRegisterForm) { fs.S3[0].Leitung = "satz-x" })
	pruefen("N 0", func(fs *quantenWRegisterForm) { fs.S3[0].NPuls = 0 })
	pruefen("Deckung zu klein", func(fs *quantenWRegisterForm) { fs.S3[0].Deckung = 1 })
	pruefen("Pfand fehlt", func(fs *quantenWRegisterForm) { fs.S3[0].Pfand = 0 })
	pruefen("Pfand kleiner", func(fs *quantenWRegisterForm) { fs.S3[0].Pfand-- })
	pruefen("Pfand groesser", func(fs *quantenWRegisterForm) { fs.S3[0].Pfand++ })
	pruefen("Deckung ohne passendes Pfand", func(fs *quantenWRegisterForm) { fs.S3[0].Deckung *= 10 })
	pruefen("weniger gemeldet als gamma", func(fs *quantenWRegisterForm) { fs.S3[0].Gemeldet = 19_999 })
	pruefen("gemeldet ohne Hash", func(fs *quantenWRegisterForm) { fs.S3[0].Meldung = "" })
	pruefen("Meldung nicht nach der Vormerkung", func(fs *quantenWRegisterForm) { fs.S3[0].MeldungZeit = fs.S3[0].Zeit })
	pruefen("andere Runde", func(fs *quantenWRegisterForm) { fs.S3[0].Runde++ })
	pruefen("Zufall fehlt", func(fs *quantenWRegisterForm) { fs.S3[0].Zufall = "" })
	pruefen("Teilmenge zu klein", func(fs *quantenWRegisterForm) {
		teilOhne(&fs.S3[0], func(teil []int) []int { return teil[1:] })
	})
	pruefen("Puls der Teilmenge ausserhalb N", func(fs *quantenWRegisterForm) {
		teilOhne(&fs.S3[0], func(teil []int) []int { return append(teil[1:], s3N) })
	})
	pruefen("Teilmenge ohne Runde", func(fs *quantenWRegisterForm) { fs.S3[0].Runde, fs.S3[0].Zufall = 0, "" })
	pruefen("Runde ohne Meldung", func(fs *quantenWRegisterForm) {
		fs.S3[0].Gemeldet, fs.S3[0].Meldung, fs.S3[0].MeldungZeit = 0, "", 0
	})
	pruefen("zweimal dasselbe K", func(fs *quantenWRegisterForm) { fs.S3 = append(fs.S3, fs.S3[0]) })
	pruefen("K zugleich Token", func(fs *quantenWRegisterForm) {
		tf := fs.S3[0]
		w2 := neueQsWelt(t, 64, 24)
		w2.annehmen(w2.ausgebenTx(qwT0, s3Deckung), qwT0)
		tok := w2.cs.quantenWJetzt().form().Token[0]
		tok.K = tf.K
		fs.Token = []quantenWSTokenForm{tok}
	})
	pruefen("zu viele", func(fs *quantenWRegisterForm) {
		for i := 0; i < quantenS3VormerkungenHoechstens; i++ {
			v := fs.S3[0]
			v.K = strings.Repeat(hex.EncodeToString([]byte{byte(0xa0 + i)}), 32)
			_, v.Von = neuerSchluessel(t)
			fs.S3 = append(fs.S3, v)
		}
		sort.Slice(fs.S3, func(i, j int) bool { return fs.S3[i].K < fs.S3[j].K })
	})
	// Unbekanntes Feld in der Zeile: streng abgewiesen.
	zeile := gut.zeilen()[quantenWZeile{quantenWArtS3, K}]
	var v quantenS3VormerkungForm
	if err := quantenWZeileLesen(strings.Replace(zeile, `{"k"`, `{"fremd":1,"k"`, 1), &v); err == nil {
		t.Fatal("Zeile mit fremdem Feld gelesen")
	}
	if _, err := quantenWRegisterLesen(gut.kodiert()); err != nil {
		t.Fatalf("das gute Register: %v", err)
	}
}

// Ueber die Annahme (Sperre, echte Uhr): die Frist zahlt an den Ausgeber, der
// in der Transaktion nicht steht -- er muss vorher gesperrt sein, sonst wiese
// die Annahme mit "wiederholen" ab (wie #688 beim Pfandgeber).
func TestQuantenS3_Annahme(t *testing.T) {
	w := neueS3Welt(t)
	for _, o := range []*atomic.Int64{&quantenWAbOverride, &quantenLeitungAbOverride, &quantenS3AbOverride} {
		o.Store(1)
	}
	w.leitungSetzen(20, 50_000, s3Gamma, 1)
	K := w.p.K
	jetzt := nowUnix()
	w.nullifier(pfNAnna, w.praeger)
	fertig, err := w.cs.quantenWAnnehmen(w.vormerkenTx(w.praeger, w.keys[w.praeger], K, w.anna, s3N, s3Deckung, jetzt))
	if err != nil || fertig.QuantenW.Pfand != s3Deckung+s3Pfand {
		t.Fatalf("Vormerken: %v", err)
	}
	if err := w.nachspielen(fertig, fertig.BuchAt+3); err != nil {
		t.Fatalf("Nachspielen Vormerken: %v", err)
	}
	if w.vs(w.cs, K).N != pfNAnna {
		t.Fatal("Nullifier nicht in der Vormerkung")
	}
	// Der Ausgeber zieht um; die Vormerkung altert (beide Knoten gleich).
	_, neu := neuerSchluessel(t)
	addHuman(w.cs, neu, 0)
	addHuman(w.nach, neu, 0)
	w.nullifier(pfNAnna, neu)
	for _, cs := range []*ChainState{w.cs, w.nach} {
		reg := cs.quantenWJetzt().kopie()
		v := *reg.S3[K]
		v.Zeit = jetzt - quantenS3Frist - 10
		reg.S3[K] = &v
		cs.mu.Lock()
		if err := cs.quantenWSetzenLocked(context.Background(), reg); err != nil {
			t.Fatal(err)
		}
		cs.mu.Unlock()
	}
	fertig, err = w.cs.quantenWAnnehmen(s3FristTx(K))
	if err != nil || fertig.QuantenW.PfandAn != neu || w.stand(w.cs, neu) != s3Deckung+s3Pfand {
		t.Fatalf("Frist ueber die Annahme (an die neue Wallet): %v %+v", err, fertig.QuantenW)
	}
	if err := w.nachspielen(fertig, fertig.BuchAt+3); err != nil {
		t.Fatalf("Nachspielen Frist: %v", err)
	}
	if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() || w.stand(w.nach, neu) != s3Deckung+s3Pfand {
		t.Fatal("Knoten weichen ab")
	}
}

// Die Meldezeit bestimmt die Runde: ein Erzeuger darf sie hoechstens
// quantenSBestellBuchSpiel vor den Block legen (sonst waehlte er eine schon
// bekannte Runde).
func TestQuantenS3_MeldezeitNichtZurueckdatiert(t *testing.T) {
	w := neueS3Welt(t)
	K, z := w.p.K, w.z
	w.annehmen(w.vormerkenTx(w.praeger, w.keys[w.praeger], K, w.anna, s3N, s3Deckung, z), z)
	tx := w.meldenTx(w.anna, w.keys[w.anna], K, s3Text(t, s3Lambda(8, s3N, 20_000)))
	tx.BuchAt = z + 10
	if err := w.nachspielen(tx, z+10+quantenSBestellBuchSpiel+1); err == nil || !strings.Contains(err.Error(), "passt nicht zu Block") {
		t.Fatalf("zurueckdatierte Meldung im Block: %v", err)
	}
	if quantenS3ZiehAbstand <= quantenSBestellBuchSpiel+60 {
		t.Fatal("die Runde laege bei der Annahme schon in der Vergangenheit")
	}
	if err := w.nachspielen(tx, z+10+quantenSBestellBuchSpiel); err != nil {
		t.Fatalf("Meldung im Spiel: %v", err)
	}
}
