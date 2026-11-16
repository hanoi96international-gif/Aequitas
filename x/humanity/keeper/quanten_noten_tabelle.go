package keeper

import (
	"fmt"
	"strings"
)

// QUANTEN-NOTEN JE ZEILE (QG3, 16.11.2026)
//
// Bis hierhin stand das ganze Quantengeld-Register als EIN JSON-Wert in
// chain_config und wurde bei jeder Anmeldung und jedem Einloesen ganz neu
// geschrieben -- bei 50.000 Noten rund 10 MB je Transaktion. Jetzt:
//
//   - Saetze (hoechstens 8) weiter in chain_config (quantenGeldKonfigSchluessel);
//   - jede Note eine Zeile in quantenNotenTabelle: Anmelden = eine Zeile
//     mehr (quantenNoteSpeichernLocked), Einloesen = eine weniger
//     (quantenNoteEntfernenLocked);
//   - die StateRoot ist byte-gleich (SHA-256 ueber das ganze Register als
//     JSON), einmal je Stand gerechnet;
//   - Snapshot und Resync tragen die Noten als eigenen Abschnitt
//     (StateSnapshot.QuantenNoten).

// pruefeSnapshotQuantenNoten: die Noten eines Snapshots, BEVOR etwas
// geschrieben wird -- Grenze, Form, Ordnung, Summe. Ob jede Note zu einem
// Satz gehoert, prueft danach quantenGeldLaden gegen die Saetze, die dann
// in chain_config stehen.
func pruefeSnapshotQuantenNoten(noten []quantenNoteEintrag) error {
	if len(noten) > quantenNotenHoechstens {
		return fmt.Errorf("quantum notes: %d, at most %d", len(noten), quantenNotenHoechstens)
	}
	var summe int64
	for i, e := range noten {
		if !istHash64(e.K) || !kanonischeAdresse(e.Von) || !quantenSatzGueltig(e.Satz) || e.Anteile <= 0 || e.Anteile > quantenAnteileHoechstens ||
			(i > 0 && e.K <= noten[i-1].K) {
			return fmt.Errorf("quantum note %d invalid", i)
		}
		if summe > quantenAnteileHoechstens-e.Anteile {
			return fmt.Errorf("quantum notes: sum of shares over the limit")
		}
		summe += e.Anteile
	}
	return nil
}

// quantenNotenBlock: so viele Zeilen je INSERT (4 Parameter je Zeile, weit
// unter der Grenze von PostgreSQL).
const quantenNotenBlock = 500

// quantenNotenImportieren: die Noten eines (geprueften) Snapshots in die
// Tabelle. ersetzen (Resync): vorher alle loeschen. Sonst (Import in einen
// laufenden Knoten): nur, wenn der Knoten noch keine Note hat -- ein
// laufender Knoten behaelt seinen Stand, wie bei den Saetzen in
// chain_config.
func quantenNotenImportieren(db sqlExecutor, noten []quantenNoteEintrag, ersetzen bool) error {
	if ersetzen {
		if _, err := db.Exec(`DELETE FROM ` + quantenNotenTabelle); err != nil {
			return fmt.Errorf("could not clear %s: %w", quantenNotenTabelle, err)
		}
	} else {
		var da bool
		if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM ` + quantenNotenTabelle + `)`).Scan(&da); err != nil {
			return fmt.Errorf("could not read %s: %w", quantenNotenTabelle, err)
		}
		if da {
			return nil
		}
	}
	for start := 0; start < len(noten); start += quantenNotenBlock {
		ende := start + quantenNotenBlock
		if ende > len(noten) {
			ende = len(noten)
		}
		var werte []string
		var args []interface{}
		for i, e := range noten[start:ende] {
			werte = append(werte, fmt.Sprintf("($%d, $%d, $%d, $%d)", 4*i+1, 4*i+2, 4*i+3, 4*i+4))
			args = append(args, e.K, e.Satz, e.Von, e.Anteile)
		}
		if _, err := db.Exec(`INSERT INTO `+quantenNotenTabelle+` (k, satz, von, anteile) VALUES `+strings.Join(werte, ", "), args...); err != nil {
			return fmt.Errorf("could not insert quantum notes: %w", err)
		}
	}
	return nil
}

// registerSicherung: die Register im Speicher, die Snapshot-Import und Resync
// aus der Datenbank neu laden. Scheitert ein Schritt danach (bis zum
// Commit), kehren sie zurueck -- sonst stuende im Speicher der Stand des
// Snapshots, in der zurueckgerollten Datenbank der alte, und die StateRoot
// des Knotens rechnete mit Registern, die es nirgends gibt.
type registerSicherung struct {
	satz     satzPlan
	verifier *verifierRegister
	umzug    *umzugRegister
	quanten  *quantenGeldRegister
	offline  *offlineStand
	quantenW *quantenWStand
}

// registerSicherungLocked: Caller haelt cs.mu.
func (cs *ChainState) registerSicherungLocked() registerSicherung {
	return registerSicherung{
		satz:     cs.satzPlan,
		verifier: cs.verifierReg.Load(),
		umzug:    cs.umzugReg.Load(),
		quanten:  cs.quantenGeld.Load(),
		offline:  cs.offlineReg.Load(),
		quantenW: cs.quantenWReg.Load(),
	}
}

// registerSicherungZurueckLocked: Caller haelt cs.mu.
func (cs *ChainState) registerSicherungZurueckLocked(s registerSicherung) {
	cs.satzPlan = s.satz
	cs.verifierReg.Store(s.verifier)
	cs.umzugReg.Store(s.umzug)
	cs.quantenGeld.Store(s.quanten)
	cs.offlineReg.Store(s.offline)
	cs.quantenWReg.Store(s.quantenW)
}
