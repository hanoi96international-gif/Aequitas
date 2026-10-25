package keeper

import (
	"context"
	"strings"
)

// Ueberweisungen beim Nachspielen (Audit 2026-10-21, K2 und M1).
//
// Die Unterschrift und die Nonce prueft jeder Knoten seit der Pflicht zur
// Rohform selbst (signierte_ueberweisung.go). Zwei Werte stehen aber nicht in
// der Unterschrift und kamen bis hierhin ungeprueft vom Erzeuger:
//
//   - die GEBUEHR (tx.Gebuehr): obendrauf vom Absender abgebucht. Ein
//     boesartiger Erzeuger konnte sie auf das ganze Restguthaben setzen --
//     der Absender hat nur den Betrag unterschrieben, nicht die Gebuehr.
//   - der BETRAG unter einem Mikro-AEQ: rundet auf null, bewegt nichts und
//     fuellt trotzdem Bloecke (betrag_untergrenze.go).
//
// Geprueft wird bewusst nur die OBERGRENZE der Gebuehr, nicht ihr genauer
// Wert. Der genaue Wert haengt am monatlichen Freibetrag eines Menschen
// (Buchfuehrung, wirtschaft.go), und die fehlt einem Knoten nach einem
// Snapshot-Import -- eine genaue Pruefung liesse ihn dann ehrliche Bloecke
// abweisen. Die Obergrenze haengt nur am Betrag und an den Kontoarten
// (Register, Konsens): mehr als 0,1 % des Betrags gibt es nie, und ein
// Unternehmen zahlt auf Loehne an Menschen nichts. Zu wenig Gebuehr schadet
// nur dem Grundeinkommen, nie einem Konto.
func (cs *ChainState) nachrechnenUeberweisungLocked(tx *Transaction, wallet string, blockZeit int64) error {
	if tx.Amount > 0 && NewDecimal(tx.Amount) <= 0 {
		return nachrechnenAbweichung("betrag_unter_mikro", blockZeit,
			"%s: Betrag %g AEQ rundet auf null", kurzAdresse(wallet), tx.Amount)
	}
	if tx.Gebuehr <= 0 {
		return nil
	}
	jetzt := buchZeitBeimNachspielen(tx.BuchAt, blockZeit)
	if !wirtschaftAktiv(jetzt) {
		return nil // davor galt der Aufschlag nach Guthaben (ueberweisungsGebuehrFuer)
	}
	// Ab dem Gleichgewicht (wirtschaft3.go, e) gibt es keine Gebuehr. Der
	// Buchungsaugenblick entscheidet, damit eine kurz vorher angenommene
	// Ueberweisung ihre Gebuehr behalten darf; er liegt hoechstens eine
	// Woche vor dem Block (buchZeitBeimNachspielen).
	if gleichgewichtAktiv(jetzt) {
		return nachrechnenAbweichung("gebuehr_nach_gleichgewicht", blockZeit,
			"%s -> %s: Gebuehr %.6f im Block, seit dem Gleichgewicht gibt es keine",
			kurzAdresse(wallet), kurzAdresse(tx.To), tx.Gebuehr)
	}
	to := strings.ToLower(strings.TrimSpace(tx.To))
	cs.ensureAccountLoadedCtx(context.Background(), wallet)
	cs.ensureAccountLoadedCtx(context.Background(), to)
	vonMensch, zuMensch := false, false
	if acc, ok := cs.accounts.Get(wallet); ok {
		vonMensch = acc.IsHuman
	}
	if acc, ok := cs.accounts.Get(to); ok {
		zuMensch = acc.IsHuman
	}
	fromArt := cs.kontoartVon(wallet, vonMensch)
	toArt := cs.kontoartVon(to, zuMensch)
	hoechstens := grundGebuehr(tx.Amount)
	if fromArt == artUnternehmen && toArt == artMensch {
		hoechstens = 0
	}
	if tx.Gebuehr > hoechstens+1e-6 {
		return nachrechnenAbweichung("ueberweisung_gebuehr", blockZeit,
			"%s -> %s: Gebuehr %.6f im Block, hoechstens %.6f", kurzAdresse(wallet), kurzAdresse(to), tx.Gebuehr, hoechstens)
	}
	return nil
}

// lohnGebuehrPraefix kuerzt einen Lauf fuer den schnellen Pfad
// (replay_parallel.go) vor die erste Ueberweisung mit Gebuehr, deren
// Absender ein Unternehmen ist (Sicherheitspruefung des Diffs zum Audit
// 2026-10-21, F1). Fuer sie gilt die Lohnregel (Unternehmen an Mensch:
// Gebuehr null), und die prueft nur nachrechnenUeberweisungLocked im
// seriellen Pfad. collectDisjointTransferBatch ist zustandslos und sah nur
// die 0,1 %-Obergrenze; weil fast jede Ueberweisung den schnellen Pfad nimmt,
// erreichte eine Lohnzahlung den seriellen nie -- ein Erzeuger konnte jedem
// Lohn eine nicht unterschriebene Gebuehr bis 0,1 % aufschlagen.
//
// Bewusst grob: jede Gebuehr eines Unternehmens geht seriell, nicht nur die
// an Menschen. Die Kontoart des Empfaengers braucht das Konto (Datenbank),
// die des Absenders nur das Register im Speicher. Ein Lauf enthaelt nur
// Ueberweisungen, und die aendern das Register nicht -- die Antwort steht
// fuer den ganzen Lauf fest.
//
// Ab dem Gleichgewicht (wirtschaft3.go, e) geht JEDE Ueberweisung mit
// Gebuehr seriell: dort lehnt nachrechnenUeberweisungLocked sie ab
// (gebuehr_nach_gleichgewicht). Der schnelle Pfad kennt nur die
// 0,1 %-Obergrenze und haette sie gebucht.
func (cs *ChainState) lohnGebuehrPraefix(batch []Transaction, blockZeit int64) []Transaction {
	w := cs.wirt()
	for i, tx := range batch {
		if tx.Gebuehr <= 0 {
			continue
		}
		if gleichgewichtAktiv(buchZeitBeimNachspielen(tx.BuchAt, blockZeit)) ||
			w.istUnternehmen(strings.ToLower(strings.TrimSpace(tx.Wallet))) {
			return batch[:i]
		}
	}
	return batch
}
