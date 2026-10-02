// Package secp stellt den oeffentlichen Schluessel aus einer secp256k1-
// Signatur wieder her -- mit libsecp256k1 v0.6.0 statt der Fassung von 2017,
// die go-ethereum v1.13 mitbringt.
//
// # WARUM
//
// Jede Ueberweisung kostet eine Wiederherstellung, bei der Annahme und bei
// jedem Validator beim Nachspielen (Stufe 1.0). Gemessen am 02.10.2026 auf
// demselben Rechner (BenchmarkEcrecover, 4 Kerne):
//
//	go-ethereum v1.13.0 (libsecp256k1 von 2017)  64 us einzeln, 20 us parallel
//	libsecp256k1 v0.6.0 (go-ethereum v1.17.7)    42 us einzeln, 14 us parallel
//
// Ein Wechsel auf go-ethereum v1.17 haette die EVM mitgezogen, also
// Konsensregeln. Deshalb nur die Kurvenrechnung, als eigene Kopie: dieselben
// Quellen, die go-ethereum v1.17.7 unter crypto/secp256k1/libsecp256k1
// ausliefert (MIT, siehe libsecp256k1/COPYING), unveraendert bis auf das
// Weglassen von Tests, Benchmarks und nicht benutzten Modulen.
//
// Alle globalen Symbole bekommen den Praefix aeq_ (umbenennen.h), sonst
// gaebe es sie neben der Kopie von go-ethereum zweimal.
//
// Was eine gueltige Signatur ist, entscheidet weiter der Aufrufer
// (keeper/absender_schnell.go, mit denselben Pruefungen wie go-ethereum);
// diese Datei rechnet nur.
package secp

import "errors"

var (
	ErrNachrichtLaenge  = errors.New("invalid message length, need 32 bytes")
	ErrSignaturLaenge   = errors.New("invalid signature length")
	ErrWiederherstellID = errors.New("invalid signature recovery id")
	ErrNichtHergestellt = errors.New("recovery failed")
)

// pruefeEingabe: dieselben Pruefungen wie go-ethereum
// crypto/secp256k1.RecoverPubkey, in derselben Reihenfolge.
func pruefeEingabe(nachricht, sig []byte) error {
	if len(nachricht) != 32 {
		return ErrNachrichtLaenge
	}
	if len(sig) != 65 {
		return ErrSignaturLaenge
	}
	if sig[64] >= 4 {
		return ErrWiederherstellID
	}
	return nil
}
