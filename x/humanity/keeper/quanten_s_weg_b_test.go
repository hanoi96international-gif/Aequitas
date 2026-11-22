package keeper

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Weg B (quanten_s_weg_b.go): ein annehmender und ein nachspielender Knoten
// im Gleichschritt, wie die W-Tests. Gelost wird mit dem echten
// quicknet-Beacon der Runde drandEchtRunde (drand_zufall_test.go).

// qbBestellZeit: die Bestellzeit, zu der die feste Runde genau drandEchtRunde ist.
var qbBestellZeit = drandQuicknetGenesis + (drandEchtRunde-1)*drandQuicknetPeriode - quantenSLosAbstand

type qbWelt struct {
	t        *testing.T
	cs, nach *ChainState
	keys     map[string]*ecdsa.PrivateKey
	menschen []string // gemeldet werden koennen
	null     map[string]string
}

func neueQbWelt(t *testing.T, menschen int) *qbWelt {
	t.Helper()
	quantenWAbOverride.Store(1)
	quantenSWegBAbOverride.Store(1)
	t.Cleanup(func() { quantenWAbOverride.Store(0); quantenSWegBAbOverride.Store(0) })
	w := &qbWelt{t: t, cs: newTestState(), nach: newTestState(), keys: map[string]*ecdsa.PrivateKey{}, null: map[string]string{}}
	w.cs.nullifiers, w.nach.nullifiers = map[string]string{}, map[string]string{}
	for i := 0; i < menschen; i++ {
		w.mensch(fmt.Sprintf("null-%03d", i))
	}
	return w
}

// mensch: eine neue Wallet, Mensch mit Nullifier n (in beiden Knoten).
func (w *qbWelt) mensch(n string) string {
	k, a := neuerSchluessel(w.t)
	w.keys[a] = k
	addHuman(w.cs, a, 10)
	addHuman(w.nach, a, 10)
	if n != "" {
		w.cs.nullifiers[n], w.nach.nullifiers[n] = a, a
		w.null[a] = n
	}
	w.menschen = append(w.menschen, a)
	return a
}

func (w *qbWelt) melden(a string, z int64) Transaction {
	return Transaction{Type: quantenSMeldenTxArt, Wallet: a, DistributionAt: z,
		QuantenW: &QuantenWAngaben{WalletSig: personalSign(w.t, w.keys[a], quantenSMeldenNachricht(a, z))}}
}

func (w *qbWelt) abmelden(a string, gemeldet, z int64) Transaction {
	return Transaction{Type: quantenSAbmeldenTxArt, Wallet: a, DistributionAt: z,
		QuantenW: &QuantenWAngaben{WalletSig: personalSign(w.t, w.keys[a], quantenSAbmeldenNachricht(a, gemeldet, z))}}
}

func qbK(i int) string { return fmt.Sprintf("%064x", 0xb2a000+i) }

func (w *qbWelt) bestellen(a, K string, v, t int, z int64) Transaction {
	return Transaction{Type: quantenSBestellenTxArt, Wallet: a, DistributionAt: z,
		QuantenW: &QuantenWAngaben{K: K, V: v, T: t, WalletSig: personalSign(w.t, w.keys[a], quantenSBestellenNachricht(a, K, v, t, z))}}
}

func qbLosen(K string, runde uint64, sig string) Transaction {
	return Transaction{Type: quantenSLosenTxArt, QuantenW: &QuantenWAngaben{K: K, Runde: runde, DrandSig: sig}}
}

func (w *qbWelt) bescheid(cs *ChainState, tx Transaction, zeit int64) quantenWBescheid {
	w.t.Helper()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	u, err := cs.quantenWBescheidLocked(context.Background(), &tx, zeit)
	if err != nil {
		w.t.Fatal(err)
	}
	return u
}

// annehmen: anwenden im ersten Knoten, nachspielen im zweiten; beide
// Register gleich.
func (w *qbWelt) annehmen(tx Transaction, zeit int64) {
	w.t.Helper()
	u := w.bescheid(w.cs, tx, zeit)
	if u.regel != "" {
		w.t.Fatalf("%s abgewiesen: %s: %s", tx.Type, u.regel, u.text)
	}
	w.cs.mu.Lock()
	err := w.cs.quantenWAnwendenLocked(context.Background(), u)
	w.cs.mu.Unlock()
	if err != nil {
		w.t.Fatal(err)
	}
	tx.BuchAt = zeit
	w.nach.mu.Lock()
	err = quantenWFeldErlaubt(&tx, zeit)
	if err == nil {
		err = w.nach.applyQuantenWLocked(context.Background(), &tx, zeit+5)
	}
	w.nach.mu.Unlock()
	if err != nil {
		w.t.Fatalf("Nachspielen %s: %v", tx.Type, err)
	}
	if w.cs.quantenWJetzt().kodiert() != w.nach.quantenWJetzt().kodiert() || w.cs.quantenWWurzel() != w.nach.quantenWWurzel() {
		w.t.Fatal("Register der beiden Knoten weichen ab")
	}
}

// abgewiesen: die Regel, mit der der erste Knoten ablehnt ("" = angenommen).
func (w *qbWelt) abgewiesen(tx Transaction, zeit int64) string {
	w.t.Helper()
	return w.bescheid(w.cs, tx, zeit).regel
}

// gemeldetAlle: alle Menschen melden sich zwei Tage vor der Bestellung.
func (w *qbWelt) gemeldetAlle() int64 {
	z := qbBestellZeit - 2*86400
	for _, a := range w.menschen {
		w.annehmen(w.melden(a, z), z)
	}
	return z
}

func TestQuantenSWegB_FesteRunde(t *testing.T) {
	if r := quantenSLosRunde(qbBestellZeit); r != drandEchtRunde {
		t.Fatalf("feste Runde %d, erwartet %d", r, drandEchtRunde)
	}
	// Die Runde liegt mindestens den Abstand nach der Bestellung, die
	// vorige nicht.
	for _, z := range []int64{qbBestellZeit, qbBestellZeit + 1, qbBestellZeit + 2, qbBestellZeit + 3, drandQuicknetGenesis - 100} {
		r := quantenSLosRunde(z)
		tr, _ := drandQuicknet.zeitDerRunde(r)
		if tr < z+quantenSLosAbstand {
			t.Fatalf("Bestellung %d: Runde %d zu frueh", z, r)
		}
		if vor, ok := drandQuicknet.zeitDerRunde(r - 1); ok && vor >= z+quantenSLosAbstand {
			t.Fatalf("Bestellung %d: Runde %d nicht die erste", z, r)
		}
	}
}

// Gutfall: melden, bestellen, losen mit dem echten Beacon; das Los ist das
// der Spezifikation aus dem Register zur Bestellung.
func TestQuantenSWegB_Gutfall(t *testing.T) {
	w := neueQbWelt(t, 14)
	w.gemeldetAlle()
	K := qbK(1)
	w.annehmen(w.bestellen(w.menschen[0], K, 4, 3, qbBestellZeit), qbBestellZeit)
	w.annehmen(qbLosen(K, drandEchtRunde, drandEchtSig), qbBestellZeit+60)
	b := w.cs.quantenWJetzt().Bestellt[K]
	z, _ := drandQuicknet.pruefen(drandEchtRunde, drandEchtSig)
	erwartet, err := quantenSLos(slices.Sorted(slices.Values(w.menschen)), z, K, 4)
	if err != nil || !reflect.DeepEqual(b.Los, erwartet) || b.Runde != drandEchtRunde {
		t.Fatalf("Los %v (Runde %d), erwartet %v (%v)", b.Los, b.Runde, erwartet, err)
	}
	// Register kodiert, gelesen, wieder kodiert: dieselben Bytes; Zeilen ebenso.
	reg := w.cs.quantenWJetzt()
	r2, err := quantenWRegisterLesen(reg.kodiert())
	if err != nil || r2.kodiert() != reg.kodiert() {
		t.Fatalf("Register nicht stabil: %v", err)
	}
	if len(reg.form().zeilen()) != 14+1 {
		t.Fatalf("Zeilen: %d", len(reg.form().zeilen()))
	}
	// Zweimal losen geht nicht.
	if r := w.abgewiesen(qbLosen(K, drandEchtRunde, drandEchtSig), qbBestellZeit+90); r != "quanten_s_los" {
		t.Fatalf("zweites Los: %q", r)
	}
}

// Missbrauch beim Losen: andere Runde (Wahl unter Runden), gefaelschte
// Signatur, fremde Runde mit echter Signatur, unbekanntes K, nach der Frist.
func TestQuantenSWegB_LosenMissbrauch(t *testing.T) {
	w := neueQbWelt(t, 14)
	w.gemeldetAlle()
	K := qbK(2)
	w.annehmen(w.bestellen(w.menschen[0], K, 4, 3, qbBestellZeit), qbBestellZeit)
	falsch := []byte(drandEchtSig)
	falsch[10] ^= 1
	for name, tx := range map[string]Transaction{
		"spaetere Runde":   qbLosen(K, drandEchtRunde+1, drandEchtSig),
		"fruehere Runde":   qbLosen(K, drandEchtRunde-1, drandEchtSig),
		"Signatur falsch":  qbLosen(K, drandEchtRunde, string(falsch)),
		"Signatur leer":    qbLosen(K, drandEchtRunde, ""),
		"unbekanntes K":    qbLosen(qbK(99), drandEchtRunde, drandEchtSig),
		"mit Wallet":       {Type: quantenSLosenTxArt, Wallet: w.menschen[1], QuantenW: &QuantenWAngaben{K: K, Runde: drandEchtRunde, DrandSig: drandEchtSig}},
		"mit fremdem Feld": {Type: quantenSLosenTxArt, QuantenW: &QuantenWAngaben{K: K, Runde: drandEchtRunde, DrandSig: drandEchtSig, N: 1}},
	} {
		if r := w.abgewiesen(tx, qbBestellZeit+60); r == "" {
			t.Fatalf("%s: angenommen", name)
		}
	}
	// Nach der Frist ist die Bestellung weg.
	if r := w.abgewiesen(qbLosen(K, drandEchtRunde, drandEchtSig), qbBestellZeit+quantenSLosFrist+1); r != "quanten_s_los" {
		t.Fatalf("nach der Frist: %q", r)
	}
}

// Grinding: mit einem Netz, dessen Schluessel der Test kennt, ist JEDE Runde
// gueltig unterschrieben. Trotzdem gilt nur die feste Runde -- wer einreicht,
// kann nicht unter vielen Runden die guenstigste waehlen.
func TestQuantenSWegB_KeineWahlDerRunde(t *testing.T) {
	netz, signiere := drandTestNetz(t, 424242)
	alt := quantenSDrand
	quantenSDrand = netz
	t.Cleanup(func() { quantenSDrand = alt })
	w := neueQbWelt(t, 14)
	w.gemeldetAlle()
	K := qbK(4)
	w.annehmen(w.bestellen(w.menschen[0], K, 4, 3, qbBestellZeit), qbBestellZeit)
	fest := quantenSLosRunde(qbBestellZeit)
	for _, r := range []uint64{fest - 1, fest + 1, fest + 2, fest + 100} {
		if _, err := netz.pruefen(r, signiere(r)); err != nil {
			t.Fatalf("Testnetz: Runde %d: %v", r, err)
		}
		if reg := w.abgewiesen(qbLosen(K, r, signiere(r)), qbBestellZeit+400); reg != "quanten_s_los" {
			t.Fatalf("Runde %d (fest %d) gueltig unterschrieben: %q", r, fest, reg)
		}
	}
	w.annehmen(qbLosen(K, fest, signiere(fest)), qbBestellZeit+400)
}

// Wer nach der Bestellung abmeldet, verschiebt das Los nicht (sonst koennte,
// wer die Runde kennt, das Los verschieben). Wer vor der Bestellung
// abgemeldet war oder zu spaet gemeldet, ist nicht im Los.
func TestQuantenSWegB_LosAusDemRegisterZurBestellung(t *testing.T) {
	w := neueQbWelt(t, 14)
	z0 := w.gemeldetAlle()
	K := qbK(3)
	zufall, _ := drandQuicknet.pruefen(drandEchtRunde, drandEchtSig)
	alle := slices.Sorted(slices.Values(w.menschen))
	erwartet, _ := quantenSLos(alle, zufall, K, 4)
	// Ein Gezogener meldet sich nach der Bestellung ab; ein Neuer meldet
	// sich eine Stunde vor der Bestellung an (Vorlauf nicht erfuellt).
	spaet := w.mensch("null-spaet")
	w.annehmen(w.melden(spaet, qbBestellZeit-3600), qbBestellZeit-3600)
	w.annehmen(w.bestellen(w.menschen[0], K, 4, 3, qbBestellZeit), qbBestellZeit)
	w.annehmen(w.abmelden(erwartet[0], z0, qbBestellZeit+10), qbBestellZeit+10)
	w.annehmen(qbLosen(K, drandEchtRunde, drandEchtSig), qbBestellZeit+60)
	if los := w.cs.quantenWJetzt().Bestellt[K].Los; !reflect.DeepEqual(los, erwartet) {
		t.Fatalf("Los %v, erwartet %v", los, erwartet)
	}
	// Abgemeldet VOR der Bestellung: nicht im Los.
	w2 := neueQbWelt(t, 14)
	z0 = w2.gemeldetAlle()
	alle2 := slices.Sorted(slices.Values(w2.menschen))
	vorher, _ := quantenSLos(alle2, zufall, K, 4)
	w2.annehmen(w2.abmelden(vorher[0], z0, qbBestellZeit-100), qbBestellZeit-100)
	w2.annehmen(w2.bestellen(w2.menschen[1], K, 4, 3, qbBestellZeit), qbBestellZeit)
	w2.annehmen(qbLosen(K, drandEchtRunde, drandEchtSig), qbBestellZeit+60)
	los := w2.cs.quantenWJetzt().Bestellt[K].Los
	if slices.Contains(los, vorher[0]) {
		t.Fatal("vor der Bestellung Abgemeldeter im Los")
	}
	ohne := slices.DeleteFunc(slices.Clone(alle2), func(a string) bool { return a == vorher[0] })
	if e, _ := quantenSLos(ohne, zufall, K, 4); !reflect.DeepEqual(los, e) {
		t.Fatalf("Los %v, erwartet %v", los, e)
	}
}

// Register: nur Menschen mit Nullifier, je Wallet ein Eintrag, ein Mensch
// ein aktiver Eintrag (Umzug), Abmelden bindet die Meldezeit.
func TestQuantenSWegB_RegisterMissbrauch(t *testing.T) {
	w := neueQbWelt(t, 3)
	z := qbBestellZeit
	a := w.menschen[0]
	w.annehmen(w.melden(a, z), z)
	if r := w.abgewiesen(w.melden(a, z+1), z+1); r != "quanten_s_register" {
		t.Fatalf("zweimal gemeldet: %q", r)
	}
	// Kein Mensch.
	k, fremd := neuerSchluessel(t)
	w.keys[fremd] = k
	if r := w.abgewiesen(w.melden(fremd, z), z); r != "quanten_s_mensch" {
		t.Fatalf("kein Mensch: %q", r)
	}
	// Mensch ohne Nullifier.
	ohne := w.mensch("")
	if r := w.abgewiesen(w.melden(ohne, z), z); r != "quanten_s_mensch" {
		t.Fatalf("ohne Nullifier: %q", r)
	}
	// Fremde Unterschrift, Zeit ausserhalb des Fensters.
	tx := w.melden(w.menschen[1], z)
	tx.QuantenW.WalletSig = personalSign(t, w.keys[a], quantenSMeldenNachricht(w.menschen[1], z))
	if r := w.abgewiesen(tx, z); r != "quanten_w_unterschrift" {
		t.Fatalf("fremde Unterschrift: %q", r)
	}
	if r := w.abgewiesen(w.melden(w.menschen[1], z-quantenWAlt-10), z); r != "quanten_w_zeit" {
		t.Fatalf("alte Zeit: %q", r)
	}
	// Umzug: derselbe Mensch (Nullifier) mit neuer Wallet -- der alte
	// Eintrag wird abgemeldet, nie zwei aktive.
	neu := w.mensch("")
	w.cs.nullifiers[w.null[a]], w.nach.nullifiers[w.null[a]] = neu, neu
	w.annehmen(w.melden(neu, z+50), z+50)
	reg := w.cs.quantenWJetzt()
	if reg.Praep[a].Abgemeldet != z+50 || reg.Praep[neu].Abgemeldet != 0 {
		t.Fatalf("Umzug: alt %+v, neu %+v", reg.Praep[a], reg.Praep[neu])
	}
	// Abmelden: falsche Meldezeit in der Unterschrift (alte Unterschrift)
	// geht nicht; richtige geht einmal.
	if r := w.abgewiesen(w.abmelden(neu, z, z+60), z+60); r != "quanten_w_unterschrift" {
		t.Fatalf("Abmelden mit fremder Meldezeit: %q", r)
	}
	w.annehmen(w.abmelden(neu, z+50, z+60), z+60)
	if r := w.abgewiesen(w.abmelden(neu, z+50, z+61), z+61); r != "quanten_s_register" {
		t.Fatalf("zweimal abgemeldet: %q", r)
	}
	// Wieder melden erst, wenn der alte Eintrag weg ist (dann ist auch eine
	// alte Melde-Unterschrift abgelaufen).
	if r := w.abgewiesen(w.melden(neu, z+100), z+100); r != "quanten_s_register" {
		t.Fatalf("sofort wieder gemeldet: %q", r)
	}
	spaeter := z + 60 + quantenWToteBehalten + 1
	w.annehmen(w.melden(neu, spaeter), spaeter)
	if p := w.cs.quantenWJetzt().Praep[neu]; p.Gemeldet != spaeter || p.Abgemeldet != 0 {
		t.Fatalf("neu gemeldet: %+v", p)
	}
	if _, da := w.cs.quantenWJetzt().Praep[a]; da {
		t.Fatal("alter Eintrag nicht aufgeraeumt")
	}
}

// Bestellen: nur Menschen, v und t in Grenzen, K frei, je Wallet eine offene
// Bestellung; eine Ausgabe zum bestellten K wird abgewiesen (B2b).
func TestQuantenSWegB_BestellenMissbrauch(t *testing.T) {
	w := neueQbWelt(t, 14)
	w.gemeldetAlle()
	z := qbBestellZeit
	a := w.menschen[0]
	k, fremd := neuerSchluessel(t)
	w.keys[fremd] = k
	for name, f := range map[string]struct {
		tx   Transaction
		soll string
	}{
		"kein Mensch": {w.bestellen(fremd, qbK(10), 4, 3, z), "quanten_s_mensch"},
		"v zu klein":  {w.bestellen(a, qbK(10), 3, 2, z), "quanten_w_form"},
		"v zu gross":  {w.bestellen(a, qbK(10), 17, 3, z), "quanten_w_form"},
		"t zu klein":  {w.bestellen(a, qbK(10), 4, 1, z), "quanten_w_form"},
		"t ueber v":   {w.bestellen(a, qbK(10), 4, 5, z), "quanten_w_form"},
		"K kein Hash": {w.bestellen(a, "abc", 4, 3, z), "quanten_w_form"},
		"ohne Zeit":   {w.bestellen(a, qbK(10), 4, 3, 0), "quanten_w_form"},
		"fremde Sig":  {func() Transaction { x := w.bestellen(a, qbK(10), 4, 3, z); x.QuantenW.V = 5; return x }(), "quanten_w_unterschrift"},
		"mit Feld N":  {func() Transaction { x := w.bestellen(a, qbK(10), 4, 3, z); x.QuantenW.N = 9; return x }(), "quanten_w_form"},
		"mit Validat.": {func() Transaction {
			x := w.bestellen(a, qbK(10), 4, 3, z)
			x.QuantenW.Validatoren = []string{a}
			return x
		}(), "quanten_w_form"},
	} {
		if r := w.abgewiesen(f.tx, z); r != f.soll {
			t.Fatalf("%s: %q, erwartet %q", name, r, f.soll)
		}
	}
	w.annehmen(w.bestellen(a, qbK(10), 4, 3, z), z)
	if r := w.abgewiesen(w.bestellen(a, qbK(11), 4, 3, z+1), z+1); r != "quanten_s_bestellung" {
		t.Fatalf("zweite offene Bestellung: %q", r)
	}
	if r := w.abgewiesen(w.bestellen(w.menschen[1], qbK(10), 4, 3, z+1), z+1); r != "quanten_s_bestellung" {
		t.Fatalf("K doppelt: %q", r)
	}
	// Ausgabe zum bestellten K: abgewiesen, bevor irgendetwas anderes zaehlt.
	aus := Transaction{Type: quantenWSAusgebenTxArt, Wallet: a, DistributionAt: z + 2, QuantenW: &QuantenWAngaben{K: qbK(10)}}
	w.cs.mu.Lock()
	u, err := w.cs.quantenWSAusgebenBescheidLocked(context.Background(), &aus, a, w.cs.quantenWJetzt(), z+2)
	w.cs.mu.Unlock()
	if err != nil || u.regel != "quanten_s_bestellung" {
		t.Fatalf("Ausgabe zum bestellten K: %q (%v)", u.regel, err)
	}
	// Ungelost nach der Frist verfallen: das K ist wieder frei, die Wallet darf neu bestellen.
	spaet := z + quantenSLosFrist + 1
	w.annehmen(w.bestellen(a, qbK(10), 4, 3, spaet), spaet)
}

// Schlafend: vor dem Stichtag von Weg B weist jeder Knoten jede Weg-B-Art
// ab, und die Bloecke nehmen sie nicht auf.
func TestQuantenSWegB_Schlafend(t *testing.T) {
	w := neueQbWelt(t, 1)
	quantenSWegBAbOverride.Store(0)
	if quantenSWegBAb() != math.MaxInt64 {
		t.Fatal("Weg B ist nicht schlafend")
	}
	tx := w.melden(w.menschen[0], qbBestellZeit)
	if r := w.abgewiesen(tx, qbBestellZeit); r != "quanten_s_weg_b_inaktiv" {
		t.Fatalf("vor dem Stichtag: %q", r)
	}
	tx.BuchAt = qbBestellZeit
	if f := auftragsFenster(&tx, qbBestellZeit); f.von <= f.bis && f.bis >= qbBestellZeit && f.von <= qbBestellZeit {
		t.Fatalf("Block nimmt Weg B vor dem Stichtag auf: %+v", f)
	}
	if _, ok := quantenWStatus()["weg_b_ab"]; ok {
		t.Fatal("Status nennt einen Stichtag")
	}
}

// Leeres Weg-B-Register aendert die Bytes des W-Registers nicht (StateRoot
// unveraendert fuer jeden Knoten, solange Weg B schlaeft).
func TestQuantenSWegB_LeerUnveraendert(t *testing.T) {
	r := neuesQuantenWRegister()
	if r.kodiert() != "" {
		t.Fatal("leeres Register nicht leer kodiert")
	}
	r.Tote[strings.Repeat("a", 64)] = 5
	if strings.Contains(r.kodiert(), "praep") || strings.Contains(r.kodiert(), "bestellt") {
		t.Fatalf("Weg-B-Felder im Register ohne Weg B: %s", r.kodiert())
	}
}

// Gelesenes Register: fail-closed bei zwei aktiven Eintraegen desselben
// Menschen, Bestellung zu einem vergebenen K, Los ohne Runde, falscher Runde.
func TestQuantenSWegB_RegisterLesenStreng(t *testing.T) {
	a, b := "0x"+strings.Repeat("1", 40), "0x"+strings.Repeat("2", 40)
	K := strings.Repeat("c", 64)
	los := []string{a, b, "0x" + strings.Repeat("3", 40), "0x" + strings.Repeat("4", 40)}
	for name, f := range map[string]quantenWRegisterForm{
		"Mensch zweimal aktiv":   {Praep: []quantenSPraepForm{{Konto: a, Nullifier: "n1", Gemeldet: 5}, {Konto: b, Nullifier: "n1", Gemeldet: 6}}},
		"Abmeldung vor Meldung":  {Praep: []quantenSPraepForm{{Konto: a, Nullifier: "n1", Gemeldet: 5, Abgemeldet: 4}}},
		"Ordnung":                {Praep: []quantenSPraepForm{{Konto: b, Nullifier: "n1", Gemeldet: 5}, {Konto: a, Nullifier: "n2", Gemeldet: 6}}},
		"Bestellung zu Totem":    {Tote: []quantenWTotForm{{K: K, Zeit: 5}}, Bestellt: []quantenSBestellForm{{K: K, Von: a, V: 4, T: 3, Zeit: 5}}},
		"Los ohne Runde":         {Bestellt: []quantenSBestellForm{{K: K, Von: a, V: 4, T: 3, Zeit: 5, Los: los}}},
		"Runde nicht die feste":  {Bestellt: []quantenSBestellForm{{K: K, Von: a, V: 4, T: 3, Zeit: qbBestellZeit, Runde: drandEchtRunde + 1, Los: los}}},
		"Los mit doppeltem":      {Bestellt: []quantenSBestellForm{{K: K, Von: a, V: 4, T: 3, Zeit: qbBestellZeit, Runde: drandEchtRunde, Los: []string{a, a, b, b}}}},
		"v ausserhalb":           {Bestellt: []quantenSBestellForm{{K: K, Von: a, V: 2, T: 2, Zeit: 5}}},
		"zwei Bestellungen je W": {Bestellt: []quantenSBestellForm{{K: K, Von: a, V: 4, T: 3, Zeit: 5}, {K: strings.Repeat("d", 64), Von: a, V: 4, T: 3, Zeit: 5}}},
	} {
		if _, err := quantenWRegisterLesen(f.kodiert()); err == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
	gut := quantenWRegisterForm{Praep: []quantenSPraepForm{{Konto: a, Nullifier: "n1", Gemeldet: 5}},
		Bestellt: []quantenSBestellForm{{K: K, Von: a, V: 4, T: 3, Zeit: qbBestellZeit, Runde: drandEchtRunde, Los: los}}}
	r, err := quantenWRegisterLesen(gut.kodiert())
	if err != nil || r.kodiert() != gut.kodiert() {
		t.Fatalf("gutes Register: %v", err)
	}
}

// Abbild zur Bestellung, an den Grenzen: gemeldet genau Vorlauf vorher (drin),
// eine Sekunde spaeter (draussen); abgemeldet genau zur Bestellung (draussen),
// eine Sekunde danach (drin).
func TestQuantenSWegB_AbbildGrenzen(t *testing.T) {
	b := int64(1_800_000_000)
	praep := map[string]quantenSPraep{
		"0xa1": {Nullifier: "a", Gemeldet: b - quantenSMeldeVorlauf},
		"0xa2": {Nullifier: "b", Gemeldet: b - quantenSMeldeVorlauf + 1},
		"0xa3": {Nullifier: "c", Gemeldet: 1, Abgemeldet: b},
		"0xa4": {Nullifier: "d", Gemeldet: 1, Abgemeldet: b + 1},
		"0xa5": {Nullifier: "e", Gemeldet: 1},
	}
	if a := quantenSAbbild(praep, b); !reflect.DeepEqual(a, []string{"0xa1", "0xa4", "0xa5"}) {
		t.Fatalf("Abbild %v", a)
	}
}

// ------------------------------------------------------------ B2b

// qbsWelt: eine S-Welt (echte Praeparation, Inhaberin, Annahmestellen) mit
// Praeparatoren-Register; gelost wird mit einem Test-drand-Netz.
type qbsWelt struct {
	*qsWelt
	praep    []string
	los      []string
	signiere func(uint64) string
}

func neueQbsWelt(t *testing.T, gelost bool) *qbsWelt {
	t.Helper()
	w := &qbsWelt{qsWelt: neueQsWelt(t, 128, 80)}
	quantenSWegBAbOverride.Store(1)
	netz, sig := drandTestNetz(t, 777)
	alt := quantenSDrand
	quantenSDrand, w.signiere = netz, sig
	t.Cleanup(func() { quantenSWegBAbOverride.Store(0); quantenSDrand = alt })
	w.cs.nullifiers, w.nach.nullifiers = map[string]string{}, map[string]string{}
	melde := qwT0 - 86400
	for i := 0; i < 16; i++ {
		k, a := neuerSchluessel(t)
		w.keys[a] = k
		addHuman(w.cs, a, 1)
		addHuman(w.nach, a, 1)
		n := fmt.Sprintf("praep-%02d", i)
		w.cs.nullifiers[n], w.nach.nullifiers[n] = a, a
		w.praep = append(w.praep, a)
		w.annehmen(Transaction{Type: quantenSMeldenTxArt, Wallet: a, DistributionAt: melde,
			QuantenW: &QuantenWAngaben{WalletSig: personalSign(t, k, quantenSMeldenNachricht(a, melde))}}, melde)
	}
	v := len(w.val)
	w.annehmen(Transaction{Type: quantenSBestellenTxArt, Wallet: w.praeger, DistributionAt: qwT0, QuantenW: &QuantenWAngaben{K: w.p.K, V: v, T: w.p.t,
		WalletSig: personalSign(t, w.keys[w.praeger], quantenSBestellenNachricht(w.praeger, w.p.K, v, w.p.t, qwT0))}}, qwT0)
	if gelost {
		r := quantenSLosRunde(qwT0)
		w.annehmen(qbLosen(w.p.K, r, w.signiere(r)), qwT0+40)
		w.los = w.cs.quantenWJetzt().Bestellt[w.p.K].Los
		w.setzeValidatoren(w.los)
	}
	return w
}

// setzeValidatoren: die Ausgabe nennt diese Konten (mit ihren Schluesseln).
func (w *qbsWelt) setzeValidatoren(konten []string) {
	w.val, w.valKeys = slices.Clone(konten), nil
	for _, a := range konten {
		w.valKeys = append(w.valKeys, w.keys[a])
	}
}

// Gutfall: Ausgabe genau gegen das Los, dann binden, waehlen, anmelden,
// offenlegen -- der Token ist echt eingeloest; die Bestellung ist erledigt.
func TestQuantenSWegB_AusgabeGegenDasLos(t *testing.T) {
	w := neueQbsWelt(t, true)
	z := qwT0 + 60
	w.annehmen(w.ausgebenTx(z, 10_000_000), z+1)
	reg := w.cs.quantenWJetzt()
	if reg.Token[w.p.K] == nil || reg.Token[w.p.K].token.NVal != len(w.los) {
		t.Fatal("Token nicht ausgegeben")
	}
	if _, da := reg.Bestellt[w.p.K]; da {
		t.Fatal("Bestellung nach der Ausgabe noch offen")
	}
	w.annehmen(w.bindenTx(w.anna), z+2)
	w.annehmen(w.waehlenTx(w.anna, "1"), z+3)
	w.annehmen(w.anmeldenTx(w.stelle[0], 0, w.probe[0], w.vorlage(0, w.keys[w.anna])), z+10)
	if g := w.offenlegenAlle(0, z+20); g.Urteil != "echt" || g.WertAn != w.stelle[0] {
		t.Fatalf("Einloesen: %+v", g)
	}
}

// Missbrauch der Ausgabe zu einem bestellten K.
func TestQuantenSWegB_AusgabeMissbrauch(t *testing.T) {
	z := qwT0 + 60
	// Vor dem Los: abgewiesen.
	vor := neueQbsWelt(t, false)
	vor.setzeValidatoren(vor.praep[:len(vor.val)])
	if u := vor.bescheid(vor.cs, vor.ausgebenTx(z, 10_000_000), z+1); u.regel != "quanten_s_bestellung" {
		t.Fatalf("vor dem Los: %q", u.regel)
	}
	w := neueQbsWelt(t, true)
	fremd := ""
	for _, a := range w.praep {
		if !slices.Contains(w.los, a) {
			fremd = a
			break
		}
	}
	vertauscht := slices.Clone(w.los)
	vertauscht[0], vertauscht[1] = vertauscht[1], vertauscht[0]
	ersetzt := slices.Clone(w.los)
	ersetzt[2] = fremd
	for name, konten := range map[string][]string{"Reihenfolge": vertauscht, "fremder Praeparator": ersetzt, "einer weniger": w.los[:len(w.los)-1]} {
		w.setzeValidatoren(konten)
		if u := w.bescheid(w.cs, w.ausgebenTx(z, 10_000_000), z+1); u.regel != "quanten_s_los" {
			t.Fatalf("%s: %q", name, u.regel)
		}
	}
	w.setzeValidatoren(w.los)
	// Nicht der Besteller.
	andere := w.ausgebenTx(z, 10_000_000)
	andere.Wallet = w.anna
	if u := w.bescheid(w.cs, andere, z+1); u.regel != "quanten_s_los" {
		t.Fatalf("nicht der Besteller: %q", u.regel)
	}
	// Anderes t als bestellt.
	tAnders := w.ausgebenTx(z, 10_000_000)
	tAnders.QuantenW.T = w.p.t - 1
	if u := w.bescheid(w.cs, tAnders, z+1); u.regel != "quanten_s_los" {
		t.Fatalf("anderes t: %q", u.regel)
	}
	// Ein Geloster hat nicht bestaetigt (fremde Unterschrift an seiner Stelle).
	ohne := w.ausgebenTx(z, 10_000_000)
	ohne.QuantenW.Bestaetigungen[1] = ohne.QuantenW.Bestaetigungen[0]
	if u := w.bescheid(w.cs, ohne, z+1); u.regel != "quanten_w_bestaetigung" {
		t.Fatalf("Bestaetigung fehlt: %q", u.regel)
	}
	// Ohne Bestellung gilt weiter Weg A: dieselben Konten sind keine
	// gebundenen Validatoren.
	ohneBest := neueQsWelt(t, 128, 80)
	ohneBest.val, ohneBest.valKeys = w.val, w.valKeys
	for i, a := range w.val {
		ohneBest.keys[a] = w.valKeys[i]
	}
	if u := ohneBest.bescheid(ohneBest.cs, ohneBest.ausgebenTx(z, 10_000_000), z+1); u.regel != "quanten_w_validator" {
		t.Fatalf("Weg A ohne Bindung: %q", u.regel)
	}
}
