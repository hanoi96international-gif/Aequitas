package keeper

import (
	"context"
	"database/sql"
)

// Ausfuehrung eines Vorbehalts nachrechnen (K-2, Analyse 03.10.2026).
//
// # WARUM
//
// Stufe 2 teilt Tausch und Liquiditaet in zwei Schritte (vorbehalt.go): der
// Zustaendige des Kontos nimmt den unterschriebenen Auftrag an und legt den
// Einsatz auf ein Vorbehaltskonto; der Leiter fuehrt ihn spaeter aus. Den
// ERSTEN Schritt prueft jeder Knoten wie einen direkten Tausch (Nachweis,
// Auftrags-Nonce). Den ZWEITEN spielte jeder Knoten "genau wie getragen"
// nach -- ohne die Regeln, die fuer den direkten Tausch gelten:
//
//   - das Tauschergebnis (AmountOut) und die LP-Anteile kamen ungeprueft aus
//     dem Block. nachrechnenTxLocked kannte nur swap_* und add_liquidity als
//     Typ; die Ausfuehrung traegt "vorbehalt_ausfuehrung". Ein Leiter haette
//     beliebig viel tUSD/AEQ aus dem Pool auszahlen koennen.
//   - der Betrag war nicht an den unterschriebenen gebunden: der Leiter
//     konnte mehr vom Guthaben des Auftraggebers tauschen, als dieser
//     beauftragt hatte, oder den Mindestbetrag (MinOut) unterschreiten.
//
// # WAS
//
// Der unterschriebene Auftrag liegt bei jedem Knoten (vorbehalte_offen,
// angelegt beim Nachspielen des ersten Schritts). Dagegen:
//
//   - vorbehalt_unbekannt: kein offener Vorbehalt unter dieser Referenz.
//   - vorbehalt_fremd:     Wallet oder Art weichen vom Auftrag ab.
//   - vorbehalt_betrag:    Betrag (bei der Einlage beide) weicht vom
//     unterschriebenen ab.
//   - vorbehalt_mindestbetrag: Tauschergebnis unter MinOut.
//   - tausch_ergebnis / lp_anteile: dieselbe Rechnung wie beim direkten
//     Auftrag (nachrechnenPoolAuftragLocked).
//
// Eine Erstattung (der Auftrag scheiterte, alles geht zurueck) wird nur auf
// Referenz, Wallet und Art geprueft: sie bucht nichts als die Rueckgabe, und
// rueckbuchenLocked begrenzt die auf den Inhalt des Vorbehaltskontos.
//
// Gelesen wird aus der Datenbank in der laufenden Transaktion, nicht aus
// cs.vorbehalte: die Tabelle ist die einzige Quelle, wo es eine Datenbank
// gibt (#284). Die Karte im Speicher gilt nur ohne Datenbank und geht beim
// Zurueckrollen mit (vorbehaltSicherung).

type vorbehaltAuftrag struct {
	wallet, art     string
	betrag, betrag2 float64
	minOut          float64
}

// vorbehaltAuftragLocked: der unterschriebene Auftrag zu einem
// Vorbehaltskonto, so wie dieser Knoten ihn kennt.
func (cs *ChainState) vorbehaltAuftragLocked(konto string) (vorbehaltAuftrag, bool, error) {
	var a vorbehaltAuftrag
	if cs.db == nil {
		offeneVorbehalteMu.Lock()
		o, ok := cs.vorbehalte[konto]
		offeneVorbehalteMu.Unlock()
		return vorbehaltAuftrag{o.wallet, o.art, o.betrag, o.betrag2, o.minOut}, ok, nil
	}
	err := cs.dbExecCtx(context.Background()).QueryRow(
		`SELECT wallet, art, betrag, betrag2, min_out FROM vorbehalte_offen WHERE konto = $1`, konto,
	).Scan(&a.wallet, &a.art, &a.betrag, &a.betrag2, &a.minOut)
	if err == sql.ErrNoRows {
		return a, false, nil
	}
	if err != nil {
		return a, false, err
	}
	return a, true, nil
}

func (cs *ChainState) nachrechnenVorbehaltLocked(tx *Transaction, wallet string, blockZeit int64) error {
	vb := tx.Vorbehalt
	if vb == nil || vb.Ref == "" {
		return nil // applyVorbehaltAusfuehrungLocked lehnt ab
	}
	auftrag, gibt, err := cs.vorbehaltAuftragLocked(vorbehaltsKonto(vb.Ref))
	if err != nil {
		// Fail-closed: ohne lesbaren Auftrag wird nicht geglaubt.
		return nachrechnenAbweichung("vorbehalt_unlesbar", blockZeit, "%s: %v", kurzAdresse(wallet), err)
	}
	if !gibt {
		return nachrechnenAbweichung("vorbehalt_unbekannt", blockZeit,
			"%s: kein offener Vorbehalt %s", kurzAdresse(wallet), vb.Ref)
	}
	if auftrag.wallet != wallet || auftrag.art != vb.Art {
		return nachrechnenAbweichung("vorbehalt_fremd", blockZeit,
			"%s %s, beauftragt %s %s", kurzAdresse(wallet), vb.Art, kurzAdresse(auftrag.wallet), auftrag.art)
	}
	if vb.Erstattet {
		return nil
	}
	betragPasst := nahe(tx.Amount, auftrag.betrag)
	if vb.Art == "add_liquidity" {
		betragPasst = betragPasst && nahe(tx.AmountOut, auftrag.betrag2)
	}
	if !betragPasst {
		return nachrechnenAbweichung("vorbehalt_betrag", blockZeit,
			"%s %s: im Block %.6f/%.6f, unterschrieben %.6f/%.6f", kurzAdresse(wallet), vb.Art,
			tx.Amount, tx.AmountOut, auftrag.betrag, auftrag.betrag2)
	}
	if (vb.Art == "swap_aeq_tusd" || vb.Art == "swap_tusd_aeq") && auftrag.minOut > 0 &&
		tx.AmountOut+1e-6 < auftrag.minOut {
		return nachrechnenAbweichung("vorbehalt_mindestbetrag", blockZeit,
			"%s: Ergebnis %.6f unter dem Mindestbetrag %.6f", kurzAdresse(wallet), tx.AmountOut, auftrag.minOut)
	}
	return cs.nachrechnenPoolAuftragLocked(vb.Art, tx, wallet, blockZeit)
}
