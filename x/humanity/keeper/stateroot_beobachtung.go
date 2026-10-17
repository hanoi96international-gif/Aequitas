package keeper

import (
	"sort"
	"strings"
	"sync"
)

// STATEROOT-ABWEICHUNGEN JE PRODUZENT -- sichtbar statt nur im Log.
//
// Offener Befund (docs/BETA_BEREITSCHAFT.md, Abschnitt 2): eine
// StateRoot-Abweichung wird geloggt, der Block angenommen. Dieser Stand
// aendert daran NICHTS am Konsens -- er macht die Abweichung fuer den
// Betreiber sichtbar, je Produzent, in /api/health/combined
// (stateroot_je_produzent).
//
// WARUM KEIN ABWEISEN, AUCH NICHT IM "EINDEUTIGEN" FALL (einzelner Elternblock
// = eigene Spitze, keine Geschwister, dieser Knoten ruht). Der StateRoot
// eines Blocks ist KEINE Funktion seiner Vorfahren und seiner
// Transaktionen. ProduceBlock hasht dag.state.StateRoot(), also den
// aktuellen Zustand des Produzenten -- und der traegt jede dort angenommene
// Ueberweisung ab dem Moment der Annahme (TransferAtomic wendet sofort an,
// der Block buendelt spaeter). Der Root laeuft sogar nebenlaeufig zu
// LoadPendingTxs (block.go, "CADENCE FIX"). Damit enthaelt der Root eines
// ehrlichen Blocks Ueberweisungen, die in keinem Block stehen, sobald der
// Produzent:
//   - mehr angenommen hat, als in einen Block passt (am 15.09.2026: 1,7 Mio.
//     angenommen, 600.000 verblockt -- zwanzig Minuten Vorlauf),
//   - zwischen dem Laden der Pendings und dem Hashen etwas angenommen hat,
//   - ueber /api/... Registrierungen, Tauschgeschaefte usw. synchron
//     angewendet hat, die erst im naechsten Block stehen.
// Kein Pruefer kann das aus dem Block erkennen: der Block traegt keine
// Angabe, ob der Ausgangskorb des Produzenten beim Hashen leer war. Ein
// Abweisen im "eindeutigen" Fall wiese also gerade die Bloecke eines
// belasteten, ehrlichen Produzenten ab -- und mit ihnen jeden Nachfahren
// (Waisen-Kaskade, block.go an der Vergleichsstelle). Das ist der
// Sync-Stillstand, den der Befund vermeiden will. Ein Stichtag dafuer waere
// ein Schalter, den der Betreiber nie gefahrlos umlegen koennte.
//
// Was die Abweichung trotzdem leistet: der Zaehler "echt" (dieser Knoten
// ruht, hat an der Hoehe nicht selbst produziert) und vor allem die QUOTE
// je Produzent. Ein Produzent, dessen Bloecke auch in der Ruhe dauerhaft
// abweichen, faellt hier auf; den Kontostand-Vergleich in der Ruhe macht
// der Divergenz-Waechter (divergenz_waechter.go). Abweisen wird erst sicher,
// wenn der Root nur noch aus angewendeten Bloecken entsteht (Root von vor D
// Bloecken, docs/KONSENS_UND_AUSFUEHRUNG_2026.md) -- das ist eine
// Konsensaenderung, kein Schalter.
//
// GRENZEN. Hoechstens stateRootProduzentenMax Eintraege. Wer danach kommt,
// landet im Sammeltopf "uebrige" -- bestehende Eintraege werden nie
// verdraengt, damit ein Schwall neuer Produzenten (offene Produzentenliste
// ohne AUTHORIZED_VALIDATORS) keinen auffaelligen Produzenten aus der
// Anzeige schieben kann. Adresse und Root-Praefixe werden auf feste Laenge
// und Zeichen gekappt; sie kommen aus einem fremden, wenn auch signierten
// Block.

const (
	stateRootProduzentenMax = 64
	stateRootPraefixLaenge  = 16
	stateRootAdresseMax     = 42
)

type stateRootProduzentStand struct {
	Vergleiche  uint64 `json:"vergleiche"`
	Gleich      uint64 `json:"gleich"`
	Abweichend  uint64 `json:"abweichend"`
	Echt        uint64 `json:"echt"`
	Geschwister uint64 `json:"geschwisterbedingt"`
	// Letzte Abweichung: Zeit, Hoehe und die ersten Zeichen beider Roots.
	LetzteAbweichungUnix  int64  `json:"letzte_abweichung_unix,omitempty"`
	LetzteAbweichungHoehe int64  `json:"letzte_abweichung_hoehe,omitempty"`
	LetzterRootBehauptet  string `json:"letzter_root_behauptet,omitempty"`
	LetzterRootLokal      string `json:"letzter_root_lokal,omitempty"`
}

type stateRootBeobachtung struct {
	mu      sync.Mutex
	je      map[string]*stateRootProduzentStand
	uebrige stateRootProduzentStand
	// Wie viele Vergleiche in "uebrige" gingen, weil die Tabelle voll war.
	verdichtet uint64
}

// stateRootSchluessel normalisiert die Produzentenadresse: Kleinschreibung
// (ein Produzent darf nicht mit Gross-/Kleinvarianten mehrere Eintraege
// erzeugen) und feste Hoechstlaenge.
func stateRootSchluessel(p string) string {
	p = strings.ToLower(strings.TrimSpace(p))
	if len(p) > stateRootAdresseMax {
		p = p[:stateRootAdresseMax]
	}
	if p == "" {
		return "(leer)"
	}
	return p
}

// stateRootPraefix kappt einen Root auf wenige Hex-Zeichen und laesst nur
// [0-9a-fx] durch -- der Wert stammt aus einem fremden Block.
func stateRootPraefix(r string) string {
	var b strings.Builder
	for i := 0; i < len(r) && b.Len() < stateRootPraefixLaenge; i++ {
		c := r[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F', c == 'x':
			b.WriteByte(c)
		}
	}
	return strings.ToLower(b.String())
}

// erfasse zaehlt einen Vergleich. Nur Zaehler -- entscheidet nichts.
func (o *stateRootBeobachtung) erfasse(produzent string, hoehe int64, gleich, geschwister bool, behauptet, lokal string, jetzt int64) {
	k := stateRootSchluessel(produzent)
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.je == nil {
		o.je = make(map[string]*stateRootProduzentStand)
	}
	s, da := o.je[k]
	if !da {
		if len(o.je) < stateRootProduzentenMax {
			s = &stateRootProduzentStand{}
			o.je[k] = s
		} else {
			s = &o.uebrige
			o.verdichtet++
		}
	}
	s.Vergleiche++
	if gleich {
		s.Gleich++
		return
	}
	s.Abweichend++
	if geschwister {
		s.Geschwister++
	} else {
		s.Echt++
	}
	s.LetzteAbweichungUnix = jetzt
	s.LetzteAbweichungHoehe = hoehe
	s.LetzterRootBehauptet = stateRootPraefix(behauptet)
	s.LetzterRootLokal = stateRootPraefix(lokal)
}

// stand liefert die Anzeige: hoechstens stateRootProduzentenMax Eintraege
// plus Sammeltopf, nach Adresse sortiert (stabile Ausgabe).
func (o *stateRootBeobachtung) stand() map[string]interface{} {
	o.mu.Lock()
	defer o.mu.Unlock()
	keys := make([]string, 0, len(o.je))
	for k := range o.je {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	liste := make([]map[string]interface{}, 0, len(keys))
	for _, k := range keys {
		s := *o.je[k]
		liste = append(liste, map[string]interface{}{"produzent": k, "stand": s})
	}
	return map[string]interface{}{
		"bedeutung": "Kumulierte StateRoot-Vergleiche je Produzent seit Prozessstart (nicht zurueckgesetzt). " +
			"Nur Anzeige: kein Block wird deswegen abgewiesen, weil der Root eines ehrlichen Produzenten " +
			"unter Last auch noch nicht verblockte Ueberweisungen enthaelt. Auffaellig ist ein Produzent, " +
			"dessen 'echt' im Verhaeltnis zu 'vergleiche' dauerhaft hoch bleibt, waehrend die anderen gleich sind.",
		"grenze":      stateRootProduzentenMax,
		"produzenten": liste,
		"uebrige":     o.uebrige,
		"verdichtet":  o.verdichtet,
	}
}

// StateRootJeProduzent: fuer /api/health/combined.
func (dag *BlockDAG) StateRootJeProduzent() map[string]interface{} {
	return dag.srBeobachtung.stand()
}

// stateRootZaehlerPlatz: duerfen die alten Zaehler-Maps (stateRootMismatches
// und Geschwister) einen Eintrag fuer k bekommen? Bestehende immer, neue nur
// bis stateRootProduzentenMax. Ohne diese Grenze wuchsen sie mit jeder
// Produzentenadresse, die je einen Block geliefert hat. Aufrufer haelt
// stateRootMismatchesMu.
func stateRootZaehlerPlatz(m map[string]int, k string) bool {
	if _, da := m[k]; da {
		return true
	}
	return len(m) < stateRootProduzentenMax
}
