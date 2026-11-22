package keeper

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
)

// Los der Praeparatoren fuer Quantengeld Weg B (forschung/quantengeld/
// s_weg_b.py auslosen, DEZENTRAL.md 18). Aus dem Abbild des Registers zur
// Bestellung nimmt das Los die v Konten mit dem kleinsten
// H(DOM_LOS, Zufall, K, Konto); bei gleichem Hash entscheidet das Konto.
// H haengt vor jeden Teil seine Laenge (4 Byte big-endian), wie h() in der
// Spezifikation. Der Zufall kommt aus einer drand-Runde nach der Bestellung
// (drand_zufall.go). Noch nicht verdrahtet (B2).
const (
	quantenSLosVMin       = 4
	quantenSLosVMax       = 16
	quantenSLosPoolFaktor = 3
)

var quantenSDomLos = []byte("aequitas-quanten-s-weg-b-los-v1")

func quantenSLosHash(teile ...[]byte) []byte {
	h := sha256.New()
	var l [4]byte
	for _, t := range teile {
		binary.BigEndian.PutUint32(l[:], uint32(len(t)))
		h.Write(l[:])
		h.Write(t)
	}
	return h.Sum(nil)
}

// quantenSLos: die v Gelosten in Losreihenfolge (Praeparator 1 = erster).
// Fail-closed: v ausserhalb der Grenzen, Abbild zu klein, ein Konto doppelt
// oder nicht kanonisch.
func quantenSLos(abbild []string, zufall [32]byte, K string, v int) ([]string, error) {
	if v < quantenSLosVMin || v > quantenSLosVMax {
		return nil, fmt.Errorf("Los: v = %d ausserhalb %d..%d", v, quantenSLosVMin, quantenSLosVMax)
	}
	if len(abbild) < quantenSLosPoolFaktor*v {
		return nil, fmt.Errorf("Los: Register zu klein (%d fuer v = %d)", len(abbild), v)
	}
	type rang struct {
		h     []byte
		konto string
	}
	gesehen := make(map[string]bool, len(abbild))
	r := make([]rang, 0, len(abbild))
	for _, k := range abbild {
		if !kanonischeAdresse(k) || gesehen[k] {
			return nil, fmt.Errorf("Los: Konto doppelt oder nicht kanonisch")
		}
		gesehen[k] = true
		r = append(r, rang{quantenSLosHash(quantenSDomLos, zufall[:], []byte(K), []byte(k)), k})
	}
	sort.Slice(r, func(i, j int) bool {
		if c := bytes.Compare(r[i].h, r[j].h); c != 0 {
			return c < 0
		}
		return r[i].konto < r[j].konto
	})
	out := make([]string, v)
	for i := range out {
		out[i] = r[i].konto
	}
	return out, nil
}
