package keeper

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Bloecke aus dem Speicher statt ueber Postgres (Stufe 1 in
// docs/KONSENS_UND_AUSFUEHRUNG_2026.md).
//
// GEMESSEN AM 01.10.2026 (C1-Pruefstand, Lauf 6, /api/produktion je
// Versuch): ohne Last dauert ein Block 3-12 ms, unter Last 300-2.500 ms --
// fast alles in zwei Postgres-Schritten: LADEN der offenen Ueberweisungen
// (100-1.144 ms) und SPEICHERN des Blocks (150-1.363 ms). Jede Ueberweisung
// lief durch Postgres hin und zurueck: WAL -> Flush in pending_txs -> der
// Block laedt sie aus derselben Tabelle -> loescht sie beim Speichern,
// waehrend der Flush schon die naechsten 10.000 Zeilen hineinschreibt. Die
// Bloecke dauerten laenger als der Takt, Zusatzbloecke fielen aus, die Kette
// blieb bei ~8.000/s, obwohl 18.000-24.000 Ueberweisungen warteten.
//
// DER SPEICHERKORB. Eine angenommene Schnellpfad-Ueberweisung steht nach dem
// WAL-Append zusaetzlich hier, geordnet nach WAL-Seq. Der Blockbau nimmt das
// Praefix, das schon haltbar ist (Seq <= DurableSeq), und speichert mit dem
// Block in DERSELBEN Datenbanktransaktion die Marke "aufgenommen bis Seq S"
// (chain_config.speicherkorb_bis). Die Zeile in pending_txs entfaellt fuer
// diese Ueberweisungen; die Kontostaende schreibt der Flush weiter.
//
// WARUM DAS ABSTURZSICHER IST.
//   - Jede angenommene Ueberweisung steht haltbar im WAL, bevor sie quittiert
//     wird (unveraendert). Der Korb ist nur eine Abschrift.
//   - Absturz VOR dem Speichern eines Blocks: die Marke steht noch auf dem
//     alten S. Beim Start kommt jeder WAL-Datensatz mit Seq > S wieder in den
//     Korb (recoverFromWAL). Nichts geht verloren.
//   - Absturz NACH dem Speichern: die Marke S' steht mit dem Block in einer
//     Transaktion. Datensaetze <= S' kommen nicht wieder. Nichts doppelt.
//   - Die WAL-Kompaktierung kuerzt nie ueber S (wal_kompaktierung.go).
//
// WARUM DIE REIHENFOLGE STIMMT. Der Block muss die Ueberweisungen in der
// Reihenfolge tragen, in der sie angewendet wurden (pending_reihenfolge.go):
// das ist die WAL-Seq. Der Korb gibt nur ein LUECKENLOSES Praefix frei: Seq
// n+1 erst, wenn n da ist. Zwei Ueberweisungen, die ein Konto teilen, halten
// dieselbe Shard-Sperre ueber Append UND Einreihen, kommen also in Seq-
// Reihenfolge an; fremde koennen sich ueberholen, warten dann aber kurz in
// `warten`. Zeilen anderer Herkunft (Registrierung, Swap, ...) bleiben in
// pending_txs und werden nach ihrer wal_seq einsortiert -- und nur
// aufgenommen, wenn alle Schnellpfad-Ueberweisungen mit kleinerer Seq im
// selben oder einem frueheren Block stehen (blockKorbMischen).
//
// WAS SICH NICHT AENDERT. Jeder Knoten prueft fremde Bloecke wie bisher
// selbst (Signatur, Nonce, Deckung beim Nachspielen). Der Deckel je Block,
// Inflight, Rueckstau, WAL-Druck und das Rate-Limit gelten unveraendert.
//
// Schalter: AEQUITAS_BLOCK_AUS_SPEICHER=1 (Standard aus). Gelesen beim Start.

const speicherKorbEnv = "AEQUITAS_BLOCK_AUS_SPEICHER"

// speicherKorbMarke: Schluessel in chain_config. Wert = hoechste WAL-Seq, bis
// zu der alle Schnellpfad-Ueberweisungen in gespeicherten Bloecken stehen.
const speicherKorbMarke = "speicherkorb_bis"

// speicherKorbLueckeFrist: so lange darf die naechste erwartete Seq fehlen,
// waehrend spaetere schon da sind. Zwischen Append und Einreihen liegen nur
// Mikrosekunden derselben Goroutine; fehlt sie laenger, ist etwas kaputt
// (z. B. Panik dazwischen). Dann wird nichts mehr aus dem Korb verblockt,
// bis ein Neustart den Korb aus dem WAL neu aufbaut (fail-closed).
var speicherKorbLueckeFrist = 30 * time.Second

func speicherKorbGewuenscht() bool { return os.Getenv(speicherKorbEnv) == "1" }

type korbEintrag struct {
	seq uint64
	tx  Transaction
}

// speicherKorb haelt angenommene, noch nicht verblockte Schnellpfad-
// Ueberweisungen in WAL-Reihenfolge. Begrenzt ist er durch die Annahme:
// der Rueckstau (rueckstau_grenze.go) zaehlt ihn mit und weist darueber ab;
// `warten` haelt hoechstens so viele Eintraege, wie Ueberweisungen
// gleichzeitig zwischen Append und Einreihen stehen (Inflight-Grenze).
type speicherKorb struct {
	mu       sync.Mutex
	naechste uint64 // naechste Seq, die an `bereit` angehaengt werden darf
	bereit   []korbEintrag
	warten   map[uint64]Transaction
	// Seit wann fehlt `naechste`, obwohl Spaeteres wartet (0 = keine Luecke).
	lueckeSeit time.Time

	// bauer: ein Blockbauer haelt den Korb vom Nehmen bis zum Speichern oder
	// Zuruecklegen. Ein zweiter gleichzeitiger bekaeme sonst die spaeteren
	// Eintraege, speicherte womoeglich zuerst -- und die Marke stuende ueber
	// Eintraegen, die der erste noch zuruecklegt (beim Wiederanlauf verloren).
	bauer atomic.Bool

	doppelt  atomic.Int64 // Seq kam ein zweites Mal (z. B. Wiederanlauf), ignoriert
	genommen atomic.Int64
	zurueck  atomic.Int64
}

func neuerSpeicherKorb(ab uint64) *speicherKorb {
	if ab == 0 {
		ab = 1 // WAL-Seqs beginnen bei 1
	}
	return &speicherKorb{naechste: ab, warten: make(map[uint64]Transaction)}
}

// hinzu reiht eine Ueberweisung ein. Seqs unter `naechste` sind schon im
// Korb oder verblockt und werden ignoriert.
func (k *speicherKorb) hinzu(seq uint64, tx Transaction) {
	k.mu.Lock()
	defer k.mu.Unlock()
	switch {
	case seq < k.naechste:
		k.doppelt.Add(1)
		return
	case seq > k.naechste:
		if _, schon := k.warten[seq]; schon {
			k.doppelt.Add(1)
			return
		}
		k.warten[seq] = tx
		if k.lueckeSeit.IsZero() {
			k.lueckeSeit = time.Now()
		}
		return
	}
	k.bereit = append(k.bereit, korbEintrag{seq: seq, tx: tx})
	k.naechste++
	for {
		t, ok := k.warten[k.naechste]
		if !ok {
			break
		}
		delete(k.warten, k.naechste)
		k.bereit = append(k.bereit, korbEintrag{seq: k.naechste, tx: t})
		k.naechste++
	}
	if len(k.warten) == 0 {
		k.lueckeSeit = time.Time{}
	} else {
		k.lueckeSeit = time.Now()
	}
}

// nehmen gibt hoechstens n Eintraege vom Anfang zurueck, deren Seq schon
// haltbar ist (<= haltbarBis). Steht eine Luecke laenger als die Frist,
// gibt der Korb nichts mehr her (fail-closed, siehe speicherKorbLueckeFrist).
func (k *speicherKorb) nehmen(n int, haltbarBis uint64) []korbEintrag {
	if n <= 0 {
		return nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.lueckeSeit.IsZero() && time.Since(k.lueckeSeit) > speicherKorbLueckeFrist {
		return nil
	}
	m := 0
	for m < len(k.bereit) && m < n && k.bereit[m].seq <= haltbarBis {
		m++
	}
	if m == 0 {
		return nil
	}
	out := make([]korbEintrag, m)
	copy(out, k.bereit[:m])
	k.bereit = k.bereit[m:]
	k.genommen.Add(int64(m))
	return out
}

// zurueckLegen legt genommene, aber nicht verblockte Eintraege wieder an den
// Anfang -- in derselben Reihenfolge. Es gibt nur einen Blockbauer
// gleichzeitig; dazwischen kann nichts Kleineres nachgekommen sein.
func (k *speicherKorb) zurueckLegen(e []korbEintrag) {
	if len(e) == 0 {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	neu := make([]korbEintrag, 0, len(e)+len(k.bereit))
	neu = append(neu, e...)
	neu = append(neu, k.bereit...)
	k.bereit = neu
	k.zurueck.Add(int64(len(e)))
}

func (k *speicherKorb) laenge() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.bereit) + len(k.warten)
}

// verwerfen leert den Korb und liefert die hoechste je eingereihte Seq --
// fuer den ueberholten Leiter (leitung_netz.go), der Unverteiltes verwirft.
func (k *speicherKorb) verwerfen() (anzahl int, bisSeq uint64) {
	k.mu.Lock()
	defer k.mu.Unlock()
	anzahl = len(k.bereit) + len(k.warten)
	bisSeq = k.naechste - 1
	for s := range k.warten {
		if s > bisSeq {
			bisSeq = s
		}
	}
	k.bereit = nil
	k.warten = make(map[uint64]Transaction)
	k.lueckeSeit = time.Time{}
	k.naechste = bisSeq + 1
	return anzahl, bisSeq
}

func (k *speicherKorb) stand() map[string]interface{} {
	k.mu.Lock()
	defer k.mu.Unlock()
	luecke := 0.0
	if !k.lueckeSeit.IsZero() {
		luecke = time.Since(k.lueckeSeit).Seconds()
	}
	return map[string]interface{}{
		"bereit":       len(k.bereit),
		"wartend":      len(k.warten),
		"naechste_seq": k.naechste,
		"luecke_s":     luecke,
		"doppelt":      k.doppelt.Load(),
		"genommen":     k.genommen.Load(),
		"zurueck":      k.zurueck.Load(),
	}
}

// langsameZeile: eine Zeile aus pending_txs (Weg ausserhalb des
// Schnellpfads), mit ihrer wal_seq.
type langsameZeile struct {
	id     int64
	walSeq int64
	tx     Transaction
}

// blockKorbMischen setzt den Inhalt eines Blocks aus Korb und pending_txs
// zusammen -- rein, ohne Zustand, damit die Regel testbar ist.
//
// Reihenfolge: nach Seq; bei gleicher Seq die langsame Zeile zuerst (sie
// bekam die Seq, die der WAL als NAECHSTES vergeben wuerde, ist also vor der
// Schnellpfad-Ueberweisung mit dieser Seq angewendet worden).
//
// Eine langsame Zeile mit Seq X darf nur hinein, wenn alle Schnellpfad-
// Ueberweisungen mit Seq < X im Block oder davor stehen: X <= bis+1, wobei
// bis die hoechste danach verblockte Schnellpfad-Seq ist.
//
// Rueckgabe: die Transaktionen des Blocks; die genommenen Korb-Eintraege;
// die IDs der aufgenommenen Zeilen; was zurueck in den Korb muss; welche
// Zeilen-IDs freizugeben sind; die neue Marke.
//
// zeilenPos: an welchen Stellen von txs Zeilen stehen (aufsteigend) -- fuer
// korbPraefix, wenn ein Block nur einen Anfang davon tragen kann
// (block_tauglich.go).
func blockKorbMischen(korb []korbEintrag, zeilen []langsameZeile, deckel int, altBis uint64) (
	txs []Transaction, genommen []korbEintrag, ids []int64, zurueck []korbEintrag, freigeben []int64, neuBis uint64, zeilenPos []int) {

	sort.SliceStable(zeilen, func(i, j int) bool {
		if zeilen[i].walSeq != zeilen[j].walSeq {
			return zeilen[i].walSeq < zeilen[j].walSeq
		}
		return zeilen[i].id < zeilen[j].id
	})
	neuBis = altBis
	// Kapazitaet vorab: ohne sie wuchsen txs und genommen beim Anhaengen in
	// Stufen und wurden dabei mehrfach ganz umkopiert -- bei 7.000 grossen
	// Transaction-Werten je Block 10 % aller Allokationen des Knotens
	// (Pruefstand Lauf 18, alloc_space).
	if n := len(korb) + len(zeilen); n > 0 {
		if n > deckel {
			n = deckel
		}
		if n < 0 {
			n = 0
		}
		txs = make([]Transaction, 0, n)
		m := len(korb)
		if m > n {
			m = n
		}
		genommen = make([]korbEintrag, 0, m)
	}
	i, j := 0, 0
	for len(txs) < deckel && (i < len(korb) || j < len(zeilen)) {
		nimmZeile := false
		if j < len(zeilen) {
			z := zeilen[j]
			if i >= len(korb) {
				nimmZeile = true
			} else if z.walSeq <= int64(korb[i].seq) {
				nimmZeile = true
			}
			// Nur, wenn alle kleineren Schnellpfad-Seqs schon drin sind.
			if nimmZeile && z.walSeq > int64(neuBis)+1 {
				break
			}
		}
		if nimmZeile {
			zeilenPos = append(zeilenPos, len(txs))
			txs = append(txs, zeilen[j].tx)
			ids = append(ids, zeilen[j].id)
			j++
			continue
		}
		txs = append(txs, korb[i].tx)
		genommen = append(genommen, korb[i])
		neuBis = korb[i].seq
		i++
	}
	if i < len(korb) {
		zurueck = append(zurueck, korb[i:]...)
	}
	for ; j < len(zeilen); j++ {
		freigeben = append(freigeben, zeilen[j].id)
	}
	return
}

// SpeicherKorbStand fuer /api/health/combined.
func (cs *ChainState) SpeicherKorbStand() map[string]interface{} {
	k := cs.korb
	if k == nil {
		return map[string]interface{}{"an": false, "gewuenscht": speicherKorbGewuenscht()}
	}
	s := k.stand()
	s["an"] = true
	s["flush_teile"] = walFlushTeileWert
	s["flush_aufgeteilt"] = walFlushTeileLaeufe.Load()
	s["flush_zusammengefasst"] = walFlushZusammengefasst.Load()
	s["bis"] = cs.korbBis.Load()
	s["bedeutung"] = "Bloecke aus dem Speicher (" + speicherKorbEnv + "=1): angenommene Schnellpfad-Ueberweisungen in WAL-Reihenfolge; bis = hoechste Seq, die in einem gespeicherten Block steht (mit dem Block in einer Transaktion gesichert)."
	return s
}

// speicherKorbMarkeLesen liest die Marke aus chain_config. ok=false, wenn es
// keine gibt (Korb war nie an).
func (cs *ChainState) speicherKorbMarkeLesen() (uint64, bool, error) {
	v, ok := cs.getConfigValueExistsDB(speicherKorbMarke)
	if !ok || v == "" {
		return 0, false, nil
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("%s unlesbar (%q): %w", speicherKorbMarke, v, err)
	}
	return n, true, nil
}

// SaveBlockMitKorb speichert den Block, loescht die aufgenommenen
// pending_txs-Zeilen und schreibt die Korb-Marke -- alles in EINER
// Transaktion. Scheitert irgendetwas, steht nichts davon.
func (cs *ChainState) SaveBlockMitKorb(block *Block, ids []int64, bis uint64) error {
	if cs.db == nil {
		return nil
	}
	t0 := time.Now()
	args, err := cs.blockZeileArgs(block)
	if err != nil {
		return err
	}
	tuSpeichernArgs.seit(t0)
	defer tuSpeichernDB.seit(time.Now())
	tx, err := cs.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if _, err := tx.Exec(blockZeileInsertSQL, args...); err != nil {
		tx.Rollback()
		return fmt.Errorf("save block: %w", err)
	}
	if len(ids) > 0 {
		if _, err := tx.Exec(`DELETE FROM pending_txs WHERE id = ANY($1)`, ids); err != nil {
			tx.Rollback()
			return fmt.Errorf("clear pending txs: %w", err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO chain_config (key, value) VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, speicherKorbMarke, strconv.FormatUint(bis, 10)); err != nil {
		tx.Rollback()
		return fmt.Errorf("korb-marke: %w", err)
	}
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// speicherKorbNachWiederanlauf schliesst den Start ab, nachdem das WAL
// nachgespielt und geoeffnet ist (peek = naechste Seq, die der WAL vergibt).
func (cs *ChainState) speicherKorbNachWiederanlauf(peek uint64, korbAn, markeDa bool) error {
	if cs.korbUebergang {
		// Korb ausgeschaltet: die Zeilen sind nachgeholt (recoverFromWAL),
		// ab jetzt der alte Weg. Die Marke weg, sonst holte ein spaeterer
		// Start dieselben Datensaetze noch einmal (die Abfrage dort ist zwar
		// gegen Doppelte geschuetzt, aber nicht gegen inzwischen Verblocktes).
		cs.korbUebergang = false
		if cs.db != nil {
			if _, err := cs.db.Exec(`DELETE FROM chain_config WHERE key = $1`, speicherKorbMarke); err != nil {
				return fmt.Errorf("Korb-Marke nicht entfernt: %w", err)
			}
		}
		fmt.Printf("[KORB] aus -- Ueberweisungen ueber Seq %d stehen wieder in pending_txs\n", cs.korbUebergangBis)
		return nil
	}
	if !korbAn {
		return nil
	}
	if peek == 0 {
		peek = 1
	}
	if !markeDa {
		// Erstes Einschalten: alles bis hierher ging den alten Weg (Zeilen in
		// pending_txs, vom Nachspielen eben vervollstaendigt).
		bis := peek - 1
		if err := cs.setConfigValueDB(speicherKorbMarke, strconv.FormatUint(bis, 10)); err != nil {
			return fmt.Errorf("Korb-Marke nicht gesetzt: %w", err)
		}
		cs.korb = neuerSpeicherKorb(bis + 1)
		cs.korbBis.Store(bis)
		fmt.Printf("[KORB] ✓ Bloecke aus dem Speicher -- erstmals an, ab Seq %d\n", bis+1)
		return nil
	}
	k := cs.korb
	if k == nil {
		return fmt.Errorf("Speicherkorb fehlt nach dem Wiederanlauf")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if len(k.warten) > 0 {
		// Eine Luecke MITTEN in der Datei: ein Datensatz fehlt, spaetere sind
		// da. Das ist kein abgeschnittenes Ende, sondern ein Schaden -- nicht
		// weitermachen, als waere nichts.
		return fmt.Errorf("Speicherkorb: Seq %d fehlt im WAL, %d spaetere vorhanden", k.naechste, len(k.warten))
	}
	if k.naechste < peek {
		// Abgeschnittenes Dateiende (Absturz mitten im Schreiben): diese Seqs
		// wurden nie haltbar und nie quittiert.
		fmt.Printf("[KORB] Seq %d..%d nicht im WAL (abgeschnittenes Ende, nie quittiert) -- weiter ab %d\n", k.naechste, peek-1, peek)
		k.naechste = peek
	}
	fmt.Printf("[KORB] ✓ Bloecke aus dem Speicher -- %d Ueberweisung(en) aus dem WAL wieder im Korb, verblockt bis Seq %d\n", len(k.bereit), cs.korbBis.Load())
	return nil
}

// korbUebergangZeile schreibt eine Ueberweisung, die nur im Korb stand, als
// Zeile nach pending_txs -- hoechstens einmal, auch wenn der Uebergang durch
// einen Absturz wiederholt wird.
func (cs *ChainState) korbUebergangZeile(tx Transaction, seq uint64) error {
	data, err := json.Marshal(tx)
	if err != nil {
		return err
	}
	_, err = cs.db.Exec(`INSERT INTO pending_txs (tx_json, created_at, wal_seq)
SELECT $1::text, $2::bigint, $3::bigint
WHERE NOT EXISTS (SELECT 1 FROM pending_txs WHERE wal_seq = $3::bigint AND tx_json = $1::text)`,
		string(data), time.Now().Unix(), int64(seq))
	return err
}

// korbFuerBlock stellt den Inhalt des naechsten eigenen Blocks zusammen:
// haltbare Korb-Eintraege und die langsamen Zeilen aus pending_txs, die
// davor oder dazwischen gehoeren (blockKorbMischen). Was nicht hineinpasst,
// geht sofort zurueck (Korb) bzw. wird freigegeben (Zeilen).
func (cs *ChainState) korbFuerBlock(deckel int) (txs []Transaction, ids []int64, genommen []korbEintrag, neuBis uint64, gehalten bool) {
	txs, ids, genommen, neuBis, _, gehalten = cs.korbFuerBlockMitPos(deckel)
	return
}

// korbFuerBlockMitPos: wie korbFuerBlock, dazu die Stellen der Zeilen in txs
// (blockKorbMischen, zeilenPos).
func (cs *ChainState) korbFuerBlockMitPos(deckel int) (txs []Transaction, ids []int64, genommen []korbEintrag, neuBis uint64, zeilenPos []int, gehalten bool) {
	k := cs.korb
	alt := cs.korbBis.Load()
	if k == nil || deckel <= 0 {
		return nil, nil, nil, alt, nil, false
	}
	if !k.bauer.CompareAndSwap(false, true) {
		// Ein anderer Blockbau haelt den Korb: dieser Block bleibt leer
		// (fail-closed), statt die Reihenfolge zu riskieren.
		return nil, nil, nil, alt, nil, false
	}
	var haltbar uint64
	if cs.wal != nil {
		haltbar = cs.wal.DurableSeq()
	}
	mem := k.nehmen(deckel, haltbar)
	grenze := alt
	if len(mem) > 0 {
		grenze = mem[len(mem)-1].seq
	}
	var zeilen []langsameZeile
	if cs.db != nil {
		t0 := time.Now()
		zeilen = cs.ladeOffeneZeilen(deckel, int64(grenze)+1)
		tuKorbOffeneZeilen.seit(t0)
	}
	var zurueck []korbEintrag
	var freigeben []int64
	txs, genommen, ids, zurueck, freigeben, neuBis, zeilenPos = blockKorbMischen(mem, zeilen, deckel, alt)
	k.zurueckLegen(zurueck)
	if len(freigeben) > 0 {
		cs.PendingTxIDsFreigeben(freigeben)
	}
	return txs, ids, genommen, neuBis, zeilenPos, true
}

// korbPraefix: von einem aus Korb und Zeilen gemischten Block (in dieser
// Reihenfolge, blockKorbMischen) nur die ersten j Transaktionen behalten.
// Zurueck in den Korb gehen die uebrigen Korb-Eintraege (vorn, in ihrer
// Reihenfolge), freigegeben werden die uebrigen Zeilen; die Marke ist die
// letzte behaltene Korb-Seq (oder die alte). Ein Anfang einer gueltigen
// Mischung ist selbst gueltig: jede behaltene Zeile hatte ihre kleineren
// Schnellpfad-Seqs schon vor sich.
func korbPraefix(genommen []korbEintrag, ids []int64, zeilenPos []int, j int, altBis uint64) (
	behGenommen []korbEintrag, behIds []int64, zurueck []korbEintrag, freigeben []int64, neuBis uint64) {
	zeilen := 0
	for zeilen < len(zeilenPos) && zeilenPos[zeilen] < j {
		zeilen++
	}
	korb := j - zeilen
	if korb < 0 {
		korb = 0
	}
	if korb > len(genommen) {
		korb = len(genommen)
	}
	behGenommen = append([]korbEintrag(nil), genommen[:korb]...)
	zurueck = append([]korbEintrag(nil), genommen[korb:]...)
	behIds = append([]int64(nil), ids[:zeilen]...)
	freigeben = append([]int64(nil), ids[zeilen:]...)
	neuBis = altBis
	if korb > 0 {
		neuBis = genommen[korb-1].seq
	}
	return
}

// korbFreigeben: der Blockbau ist fertig (gespeichert oder zurueckgelegt).
func (cs *ChainState) korbFreigeben() {
	if k := cs.korb; k != nil {
		k.bauer.Store(false)
	}
}
