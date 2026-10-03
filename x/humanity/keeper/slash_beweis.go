package keeper

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Beweis fuer eine Doppelsignatur (Analyse 03.10.2026, K-2).
//
// # WARUM
//
// slash_equivocation trug nur zwei Blockhashes. replayTransactions rief damit
// RecordEquivocationAndSuspend auf -- ohne zu pruefen, ob es die Bloecke gibt
// und ob der Beschuldigte sie wirklich beide unterschrieben hat. Seit dem
// 05.07.2026 (equivocationSlashingActivationUnix) gilt die Strafe: ein
// Produzent konnte mit zwei erfundenen Hashes jeden Validator 14 Tage
// sperren und ab dem zweiten "Vergehen" 50 AEQ von ihm nehmen.
//
// Pruefen gegen die eigenen Bloecke geht nicht: ein Nachspielender hat den
// verdraengten Block oft nie gesehen -- genau deshalb wird die Strafe als
// Transaktion verteilt. Also traegt die Transaktion den Beweis selbst: die
// beiden KOEPFE. Der Blockhash haengt nur am Kopf (calculateBlockHash: der
// Inhalt geht nur als TxRoot ein), und die Unterschrift am Hash. Jeder Knoten
// kann damit ohne die Bloecke pruefen:
//
//   - beide Koepfe ergeben ihren Hash, beide Unterschriften stammen vom
//     Beschuldigten (Proposer = Unterzeichner = tx.Wallet),
//   - verschiedene Hashes, aber dieselbe Elternmenge -- das ist die
//     Doppelsignatur, die checkAndIndexEquivocation erkennt,
//   - die Hashes sind die der Transaktion.

// BlockKopf: alles, was calculateBlockHash und die Unterschrift brauchen.
type BlockKopf struct {
	Height       int64    `json:"height"`
	Timestamp    int64    `json:"timestamp"`
	ParentHashes []string `json:"parent_hashes"`
	Proposer     string   `json:"proposer"`
	Humans       int      `json:"humans"`
	StateRoot    string   `json:"state_root,omitempty"`
	TxRoot       string   `json:"tx_root"`
	Hash         string   `json:"hash"`
	Signature    string   `json:"signature"`
}

// Doppelbeweis: die beiden widerspruechlichen Koepfe.
type Doppelbeweis struct {
	A BlockKopf `json:"a"`
	B BlockKopf `json:"b"`
}

// kopfVon: der Kopf eines Blocks, TxRoot so, wie calculateBlockHash ihn
// rechnet.
func kopfVon(b *Block) BlockKopf {
	txRoot := b.TxRoot
	if len(b.Transactions) > 0 || txRoot == "" {
		h := sha256.Sum256(b.transaktionenJSON())
		txRoot = hex.EncodeToString(h[:])
	}
	return BlockKopf{
		Height: b.Height, Timestamp: b.Timestamp, ParentHashes: append([]string(nil), b.ParentHashes...),
		Proposer: b.Proposer, Humans: b.Humans, StateRoot: b.StateRoot, TxRoot: txRoot,
		Hash: b.Hash, Signature: b.Signature,
	}
}

// unterzeichner: prueft Hash und Unterschrift des Kopfs; zurueck kommt die
// Adresse, die unterschrieben hat (kleingeschrieben).
func (k BlockKopf) unterzeichner() (string, error) {
	if k.TxRoot == "" {
		return "", fmt.Errorf("Kopf ohne TxRoot")
	}
	b := &Block{Height: k.Height, Timestamp: k.Timestamp, ParentHashes: k.ParentHashes,
		Proposer: k.Proposer, Humans: k.Humans, StateRoot: k.StateRoot, TxRoot: k.TxRoot}
	if got := calculateBlockHash(b); got != k.Hash {
		return "", fmt.Errorf("Kopf ergibt %s, behauptet %s", kurzHash(got), kurzHash(k.Hash))
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(k.Signature, "0x"))
	if err != nil || len(sig) != 65 {
		return "", fmt.Errorf("Unterschrift unlesbar")
	}
	h := common.HexToHash(k.Hash)
	pub, err := crypto.Ecrecover(h[:], sig)
	if err != nil {
		return "", fmt.Errorf("Unterschrift: %v", err)
	}
	pk, err := crypto.UnmarshalPubkey(pub)
	if err != nil {
		return "", fmt.Errorf("Schluessel: %v", err)
	}
	return strings.ToLower(crypto.PubkeyToAddress(*pk).Hex()), nil
}

func kurzHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// pruefeDoppelbeweis: belegt der Beweis eine Doppelsignatur von signierer
// mit genau diesen beiden Hashes? Gibt den Grund zurueck, wenn nicht.
func pruefeDoppelbeweis(beweis *Doppelbeweis, signierer, hashA, hashB string) error {
	if beweis == nil {
		return fmt.Errorf("kein Beweis")
	}
	signierer = strings.ToLower(strings.TrimSpace(signierer))
	for _, k := range []BlockKopf{beweis.A, beweis.B} {
		von, err := k.unterzeichner()
		if err != nil {
			return err
		}
		if von != signierer || strings.ToLower(k.Proposer) != signierer {
			return fmt.Errorf("Block %s unterschrieb %s, nicht %s", kurzHash(k.Hash), kurzAdresse(von), kurzAdresse(signierer))
		}
	}
	if beweis.A.Hash == beweis.B.Hash {
		return fmt.Errorf("zweimal derselbe Block")
	}
	if equivocationParentKey(beweis.A.ParentHashes) != equivocationParentKey(beweis.B.ParentHashes) {
		return fmt.Errorf("verschiedene Eltern -- keine Doppelsignatur")
	}
	a, b := beweis.A.Hash, beweis.B.Hash
	if a > b {
		a, b = b, a
	}
	if hashA > hashB {
		hashA, hashB = hashB, hashA
	}
	if a != hashA || b != hashB {
		return fmt.Errorf("Beweis gehoert zu anderen Bloecken")
	}
	return nil
}

// nachrechnenSlashLocked: slash_equivocation nur mit gueltigem Beweis, und
// der Zeitpunkt (DetectedAt bestimmt Sperrfrist und Wiederholungsfenster)
// muss der eines der beiden Bloecke sein.
func nachrechnenSlashLocked(tx *Transaction, blockZeit int64) error {
	if tx.DetectedAt < equivocationSlashingActivationUnix {
		return nil // wird nicht bestraft (RecordEquivocationAndSuspend)
	}
	if err := pruefeDoppelbeweis(tx.Doppelbeweis, tx.Wallet, tx.BlockAHash, tx.BlockBHash); err != nil {
		return nachrechnenAbweichung("slash_ohne_beweis", blockZeit, "%s: %v", kurzAdresse(tx.Wallet), err)
	}
	// Der Erkennende setzt DetectedAt auf die Blockzeit des zweiten Blocks
	// (AddPeerBlock: detectedAt := block.Timestamp) -- also genau eine der
	// beiden. Alles andere hat der Produzent gewaehlt.
	if tx.DetectedAt != tx.Doppelbeweis.A.Timestamp && tx.DetectedAt != tx.Doppelbeweis.B.Timestamp {
		return nachrechnenAbweichung("slash_zeit", blockZeit,
			"%s: erkannt %d, die Bloecke tragen %d und %d", kurzAdresse(tx.Wallet), tx.DetectedAt,
			tx.Doppelbeweis.A.Timestamp, tx.Doppelbeweis.B.Timestamp)
	}
	return nil
}
