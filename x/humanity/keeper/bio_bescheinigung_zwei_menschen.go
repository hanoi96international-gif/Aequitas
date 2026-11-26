package keeper

// DER BETREIBER ALLEIN ERSCHAFFT KEINE MENSCHEN (docs/DEZENTRALITAET.md,
// Schritt 4; Auftrag des Betreibers 26.11.2026: das Versprechen "niemand kann
// Geld fuer erfundene Menschen erschaffen" soll gehalten werden).
//
// # WARUM
//
// Seit K1 prueft jeder Knoten die Bescheinigung einer Registrierung selbst
// (bio_bescheinigung_konsens.go): ein Blockerzeuger allein legt keine
// Menschen mehr an. Die Schluessel der Genesis -- Coordinator und
// Vergleichsdienst -- liegen aber beim Betreiber, und mit quorum 1 reichen
// sie. Wer beide haelt, kann allein beliebig viele "Menschen" bescheinigen,
// und jeder bekommt 1.000 AEQ und Grundeinkommen.
//
// # WAS AB bescheinigungZweiMenschenAb GILT
//
// Sobald das Pruefer-Register (verifier_register.go) zur Zeit der
// Bescheinigung ODER des Blocks mindestens zwei verschiedene Menschen hat,
// braucht jede Bescheinigung (Registrierung, Umzug) Unterschriften von
// mindestens zwei verschiedenen Menschen. Die Genesis-Schluessel und alle
// Gruender zaehlen zusammen als EIN Mensch (genesisMensch). Mit nur den
// Schluesseln des Betreibers wird die Registrierung abgewiesen -- ein zweiter
// Pruefer muss den Menschen mit bezeugt haben (B1b-Bindung).
//
// Solange es nur einen Pruefer gibt, aendert sich nichts: sonst stuende jede
// Registrierung still. Die Regel schaltet sich mit dem zweiten Pruefer
// AUTOMATISCH scharf; niemand muss dafuer etwas freigeben.
//
// # EHRLICHE GRENZEN
//
//   - Der Gruender kann allein den ersten Pruefer neben sich aufnehmen
//     (verifier_register.go). Wer der zweite Mensch ist, entscheidet also der
//     Betreiber -- oeffentlich auf der Kette. Ab dem dritten braucht jede
//     Aufnahme zwei Drittel.
//   - Tritt der zweite Pruefer aus, gilt wieder "einer reicht" (sonst stuende
//     jede Registrierung still).
//   - Zwei Pruefer, die zusammenarbeiten, koennen weiterhin Menschen
//     erfinden; der Gesichtsabgleich hat benannte Grenzen (Schwelle nicht
//     kalibriert, Deepfakes). "Niemand kann" heisst darum: kein Einzelner,
//     auch nicht der Betreiber.
//
// # REIHENFOLGE
//
// Die Zahl der Pruefer zur Zeit t steht fest, bevor ein Block, der t
// betrifft, nachgespielt wird (verifier_register.go, REIHENFOLGE). Gezaehlt
// wird zur Zeit der Bescheinigung und zur Zeit des Blocks; reicht eine der
// beiden Zahlen, gilt die Regel (eine spaetere Wiederaufnahme, die einen
// alten Eintrag ueberschreibt, kann sie so nicht nachtraeglich abschalten).

import "sync/atomic"

// bescheinigungZweiMenschenAbUnix: Stichtag der Regel (Kettenregel).
// Gesetzt vom Betreiber am 26.11.2026: 27.11.2026 12:00 UTC (im selben Deploy
// wie der Stichtag der Pfaender). Bis es einen zweiten Pruefer gibt, aendert
// sich damit nichts. Nie rueckwirkend: der Deploy-Waechter
// .github/neustart/zwei-menschen-stichtag-nicht-verpasst.sh haelt jeden
// Wechsel an, der nicht mindestens 30 Minuten in der Zukunft liegt oder einen
// laufenden Stichtag aendern wuerde.
const bescheinigungZweiMenschenAbUnix int64 = 1795780800

// bescheinigungZweiMenschenOverride: nur fuer Tests (0 = Stichtag oben).
var bescheinigungZweiMenschenOverride atomic.Int64

func bescheinigungZweiMenschenAb() int64 {
	if v := bescheinigungZweiMenschenOverride.Load(); v != 0 {
		return v
	}
	return bescheinigungZweiMenschenAbUnix
}

// menschenZu: wie viele verschiedene Menschen sind zur Zeit t Pruefer? Alle
// Gruender zaehlen zusammen als einer (wie genesisMensch).
func (r *verifierRegister) menschenZu(t int64) int {
	n, gruender := 0, false
	for _, m := range r.mitgliederZu(t) {
		if verifierIstGruender(m) {
			gruender = true
		} else {
			n++
		}
	}
	if gruender {
		n++
	}
	return n
}

// menschenZu fuer die Testquelle: unbekannt (-1) -- die Regel nimmt dann an,
// dass zwei Menschen noetig sind (fail-closed).
func (f prueferNurMitglied) menschenZu(int64) int { return -1 }

// zweiMenschenNoetig: braucht eine Bescheinigung mit Zeit bZeit in einem
// Block zur Zeit blockZeit Unterschriften von zwei verschiedenen Menschen?
// Ohne lesbares Register: ja (fail-closed).
func zweiMenschenNoetig(bZeit, blockZeit int64, pruefer prueferQuelle) bool {
	if blockZeit <= 0 || blockZeit < bescheinigungZweiMenschenAb() {
		return false
	}
	if pruefer == nil {
		return true
	}
	a, b := pruefer.menschenZu(bZeit), pruefer.menschenZu(blockZeit)
	return a < 0 || b < 0 || a >= 2 || b >= 2
}
