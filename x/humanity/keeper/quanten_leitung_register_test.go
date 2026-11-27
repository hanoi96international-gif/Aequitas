package keeper

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"math"
	"strings"
	"testing"
)

// Weg C, C2a: das Register der Quantenleitungen (quanten_leitung_register.go),
// ueber den echten Stimmweg der Freischaltung.

func qlAktiv(t *testing.T, ab int64) {
	t.Helper()
	vorher := quantenLeitungAbOverride.Load()
	quantenLeitungAbOverride.Store(ab)
	t.Cleanup(func() { quantenLeitungAbOverride.Store(vorher) })
}

// qlGamma: gamma_det der Test-Leitungen (20 %).
const qlGamma = 200_000

func qlStimme(f *vrFall, von int, name string, theta, frei, gamma int, ab, zeit int64) VerifierStimme {
	b := f.menschen[von]
	sig := ed25519.Sign(b.priv, []byte(quantenLeitungNachricht(name, theta, frei, gamma, ab)("", zeit)))
	return VerifierStimme{Zeit: zeit, Bescheinigung: Lebendigkeitsbescheinigung{PublicKey: b.pubHex,
		Signature: hex.EncodeToString(sig), Mensch: b.bindung.Mensch, MenschSig: b.bindung.MenschSig, SchluesselSig: b.bindung.SchluesselSig}}
}

func qlFreischaltung(f *vrFall, name string, theta, frei int, ab, zeit int64, von ...int) Transaction {
	return qlFreischaltungG(f, name, theta, frei, qlGamma, ab, zeit, von...)
}

func qlFreischaltungG(f *vrFall, name string, theta, frei, gamma int, ab, zeit int64, von ...int) Transaction {
	q := &QuantenAngaben{Satz: name, Theta: theta, Frei: frei, Gamma: gamma, Ab: ab}
	for _, i := range von {
		q.Stimmen = append(q.Stimmen, qlStimme(f, i, name, theta, frei, gamma, ab, zeit))
	}
	return Transaction{Type: quantenFreischaltungTxArt, Wallet: f.m(0), DistributionAt: zeit, Quanten: q}
}

func qlAnwenden(t *testing.T, cs *ChainState, tx Transaction, z int64) error {
	t.Helper()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.applyQuantenFreischaltungLocked(context.Background(), &tx, z)
}

// Gutfall: fuenf von sechs Pruefern tragen eine Leitung ein; ihre Werte gelten
// ab "ab"; sie steht in Kodierung, StateRoot und Stand -- aber nie als Satz.
func TestQuantenLeitung_Gutfall(t *testing.T) {
	f := neuerVrFall(t)
	zeit := qgPruefer(f, 6) + 1000
	qgAktiv(t)
	qlAktiv(t, zeit-10)
	ab := zeit + quantenFreischaltungVorlauf
	vorher := f.cs.quantenGeldJetzt().stateRootTeil()
	tx := qlFreischaltung(f, "leitung-bristol-1", 50, 50_000, ab, zeit, qgBis(5)...)
	if u := qgUrteil(t, f.cs, tx, zeit); u.regel != "" || u.leitung == nil {
		t.Fatalf("fuenf von sechs: %s %s", u.regel, u.text)
	}
	if err := qlAnwenden(t, f.cs, tx, zeit); err != nil {
		t.Fatal(err)
	}
	reg := f.cs.quantenGeldJetzt()
	if _, _, _, ok := reg.leitungZu("leitung-bristol-1", ab-1); ok {
		t.Fatal("Leitung vor ab gueltig")
	}
	if th, fr, ga, ok := reg.leitungZu("leitung-bristol-1", ab); !ok || th != 50 || fr != 50_000 || ga != qlGamma {
		t.Fatalf("Leitung ab ab: %d %d %d %v", th, fr, ga, ok)
	}
	if _, ok := reg.satz("leitung-bristol-1"); ok || len(reg.Saetze) != 0 {
		t.Fatal("Leitung steht unter den Saetzen")
	}
	if reg.stateRootTeil() == vorher || !strings.Contains(reg.kodiert(), `"leitungen"`) || reg.pruefen() != nil {
		t.Fatal("Leitung nicht in Kodierung/StateRoot, oder Register ungueltig")
	}
	// chain_config: nurSaetze traegt die Leitungen (sonst gingen sie beim Neustart verloren).
	wieder, err := quantenGeldRegisterLesen(reg.nurSaetze().kodiert())
	if err != nil || len(wieder.Leitungen) != 1 || wieder.Leitungen[0] != reg.Leitungen[0] {
		t.Fatalf("Leitung nach dem Speichern: %v %+v", err, wieder)
	}
	if st := f.cs.QuantenStand(""); len(st["leitungen"].([]map[string]interface{})) != 1 {
		t.Fatalf("Stand: %v", st["leitungen"])
	}
	if u := qgUrteil(t, f.cs, tx, zeit); u.regel != "quanten_satz_da" {
		t.Fatalf("Leitung doppelt: %+v", u)
	}
}

// Schlafend: die Leitungsform ist ungueltig wie bisher (keine Gruppenordnung),
// das Register bleibt leer und byte-gleich.
func TestQuantenLeitung_Schlafend(t *testing.T) {
	if quantenLeitungAbUnix != math.MaxInt64 {
		t.Skip("Stichtag gesetzt")
	}
	f := neuerVrFall(t)
	zeit := qgPruefer(f, 6) + 1000
	qgAktiv(t)
	ab := zeit + quantenFreischaltungVorlauf
	u := qgUrteil(t, f.cs, qlFreischaltung(f, "leitung-bristol-1", 50, 50_000, ab, zeit, qgBis(5)...), zeit)
	if u.regel != "quanten_ordnung" || u.leitung != nil {
		t.Fatalf("schlafend: %+v", u)
	}
	if f.cs.quantenGeldJetzt().kodiert() != "" {
		t.Fatal("schlafend: Register nicht leer")
	}
	// Ohne Leitungen ist die Kodierung genau die alte (kein Feld "leitungen").
	r := neuesQuantenRegister([]quantenSatzEintrag{{Name: "isogenie-8192", Ordnung: qgSatzN.String(), Ab: 5, Seit: 1}}, nil)
	if strings.Contains(r.kodiert(), "leitungen") || strings.Contains(r.nurSaetze().kodiert(), "leitungen") {
		t.Fatal("leeres Feld leitungen in der Kodierung")
	}
}

// Missbrauch: wer die Werte druecken will, braucht zwei Drittel der Pruefer
// und mindestens fuenf, fuer GENAU diese Werte.
func TestQuantenLeitung_Angriffe(t *testing.T) {
	f := neuerVrFall(t)
	zeit := qgPruefer(f, 6) + 1000
	qgAktiv(t)
	qlAktiv(t, zeit-10)
	ab := zeit + quantenFreischaltungVorlauf
	regel := func(tx Transaction) string { return qgUrteil(t, f.cs, tx, zeit).regel }

	if r := regel(qlFreischaltung(f, "leitung-a", 50, 50_000, ab, zeit, qgBis(4)...)); r != "quanten_quorum" {
		t.Fatalf("vier Stimmen: %q", r)
	}
	// Stimmen fuer andere Werte (gedrueckt): die Unterschriften passen nicht.
	gedrueckt := qlFreischaltung(f, "leitung-a", 50, 50_000, ab, zeit, qgBis(5)...)
	gedrueckt.Quanten.Theta, gedrueckt.Quanten.Frei = 0, 0
	if r := regel(gedrueckt); r != "quanten_stimme" {
		t.Fatalf("Werte nach der Abstimmung gedrueckt: %q", r)
	}
	// gamma_det nach der Abstimmung gesenkt (eine kleinere Meldung ginge
	// durch): die Unterschriften passen nicht.
	gesenkt := qlFreischaltung(f, "leitung-a", 50, 50_000, ab, zeit, qgBis(5)...)
	gesenkt.Quanten.Gamma = 1
	if r := regel(gesenkt); r != "quanten_stimme" {
		t.Fatalf("gamma nach der Abstimmung gesenkt: %q", r)
	}
	// Stimmen fuer einen SATZ gelten nicht fuer eine Leitung (eigene Domaene).
	satzStimmen := qlFreischaltung(f, "leitung-a", 0, 0, ab, zeit)
	for _, i := range qgBis(5) {
		satzStimmen.Quanten.Stimmen = append(satzStimmen.Quanten.Stimmen, qgStimme(f, i, "leitung-a", "", ab, zeit))
	}
	if r := regel(satzStimmen); r != "quanten_stimme" {
		t.Fatalf("Satz-Stimmen fuer eine Leitung: %q", r)
	}
	// Ausserhalb der Grenzen, mit Gruppenordnung, ohne Vorlauf.
	for name, tx := range map[string]Transaction{
		"theta 101":     qlFreischaltung(f, "leitung-a", 101, 0, ab, zeit, qgBis(5)...),
		"theta -1":      qlFreischaltung(f, "leitung-a", -1, 0, ab, zeit, qgBis(5)...),
		"frei zu gross": qlFreischaltung(f, "leitung-a", 0, quantenSFreiPpmMax+1, ab, zeit, qgBis(5)...),
		"frei -1":       qlFreischaltung(f, "leitung-a", 0, -1, ab, zeit, qgBis(5)...),
		"gamma 0":       qlFreischaltungG(f, "leitung-a", 0, 0, 0, ab, zeit, qgBis(5)...),
		"gamma > 10^6":  qlFreischaltungG(f, "leitung-a", 0, 0, 1_000_001, ab, zeit, qgBis(5)...),
		"gamma -1":      qlFreischaltungG(f, "leitung-a", 0, 0, -1, ab, zeit, qgBis(5)...),
	} {
		if r := regel(tx); r != "quanten_leitung" {
			t.Errorf("%s: %q", name, r)
		}
	}
	mitOrdnung := qlFreischaltung(f, "leitung-a", 50, 0, ab, zeit, qgBis(5)...)
	mitOrdnung.Quanten.Ordnung = qgSatzN.String()
	if r := regel(mitOrdnung); r != "quanten_leitung" {
		t.Fatalf("Leitung mit Gruppenordnung: %q", r)
	}
	if r := regel(qlFreischaltung(f, "leitung-a", 50, 0, zeit+10, zeit, qgBis(5)...)); r != "quanten_vorlauf" {
		t.Fatalf("ohne Vorlauf: %q", r)
	}
	// Ein Satz heisst nach dem Stichtag nie "leitung-...", und traegt keine Messwerte.
	if r := regel(qgFreischaltung(f, "leitung-x", qgSatzN.String(), ab, zeit, qgBis(5)...)); r != "quanten_leitung" {
		t.Fatalf("Satz namens leitung-: %q", r)
	}
	satzMitWerten := qgFreischaltung(f, "isogenie-8192", qgSatzN.String(), ab, zeit, qgBis(5)...)
	satzMitWerten.Quanten.Theta = 20
	if r := regel(satzMitWerten); r != "quanten_form" {
		t.Fatalf("Satz mit Messwerten: %q", r)
	}
	satzMitGamma := qgFreischaltung(f, "isogenie-8192", qgSatzN.String(), ab, zeit, qgBis(5)...)
	satzMitGamma.Quanten.Gamma = 1
	if r := regel(satzMitGamma); r != "quanten_form" {
		t.Fatalf("Satz mit gamma: %q", r)
	}
}

// Eine Note kann eine Leitung nie als Satz nennen; ein Register von aussen mit
// falscher Leitung wird abgewiesen.
func TestQuantenLeitung_RegisterStreng(t *testing.T) {
	ok := quantenLeitungEintrag{Name: "leitung-a", ThetaZehntel: 20, FreiPpm: 1000, GammaPpm: qlGamma, Ab: 10, Seit: 5}
	gut := &quantenGeldRegister{Leitungen: []quantenLeitungEintrag{ok}}
	if err := gut.pruefen(); err != nil {
		t.Fatalf("gueltige Leitung: %v", err)
	}
	if len(gut.saetzeZu(1<<40)) != 0 {
		t.Fatal("Leitung als Satz gueltig")
	}
	mitNote := &quantenGeldRegister{Leitungen: []quantenLeitungEintrag{ok},
		Noten: []quantenNoteEintrag{{K: strings.Repeat("a", 64), Satz: "leitung-a", Von: "0x" + strings.Repeat("1", 40), Anteile: 1}}}
	if mitNote.pruefen() == nil {
		t.Fatal("Note zu einer Leitung angenommen")
	}
	falsch := func(m func(*quantenLeitungEintrag)) *quantenGeldRegister {
		l := ok
		m(&l)
		return &quantenGeldRegister{Leitungen: []quantenLeitungEintrag{l}}
	}
	for name, r := range map[string]*quantenGeldRegister{
		"ohne Praefix":   falsch(func(l *quantenLeitungEintrag) { l.Name = "bristol" }),
		"theta":          falsch(func(l *quantenLeitungEintrag) { l.ThetaZehntel = 101 }),
		"frei":           falsch(func(l *quantenLeitungEintrag) { l.FreiPpm = quantenSFreiPpmMax + 1 }),
		"gamma 0":        falsch(func(l *quantenLeitungEintrag) { l.GammaPpm = 0 }),
		"gamma zu gross": falsch(func(l *quantenLeitungEintrag) { l.GammaPpm = 1_000_001 }),
		"ab":             falsch(func(l *quantenLeitungEintrag) { l.Ab = 0 }),
		"seit":           falsch(func(l *quantenLeitungEintrag) { l.Seit = 0 }),
		"Reihenfolge":    {Leitungen: []quantenLeitungEintrag{{Name: "leitung-b", GammaPpm: 1, Ab: 1, Seit: 1}, {Name: "leitung-a", GammaPpm: 1, Ab: 1, Seit: 1}}},
		"doppelt":        {Leitungen: []quantenLeitungEintrag{{Name: "leitung-a", GammaPpm: 1, Ab: 1, Seit: 1}, {Name: "leitung-a", GammaPpm: 1, Ab: 1, Seit: 1}}},
		"wie ein Satz":   {Saetze: []quantenSatzEintrag{{Name: "leitung-a", Ordnung: qgSatzN.String(), Ab: 1, Seit: 1}}, Leitungen: []quantenLeitungEintrag{ok}},
	} {
		if r.pruefen() == nil {
			t.Errorf("%s: angenommen", name)
		}
	}
	viele := &quantenGeldRegister{}
	for i := 0; i <= quantenLeitungenHoechstens; i++ {
		viele.Leitungen = append(viele.Leitungen, quantenLeitungEintrag{Name: "leitung-" + string(rune('a'+i)), GammaPpm: 1, Ab: 1, Seit: 1})
	}
	if viele.pruefen() == nil {
		t.Fatal("zu viele Leitungen angenommen")
	}
}

// Vor dem Stichtag werden theta/frei ueberlesen wie von aelteren Knoten;
// danach gehoeren sie nur an eine Leitung (Anmeldung, Einloesung, Satz).
func TestQuantenLeitung_FelderNurAnDerLeitung(t *testing.T) {
	f := neuerVrFall(t)
	zeit := qgPruefer(f, 6) + 1000
	qgAktiv(t)
	ab := zeit + quantenFreischaltungVorlauf
	satz := qgFreischaltung(f, "isogenie-8192", qgSatzN.String(), ab, zeit, qgBis(5)...)
	satz.Quanten.Theta = 20
	if quantenLeitungAbUnix == math.MaxInt64 {
		if u := qgUrteil(t, f.cs, satz, zeit); u.regel != "" {
			t.Fatalf("schlafend: theta an einem Satz nicht ueberlesen: %+v", u)
		}
	}
	qlAktiv(t, zeit-10)
	if u := qgUrteil(t, f.cs, satz, zeit); u.regel != "quanten_form" {
		t.Fatalf("aktiv: theta an einem Satz: %+v", u)
	}
}

// Die Hoechstzahl gilt auch im Urteil; Anmeldung und Einloesung tragen keine
// Messwerte; beim Start kommen die Leitungen aus chain_config zurueck.
func TestQuantenLeitung_GrenzenUndLaden(t *testing.T) {
	f := neuerVrFall(t)
	zeit := qgPruefer(f, 6) + 1000
	qgAktiv(t)
	qlAktiv(t, zeit-10)
	ab := zeit + quantenFreischaltungVorlauf
	voll := &quantenGeldRegister{}
	for i := 0; i < quantenLeitungenHoechstens; i++ {
		voll = voll.mitLeitung(quantenLeitungEintrag{Name: "leitung-" + string(rune('a'+i)), GammaPpm: 1, Ab: 1, Seit: 1})
	}
	f.cs.quantenGeld.Store(voll)
	if u := qgUrteil(t, f.cs, qlFreischaltung(f, "leitung-z", 20, 0, ab, zeit, qgBis(5)...), zeit); u.regel != "quanten_voll" {
		t.Fatalf("eine Leitung zu viel: %+v", u)
	}
	// Laden: Saetze und Leitungen aus chain_config, Noten aus der Tabelle.
	r, err := quantenGeldAusTeilen(voll.nurSaetze().kodiert(), nil)
	if err != nil || len(r.Leitungen) != quantenLeitungenHoechstens {
		t.Fatalf("Leitungen nach dem Laden: %v %d", err, len(r.Leitungen))
	}
	if _, err := quantenGeldAusTeilen(`{"leitungen":[{"name":"bristol","theta":0,"frei":0,"ab":1,"seit":1}]}`, nil); err == nil {
		t.Fatal("falsche Leitung aus chain_config geladen")
	}

	// Anmeldung mit Messwerten: ab dem Stichtag abgewiesen.
	p := neuerQgPraegeFall(t)
	tx := p.tx("isogenie-8192", "04d2", "0x"+strings.Repeat("1", 64), 300)
	tx.Quanten.Theta = 20
	z := qgAb + 200
	quantenLeitungAbOverride.Store(z + 1000)
	if u := p.urteil(tx, z); u.regel != "" {
		t.Fatalf("vor dem Stichtag: theta an der Anmeldung nicht ueberlesen: %+v", u)
	}
	quantenLeitungAbOverride.Store(z - 10)
	if u := p.urteil(tx, z); u.regel != "quanten_form" {
		t.Fatalf("Anmeldung mit theta: %+v", u)
	}
	tx.Quanten.Theta, tx.Quanten.Gamma = 0, 1
	if u := p.urteil(tx, z); u.regel != "quanten_form" {
		t.Fatalf("Anmeldung mit gamma: %+v", u)
	}

	// Einloesung mit Messwerten: ab dem Stichtag abgewiesen (Form vor allem anderen).
	e := neuerQgEinloeseFall(t)
	empf := "0x" + strings.Repeat("e", 40)
	etx := e.tx(empf, e.einmal(empf))
	ohne := func() string {
		e.cs.mu.Lock()
		defer e.cs.mu.Unlock()
		return e.cs.quantenEinloesenUrteilLocked(&etx, qgAb+200).regel
	}
	vorher := ohne()
	etx.Quanten.Frei = 5
	if r := ohne(); r != "quanten_form" || vorher == "quanten_form" {
		t.Fatalf("Einloesung mit frei: %q (ohne: %q)", r, vorher)
	}
	etx.Quanten.Frei, etx.Quanten.Gamma = 0, 1
	if r := ohne(); r != "quanten_form" {
		t.Fatalf("Einloesung mit gamma: %q", r)
	}
}
