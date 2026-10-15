package keeper

// Grenzen fuer /api/blocks/push (Beta-Pruefung 15.10.2026, HOCH).
//
// Der Weg ist ohne Anmeldung offen. Jede Anfrage liest bis zu
// maxBlockStreamBytes (32 MB) und entpackt sie; ohne Obergrenze genuegten
// parallele, ungueltige Ruempfe, um den Speicher zu fuellen. Dazu loeste ein
// gestrippter Kopf mit erfundener tx_root VOR jeder Signaturpruefung einen
// ausgehenden Rumpf-Abruf bei jedem bekannten Peer aus (Frist 25 s).
//
//   - blockPushPlaetze: hoechstens blockPushGleichzeitig Anfragen werden
//     gleichzeitig gelesen; darueber 503 (der Absender schickt den Block beim
//     naechsten Mal, und der Sync holt ihn ohnehin).
//   - kopfVorAbruf: ein gestrippter Kopf muss zu seinem Hash passen und vom
//     Proposer signiert sein, BEVOR ein Rumpf von aussen geholt wird.
//   - Jede Ablehnung zaehlt im Flutschutz je IP (blockPushRecordOutcome),
//     nicht nur die nach AddPeerBlock.

import (
	"encoding/hex"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	blockPushGleichzeitig     = 8
	blockPushGleichzeitigJeIP = 2
)

var (
	blockPushPlaetze = make(chan struct{}, blockPushGleichzeitig)
	// Je IP: Eintraege nur, solange Anfragen laufen -- hoechstens
	// blockPushGleichzeitig Schluessel, weil jeder einen globalen Platz haelt.
	blockPushJeIPMu sync.Mutex
	blockPushJeIP   = map[string]int{}
)

// blockPushPlatz: einen Platz belegen (global und je IP). false = voll.
func blockPushPlatz(ip string) (frei func(), ok bool) {
	select {
	case blockPushPlaetze <- struct{}{}:
	default:
		return nil, false
	}
	blockPushJeIPMu.Lock()
	if blockPushJeIP[ip] >= blockPushGleichzeitigJeIP {
		blockPushJeIPMu.Unlock()
		<-blockPushPlaetze
		return nil, false
	}
	blockPushJeIP[ip]++
	blockPushJeIPMu.Unlock()
	return func() {
		blockPushJeIPMu.Lock()
		if blockPushJeIP[ip]--; blockPushJeIP[ip] <= 0 {
			delete(blockPushJeIP, ip)
		}
		blockPushJeIPMu.Unlock()
		<-blockPushPlaetze
	}, true
}

type kopfUrteil int

const (
	kopfGueltig    kopfUrteil = iota
	kopfGefaelscht            // Hash oder Signatur passt nicht
	kopfUnbekannt             // echt signiert, Erzeuger hier (noch) nicht erlaubt
)

// kopfVorAbruf: darf fuer diesen gestrippten Kopf ein Rumpf geholt werden?
func (dag *BlockDAG) kopfVorAbruf(b *Block) kopfUrteil {
	if b.Hash == "" || b.Signature == "" || calculateBlockHash(b) != b.Hash {
		return kopfGefaelscht
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(b.Signature, "0x"))
	if err != nil || len(sig) != 65 {
		return kopfGefaelscht
	}
	h := common.HexToHash(b.Hash)
	roh, err := crypto.Ecrecover(h[:], sig)
	if err != nil {
		return kopfGefaelscht
	}
	pub, err := crypto.UnmarshalPubkey(roh)
	if err != nil || strings.ToLower(crypto.PubkeyToAddress(*pub).Hex()) != strings.ToLower(b.Proposer) {
		return kopfGefaelscht
	}
	if !dag.erzeugerErlaubtLesend(strings.ToLower(b.Proposer), b.Timestamp) {
		return kopfUnbekannt
	}
	return kopfGueltig
}

// erzeugerErlaubtLesend: erzeugerErlaubt unter der Lesesperre (mit defer,
// sperre_mit_defer_test.go).
func (dag *BlockDAG) erzeugerErlaubtLesend(proposer string, zeit int64) bool {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	return dag.erzeugerErlaubt(proposer, zeit)
}
