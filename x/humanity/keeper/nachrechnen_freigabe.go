package keeper

import (
	"context"
	"strings"
)

// Staffel-Freigaben nachrechnen (Audit 2026-09-29, K-2, Schritt 3).
//
// Der Erzeuger (grantReleasesLocked) gibt je Tagesrunde jedem Menschen mit
// offenem Staffel-Rest hoechstens frei, was staffelFreigabeHoechstens zur
// Rundenzeit erlaubt: mit gueltigem Lebenszeichen die Tagesrate (800/30 AEQ),
// ab Tag 30 den ganzen Rest, ausser ein Einspruch haelt ihn an
// (staffel_einspruch.go). Beim Nachspielen begrenzte
// applyGrantReleaseDeltaLocked den Betrag nur auf den offenen Rest: Ein
// Produzent konnte den ganzen Rest auf einmal freigeben, ohne Lebenszeichen
// freigeben, oder in einer Runde mehrmals.
//
//   - freigabe_zu_hoch:         Betrag ueber dem, was zur Rundenzeit erlaubt ist
//   - freigabe_ohne_lebenszeichen: vor Tag 30 ohne gueltige zweite Pruefung
//   - freigabe_angehalten:      ab Tag 30, aber ein Einspruch haelt an
//   - freigabe_rundenzeit:      Rundenzeit mehr als 60 s nach dem Block
//   - freigabe_doppelt:         zweite Freigabe fuer denselben Menschen,
//     bevor eine Rundenmarke die Runde beendet hat
//
// Die Rundenzeit traegt die Freigabe selbst (DistributionAt); Freigaben ohne
// sie (vor dieser Regel erzeugt, wegen der Aktivierung 2100 keine) rechnen
// mit der Blockzeit.
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
	t := blockZeit
	if tx.DistributionAt > 0 {
		if tx.DistributionAt > blockZeit+60 {
			return nachrechnenAbweichung("freigabe_rundenzeit", blockZeit,
				"%s: Rundenzeit %d, Blockzeit %d", kurzAdresse(wallet), tx.DistributionAt, blockZeit)
		}
		t = tx.DistributionAt
	}
	cs.ensureAccountLoadedCtx(context.Background(), wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok || !acc.IsHuman || acc.GrantStagedRest <= 0 {
		// Kein Mensch: lehnt das Nachspielen ab; kein Rest: dort Leerlauf.
		// Hoechstens die Tagesrate gilt trotzdem -- mehr gibt kein Erzeuger
		// einem Konto ohne offenen Rest.
		if NewDecimal(tx.Amount).Micro() > NewDecimal(grantStaffelTagesrate()).Micro()+1 {
			return nachrechnenAbweichung("freigabe_zu_hoch", blockZeit,
				"%s: %.6f AEQ ohne offenen Rest", kurzAdresse(wallet), tx.Amount)
		}
		return nil
	}
	erlaubt, regel := staffelFreigabeHoechstens(acc, t)
	if erlaubt <= 0 {
		return nachrechnenAbweichung(regel, blockZeit,
			"%s: zur Rundenzeit %d keine Freigabe erlaubt", kurzAdresse(wallet), t)
	}
	if NewDecimal(tx.Amount).Micro() > NewDecimal(erlaubt).Micro()+1 {
		if err := nachrechnenAbweichung("freigabe_zu_hoch", blockZeit,
			"%s: %.6f AEQ, erlaubt %.6f", kurzAdresse(wallet), tx.Amount, erlaubt); err != nil {
			return err
		}
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
