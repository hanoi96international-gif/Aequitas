package keeper

// Aequitas fuer Unternehmen (docs/UNTERNEHMEN_KONZEPT.md, Entscheidung 25.09.2026).
//
// DREI KONTOARTEN. Mensch (verifiziert), Unternehmen (von 1-10 Menschen
// eroeffnet, Register unten) und freie Adresse (alles andere ausser den
// Protokoll-Toepfen). Unternehmen haben keine 25.000-Grenze, zahlen aber
// Liegegeld auf Geld, das bei ihnen liegen bleibt. Freie Adressen duerfen
// hoechstens 1.000 AEQ halten und zahlen 1 %/Monat. Menschen zahlen auf
// Erspartes erst ueber 5.000 AEQ etwas.
//
// WAS KONSENS IST UND WAS BUCHFUEHRUNG. Konsens ist nur das Register der
// Unternehmen: es entscheidet, ob die Vermoegensgrenze greift
// (enforceWealthCapLockedCtx), und wird deshalb auf jedem Knoten aus den
// Transaktionen unternehmen_* gleich aufgebaut. Alles andere -- das Alter
// des Geldes, die Monatszaehler fuer die Freibetraege -- ist Buchfuehrung
// des erzeugenden Knotens. Die BETRAEGE, die daraus folgen (Gebuehr,
// Ausstiegsabgabe, Liegegeld), stehen in den Transaktionen, und jeder
// nachspielende Knoten wendet genau sie an. Dasselbe Modell wie die
// Demurrage (FromDemurrageLost): der Erzeuger rechnet, die Kette traegt das
// Ergebnis. Die Buchfuehrung wird auch beim Nachspielen mitgefuehrt, damit
// ein anderer Knoten, der die Erzeugung uebernimmt, dieselben Zahlen hat.
//
// FREIBETRAG NACH UMSATZ (Konzept Abschnitt 14, beschlossen 25.09.2026).
// Unternehmen halten bis zu 1,5 Monatsumsaetze frei (mindestens den Sockel),
// bis 3 Monatsumsaetze kostet der Teil darueber 0,5 %/Monat, alles darueber
// 2 %/Monat. Frueher trug jedes AEQ ein Alter und wurde in fester Reihenfolge
// ausgegeben -- gruendlich gegen Umgehung, aber fuer echte Unternehmen zu
// teuer (12-36 %/Jahr auf ganz normale Reserven) und fuer Buchhaltung und
// Kassen nicht abbildbar. Ein AEQ ist jetzt wieder wie das andere.
//
// Der Monatsumsatz ist der Durchschnitt der anrechenbaren Eingaenge der
// letzten 90 Tage. Damit niemand ihn aufblaeht, gelten drei Schutzregeln:
//   - Einkaeufe von Menschen zaehlen je Mensch hoechstens 9 x fairer Anteil
//     pro Unternehmen und Quartal (Zaehler beim Menschen: Gezaehlt). Pro
//     Quartal statt pro Monat, damit ein grosser Einkauf (Moebel, Reparatur)
//     voll zaehlt, ohne dass das Aufblaehen leichter wird.
//   - Zwischen Unternehmen zaehlt nur der Ueberschuss: alle Eingaenge von
//     Unternehmen minus alle Zahlungen an Unternehmen. Ein Dreieck, in dem
//     drei Firmen sich Geld im Kreis schicken, gewinnt so nichts. Firmen mit
//     gemeinsamen Verantwortlichen zaehlen fuereinander gar nicht.
//   - Zahlt ein Unternehmen einem Menschen Geld, hebt das dessen gezaehlte
//     Einkaeufe dort auf (rueckzahlungLocked): einkaufen und das Geld
//     zurueckbekommen bringt nichts.
//   - Loehne, Entnahmen, Zahlungen der eigenen Verantwortlichen, Eingaenge
//     von freien Adressen und der Einstieg aus tUSD zaehlen nicht.
//
// FEHLENDE BUCHFUEHRUNG GEHT ZUGUNSTEN DER KONTOINHABER AUS. Hat DIESER
// KNOTEN weniger als 30 Tage Daten (kurz nach der Aktivierung, oder frisch aus
// einem Snapshot: buchSeit), berechnet er kein Liegegeld -- sonst wuerde ein
// Knoten ohne Daten jedes Unternehmen als umsatzlos behandeln. Ein NEUES
// UNTERNEHMEN bekommt dagegen keine Schonfrist: sonst liesse sich Geld Monat
// fuer Monat in eine frisch eroeffnete Firma schieben und nie Liegegeld
// zahlen. Sein Umsatz wird ueber mindestens 30 Tage gemittelt, damit wenige
// Tage nicht hochgerechnet werden.
//
// DIE BUCHFUEHRUNG IST ABSTURZSICHER. Sie wird in derselben
// Datenbank-Transaktion geschrieben wie die Kontostaende (speichereBuchCtx)
// und bei einem Abbruch mit ihnen zurueckgenommen (blockRollbackSnapshot).
// Jeder Knoten, der dieselben Bloecke angewandt hat, hat damit dieselben
// Zahlen -- darauf baut die Pruefung beim Nachspielen (liegegeld_pruefung.go).
//
// AKTIVIERUNG. Nichts davon wirkt vor wirtschaftAktivAbUnix. Danach laufen
// alle Ueberweisungen ueber transferMutateLocked (die schnellen Pfade
// treten zurueck), damit die Regeln an genau einer Stelle stehen.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// 2026-10-01T00:00:00Z
	wirtschaftAktivAbUnix int64 = 1790812800

	// Alle Betraege sind Vielfache des fairen Anteils (registrationGrant),
	// nicht feste Summen und nicht an den Dollar gekoppelt. Die Geldmenge ist
	// Menschen x fairer Anteil, der Durchschnittsmensch haelt also immer genau
	// einen fairen Anteil. Eine Grenze von 5x bleibt "fuenfmal so viel wie der
	// Durchschnitt", egal ob AEQ steigt oder faellt. Eine Kopplung an den
	// Dollar braeuchte eine Kursquelle, die jemand verschieben koennte, und
	// wuerde die Grenzen bei steigendem Kurs still verschaerfen.
	// (Konzept Abschnitt 6.7; der Monatsfreibetrag soll nach der Pilotstadt
	// dem Median der echten Monatsausgaben folgen, nie unter 1x.)

	// Menschen (Fairness-Garantie, Konzept Abschnitt 3)
	menschFreiAusgabenMonat = 1 * registrationGrant // gebuehrenfreie Ausgaben je Monat
	menschTauschFreiMonat   = 3 * registrationGrant // Umtausch ohne Abgabe je Monat, egal woher
	menschSparFreibetrag    = 5 * registrationGrant // darunter keine Umlaufsicherung
	menschUmlaufMonat       = 0.005                 // 0,5 %/Monat auf den Teil darueber

	// Freie Adressen
	freiGrenze      = 1 * registrationGrant
	freiUmlaufMonat = 0.01

	// Unternehmen
	unternehmenSockel        = 2 * registrationGrant
	umsatzFreiFaktor         = 1.5                   // bis 1,5 Monatsumsaetze frei
	umsatzStufe2Faktor       = 3.0                   // ab 3 Monatsumsaetzen die hohe Stufe
	liegeRate1Monat          = 0.005                 // 0,5 %/Monat zwischen 1,5 und 3 Monatsumsaetzen
	liegeRate2Monat          = 0.02                  // 2 %/Monat darueber
	umsatzFensterTage        = 90                    // Durchschnitt ueber so viele Tage
	umsatzJahrTage           = 365                   // ... oder ueber das Jahr, wenn das mehr ist (Saison)
	gruendungTage            = 182                   // erstes halbes Jahr: wie ein Mensch behandelt
	gruendungAbstandTage     = 365                   // hoechstens einmal je Mensch in dieser Zeit
	umsatzMindestTage        = 30                    // darunter kein Liegegeld (zu wenig Daten)
	menschZaehltJeUntQuartal = 9 * registrationGrant // je Mensch und Unternehmen und Quartal
	maxUnternehmenJeMensch   = 3
	maxVerantwortlicheJeUnt  = 10

	// Ausstieg AEQ -> Stable
	ausstiegsAbgabeBps = 200 // 2 %

	sekundenJeMonat = 30 * 86400
)

var unternehmenKategorien = map[string]bool{
	"lebensmittel": true, "gastronomie": true, "handel": true, "handwerk": true,
	"dienstleistung": true, "gesundheit": true, "bildung": true, "kultur": true,
	"verein": true, "landwirtschaft": true, "technik": true, "sonstiges": true,
}

// wirtschaftAktivOverride: nur fuer Tests (0 = Konstante gilt).
var wirtschaftAktivOverride atomic.Int64

func wirtschaftAktiv(unix int64) bool {
	a := wirtschaftAktivAbUnix
	if o := wirtschaftAktivOverride.Load(); o != 0 {
		a = o
	}
	return unix >= a
}

type kontoart int

const (
	artFrei kontoart = iota
	artMensch
	artUnternehmen
	artSystem
)

func (k kontoart) String() string {
	switch k {
	case artMensch:
		return "mensch"
	case artUnternehmen:
		return "unternehmen"
	case artSystem:
		return "system"
	}
	return "frei"
}

func istSystemAdresse(addr string) bool {
	addr = strings.ToLower(addr)
	return isTokenomicsPoolAddress(addr) || addr == strings.ToLower(V7_CONTRACT_ADDR) ||
		strings.HasPrefix(addr, "0x00000000000000000000000000000000000000")
}

// ------------------------------------------------------------ Zustand

type unternehmenEintrag struct {
	Adresse         string   `json:"adresse"`
	Name            string   `json:"name"`
	Kategorie       string   `json:"kategorie"`
	Verantwortliche []string `json:"verantwortliche"`
	EroeffnetAm     int64    `json:"eroeffnet_am"`
	GeschlossenAm   int64    `json:"geschlossen_am,omitempty"`
}

func (e *unternehmenEintrag) offen() bool { return e != nil && e.GeschlossenAm == 0 }

func (e *unternehmenEintrag) istVerantwortlich(mensch string) bool {
	if e == nil {
		return false
	}
	for _, v := range e.Verantwortliche {
		if v == mensch {
			return true
		}
	}
	return false
}

// tagesUmsatz: anrechenbare Eingaenge eines Unternehmens an einem Tag.
type tagesUmsatz struct {
	Tag    int64   `json:"t"`            // Unix-Tag (unix / 86400)
	Mensch float64 `json:"m,omitempty"`  // von Menschen, je Mensch gedeckelt
	BEin   float64 `json:"be,omitempty"` // von anderen Unternehmen
	BAus   float64 `json:"ba,omitempty"` // an andere Unternehmen
}

type buchKonto struct {
	Monat int `json:"m,omitempty"` // JJJJMM der Zaehler
	// Menschen
	Ausgegeben float64 `json:"aus,omitempty"`
	Getauscht  float64 `json:"tau,omitempty"`
	Lohn       float64 `json:"lohn,omitempty"`
	// je Unternehmen schon als Umsatz gezaehlt in diesem Monat
	Gezaehlt  map[string]float64 `json:"gz,omitempty"`
	GzQuartal int                `json:"gq,omitempty"` // JJJJQ, Zeitraum von Gezaehlt
	// Gezaehlt des Vorquartals: eine Rueckzahlung kurz nach dem Wechsel
	// hebt den Einkauf von kurz davor noch auf (siehe rueckzahlungLocked).
	GezaehltVorher map[string]float64 `json:"gzv,omitempty"`
	// Unternehmen: Umsatz der letzten 90 Tage, je Tag
	Tage []tagesUmsatz `json:"td,omitempty"`
	// Unternehmen (oeffentliche Monatssummen)
	Einnahmen   float64 `json:"ein,omitempty"`
	LohnGezahlt float64 `json:"lgz,omitempty"`
	Entnahmen   float64 `json:"ent,omitempty"`
	// alle Kontoarten: selbst von Stable in AEQ getauscht und noch nicht
	// zurueckgetauscht -- geht ohne Ausstiegsabgabe zurueck (nicht monatlich)
	Eingezahlt float64 `json:"ez,omitempty"`
}

type wirtschaft struct {
	mu          sync.Mutex
	unternehmen map[string]*unternehmenEintrag
	buch        map[string]*buchKonto
	// letzter Umlauf-Durchlauf (Blockzeit), fuer die Laenge des Zeitraums
	letzterUmlauf int64
	// seit wann dieser Knoten vollstaendig Buch fuehrt (0 = seit Beginn).
	// Nach einem Snapshot-Import neu gesetzt: die Buchfuehrung davor fehlt.
	buchSeit int64
	// der laufende Tagesdurchlauf beim Nachspielen: sein at und das
	// letzterUmlauf davor -- die Laenge des Zeitraums fuer die Pruefung.
	laufAt, laufVorher int64
}

func neueWirtschaft() *wirtschaft {
	return &wirtschaft{
		unternehmen: map[string]*unternehmenEintrag{},
		buch:        map[string]*buchKonto{},
	}
}

func (cs *ChainState) wirt() *wirtschaft {
	if w := cs.wirtschaftP.Load(); w != nil {
		return w
	}
	cs.wirtschaftP.CompareAndSwap(nil, neueWirtschaft())
	return cs.wirtschaftP.Load()
}

func monatVon(unix int64) int {
	t := time.Unix(unix, 0).UTC()
	return t.Year()*100 + int(t.Month())
}

// kontoLocked: Buchkonto, Zaehler auf den laufenden Monat gebracht. w.mu gehalten.
func (w *wirtschaft) kontoLocked(addr string, jetzt int64) *buchKonto {
	k := w.buch[addr]
	if k == nil {
		k = &buchKonto{}
		w.buch[addr] = k
	}
	if m := monatVon(jetzt); k.Monat != m {
		k.Monat = m
		k.Ausgegeben, k.Getauscht, k.Lohn = 0, 0, 0
		k.Einnahmen, k.LohnGezahlt, k.Entnahmen = 0, 0, 0
	}
	return k
}

func quartalVon(unix int64) int {
	t := time.Unix(unix, 0).UTC()
	return t.Year()*10 + (int(t.Month())-1)/3 + 1
}

func (w *wirtschaft) offenesUnternehmenLocked(addr string) *unternehmenEintrag {
	if e := w.unternehmen[addr]; e.offen() {
		return e
	}
	return nil
}

// istUnternehmen: offenes Unternehmenskonto? (Konsens: nur aus dem Register.)
func (w *wirtschaft) istUnternehmen(addr string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.offenesUnternehmenLocked(strings.ToLower(addr)) != nil
}

func (w *wirtschaft) unternehmenVon(mensch string) []string {
	var r []string
	for a, e := range w.unternehmen {
		if e.offen() && e.istVerantwortlich(mensch) {
			r = append(r, a)
		}
	}
	sort.Strings(r)
	return r
}

func (cs *ChainState) kontoartVon(addr string, istMensch bool) kontoart {
	addr = strings.ToLower(addr)
	switch {
	case istSystemAdresse(addr):
		return artSystem
	case istMensch:
		return artMensch
	case cs.wirt().istUnternehmen(addr):
		return artUnternehmen
	}
	return artFrei
}

// ------------------------------------------------------------ Umsatz

func unixTag(unix int64) int64 { return unix / 86400 }

// tagLocked: der Umsatz-Eintrag fuer heute; alte Tage fallen heraus. w.mu gehalten.
func (k *buchKonto) tagLocked(jetzt int64) *tagesUmsatz {
	heute := unixTag(jetzt)
	grenze := heute - umsatzJahrTage
	behalten := k.Tage[:0]
	for _, t := range k.Tage {
		if t.Tag > grenze {
			behalten = append(behalten, t)
		}
	}
	k.Tage = behalten
	if n := len(k.Tage); n > 0 && k.Tage[n-1].Tag == heute {
		return &k.Tage[n-1]
	}
	k.Tage = append(k.Tage, tagesUmsatz{Tag: heute})
	return &k.Tage[len(k.Tage)-1]
}

func gemeinsameVerantwortliche(a, b *unternehmenEintrag) bool {
	if a == nil || b == nil {
		return false
	}
	for _, v := range a.Verantwortliche {
		if b.istVerantwortlich(v) {
			return true
		}
	}
	return false
}

func aktivAb() int64 {
	if o := wirtschaftAktivOverride.Load(); o != 0 {
		return o
	}
	return wirtschaftAktivAbUnix
}

// knotenTageLocked: seit wie vielen Tagen DIESER KNOTEN vollstaendig Buch
// fuehrt (ab Aktivierung bzw. buchSeit). w.mu gehalten.
func (w *wirtschaft) knotenTageLocked(jetzt int64) int64 {
	ab := aktivAb()
	if w.buchSeit > ab {
		ab = w.buchSeit
	}
	if t := (jetzt - ab) / 86400; t > 0 {
		return t
	}
	return 0
}

// mittelTageLocked: ueber wie viele Tage der Umsatz eines Unternehmens
// gemittelt wird -- die bekannten Tage, mindestens 30 (ein neues Unternehmen
// wird nicht aus wenigen Tagen hochgerechnet), hoechstens fenster.
// w.mu gehalten.
func (w *wirtschaft) mittelTageLocked(e *unternehmenEintrag, jetzt, fenster int64) int64 {
	ab := aktivAb()
	if e != nil && e.EroeffnetAm > ab {
		ab = e.EroeffnetAm
	}
	if w.buchSeit > ab {
		ab = w.buchSeit
	}
	// Kalendertage einschliesslich des ersten und des heutigen: sonst fiele
	// der Umsatz des Eroeffnungstags aus dem Fenster (monatsUmsatzLocked).
	tage := unixTag(jetzt) - unixTag(ab) + 1
	if tage < umsatzMindestTage {
		tage = umsatzMindestTage
	}
	if tage > fenster {
		tage = fenster
	}
	return tage
}

// umsatzLocked: der Monatsumsatz, der zaehlt -- der hoehere aus dem
// 90-Tage-Durchschnitt und dem Jahresdurchschnitt. Ein Saisonbetrieb
// (Skiverleih, Hofladen, Weihnachtsgeschaeft) verdient in wenigen Monaten
// und braucht danach seine Ruecklage; der 90-Tage-Durchschnitt allein
// wuerde sie nach der Saison als Horten behandeln. Gemessen wird in beiden
// Faellen echter Umsatz, niemand bekommt einen Vorteil. w.mu gehalten.
func (w *wirtschaft) umsatzLocked(addr string, jetzt int64) float64 {
	e := w.unternehmen[addr]
	k := w.kontoLocked(addr, jetzt)
	u90 := w.monatsUmsatzLocked(k, w.mittelTageLocked(e, jetzt, umsatzFensterTage), jetzt)
	uJahr := w.monatsUmsatzLocked(k, w.mittelTageLocked(e, jetzt, umsatzJahrTage), jetzt)
	return math.Max(u90, uJahr)
}

// inGruendungLocked: ist das Unternehmen in seinem ersten halben Jahr, und
// hat die Person, die es eroeffnet hat, in den zwoelf Monaten davor kein
// anderes eroeffnet? Folgt allein aus dem Register (Konsens). w.mu gehalten.
func (w *wirtschaft) inGruendungLocked(e *unternehmenEintrag, jetzt int64) bool {
	if e == nil || len(e.Verantwortliche) == 0 || jetzt-e.EroeffnetAm >= gruendungTage*86400 {
		return false
	}
	gruender := e.Verantwortliche[0]
	for _, x := range w.unternehmen {
		if x == e || len(x.Verantwortliche) == 0 || x.Verantwortliche[0] != gruender {
			continue
		}
		if x.EroeffnetAm < e.EroeffnetAm && e.EroeffnetAm-x.EroeffnetAm < gruendungAbstandTage*86400 {
			return false
		}
		// gleiche Sekunde: das mit der kleineren Adresse gilt als erstes
		if x.EroeffnetAm == e.EroeffnetAm && x.Adresse < e.Adresse {
			return false
		}
	}
	return true
}

// menschUmlaufFuer: was ein Mensch mit diesem Guthaben im Monat zahlt --
// bis zur Vermoegensgrenze, die fuer Menschen gilt.
func menschUmlaufFuer(stand float64) float64 {
	return math.Max(0, stand-menschSparFreibetrag) * menschUmlaufMonat
}

// liegegeldLocked: Liegegeld pro Monat fuer ein Unternehmen bei diesem Stand.
// Weniger als 30 Tage Daten auf diesem Knoten: 0 (siehe Kopf).
//
// GRUENDUNG. Im ersten halben Jahr zahlt ein Unternehmen hoechstens, was ein
// Mensch zahlen wuerde: 5.000 AEQ frei, darueber 0,5 %. Das gilt nur bis zur
// Grenze, die fuer Menschen gilt (25.000 AEQ); darueber die Regeln fuer
// Unternehmen -- sonst liesse sich ein halbes Jahr lang beliebig viel billig
// parken. Kein Vorrecht fuer Gruender, nur kein Nachteil: Startkapital, das
// die Gruenderin als Mensch halten koennte, kostet in der Firma nicht mehr.
// Einmal je Mensch in zwoelf Monaten (inGruendungLocked). w.mu gehalten.
func (w *wirtschaft) liegegeldLocked(addr string, stand float64, jetzt int64) float64 {
	if w.knotenTageLocked(jetzt) < umsatzMindestTage {
		return 0
	}
	umsatz := w.umsatzLocked(addr, jetzt)
	normal := liegegeldFuerStand(stand, umsatz)
	if !w.inGruendungLocked(w.unternehmen[addr], jetzt) {
		return normal
	}
	grenze := registrationGrant * wealthCapMultiplier
	unten := math.Min(stand, grenze)
	wieMensch := menschUmlaufFuer(unten) + liegegeldFuerStand(stand, umsatz) - liegegeldFuerStand(unten, umsatz)
	return math.Min(normal, wieMensch)
}

// monatsUmsatzLocked: anrechenbarer Umsatz pro Monat (30 Tage) im Durchschnitt
// der bekannten Tage. Menschen gedeckelt, Unternehmen nur Ueberschuss. w.mu gehalten.
func (w *wirtschaft) monatsUmsatzLocked(k *buchKonto, tage, jetzt int64) float64 {
	if tage <= 0 {
		return 0
	}
	grenze := unixTag(jetzt) - tage
	var mensch, ein, aus float64
	for _, t := range k.Tage {
		if t.Tag > grenze {
			mensch += t.Mensch
			ein += t.BEin
			aus += t.BAus
		}
	}
	// mensch kann durch Rueckzahlungen (rueckzahlungLocked) unter null
	// fallen, wenn der Einkauf schon aus dem Fenster ist.
	return (math.Max(0, mensch) + math.Max(0, ein-aus)) * 30 / float64(tage)
}

// quartalLocked: Gezaehlt auf das laufende Quartal bringen; das eben
// abgelaufene bleibt als GezaehltVorher. w.mu gehalten.
func (k *buchKonto) quartalLocked(jetzt int64) {
	q := quartalVon(jetzt)
	if k.GzQuartal == q {
		return
	}
	k.GezaehltVorher = nil
	if naechstesQuartal(k.GzQuartal) == q {
		k.GezaehltVorher = k.Gezaehlt
	}
	k.GzQuartal, k.Gezaehlt = q, nil
}

func naechstesQuartal(q int) int {
	if q%10 == 4 {
		return (q/10+1)*10 + 1
	}
	return q + 1
}

// rueckzahlungLocked: zahlt ein Unternehmen einem Menschen Geld, hebt das
// auf, was dessen Einkaeufe dort als Umsatz gezaehlt haben (dieses und
// voriges Quartal). Geld, das an denselben Menschen zurueckgeht, war kein
// Umsatz: sonst kaufen Freunde ein, bekommen das Geld als "Lohn" zurueck,
// und der Freibetrag waechst ohne einen echten Verkauf. Rueckerstattungen
// fuer zurueckgegebene Ware fallen genauso heraus -- richtig so. Kauft eine
// Angestellte bei ihrem Arbeitgeber ein, zaehlt ihr Einkauf dort nicht;
// das kostet das Unternehmen wenig. w.mu gehalten.
func rueckzahlungLocked(fk, mensch *buchKonto, firma string, amount float64, jetzt int64) {
	mensch.quartalLocked(jetzt)
	rest := amount
	for _, m := range []map[string]float64{mensch.Gezaehlt, mensch.GezaehltVorher} {
		if rest <= 0 || m == nil || m[firma] <= 0 {
			continue
		}
		n := math.Min(rest, m[firma])
		m[firma] -= n
		rest -= n
		fk.tagLocked(jetzt).Mensch -= n
	}
}

// liegegeldFuerStand: Liegegeld pro Monat bei diesem Guthaben und Monatsumsatz.
func liegegeldFuerStand(stand, umsatz float64) float64 {
	frei := math.Max(unternehmenSockel, umsatzFreiFaktor*umsatz)
	stufe2 := math.Max(unternehmenSockel, umsatzStufe2Faktor*umsatz)
	mittel := math.Max(0, math.Min(stand, stufe2)-frei)
	hoch := math.Max(0, stand-stufe2)
	return mittel*liegeRate1Monat + hoch*liegeRate2Monat
}

// ------------------------------------------------------------ Gebuehren

// gebuehrMitWirtschaft ersetzt ueberweisungsGebuehrFuer ab der Aktivierung:
//   - Mensch: die ersten 1.000 AEQ Ausgaben im Monat gebuehrenfrei, danach
//     0,1 % ohne Aufschlagstufen (Konzept 14.5).
//   - Unternehmen -> Mensch: 0 (Lohn, Entnahme, Erstattung).
//   - Unternehmen -> sonst und freie Adresse: 0,1 %.
//
// Vor der Aktivierung gilt die bisherige Gebuehr mit Aufschlag unveraendert --
// sonst aendert sich das Nachspielen bestehender Bloecke.
func (cs *ChainState) gebuehrMitWirtschaft(from, to string, fromArt, toArt kontoart, amount, guthaben float64, jetzt int64) float64 {
	if !wirtschaftAktiv(jetzt) {
		return ueberweisungsGebuehrFuer(amount, guthaben)
	}
	switch fromArt {
	case artMensch:
		w := cs.wirt()
		w.mu.Lock()
		frei := math.Max(0, menschFreiAusgabenMonat-w.kontoLocked(from, jetzt).Ausgegeben)
		w.mu.Unlock()
		return grundGebuehr(math.Max(0, amount-frei))
	case artUnternehmen:
		if toArt == artMensch {
			return 0
		}
	}
	return grundGebuehr(amount)
}

// grundGebuehr: 0,1 % ohne Aufschlag, auf Mikro-AEQ gerundet.
func grundGebuehr(betrag float64) float64 {
	if betrag <= 0 || math.IsNaN(betrag) || math.IsInf(betrag, 0) {
		return 0
	}
	return round6(betrag * float64(ueberweisungsGebuehrBps) / 10_000)
}

// pruefeEmpfaengerWirtschaft: eine freie Adresse haelt hoechstens 1.000 AEQ.
// Nur bei der Annahme -- das Nachspielen bestehender Bloecke aendert sich nicht.
func pruefeEmpfaengerWirtschaft(toArt kontoart, standVorher, zufluss float64, jetzt int64) error {
	if !wirtschaftAktiv(jetzt) || toArt != artFrei {
		return nil
	}
	if standVorher+zufluss > freiGrenze+1e-9 {
		return fmt.Errorf("recipient is a free address and may hold at most %.0f AEQ (Empfaenger ist eine freie Adresse: hoechstens %.0f AEQ; fuer mehr ein Unternehmenskonto eroeffnen)", freiGrenze, freiGrenze)
	}
	return nil
}

// nachUeberweisung fuehrt Monatszaehler und Umsatz nach einer Ueberweisung.
// Buchfuehrung, kein Konsens (siehe Kopf); gespeichert in der Transaktion aus ctx.
func (cs *ChainState) nachUeberweisung(ctx context.Context, from, to string, fromArt, toArt kontoart, amount, gebuehr, fromNach, toNach float64, jetzt int64) error {
	if !wirtschaftAktiv(jetzt) || fromArt == artSystem || toArt == artSystem || amount <= 0 {
		return nil
	}
	w := cs.wirt()
	w.mu.Lock()
	fk := w.kontoLocked(from, jetzt)
	tk := w.kontoLocked(to, jetzt)
	fromU, toU := w.offenesUnternehmenLocked(from), w.offenesUnternehmenLocked(to)

	switch {
	case fromArt == artMensch:
		fk.Ausgegeben += amount
		if toArt == artUnternehmen && !toU.istVerantwortlich(from) {
			fk.quartalLocked(jetzt)
			bisher := 0.0
			if fk.Gezaehlt != nil {
				bisher = fk.Gezaehlt[to]
			}
			if n := math.Min(amount, math.Max(0, menschZaehltJeUntQuartal-bisher)); n > 0 {
				if fk.Gezaehlt == nil {
					fk.Gezaehlt = map[string]float64{}
				}
				fk.Gezaehlt[to] = bisher + n
				tk.tagLocked(jetzt).Mensch += n
			}
		}
	case fromArt == artUnternehmen && toArt == artMensch:
		if fromU.istVerantwortlich(to) {
			fk.Entnahmen += amount
		} else {
			fk.LohnGezahlt += amount
			tk.Lohn += amount
			rueckzahlungLocked(fk, tk, from, amount, jetzt)
		}
	case fromArt == artUnternehmen && toArt == artUnternehmen:
		if !gemeinsameVerantwortliche(fromU, toU) {
			fk.tagLocked(jetzt).BAus += amount
			tk.tagLocked(jetzt).BEin += amount
		}
	}
	if toArt == artUnternehmen {
		tk.Einnahmen += amount
	}
	w.mu.Unlock()
	return cs.speichereBuchCtx(ctx, from, to)
}

// ausstiegsAbgabe: 2 % auf AEQ -> Stable. Abgabefrei ist, was das Konto
// selbst von Stable in AEQ getauscht hat (Eingezahlt): wer Geld einzahlt und
// wieder abhebt, gewinnt nichts und nimmt niemandem etwas. Menschen tauschen
// darueber hinaus 3.000 AEQ im Monat ohne Abgabe, egal woher (Konzept 14.5).
func (cs *ChainState) ausstiegsAbgabe(addr string, art kontoart, amountIn float64, jetzt int64) float64 {
	if !wirtschaftAktiv(jetzt) || amountIn <= 0 || art == artSystem {
		return 0
	}
	w := cs.wirt()
	w.mu.Lock()
	k := w.kontoLocked(addr, jetzt)
	rest := amountIn - math.Min(amountIn, math.Max(0, k.Eingezahlt))
	frei := 0.0
	if art == artMensch {
		frei = math.Max(0, menschTauschFreiMonat-k.Getauscht)
	}
	w.mu.Unlock()
	return round6(math.Max(0, rest-frei) * ausstiegsAbgabeBps / 10_000)
}

// nachTausch: AEQ -> Stable verbucht. Zuerst wird die eigene Einlage
// aufgebraucht, der Rest zaehlt gegen den Monatsfreibetrag.
func (cs *ChainState) nachTausch(ctx context.Context, addr string, amountIn float64, jetzt int64) error {
	if !wirtschaftAktiv(jetzt) {
		return nil
	}
	w := cs.wirt()
	w.mu.Lock()
	k := w.kontoLocked(addr, jetzt)
	aus := math.Min(amountIn, math.Max(0, k.Eingezahlt))
	k.Eingezahlt = round6(k.Eingezahlt - aus)
	k.Getauscht += amountIn - aus
	w.mu.Unlock()
	return cs.speichereBuchCtx(ctx, addr)
}

// nachEinzahlung: Stable -> AEQ verbucht (aeqErhalten).
func (cs *ChainState) nachEinzahlung(ctx context.Context, addr string, aeqErhalten float64, jetzt int64) error {
	if !wirtschaftAktiv(jetzt) || aeqErhalten <= 0 {
		return nil
	}
	w := cs.wirt()
	w.mu.Lock()
	k := w.kontoLocked(addr, jetzt)
	k.Eingezahlt = round6(k.Eingezahlt + aeqErhalten)
	w.mu.Unlock()
	return cs.speichereBuchCtx(ctx, addr)
}

// ------------------------------------------------------------ Umlauf (taeglich)

// umlaufBetrag: was ein Konto fuer den Zeitraum sekunden schuldet.
func (cs *ChainState) umlaufBetrag(addr string, art kontoart, stand float64, jetzt, sekunden int64) float64 {
	if stand <= 0 || sekunden <= 0 {
		return 0
	}
	anteil := float64(sekunden) / sekundenJeMonat
	switch art {
	case artMensch:
		return round6(math.Max(0, stand-menschSparFreibetrag) * menschUmlaufMonat * anteil)
	case artFrei:
		return round6(stand * freiUmlaufMonat * anteil)
	case artUnternehmen:
		w := cs.wirt()
		w.mu.Lock()
		defer w.mu.Unlock()
		return round6(w.liegegeldLocked(addr, stand, jetzt) * anteil)
	}
	return 0
}

// umlaufLocked berechnet die Umlauf-Transaktionen eines Tagesdurchlaufs und
// wendet sie an. Reihenfolge nach Adresse, damit jeder Knoten dieselbe
// Folge sieht. Vor der Aktivierung leer. Caller haelt cs.mu.
func (cs *ChainState) umlaufLocked(ctx context.Context, at int64) ([]Transaction, error) {
	if !wirtschaftAktiv(at) {
		return nil, nil
	}
	w := cs.wirt()
	w.mu.Lock()
	sekunden := int64(86400)
	if w.letzterUmlauf > 0 {
		sekunden = at - w.letzterUmlauf
	}
	w.mu.Unlock()
	if sekunden <= 0 {
		return nil, nil
	}
	if sekunden > 7*86400 {
		sekunden = 7 * 86400
	}

	kandidaten := map[string]bool{}
	if cs.db != nil {
		rows, err := cs.dbExecCtx(ctx).Query(
			`SELECT lower(address) FROM chain_accounts
			 WHERE (is_human = true AND balance > $1) OR (is_human = false AND balance > 0)
			 ORDER BY 1`, menschSparFreibetrag)
		if err != nil {
			return nil, fmt.Errorf("umlauf: %w", err)
		}
		for rows.Next() {
			var a string
			if rows.Scan(&a) == nil {
				kandidaten[a] = true
			}
		}
		rows.Close()
	} else {
		cs.accounts.Range(func(a string, acc *AccountState) bool {
			if acc.Balance.Float() > 0 {
				kandidaten[strings.ToLower(a)] = true
			}
			return true
		})
	}
	adressen := make([]string, 0, len(kandidaten))
	for a := range kandidaten {
		if !istSystemAdresse(a) {
			adressen = append(adressen, a)
		}
	}
	sort.Strings(adressen)

	var txs []Transaction
	for _, a := range adressen {
		cs.ensureAccountLoadedCtx(ctx, a)
		acc, ok := cs.accounts.Get(a)
		if !ok {
			continue
		}
		art := cs.kontoartVon(a, acc.IsHuman)
		betrag := cs.umlaufBetrag(a, art, acc.Balance.Float(), at, sekunden)
		if betrag <= 0 {
			continue
		}
		if err := cs.applyUmlaufDeltaLocked(ctx, a, betrag, at); err != nil {
			return nil, err
		}
		txs = append(txs, Transaction{Type: "umlauf", Wallet: a, Amount: betrag, DistributionAt: at})
	}
	w.mu.Lock()
	w.letzterUmlauf = at
	w.mu.Unlock()
	cs.speichereLetztenUmlauf(ctx, at)
	return txs, nil
}

// applyUmlaufDeltaLocked: Betrag vom Konto ins Grundeinkommen. Auf das
// Guthaben gedeckelt -- ein Block nimmt nie mehr, als da ist. Caller haelt cs.mu.
func (cs *ChainState) applyUmlaufDeltaLocked(ctx context.Context, wallet string, amount float64, at int64) error {
	wallet = strings.ToLower(strings.TrimSpace(wallet))
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return fmt.Errorf("umlauf %s: Betrag %v: %w", wallet, amount, ErrZustandLehntAb)
	}
	if istSystemAdresse(wallet) {
		return fmt.Errorf("umlauf %s: Systemadresse: %w", wallet, ErrZustandLehntAb)
	}
	cs.ensureAccountLoadedCtx(ctx, wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		return nil
	}
	b := NewDecimal(amount)
	if b > acc.Balance {
		b = acc.Balance
	}
	if b <= 0 {
		return nil
	}
	acc.Balance = acc.Balance.Sub(b)
	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return fmt.Errorf("umlauf %s: %w", wallet, err)
	}
	cs.ensureAccountLoadedCtx(ctx, ubiPoolAddr)
	ubiAcc, ok := cs.accounts.Get(ubiPoolAddr)
	if !ok {
		ubiAcc = &AccountState{Address: ubiPoolAddr}
		cs.accounts.Set(ubiPoolAddr, ubiAcc)
	}
	ubiAcc.Balance = ubiAcc.Balance.Add(b)
	if err := cs.saveAccountToDBCtx(ctx, ubiAcc); err != nil {
		return fmt.Errorf("umlauf: Grundeinkommen: %w", err)
	}
	w := cs.wirt()
	w.mu.Lock()
	if at > w.letzterUmlauf {
		w.laufAt, w.laufVorher = at, w.letzterUmlauf
		w.letzterUmlauf = at
	}
	w.mu.Unlock()
	return nil
}

// ------------------------------------------------------------ Register (Konsens)

func normAdresse(a string) (string, bool) {
	a = strings.ToLower(strings.TrimSpace(a))
	if len(a) != 42 || !strings.HasPrefix(a, "0x") {
		return "", false
	}
	for _, c := range a[2:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", false
		}
	}
	return a, true
}

func normName(n string) string {
	n = strings.TrimSpace(n)
	n = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r == '|' {
			return ' '
		}
		return r
	}, n)
	if r := []rune(n); len(r) > 60 {
		n = string(r[:60])
	}
	return n
}

// pruefeMenschLocked: registrierter Mensch? Caller haelt cs.mu.
func (cs *ChainState) pruefeMenschLocked(ctx context.Context, addr string) error {
	cs.ensureAccountLoadedCtx(ctx, addr)
	acc, ok := cs.accounts.Get(addr)
	if !ok || !acc.IsHuman {
		return fmt.Errorf("%s ist kein registrierter Mensch: %w", addr, ErrZustandLehntAb)
	}
	return nil
}

// applyUnternehmenEroeffnenLocked traegt ein Unternehmenskonto ein.
// Deterministisch aus dem Zustand -- Erzeuger und Nachspielen pruefen gleich.
func (cs *ChainState) applyUnternehmenEroeffnenLocked(ctx context.Context, unternehmen, mensch, name, kategorie string, at int64) error {
	if !wirtschaftAktiv(at) {
		return fmt.Errorf("unternehmen_eroeffnen vor der Aktivierung: %w", ErrZustandLehntAb)
	}
	u, ok1 := normAdresse(unternehmen)
	m, ok2 := normAdresse(mensch)
	if !ok1 || !ok2 || u == m {
		return fmt.Errorf("unternehmen_eroeffnen: ungueltige Adressen: %w", ErrZustandLehntAb)
	}
	kategorie = strings.ToLower(strings.TrimSpace(kategorie))
	if !unternehmenKategorien[kategorie] {
		return fmt.Errorf("unternehmen_eroeffnen: unbekannte Kategorie %q: %w", kategorie, ErrZustandLehntAb)
	}
	if istSystemAdresse(u) {
		return fmt.Errorf("unternehmen_eroeffnen: Systemadresse: %w", ErrZustandLehntAb)
	}
	if err := cs.pruefeMenschLocked(ctx, m); err != nil {
		return err
	}
	cs.ensureAccountLoadedCtx(ctx, u)
	if acc, ok := cs.accounts.Get(u); ok && acc.IsHuman {
		return fmt.Errorf("unternehmen_eroeffnen: %s ist ein Mensch: %w", u, ErrZustandLehntAb)
	}
	w := cs.wirt()
	w.mu.Lock()
	if w.offenesUnternehmenLocked(u) != nil {
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_eroeffnen: %s ist schon ein Unternehmen: %w", u, ErrZustandLehntAb)
	}
	if len(w.unternehmenVon(m)) >= maxUnternehmenJeMensch {
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_eroeffnen: %s ist schon fuer %d Unternehmen verantwortlich: %w", m, maxUnternehmenJeMensch, ErrZustandLehntAb)
	}
	e := &unternehmenEintrag{Adresse: u, Name: normName(name), Kategorie: kategorie, Verantwortliche: []string{m}, EroeffnetAm: at}
	w.unternehmen[u] = e
	w.mu.Unlock()
	return cs.speichereUnternehmen(ctx, e)
}

func (cs *ChainState) applyUnternehmenMitinhaberLocked(ctx context.Context, unternehmen, mensch string, at int64) error {
	if !wirtschaftAktiv(at) {
		return fmt.Errorf("unternehmen_mitinhaber vor der Aktivierung: %w", ErrZustandLehntAb)
	}
	u, ok1 := normAdresse(unternehmen)
	m, ok2 := normAdresse(mensch)
	if !ok1 || !ok2 {
		return fmt.Errorf("unternehmen_mitinhaber: ungueltige Adressen: %w", ErrZustandLehntAb)
	}
	if err := cs.pruefeMenschLocked(ctx, m); err != nil {
		return err
	}
	w := cs.wirt()
	w.mu.Lock()
	e := w.offenesUnternehmenLocked(u)
	switch {
	case e == nil:
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_mitinhaber: %s ist kein offenes Unternehmen: %w", u, ErrZustandLehntAb)
	case e.istVerantwortlich(m):
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_mitinhaber: %s ist schon verantwortlich: %w", m, ErrZustandLehntAb)
	case len(e.Verantwortliche) >= maxVerantwortlicheJeUnt:
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_mitinhaber: hoechstens %d Verantwortliche: %w", maxVerantwortlicheJeUnt, ErrZustandLehntAb)
	case len(w.unternehmenVon(m)) >= maxUnternehmenJeMensch:
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_mitinhaber: %s ist schon fuer %d Unternehmen verantwortlich: %w", m, maxUnternehmenJeMensch, ErrZustandLehntAb)
	}
	e.Verantwortliche = append(e.Verantwortliche, m)
	sort.Strings(e.Verantwortliche)
	cp := *e
	w.mu.Unlock()
	return cs.speichereUnternehmen(ctx, &cp)
}

// applyUnternehmenSchliessenLocked: nur mit leerem Konto. Danach ist die
// Adresse wieder eine freie Adresse.
func (cs *ChainState) applyUnternehmenSchliessenLocked(ctx context.Context, unternehmen string, at int64) error {
	u, ok := normAdresse(unternehmen)
	if !ok {
		return fmt.Errorf("unternehmen_schliessen: ungueltige Adresse: %w", ErrZustandLehntAb)
	}
	cs.ensureAccountLoadedCtx(ctx, u)
	if acc, ok := cs.accounts.Get(u); ok && acc.Balance.Float() > 0.000001 {
		return fmt.Errorf("unternehmen_schliessen: Konto haelt noch %.6f AEQ -- erst auszahlen: %w", acc.Balance.Float(), ErrZustandLehntAb)
	}
	w := cs.wirt()
	w.mu.Lock()
	e := w.offenesUnternehmenLocked(u)
	if e == nil {
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_schliessen: %s ist kein offenes Unternehmen: %w", u, ErrZustandLehntAb)
	}
	e.GeschlossenAm = at
	cp := *e
	w.mu.Unlock()
	return cs.speichereUnternehmen(ctx, &cp)
}

// ------------------------------------------------------------ Speicher

func (cs *ChainState) wirtschaftInitDB() {
	if cs.db == nil {
		return
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS wirtschaft_unternehmen (
			address TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', kategorie TEXT NOT NULL DEFAULT '',
			verantwortliche TEXT NOT NULL DEFAULT '', eroeffnet_at BIGINT NOT NULL DEFAULT 0, geschlossen_at BIGINT NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS wirtschaft_buch (address TEXT PRIMARY KEY, daten TEXT NOT NULL DEFAULT '{}')`,
		`CREATE TABLE IF NOT EXISTS wirtschaft_meta (schluessel TEXT PRIMARY KEY, wert BIGINT NOT NULL DEFAULT 0)`,
	} {
		if _, err := cs.db.Exec(q); err != nil {
			fmt.Printf("[WIRTSCHAFT] Tabelle: %v\n", err)
		}
	}
}

func (cs *ChainState) speichereUnternehmen(ctx context.Context, e *unternehmenEintrag) error {
	if cs.db == nil {
		return nil
	}
	_, err := cs.dbExecCtx(ctx).Exec(`INSERT INTO wirtschaft_unternehmen (address, name, kategorie, verantwortliche, eroeffnet_at, geschlossen_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (address) DO UPDATE SET name=$2, kategorie=$3, verantwortliche=$4, eroeffnet_at=$5, geschlossen_at=$6`,
		e.Adresse, e.Name, e.Kategorie, strings.Join(e.Verantwortliche, ","), e.EroeffnetAm, e.GeschlossenAm)
	if err != nil {
		return fmt.Errorf("unternehmen speichern: %w", err)
	}
	return nil
}

func (cs *ChainState) setzeBuchSeit(at int64) {
	w := cs.wirt()
	w.mu.Lock()
	w.buchSeit = at
	w.mu.Unlock()
	if cs.db == nil {
		return
	}
	cs.db.Exec(`INSERT INTO wirtschaft_meta (schluessel, wert) VALUES ('buch_seit', $1)
		ON CONFLICT (schluessel) DO UPDATE SET wert = $1`, at)
}

func (cs *ChainState) speichereLetztenUmlauf(ctx context.Context, at int64) {
	if cs.db == nil {
		return
	}
	cs.dbExecCtx(ctx).Exec(`INSERT INTO wirtschaft_meta (schluessel, wert) VALUES ('letzter_umlauf', $1)
		ON CONFLICT (schluessel) DO UPDATE SET wert = GREATEST(wirtschaft_meta.wert, $1)`, at)
}

// wirtschaftLaden: beim Start, nach den Konten.
func (cs *ChainState) wirtschaftLaden() {
	if cs.db == nil {
		return
	}
	w := cs.wirt()
	w.mu.Lock()
	defer w.mu.Unlock()
	if rows, err := cs.db.Query(`SELECT address, name, kategorie, verantwortliche, eroeffnet_at, geschlossen_at FROM wirtschaft_unternehmen`); err == nil {
		for rows.Next() {
			var e unternehmenEintrag
			var v string
			if rows.Scan(&e.Adresse, &e.Name, &e.Kategorie, &v, &e.EroeffnetAm, &e.GeschlossenAm) == nil {
				if v != "" {
					e.Verantwortliche = strings.Split(v, ",")
				}
				w.unternehmen[e.Adresse] = &e
			}
		}
		rows.Close()
	}
	if rows, err := cs.db.Query(`SELECT address, daten FROM wirtschaft_buch`); err == nil {
		for rows.Next() {
			var a, d string
			if rows.Scan(&a, &d) == nil {
				var k buchKonto
				if json.Unmarshal([]byte(d), &k) == nil {
					w.buch[a] = &k
				}
			}
		}
		rows.Close()
	}
	var at sql.NullInt64
	if cs.db.QueryRow(`SELECT wert FROM wirtschaft_meta WHERE schluessel = 'letzter_umlauf'`).Scan(&at) == nil && at.Valid {
		w.letzterUmlauf = at.Int64
	}
	var seit sql.NullInt64
	if cs.db.QueryRow(`SELECT wert FROM wirtschaft_meta WHERE schluessel = 'buch_seit'`).Scan(&seit) == nil && seit.Valid {
		w.buchSeit = seit.Int64
	}
	fmt.Printf("✓ Wirtschaft: %d Unternehmen, %d Buchkonten\n", len(w.unternehmen), len(w.buch))
}

// speichereBuchCtx schreibt Buchkonten in die Transaktion aus ctx -- dieselbe
// wie die Kontostaende. Frueher alle 10 Sekunden im Hintergrund: ein Absturz
// verlor die letzten Sekunden, und zwei Knoten mit denselben Bloecken konnten
// verschiedene Zahlen haben. Traegt ctx einen buchSammler (Stapel-Pfad),
// werden die Adressen nur vorgemerkt und am Ende des Stapels in einer
// Anweisung geschrieben -- wie die Konten dort auch.
func (cs *ChainState) speichereBuchCtx(ctx context.Context, adressen ...string) error {
	if cs.db == nil {
		return nil
	}
	if s, _ := ctx.Value(buchSammlerKey{}).(*buchSammler); s != nil {
		for _, a := range adressen {
			s.adressen[a] = true
		}
		return nil
	}
	w := cs.wirt()
	adressen = append([]string(nil), adressen...)
	sort.Strings(adressen)
	var werte []interface{}
	var platz []string
	vorige := ""
	w.mu.Lock()
	for _, a := range adressen {
		if a == vorige {
			continue
		}
		vorige = a
		if k := w.buch[a]; k != nil {
			b, err := json.Marshal(k)
			if err != nil {
				w.mu.Unlock()
				return fmt.Errorf("buch %s: %w", a, err)
			}
			platz = append(platz, fmt.Sprintf("($%d,$%d)", len(werte)+1, len(werte)+2))
			werte = append(werte, a, string(b))
		}
	}
	w.mu.Unlock()
	if len(platz) == 0 {
		return nil
	}
	if _, err := cs.dbExecCtx(ctx).Exec(`INSERT INTO wirtschaft_buch (address, daten) VALUES `+strings.Join(platz, ",")+`
		ON CONFLICT (address) DO UPDATE SET daten = EXCLUDED.daten`, werte...); err != nil {
		return fmt.Errorf("buch speichern (%d Konten): %w", len(platz), err)
	}
	return nil
}

type buchZeitKey struct{}

// mitBuchZeit: unter ctx bucht die Buchfuehrung zu at (Transaction.BuchAt).
func mitBuchZeit(ctx context.Context, at int64) context.Context {
	return context.WithValue(ctx, buchZeitKey{}, at)
}

// buchZeit: der Buchungsaugenblick aus ctx, sonst sonst.
func buchZeit(ctx context.Context, sonst int64) int64 {
	if at, ok := ctx.Value(buchZeitKey{}).(int64); ok && at > 0 {
		return at
	}
	return sonst
}

// buchStempel: was in Transaction.BuchAt kommt -- vor der Aktivierung
// nichts, damit sich alte Transaktionen nicht aendern.
func buchStempel(at int64) int64 {
	if wirtschaftAktiv(at) {
		return at
	}
	return 0
}

// buchZeitBeimNachspielen: BuchAt des Erzeugers, wenn plausibel -- nicht
// nach dem Block (eine Minute Uhrenspiel) und nicht mehr als eine Woche
// davor. Sonst die Blockzeit wie bisher.
func buchZeitBeimNachspielen(buchAt, blockZeit int64) int64 {
	if buchAt > 0 && wirtschaftAktiv(buchAt) && buchAt <= blockZeit+60 && buchAt >= blockZeit-7*86400 {
		return buchAt
	}
	return blockZeit
}

type buchSammlerKey struct{}

type buchSammler struct{ adressen map[string]bool }

// mitBuchSammler: speichereBuchCtx merkt unter dem zurueckgegebenen ctx nur
// vor; schreiben() schreibt alles Vorgemerkte in die Transaktion aus basis.
func mitBuchSammler(basis context.Context) (context.Context, *buchSammler) {
	s := &buchSammler{adressen: map[string]bool{}}
	return context.WithValue(basis, buchSammlerKey{}, s), s
}

func (s *buchSammler) schreiben(cs *ChainState, basis context.Context) error {
	adressen := make([]string, 0, len(s.adressen))
	for a := range s.adressen {
		adressen = append(adressen, a)
	}
	return cs.speichereBuchCtx(basis, adressen...)
}

// buchStand: Kopie von Buchfuehrung, Register und Umlauf-Stand fuer
// blockRollbackSnapshot -- ein abgelehnter Block darf nichts davon aendern.
type buchStand struct {
	voll                              bool
	konten                            map[string]*buchKonto          // nil-Wert: gab es nicht
	register                          map[string]*unternehmenEintrag // dito
	letzterUmlauf, laufAt, laufVorher int64
}

func (e *unternehmenEintrag) kopie() *unternehmenEintrag {
	cp := *e
	cp.Verantwortliche = append([]string(nil), e.Verantwortliche...)
	return &cp
}

func (k *buchKonto) kopie() *buchKonto {
	cp := *k
	if k.Gezaehlt != nil {
		cp.Gezaehlt = make(map[string]float64, len(k.Gezaehlt))
		for a, v := range k.Gezaehlt {
			cp.Gezaehlt[a] = v
		}
	}
	if k.GezaehltVorher != nil {
		cp.GezaehltVorher = make(map[string]float64, len(k.GezaehltVorher))
		for a, v := range k.GezaehltVorher {
			cp.GezaehltVorher[a] = v
		}
	}
	cp.Tage = append([]tagesUmsatz(nil), k.Tage...)
	return &cp
}

func (cs *ChainState) buchSichern(addrs []string, voll bool) *buchStand {
	w := cs.wirt()
	w.mu.Lock()
	defer w.mu.Unlock()
	st := &buchStand{voll: voll, letzterUmlauf: w.letzterUmlauf, laufAt: w.laufAt, laufVorher: w.laufVorher}
	if voll {
		st.konten = make(map[string]*buchKonto, len(w.buch))
		for a, k := range w.buch {
			st.konten[a] = k.kopie()
		}
		st.register = make(map[string]*unternehmenEintrag, len(w.unternehmen))
		for a, e := range w.unternehmen {
			st.register[a] = e.kopie()
		}
		return st
	}
	st.konten = make(map[string]*buchKonto, len(addrs))
	st.register = make(map[string]*unternehmenEintrag, len(addrs))
	for _, a := range addrs {
		st.konten[a] = nil
		if k := w.buch[a]; k != nil {
			st.konten[a] = k.kopie()
		}
		st.register[a] = nil
		if e := w.unternehmen[a]; e != nil {
			st.register[a] = e.kopie()
		}
	}
	return st
}

// buchZurueck: nur der Speicher -- die Zeilen in der Datenbank lagen in der
// zurueckgerollten Transaktion.
func (cs *ChainState) buchZurueck(st *buchStand) {
	if st == nil {
		return
	}
	w := cs.wirt()
	w.mu.Lock()
	defer w.mu.Unlock()
	if st.voll {
		w.buch = make(map[string]*buchKonto, len(st.konten))
		w.unternehmen = make(map[string]*unternehmenEintrag, len(st.register))
	}
	for a, k := range st.konten {
		if k == nil {
			delete(w.buch, a)
		} else {
			w.buch[a] = k
		}
	}
	for a, e := range st.register {
		if e == nil {
			delete(w.unternehmen, a)
		} else {
			w.unternehmen[a] = e
		}
	}
	w.letzterUmlauf, w.laufAt, w.laufVorher = st.letzterUmlauf, st.laufAt, st.laufVorher
}

// ------------------------------------------------------------ Snapshot

func (cs *ChainState) unternehmenFuerSnapshot() []*unternehmenEintrag {
	w := cs.wirt()
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]*unternehmenEintrag, 0, len(w.unternehmen))
	for _, e := range w.unternehmen {
		cp := *e
		cp.Verantwortliche = append([]string(nil), e.Verantwortliche...)
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Adresse < out[j].Adresse })
	return out
}

func (cs *ChainState) unternehmenAusSnapshot(liste []*unternehmenEintrag) {
	// Nach einem Snapshot fehlt diesem Knoten die Buchfuehrung davor: der
	// Umsatz zaehlt ab jetzt, und bis 30 Tage Daten da sind, berechnet er
	// kein Liegegeld (zugunsten der Unternehmen, siehe Kopf).
	cs.setzeBuchSeit(nowUnix())
	if len(liste) == 0 {
		return
	}
	w := cs.wirt()
	w.mu.Lock()
	for _, e := range liste {
		if e == nil {
			continue
		}
		cp := *e
		w.unternehmen[cp.Adresse] = &cp
	}
	w.mu.Unlock()
	for _, e := range liste {
		if e != nil {
			cs.speichereUnternehmen(context.Background(), e)
		}
	}
}

// abgabeInsGrundeinkommen schreibt eine Abgabe dem UBI-Topf gut (Aufrufer
// hat sie dem Konto schon abgezogen). Caller haelt cs.mu.
func (cs *ChainState) abgabeInsGrundeinkommen(ctx context.Context, betrag float64) error {
	if betrag <= 0 {
		return nil
	}
	cs.ensureAccountLoadedCtx(ctx, ubiPoolAddr)
	ubiAcc, ok := cs.accounts.Get(ubiPoolAddr)
	if !ok {
		ubiAcc = &AccountState{Address: ubiPoolAddr}
		cs.accounts.Set(ubiPoolAddr, ubiAcc)
	}
	ubiAcc.Balance = ubiAcc.Balance.Add(NewDecimal(betrag))
	if err := cs.saveAccountToDBCtx(ctx, ubiAcc); err != nil {
		return fmt.Errorf("Abgabe ans Grundeinkommen: %w", err)
	}
	return nil
}
