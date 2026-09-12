package keeper

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
)

// SCALING_ARCHITECTURE.md roadmap step 4 — block bodies by reference.
//
// THE CEILING THIS REMOVES (from that document's own measurements):
//
//	TXs/Block   calculateBlockHash   json.Unmarshal   Payload
//	   10,000              37.6 ms          37.9 ms    2.32 MB
//	   50,000              83.1 ms         191.7 ms   11.59 MB
//	  100,000             148.3 ms         379.1 ms   23.17 MB
//
// At 50,000 transactions a receiver spends ~275 ms on hash verification and
// decoding alone, before any network transfer of an 11.6 MB payload and
// before replay. That is why maxTxsPerBlock exists -- it stands at 10,000
// since 2026-08-21, not the 20,000 this comment used to name -- and why the
// document states plainly that block relay caps throughput at 10,000-20,000
// TPS no matter how fast the storage layer becomes.
//
// The fix is the one Narwhal established and Mysticeti refined: separate data
// dissemination from ordering. Consensus orders 32-byte references; the
// bodies travel on their own path.
//
// WHY THIS IS NOT A CONSENSUS BREAK (the discovery that made this cheap):
//
// calculateBlockHash already commits to transactions INDIRECTLY, through a
// single tx_root field:
//
//	txRoot := sha256(json(b.Transactions))
//	hash   := sha256(json{height, timestamp, parent_hashes, proposer,
//	                      humans, state_root, tx_root})
//
// The hash preimage therefore contains one 32-byte digest, never the
// transaction list itself. Carrying that digest explicitly on the block —
// rather than always recomputing it from an attached body — leaves the
// preimage byte-identical. Blocks hash exactly as they always did, so no
// activation height and no fork are required for the commitment. Only the
// TRANSPORT changes.
//
// SECURITY: a body fetched by reference is never trusted. It is accepted only
// if its own digest equals the TxRoot the (signed) block committed to, so a
// peer cannot substitute different transactions than the producer signed.

// txBatchRoot computes the digest a block commits to for a transaction list.
// It must match calculateBlockHash's own computation exactly, including the
// nil-to-empty normalisation that exists because `omitempty` strips the field
// in transport and the receiver would otherwise decode nil and derive a
// different root.
func txBatchRoot(txs []Transaction) string {
	if txs == nil {
		txs = []Transaction{}
	}
	data, _ := json.Marshal(txs)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// txBatchCache keeps recently produced/fetched bodies in memory so the common
// case (a body that arrived moments ago, or one this node produced itself)
// never touches the database.
type txBatchCache struct {
	mu    sync.RWMutex
	items map[string][]Transaction
	order []string
	txs   int // Transaktionen ueber alle Eintraege -- das ist, was Speicher kostet
}

// txBatchCacheMax deckelt die Eintraege, txBatchCacheMaxTxs die
// Transaktionen darin. Erst der zweite Deckel begrenzt den Speicher.
//
// GEMESSEN AM 12.09.2026 (Heap-Profil C2 nach dem Lasttest, 2,4 GB lebend):
// rund 0,9 GB davon waren dekodierte Ruempfe in diesem Cache -- 512 Eintraege
// zu je 7.000 Transaktionen sind 3,6 Millionen Transaktionen im Speicher,
// obwohl die DAG-Ausduennung die Ruempfe aus den Bloecken selbst laengst
// entfernt hatte. Der Cache fuellte sich sogar weiter, waehrend nur der
// Lasttest alte Seiten ueber /api/blocks las: jeder nachgeladene Rumpf landete
// hier. Gebraucht wird der Cache fuer Sekunden -- bis der Partner den Rumpf
// des eben produzierten Blocks geholt hat. 100.000 Transaktionen sind rund 14
// volle Bloecke; mehr haelt er nicht.
const (
	txBatchCacheMax    = 512
	txBatchCacheMaxTxs = 100_000
)

func newTxBatchCache() *txBatchCache {
	return &txBatchCache{items: make(map[string][]Transaction, txBatchCacheMax)}
}

// Both accessors tolerate a nil receiver: ChainState values built directly in
// tests (rather than through NewChainState) have no cache, and a body store
// is an optimisation — its absence must degrade to "not cached", never panic.
func (c *txBatchCache) get(root string) ([]Transaction, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	txs, ok := c.items[root]
	return txs, ok
}

func (c *txBatchCache) put(root string, txs []Transaction) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.items[root]; exists {
		return
	}
	c.items[root] = txs
	c.order = append(c.order, root)
	c.txs += len(txs)
	// Plain FIFO eviction: bodies are needed briefly, around the moment their
	// block is replayed, so recency of insertion tracks usefulness closely
	// enough and costs nothing to maintain. Der juengste Eintrag bleibt immer,
	// auch wenn er allein ueber dem Transaktionsdeckel liegt.
	for len(c.order) > 1 && (len(c.order) > txBatchCacheMax || c.txs > txBatchCacheMaxTxs) {
		oldest := c.order[0]
		c.order = c.order[1:]
		c.txs -= len(c.items[oldest])
		delete(c.items, oldest)
	}
}

// Stand fuer die Anzeige: Eintraege und Transaktionen im Cache.
func (c *txBatchCache) stand() (eintraege, txs int) {
	if c == nil {
		return 0, 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items), c.txs
}

// TxBatchCacheStand fuer /api/health/combined.
func (cs *ChainState) TxBatchCacheStand() map[string]interface{} {
	e, n := cs.txBatches.stand()
	return map[string]interface{}{
		"bedeutung": "Dekodierte Block-Ruempfe im Speicher, gedeckelt auf " + fmt.Sprint(txBatchCacheMaxTxs) +
			" Transaktionen. Vor dem 12.09.2026 hielt der Cache 512 Ruempfe ohne Ruecksicht auf ihre Groesse -- unter Last 0,9 GB.",
		"eintraege":     e,
		"transaktionen": n,
		"deckel_txs":    txBatchCacheMaxTxs,
	}
}

// ensureTxBatchTable creates the body store. Separate from chain_blocks on
// purpose: a node that received a block by reference has the header but not
// the body, so the two cannot share a row, and a body may be referenced by
// more than one block (identical transaction sets produce identical roots).
func (cs *ChainState) ensureTxBatchTable() {
	if cs.db == nil {
		return
	}
	cs.txBatchTableOnce.Do(func() {
		cs.db.Exec(`CREATE TABLE IF NOT EXISTS chain_tx_batches (
			root       TEXT PRIMARY KEY,
			txs        TEXT NOT NULL,
			created_at BIGINT NOT NULL
		)`)
	})
}

// SaveTxBatch persists a body under its own digest. Idempotent: the same
// transaction list always yields the same root, so a repeat insert is a
// no-op rather than a conflict.
func (cs *ChainState) SaveTxBatch(root string, txs []Transaction) error {
	if root == "" || len(txs) == 0 {
		return nil
	}
	// DER SPEICHER SYNCHRON, DIE PLATTE NICHT.
	//
	// Diese Funktion wird aus ProduceBlock gerufen, und die haelt dag.mu
	// EXKLUSIV -- der Kommentar an der Aufrufstelle sagt es selbst: "if it's
	// slow, EVERY other dag.mu consumer stalls for the same duration". Genau
	// das ist eingetreten. Der Wachhund (sperren_wachhund.go) zog am
	// 07.09.2026 einen Abzug, waehrend ProduceBlock 3 Sekunden auf die Sperre
	// wartete, und fand als EINZIGE laufende Goroutine des Pakets:
	//
	//	goroutine 176 [runnable]: SaveTxBatch(..., {..., 0x168f, 0x1698})
	//	  /app/x/humanity/keeper/tx_batch.go:143
	//
	// 0x168f sind 5.775 Ueberweisungen. Bei dieser Groesse kostet allein das
	// json.Marshal ein Vielfaches der Blockzeit, der Insert derselben gut
	// 1,2 MB noch einmal -- und beides lief unter der exklusiven Sperre.
	// Gemessen wurden dadurch Wartezeiten von 3.846 ms (C1) und 19.262 ms
	// (C2) auf einen Block, der selbst keine einzige Transaktion trug.
	//
	// Der Zwischenspeicher bleibt synchron: er ist eine Map-Zuweisung, und
	// LoadTxBatch schaut zuerst dort nach. Ein Peer, der den Rumpf sofort
	// anfragt, wird also weiterhin bedient, ohne dass die Platte fertig sein
	// muss. Nur Serialisierung und Insert wandern hinaus.
	//
	// Der Aufrufer behandelt einen Fehler hier ohnehin als nicht toedlich
	// ("peers get the block with its body inline, as they always did"), also
	// verliert der Weg nach draussen keine Zusicherung, die vorher galt.
	cs.txBatches.put(root, txs)
	if cs.db == nil {
		return nil
	}
	SafeGoroutine("saveTxBatch", func() {
		cs.speichereTxBuendelDauerhaft(root, txs)
	})
	return nil
}

// speichereTxBuendelDauerhaft ist der teure Teil von SaveTxBatch, ausgelagert,
// damit er nicht unter dag.mu laeuft. Siehe dort.
func (cs *ChainState) speichereTxBuendelDauerhaft(root string, txs []Transaction) {
	cs.ensureTxBatchTable()
	data, err := json.Marshal(txs)
	if err != nil {
		fmt.Printf("[TX] ⚠ Konnte das Transaktionsbuendel %s nicht kodieren: %v — der Rumpf bleibt im Zwischenspeicher\n", root, err)
		return
	}
	// cs.db directly, never dbExec(): this is a standalone write that must not
	// join whatever transaction some other goroutine happens to have open —
	// see SaveBlockToDB's own comment for the wire-protocol corruption that
	// caused in production on 2026-07-25.
	if _, err := cs.db.Exec(
		`INSERT INTO chain_tx_batches (root, txs, created_at) VALUES ($1,$2,$3)
		 ON CONFLICT (root) DO NOTHING`,
		root, string(data), nowUnix(),
	); err != nil {
		fmt.Printf("[TX] ⚠ Konnte das Transaktionsbuendel %s nicht speichern: %v — der Rumpf bleibt im Zwischenspeicher\n", root, err)
	}
}

// LoadTxBatch returns a body by digest, from memory or the database.
func (cs *ChainState) LoadTxBatch(root string) ([]Transaction, bool) {
	return cs.ladeTxBatch(root, true)
}

// LoadTxBatchOhneCache ist LoadTxBatch fuer Leser alter Bloecke (API-Seiten,
// nachgeladene Ruempfe): ein Treffer im Cache wird genutzt, ein Fehltreffer
// aber NICHT hineingelegt. Sonst verdraengt ein Explorer, der Geschichte
// blaettert, genau die Ruempfe, die der Partner in der naechsten Sekunde
// holen will.
func (cs *ChainState) LoadTxBatchOhneCache(root string) ([]Transaction, bool) {
	return cs.ladeTxBatch(root, false)
}

// HasTxBatch sagt, ob ein Rumpf ausgeliefert werden koennte -- ohne ihn zu
// laden. Der Cache antwortet sofort, sonst eine Ein-Zeilen-Abfrage. Vorher
// lud der Pruefer den ganzen Rumpf und dekodierte ihn, nur um ihn wegzuwerfen.
func (cs *ChainState) HasTxBatch(root string) bool {
	if root == "" {
		return false
	}
	if _, ok := cs.txBatches.get(root); ok {
		return true
	}
	if cs.db == nil {
		return false
	}
	cs.ensureTxBatchTable()
	var eins int
	return cs.db.QueryRow(`SELECT 1 FROM chain_tx_batches WHERE root = $1`, root).Scan(&eins) == nil
}

func (cs *ChainState) ladeTxBatch(root string, merken bool) ([]Transaction, bool) {
	if root == "" {
		return nil, false
	}
	if txs, ok := cs.txBatches.get(root); ok {
		return txs, true
	}
	if cs.db == nil {
		return nil, false
	}
	cs.ensureTxBatchTable()
	var data string
	if err := cs.db.QueryRow(`SELECT txs FROM chain_tx_batches WHERE root = $1`, root).Scan(&data); err != nil {
		return nil, false
	}
	var txs []Transaction
	if err := json.Unmarshal([]byte(data), &txs); err != nil {
		return nil, false
	}
	if merken {
		cs.txBatches.put(root, txs)
	}
	return txs, true
}

// AttachTxBatch validates a body against the root its block committed to and,
// only if it matches, stores it and attaches it to the block.
//
// This is the security boundary of the whole scheme: the block is signed by
// its producer and commits to exactly one digest, so a peer serving a body
// cannot substitute different transactions. A mismatch is reported rather
// than silently ignored, because it means either corruption in transit or a
// peer actively serving something the producer never signed.
func (cs *ChainState) AttachTxBatch(block *Block, txs []Transaction) error {
	if block == nil {
		return fmt.Errorf("attach tx batch: nil block")
	}
	want := block.TxRoot
	if want == "" {
		return fmt.Errorf("attach tx batch: block #%d carries no tx_root to verify against", block.Height)
	}
	got := txBatchRoot(txs)
	if got != want {
		return fmt.Errorf("attach tx batch for block #%d: body digest %s does not match the committed tx_root %s — the peer served transactions the producer never signed",
			block.Height, got[:min(16, len(got))], want[:min(16, len(want))])
	}
	block.Transactions = txs
	if err := cs.SaveTxBatch(want, txs); err != nil {
		return fmt.Errorf("attach tx batch for block #%d: %w", block.Height, err)
	}
	return nil
}

// NeedsTxBatch reports whether this block references a body that is not
// present locally — i.e. it arrived by reference and cannot be replayed yet.
func (cs *ChainState) NeedsTxBatch(block *Block) bool {
	if block == nil || block.TxRoot == "" || len(block.Transactions) > 0 {
		return false
	}
	// An empty body is legitimately empty: the root of an empty list is a
	// fixed, known digest, and blocks in normal operation carry no
	// transactions at all. Those must never be treated as "body missing".
	return block.TxRoot != txBatchRoot(nil)
}
