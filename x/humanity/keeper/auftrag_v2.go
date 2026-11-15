package keeper

import (
	"fmt"
	"math"
	"strings"
)

// Audit B15 (14.11.2026): Tausch und Liquiditaet, Nachrichtenform 2.
//
// Die bisherige Nachricht ("Aequitas Swap: <richtung> <betrag> nonce ts")
// trug weder die Kette noch die Wallet noch die Schlupfgrenze. Wer die
// Anfrage unterwegs aendern kann (der weiterleitende Knoten, die Leitung
// zwischen Validatoren), konnte min_amount_out auf 0 setzen, und dieselbe
// Unterschrift galt auf jeder Kette mit derselben Nachricht.
//
// Form 2 bindet alles, was den Auftrag ausmacht:
//
//	aequitas-tausch-v2|chain:1926|<wallet>|<richtung>|<betrag>|min:<min>|nonce:<n>|ts:<t>
//	aequitas-liquiditaet-v2|chain:1926|<wallet>|<aeq>|<tusd>|nonce:<n>|ts:<t>
//	aequitas-liquiditaet-ab-v2|chain:1926|<wallet>|<anteile>|nonce:<n>|ts:<t>
//
// Jeder Knoten prueft beim Nachspielen (auftragsNachricht): die Unterschrift
// ueber genau diese Nachricht, und beim Tausch, dass der Block die
// unterschriebene Grenze einhaelt -- direkt (AmountOut >= min) oder ueber
// den Vorbehalt (MinOut == min, die Ausfuehrung prueft dann das Ergebnis).
//
// Form 1 bleibt fuer die Bloecke, die es schon gibt, gueltig: eine
// Unterschrift der Form 1 kann nur der Auftraggeber selbst erzeugen, und das
// tut ab der App mit Form 2 niemand mehr. Die ANNAHME nimmt Form 1 nur bis
// auftragForm1AnnahmeBisUnix an (fail-closed danach).
const (
	auftragForm2 = 2
	// 01.12.2026 00:00 UTC: bis dahin hat jede App die Form 2.
	auftragForm1AnnahmeBisUnix int64 = 1796083200
)

func tauschNachrichtV2(wallet, richtung string, betrag, min float64, nonce, ts int64) string {
	return fmt.Sprintf("aequitas-tausch-v2|chain:%d|%s|%s|%.8f|min:%.8f|nonce:%d|ts:%d",
		eingefroreneChainID, strings.ToLower(wallet), richtung, betrag, min, nonce, ts)
}

func liquiditaetNachrichtV2(wallet string, aeq, tusd float64, nonce, ts int64) string {
	return fmt.Sprintf("aequitas-liquiditaet-v2|chain:%d|%s|%.8f|%.8f|nonce:%d|ts:%d",
		eingefroreneChainID, strings.ToLower(wallet), aeq, tusd, nonce, ts)
}

func liquiditaetAbNachrichtV2(wallet string, anteile float64, nonce, ts int64) string {
	return fmt.Sprintf("aequitas-liquiditaet-ab-v2|chain:%d|%s|%.8f|nonce:%d|ts:%d",
		eingefroreneChainID, strings.ToLower(wallet), anteile, nonce, ts)
}

// auftragFormAnnehmbar: darf die Annahme einen Auftrag dieser Form jetzt
// noch annehmen? Unbekannte Formen nie.
func auftragFormAnnehmbar(form int, jetzt int64) error {
	switch form {
	case auftragForm2:
		return nil
	case 0, 1:
		if jetzt < auftragForm1AnnahmeBisUnix {
			return nil
		}
		return fmt.Errorf("signature format outdated: please update the app (Nachrichtenform 2 noetig)")
	}
	return fmt.Errorf("unknown signature format %d", form)
}

// gueltigeMindestgrenze: min_amount_out muss endlich und >= 0 sein.
func gueltigeMindestgrenze(min float64) bool {
	return !math.IsNaN(min) && !math.IsInf(min, 0) && min >= 0
}
