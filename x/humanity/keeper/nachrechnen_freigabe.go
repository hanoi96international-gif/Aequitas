package keeper

import (
	"context"
	"strings"
)

// Staffel-Freigaben nachrechnen (Audit 2026-09-29, K-2, Schritt 3).
//
// Der Erzeuger (grantReleasesLocked) gibt je Tagesrunde jedem Menschen mit
// offenem Staffel-Rest UND erneuertem Lebenszeichen hoechstens die
// Tagesrate frei (800/30 AEQ, am letzten Tag den Rest). Beim Nachspielen
// begrenzte applyGrantReleaseDeltaLocked den Betrag nur auf den offenen
// Rest: Ein Produzent konnte den ganzen Rest auf einmal freigeben, ohne
// Lebenszeichen freigeben, oder in einer Runde mehrmals.
//
//   - freigabe_zu_hoch:         Betrag ueber der Tagesrate
//   - freigabe_ohne_lebenszeichen: LivenessRenewedAt == 0
//   - freigabe_doppelt:         zweite Freigabe fuer denselben Menschen,
//     bevor eine Rundenmarke die Runde beendet hat
//
// Die Menge der schon Freigegebenen enthaelt nur Menschen mit offenem Rest
// und Lebenszeichen -- begrenzt durch den Zustand. Rueckrollen wie in
// nachrechnen_ubi.go ueber die laufende Nummer. Nach einem Neustart ist sie
// leer: es wird hoechstens zu wenig gemeldet, nie zu viel.

type freigabeRunde struct {
	n           int64
	freigegeben map[string]int64 // Wallet -> laufende Nummer
}

func (r freigabeRunde) zurueck() freigabeRunde {
	for k, nr := range r.freigegeben {
		if nr > r.n {
			delete(r.freigegeben, k)
		}
	}
	return r
}

func (cs *ChainState) nachrechnenFreigabeLocked(tx *Transaction, blockZeit int64) error {
	if !stagedGrantAktiv(blockZeit) {
		return nil // vor der Aktivierung ist grant_release Leerlauf
	}
	wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
	if NewDecimal(tx.Amount).Micro() > NewDecimal(grantStaffelTagesrate()).Micro()+1 {
		if err := nachrechnenAbweichung("freigabe_zu_hoch", blockZeit,
			"%s: %.6f AEQ, die Tagesrate ist %.6f", kurzAdresse(wallet), tx.Amount, grantStaffelTagesrate()); err != nil {
			return err
		}
	}
	cs.ensureAccountLoadedCtx(context.Background(), wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok || !acc.IsHuman || acc.GrantStagedRest <= 0 {
		return nil // kein Mensch: lehnt das Nachspielen ab; kein Rest: dort Leerlauf
	}
	if acc.LivenessRenewedAt <= 0 {
		return nachrechnenAbweichung("freigabe_ohne_lebenszeichen", blockZeit,
			"%s hat kein erneuertes Lebenszeichen", kurzAdresse(wallet))
	}
	r := &cs.freigaben
	if r.freigegeben == nil {
		r.freigegeben = make(map[string]int64)
	}
	r.n++
	if _, schon := r.freigegeben[wallet]; schon {
		return nachrechnenAbweichung("freigabe_doppelt", blockZeit,
			"%s bekommt in dieser Runde eine zweite Freigabe", kurzAdresse(wallet))
	}
	r.freigegeben[wallet] = r.n
	return nil
}

// nachrechnenFreigabeRundeEndeLocked: an jeder (nicht uebersprungenen)
// Rundenmarke.
func (cs *ChainState) nachrechnenFreigabeRundeEndeLocked() {
	cs.freigaben = freigabeRunde{}
}
