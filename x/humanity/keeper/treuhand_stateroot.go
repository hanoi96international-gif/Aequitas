package keeper

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
)

// Treuhand in der StateRoot (offener Punkt aus docs/ERINNERUNG.md).
//
// # WARUM
//
// escrow_move nullt das Guthaben eines 2,5 Jahre inaktiven Menschen und legt
// den Betrag als Zeile in escrow_accounts ab; Freigabe und Rueckholung
// entfernen sie wieder. In die StateRoot ging nur das genullte Guthaben,
// nicht die Zeile. Zwei Knoten mit verschiedener Treuhand -- einer hat die
// Zeile, einer nicht, oder mit anderem Betrag oder anderer Frist -- hatten
// dieselbe Wurzel; erst die Nachrechen-Regeln (nachrechnen_treuhand.go)
// fielen auf, und auch nur, wenn eine Freigabe kam.
//
// # WAS
//
// escrowSetXOR summiert die Zeilen wie accountSetXOR die Konten: das Blatt
// jeder Zeile (Wallet und Betrag in Mikro-AEQ) wird beim Anlegen hinein- und
// beim Entfernen herausgerechnet. Die Frist (moved_at) steht NICHT im Blatt:
// der Erzeuger setzt sie auf seine Uhr (checkAndMoveToEscrowLocked), wer
// nachspielt, auf die Blockzeit (applyEscrowMoveDeltaLocked) -- mit ihr
// liefen Erzeuger und Nachspielende sofort auseinander. Ob eine Freigabe vor
// der Frist kommt, prueft weiter nachrechnen_treuhand.go.
//
// Eine Zeile ueber 0 AEQ entsteht nirgends mehr: der Erzeuger legte sie bei
// reinem Staub (nur LP/tUSD aufgeloest) an, wer nachspielt, nie -- die
// Summen waeren verschieden gewesen. Die StateRoot traegt die Summe
// NUR, wenn es eine Treuhand gibt -- ohne bleibt die Wurzel byte-gleich, also
// auch die StateRoot jedes bisherigen Blocks (die erste Verschiebung ist
// fruehestens am 09.12.2028 moeglich). Zurueckgerollt wird die Summe mit dem
// Block (blockRollbackSnapshot), neu aufgebaut beim Start und nach jedem
// Snapshot-Import (rebuildStateAccumulators). Der Snapshot traegt die
// Zeilen mit (StateSnapshot.Treuhand), sonst haette ein daraus gestarteter
// Knoten keine.

func treuhandBlatt(wallet string, amount float64) [32]byte {
	b := []byte("escrow:" + strings.ToLower(strings.TrimSpace(wallet)) + ":")
	b = strconv.AppendInt(b, NewDecimal(amount).Micro(), 10)
	return sha256.Sum256(b)
}

// treuhandBlattUmlegenLocked: eine Zeile hinein- oder herausrechnen (XOR ist
// beides). Unter cs.mu (Schreibsperre) -- alle Schreibwege der Treuhand
// laufen dort: Verteilung, Rueckholung, Nachspielen.
func (cs *ChainState) treuhandBlattUmlegenLocked(wallet string, amount float64) {
	xorInto(&cs.escrowSetXOR, treuhandBlatt(wallet, amount))
}

// treuhandZeileAnlegenLocked: eine bestehende Zeile bleibt, ihre Frist wird
// nie neu gestartet (ON CONFLICT DO NOTHING); das Blatt kommt nur hinzu,
// wenn wirklich eine Zeile entstand. Ueber 0 AEQ keine Zeile (siehe oben).
func (cs *ChainState) treuhandZeileAnlegenLocked(ctx context.Context, wallet string, amount float64, movedAt int64) error {
	wallet = strings.ToLower(strings.TrimSpace(wallet))
	if NewDecimal(amount).Micro() <= 0 {
		return nil
	}
	res, err := cs.dbExecCtx(ctx).Exec(
		`INSERT INTO escrow_accounts (wallet_address, amount, moved_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (wallet_address) DO NOTHING`,
		wallet, amount, movedAt,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		cs.treuhandBlattUmlegenLocked(wallet, amount)
	}
	return nil
}

// treuhandSummeAusDB: die Summe aller Zeilen, fuer rebuildStateAccumulators.
// Fehlt die Tabelle (Knoten ohne Treuhand-Pfade), ist die Summe null.
func (cs *ChainState) treuhandSummeAusDB() ([32]byte, error) {
	var x [32]byte
	if cs.db == nil {
		return x, nil
	}
	var gibt bool
	if err := cs.db.QueryRow(`SELECT to_regclass('escrow_accounts') IS NOT NULL`).Scan(&gibt); err != nil {
		return x, err
	}
	if !gibt {
		return x, nil
	}
	rows, err := cs.db.Query(`SELECT wallet_address, amount FROM escrow_accounts`)
	if err != nil {
		return x, err
	}
	defer rows.Close()
	for rows.Next() {
		var w string
		var amount float64
		if err := rows.Scan(&w, &amount); err != nil {
			return x, fmt.Errorf("escrow_accounts lesen: %w", err)
		}
		xorInto(&x, treuhandBlatt(w, amount))
	}
	return x, rows.Err()
}

// SnapshotTreuhand: eine Treuhand-Zeile im Snapshot.
type SnapshotTreuhand struct {
	Wallet  string  `json:"wallet"`
	Amount  float64 `json:"amount"`
	MovedAt int64   `json:"moved_at"`
}

// treuhandFuerSnapshot: alle Zeilen, nach Wallet geordnet (der Snapshot ist
// signiert; die Reihenfolge muss fest sein).
func (cs *ChainState) treuhandFuerSnapshot() ([]SnapshotTreuhand, error) {
	if cs.db == nil {
		return nil, nil
	}
	var gibt bool
	if err := cs.db.QueryRow(`SELECT to_regclass('escrow_accounts') IS NOT NULL`).Scan(&gibt); err != nil || !gibt {
		return nil, err
	}
	rows, err := cs.db.Query(`SELECT wallet_address, amount, moved_at FROM escrow_accounts ORDER BY wallet_address`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SnapshotTreuhand
	for rows.Next() {
		var t SnapshotTreuhand
		if err := rows.Scan(&t.Wallet, &t.Amount, &t.MovedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
