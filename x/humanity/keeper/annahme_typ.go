package keeper

import (
	"errors"

	"github.com/ethereum/go-ethereum/core/types"
)

// Welche Transaktionstypen ein Knoten bei der ANNAHME (eth_sendRawTransaction)
// zulaesst: Legacy, EIP-2930 und EIP-1559. Blob- (Typ 3) und SetCode-
// Transaktionen (Typ 4) haben auf dieser Kette keine Bedeutung.
//
// Sicherheitspruefung #359 (HOCH): Typ 3 wurde angenommen, auch mit
// Blob-Sidecar. Die Rohform landet als Roh im Block -- mit Sidecar bis etwa
// 1 MB Blob-Daten in jedem Block. Abgelehnt wird vor jeder Nonce-Reservierung
// (auch im Buendel, nonce_batch_reserve.go), damit eine abgewiesene
// Transaktion nichts verbraucht.
//
// Beim NACHSPIELEN gilt diese Regel nicht: dort entscheidet, was v1.13
// gelesen hat (decodeRohTransaktion, rohSidecarAlsV113), sonst spielten alte
// Bloecke anders nach.
func annahmeTypErlaubt(t *types.Transaction) bool {
	switch t.Type() {
	case types.LegacyTxType, types.AccessListTxType, types.DynamicFeeTxType:
		return true
	}
	return false
}

// errSidecarV1: go-ethereum v1.17 liest Blob-Transaktionen auch im
// Netzformat mit Versionsbyte ([tx, 0x01, blobs, commitments, cellProofs]),
// v1.13 brach dabei mit einem RLP-Fehler ab. Ohne diese Ablehnung naehme ein
// aktualisierter Knoten eine Ueberweisung an und bestaetigte sie im Block,
// die jeder noch nicht aktualisierte Validator verwirft -- eine Spaltung,
// die jeder Nutzer ausloesen kann (Sicherheitspruefung #359, HOCH).
var errSidecarV1 = errors.New("blob sidecar version 1 not supported")

// rohSidecarAlsV113: lehnt ab, was v1.13 nicht lesen konnte. Sidecar v0
// las v1.13 und bleibt deshalb lesbar (Nachspielen alter Bloecke).
func rohSidecarAlsV113(t *types.Transaction) error {
	if sc := t.BlobTxSidecar(); sc != nil && sc.Version != types.BlobSidecarVersion0 {
		return errSidecarV1
	}
	return nil
}
