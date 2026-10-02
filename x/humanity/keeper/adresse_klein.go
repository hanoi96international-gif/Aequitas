package keeper

import (
	"encoding/hex"

	"github.com/ethereum/go-ethereum/common"
)

// adresseKlein: die Adresse als "0x" + Kleinbuchstaben-Hex -- dasselbe
// Ergebnis wie strings.ToLower(a.Hex()), ohne den Umweg ueber die
// EIP-55-Pruefsumme. a.Hex() rechnet je Aufruf einen Keccak-Hash fuer die
// Gross-/Kleinschreibung, die ToLower danach wieder wegwirft.
//
// Pruefstand Lauf 17 (02.10.2026): strings.ToLower 2,5 %, checksumHex und
// keccakF1600 zusammen gut 2 % der Knoten-CPU -- auf dem Annahmepfad, der
// jetzt die Grenze ist (Rueckstau 0, Inflight voll).
func adresseKlein(a common.Address) string {
	var buf [2 + 2*common.AddressLength]byte
	buf[0], buf[1] = '0', 'x'
	hex.Encode(buf[2:], a[:])
	return string(buf[:])
}
