package keeper

import (
	"context"
	"strings"
)

// anwesenheitsZeitraum: der Tag vor der Verteilung.
const anwesenheitsZeitraum = 24 * 60 * 60

// validatorAnwesenheitCtx: je Betreiber-Wallet die Zahl der Minuten in
// [seit, bis), in denen sein Knoten mindestens einen Block gebaut hat.
//
// Jeder Validator baut in jedem Takt einen Block, auch ohne Ueberweisungen --
// eine Minute mit einem Block heisst: er war da. Mehr Bloecke in derselben
// Minute (der Leiter unter Last) zaehlen nicht mehr: Anwesenheit ist das
// Einzige, was jeder Mensch mit einem gewoehnlichen Rechner genauso gut
// leisten kann wie mit einem teuren.
//
// Die Bloecke stehen in chain_blocks; ihr Bauer ist der Signierschluessel
// (registered_nodes.signing_address), aeltere Eintraege nennen die Wallet
// selbst -- beides zaehlt, wie beim Hochzaehlen von blocks_produced.
func (cs *ChainState) validatorAnwesenheitCtx(ctx context.Context, wallets []string, seit, bis int64) map[string]int64 {
	out := map[string]int64{}
	if cs.db == nil || len(wallets) == 0 {
		return out
	}
	gesucht := map[string]bool{}
	for _, w := range wallets {
		gesucht[strings.ToLower(w)] = true
	}
	// Ein Betreiber, eine Minute: Bloecke unter Schluessel UND Wallet in
	// derselben Minute zaehlen einmal (DISTINCT je Wallet).
	rows, err := cs.dbExecCtx(ctx).Query(`SELECT lower(rn.wallet_address), COUNT(DISTINCT cb.timestamp / 60)
		FROM chain_blocks cb
		JOIN registered_nodes rn
		  ON lower(cb.proposer) = lower(rn.wallet_address)
		  OR (COALESCE(rn.signing_address, '') <> '' AND lower(cb.proposer) = lower(rn.signing_address))
		WHERE cb.timestamp >= $1 AND cb.timestamp < $2
		GROUP BY lower(rn.wallet_address)`, seit, bis)
	if err != nil {
		return out
	}
	defer rows.Close()
	namen := map[string]string{}
	for _, w := range wallets {
		namen[strings.ToLower(w)] = w
	}
	for rows.Next() {
		var w string
		var minuten int64
		if rows.Scan(&w, &minuten) == nil && gesucht[w] {
			out[namen[w]] = minuten
		}
	}
	return out
}
