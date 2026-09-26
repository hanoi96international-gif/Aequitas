package keeper

// STUFE 1.0 -- JEDE UEBERWEISUNG IST VON JEDEM VALIDATOR NACHPRUEFBAR.
//
// Siehe docs/SKALIERUNG_DEZENTRAL.md. Der Befund vom 25.09.2026: Ueberweisungen
// entstehen nur aus vom Absender signierten EVM-Transaktionen
// (eth_sendRawTransaction). Der annehmende Knoten prueft die Signatur -- in den
// Block kamen aber nur Wallet, To, Amount und TxHash. Beim Nachspielen pruefte
// jeder andere Validator nur die Signatur des BLOCKS (AddPeerBlock), nie die
// des Absenders, und die Nonce lebte nur im RPC-Server des annehmenden Knotens.
// Wer den annehmenden Knoten kontrolliert, konnte also Ueberweisungen von
// fremden Konten in Bloecke schreiben, ohne dass es irgendwer bemerken konnte.
// Das ist genau die zentrale Instanz, die Aequitas nicht haben darf -- und
// Stufe 2 (mehrere annehmende Knoten) haette sie auf jeden Validator
// ausgeweitet.
//
// Ab der Aktivierung gilt:
//
//  1. Jede Ueberweisung traegt ihre signierte Rohform (Transaction.Roh).
//  2. Jeder Validator prueft beim Nachspielen, vorab und parallel fuer den
//     ganzen Block (das ist zugleich Stufe 1.2): Absender aus der Signatur ==
//     Wallet, Empfaenger und Betrag == was signiert wurde, Chain-ID 1926,
//     keccak(Roh) == TxHash.
//  3. Schutz gegen das erneute Einreichen im GEMEINSAMEN Zustand: jedes Konto
//     fuehrt NaechsteNonce. Eine Ueberweisung braucht nonce >= NaechsteNonce,
//     danach gilt NaechsteNonce = nonce + 1. Wallets, die schon Nonces ueber 0
//     haben, funktionieren unveraendert. Innerhalb eines Blocks muessen die
//     Nonces desselben Absenders also steigen.
//  4. Ein Verstoss macht den GANZEN Block ungueltig: der Erzeuger hat sich
//     falsch verhalten. Das ist bewusst kein "Ueberweisung ueberspringen" --
//     eine gefaelschte Ueberweisung ist kein Zustandsunterschied, sondern ein
//     Angriff.
//
// Der Erzeuger setzt NaechsteNonce bei der ANNAHME (er spielt seine eigenen
// Bloecke nicht nach), alle anderen beim NACHSPIELEN -- beide landen beim
// selben Wert. Vor der Aktivierung ist der Wert ueberall 0 und geht nicht in
// den Blattwert ein (accountLeaf), alte Bloecke ergeben denselben StateRoot.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/core/types"
)

// signierteUeberweisungenAbUnix: Blockzeit, ab der die Pruefung Pflicht ist.
//
// math.MaxInt64 heisst: AUS. Gesetzt wird der Wert erst, wenn ALLE Knoten
// diese Version laufen -- ein aelterer Knoten verwirft Bloecke mit Roh am
// Blockhash (er kennt das Feld nicht und rechnet ohne es), und ein Knoten
// ohne diese Pruefung wuerde gefaelschte Ueberweisungen weiter annehmen.
// Wie KNIGHTDAG_ACTIVATION_HEIGHT: ein Wert, auf allen Knoten gleich.
//
// 2026-09-26T20:00:00Z, vom Betreiber am 26.09.2026 freigegeben und auf
// seinen Wunsch von 21:00 vorgezogen. Der Vorlauf (Rohform anhaengen) beginnt
// damit sofort nach dem Ausrollen; was davor ohne Rohform angenommen wurde,
// steht nach Sekunden in einem Block, lange vor 20:00. Seit #202 nimmt auch der
// WAL-Schnellpfad signierte Ueberweisungen an (wal_nonce_reihenfolge.go).
// Rueckweg: wieder math.MaxInt64, ausrollen.
const signierteUeberweisungenAbUnix int64 = 1790452800

// signierteUeberweisungenVorlauf: so lange VOR der Aktivierung nehmen
// annehmende Knoten die Rohform schon in ihre Ueberweisungen auf. Sonst
// landete eine kurz vor der Grenze angenommene Ueberweisung ohne Roh in einem
// Block kurz danach -- und jeder andere Validator verwuerfe ihn.
const signierteUeberweisungenVorlauf int64 = 3600

// signierteUeberweisungenOverride: nur fuer Tests (0 = Konstante gilt).
var signierteUeberweisungenOverride atomic.Int64

func signierteUeberweisungenAb() int64 {
	if o := signierteUeberweisungenOverride.Load(); o != 0 {
		return o
	}
	return signierteUeberweisungenAbUnix
}

// signierteUeberweisungenPflicht: gilt die Pruefung fuer einen Block mit
// dieser Blockzeit?
func signierteUeberweisungenPflicht(blockZeit int64) bool {
	return blockZeit >= signierteUeberweisungenAb()
}

// signierteUeberweisungenAufnehmen: soll eine jetzt angenommene Ueberweisung
// ihre Rohform tragen und ihre Nonce gegen den gemeinsamen Zustand pruefen?
func signierteUeberweisungenAufnehmen(jetzt int64) bool {
	ab := signierteUeberweisungenAb()
	return ab != math.MaxInt64 && jetzt >= ab-signierteUeberweisungenVorlauf
}

// ErrUeberweisungNichtSigniert: der Block traegt eine Ueberweisung, die ihr
// Absender so nicht unterschrieben hat (oder deren Nonce verbraucht ist).
var ErrUeberweisungNichtSigniert = errors.New("Ueberweisung nicht vom Absender signiert")

// aequitasChainID: die Chain-ID, gegen die signiert sein muss.
var aequitasChainID = big.NewInt(1926)

// betragAusWei rechnet genau so um wie sendRawTransaction -- dieselbe
// Rechnung, damit der Vergleich mit Transaction.Amount bitgenau ist.
func betragAusWei(wei *big.Int) float64 {
	decimals := new(big.Float).SetInt(weiPerAEQ)
	f, _ := new(big.Float).Quo(new(big.Float).SetInt(wei), decimals).Float64()
	return f
}

// ueberweisungAusRoh liest Empfaenger und Betrag aus einer signierten
// Transaktion -- in exakt der Reihenfolge und nach exakt den Regeln der beiden
// Ueberweisungszweige von sendRawTransaction (evm_rpc.go): zuerst die
// einfache AEQ-Ueberweisung, dann transfer(address,uint256) an den
// V7-Vertrag. Alles andere ist keine Ueberweisung.
func ueberweisungAusRoh(t *types.Transaction) (to string, betrag float64, ok bool) {
	if t.To() == nil {
		return "", 0, false
	}
	if len(t.Data()) == 0 && t.Value().Sign() > 0 {
		return strings.ToLower(t.To().Hex()), betragAusWei(t.Value()), true
	}
	d := t.Data()
	if len(d) >= 68 && strings.EqualFold(t.To().Hex(), V7_CONTRACT_ADDR) &&
		d[0] == 0xa9 && d[1] == 0x05 && d[2] == 0x9c && d[3] == 0xbb {
		to := strings.ToLower(("0x" + fmt.Sprintf("%x", d[16:36])))
		return to, betragAusWei(new(big.Int).SetBytes(d[36:68])), true
	}
	return "", 0, false
}

// pruefeUeberweisungsRoh prueft eine einzelne Ueberweisung gegen ihre
// signierte Rohform und gibt deren Nonce zurueck. Zustandslos und damit
// beliebig parallel ausfuehrbar.
func pruefeUeberweisungsRoh(tx *Transaction) (uint64, error) {
	if tx.Roh == "" {
		return 0, fmt.Errorf("%w: keine Rohform (TxHash %s)", ErrUeberweisungNichtSigniert, tx.TxHash)
	}
	t, absender, _, err := decodeAndRecoverSender(tx.Roh)
	if err != nil {
		return 0, fmt.Errorf("%w: Rohform unlesbar: %v", ErrUeberweisungNichtSigniert, err)
	}
	if cid := t.ChainId(); cid == nil || cid.Cmp(aequitasChainID) != 0 {
		return 0, fmt.Errorf("%w: Chain-ID %v statt 1926", ErrUeberweisungNichtSigniert, cid)
	}
	if absender != strings.ToLower(strings.TrimSpace(tx.Wallet)) {
		return 0, fmt.Errorf("%w: signiert von %s, im Block steht %s", ErrUeberweisungNichtSigniert, absender, tx.Wallet)
	}
	if !strings.EqualFold(t.Hash().Hex(), strings.TrimSpace(tx.TxHash)) {
		return 0, fmt.Errorf("%w: TxHash %s passt nicht zur Rohform (%s)", ErrUeberweisungNichtSigniert, tx.TxHash, t.Hash().Hex())
	}
	to, betrag, ok := ueberweisungAusRoh(t)
	if !ok {
		return 0, fmt.Errorf("%w: Rohform ist keine Ueberweisung", ErrUeberweisungNichtSigniert)
	}
	if to != strings.ToLower(strings.TrimSpace(tx.To)) {
		return 0, fmt.Errorf("%w: signiert an %s, im Block an %s", ErrUeberweisungNichtSigniert, to, tx.To)
	}
	if betrag != tx.Amount {
		return 0, fmt.Errorf("%w: signiert %.18g AEQ, im Block %.18g", ErrUeberweisungNichtSigniert, betrag, tx.Amount)
	}
	if t.Nonce() >= math.MaxInt64 {
		return 0, fmt.Errorf("%w: Nonce %d zu gross", ErrUeberweisungNichtSigniert, t.Nonce())
	}
	return t.Nonce(), nil
}

// nonceAusVorlage: die Nonce der signierten Rohform einer gerade
// angenommenen Ueberweisung. Die Signatur hat sendRawTransaction bereits
// geprueft; hier wird nur gelesen.
func nonceAusVorlage(vorlage Transaction) (uint64, bool) {
	if vorlage.Roh == "" {
		return 0, false
	}
	// Nur lesen, nicht wiederherstellen: die Signatur hat sendRawTransaction
	// gerade geprueft (Stufe 1.2).
	t, err := decodeRohTransaktion(vorlage.Roh)
	if err != nil || t.Nonce() >= math.MaxInt64 {
		return 0, false
	}
	return t.Nonce(), true
}

// setzeNaechsteNonce hebt NaechsteNonce auf nonce+1, nie herunter. Der
// Blattwert wird vom Speichern des Kontos nachgezogen; wer nicht speichert,
// muss updateAccountLeafLocked selbst rufen.
func setzeNaechsteNonce(acc *AccountState, nonce uint64) bool {
	n := int64(nonce) + 1
	if acc == nil || n <= acc.NaechsteNonce {
		return false
	}
	acc.NaechsteNonce = n
	return true
}

// ueberweisungsNonce: Ergebnis der Vorpruefung fuer eine Ueberweisung im Block.
type ueberweisungsNonce struct {
	idx      int
	absender string
	nonce    uint64
}

// pruefeUeberweisungenImBlock prueft alle Ueberweisungen eines Blocks
// parallel gegen ihre Rohform (Stufe 1.2: jede Signatur einmal, auf allen
// Kernen). Die Rueckgabe steht in Blockreihenfolge. Bei mehreren Fehlern wird
// der mit dem kleinsten Index gemeldet, damit jeder Knoten dieselbe Meldung
// schreibt.
func pruefeUeberweisungenImBlock(txs []Transaction) ([]ueberweisungsNonce, error) {
	var idx []int
	for i := range txs {
		if txs[i].Type == "transfer" {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return nil, nil
	}
	out := make([]ueberweisungsNonce, len(idx))
	fehler := make([]error, len(idx))
	workers := runtime.NumCPU()
	if workers > len(idx) {
		workers = len(idx)
	}
	var wg sync.WaitGroup
	var naechster atomic.Int64
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				k := int(naechster.Add(1) - 1)
				if k >= len(idx) {
					return
				}
				tx := &txs[idx[k]]
				n, err := pruefeUeberweisungsRoh(tx)
				out[k] = ueberweisungsNonce{idx: idx[k], absender: strings.ToLower(strings.TrimSpace(tx.Wallet)), nonce: n}
				fehler[k] = err
			}
		}()
	}
	wg.Wait()
	for k, err := range fehler {
		if err != nil {
			return nil, fmt.Errorf("Transaktion %d: %w", idx[k], err)
		}
	}
	return out, nil
}

// naechsteNoncenFuerBlockLocked prueft die Nonces eines Blocks in
// Blockreihenfolge gegen den gemeinsamen Zustand und gibt zurueck, auf
// welchen Wert NaechsteNonce je Absender danach steht. cs.mu gehalten.
func (cs *ChainState) naechsteNoncenFuerBlockLocked(ctx context.Context, liste []ueberweisungsNonce) (map[string]int64, error) {
	stand := make(map[string]int64, len(liste))
	for _, u := range liste {
		cur, ok := stand[u.absender]
		if !ok {
			cs.ensureAccountLoadedCtx(ctx, u.absender)
			if acc, da := cs.accounts.Get(u.absender); da {
				cur = acc.NaechsteNonce
			}
		}
		if int64(u.nonce) < cur {
			return nil, fmt.Errorf("Transaktion %d: %w: Nonce %d von %s ist verbraucht (naechste erlaubte %d)",
				u.idx, ErrUeberweisungNichtSigniert, u.nonce, u.absender, cur)
		}
		stand[u.absender] = int64(u.nonce) + 1
	}
	return stand, nil
}

// setzeNaechsteNoncenLocked schreibt das Ergebnis von
// naechsteNoncenFuerBlockLocked in die Konten -- in fester Reihenfolge, damit
// jeder Knoten dieselben Schritte tut. Konten, die es nicht gibt, bleiben
// unberuehrt: deren Ueberweisung scheitert ohnehin am Absender. sammler darf
// nil sein, dann wird sofort gespeichert. cs.mu gehalten.
func (cs *ChainState) setzeNaechsteNoncenLocked(ctx context.Context, neu map[string]int64, sammler *kontenSammler) error {
	adressen := make([]string, 0, len(neu))
	for a := range neu {
		adressen = append(adressen, a)
	}
	sort.Strings(adressen)
	for _, a := range adressen {
		acc, ok := cs.accounts.Get(a)
		if !ok || neu[a] <= acc.NaechsteNonce {
			continue
		}
		acc.NaechsteNonce = neu[a]
		cs.updateAccountLeafLocked(acc)
		if sammler != nil {
			sammler.hinzufuegen(acc)
			continue
		}
		if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
			return fmt.Errorf("NaechsteNonce fuer %s speichern: %w", a, err)
		}
	}
	return nil
}

// pruefeAnnahmeNonce: vor dem Annehmen einer Ueberweisung -- eine Nonce
// unter NaechsteNonce ist verbraucht und wuerde den naechsten Block fuer
// jeden anderen Validator ungueltig machen. Nur lesend.
func (cs *ChainState) pruefeAnnahmeNonce(absender string, nonce uint64) error {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	acc, ok := cs.accounts.Get(strings.ToLower(absender))
	if !ok {
		return nil // kaltes Konto: die Annahme laedt es; NaechsteNonce ist dort, wo es fehlt, 0
	}
	if int64(nonce) < acc.NaechsteNonce {
		return fmt.Errorf("nonce too low: %d (next allowed %d)", nonce, acc.NaechsteNonce)
	}
	return nil
}

// merkeAngenommeneNonceLocked: fuer Annahmepfade, die das Absenderkonto schon
// gespeichert haben (direkter Pfad, V7) -- NaechsteNonce setzen und das Konto
// in derselben Transaktion (ctx) noch einmal speichern. cs.mu gehalten.
func (cs *ChainState) merkeAngenommeneNonceLocked(ctx context.Context, absender string, vorlage Transaction) error {
	n, ok := nonceAusVorlage(vorlage)
	if !ok {
		return nil
	}
	acc, da := cs.accounts.Get(strings.ToLower(absender))
	if !da || !setzeNaechsteNonce(acc, n) {
		return nil
	}
	cs.updateAccountLeafLocked(acc)
	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return fmt.Errorf("NaechsteNonce fuer %s speichern: %w", absender, err)
	}
	return nil
}

// ungueltigeSignaturBloecke: wie viele Bloecke dieser Knoten wegen einer nicht
// signierten Ueberweisung oder verbrauchten Nonce abgelehnt hat. Jeder Wert
// ueber 0 heisst: ein Erzeuger hat versucht, etwas in die Kette zu schreiben,
// das der Kontoinhaber nicht unterschrieben hat -- oder es gibt einen Fehler.
var ungueltigeSignaturBloecke atomic.Int64

func merkeUngueltigeSignaturBlock() { ungueltigeSignaturBloecke.Add(1) }

// SignierteUeberweisungenStand fuer /health.
func SignierteUeberweisungenStand() map[string]interface{} {
	ab := signierteUeberweisungenAb()
	aktivAb := interface{}("aus")
	if ab != math.MaxInt64 {
		aktivAb = ab
	}
	return map[string]interface{}{
		"aktiv_ab":           aktivAb,
		"abgelehnte_bloecke": ungueltigeSignaturBloecke.Load(),
		"vorlauf_sekunden":   signierteUeberweisungenVorlauf,
	}
}
