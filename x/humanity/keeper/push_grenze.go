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
//   - pushMitLeseFrist (Beta-Pruefung, MITTEL): wer einen Platz belegt, muss
//     auch senden. Bis dahin hielt nur die ReadTimeout des Servers (30 s)
//     einen Platz; vier Adressen mit je zwei tropfenden Anfragen belegten
//     alle acht und wiederholten das beliebig -- jeder echte Push bekam 503.
//     Jetzt verlaengert jedes empfangene Byte die Lesefrist nur so weit, wie
//     es bei pushMindestRate dauern darf, nach einem Anlauf von
//     pushLeseAnlauf. Ein Tropfen verliert seinen Platz nach rund 5 s, und
//     der Abbruch zaehlt im Flutschutz seiner IP (je /64).

import (
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

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

// Mindestrate beim Lesen eines Pushes. var: Tests verkuerzen sie.
var (
	pushLeseAnlauf  = 5 * time.Second
	pushMindestRate = int64(32 << 10) // Byte je Sekunde
)

// pushLeseFrist: Lesefrist, die mit jedem empfangenen Byte waechst -- aber
// nur um die Zeit, die es bei pushMindestRate dauern darf.
type pushLeseFrist struct {
	io.ReadCloser
	rc    *http.ResponseController
	start time.Time
	n     int64
}

// pushMitLeseFrist setzt die Frist auf r.Body. Unterstuetzt der Writer keine
// Lesefrist (ErrNotSupported), bleibt die ReadTimeout des Servers.
func pushMitLeseFrist(w http.ResponseWriter, r *http.Request) {
	f := &pushLeseFrist{ReadCloser: r.Body, rc: http.NewResponseController(w), start: time.Now()}
	f.verlaengern()
	r.Body = f
}

func (f *pushLeseFrist) verlaengern() {
	erlaubt := pushLeseAnlauf + time.Duration(f.n*int64(time.Second)/pushMindestRate)
	_ = f.rc.SetReadDeadline(f.start.Add(erlaubt))
}

func (f *pushLeseFrist) Read(p []byte) (int, error) {
	n, err := f.ReadCloser.Read(p)
	if n > 0 {
		f.n += int64(n)
		f.verlaengern()
	}
	return n, err
}
