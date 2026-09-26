package bereiche

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
)

// LOSUNG DER AUSSCHUESSE.
//
// Je Epoche werden die Validatoren mit einer Zufallszahl aus dem Sammelblock
// der Vorepoche gemischt (Fisher-Yates, deterministisch) und in S Ausschuesse
// zu m Mitgliedern geteilt; niemand sitzt in zwei. Jeder kann die Losung
// nachrechnen. Die Zufallszahl kommt aus dem Hash des Sammelblocks -- spaeter
// aus einer VRF, weil der letzte Ausschuss einer Epoche den Hash in Grenzen
// beeinflussen kann (siehe docs).
//
// Warum das bei Aequitas traegt: ausgeloste Ausschuesse sind nur sicher, wenn
// niemand viele Identitaeten hat. Hier ist jeder Validator ein Mensch.

// MindestValidatoren: darunter bleibt Stufe 3 aus (docs: "nicht unter 32").
const MindestValidatoren = 32

// Aktiv: darf Stufe 3 mit s Bereichen und Ausschussgroesse m laufen?
func Aktiv(validatoren, s, m int) bool {
	return validatoren >= MindestValidatoren && s >= 1 && m >= 1 && validatoren >= s*m
}

type zufallsquelle struct {
	seed Hash
	n    uint64
}

func (z *zufallsquelle) naechste(bis int) int {
	var b [40]byte
	copy(b[:], z.seed[:])
	binary.BigEndian.PutUint64(b[32:], z.n)
	z.n++
	h := sha256.Sum256(b[:])
	return int(binary.BigEndian.Uint64(h[:8]) % uint64(bis))
}

func mische(validatoren []string, seed Hash) []string {
	v := append([]string(nil), validatoren...)
	sort.Strings(v)
	z := &zufallsquelle{seed: seed}
	for i := len(v) - 1; i > 0; i-- {
		j := z.naechste(i + 1)
		v[i], v[j] = v[j], v[i]
	}
	return v
}

// Ausschuesse fuer eine Epoche.
func Ausschuesse(validatoren []string, s, m int, zufall Hash) ([][]string, error) {
	if len(validatoren) < s*m {
		return nil, fmt.Errorf("%d Validatoren reichen nicht fuer %d x %d", len(validatoren), s, m)
	}
	v := mische(validatoren, sha256.Sum256(append([]byte("ausschuss|"), zufall[:]...)))
	out := make([][]string, s)
	for i := 0; i < s; i++ {
		out[i] = append([]string(nil), v[i*m:(i+1)*m]...)
		sort.Strings(out[i])
	}
	return out, nil
}

// Pruefausschuss: unabhaengig gelost, ohne die Mitglieder des eigenen
// Ausschusses des Bereichs.
func Pruefausschuss(validatoren []string, eigener []string, m int, zufall Hash, bereich int) []string {
	aus := map[string]bool{}
	for _, a := range eigener {
		aus[a] = true
	}
	var rest []string
	for _, a := range validatoren {
		if !aus[a] {
			rest = append(rest, a)
		}
	}
	var b []byte
	b = append(b, "pruefausschuss|"...)
	b = append(b, zufall[:]...)
	b = binary.BigEndian.AppendUint64(b, uint64(bereich))
	v := mische(rest, sha256.Sum256(b))
	if len(v) > m {
		v = v[:m]
	}
	sort.Strings(v)
	return v
}

func pubAus(addr string) (ed25519.PublicKey, error) {
	b, err := hex.DecodeString(addr)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("keine Adresse")
	}
	return ed25519.PublicKey(b), nil
}
