package keeper

import (
	"context"
	"fmt"
)

// Der Schluessel, unter dem festgehalten wird, dass die Korrektur gelaufen ist.
const poolCorrectionFlagV1 = "pool_correction_applied_v1"

// Die Korrektur einer Geldschoepfung, die bereits stattgefunden hat.
//
// WAS HIER KORRIGIERT WIRD
//
// Am 15.08.2026 wies /api/health/combined erstmals aus, dass die Kette mehr
// AEQ haelt, als die Regel erlaubt: gemessen 17.305,277988 gegen 17 Menschen
// x 1.000 = 17.000. Beide harmlosen Erklaerungen wurden live ausgeschlossen
// -- der Menschenzaehler war nicht gedriftet, und es gab keine abgemeldeten
// Konten. Es wurde wirklich geschoepft.
//
// Die Ursache ist gefunden und behoben: Konto und Pool sind zwei getrennte
// Schreibvorgaenge, und die Reihenfolge entscheidet, wohin ein Fehler
// zwischen ihnen faellt. Fuenf Stellen schrieben in der SCHOEPFENDEN
// Reihenfolge; seit dem 20.08.2026 persistieren alle die Belastung vor der
// Gutschrift, sodass ein Fehler Geld VERNICHTET statt es zu erschaffen.
//
// Der Code schoepft also nichts mehr. Der Ueberschuss von damals liegt aber
// weiter in der AMM-Reserve, und Geld, das keine Registrierung erklaert,
// widerspricht dem einzigen Satz, auf dem diese Waehrung steht.
//
// WARUM ALS TRANSAKTION UND NICHT ALS MIGRATION
//
// Der naheliegende Weg waere eine einmalige Migration beim Start, wie
// MigrateStrandedPoolTUsdFeesV1. Der Pool-Zustand geht aber in den StateRoot
// ein. Ein Knoten, der schon neu gestartet ist, haette andere Reserven als
// einer, der noch laeuft -- und zwischen den beiden Neustarts wuerde jeder
// Block als Abweichung verworfen.
//
// Als replizierte Transaktion wenden beide Knoten die Korrektur am SELBEN
// Block an. Kein Zeitfenster, in dem sie sich uneinig sind. Der Preis dafuer
// ist ein Ausrollen in zwei Schritten: erst muessen beide den Typ KENNEN,
// dann darf er auftauchen. Ein Knoten, der ihn nicht kennt, wuerde den Block
// verwerfen.
//
// WARUM SIE NUR VERKLEINERN KANN
//
// Diese Transaktion wird nicht von einem Menschen unterschrieben, sondern
// entsteht auf einem Knoten -- wie ubi_distribution. Ein boesartiger oder
// fehlerhafter Produzent koennte sie also fabrizieren. Deshalb kann sie
// ausschliesslich verkleinern: der schlimmste Missbrauch ist, protokolleigene
// Liquiditaet zu vernichten. Koennte sie vergroessern, waere sie genau das
// Werkzeug, gegen das sie gebaut wurde.

// applyPoolCorrectionLocked verkleinert beide Reserven um exakte Betraege.
//
// DIESELBE Funktion laeuft auf dem erzeugenden und auf dem replayenden
// Knoten. Das ist Absicht: zwei Fassungen derselben Rechnung sind die
// haeufigste Quelle von StateRoot-Abweichungen in diesem Projekt gewesen.
//
// cs.mu muss gehalten werden.
func (cs *ChainState) applyPoolCorrectionLocked(ctx context.Context, aeqBurn, tusdBurn float64) error {
	if cs.pool == nil {
		return fmt.Errorf("pool_correction: dieser Knoten fuehrt keinen Pool")
	}
	// GENAU EINMAL, ueber die Lebenszeit der Kette.
	//
	// Wird derselbe Block ein zweites Mal zugestellt oder erneut abgespielt,
	// wuerde ohne diese Sperre ein zweites Mal verbrannt. Das ist keine
	// theoretische Sorge: bei den Verteilungen ist genau das nachgewiesen
	// passiert (Audit 2026-08-16, ein doppelt gelieferter Block zahlte
	// Validatoren und LPs vollstaendig erneut aus).
	//
	// Kein Fehler, sondern ein stiller Erfolg: der Zustand ist bereits der
	// gewuenschte. Ein Fehler wuerde den ganzen Block verwerfen und den
	// Knoten anhalten, obwohl nichts falsch ist.
	//
	// Absichtlich mit Versionsnummer. Eine spaetere, ANDERE Korrektur waere
	// ein neuer Schluessel -- sie soll nicht deshalb ausbleiben, weil vor
	// Monaten schon einmal korrigiert wurde.
	// getConfigValueCtx, NICHT getConfigValueDB.
	//
	// getConfigValueDB liest immer cs.db direkt und nie die offene
	// Transaktion. replayTransactions oeffnet aber EINE Transaktion fuer den
	// GANZEN Block. Enthielte ein Block zwei pool_correction-Transaktionen,
	// haette die erste die Sperre nur INNERHALB dieser Transaktion gesetzt --
	// die zweite haette daran vorbeigelesen, sie nicht gesehen und erneut
	// gebrannt. Und die dritte, und die vierte.
	//
	// Uebrig geblieben waere als einzige Schranke "die Reserve muss den Betrag
	// decken", und die schrumpft mit jedem Schritt: ein einziger Block haette
	// den Pool asymptotisch leergeraeumt. Der Typ ist unsigniert und entsteht
	// auf einem Knoten -- jeder Validator kann ihn ausstellen.
	//
	// "Genau einmal ueber die Lebenszeit der Kette" war damit in Wahrheit
	// "einmal je committeter Transaktionsgrenze".
	if cs.getConfigValueCtx(ctx, poolCorrectionFlagV1) == "1" {
		fmt.Printf("[SUPPLY] pool_correction bereits angewendet (%s) -- uebersprungen\n", poolCorrectionFlagV1)
		return nil
	}
	// Nur verkleinern. Siehe Kopfkommentar: eine Korrektur, die vergroessern
	// kann, waere selbst ein Weg zur Geldschoepfung.
	if aeqBurn <= 0 || tusdBurn < 0 {
		return fmt.Errorf("pool_correction: Betraege muessen positiv sein (aeq=%.6f tusd=%.6f)", aeqBurn, tusdBurn)
	}

	habenAEQ := cs.pool.ReserveAEQ.Float()
	habenTUSD := cs.pool.ReserveTUSD.Float()

	// NICHT auf null kappen, sondern scheitern.
	//
	// Kappen wuerde auf einem Knoten mit etwas kleinerer Reserve ein anderes
	// Ergebnis liefern als auf dem anderen -- beide "erfolgreich", beide
	// verschieden. Genau die Art stiller Abweichung, die dieses Projekt
	// schon mehrfach Tage gekostet hat. Reicht die Reserve nicht, stimmt eine
	// Annahme nicht, und dann gehoert der Block verworfen.
	if habenAEQ+1e-9 < aeqBurn {
		return fmt.Errorf("pool_correction: Reserve haelt nur %.6f AEQ, korrigiert werden sollen %.6f", habenAEQ, aeqBurn)
	}
	if habenTUSD+1e-9 < tusdBurn {
		return fmt.Errorf("pool_correction: Reserve haelt nur %.6f tUSD, korrigiert werden sollen %.6f", habenTUSD, tusdBurn)
	}

	cs.pool.ReserveAEQ = NewDecimal(round6(habenAEQ - aeqBurn))
	cs.pool.ReserveTUSD = NewDecimal(round6(habenTUSD - tusdBurn))

	if err := cs.savePoolToDBCtx(ctx); err != nil {
		return fmt.Errorf("pool_correction: Pool nicht gespeichert: %w", err)
	}
	// NACH dem Pool. Faellt der Knoten dazwischen aus, steht die Sperre nicht
	// und die Korrektur liefe erneut -- das ist die harmlosere Richtung als
	// eine gesetzte Sperre ohne durchgefuehrte Korrektur, die den Ueberschuss
	// fuer immer festschriebe.
	if err := cs.setConfigValueCtx(ctx, poolCorrectionFlagV1, "1"); err != nil {
		return fmt.Errorf("pool_correction: Sperre nicht gesetzt: %w", err)
	}
	fmt.Printf("[SUPPLY] Reserve korrigiert: %.6f -> %.6f AEQ (-%.6f), %.6f -> %.6f tUSD (-%.6f)\n",
		habenAEQ, cs.pool.ReserveAEQ.Float(), aeqBurn,
		habenTUSD, cs.pool.ReserveTUSD.Float(), tusdBurn)
	return nil
}

// ApplyPoolCorrectionDelta ist der Replay-Weg: ein Knoten wendet die
// Korrektur aus einem empfangenen Block an.
func (cs *ChainState) ApplyPoolCorrectionDelta(ctx context.Context, aeqBurn, tusdBurn float64) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.applyPoolCorrectionLocked(ctx, aeqBurn, tusdBurn)
}

// CorrectPhantomSupplyAtomic ist der erzeugende Weg. Sie aendert den Zustand
// und legt die Transaktion in denselben Ausgang, damit jeder andere Knoten
// GENAU dieselbe Korrektur anwendet statt sie nachzurechnen.
//
// Die Betraege werden uebergeben und nicht hier bestimmt. Eine Funktion, die
// sich selbst ausrechnet, wieviel Geld zuviel da ist, wuerde bei jedem Aufruf
// erneut zuschlagen -- und ein Fehler in dieser Rechnung waere unbemerkt eine
// Enteignung.
func (cs *ChainState) CorrectPhantomSupplyAtomic(aeqBurn, tusdBurn float64) error {
	return cs.runAtomicDistributionWithOutbox(func(ctx context.Context) ([]Transaction, error) {
		if err := cs.applyPoolCorrectionLocked(ctx, aeqBurn, tusdBurn); err != nil {
			return nil, err
		}
		// Amount = AEQ, AmountOut = tUSD. Bestehende Felder, damit das
		// Drahtformat unveraendert bleibt und aeltere Leser nichts verlieren,
		// was sie ohnehin nicht deuten koennten.
		return []Transaction{{Type: "pool_correction", Amount: aeqBurn, AmountOut: tusdBurn}}, nil
	})
}
