package keeper

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	mrand "math/rand"
	"strings"
	"testing"
)

// Offline-Bargeld, Stufe 1a (offline_bargeld.go): Gutfall und je Angriff ein
// Test, mit echten P-256-Unterschriften wie aus dem Android Keystore.

const offT0 int64 = 1_900_000_000

func offAktiv(t *testing.T) {
	t.Helper()
	offlineAbOverride.Store(offT0 - 1)
	t.Cleanup(func() { offlineAbOverride.Store(0) })
}

type offGeraet struct {
	k   *ecdsa.PrivateKey
	hex string
}

func neuesOffGeraet(t *testing.T) offGeraet {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b := elliptic.Marshal(elliptic.P256(), k.X, k.Y) //nolint:staticcheck // unkomprimiert, wie Android
	return offGeraet{k: k, hex: hex.EncodeToString(b)}
}

func (g offGeraet) zahle(t *testing.T, zaehler int64, an string, mik int64) string {
	t.Helper()
	d := sha256.Sum256([]byte(offlineZahlungNachricht(g.hex, zaehler, an, mik)))
	r, s, err := ecdsa.Sign(rand.Reader, g.k, d[:])
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%064x%064x", r, s)
}

func offAdr(i int) string { return fmt.Sprintf("0x%040x", 0xa0000+i) }

func offRegel(err error) string {
	var a *offlineAbgelehnt
	if errors.As(err, &a) {
		return a.regel
	}
	return ""
}

func offMitPfand(t *testing.T, g offGeraet, mensch string, mik int64) *offlineRegister {
	t.Helper()
	r, err := offlinePfandAnlegen(&offlineRegister{}, mensch, true, g.hex, mik, offT0)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestOffline_SaetzeFest(t *testing.T) {
	if got := offlineZahlungNachricht("04AB", 7, "0xABC", 5); got != "aequitas-offline-zahlung-v1|chain:1926|04ab|7|0xabc|5" {
		t.Fatalf("Zahlungssatz geaendert: %q", got)
	}
	if got := offlineZertifikatNachricht("04AB", "0xM", 10, 2, 99); got != "aequitas-offline-zertifikat-v1|chain:1926|04ab|0xm|10|2|99" {
		t.Fatalf("Zertifikatssatz geaendert: %q", got)
	}
	if offlineTopfAddr == quantenTopfAddr || !kanonischeAdresse(offlineTopfAddr) {
		t.Fatalf("Offline-Topf %s", offlineTopfAddr)
	}
}

func TestOffline_SchlafendOhneStichtag(t *testing.T) {
	g := neuesOffGeraet(t)
	if _, err := offlinePfandAnlegen(&offlineRegister{}, offAdr(1), true, g.hex, 10_000_000, offT0); offRegel(err) != "offline_inaktiv" {
		t.Fatalf("ohne Stichtag angenommen: %v", err)
	}
}

func TestOffline_PfandAnlegen(t *testing.T) {
	offAktiv(t)
	g, m := neuesOffGeraet(t), offAdr(1)
	r := offMitPfand(t, g, m, 10_000_000)
	if r.Aktiv[m] != g.hex || r.Pfaender[g.hex].PfandMikro != 10_000_000 {
		t.Fatalf("Pfand nicht eingetragen: %+v", r)
	}
	for name, f := range map[string]func() error{
		"kein Mensch": func() error {
			_, e := offlinePfandAnlegen(&offlineRegister{}, m, false, g.hex, 10_000_000, offT0)
			return e
		},
		"Systemadresse": func() error {
			_, e := offlinePfandAnlegen(&offlineRegister{}, offlineTopfAddr, true, g.hex, 10_000_000, offT0)
			return e
		},
		"zu klein": func() error {
			_, e := offlinePfandAnlegen(&offlineRegister{}, m, true, g.hex, offlinePfandMinMik-1, offT0)
			return e
		},
		"zu gross": func() error {
			_, e := offlinePfandAnlegen(&offlineRegister{}, m, true, g.hex, offlinePfandMaxMik+1, offT0)
			return e
		},
		"Geraet gross": func() error {
			_, e := offlinePfandAnlegen(&offlineRegister{}, m, true, strings.ToUpper(g.hex), 10_000_000, offT0)
			return e
		},
		"Geraet komprimiert": func() error {
			_, e := offlinePfandAnlegen(&offlineRegister{}, m, true, "02"+g.hex[2:66]+strings.Repeat("0", 64), 10_000_000, offT0)
			return e
		},
		"nicht auf der Kurve": func() error {
			_, e := offlinePfandAnlegen(&offlineRegister{}, m, true, "04"+strings.Repeat("1", 128), 10_000_000, offT0)
			return e
		},
		"zweites Pfand": func() error {
			_, e := offlinePfandAnlegen(r, m, true, neuesOffGeraet(t).hex, 10_000_000, offT0)
			return e
		},
		"Geraet doppelt": func() error { _, e := offlinePfandAnlegen(r, offAdr(2), true, g.hex, 10_000_000, offT0); return e },
	} {
		if err := f(); err == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
}

func TestOffline_EinloesenGutfallUndWiederholung(t *testing.T) {
	offAktiv(t)
	g, m, e := neuesOffGeraet(t), offAdr(1), offAdr(2)
	r := offMitPfand(t, g, m, 10_000_000)
	sig := g.zahle(t, 1, e, 3_000_000)
	r2, x, err := offlineEinloesen(r, g.hex, 1, e, 3_000_000, sig, offT0+60)
	if err != nil || x.AusTopf != 3_000_000 || x.Rest != 0 || x.Betrug || x.Mensch != m {
		t.Fatalf("Gutfall: %+v %v", x, err)
	}
	if r2.Pfaender[g.hex].AusgezahltMikro != 3_000_000 || r.Pfaender[g.hex].AusgezahltMikro != 0 {
		t.Fatal("Register nicht kopiert oder nicht gebucht")
	}
	// Wiederholung derselben Zahlung: abgewiesen, nichts doppelt.
	if _, _, err := offlineEinloesen(r2, g.hex, 1, e, 3_000_000, sig, offT0+61); offRegel(err) != "offline_doppelt" {
		t.Fatalf("Wiederholung: %v", err)
	}
}

func TestOffline_FalscheUnterschriften(t *testing.T) {
	offAktiv(t)
	g, fremd, m, e := neuesOffGeraet(t), neuesOffGeraet(t), offAdr(1), offAdr(2)
	r := offMitPfand(t, g, m, 10_000_000)
	sig := g.zahle(t, 1, e, 3_000_000)
	for name, f := range map[string]func() error{
		"fremdes Geraet": func() error {
			_, _, x := offlineEinloesen(r, g.hex, 1, e, 3_000_000, fremd.zahle(t, 1, e, 3_000_000), offT0)
			return x
		},
		"Betrag erhoeht":     func() error { _, _, x := offlineEinloesen(r, g.hex, 1, e, 4_000_000, sig, offT0); return x },
		"anderer Empfaenger": func() error { _, _, x := offlineEinloesen(r, g.hex, 1, offAdr(3), 3_000_000, sig, offT0); return x },
		"anderer Zaehler":    func() error { _, _, x := offlineEinloesen(r, g.hex, 2, e, 3_000_000, sig, offT0); return x },
		"Unsinn": func() error {
			_, _, x := offlineEinloesen(r, g.hex, 1, e, 3_000_000, strings.Repeat("0", 128), offT0)
			return x
		},
		"kurz": func() error { _, _, x := offlineEinloesen(r, g.hex, 1, e, 3_000_000, sig[:126], offT0); return x },
	} {
		if err := f(); offRegel(err) != "offline_unterschrift" {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for name, f := range map[string]func() error{
		"Betrag 0": func() error { _, _, x := offlineEinloesen(r, g.hex, 1, e, 0, sig, offT0); return x },
		"Betrag zu gross": func() error {
			_, _, x := offlineEinloesen(r, g.hex, 1, e, offlineZahlungMaxMik+1, sig, offT0)
			return x
		},
		"Zaehler 0": func() error { _, _, x := offlineEinloesen(r, g.hex, 0, e, 3_000_000, sig, offT0); return x },
		"an sich selbst": func() error {
			_, _, x := offlineEinloesen(r, g.hex, 1, m, 3_000_000, g.zahle(t, 1, m, 3_000_000), offT0)
			return x
		},
		"an den Topf": func() error {
			_, _, x := offlineEinloesen(r, g.hex, 1, offlineTopfAddr, 3_000_000, g.zahle(t, 1, offlineTopfAddr, 3_000_000), offT0)
			return x
		},
		"unbekanntes Geraet": func() error {
			_, _, x := offlineEinloesen(r, fremd.hex, 1, e, 3_000_000, fremd.zahle(t, 1, e, 3_000_000), offT0)
			return x
		},
	} {
		if err := f(); err == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
}

// Der Beweis: zwei verschiedene Zahlungen mit demselben Zaehler.
func TestOffline_DoppelterZaehlerIstBetrug(t *testing.T) {
	offAktiv(t)
	g, m, a, b := neuesOffGeraet(t), offAdr(1), offAdr(2), offAdr(3)
	r := offMitPfand(t, g, m, 10_000_000)
	r, x, err := offlineEinloesen(r, g.hex, 5, a, 8_000_000, g.zahle(t, 5, a, 8_000_000), offT0)
	if err != nil || x.Betrug {
		t.Fatalf("erste Zahlung: %+v %v", x, err)
	}
	r, y, err := offlineEinloesen(r, g.hex, 5, b, 8_000_000, g.zahle(t, 5, b, 8_000_000), offT0+1)
	if err != nil {
		t.Fatal(err)
	}
	if !y.Betrug || y.AusTopf != 2_000_000 || y.Rest != 6_000_000 {
		t.Fatalf("Betrug nicht erkannt oder falsch gerechnet: %+v", y)
	}
	if !r.Gesperrt[m] || !r.Pfaender[g.hex].Betrug {
		t.Fatal("Betrueger nicht gesperrt")
	}
	// Gesperrt: kein neues Pfand, auch nicht mit neuem Geraet.
	if _, err := offlinePfandAnlegen(r, m, true, neuesOffGeraet(t).hex, 10_000_000, offT0+2); err == nil {
		t.Fatal("Betrueger bekommt ein neues Pfand")
	}
}

// Mehr ausgegeben, als das Pfand deckt (ohne doppelten Zaehler): auch Betrug.
func TestOffline_UeberziehenIstBetrug(t *testing.T) {
	offAktiv(t)
	g, m := neuesOffGeraet(t), offAdr(1)
	r := offMitPfand(t, g, m, 10_000_000)
	var x offlineEinloesung
	var err error
	for i := int64(1); i <= 3; i++ {
		e := offAdr(10 + int(i))
		r, x, err = offlineEinloesen(r, g.hex, i, e, 4_000_000, g.zahle(t, i, e, 4_000_000), offT0)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !x.Betrug || x.AusTopf != 2_000_000 || x.Rest != 2_000_000 || !r.Gesperrt[m] {
		t.Fatalf("Ueberziehen nicht erkannt: %+v", x)
	}
}

// Erhaltung: was der Topf auszahlt, ist nie mehr als das Pfand -- egal in
// welcher Reihenfolge, mit welchen Betraegen und Doppelungen eingeloest wird.
func TestOffline_TopfZahltNieMehrAlsDasPfand(t *testing.T) {
	offAktiv(t)
	rng := mrand.New(mrand.NewSource(7))
	for lauf := 0; lauf < 30; lauf++ {
		g, m := neuesOffGeraet(t), offAdr(1)
		pfand := offlinePfandMinMik + rng.Int63n(offlinePfandMaxMik-offlinePfandMinMik)
		r := offMitPfand(t, g, m, pfand)
		var ausTopf, rest, gesamt int64
		for i := 0; i < 25; i++ {
			n := 1 + rng.Int63n(10)
			e := offAdr(100 + rng.Intn(5))
			b := 1 + rng.Int63n(offlineZahlungMaxMik)
			r2, x, err := offlineEinloesen(r, g.hex, n, e, b, g.zahle(t, n, e, b), offT0)
			if err != nil {
				continue
			}
			r = r2
			ausTopf += x.AusTopf
			rest += x.Rest
			gesamt += b
		}
		if ausTopf > pfand || ausTopf+rest != gesamt || r.Pfaender[g.hex].AusgezahltMikro != ausTopf {
			t.Fatalf("Lauf %d: Pfand %d, aus dem Topf %d, Rest %d, eingeloest %d", lauf, pfand, ausTopf, rest, gesamt)
		}
	}
}

func TestOffline_PfandVollWirdAbgewiesen(t *testing.T) {
	offAktiv(t)
	g, m, e := neuesOffGeraet(t), offAdr(1), offAdr(2)
	r := offMitPfand(t, g, m, 10_000_000)
	z := map[int64]string{}
	for i := int64(1); i <= offlineZaehlerJePfand; i++ {
		z[i+100] = strings.Repeat("a", 64)
	}
	r.Pfaender[g.hex].Zaehler = z
	if _, _, err := offlineEinloesen(r, g.hex, 1, e, 1, g.zahle(t, 1, e, 1), offT0); offRegel(err) != "offline_voll" {
		t.Fatalf("volles Pfand: %v", err)
	}
}

func TestOffline_KuendigenUndAbschluss(t *testing.T) {
	offAktiv(t)
	g, m, e := neuesOffGeraet(t), offAdr(1), offAdr(2)
	r := offMitPfand(t, g, m, 10_000_000)
	if _, _, err := offlineAbschliessen(r, m, offT0+1); offRegel(err) != "offline_frist" {
		t.Fatalf("Abschluss ohne Kuendigung: %v", err)
	}
	r, err := offlineKuendigen(r, m, offT0+10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := offlineKuendigen(r, m, offT0+11); err == nil {
		t.Fatal("zweimal gekuendigt")
	}
	// Waehrend der Frist wird weiter eingeloest.
	r, _, err = offlineEinloesen(r, g.hex, 1, e, 3_000_000, g.zahle(t, 1, e, 3_000_000), offT0+20)
	if err != nil {
		t.Fatal(err)
	}
	ab := offlineAbschlussAb(offT0 + 10)
	if _, _, err := offlineAbschliessen(r, m, ab-1); offRegel(err) != "offline_frist" {
		t.Fatalf("Abschluss vor der Frist: %v", err)
	}
	r, aus, err := offlineAbschliessen(r, m, ab)
	if err != nil || len(aus) != 1 || aus[0].An != m || aus[0].Mikro != 7_000_000 {
		t.Fatalf("Abschluss: %+v %v", aus, err)
	}
	if _, da := r.Aktiv[m]; da || r.Pfaender[g.hex].verfuegbar() != 0 {
		t.Fatal("Pfand nach Abschluss noch aktiv")
	}
	// Spaete Einloesung nach Abschluss: kein Betrug, ganz Rest (Schuld).
	_, x, err := offlineEinloesen(r, g.hex, 2, e, 1_000_000, g.zahle(t, 2, e, 1_000_000), ab+1)
	if err != nil || x.Betrug || x.AusTopf != 0 || x.Rest != 1_000_000 {
		t.Fatalf("spaete Einloesung: %+v %v", x, err)
	}
	// Neues Pfand nach Abschluss erlaubt (neues Geraet).
	if _, err := offlinePfandAnlegen(r, m, true, neuesOffGeraet(t).hex, 5_000_000, ab+2); err != nil {
		t.Fatalf("neues Pfand nach Abschluss: %v", err)
	}
}

// Beim Abschluss zuerst die Glaeubiger, dann der Halter.
func TestOffline_AbschlussBegleichtSchuldenZuerst(t *testing.T) {
	offAktiv(t)
	g, m := neuesOffGeraet(t), offAdr(1)
	r := offMitPfand(t, g, m, 10_000_000)
	r, _ = offlineSchuldEintragen(r, m, offAdr(5), 4_000_000)
	r, _ = offlineSchuldEintragen(r, m, offAdr(6), 9_000_000)
	r, _ = offlineKuendigen(r, m, offT0)
	r, aus, err := offlineAbschliessen(r, m, offlineAbschlussAb(offT0))
	if err != nil {
		t.Fatal(err)
	}
	if len(aus) != 2 || aus[0] != (offlineAuszahlung{offAdr(5), 4_000_000}) || aus[1] != (offlineAuszahlung{offAdr(6), 6_000_000}) {
		t.Fatalf("Reihenfolge/Betraege: %+v", aus)
	}
	if len(r.Schulden[m]) != 1 || r.Schulden[m][0].Mikro != 3_000_000 {
		t.Fatalf("Restschuld: %+v", r.Schulden[m])
	}
}

// Die Liste der Forderungen ist begrenzt, und keine Forderung landet je bei
// einem anderen Glaeubiger: ist die Liste voll, gilt die Einloesung eines
// neuen Glaeubigers nicht (spaeter erneut), ein bekannter waechst weiter.
func TestOffline_SchuldenBegrenzt(t *testing.T) {
	r := &offlineRegister{}
	m := offAdr(1)
	for i := 0; i < offlineSchuldenJeMensch; i++ {
		var err error
		if r, err = offlineSchuldEintragen(r, m, offAdr(1000+i), 7); err != nil {
			t.Fatal(err)
		}
	}
	vorher := r.kodiert()
	if _, err := offlineSchuldEintragen(r, m, offAdr(5000), 9); offRegel(err) != "offline_voll" {
		t.Fatalf("neuer Glaeubiger bei voller Liste: %v", err)
	}
	if r.kodiert() != vorher {
		t.Fatal("abgewiesene Forderung hat das Register veraendert")
	}
	letzter := offAdr(1000 + offlineSchuldenJeMensch - 1)
	for _, s := range r.Schulden[m] {
		if s.Glaeubiger == letzter && s.Mikro != 7 {
			t.Fatalf("letzter Glaeubiger bekam fremde Forderung: %+v", s)
		}
	}
	r, err := offlineSchuldEintragen(r, m, offAdr(1000), 3) // gleicher Glaeubiger: zusammen
	if err != nil || len(r.Schulden[m]) != offlineSchuldenJeMensch || r.Schulden[m][0] != (offlineSchuld{offAdr(1000), 10}) {
		t.Fatalf("bekannter Glaeubiger: %v %+v", err, r.Schulden[m][0])
	}
}

func TestOffline_ArchivBegrenzt(t *testing.T) {
	offAktiv(t)
	m := offAdr(1)
	r := &offlineRegister{}
	zeit := offT0
	for i := 0; i < offlineArchivJeMensch+3; i++ {
		var err error
		r, err = offlinePfandAnlegen(r, m, true, neuesOffGeraet(t).hex, 2_000_000, zeit)
		if err != nil {
			t.Fatal(err)
		}
		r, _ = offlineKuendigen(r, m, zeit)
		zeit = offlineAbschlussAb(zeit)
		if r, _, err = offlineAbschliessen(r, m, zeit); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.Pfaender) != offlineArchivJeMensch {
		t.Fatalf("Archiv hat %d Pfaender, Grenze %d", len(r.Pfaender), offlineArchivJeMensch)
	}
}

// Ein Register von aussen (Snapshot) wird nicht geglaubt.
func TestOffline_RegisterStrengGelesen(t *testing.T) {
	offAktiv(t)
	g, m := neuesOffGeraet(t), offAdr(1)
	r := offMitPfand(t, g, m, 10_000_000)
	gut := r.kodiert()
	if n, err := offlineRegisterLesen(gut); err != nil || n.kodiert() != gut {
		t.Fatalf("eigenes Register nicht lesbar: %v", err)
	}
	for name, s := range map[string]string{
		"mehr ausgezahlt als Pfand": strings.Replace(gut, `"ausgezahlt":0`, `"ausgezahlt":99000000`, 1),
		"Pfand zu gross":            strings.Replace(gut, `"pfand":10000000`, `"pfand":99000000`, 1),
		"Geraet fremd":              strings.Replace(gut, `"geraet":"`+g.hex, `"geraet":"04`+strings.Repeat("1", 128), 1),
		"aktiv falsch":              strings.Replace(gut, `"aktiv":{"`+m, `"aktiv":{"`+offAdr(9), 1),
		"kein JSON":                 "{",
	} {
		if s == gut {
			t.Fatalf("%s: Ersetzung griff nicht", name)
		}
		if _, err := offlineRegisterLesen(s); err == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
	if (&offlineRegister{}).stateRootTeil() != "" || r.stateRootTeil() == "" {
		t.Fatal("StateRoot-Teil: leer muss leer bleiben, sonst gesetzt")
	}
}

// Doppelter Zaehler, obwohl beide Zahlungen zusammen ins Pfand passen: der
// Beweis allein reicht -- nicht erst das Ueberziehen.
func TestOffline_DoppelterZaehlerOhneUeberziehen(t *testing.T) {
	offAktiv(t)
	g, m, a, b := neuesOffGeraet(t), offAdr(1), offAdr(2), offAdr(3)
	r := offMitPfand(t, g, m, 10_000_000)
	r, _, err := offlineEinloesen(r, g.hex, 9, a, 3_000_000, g.zahle(t, 9, a, 3_000_000), offT0)
	if err != nil {
		t.Fatal(err)
	}
	r, y, err := offlineEinloesen(r, g.hex, 9, b, 3_000_000, g.zahle(t, 9, b, 3_000_000), offT0+1)
	if err != nil {
		t.Fatal(err)
	}
	if !y.Betrug || !r.Gesperrt[m] || y.AusTopf != 3_000_000 || y.Rest != 0 {
		t.Fatalf("doppelter Zaehler ohne Ueberziehen nicht als Betrug erkannt: %+v", y)
	}
}
