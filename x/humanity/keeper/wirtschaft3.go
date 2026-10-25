package keeper

// Wirtschaft, dritte Stufe: "Gleichgewicht" (docs/WIRTSCHAFT_ENDFASSUNG.md,
// Abschnitt 10, Schritt 1). SCHLAFEND: gleichgewichtAktivAbUnix liegt im
// Jahr 2100. Das echte Datum erst nach der Entscheidung des Betreibers und
// nachdem alle Knoten diesen Stand haben (wie wirtschaftAktivAbUnix).
//
// Ab der Aktivierung, entschieden an der Rundenzeit bzw. am
// Buchungsaugenblick, nie an der Uhr des nachspielenden Knotens:
//
//   a  Umlauf (umlaufBetragRoh): jedes Konto -- Mensch, Unternehmen, freie
//      Adresse -- zahlt gleichgewichtSatzMonat auf das ganze Guthaben samt
//      LP-Anteilen. Kein Freibetrag, keine Stufen, kein Umsatz. Systemadressen
//      zahlen nichts.
//   b  Kandidaten der Runde (umlaufLocked): jedes Konto mit Guthaben oder
//      LP-Anteilen, aus der Datenbank UND dem Speicher (umlaufKandidatenLocked).
//   c  pruefeUmlaufLocked rechnet jeden Betrag nach, auch bei Unternehmen --
//      die Rechnung braucht keine Buchfuehrung mehr.
//   d  Vollstaendigkeit (nachrechnen_umlauf.go): kein Konto, das schuldet,
//      darf in der Runde fehlen; keines zweimal.
//   e  Keine Ueberweisungsgebuehr (gebuehrMitWirtschaft). Eine Gebuehr in
//      einer Ueberweisung nach der Aktivierung ist ein Regelbruch
//      (gebuehr_nach_gleichgewicht, nachrechnen_ueberweisung.go); der
//      schnelle Pfad gibt sie an den seriellen ab (lohnGebuehrPraefix).
//
// Unveraendert: Ausstiegsabgabe, Vermoegensgrenze, Grenze freier Adressen,
// Verteilung des Topfs an die Menschen mit gueltigem Lebensnachweis. Die
// Buchfuehrung (Monatszaehler, Umsatz, kundschaft.go) laeuft weiter; keine
// Regel des Gleichgewichts liest sie, abschalten laesst sie sich spaeter
// (Endfassung, Punkt f).
//
// WARUM DAS KEINE LUECKE OEFFNET. Eine Regel fuer jedes Konto: Geld zwischen
// Kontoarten zu verschieben aendert die Summe der Abgaben nicht -- das
// Etikett "Unternehmen" oder "freie Adresse" bringt nichts, und nichts muss
// geprueft werden, um das sicherzustellen (Test
// TestGleichgewicht_VerschiebenBringtNichtsInDerKette).

import (
	"sort"
	"strings"
	"sync/atomic"
)

// gleichgewichtAktivAbUnix: 2100-01-01 00:00 UTC -- schlafend.
const gleichgewichtAktivAbUnix int64 = 4102444800

// gleichgewichtSatzMonat: 0,5 % im Monat auf jedes Guthaben -- der heutige
// Satz der Menschen, jetzt fuer alle und ab dem ersten AEQ.
const gleichgewichtSatzMonat = 0.005

// gleichgewichtAktivOverride: nur fuer Tests (0 = Konstante gilt).
var gleichgewichtAktivOverride atomic.Int64

// gleichgewichtAktiv: gilt das Gleichgewicht zu unix? Nur zusammen mit den
// Wirtschaftsregeln -- ohne sie gibt es keinen Umlauf, den es ersetzen
// koennte.
func gleichgewichtAktiv(unix int64) bool {
	if !wirtschaftAktiv(unix) {
		return false
	}
	a := gleichgewichtAktivAbUnix
	if o := gleichgewichtAktivOverride.Load(); o != 0 {
		a = o
	}
	return unix >= a
}

// gleichgewichtAb: ab wann das Gleichgewicht gilt (fuer block_tauglich.go).
func gleichgewichtAb() int64 {
	a := gleichgewichtAktivAbUnix
	if o := gleichgewichtAktivOverride.Load(); o != 0 {
		a = o
	}
	return max(a, aktivAb())
}

// gleichgewichtUmlauf: was ein Konto dieser Art bei diesem Stand fuer den
// Anteil eines Monats schuldet, bei satz je Monat (gleichgewichtSatzZuLocked:
// der Satz, der zur Rundenzeit gilt, satz_plan.go). Dieselbe Formel wie
// GleichgewichtParameter.Umlauf im Modell (Test
// TestWirtschaftsmodell_FormelnGleichDerKette).
func gleichgewichtUmlauf(art kontoart, stand, anteil, satz float64) float64 {
	if art == artSystem || stand <= 0 || anteil <= 0 {
		return 0
	}
	return round6(stand * satz * anteil)
}

// umlaufKandidatenLocked: die Konten, die in der Runde zu at schulden
// koennten, sortiert, ohne Systemadressen. Vor dem Gleichgewicht die
// bisherige Auswahl (Menschen ueber dem Sparfreibetrag, Nicht-Menschen mit
// Guthaben, LP-Halter) allein aus der Datenbank -- unveraendert, damit sich
// keine laufende Runde aendert.
//
// Ab dem Gleichgewicht jedes Konto mit Guthaben oder LP-Anteilen, aus der
// Datenbank UND aus dem Speicher: ein Konto, das sein erstes Geld ueber den
// WAL-Schnellpfad bekommen hat, steht im Speicher, aber vielleicht noch
// nicht in der Datenbank (transfer_wal.go schreibt nach). Beim Nachspielen
// ist die Datenbank aktuell -- ohne den Speicher saehe der Nachspielende ein
// Konto, das der Erzeuger nicht sah, und die Vollstaendigkeitspruefung
// (nachrechnen_umlauf.go) meldete eine Luecke, die keine ist. Konten, die
// nur in der Datenbank Guthaben haben, im Speicher aber nicht, fallen mit
// Stand 0 heraus (umlaufBetrag). Caller haelt cs.mu.
func (cs *ChainState) umlaufKandidatenLocked(q sqlExecutor, at int64) ([]string, error) {
	gg := gleichgewichtAktiv(at)
	kandidaten := map[string]bool{}
	if cs.db != nil {
		schwelle := menschSparFreibetrag
		if gg {
			schwelle = 0
		}
		// lp_shares > 0: ab umlaufMitLPAbUnix zaehlen LP-Anteile mit
		// (umlaufStandLocked) -- wer sein Geld im Pool haelt, stand sonst
		// gar nicht erst auf der Liste. Frueher mitgelesen schadet nicht:
		// umlaufBetrag entscheidet.
		rows, err := q.Query(
			`SELECT lower(address) FROM chain_accounts
			 WHERE (is_human = true AND balance > $1) OR (is_human = false AND balance > 0) OR lp_shares > 0
			 ORDER BY 1`, schwelle)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var a string
			if rows.Scan(&a) == nil {
				kandidaten[a] = true
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	if cs.db == nil || gg {
		cs.accounts.Range(func(a string, acc *AccountState) bool {
			if acc.Balance.Float() > 0 || acc.LPShares > 0 {
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
	return adressen, nil
}

// gleichgewichtStand: fuer /api/wirtschaft/regeln.
func (cs *ChainState) gleichgewichtStand(jetzt int64) map[string]interface{} {
	g := StandardGleichgewicht()
	satz := map[string]interface{}{}
	if cs != nil {
		satz = cs.SatzStand(jetzt)
		if bp, ok := satz["bp_jetzt"].(int64); ok {
			g.SatzMonat = float64(bp) / 10_000
		}
	}
	return map[string]interface{}{
		"aktiv":                        gleichgewichtAktiv(jetzt),
		"aktiv_ab":                     gleichgewichtAktivAbUnix,
		"umlauf_prozent_monat":         g.SatzMonat * 100,
		"satz":                         satz,
		"gilt_fuer":                    "jedes Konto ab dem ersten AEQ, Menschen, Unternehmen und freie Adressen gleich",
		"ueberweisung_prozent":         0,
		"dividende_je_mensch_monat":    g.DividendeJeMensch(),
		"netto_je_mensch_monat_formel": "Satz x (fairer Anteil - Guthaben)",
		"halbwertzeit_monate":          round6(g.HalbwertMonate()),
		"vollstaendigkeit":             umlaufRundeStand(),
		"entwurf":                      "docs/WIRTSCHAFT_ENDFASSUNG.md, Abschnitt 10",
	}
}
