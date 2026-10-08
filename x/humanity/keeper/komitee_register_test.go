package keeper

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"
)

// komiteeStand: ein Stand mit n Schluesseln, jeder von einem eigenen
// Menschen gebunden, gueltig in [von, bis).
func komiteeStand(n int, von, bis int64) *erzeugerStand {
	st := &erzeugerStand{fenster: map[string][]zeitfenster{}, halter: map[string]string{}}
	for i := 0; i < n; i++ {
		addr := fmt.Sprintf("0x%040x", 0x1000+i)
		st.fenster[addr] = []zeitfenster{{betreiber: fmt.Sprintf("0xb%039x", i), von: von, bis: bis}}
	}
	return st
}

func komiteeDAG(st *erzeugerStand, self string) *BlockDAG {
	dag := newGhostdagTestDAG()
	dag.state = newTestState()
	if st != nil {
		dag.state.erzeugerRegister.Store(st)
	}
	dag.selfProposer = self
	return dag
}

func erzeugerSchnittAb1(t *testing.T) {
	t.Helper()
	erzeugerSchnittOverride.Store(1)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
}

// erwartetesKomitee: unabhaengig nachgerechnet -- die Zugelassenen zu t,
// nach sha256(addr:epoche) geordnet, die ersten targetCommitteeSize.
func erwartetesKomitee(st *erzeugerStand, t int64) map[string]bool {
	var zu []string
	for addr := range st.fenster {
		if st.erzeugerFenster(addr, t) != "" {
			zu = append(zu, addr)
		}
	}
	e := t / epochLength
	sort.Slice(zu, func(i, j int) bool {
		a, b := komiteePunkte(zu[i], e), komiteePunkte(zu[j], e)
		return bytes.Compare(a[:], b[:]) < 0
	})
	if len(zu) > targetCommitteeSize {
		zu = zu[:targetCommitteeSize]
	}
	out := map[string]bool{}
	for _, a := range zu {
		out[a] = true
	}
	return out
}

func gleicheMenge(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// Missbrauch: ab erzeugerSchnittAb waehlt jeder Knoten dasselbe Komitee aus
// dem Register -- auch wenn einer eine lokale Liste mit 500 erfundenen
// Adressen kennt (Abgleich unter Peers, Registrierungen bei ihm), seine
// Spitze auf einer anderen Hoehe steht und er den Stand selbst gelesen hat.
// Vorher drueckte so eine Liste echte Validatoren aus dem Komitee dieses
// Knotens.
func TestKomiteeAusRegister_GleichAufJedemKnoten(t *testing.T) {
	jetzt := int64(1_900_000_000)
	gefuellt := komiteeDAG(komiteeStand(200, 0, jetzt+86400), "")
	gefuellt.authorizedValidators = map[string]bool{}
	for i := 0; i < 500; i++ {
		gefuellt.authorizedValidators[fmt.Sprintf("0x%040x", 0x900000+i)] = true
	}
	leer := komiteeDAG(komiteeStand(200, 0, jetzt+86400), "")

	// Vor dem Stichtag: wie bisher aus der lokalen Liste -- die erfundenen
	// Adressen besetzen das Komitee.
	vorher := gefuellt.erzeugerKomitee(5, jetzt)
	if vorher == nil {
		t.Fatal("vor dem Stichtag kein Komitee")
	}
	erfunden := 0
	for a := range vorher.Members {
		if gefuellt.authorizedValidators[a] {
			erfunden++
		}
	}
	if erfunden != targetCommitteeSize {
		t.Fatalf("vor dem Stichtag: %d von %d aus der lokalen Liste -- der Test beweist nichts", erfunden, vorher.Size)
	}

	erzeugerSchnittAb1(t)
	a := gefuellt.erzeugerKomitee(5, jetzt)
	b := leer.erzeugerKomitee(987_654, jetzt)
	if a.Size != targetCommitteeSize || !gleicheMenge(a.Members, b.Members) {
		t.Fatalf("verschiedene Komitees: %d / %d Mitglieder", a.Size, b.Size)
	}
	if a.Number != jetzt/epochLength || b.Number != a.Number {
		t.Fatalf("Epoche nach der Zeit erwartet: %d / %d, Zeit %d", a.Number, b.Number, jetzt/epochLength)
	}
	if soll := erwartetesKomitee(gefuellt.state.erzeugerRegister.Load(), jetzt); !gleicheMenge(a.Members, soll) {
		t.Fatal("Komitee weicht von der Rangfolge ab")
	}
}

// Im Komitee ist, wer ZUR ZEIT in genau einem Erzeugerfenster steht:
// frisch gebunden sofort, abgelaufen nicht mehr, umstritten nie.
func TestKomiteeAusRegister_FensterZurZeit(t *testing.T) {
	erzeugerSchnittAb1(t)
	beginn := int64(1_900_000_800) / epochLength * epochLength
	jetzt := beginn + 1800
	st := &erzeugerStand{fenster: map[string][]zeitfenster{
		"0xaa": {{betreiber: "0xm1", von: 0, bis: beginn + 86400}},
		// mitten in der Epoche gebunden (Frist abgelaufen): sofort dabei
		"0xbb": {{betreiber: "0xm2", von: beginn + 60, bis: beginn + 86400}},
		// vor Beginn der Epoche abgelaufen
		"0xcc": {{betreiber: "0xm3", von: 0, bis: beginn}},
		// umstritten: zwei Fenster zugleich
		"0xdd": {{betreiber: "0xm4", von: 0, bis: beginn + 86400}, {betreiber: "0xm5", von: 0, bis: beginn + 86400}},
		// mitten in der Epoche abgelaufen
		"0xee": {{betreiber: "0xm6", von: 0, bis: beginn + 60}},
		// erst spaeter in der Epoche
		"0xff": {{betreiber: "0xm7", von: beginn + 2400, bis: beginn + 86400}},
	}}
	dag := komiteeDAG(st, "0xaa")
	ec := dag.erzeugerKomitee(1, jetzt)
	if soll := map[string]bool{"0xaa": true, "0xbb": true}; !gleicheMenge(ec.Members, soll) {
		t.Fatalf("Komitee %v, erwartet %v", ec.Members, soll)
	}
	spaeter := dag.erzeugerKomitee(1, beginn+2500)
	if soll := map[string]bool{"0xaa": true, "0xbb": true, "0xff": true}; !gleicheMenge(spaeter.Members, soll) {
		t.Fatalf("spaeter in der Epoche: %v, erwartet %v", spaeter.Members, soll)
	}
}

// Missbrauch (Liveness, Sicherheitsdurchgang zum Komitee, MEDIUM-1): ein
// Netz mit EINEM Validator wechselt mitten in der Epoche den Schluessel. Zu
// jeder Zeit darf genau einer der beiden erzeugen -- laut Register UND
// Komitee. Mit Kandidaten nur zum Epochenbeginn stand die Kette bis zum
// Epochenende.
func TestKomiteeAusRegister_SchluesselwechselNahtlos(t *testing.T) {
	erzeugerSchnittAb1(t)
	beginn := int64(1_900_000_800) / epochLength * epochLength
	wechsel := beginn + 600
	alt, neu := "0x00000000000000000000000000000000000000a1", "0x00000000000000000000000000000000000000a2"
	st := &erzeugerStand{fenster: map[string][]zeitfenster{
		alt: {{betreiber: "0xm1", von: 0, bis: wechsel}},
		neu: {{betreiber: "0xm1", von: wechsel, bis: 1 << 62}},
	}}
	knoten := map[string]*BlockDAG{}
	for _, self := range []string{alt, neu} {
		dag := komiteeDAG(st, self)
		dag.produzentenFest = map[string]bool{alt: true, neu: true}
		knoten[self] = dag
	}
	for _, dt := range []int64{1, 300, 599, 600, 601, 900, 3599, 3600, 3605} {
		jetzt := beginn + dt
		erzeugen := 0
		for self, dag := range knoten {
			if dag.erzeugerNachRegister(self, jetzt) && dag.erzeugerKomitee(1, jetzt).Members[self] {
				erzeugen++
			}
		}
		if erzeugen != 1 {
			t.Fatalf("Epochenbeginn+%d: %d Schluessel duerfen erzeugen, erwartet genau einer", dt, erzeugen)
		}
	}
}

// Mehr Zugelassene als Sitze: genau targetCommitteeSize, nach der Rangfolge
// der Epoche; in der naechsten Epoche andere (die Rangfolge rotiert); endet
// ein Fenster in der Epoche, rueckt der Naechste nach.
func TestKomiteeAusRegister_GrenzeUndRotation(t *testing.T) {
	erzeugerSchnittAb1(t)
	beginn := int64(1_900_000_800) / epochLength * epochLength
	st := komiteeStand(300, 0, 1<<62)
	dag := komiteeDAG(st, "")
	ec := dag.erzeugerKomitee(1, beginn+10)
	if ec.Size != targetCommitteeSize || !gleicheMenge(ec.Members, erwartetesKomitee(st, beginn+10)) {
		t.Fatalf("Komitee mit %d Mitgliedern weicht von der Rangfolge ab", ec.Size)
	}
	naechste := dag.erzeugerKomitee(1, beginn+epochLength+10)
	if naechste.Size != targetCommitteeSize || gleicheMenge(ec.Members, naechste.Members) ||
		!gleicheMenge(naechste.Members, erwartetesKomitee(st, beginn+epochLength+10)) {
		t.Fatal("die naechste Epoche hat dasselbe Komitee -- die Rangfolge rotiert nicht")
	}
	// Ein Mitglied verliert sein Fenster mitten in der Epoche (neuer Stand):
	// der Erste ausserhalb rueckt nach.
	var raus string
	for a := range ec.Members {
		raus = a
		break
	}
	neu := &erzeugerStand{fenster: map[string][]zeitfenster{}}
	for a, f := range st.fenster {
		neu.fenster[a] = f
	}
	neu.fenster[raus] = []zeitfenster{{betreiber: "0xweg", von: 0, bis: beginn + 100}}
	dag.state.erzeugerRegister.Store(neu)
	danach := dag.erzeugerKomitee(1, beginn+200)
	if danach.Size != targetCommitteeSize || danach.Members[raus] || !gleicheMenge(danach.Members, erwartetesKomitee(neu, beginn+200)) {
		t.Fatalf("nach dem Ende eines Fensters: %d Mitglieder, %s noch dabei: %v", danach.Size, raus, danach.Members[raus])
	}
}

// Fail-closed: ohne Stand oder nach einem Lesefehler ein LEERES Komitee --
// auch wenn der Stand erst zwischen der Pruefung des eigenen Schluessels
// und dem Komitee auf einen Lesefehler wechselt (LOW-1 der Pruefung).
func TestKomiteeAusRegister_OhneStandLeer(t *testing.T) {
	erzeugerSchnittAb1(t)
	jetzt := int64(1_900_000_000)
	self := "0x0000000000000000000000000000000000001000"
	ohne := komiteeDAG(nil, self)
	if ec := ohne.erzeugerKomitee(1, jetzt); ec == nil || len(ec.Members) != 0 {
		t.Fatalf("ohne Stand: %+v", ec)
	}
	if ohne.erzeugerNachRegister(self, jetzt) {
		t.Fatal("ohne Stand darf dieser Knoten erzeugen")
	}
	st := komiteeStand(3, 0, jetzt+86400)
	gut := komiteeDAG(st, self)
	if !gut.erzeugerNachRegister(self, jetzt) || !gut.erzeugerKomitee(1, jetzt).Members[self] {
		t.Fatal("mit Stand: dieser Knoten erzeugt nicht -- der Test beweist nichts")
	}
	// Der Stand wechselt nach der Pruefung des eigenen Schluessels.
	gut.state.erzeugerRegister.Store(&erzeugerStand{fenster: st.fenster, fehler: errors.New("db weg")})
	if ec := gut.erzeugerKomitee(1, jetzt); ec == nil || len(ec.Members) != 0 {
		t.Fatalf("nach einem Lesefehler: %+v", ec)
	}
	var keinZustand BlockDAG
	if ec := keinZustand.erzeugerKomitee(1, jetzt); ec == nil || len(ec.Members) != 0 {
		t.Fatalf("ohne Zustand: %+v", ec)
	}
}

// Die Rangliste: beim Auffrischen fuer diese und die naechste Epoche
// vorberechnet (ausserhalb von dag.mu) und dann benutzt; vor dem Stichtag
// nicht. Fehlt sie, rechnet das Komitee sie selbst, einmal je Stand.
func TestKomiteeAusRegister_Rangliste(t *testing.T) {
	jetzt := int64(1_900_000_000)
	e := jetzt / epochLength
	st := komiteeStand(150, 0, 1<<62)
	if komiteeVorberechnen(st, jetzt) != nil {
		t.Fatal("vor dem Stichtag vorberechnet")
	}
	erzeugerSchnittAb1(t)
	vor := komiteeVorberechnen(st, jetzt)
	if len(vor) != 2 || len(vor[e]) != 150 || len(vor[e+1]) != 150 {
		t.Fatalf("vorberechnet: %d Epochen", len(vor))
	}
	for _, ep := range []int64{e, e + 1} {
		selbst := komiteeRangBerechnen(st, ep)
		for i := range selbst {
			if selbst[i] != vor[ep][i] {
				t.Fatalf("Epoche %d, Platz %d: %s vorberechnet, %s selbst", ep, i, vor[ep][i], selbst[i])
			}
		}
	}
	// So wird der Stand beim Auffrischen gebaut: mit Rangliste.
	if gebaut := erzeugerStandBauen(st.fenster, nil, nil, time.Unix(jetzt, 0), jetzt); len(gebaut.komiteeRang[jetzt/epochLength]) != 150 {
		t.Fatal("der gebaute Stand traegt keine Rangliste")
	}
	if kaputt := erzeugerStandBauen(st.fenster, nil, errors.New("db weg"), time.Unix(jetzt, 0), jetzt); kaputt.komiteeRang != nil || kaputt.fehler == nil {
		t.Fatal("Rangliste fuer einen Stand mit Lesefehler")
	}
	// Die vorberechnete Liste wird benutzt: eine umgedrehte Liste ergibt das
	// umgedrehte Komitee.
	umgedreht := make([]string, len(vor[e]))
	for i, a := range vor[e] {
		umgedreht[len(umgedreht)-1-i] = a
	}
	mitListe := &erzeugerStand{fenster: st.fenster, komiteeRang: map[int64][]string{e: umgedreht}}
	dag := komiteeDAG(mitListe, "")
	ec := dag.erzeugerKomitee(1, jetzt)
	for _, a := range umgedreht[:targetCommitteeSize] {
		if !ec.Members[a] {
			t.Fatal("die vorberechnete Rangliste wurde nicht benutzt")
		}
	}
	// Ohne Liste: selbst gerechnet, und fuer denselben Stand gemerkt.
	ohneListe := komiteeDAG(st, "")
	ohneListe.erzeugerKomitee(1, jetzt)
	gemerkt := ohneListe.registerRang
	ohneListe.erzeugerKomitee(1, jetzt+1)
	if len(gemerkt) == 0 || &ohneListe.registerRang[0] != &gemerkt[0] {
		t.Fatal("dieselbe Rangliste fuer denselben Stand neu berechnet")
	}
	// Ein neuer Stand mit anderen Schluesseln: neu gerechnet.
	anders := &erzeugerStand{fenster: map[string][]zeitfenster{
		"0x00000000000000000000000000000000000f00d1": {{betreiber: "0xm1", von: 0, bis: 1 << 62}},
		"0x00000000000000000000000000000000000f00d2": {{betreiber: "0xm2", von: 0, bis: 1 << 62}},
	}}
	ohneListe.state.erzeugerRegister.Store(anders)
	if ec := ohneListe.erzeugerKomitee(1, jetzt+2); !gleicheMenge(ec.Members, map[string]bool{
		"0x00000000000000000000000000000000000f00d1": true, "0x00000000000000000000000000000000000f00d2": true}) {
		t.Fatalf("neuer Stand nicht uebernommen: %v", ec.Members)
	}
}

// Der Stichtag gilt fuer das Komitee wie fuer die Pruefung des eigenen
// Schluessels -- mit derselben Rueck-Toleranz (erzeugerSchnittAktiv).
func TestKomiteeAusRegister_StichtagWieErzeugerpruefung(t *testing.T) {
	jetzt := int64(1_900_000_000)
	erzeugerSchnittOverride.Store(jetzt + zeitstempelRueckToleranz)
	t.Cleanup(func() { erzeugerSchnittOverride.Store(0) })
	dag := komiteeDAG(komiteeStand(3, 0, 1<<62), "")
	dag.authorizedValidators = map[string]bool{"0x00000000000000000000000000000000000000f1": true}
	if !erzeugerSchnittAktiv(jetzt) {
		t.Fatal("Vorbedingung: der Stichtag gilt mit Toleranz")
	}
	if ec := dag.erzeugerKomitee(1, jetzt); ec.Members["0x00000000000000000000000000000000000000f1"] || ec.Size != 3 {
		t.Fatalf("Komitee nicht aus dem Register, obwohl die Erzeugerpruefung es schon ist: %+v", ec.Members)
	}
}

// Die Verdrahtung in ProduceBlock: ein Knoten, dessen Schluessel zugelassen
// ist, aber nicht im Komitee steht, erzeugt nicht (nicht_im_epochenkomitee);
// einer im Komitee kommt an dieser Pruefung vorbei; nach einem Lesefehler
// steigt er schon an der eigenen Pruefung aus.
func TestKomiteeAusRegister_ProduceBlock(t *testing.T) {
	erzeugerSchnittAb1(t)
	beginn := int64(1_900_000_800) / epochLength * epochLength
	jetzt := beginn + 1800
	uhr(t, jetzt)
	st := komiteeStand(150, 0, 1<<62)
	rang := komiteeRangBerechnen(st, jetzt/epochLength)
	grund := func(self string, stand *erzeugerStand) string {
		dag, _ := newDeterminismTestDAG()
		dag.state.erzeugerRegister.Store(stand)
		dag.selfProposer = self
		produktionLetzterGrnd.Store("")
		dag.ProduceBlock()
		g, _ := produktionLetzterGrnd.Load().(string)
		return g
	}
	if g := grund(rang[targetCommitteeSize+10], st); g != "nicht_im_epochenkomitee" {
		t.Fatalf("Schluessel ausserhalb des Komitees: Grund %q", g)
	}
	if g := grund(rang[0], st); g == "nicht_im_epochenkomitee" || g == "nicht_im_register" {
		t.Fatalf("Schluessel im Komitee: Grund %q", g)
	}
	if g := grund(rang[0], &erzeugerStand{fenster: st.fenster, fehler: errors.New("db weg")}); g != "nicht_im_register" {
		t.Fatalf("nach einem Lesefehler: Grund %q", g)
	}
}
