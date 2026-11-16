package keeper

// QUANTENGELD VARIANTE W: DAS REGISTER UND SEINE UEBERGAENGE (schlafend).
// Regeln wie forschung/quantengeld/wiesner_kette.py, Klasse KetteW.
// Unterschriften und Geld macht die Einbindung: der Aufrufer prueft die
// Unterschriften VORHER und fuehrt die gemeldeten Buchungen (quantenWBuchung)
// danach aus. Zeiten sind Blockzeiten in Sekunden.
//
// Geschaerft vor dem Einhaengen (W2b, 16.11.2026; ANALYSE.md Abschnitt 5):
//   - Zufall aus vorab festgelegten Hash-Ketten ALLER Validatoren der Note
//     statt aus dem Blockhash (den konnte ein Erzeuger waehlen). Fuer den
//     Zufall gibt es KEINE Frist: die Anfrage bleibt offen und ihr Pfand
//     gesperrt, bis er vollstaendig ist. Mit einer Frist (Pfand zurueck,
//     Stellen trotzdem verbraucht) haette ein Erzeuger, der einen Wert
//     zurueckhaelt, eine Note kostenlos erschoepfen koennen (Einloesen
//     anfragen, verfallen lassen, 64 Stellen weg) -- gefunden in der
//     Sicherheitspruefung von W2b Teil 1.
//   - Einloesen mit fester Pruefgroesse k = min(K_MAX, frei): ein Faelscher
//     kann nicht k = 8 waehlen. Pruefungen lassen quantenWReserve stehen.
//   - Ein falsches Einloesen toetet die Note nicht (sonst toetet jeder Fremde
//     jede Note fuer 1 AEQ): Pfand weg, Fehlversuch, die Note bleibt. Das
//     Pfand traegt den Wert der verbrauchten Stellen (quantenWPfandFuer).
//   - Erschoepft (weniger als K_MIN frei): tot, Wert ans Grundeinkommen.

import (
	"sort"
	"strconv"
)

// quantenWAnfrage: eine offene Pruefung oder Einloesung.
type quantenWAnfrage struct {
	Von         string
	Index       int // Nummer der Anfrage an dieser Note = Stelle der Hash-Ketten
	K           int // Zahl der Stellen
	Stellen     []int
	Basen       []int // nil, bis der Zufall vollstaendig ist
	Pfand       int64
	Zeit        int64
	Gezogen     int64 // Zeit des vollstaendigen Zufalls
	Antwort     []int
	AntwortZeit int64
	EinloesenAn string // leer = nur pruefen
}

// quantenWNote: was die Kette ueber eine Variante-W-Note weiss.
type quantenWNote struct {
	N           int
	Wurzel      [32]byte
	Validatoren []string
	T           int
	Benutzt     map[int]bool
	Fehl        int
	Gesperrt    bool
	Tot         bool
	Zaehl       int
	Anfrage     *quantenWAnfrage
	// Hash-Ketten: je Validator der zuletzt bekannte Wert (anfangs der
	// Anker) und seine Stelle (-1 = Anker).
	Z      [][32]byte
	ZIndex []int
}

// quantenWBuchung: was die Einbindung ausfuehrt (Mikro-AEQ).
type quantenWBuchung struct {
	PfandZurueckAn string // Pfand an diesen Anfragenden
	PfandVerfaellt bool   // Pfand ans Grundeinkommen
	WertAn         string // Wert der Note an dieses Konto (eingeloest)
	WertVerfaellt  bool   // Wert der Note ans Grundeinkommen (erschoepft)
}

// quantenWPfand: kleinstes Pfand (1 AEQ).
const quantenWPfand int64 = 1_000_000

// quantenWPraegenPruefen: Form einer neuen Note. Unterschriften und Deckung
// prueft die Einbindung; bestaetigt = Zahl der Validatoren, deren
// Bestaetigung (mit ihrem Anker) gilt -- es muessen ALLE sein, sonst gaebe es
// fuer einen von ihnen keinen Zufall.
func quantenWPraegenPruefen(vorhanden bool, K string, n, t int, validatoren []string, anker int, bestaetigt int) error {
	if vorhanden || !istHash64(K) {
		return quantenWFehler("K ungueltig oder schon da")
	}
	// n >= quantenWKMin: sonst liesse sich die Note nie pruefen oder
	// einloesen, ihre Deckung laege fuer immer im Topf.
	if n < quantenWKMin || n > quantenWNMax || t < 1 || t > quantenWTMax || len(validatoren) < t || len(validatoren) > quantenWVMax {
		return quantenWFehler("Groessen")
	}
	gesehen := map[string]bool{}
	for _, v := range validatoren {
		if v == "" || gesehen[v] {
			return quantenWFehler("Validatorenliste")
		}
		gesehen[v] = true
	}
	if anker != len(validatoren) {
		return quantenWFehler("Anker: %d, noetig %d", anker, len(validatoren))
	}
	if bestaetigt != len(validatoren) {
		return quantenWFehler("Bestaetigungen: %d gueltig, noetig alle %d", bestaetigt, len(validatoren))
	}
	return nil
}

// neueQuantenWNote: nach quantenWPraegenPruefen.
func neueQuantenWNote(n, t int, wurzel [32]byte, validatoren []string, anker [][32]byte) *quantenWNote {
	e := &quantenWNote{N: n, T: t, Wurzel: wurzel, Validatoren: append([]string(nil), validatoren...),
		Z: append([][32]byte(nil), anker...), ZIndex: make([]int, len(anker))}
	for i := range e.ZIndex {
		e.ZIndex[i] = -1
	}
	return e
}

func (e *quantenWNote) frei() []int {
	out := make([]int, 0, e.N-len(e.Benutzt))
	for i := 0; i < e.N; i++ {
		if !e.Benutzt[i] {
			out = append(out, i)
		}
	}
	return out
}

func (e *quantenWNote) verbrauchen(stellen []int) {
	if e.Benutzt == nil {
		e.Benutzt = map[int]bool{}
	}
	for _, i := range stellen {
		e.Benutzt[i] = true
	}
}

// erschoepft: weniger als K_MIN frei, nichts offen -> tot, Wert verfaellt.
func (e *quantenWNote) erschoepft(b *quantenWBuchung) {
	if !e.Tot && e.Anfrage == nil && e.N-len(e.Benutzt) < quantenWKMin {
		e.Tot = true
		b.WertVerfaellt = true
	}
}

// durchgefallen: Pfand ans Grundeinkommen, Fehlversuch -- beim Pruefen wie
// beim Einloesen; die Note stirbt daran nicht.
func (e *quantenWNote) durchgefallen() quantenWBuchung {
	b := quantenWBuchung{PfandVerfaellt: true}
	e.Fehl++
	if e.Fehl >= quantenWFehlMax {
		e.Gesperrt = true
	}
	e.Anfrage = nil
	e.erschoepft(&b)
	return b
}

// anfragen: eine Pruefung (einloesenAn leer) oder Einloesung mit dem Pfand,
// das der Aufrufer mit quantenWPfandFuer aus dem Wert gerechnet hat. Stellen
// und Basen stehen hier NOCH NICHT fest: sie folgen aus dem Zufall.
func (e *quantenWNote) anfragen(von string, k int, einloesenAn string, pfand, zeit int64) error {
	if e.Tot {
		return quantenWFehler("Note tot")
	}
	if e.Anfrage != nil {
		return quantenWFehler("schon eine offene Anfrage")
	}
	if einloesenAn == "" && e.Gesperrt {
		return quantenWFehler("Note gesperrt (Einloesen geht noch)")
	}
	if e.Zaehl >= quantenWZufallL {
		return quantenWFehler("Hash-Ketten der Validatoren verbraucht")
	}
	frei := e.N - len(e.Benutzt)
	if einloesenAn != "" {
		if k != min(quantenWKMax, frei) || k < quantenWKMin {
			return quantenWFehler("k = %d: beim Einloesen fest min(%d, frei = %d)", k, quantenWKMax, frei)
		}
	} else if k < quantenWKMin || k > quantenWKMax || frei-k < quantenWReserve(e.N) {
		return quantenWFehler("k = %d (Reserve fuer das Einloesen: %d von %d frei)", k, quantenWReserve(e.N), frei)
	}
	if pfand < quantenWPfand {
		return quantenWFehler("Pfand")
	}
	e.Anfrage = &quantenWAnfrage{Von: von, Index: e.Zaehl, K: k, Pfand: pfand, Zeit: zeit, EinloesenAn: einloesenAn}
	e.Zaehl++
	return nil
}

// zufall: Werte der Hash-Ketten (Validator-Index 0-basiert -> Wert) fuer die
// offene Anfrage j. Jeder darf sie einreichen; die Kette beweist die
// Herkunft. Alles oder nichts. gezogen = der Zufall ist vollstaendig, die
// Aufforderung steht und ihre Stellen sind verbraucht.
func (e *quantenWNote) zufall(K string, j int, werte map[int][32]byte, zeit int64) (gezogen bool, err error) {
	a := e.Anfrage
	if e.Tot || a == nil || a.Basen != nil {
		return false, quantenWFehler("kein Zufall faellig")
	}
	if j != a.Index || len(werte) == 0 {
		return false, quantenWFehler("Zufall fuer eine andere Anfrage")
	}
	z := append([][32]byte(nil), e.Z...)
	zi := append([]int(nil), e.ZIndex...)
	for x, w := range werte {
		if x < 0 || x >= len(z) {
			return false, quantenWFehler("Zufall: Validator %d", x)
		}
		if zi[x] >= j || quantenWKetteVor(w, j-zi[x]) != z[x] {
			return false, quantenWFehler("Zufall von Validator %d passt nicht zur Hash-Kette oder ist schon da", x+1)
		}
		z[x], zi[x] = w, j
	}
	e.Z, e.ZIndex = z, zi
	for _, m := range zi {
		if m < j {
			return false, nil
		}
	}
	saat := quantenWZufallSaat(K, j, z)
	stellen, basen := quantenWAufforderung(K, K+":"+strconv.Itoa(j), saat[:], e.frei(), a.K)
	if len(stellen) != a.K {
		// Kann nicht eintreten (anfragen prueft die freien Stellen, und bis
		// zum Zufall verbraucht nichts anderes welche); fail-closed.
		return false, quantenWFehler("Aufforderung: zu wenige freie Stellen")
	}
	e.verbrauchen(stellen) // ab jetzt verbraucht, auch ohne Antwort
	a.Stellen, a.Basen, a.Gezogen = stellen, basen, zeit
	return true, nil
}

// antworten: nur der Anfragende, erst nach dem Zufall, bis zur Frist.
func (e *quantenWNote) antworten(von string, bits []int, zeit int64) error {
	a := e.Anfrage
	if a == nil || a.Antwort != nil {
		return quantenWFehler("keine offene Anfrage")
	}
	if von != a.Von {
		return quantenWFehler("nicht vom Anfragenden")
	}
	if a.Basen == nil {
		return quantenWFehler("Aufforderung steht noch aus")
	}
	if zeit > a.Gezogen+quantenWFristAntwort {
		return quantenWFehler("Antwort nach der Frist")
	}
	if len(bits) != len(a.Stellen) {
		return quantenWFehler("Form der Antwort")
	}
	for _, x := range bits {
		if x != 0 && x != 1 {
			return quantenWFehler("Form der Antwort")
		}
	}
	a.Antwort, a.AntwortZeit = append([]int(nil), bits...), zeit
	return nil
}

// offenlegen: jeder Knoten wertet aus und urteilt; Fehler = die
// Offenlegung ist ungueltig und aendert nichts. Nach der Frist nicht mehr
// (dann gilt frist(): Pfand zurueck) -- sonst haenge das Ergebnis davon ab,
// wer zuerst einreicht.
func (e *quantenWNote) offenlegen(K string, offen []quantenWOffenStelle, zeit int64) (quantenWErgebnis, quantenWBuchung, error) {
	a := e.Anfrage
	if a == nil || a.Antwort == nil {
		return 0, quantenWBuchung{}, quantenWFehler("erst die Antwort")
	}
	if zeit > a.AntwortZeit+quantenWFristOffen {
		return 0, quantenWBuchung{}, quantenWFehler("Offenlegung nach der Frist")
	}
	treffer, fehler, err := quantenWAuswerten(K, e.N, e.T, len(e.Validatoren), e.Wurzel, a.Stellen, a.Basen, a.Antwort, offen)
	if err != nil {
		return 0, quantenWBuchung{}, err
	}
	u := quantenWUrteil(treffer, fehler)
	if u == quantenWAbgewiesen {
		return u, e.durchgefallen(), nil
	}
	b := quantenWBuchung{PfandZurueckAn: a.Von}
	e.Anfrage = nil
	if u == quantenWEcht && a.EinloesenAn != "" {
		b.WertAn = a.EinloesenAn
		e.Tot = true
		return u, b, nil
	}
	e.erschoepft(&b)
	return u, b, nil
}

// frist: beendet eine Anfrage, deren Frist abgelaufen ist (quanten_w_frist;
// die Spezifikation ruft sie vor jeder Aktion).
//   - Zufall unvollstaendig: KEINE Frist -- die Anfrage wartet (siehe oben).
//   - Keine Antwort: durchgefallen (Pruefen wie Einloesen).
//   - Antwort da, aber die Validatoren legen nicht offen: Pfand zurueck.
func (e *quantenWNote) frist(zeit int64) (bool, quantenWBuchung) {
	a := e.Anfrage
	switch {
	case a == nil || a.Basen == nil:
		return false, quantenWBuchung{}
	case a.Antwort == nil:
		if zeit > a.Gezogen+quantenWFristAntwort {
			return true, e.durchgefallen()
		}
	case zeit > a.AntwortZeit+quantenWFristOffen:
		e.Anfrage = nil
		b := quantenWBuchung{PfandZurueckAn: a.Von}
		e.erschoepft(&b)
		return true, b
	}
	return false, quantenWBuchung{}
}

// benutztListe: die verbrauchten Stellen, sortiert (fuer Ausgabe/Tests).
func (e *quantenWNote) benutztListe() []int {
	out := make([]int, 0, len(e.Benutzt))
	for i := range e.Benutzt {
		out = append(out, i)
	}
	sort.Ints(out)
	return out
}
