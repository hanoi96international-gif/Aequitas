package keeper

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Nachrechnen beim Nachspielen (Audit 2026-09-29, K-2, Schritt 1).
//
// # WARUM
//
// replayTransactions uebernahm Werte, die der Produzent in die Transaktion
// schreibt, ohne sie selbst zu pruefen: das Ergebnis eines Tauschs
// (AmountOut), die LP-Anteile einer Einlage, den Faucet-Betrag, negative
// oder unendliche Betraege. Ein boesartiger Produzent haette damit Geld aus
// dem Nichts erzeugen koennen. Heute produziert nur C1; mit dem ersten
// fremden Produzenten waere jeder dieser Wege offen.
//
// # WIE
//
// Jede Regel rechnet den Wert aus dem eigenen Zustand nach, BEVOR die
// Transaktion angewendet wird. Zwei Stufen, fest im Code (nicht per
// Umgebungsvariable -- eine Konsensregel, die je Knoten anders eingestellt
// sein kann, ist selbst nicht deterministisch):
//
//   - vor nachrechnenStrengAbUnix: BEOBACHTEN. Abweichungen werden gezaehlt,
//     protokolliert und unter /api/wirtschaft/regeln veroeffentlicht; der
//     Block gilt.
//   - ab nachrechnenStrengAbUnix: eine Abweichung lehnt den ganzen Block ab.
//
// Erst beobachten, weil eine zu strenge Regel die Kette anhalten wuerde: ob
// der Nachspielende wirklich dieselben Zahlen hat wie der Produzent
// (Poolstand, Rundung), zeigt nur der echte Verkehr. Zeigt die Beobachtung
// null Abweichungen, wird der Stichtag gesetzt -- spaetestens bevor ein
// zweiter unabhaengiger Produzent zugelassen wird (Launch-Checkliste 18).
//
// Stichtag an block.Timestamp: seit K-3 (zeitstempel_pruefung.go) kann ein
// Produzent hoechstens 120 s hinter seine Eltern zurueckdatieren.
const nachrechnenStrengAbUnix int64 = math.MaxInt64

var (
	nachrechnenGeprueft     atomic.Int64
	nachrechnenAbweichungen atomic.Int64
	nachrechnenJeRegel      sync.Map // regel -> *atomic.Int64
)

// nachrechnenStrengOverride: nur fuer Tests (wie wirtschaftAktivOverride);
// 0 = der Stichtag im Code gilt.
var nachrechnenStrengOverride atomic.Int64

func nachrechnenStrengAb() int64 {
	if o := nachrechnenStrengOverride.Load(); o != 0 {
		return o
	}
	return nachrechnenStrengAbUnix
}

func nachrechnenStreng(blockZeit int64) bool { return blockZeit >= nachrechnenStrengAb() }

// nachrechnenAbweichung zaehlt und protokolliert eine Abweichung; im
// strengen Modus ein Fehler, der den Block ablehnt.
func nachrechnenAbweichung(regel string, blockZeit int64, format string, args ...interface{}) error {
	nachrechnenAbweichungen.Add(1)
	z, _ := nachrechnenJeRegel.LoadOrStore(regel, new(atomic.Int64))
	z.(*atomic.Int64).Add(1)
	text := fmt.Sprintf(format, args...)
	fmt.Printf("[NACHRECHNEN] %s: %s\n", regel, text)
	if nachrechnenStreng(blockZeit) {
		return fmt.Errorf("nachrechnen %s: %s: %w", regel, text, ErrZustandLehntAb)
	}
	return nil
}

// endlichNichtNegativ: NaN, ±Inf und negative Werte haben in keinem
// Betragsfeld einer Transaktion etwas zu suchen.
func endlichNichtNegativ(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0) && x >= 0
}

// nahe: gleich bis auf Rundung (absolut 1e-6 oder relativ 1e-9).
func nahe(a, b float64) bool {
	d := math.Abs(a - b)
	return d <= 1e-6 || d <= 1e-9*math.Max(math.Abs(a), math.Abs(b))
}

// nachrechnenTxLocked prueft eine Transaktion gegen den eigenen Zustand,
// VOR dem Anwenden. Aufrufer haelt cs.mu (replayTransactions).
func (cs *ChainState) nachrechnenTxLocked(tx *Transaction, blockZeit int64) error {
	nachrechnenGeprueft.Add(1)

	// 1. Kein Betragsfeld negativ, NaN oder unendlich.
	for _, f := range []struct {
		name string
		wert float64
	}{
		{"amount", tx.Amount}, {"amount_out", tx.AmountOut}, {"lp_shares", tx.LPShares},
		{"amount_per_human", tx.AmountPerHuman}, {"from_demurrage_lost", tx.FromDemurrageLost},
		{"to_demurrage_lost", tx.ToDemurrageLost}, {"gebuehr", tx.Gebuehr},
	} {
		if !endlichNichtNegativ(f.wert) {
			return nachrechnenAbweichung("betrag_ungueltig", blockZeit,
				"%s %s=%v (%s)", tx.Type, f.name, f.wert, kurzAdresse(tx.Wallet))
		}
	}

	// 2. Seit der Umlaufsicherung keine Demurrage mehr (nachrechnen_ubi.go).
	if err := nachrechnenDemurrage(tx, blockZeit); err != nil {
		return err
	}

	wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
	switch tx.Type {
	case "faucet":
		// Der Produzent verteilt genau tusdFaucetAmount, und nur an Menschen
		// (claimTUsdFaucetLocked).
		if tx.Amount != tusdFaucetAmount {
			return nachrechnenAbweichung("faucet_betrag", blockZeit,
				"%s: im Block %.6f, erlaubt %.0f", kurzAdresse(wallet), tx.Amount, tusdFaucetAmount)
		}
		cs.ensureAccountLoadedCtx(context.Background(), wallet)
		if acc, ok := cs.accounts.Get(wallet); !ok || !acc.IsHuman {
			return nachrechnenAbweichung("faucet_kein_mensch", blockZeit, "%s ist kein registrierter Mensch", kurzAdresse(wallet))
		}

	case "swap_aeq_tusd", "swap_tusd_aeq", "add_liquidity":
		return cs.nachrechnenPoolAuftragLocked(tx.Type, tx, wallet, blockZeit)

	case "vormund_setzen", "lebenszeichen":
		return cs.nachrechnenVormundLocked(tx, blockZeit)

	case "vorbehalt_ausfuehrung":
		return cs.nachrechnenVorbehaltLocked(tx, wallet, blockZeit) // nachrechnen_vorbehalt.go

	case "slash_equivocation":
		return nachrechnenSlashLocked(tx, blockZeit) // slash_beweis.go

	case "liveness_renewal":
		return cs.nachrechnenErneuerungLocked(tx, wallet, blockZeit) // nachrechnen_erneuerung.go

	case "kappung":
		// Die Kappung bucht einen festen Betrag ins Grundeinkommen, den der
		// Zustaendige des Kontos bestimmt (kappung_verteilt.go). Er muss ueber
		// der Vermoegensgrenze liegen -- sonst nimmt ein Produzent einem
		// Konto Geld, das ihm zusteht. Die Grenze haengt am Durchschnitt aller
		// Menschen und kann sich zwischen Annahme und Nachspielen leicht
		// verschieben; deshalb nur die Obergrenze, mit 0,01 % der Grenze
		// Spielraum (2,5 AEQ bei 25.000). Ob das fuer ehrliche Bloecke reicht,
		// zeigt die Beobachtung, bevor der strenge Modus gilt.
		cs.ensureAccountLoadedCtx(context.Background(), wallet)
		acc, ok := cs.accounts.Get(wallet)
		if !ok {
			return nil // applyKappungDeltaLocked lehnt ab
		}
		erlaubt := cs.kappungsBetragLocked(acc)
		spielraum := 1e-6
		if grenze, ok := cs.wealthCapAmountLocked(); ok {
			spielraum += grenze * 0.0001
		}
		if tx.Amount > erlaubt+spielraum {
			return nachrechnenAbweichung("kappung_unter_grenze", blockZeit,
				"%s: %.6f AEQ gekappt, ueber der Grenze liegen %.6f", kurzAdresse(wallet), tx.Amount, erlaubt)
		}

	case "ubi_distribution":
		// Die alte Form (ein Betrag fuer alle) erzeugt kein aktueller Knoten
		// mehr; wer sie heute schickt, will allen Menschen einen beliebigen
		// Betrag gutschreiben.
		if tx.AmountPerHuman > 0 {
			return nachrechnenAbweichung("ubi_alte_form", blockZeit, "amount_per_human=%.6f", tx.AmountPerHuman)
		}
		// Wer, wie oft, wieviel (nachrechnen_ubi.go).
		return cs.nachrechnenUBILocked(tx, blockZeit)

	case "ubi_distribution_finalize":
		endstand := tx.Amount
		return cs.nachrechnenUBIAbschlussLocked(blockZeit, &endstand)

	case "lp_distribution":
		return cs.nachrechnenLPLocked(tx, blockZeit) // nachrechnen_lp.go
	case "lp_distribution_pool_zero":
		return cs.nachrechnenLPAbschlussLocked(blockZeit)

	case "escrow_move", "escrow_release", "escrow_recover":
		return cs.nachrechnenTreuhandLocked(tx, blockZeit) // nachrechnen_treuhand.go

	case "grant_release":
		return cs.nachrechnenFreigabeLocked(tx, blockZeit) // nachrechnen_freigabe.go

	case "validator_distribution":
		// Vor der Umstellung kommen die Gewichte aus registered_nodes, einem
		// lokalen Verzeichnis -- jeder Knoten prueft nur, dass der Empfaenger
		// ein Mensch ist. Danach aus der Kette, voll nachgerechnet
		// (validator_lohn_kette.go).
		return cs.nachrechnenValidatorLocked(tx, blockZeit)

	case "validator_distribution_pool_zero":
		return cs.nachrechnenValidatorAbschlussLocked(blockZeit)

	case "distribution_round_marker":
		// Die Runde traegt ihren eigenen Zeitpunkt. Liegt er weit neben dem
		// Block, verschiebt er die Doppelrunden-Erkennung aller Nachspieler.
		if d := tx.DistributionAt - blockZeit; d > 600 || d < -600 {
			if err := nachrechnenAbweichung("rundenmarke", blockZeit,
				"distribution_at=%d, Blockzeit %d (Abstand %ds)", tx.DistributionAt, blockZeit, d); err != nil {
				return err
			}
		}
		// Spaetestens hier ist die Runde zu Ende, auch ohne Abschluss.
		cs.nachrechnenFreigabeRundeEndeLocked()
		if err := cs.nachrechnenUBIAbschlussLocked(blockZeit, nil); err != nil {
			return err
		}
		if err := cs.nachrechnenValidatorAbschlussLocked(blockZeit); err != nil {
			return err
		}
		return cs.nachrechnenLPAbschlussLocked(blockZeit)
	}
	return nil
}

func kurzAdresse(a string) string {
	a = strings.ToLower(strings.TrimSpace(a))
	if len(a) > 10 {
		return a[:10] + "…"
	}
	return a
}

// nachrechnenStand fuer /api/wirtschaft/regeln.
func nachrechnenStand() map[string]interface{} {
	modus := "beobachten"
	if ab := nachrechnenStrengAb(); ab != math.MaxInt64 {
		modus = fmt.Sprintf("streng ab %d", ab)
	}
	jeRegel := map[string]int64{}
	var namen []string
	nachrechnenJeRegel.Range(func(k, v interface{}) bool {
		namen = append(namen, k.(string))
		jeRegel[k.(string)] = v.(*atomic.Int64).Load()
		return true
	})
	sort.Strings(namen)
	return map[string]interface{}{
		"modus":        modus,
		"geprueft":     nachrechnenGeprueft.Load(),
		"abweichungen": nachrechnenAbweichungen.Load(),
		"je_regel":     jeRegel,
		"bedeutung": "Werte, die der Produzent in Transaktionen schreibt (Tauschergebnis, LP-Anteile, " +
			"Faucet-Betrag, Rundenzeit, Grundeinkommen je Mensch, Demurrage), rechnet dieser Knoten beim Nachspielen selbst nach. " +
			"0 Abweichungen ist der Normalfall; erst danach wird die Pruefung scharf geschaltet.",
	}
}

// nachrechnenPoolAuftragLocked: Tausch und Einlage nachrechnen -- direkt
// (tx.Type) oder als Ausfuehrung eines Vorbehalts (art = Vorbehalt.Art).
// Dieselbe Rechnung fuer beide Wege, damit die Ausfuehrung nicht an der
// Pruefung vorbeifuehrt (Analyse 03.10.2026).
func (cs *ChainState) nachrechnenPoolAuftragLocked(art string, tx *Transaction, wallet string, blockZeit int64) error {
	switch art {
	case "swap_aeq_tusd", "swap_tusd_aeq":
		if cs.pool == nil || tx.Amount <= 0 {
			return nil
		}
		// Genau die Rechnung aus swapLockedMitAbgabe: Gebuehr vom Einsatz,
		// der Rest geht in die Konstantprodukt-Formel (swapTeilung, zum
		// selben Buchungsaugenblick wie applySwapDeltaLockedMitAbgabe).
		_, _, inPool := swapTeilung(tx.Amount, buchZeitBeimNachspielen(tx.BuchAt, blockZeit))
		var erwartet float64
		if art == "swap_aeq_tusd" {
			erwartet = AMMSwapOut(cs.pool.ReserveAEQ, cs.pool.ReserveTUSD, inPool).Float()
		} else {
			erwartet = AMMSwapOut(cs.pool.ReserveTUSD, cs.pool.ReserveAEQ, inPool).Float()
		}
		if !nahe(tx.AmountOut, erwartet) {
			return nachrechnenAbweichung("tausch_ergebnis", blockZeit,
				"%s %s: im Block %.6f, nachgerechnet %.6f", art, kurzAdresse(wallet), tx.AmountOut, erwartet)
		}

	case "add_liquidity":
		if tx.LPShares <= 0 || cs.pool == nil {
			return nil // ohne Feld rechnet addLiquidityDeltaLocked ohnehin selbst
		}
		var erwartet float64
		if cs.pool.ReserveAEQ.Float() > 0 && cs.pool.TotalLPShares.Float() > 0 {
			erwartet = (tx.Amount / cs.pool.ReserveAEQ.Float()) * cs.pool.TotalLPShares.Float()
		} else {
			erwartet = math.Sqrt(tx.Amount * tx.AmountOut)
		}
		if !nahe(tx.LPShares, erwartet) {
			return nachrechnenAbweichung("lp_anteile", blockZeit,
				"%s: im Block %.6f, nachgerechnet %.6f", kurzAdresse(wallet), tx.LPShares, erwartet)
		}
	}
	return nil
}
