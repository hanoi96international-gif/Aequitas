package keeper

// QUANTENGELD VARIANTE W, STUFE W2a: DAS REGISTER UND SEINE UEBERGAENGE
// (schlafend, nicht eingehaengt). Regeln wie forschung/quantengeld/
// wiesner_kette.py, Klasse KetteW. Unterschriften und Geld macht die
// Einbindung (W2b): hier prueft der Aufrufer die Unterschriften VORHER und
// fuehrt die gemeldeten Buchungen (quantenWBuchung) danach aus.

import (
	"sort"
	"strconv"
)

// quantenWAnfrage: eine offene Pruefung oder Einloesung.
type quantenWAnfrage struct {
	Von          string
	Stellen      []int
	Basen        []int
	Pfand        int64
	Hoehe        int64
	Antwort      []int
	AntwortHoehe int64
	EinloesenAn  string // leer = nur pruefen
}

// quantenWNote: was die Kette ueber eine Variante-W-Note weiss.
type quantenWNote struct {
	N           int
	Wurzel      [32]byte
	Wert        int64
	Validatoren []string
	T           int
	Benutzt     map[int]bool
	Fehl        int
	Gesperrt    bool
	Tot         bool
	Zaehl       int
	Anfrage     *quantenWAnfrage
}

// quantenWBuchung: was die Einbindung ausfuehrt (Mikro-AEQ).
type quantenWBuchung struct {
	PfandZurueckAn string // Pfand an diesen Anfragenden
	PfandVerfaellt bool   // Pfand ans Grundeinkommen
	WertAn         string // Wert der Note an dieses Konto (eingeloest)
	WertVerfaellt  bool   // Wert der Note ans Grundeinkommen (Faelscher/Frist)
}

const quantenWPfand int64 = 1_000_000

// quantenWPraegenPruefen: Form einer neuen Note (Unterschriften und Deckung
// prueft die Einbindung; bestaetigt = Zahl der gueltigen Bestaetigungen).
func quantenWPraegenPruefen(vorhanden bool, K string, n, t int, validatoren []string, bestaetigt int) error {
	if vorhanden || !istHash64(K) {
		return quantenWFehler("K ungueltig oder schon da")
	}
	if n < 1 || n > quantenWNMax || t < 1 || t > quantenWTMax || len(validatoren) < t || len(validatoren) > quantenWVMax {
		return quantenWFehler("Groessen")
	}
	gesehen := map[string]bool{}
	for _, v := range validatoren {
		if v == "" || gesehen[v] {
			return quantenWFehler("Validatorenliste")
		}
		gesehen[v] = true
	}
	if bestaetigt < t {
		return quantenWFehler("zu wenige Bestaetigungen (%d, noetig %d)", bestaetigt, t)
	}
	return nil
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

// quantenWAnfragen: eine Pruefung (einloesenAn leer) oder Einloesung. Die
// Stellen sind ab hier verbraucht -- auch wenn niemand antwortet.
func (e *quantenWNote) anfragen(K, von string, k int, einloesenAn string, hoehe int64, blockhash []byte) error {
	if e.Tot {
		return quantenWFehler("Note tot")
	}
	if e.Anfrage != nil {
		return quantenWFehler("schon eine offene Anfrage")
	}
	if einloesenAn == "" && e.Gesperrt {
		return quantenWFehler("Note gesperrt (Einloesen geht noch)")
	}
	frei := e.frei()
	if k < quantenWKMin || k > quantenWKMax || k > len(frei) {
		return quantenWFehler("k = %d", k)
	}
	stellen, basen := quantenWAufforderung(K, K+":"+strconv.Itoa(e.Zaehl), blockhash, frei, k)
	if e.Benutzt == nil {
		e.Benutzt = map[int]bool{}
	}
	for _, i := range stellen {
		e.Benutzt[i] = true
	}
	e.Zaehl++
	e.Anfrage = &quantenWAnfrage{Von: von, Stellen: stellen, Basen: basen, Pfand: quantenWPfand, Hoehe: hoehe, EinloesenAn: einloesenAn}
	return nil
}

// quantenWAntworten: nur der Anfragende, nur in einem spaeteren Block.
func (e *quantenWNote) antworten(von string, bits []int, hoehe int64) error {
	a := e.Anfrage
	if a == nil || a.Antwort != nil {
		return quantenWFehler("keine offene Anfrage")
	}
	if von != a.Von {
		return quantenWFehler("nicht vom Anfragenden")
	}
	if hoehe <= a.Hoehe {
		return quantenWFehler("Antwort im selben Block wie die Anfrage")
	}
	if len(bits) != len(a.Stellen) {
		return quantenWFehler("Form der Antwort")
	}
	for _, b := range bits {
		if b != 0 && b != 1 {
			return quantenWFehler("Form der Antwort")
		}
	}
	a.Antwort, a.AntwortHoehe = append([]int(nil), bits...), hoehe
	return nil
}

// quantenWOffenlegen: jeder Knoten wertet aus und urteilt; Fehler = die
// Offenlegung ist ungueltig und aendert nichts.
func (e *quantenWNote) offenlegen(K string, offen []quantenWOffenStelle) (quantenWErgebnis, quantenWBuchung, error) {
	a := e.Anfrage
	if a == nil || a.Antwort == nil {
		return 0, quantenWBuchung{}, quantenWFehler("erst die Antwort")
	}
	treffer, fehler, err := quantenWAuswerten(K, e.N, e.T, len(e.Validatoren), e.Wurzel, a.Stellen, a.Basen, a.Antwort, offen)
	if err != nil {
		return 0, quantenWBuchung{}, err
	}
	u := quantenWUrteil(treffer, fehler)
	b := quantenWBuchung{}
	switch {
	case u == quantenWUnentschieden:
		b.PfandZurueckAn = a.Von
	case a.EinloesenAn != "":
		b.PfandZurueckAn = a.Von
		if u == quantenWEcht {
			b.WertAn = a.EinloesenAn
		} else {
			b.WertVerfaellt = true
		}
		e.Tot = true
	case u == quantenWEcht:
		b.PfandZurueckAn = a.Von
	default:
		b.PfandVerfaellt = true
		e.Fehl++
		if e.Fehl >= quantenWFehlMax {
			e.Gesperrt = true
		}
	}
	e.Anfrage = nil
	return u, b, nil
}

// quantenWFrist: zu Beginn jedes Blocks. Keine Antwort in der Frist: wie
// durchgefallen (bei Einloesen: Note tot). Antwort da, aber die Validatoren
// legen nicht offen: Pfand zurueck, kein Fehlversuch.
func (e *quantenWNote) frist(hoehe int64) (bool, quantenWBuchung) {
	a := e.Anfrage
	if a == nil {
		return false, quantenWBuchung{}
	}
	if a.Antwort == nil && hoehe > a.Hoehe+quantenWFristAntwort {
		b := quantenWBuchung{PfandVerfaellt: true}
		e.Fehl++
		if e.Fehl >= quantenWFehlMax {
			e.Gesperrt = true
		}
		if a.EinloesenAn != "" {
			b.WertVerfaellt = true
			e.Tot = true
		}
		e.Anfrage = nil
		return true, b
	}
	if a.Antwort != nil && hoehe > a.AntwortHoehe+quantenWFristOffen {
		e.Anfrage = nil
		return true, quantenWBuchung{PfandZurueckAn: a.Von}
	}
	return false, quantenWBuchung{}
}

// quantenWBenutzt: die verbrauchten Stellen, sortiert (fuer Ausgabe/Tests).
func (e *quantenWNote) benutztListe() []int {
	out := make([]int, 0, len(e.Benutzt))
	for i := range e.Benutzt {
		out = append(out, i)
	}
	sort.Ints(out)
	return out
}
