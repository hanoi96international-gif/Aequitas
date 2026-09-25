package keeper

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
)

// SCALING_ARCHITECTURE.md roadmap step 6 — parallel replay.
//
// WHY THIS IS THE BOTTLENECK (measured live 2026-07-26, and the reason the
// user's question "muss primary das nicht aushalten?" was the right one):
//
// The WAL fast path (transferConcurrentWAL) accelerates only the INGESTION of
// transactions — the path they take when they arrive by RPC on that specific
// node. Every OTHER node receives the same transactions as BLOCKS and applies
// them through replayTransactions, one at a time, under cs.mu.Lock().
//
// A 20-second load test against Contabo2 (~250 TPS, 4,948 transfers, blocks
// carrying 117-173 transactions each) therefore left the primary ~1,200
// blocks behind and effectively unreachable for minutes — replay holds cs.mu,
// so /api/status cannot answer either — until it caught up on its own.
//
// So network throughput is bounded by the SLOWEST REPLAY, not by the fastest
// ingestion. 50k TPS on one node is worth nothing if the others replay at a
// few hundred.
//
// WHY THE PREVIOUS ATTEMPT WAS REVERTED (41b1eee, 2026-07-25), and how this
// differs:
//
// That version guarded its workers with a local sync.Mutex while they wrote
// through dag.state.activeTx — a single *sql.Tx shared across goroutines. A
// local mutex serializes those workers against each other but not against any
// other goroutine touching the same field, so it never established the
// invariant it claimed. Two goroutines on one Postgres connection desync the
// wire protocol; the exact signature (`pq: unexpected Parse response "(D)
// DataRow"`) appeared on Contabo2 minutes after deploy and cost two consensus
// blocks.
//
// The lesson, which Block-STM (Aptos, 160k TPS) states directly: DO NOT TOUCH
// THE DATABASE DURING THE PARALLEL PHASE. This implementation therefore has
// three strictly separated phases:
//
//	1. serial   — warm every account the batch needs (the only DB reads)
//	2. parallel — pure in-memory arithmetic on disjoint AccountState structs
//	3. serial   — ONE batched write for every account the batch touched
//
// Phase 2 performs no DB access of any kind, so there is no shared *sql.Tx to
// corrupt. It needs no account locks either: the batch is disjoint BY
// CONSTRUCTION, so no two workers can reach the same AccountState, and the
// map itself is only read (every account was made resident in phase 1). The
// one genuinely shared piece of state, cs.accountSetXOR, is mutated through
// updateAccountLeafLocked, which already carries accountSetXORMu for exactly
// this case — see that field's own comment.
//
// DETERMINISM: a batch is only ever formed from CONSECUTIVE transfers whose
// touched addresses are pairwise disjoint, so applying them in any order
// yields the identical final state. That is not an assumption — it is the
// property TestReplayTransactions_DisjointTransfers_OrderIndependent and its
// fuzz variant already pin, written before any parallel implementation
// existed precisely so this step could rely on it.

// parallelReplayMinBatch is the smallest batch this path is used for.
//
// KORREKTUR (05.09.2026, gemessen): der Wert stand auf 16, begruendet mit
// "smallest batch worth spawning goroutines for -- below it the scheduling
// overhead exceeds the arithmetic". Diese Begruendung war falsch, weil sie
// die falsche Ersparnis betrachtet. Der Gewinn dieses Pfades ist NICHT die
// parallele Arithmetik in Phase 2 -- die ist ein paar Nanosekunden je
// Ueberweisung. Er ist Phase 3: EIN gebuendelter Multi-Row-Write fuer alle
// beruehrten Konten, statt zwei einzelner Datenbank-Umlaeufe je Ueberweisung
// im seriellen Pfad (saveAccountToDBCtx fuer Sender und Empfaenger).
//
// Und diese Ersparnis faellt schon ab zwei Ueberweisungen an, nicht ab
// sechzehn. Gemessen auf dem Primary unter Last (replay_phasen, 376 Bloecke):
//
//	seriell    4,724 ms je Ueberweisung   2.365 Stueck   48,2 % der Sperre
//	parallel   0,281 ms je Ueberweisung  27.565 Stueck   32,7 % der Sperre
//
// Sieben Komma neun Prozent der Ueberweisungen kosteten also fast die Haelfte
// der Zeit, in der das Nachspielen die globale Sperre haelt -- und in der
// nach Go's RWMutex-Semantik jede ankommende Ueberweisung ausgesperrt ist.
//
// Warum ueberhaupt so viele seriell liefen: collectDisjointTransferBatch
// beendet einen Lauf, sobald sich eine Adresse wiederholt. Bei wenigen
// hundert Konten und hunderten Ueberweisungen je Block passiert das staendig,
// und jeder Lauf, der unter dieser Schwelle blieb, ging Ueberweisung fuer
// Ueberweisung ueber den teuren Pfad. Die Schwelle hat sich also selbst
// gefuettert.
//
// NACHTRAG, sobald der Blocksammler steht (replay_konten_sammler.go): die
// Schwelle faellt auf EINS. Ihre einzige verbliebene Begruendung war, dass
// ein Buendel selbst einen Datenbank-Umlauf kostet und sich bei einer
// einzelnen Ueberweisung deshalb nicht lohnt. Mit dem Sammler schreibt der
// Buendelpfad ueberhaupt nicht mehr -- er reicht die beruehrten Konten
// weiter, und geschrieben wird einmal je Block. Damit kostet ein Buendel der
// Groesse eins nichts ausser ein wenig Go-Arbeit, waehrend derselbe Transfer
// auf dem seriellen Pfad zwei Datenbank-Umlaeufe kostet.
//
// Das ist auch der Grund, warum nach dem Schritt auf 2 noch rund zehn
// Ueberweisungen je Block seriell liefen: zwei aufeinanderfolgende Transfers,
// die sich eine Adresse teilen, ergeben ein Buendel der Laenge eins.
//
// Die Semantik aendert sich nicht. Dieser Pfad lehnt weiterhin bei jedem
// Zweifel ab (unbekanntes Konto, unzureichendes Guthaben, Wohlstandsgrenze)
// und ueberlaesst die Ueberweisung dann dem seriellen Pfad, der Fehler und
// Protokollzeilen unveraendert erzeugt. Die volle keeper-Testsuite --
// einschliesslich TestParallelReplay_MatchesSerialExactly und der
// Determinismus-Fuzz -- ist mit diesem Wert gruen.
const parallelReplayMinBatch = 1

// replayBatchItem is one transfer accepted into a parallel batch, resolved to
// its two account pointers during the serial warm-up phase so the parallel
// phase needs no map lookups at all.
type replayBatchItem struct {
	from    *AccountState
	to      *AccountState
	amount  float64
	fromKey string
	toKey   string
	// buchAt: Buchungsaugenblick fuer die Unternehmens-Buchfuehrung, wie ihn
	// der serielle Pfad ueber mitBuchZeit setzt (buchZeitBeimNachspielen).
	buchAt int64
	// toNach: Stand des Empfaengers direkt NACH dieser Ueberweisung, in
	// Blockreihenfolge. Seit Stufe 1.3 kann ein Empfaenger mehrfach im
	// Buendel stehen; der serielle Pfad reicht an nachUeberweisung den Stand
	// nach genau dieser Gutschrift weiter, nicht den Endstand des Buendels.
	toNach float64
}

// collectDisjointTransferBatch walks txs starting at index start and returns
// the longest run of CONSECUTIVE transactions that may be applied in parallel.
//
// A transaction joins the run only if all of the following hold:
//   - it is a plain "transfer" with well-formed fields
//   - it carries NO demurrage (FromDemurrageLost/ToDemurrageLost both zero).
//     Demurrage settlement credits the tokenomics pools and persists them,
//     i.e. it is DB work and shared-state work, and it is exactly the
//     eligibility line transferConcurrentWAL already draws for the same
//     reason.
//   - its SENDER has not appeared anywhere earlier in this run (neither as
//     sender nor as recipient)
//   - its RECIPIENT has not appeared earlier as a SENDER
//
// STUFE 1.3 (docs/SKALIERUNG_DEZENTRAL.md): ein Empfaenger darf mehrfach
// vorkommen. Vorher beendete jede wiederholte Adresse den Lauf -- und der
// haeufigste Fall im echten Betrieb ist genau der, dass viele Menschen im
// selben Block an dasselbe Geschaeft zahlen. Jede dieser Zahlungen brach
// den Lauf ab und erzeugte ein Buendel der Laenge eins.
//
// Warum das sicher ist: eine Gutschrift ist eine Addition in ganzen
// Mikro-AEQ (Decimal ist int64), also exakt und von der Reihenfolge
// unabhaengig. Was von der Reihenfolge abhaengt, fuehrt
// applyTransferBatchParallel in Blockreihenfolge: die Wohlstandsgrenze
// (laufender Stand je Empfaenger) und die Buchfuehrung (Phase 2b). Und eine
// Adresse, die im Lauf Geld bekommt, darf darin nie senden -- sonst haenge
// ihre Deckung von einer frueheren Gutschrift ab. Absender bleiben
// eindeutig, damit ihre Deckung am Stand vor dem Buendel exakt pruefbar ist.
//
// The first transaction that fails any of these ends the run. Returning early
// rather than skipping past it is what preserves ordering semantics: anything
// after a non-batchable transaction may depend on it.
//
// touched enthaelt alle Adressen des Laufs, Absender wie Empfaenger.
func collectDisjointTransferBatch(txs []Transaction, start int) (batch []Transaction, touched map[string]bool) {
	touched = make(map[string]bool)
	absender := make(map[string]bool)
	for i := start; i < len(txs); i++ {
		tx := txs[i]
		if tx.Type != "transfer" {
			if i == start {
				merkeBuendelAblehnung(&baKeinTransfer)
			}
			break
		}
		from := strings.ToLower(strings.TrimSpace(tx.Wallet))
		to := strings.ToLower(strings.TrimSpace(tx.To))
		if from == "" || to == "" || tx.Amount <= 0 || from == to {
			if i == start {
				merkeBuendelAblehnung(&baFelder)
			}
			break
		}
		if tx.FromDemurrageLost != 0 || tx.ToDemurrageLost != 0 || tx.Gebuehr != 0 {
			// Nur zaehlen, wenn es die ERSTE ist: dann bleibt der Lauf leer und
			// die Ueberweisung geht seriell. Bricht die Demurrage einen bereits
			// laufenden Buendel ab, ist das kein Verlust -- das Buendel wird
			// angewendet und die naechste Runde beginnt bei ihr.
			if i == start {
				merkeBuendelAblehnung(&baDemurrage)
			}
			break
		}
		// Absender: nirgends zuvor im Lauf. Empfaenger: zuvor hoechstens als
		// Empfaenger (Stufe 1.3), nie als Absender.
		if touched[from] || absender[to] {
			if i == start {
				merkeBuendelAblehnung(&baKollision)
			}
			break
		}
		absender[from] = true
		touched[from] = true
		touched[to] = true
		batch = append(batch, tx)
	}
	return batch, touched
}

// applyTransferBatchParallel applies an already-validated disjoint batch.
//
// Three-valued result, and the distinction is critical:
//
//	(false, nil)  — declined BEFORE mutating anything. The caller replays
//	                these transfers serially, reproducing the serial path's
//	                behaviour and error messages exactly. Pure speed-up.
//	(true,  nil)  — applied and persisted.
//	(false, err)  — memory was ALREADY mutated and persistence then failed.
//	                The caller MUST treat this as a hard block failure and
//	                roll back. It must never fall back to the serial path
//	                here: that would apply every transfer a second time.
//
// That last case is why this returns an error at all rather than a plain
// bool. An earlier draft of this function returned false on a failed batch
// write, which would have double-applied the whole batch — the same defect
// class that drove 74 production accounts negative on 2026-07-25.
//
// activityAt is the replayed block's own Timestamp, stamped onto every
// participant's demurrage clock exactly as the serial path does — see
// touchActivityAt for why this must not be the replaying node's wall clock.
//
// Caller must hold cs.mu (write), exactly as the serial path does.
//
// sammler darf nil sein -- dann schreibt Phase 3 sofort, wie bisher. Ist er
// gesetzt, werden die beruehrten Konten stattdessen gesammelt und vom
// Aufrufer EINMAL je Block geschrieben; siehe replay_konten_sammler.go fuer
// die Messung, die das noetig macht, und dafuer, warum das an Atomizitaet
// und StateRoot nichts aendert.
// RUECKGABE seit 06.09.2026: die Zahl der ANGEWANDTEN Ueberweisungen statt
// eines Ja/Nein.
//
// Vorher lehnte eine einzige unbezahlbare Ueberweisung den GANZEN Lauf ab --
// bei gemessenen 143 Ueberweisungen je Lauf gingen also bis zu 142 gesunde
// mit ihr auf den teuren seriellen Pfad. Und genau dieser Fall ist der
// haeufigste: von allen Ablehnungsgruenden war "guthaben" mit 14.444 der
// einzige, der ueberhaupt auftrat (lauf_demurrage, lauf_kollision,
// lauf_kein_transfer alle null), weil die Wegwerfkonten des Lasttests
// leerlaufen.
//
// Die Ueberweisungen VOR der problematischen sind ein gueltiges, disjunktes
// Praefix -- sie duerfen angewandt werden, und der Aufrufer setzt danach bei
// der problematischen fort. Die Reihenfolge bleibt damit exakt erhalten.
//
// 0 heisst wie bisher: nichts angewandt, der serielle Pfad macht alles.
func (cs *ChainState) applyTransferBatchParallel(ctx context.Context, batch []Transaction, activityAt int64, sammler *kontenSammler) (angewandt int, err error) {
	if len(batch) < parallelReplayMinBatch {
		merkeBuendelAblehnung(&baZuKlein)
		return 0, nil
	}

	// ---- Phase 1 (serial): warm every account. The ONLY DB access. ----
	items := make([]replayBatchItem, 0, len(batch))
	for _, tx := range batch {
		from := strings.ToLower(strings.TrimSpace(tx.Wallet))
		to := strings.ToLower(strings.TrimSpace(tx.To))
		cs.ensureAccountLoadedCtx(ctx, from)
		cs.ensureAccountLoadedCtx(ctx, to)
		fromAcc, okFrom := cs.accounts.Get(from)
		toAcc, okTo := cs.accounts.Get(to)
		if !okFrom || !okTo {
			merkeBuendelAblehnung(&baKontoFehlt)
			return 0, nil // unknown account — let the serial path report it
		}
		items = append(items, replayBatchItem{
			from: fromAcc, to: toAcc, amount: tx.Amount, fromKey: from, toKey: to,
			buchAt: buchZeitBeimNachspielen(tx.BuchAt, activityAt),
		})
	}

	// Sufficiency is checked here, serially, BEFORE anything is mutated. Each
	// sender appears at most once in the batch and never as a recipient in
	// it (collectDisjointTransferBatch), so its balance cannot be changed by
	// another member — meaning this check is exactly as authoritative as the
	// serial path's own, just hoisted. If any single
	// transfer would fail, the whole batch is declined and the serial path
	// replays all of them, reproducing its error handling verbatim.
	//
	// FIX (audit 2026-08-15): the wealth-cap check below was missing entirely.
	// The serial path this replaces (applyTransferDeltaLocked) calls
	// enforceWealthCapLockedCtx on every recipient, which trims a balance that
	// lands above avg×multiplier back down to the cap and credits the excess to
	// the four tokenomics pools. Phase 2 cannot do that — enforceWealthCap
	// credits pools through distributeSwapFeeCtx, i.e. shared state and DB
	// work, exactly what phase 2 is forbidden to touch — so a cap crossing
	// inside a batchable run was silently skipped: the recipient kept the full
	// uncapped amount and the pools were never credited.
	//
	// That is a fork, not a rounding difference. The three INGESTION fast paths
	// (transferConcurrent, transferBatchConcurrent, transferConcurrentWAL) each
	// draw exactly this eligibility line for exactly this reason and hand the
	// transfer to the slow path, which caps it — so a block genuinely can carry
	// a capped transfer, while every node replaying it through this batch path
	// would apply it uncapped. Proven by
	// TestParallelReplay_EnforcesWealthCapLikeSerial: recipient 26,000 vs
	// 25,000 AEQ, all four pools 0 vs 400/300/200/100, different StateRoot.
	//
	// Declining here (rather than trying to cap in phase 2) keeps this path
	// what its doc comment promises — a pure speed-up whose declines cost only
	// speed — and lets the serial path reproduce the cap, the pool credits and
	// the log line verbatim. The post-transfer balance is exact (see the
	// running balance below); the Decimal add mirrors the
	// arithmetic enforceWealthCapLockedCtx itself would see, and tokenomics
	// pool addresses are exempt there, so they must not trigger a decline here.
	//
	// STUFE 1.3: ein Empfaenger kann mehrfach im Buendel stehen. Geprueft
	// wird deshalb gegen seinen LAUFENDEN Stand in Blockreihenfolge -- genau
	// das, was der serielle Pfad nach jeder einzelnen Gutschrift sieht. Die
	// erste Gutschrift, die die Grenze reisst, beendet das Praefix; sie und
	// alles danach uebernimmt der serielle Pfad. Die Grenze selbst bleibt im
	// Buendel fest: Ueberweisungen aendern die Geldmenge nicht, und eine
	// Kappung, die sie aendern koennte, kommt nie ins Buendel.
	capAmt, hasCap := cs.wealthCapAmountLocked()
	laufend := make(map[string]Decimal, len(items))
	// Auf das gesunde Praefix kuerzen statt alles abzulehnen. Die
	// Ueberweisungen davor sind disjunkt und bezahlbar; die problematische und
	// alles danach uebernimmt der serielle Pfad, der Fehler, Wohlstandsgrenze
	// und Protokollzeilen unveraendert erzeugt.
	for i := range items {
		it := &items[i]
		vorher, gesehen := laufend[it.toKey]
		if !gesehen {
			vorher = it.to.Balance
		}
		nach := vorher.Add(NewDecimal(it.amount))
		schlecht := false
		if it.from.Balance.Float() < it.amount {
			merkeBuendelAblehnung(&baGuthaben)
			schlecht = true
		} else if cs.wuerdeKappenLocked(it.toKey, it.to, nach.Float(), capAmt, hasCap) {
			merkeBuendelAblehnung(&baWohlstandsCap)
			schlecht = true
		}
		if !schlecht {
			laufend[it.toKey] = nach
			it.toNach = nach.Float()
		}
		if schlecht {
			if i < parallelReplayMinBatch {
				return 0, nil // kein brauchbares Praefix uebrig
			}
			items = items[:i]
			merkeBuendelGekuerzt(len(batch) - i)
			break
		}
	}

	// ---- Phase 2 (parallel): pure in-memory arithmetic, NO database. ----
	//
	// Arbeitseinheiten statt Ueberweisungen (Stufe 1.3): je Absender eine
	// Belastung, je EMPFAENGER eine Gutschrift mit der Summe aller seiner
	// Betraege. So beruehrt jede AccountState genau eine Einheit, und kein
	// Worker kann mit einem anderen um denselben Empfaenger konkurrieren --
	// auch wenn hundert Ueberweisungen an ihn gehen. Absender und Empfaenger
	// sind disjunkt (collectDisjointTransferBatch).
	//
	// Die Summe ist bitgleich mit den einzelnen Gutschriften des seriellen
	// Pfads: jede wird wie dort einzeln mit NewDecimal in Mikro-AEQ gewandelt
	// und dann ganzzahlig addiert. Ebenso der StateRoot: updateAccountLeafLocked
	// tauscht das zuletzt gezaehlte Blatt gegen das aktuelle, einmal mit dem
	// Endstand ergibt dasselbe XOR wie einmal je Gutschrift. Die Uhr des
	// Empfaengers startet mit dem Blockzeitpunkt, fuer jede Gutschrift
	// derselbe -- startClockIfUnsetAt ist damit idempotent.
	type einheit struct {
		acc      *AccountState
		betrag   Decimal
		absender bool
	}
	einheiten := make([]einheit, 0, len(items)*2)
	gutschrift := make(map[string]int, len(items))
	for _, it := range items {
		einheiten = append(einheiten, einheit{acc: it.from, betrag: NewDecimal(it.amount), absender: true})
	}
	for _, it := range items {
		if j, ok := gutschrift[it.toKey]; ok {
			einheiten[j].betrag = einheiten[j].betrag.Add(NewDecimal(it.amount))
			continue
		}
		gutschrift[it.toKey] = len(einheiten)
		einheiten = append(einheiten, einheit{acc: it.to, betrag: NewDecimal(it.amount)})
	}

	workers := runtime.NumCPU()
	if workers > len(einheiten) {
		workers = len(einheiten)
	}
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	chunk := (len(einheiten) + workers - 1) / workers
	for w := 0; w < workers; w++ {
		lo := w * chunk
		if lo >= len(einheiten) {
			break
		}
		hi := lo + chunk
		if hi > len(einheiten) {
			hi = len(einheiten)
		}
		wg.Add(1)
		go func(part []einheit) {
			defer wg.Done()
			for _, e := range part {
				if e.absender {
					e.acc.Balance = e.acc.Balance.Sub(e.betrag)
					touchActivityAt(e.acc, activityAt)
				} else {
					e.acc.Balance = e.acc.Balance.Add(e.betrag)
					// EMPFANGEN STARTET DIE UHR, ES SETZT SIE NIE ZURUECK.
					//
					// Hier stand touchActivityAt, also ein Zuruecksetzen -- und
					// der serielle Pfad, den dieser hier nur beschleunigen soll,
					// ruft an derselben Stelle startClockIfUnsetAt
					// (applyTransferDeltaLockedSammelnd, state.go). Damit hing
					// die Demurrage-Uhr jedes Empfaengers davon ab, ob seine
					// Ueberweisung zufaellig in einem buendelbaren Lauf lag.
					// Belegt von TestNachspielen_SeriellUndParallelGleicheEmpfaengerUhr
					// (300 Tage Unterschied) -- siehe
					// annahme_gegen_nachspielen_test.go fuer das Experiment und
					// dafuer, warum kein Waechter das melden konnte
					// (LastActivityAt steht nicht im accountLeaf).
					startClockIfUnsetAt(e.acc, activityAt)
				}
				// The one piece of genuinely shared state; guarded by
				// accountSetXORMu inside, which exists for precisely this.
				cs.updateAccountLeafLocked(e.acc)
			}
		}(einheiten[lo:hi])
	}
	wg.Wait()

	// ---- Phase 2b (serial): Buchfuehrung, in Blockreihenfolge. ----
	//
	// Ab der Aktivierung der Unternehmensregeln (wirtschaft.go) fuehrt der
	// serielle Pfad nach jeder Ueberweisung nachUeberweisung aus: Monats-
	// zaehler, Umsatz, Freibetraege. Ohne diesen Schritt musste das parallele
	// Nachspielen ab dem 1.10.2026 ganz abgeschaltet werden (block.go) --
	// die Kette waere mit dem Start der Wirtschaftsregeln langsamer geworden.
	//
	// Warum das hier richtig ist: nachUeberweisung beruehrt nur die
	// Buchkonten von Sender und Empfaenger (dazu lesend das Register). Seit
	// Stufe 1.3 kann ein Empfaenger mehrfach vorkommen, und sein Buchkonto
	// summiert Umsatz und Tageswerte in Gleitkomma -- deshalb zwingend die
	// Blockreihenfolge, mit dem Empfaengerstand nach genau dieser Gutschrift
	// (toNach). Dieselben Aufrufe mit denselben Werten in derselben Folge wie
	// im seriellen Pfad. Die Gebuehr ist
	// hier immer 0: Ueberweisungen mit Gebuehr kommen nicht ins Buendel
	// (collectDisjointTransferBatch).
	//
	// Der Speicher ist bereits mutiert. Ein Fehler hier ist deshalb ein
	// harter Blockfehler (Rueckgabe mit err), nie ein Rueckfall auf den
	// seriellen Pfad -- sonst wuerde das Buendel doppelt angewandt. Die
	// Buchkonten werden gesammelt und einmal fuer das Buendel in dieselbe
	// Transaktion geschrieben (ctx); den Rueckbau im Speicher macht
	// blockRollbackSnapshot (buchStand).
	buchCtx, buch := mitBuchSammler(ctx)
	for _, it := range items {
		if err := cs.nachUeberweisung(mitBuchZeit(buchCtx, it.buchAt), it.fromKey, it.toKey,
			cs.kontoartVon(it.fromKey, it.from.IsHuman), cs.kontoartVon(it.toKey, it.to.IsHuman),
			it.amount, 0, it.from.Balance.Float(), it.toNach, it.buchAt); err != nil {
			return 0, fmt.Errorf("parallel transfer batch: Buchfuehrung: %w", err)
		}
	}
	if err := buch.schreiben(cs, ctx); err != nil {
		return 0, fmt.Errorf("parallel transfer batch: Buchfuehrung speichern: %w", err)
	}

	// ---- Phase 3 (serial): ONE batched write for everything touched. ----
	seen := make(map[string]bool, len(items)*2)
	accs := make([]*AccountState, 0, len(items)*2)
	for _, it := range items {
		if !seen[it.fromKey] {
			seen[it.fromKey] = true
			accs = append(accs, it.from)
		}
		if !seen[it.toKey] {
			seen[it.toKey] = true
			accs = append(accs, it.to)
		}
	}
	// Sammeln statt schreiben, wenn der Aufrufer die Statements eines ganzen
	// Blocks zusammenlegt. Der Speicher ist an dieser Stelle bereits mutiert
	// und der StateRoot-Akkumulator bereits fortgeschrieben (Phase 2) -- was
	// noch aussteht, ist allein die Zeile in der Datenbank, und die schreibt
	// der Aufrufer vor seinem StateRoot-Vergleich und innerhalb derselben
	// dbTx. Die Rollback-Einheit bleibt damit unveraendert.
	if sammler != nil {
		sammler.hinzufuegen(accs...)
		return len(items), nil
	}
	if err := cs.saveAccountsToDBBatchCtx(ctx, accs); err != nil {
		// Memory is already mutated at this point. Returning (false, nil)
		// here would send the caller to the serial path and apply every
		// transfer in this batch a SECOND time. Surface it as a hard error
		// so the block is rolled back through the caller's existing
		// rollback snapshot instead — see this function's doc comment.
		return 0, fmt.Errorf("parallel transfer batch: could not persist %d account(s): %w", len(accs), err)
	}
	return len(items), nil
}
