package keeper

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lib/pq"
)

// Nonce-Reservierung ohne Postgres-Umlauf je Ueberweisung.
//
// # WAS GEMESSEN WURDE
//
// 27.09.2026, Einzelueberweisungen (Buendel 1) gegen C1: sendRawTransaction
// 66 ms je Ueberweisung, davon 12,5 ms in reserveNoncePerItem -- fast
// alles das Warten auf ReserveNonce, einen Compare-and-swap in evm_nonces,
// den ein einziger Sammler je Buendel synchron gegen Postgres faehrt.
//
// # WARUM ER FUER SIGNIERTE UEBERWEISUNGEN ENTFALLEN KANN
//
// Der Compare-and-swap war der Schutz gegen Wiederholung: dieselbe signierte
// Transaktion ein zweites Mal eingereicht, und sie buchte ein zweites Mal.
// Seit Stufe 1 (signierte_ueberweisung.go) traegt jede Ueberweisung ihre
// Nonce in die Kette: NaechsteNonce steht im Kontozustand, wird bei der
// Buchung UNTER der Kontensperre geprueft (transfer_wal.go,
// pruefeNonceLocked), ist im WAL dauerhaft und nach einem Neustart
// wiederhergestellt -- und jeder andere Validator verwirft einen Block mit
// einer verbrauchten Nonce. Die Wiederholung scheitert also an der Kette,
// nicht erst an evm_nonces.
//
// Innerhalb des Prozesses bleibt die Reservierung, wie sie war: unter der
// Nonce-Shard-Sperre gegen den Wert im Speicher, genau eine Transaktion je
// Nonce. Nur das Nachziehen von evm_nonces laeuft im Hintergrund (alle
// nonceNachtragTakt, ein Befehl fuer alle Adressen, nur steigend).
//
// # WAS DAFUER SORGT, DASS NICHTS AUSEINANDERLAEUFT
//
//   - Ein synchroner Compare-and-swap (Vertragsaufrufe, der Buendelweg)
//     schreibt vorher den offenen Nachtrag seiner Adresse -- sonst traefe er
//     einen veralteten Wert und lehnte ab.
//   - Beim ersten Kontakt nach einem Neustart gilt das Maximum aus
//     evm_nonces und NaechsteNonce der Kette: ging bei einem Absturz ein
//     Nachtrag verloren, kennt die Kette die richtige Nonce.
//
// Nur fuer einfache AEQ-Ueberweisungen und nur, solange Stufe 1 Pflicht ist.
// Vertragsaufrufe tragen ihre Nonce nicht in die Kette und reservieren
// weiter synchron.
//
//	AEQUITAS_NONCE_SYNCHRON=1   alter Weg fuer alles (Notschalter)

const nonceNachtragTakt = 50 * time.Millisecond

var (
	nonceNachtragGeschrieben atomic.Int64
	nonceNachtragFehler      atomic.Int64
	nonceSchnellReserviert   atomic.Int64
)

type nonceNachtrag struct {
	mu      sync.Mutex
	offen   map[string]uint64
	starten sync.Once
}

// nonceOhneUmlauf: darf diese Transaktion ohne synchronen Postgres-Umlauf
// reserviert werden?
func nonceOhneUmlauf(einfacheUeberweisung bool, jetzt int64) bool {
	if !einfacheUeberweisung || nonceSynchronErzwungen() {
		return false
	}
	return signierteUeberweisungenPflicht(jetzt)
}

func nonceSynchronErzwungen() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AEQUITAS_NONCE_SYNCHRON"))) {
	case "1", "true", "ja", "yes", "on":
		return true
	}
	return false
}

// merkeNonceNachtrag: evm_nonces fuer addr bald auf mindestens next setzen.
func (cs *ChainState) merkeNonceNachtrag(addr string, next uint64) {
	if cs.db == nil {
		return
	}
	n := &cs.nonceNachtrag
	n.starten.Do(func() {
		n.mu.Lock()
		n.offen = make(map[string]uint64)
		n.mu.Unlock()
		SafeGoroutine("nonceNachtrag", func() {
			t := time.NewTicker(nonceNachtragTakt)
			defer t.Stop()
			for range t.C {
				cs.schreibeNonceNachtraege()
			}
		})
	})
	n.mu.Lock()
	if next > n.offen[addr] {
		n.offen[addr] = next
	}
	n.mu.Unlock()
}

// nimmNachtraege holt die offenen Nachtraege (alle, oder nur addr).
func (cs *ChainState) nimmNachtraege(nur string) map[string]uint64 {
	n := &cs.nonceNachtrag
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.offen) == 0 {
		return nil
	}
	if nur != "" {
		v, ok := n.offen[nur]
		if !ok {
			return nil
		}
		delete(n.offen, nur)
		return map[string]uint64{nur: v}
	}
	alle := n.offen
	n.offen = make(map[string]uint64, len(alle))
	return alle
}

// gibNachtraegeZurueck: nach einem Schreibfehler wieder vormerken (nur
// steigend -- inzwischen kann ein hoeherer Wert eingetragen sein).
func (cs *ChainState) gibNachtraegeZurueck(m map[string]uint64) {
	n := &cs.nonceNachtrag
	n.mu.Lock()
	for a, v := range m {
		if v > n.offen[a] {
			n.offen[a] = v
		}
	}
	n.mu.Unlock()
}

func (cs *ChainState) schreibeNonceNachtraegeMap(m map[string]uint64) error {
	if len(m) == 0 || cs.db == nil {
		return nil
	}
	adr := make([]string, 0, len(m))
	wert := make([]int64, 0, len(m))
	for a, v := range m {
		adr = append(adr, a)
		wert = append(wert, int64(v))
	}
	_, err := cs.db.Exec(`INSERT INTO evm_nonces (address, nonce)
		SELECT a, n FROM unnest($1::text[], $2::bigint[]) AS v(a, n)
		ON CONFLICT (address) DO UPDATE SET nonce = EXCLUDED.nonce WHERE evm_nonces.nonce < EXCLUDED.nonce`,
		pq.Array(adr), pq.Array(wert))
	if err != nil {
		nonceNachtragFehler.Add(1)
		return fmt.Errorf("Nonce-Nachtrag (%d Adressen): %w", len(m), err)
	}
	nonceNachtragGeschrieben.Add(int64(len(m)))
	return nil
}

func (cs *ChainState) schreibeNonceNachtraege() {
	m := cs.nimmNachtraege("")
	if err := cs.schreibeNonceNachtraegeMap(m); err != nil {
		cs.gibNachtraegeZurueck(m)
	}
}

// nonceNachtragJetzt: vor einem synchronen Compare-and-swap den offenen
// Nachtrag dieser Adresse schreiben.
func (cs *ChainState) nonceNachtragJetzt(addr string) error {
	m := cs.nimmNachtraege(addr)
	if err := cs.schreibeNonceNachtraegeMap(m); err != nil {
		cs.gibNachtraegeZurueck(m)
		return err
	}
	return nil
}

// gespeicherteNonce: die naechste Nonce eines Absenders beim ersten Kontakt
// nach dem Start -- das Maximum aus evm_nonces und NaechsteNonce der Kette.
// Liegt die Kette vorn (ein Nachtrag ging bei einem Absturz verloren), wird
// evm_nonces nachgezogen, bevor ein synchroner Compare-and-swap es liest.
func (cs *ChainState) gespeicherteNonce(addr string) uint64 {
	n := cs.LoadNonce(addr)
	if kette := cs.naechsteNonceVon(addr); kette > n {
		n = kette
		cs.merkeNonceNachtrag(strings.ToLower(addr), kette)
	}
	return n
}

// NonceNachtragStand fuer /api/health/combined.
func NonceNachtragStand() map[string]interface{} {
	return map[string]interface{}{
		"schnell_reserviert": nonceSchnellReserviert.Load(),
		"geschrieben":        nonceNachtragGeschrieben.Load(),
		"fehler":             nonceNachtragFehler.Load(),
	}
}
