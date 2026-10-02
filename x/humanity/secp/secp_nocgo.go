//go:build !cgo

package secp

import "github.com/ethereum/go-ethereum/crypto"

// Ohne cgo (etwa ein Windows-Entwicklungsrechner): dieselbe Rechnung ueber
// go-ethereum. Das Image baut immer mit cgo (CI-Job docker-image).
func Wiederherstellen(nachricht, sig []byte) ([]byte, error) {
	if err := pruefeEingabe(nachricht, sig); err != nil {
		return nil, err
	}
	return crypto.Ecrecover(nachricht, sig)
}

const Schnell = false
