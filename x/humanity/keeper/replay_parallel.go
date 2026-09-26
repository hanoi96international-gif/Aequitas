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
//	2. parallel — pure in-memory arithmetic, one work unit per ACCOUNT
//	3. serial   — ONE batched write for every account the batch touched
//
// (Seit Stufe 1.3 steht zwischen 1 und 2 eine serielle Vorrechnung im
// Speicher, Phase 1b -- siehe applyTransferBatchParallel.)
//
// Phase 2 performs no DB access of any kind, so there is no shared *sql.Tx to
// corrupt. It needs no account locks either: each AccountState is exactly one
// work unit BY CONSTRUCTION, so no two workers can reach it, and the
// map itself is only read (every account was made resident in phase 1). The
// one genuinely shared piece of state, cs.accountSetXOR, is mutated through
// updateAccountLeafLocked, which already carries accountSetXORMu for exactly
// this case — see that field's own comment.
//
// DETERMINISM: a batch is only ever formed from CONSECUTIVE transfers. Bis
// 25.09.2026 mussten ihre Adressen paarweise disjunkt sein (belegt von
// TestReplayTransactions_DisjointTransfers_OrderIndependent und dem Fuzz
// dazu). Seit Stufe 1.3 duerfen sie sich wiederholen: alles, was von der
// Reihenfolge abhaengt (Deckung, Grenze, Buchfuehrung), rechnet Phase 1b bzw.
// 2b in Blockreihenfolge; parallel angewandt wird nur die Summe je Konto, und
// die haengt von der Reihenfolge nicht ab. Belegt von
// replay_parallel_sammelempfaenger_test.go gegen den seriellen Pfad.

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
	// fromNach: Stand des Absenders direkt nach dieser Ueberweisung. Seit
	// dem zweiten Teil von Stufe 1.3 kann auch ein Absender im Lauf vorher
	// Geld bekommen oder mehrfach senden.
	fromNach float64
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
//   - it carries no fee (Gebuehr): the fee credits the UBI pool.
//
// STUFE 1.3 (docs/SKALIERUNG_DEZENTRAL.md): Adressen duerfen sich im Lauf
// wiederholen -- als Empfaenger, als Absender, und dieselbe Adresse erst als
// Empfaenger und dann als Absender. Bis 25.09. beendete jede wiederholte
// Adresse den Lauf, und der Alltag (viele zahlen an dasselbe Geschaeft, das
// Geschaeft zahlt davon Lohn) zerfiel in Buendel der Laenge eins.
//
// Moeglich ist das, weil applyTransferBatchParallel den Lauf zuerst SERIELL
// IM SPEICHER vorrechnet (Deckung, Wohlstandsgrenze, Stand nach jeder
// Ueberweisung) und erst danach parallel anwendet -- siehe dort. Der Name
// der Funktion ist geblieben; "disjunkt" ist sie nicht mehr.
//
// The first transaction that fails any of these ends the run. Returning early
// rather than skipping past it is what preserves ordering semantics: anything
// after a non-batchable transaction may depend on it.
//
// touched enthaelt alle Adressen des Laufs.
func collectDisjointTransferBatch(txs []Transaction, start int) (batch []Transaction, touched map[string]bool) {
	touched = make(map[string]bool)
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
		touched[from] = true
		touched[to] = true
		batch = append(batch, tx)
	}
	return batch, touched
}

// applyTransferBatchParallel applies a run from collectDisjointTransferBatch.
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
// Die Ueberweisungen VOR der problematischen sind ein gueltiges
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
	//
	// Ein unbekanntes Konto beendet das Praefix: die Ueberweisungen davor
	// sind unabhaengig davon, ab ihr uebernimmt der serielle Pfad (der ein
	// fehlendes Empfaengerkonto anlegt und ein fehlendes Absenderkonto
	// meldet).
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
			break
		}
		items = append(items, replayBatchItem{
			from: fromAcc, to: toAcc, amount: tx.Amount, fromKey: from, toKey: to,
			buchAt: buchZeitBeimNachspielen(tx.BuchAt, activityAt),
		})
	}
	if len(items) < parallelReplayMinBatch {
		return 0, nil
	}

	// ---- Phase 1b (serial, nur Speicher, nichts mutiert): vorrechnen. ----
	//
	// Jede Ueberweisung wird in Blockreihenfolge gegen LAUFENDE Staende
	// geprueft -- genau die Staende, die der serielle Pfad
	// (applyTransferDeltaLockedSammelnd) an derselben Stelle saehe:
	//
	//   - Deckung: Stand des Absenders nach allem, was er im Lauf davor
	//     bekommen und gesendet hat.
	//   - Wohlstandsgrenze: Stand des Empfaengers nach dieser Gutschrift, mit
	//     seinen LP-Anteilen (wuerdeKappenLocked).
	//
	// Die erste Ueberweisung, die scheitern oder gekappt wuerde, beendet das
	// Praefix; sie und alles danach uebernimmt der serielle Pfad, der Fehler,
	// Kappung, Pool-Gutschriften und Protokollzeilen unveraendert erzeugt.
	// Das haelt diesen Pfad bei dem, was er verspricht: eine reine
	// Beschleunigung, deren Ablehnungen nur Tempo kosten.
	//
	// Geschichte: bis 15.08.2026 fehlte die Grenzpruefung ganz (belegt von
	// TestParallelReplay_EnforcesWealthCapLikeSerial: 26.000 statt 25.000
	// AEQ, andere StateRoot), bis 26.09. fehlten die LP-Anteile
	// (TestSchnellpfade_KappenWieSeriellMitLPAnteilen).
	//
	// Warum vorrechnen statt Block-STM (optimistisch parallel, bei Konflikt
	// neu): Block-STM lohnt, wenn die Ausfuehrung einer Transaktion teuer
	// ist. Hier ist sie eine Ganzzahl-Addition; teuer sind Signaturpruefung
	// (vorab parallel, Stufe 1.2) und Datenbank (Phase 1 und 3). Die serielle
	// Vorrechnung kostet Nanosekunden je Ueberweisung und ist ohne jede
	// Wiederholung deterministisch.
	//
	// Die Grenze bleibt im Lauf fest: Ueberweisungen aendern die Geldmenge
	// nicht, der Pool (LP-Wert) wird nicht beruehrt, und eine Kappung kommt
	// nie ins Buendel.
	capAmt, hasCap := cs.wealthCapAmountLocked()
	laufend := make(map[string]Decimal, len(items)*2)
	standVon := func(key string, acc *AccountState) Decimal {
		if d, ok := laufend[key]; ok {
			return d
		}
		return acc.Balance
	}
	for i := range items {
		it := &items[i]
		betrag := NewDecimal(it.amount)
		vonVorher := standVon(it.fromKey, it.from)
		schlecht := false
		if vonVorher.Float() < it.amount {
			merkeBuendelAblehnung(&baGuthaben)
			schlecht = true
		}
		var vonNach, anNach Decimal
		if !schlecht {
			vonNach = vonVorher.Sub(betrag)
			anNach = standVon(it.toKey, it.to).Add(betrag)
			if cs.wuerdeKappenLocked(it.toKey, it.to, anNach.Float(), capAmt, hasCap) {
				merkeBuendelAblehnung(&baWohlstandsCap)
				schlecht = true
			}
		}
		if schlecht {
			if i < parallelReplayMinBatch {
				return 0, nil // kein brauchbares Praefix uebrig
			}
			items = items[:i]
			merkeBuendelGekuerzt(len(batch) - i)
			break
		}
		laufend[it.fromKey] = vonNach
		laufend[it.toKey] = anNach
		it.fromNach = vonNach.Float()
		it.toNach = anNach.Float()
	}

	// ---- Phase 2 (parallel): pure in-memory arithmetic, NO database. ----
	//
	// Eine Arbeitseinheit je KONTO, nicht je Ueberweisung: jedes Konto
	// bekommt die Summe aller seiner Gutschriften minus aller Belastungen im
	// Praefix. So beruehrt jede AccountState genau ein Worker, egal wie oft
	// sie im Lauf vorkommt.
	//
	// Bitgleich mit dem seriellen Pfad:
	//   - Kontostand: jeder Betrag wird wie dort einzeln mit NewDecimal in
	//     Mikro-AEQ gewandelt, dann ganzzahlig addiert (Decimal ist int64).
	//     Die Summe haengt nicht von der Reihenfolge ab. Zwischenstaende
	//     unter null gibt es nicht -- das hat Phase 1b geprueft.
	//   - StateRoot: updateAccountLeafLocked tauscht das zuletzt gezaehlte
	//     Blatt gegen das aktuelle; einmal mit dem Endstand ergibt dasselbe
	//     XOR wie einmal je Ueberweisung.
	//   - Demurrage-Uhr: alle Ueberweisungen tragen denselben Blockzeitpunkt.
	//     Wer im Lauf sendet, bekommt touchActivityAt -- ein Empfangen davor
	//     (startClockIfUnsetAt) oder danach aendert daran nichts. Wer nur
	//     empfaengt, bekommt startClockIfUnsetAt; mehrfach ist das idempotent.
	//     Belegt von TestNachspielen_SeriellUndParallelGleicheEmpfaengerUhr
	//     (annahme_gegen_nachspielen_test.go) fuer den Fall, dass hier
	//     faelschlich die Uhr eines reinen Empfaengers zurueckgesetzt wird.
	type einheit struct {
		acc      *AccountState
		delta    Decimal
		gesendet bool
	}
	einheiten := make([]einheit, 0, len(items)*2)
	index := make(map[string]int, len(items)*2)
	einheitFuer := func(key string, acc *AccountState) *einheit {
		j, ok := index[key]
		if !ok {
			j = len(einheiten)
			index[key] = j
			einheiten = append(einheiten, einheit{acc: acc})
		}
		return &einheiten[j]
	}
	for _, it := range items {
		betrag := NewDecimal(it.amount)
		von := einheitFuer(it.fromKey, it.from)
		von.delta = von.delta.Sub(betrag)
		von.gesendet = true
		an := einheitFuer(it.toKey, it.to)
		an.delta = an.delta.Add(betrag)
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
				e.acc.Balance = e.acc.Balance.Add(e.delta)
				if e.gesendet {
					touchActivityAt(e.acc, activityAt)
				} else {
					// EMPFANGEN STARTET DIE UHR, ES SETZT SIE NIE ZURUECK --
					// wie applyTransferDeltaLockedSammelnd (state.go).
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
	// Stufe 1.3 kann jede Adresse mehrfach vorkommen, und die Buchkonten
	// summieren Umsatz und Tageswerte in Gleitkomma -- deshalb zwingend die
	// Blockreihenfolge, mit den Staenden nach genau dieser Ueberweisung
	// (fromNach, toNach aus Phase 1b). Dieselben Aufrufe mit denselben Werten
	// in derselben Folge wie im seriellen Pfad. Die Gebuehr ist hier immer 0:
	// Ueberweisungen mit Gebuehr kommen nicht ins Buendel
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
			it.amount, 0, it.fromNach, it.toNach, it.buchAt); err != nil {
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
