package keeper

import (
	"errors"
	"math"
	"strings"
	"testing"
)

// Missbrauch: der Blockerzeuger schreibt eine Bindung NACH dem Stichtag der
// Pfaender in einen Block, gibt ihr aber eine Buchzeit DAVOR -- dann naehme
// sie kein Pfand der Inhaberin. Jeder Nachspielende weist den Block ab; vor
// dem Stichtag ist derselbe Satz gueltig.
func TestQuantenSPfaenderGrenze_ZurueckdatiertUeberDenStichtag(t *testing.T) {
	w := neueIPfandWelt(t, false)
	z := qwT0
	ab := z + 1000
	quantenSPfaenderAbOverride.Store(ab)
	w.annehmen(w.ausgebenTx(z, ipDeckung), z+1) // beide Knoten
	// Bindung gebucht 10 s vor dem Stichtag: kein Pfand (Regel davor).
	binden := w.bindenTx(w.anna)
	u := w.bescheid(w.cs, binden, ab-10)
	if u.regel != "" || u.felder.Pfand != 0 {
		t.Fatalf("Bindung vor dem Stichtag: %q %+v", u.regel, u.felder)
	}
	q := *binden.QuantenW
	q.Pfand = u.felder.Pfand
	binden.QuantenW, binden.BuchAt = &q, ab-10
	for _, blockZeit := range []int64{ab, ab + 10, ab + 86400} {
		err := w.nachspielen(binden, blockZeit)
		if err == nil || !strings.Contains(err.Error(), "gebucht um") {
			t.Errorf("Buchzeit %d vor dem Stichtag %d im Block %d: %v", binden.BuchAt, ab, blockZeit, err)
		}
	}
	// Der Erzeuger waehlt fuer sie nur eine Blockzeit vor dem Stichtag.
	if f := auftragsFenster(&binden, ab+10); f.bis != ab-1 {
		t.Fatalf("Fenster bis %d, erwartet %d", f.bis, ab-1)
	}
	if err := w.nachspielen(binden, ab-5); err != nil {
		t.Fatalf("Block vor dem Stichtag: %v", err)
	}
}

// Die Grenze selbst: nur "Buchzeit davor, Block danach" ist falsch; schlafend
// gilt sie nie.
func TestQuantenSPfaenderGrenze_Regel(t *testing.T) {
	t.Cleanup(func() { quantenSPfaenderAbOverride.Store(0) })
	ab := int64(1_900_000_000)
	quantenSPfaenderAbOverride.Store(ab)
	faelle := []struct {
		buch, block int64
		ok          bool
	}{
		{ab - 1, ab - 1, true},
		{ab - 1, ab, false},
		{ab - 7*86400, ab + 1, false},
		{ab, ab, true},
		{ab + 30, ab - 30, true}, // Buchzeit bis 60 s nach dem Block: echte Annahme danach
		{ab + 100, ab + 200, true},
	}
	for _, f := range faelle {
		if got := quantenSPfaenderGrenzeOk(f.buch, f.block); got != f.ok {
			t.Errorf("Buchzeit %d, Block %d: %v, erwartet %v", f.buch-ab, f.block-ab, got, f.ok)
		}
	}
	quantenSPfaenderAbOverride.Store(math.MaxInt64)
	if !quantenSPfaenderGrenzeOk(1, math.MaxInt64-1) {
		t.Error("schlafend: Grenze greift")
	}
	var f zeitFenster = offenesFenster()
	quantenSPfaenderFenster(&Transaction{BuchAt: 5}, &f)
	if f != offenesFenster() {
		t.Error("schlafend: Fenster eingeengt")
	}
}

// Annahme: quantenSPfaenderPause vor dem Stichtag nimmt der Knoten keine
// W-Transaktion an (wiederholbar); davor und ab dem Stichtag wieder.
func TestQuantenSPfaenderGrenze_Pause(t *testing.T) {
	t.Cleanup(func() { quantenSPfaenderAbOverride.Store(0) })
	ab := int64(1_900_000_000)
	quantenSPfaenderAbOverride.Store(ab)
	for _, jetzt := range []int64{ab - quantenSPfaenderPause, ab - 1} {
		err := quantenSPfaenderPausePruefen(jetzt)
		if !errors.Is(err, ErrAnnahmePausiert) || !istWiederholbareAnnahmeAblehnung(err) {
			t.Errorf("%d s vor dem Stichtag: %v", ab-jetzt, err)
		}
	}
	for _, jetzt := range []int64{ab - quantenSPfaenderPause - 1, ab, ab + 1} {
		if err := quantenSPfaenderPausePruefen(jetzt); err != nil {
			t.Errorf("%d: %v", jetzt-ab, err)
		}
	}
	quantenSPfaenderAbOverride.Store(math.MaxInt64)
	if err := quantenSPfaenderPausePruefen(math.MaxInt64 - 1); err != nil {
		t.Errorf("schlafend: %v", err)
	}

	// Ueber den echten Annahmeweg.
	w := neueIPfandWelt(t, false)
	quantenSPfaenderAbOverride.Store(nowUnix() + 60)
	_, err := w.cs.quantenWAnnehmen(ipZurueckTx(w.p.K))
	if !errors.Is(err, ErrAnnahmePausiert) {
		t.Fatalf("Annahme kurz vor dem Stichtag: %v", err)
	}
}
