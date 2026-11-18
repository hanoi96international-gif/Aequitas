package keeper

// QUANTENGELD VARIANTE W, STUFE W2a: DIE REGELN (eingehaengt in
// quanten_w_kette.go, seit 17.11.2026 17:00 UTC im Hauptnetz freigeschaltet)
//
// Privat pruefbare Noten (Wiesner-Familie) mit klassischer Pruefung; das
// Geheimnis jeder Stelle liegt als Shamir-Anteile bei den Validatoren der
// Note. Spezifikation und Begruendung: forschung/quantengeld/wiesner_kette.py
// (dieselben Regeln, ausfuehrbar, mit Angriffen) und ANALYSE.md Abschnitt 5.
// Der Pruefvektor testdata/quanten_w_vektor.json kommt aus der Spezifikation;
// quanten_w_test.go haelt beide Byte fuer Byte gleich.
//
// OFFENLEGEN STATT GEMEINSAM RECHNEN: beim Praegen steht je Stelle nur eine
// gesalzene Pruefsumme auf der Kette (Merkle-Wurzel). Eine Pruefung: Anfrage
// (Pfand) -> jeder Validator der Note legt den naechsten Wert seiner
// Hash-Kette offen (Anker beim Praegen bestaetigt); sind alle da, folgen
// Stellen und Basen, ab da sind die Stellen verbraucht -> Antwort des
// Anfragenden -> Offenlegen von t Anteilen je Stelle. Jeder Knoten setzt
// zusammen, prueft Pruefsumme und Pfad und urteilt selbst; ein falscher
// Anteil faellt an der Pruefsumme auf, ein falscher Zufall an der
// Hash-Kette. Kein Knoten glaubt einem Validator, und kein Erzeuger kann
// die Aufforderung steuern (der Blockhash der ersten Fassung konnte er).
//
// Hier stehen nur reine Funktionen und das Register mit seinen Uebergaengen.
// Unterschriften (Anfrage, Antwort, Bestaetigung der Validatoren) prueft die
// Einbindung (W2b) vor diesen Regeln; Geldbewegungen meldet das Register als
// Ergebnis, ausgefuehrt werden sie in der Einbindung. Nichts hiervon ist in
// einem Block erreichbar.

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

const (
	quantenWNMax       = 16384
	quantenWKMin       = 8
	quantenWKMax       = 8192 // Pruefen hoechstens; Einloesen genau k_note
	quantenWMinTreffer = 4
	quantenWFehlMax    = 3
	quantenWTMax       = 16
	// quantenWZufallL: Laenge der Hash-Kette je Validator = hoechstens so
	// viele Anfragen je Note; der letzte Wert bleibt dem Einloesen.
	quantenWZufallL  = 256
	quantenWVMax     = 64
	quantenWSalzBits = 124
	// Offenlegen in Teilen zu hoechstens so vielen Stellen, der Reihe nach.
	quantenWOffenTeil = 64
	// Merkle-Pfad hoechstens so lang (Tiefe bei quantenWNMax Blaettern).
	quantenWPfadMax = 14
	// Standard-Toleranz einer Note in Prozent (die Note waehlt ihre eigene).
	quantenWTolStandard = 4
)

// SICHERE PRUEFGROESSE (W2c, ANALYSE.md „Befund Klonangriff“): ein optimaler
// Kloner macht zwei Kopien mit je 14,6 % Fehler je Treffer; die alte feste
// Toleranz von 15 % lag darueber. Jetzt hat jede Note ihre Toleranz tol
// (Prozent, Schluessel der Tabelle; die Validatoren bestaetigen sie mit)
// und daraus ihre Pruefgroesse k_note = quantenWKTabelle[tol][f16],
// f16 = ceil(16 (t-1) / v) -- eine Absprache unter der Schwelle kennt
// hoechstens den Anteil (t-1)/v der Stellen. Die Tabelle rechnet die
// Spezifikation exakt (wiesner_parameter.py): ein Faelscher besteht
// hoechstens mit 2^-40. Ganze Zahlen, kein Gleitkomma im Konsens; 0 = nicht
// sicher moeglich (Praegen abgewiesen). Pruefvektor: k_tabelle.
var quantenWKTabelle = map[int][9]int{
	2:  {552, 600, 672, 752, 856, 984, 1168, 1408, 1760},
	3:  {672, 752, 856, 984, 1144, 1368, 1680, 2144, 2880},
	4:  {840, 960, 1112, 1304, 1576, 1960, 2536, 3472, 5200},
	5:  {1064, 1240, 1464, 1784, 2240, 2936, 4096, 6264, 0},
	6:  {1360, 1632, 2000, 2536, 3360, 4752, 7416, 0, 0},
	8:  {2440, 3144, 4256, 6184, 0, 0, 0, 0, 0},
	10: {5240, 7808, 0, 0, 0, 0, 0, 0, 0},
}

// quantenWF16: ceil(16 (t-1) / v).
func quantenWF16(t, v int) int {
	if v <= 0 || t < 1 {
		return 1 << 30
	}
	return (16*(t-1) + v - 1) / v
}

// quantenWKNoteOverride: NUR Tests der Ablaeufe (0 = Tabelle gilt).
var quantenWKNoteOverride atomic.Int64

// quantenWKNote: Pruefgroesse einer Note mit Toleranz tol, Schwelle t und v
// Validatoren; 0 = nicht sicher moeglich.
func quantenWKNote(tol, t, v int) int {
	if o := quantenWKNoteOverride.Load(); o != 0 {
		if _, ok := quantenWKTabelle[tol]; !ok {
			return 0
		}
		return int(o)
	}
	z, ok := quantenWKTabelle[tol]
	f := quantenWF16(t, v)
	if !ok || f < 0 || f >= len(z) {
		return 0
	}
	return z[f]
}

// Fristen in Sekunden Blockzeit.
const (
	quantenWFristAntwort int64 = 600  // nach dem vollstaendigen Zufall
	quantenWFristOffen   int64 = 1800 // nach der Antwort
)

var (
	quantenWDomPos  = []byte("aequitas-quanten-w-pos-v1|chain:1926|")
	quantenWDomAuf  = []byte("aequitas-quanten-w-aufforderung-v1|chain:1926|")
	quantenWDomBest = "aequitas-quanten-w-anteile-v3|chain:1926|"
	quantenWDomZK   = []byte("aequitas-quanten-w-zufallskette-v1|")
	quantenWDomZuf  = []byte("aequitas-quanten-w-zufall-v1|chain:1926|")
	// quantenWP: 2^127 - 1 (Mersenne-Primzahl), Koerper der Anteile.
	quantenWP = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 127), big.NewInt(1))
)

func quantenWHash(teile ...[]byte) [32]byte {
	h := sha256.New()
	for _, t := range teile {
		h.Write(t)
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// quantenWPruefsumme: c_i = SHA-256(Domaene | K | "|" | i | "|" | v (16 Byte)).
func quantenWPruefsumme(K string, i int, v *big.Int) [32]byte {
	var vb [16]byte
	v.FillBytes(vb[:])
	return quantenWHash(quantenWDomPos, []byte(K), []byte("|"), []byte(strconv.Itoa(i)), []byte("|"), vb[:])
}

func quantenWKnoten(a, b [32]byte) [32]byte {
	return quantenWHash([]byte{1}, a[:], b[:])
}

// quantenWMerkleWurzel: ungerade Ebenen verdoppeln das letzte Blatt (wie die Spezifikation).
func quantenWMerkleWurzel(blaetter [][32]byte) [32]byte {
	ebene := append([][32]byte(nil), blaetter...)
	for len(ebene) > 1 {
		if len(ebene)%2 == 1 {
			ebene = append(ebene, ebene[len(ebene)-1])
		}
		naechste := make([][32]byte, 0, len(ebene)/2)
		for i := 0; i < len(ebene); i += 2 {
			naechste = append(naechste, quantenWKnoten(ebene[i], ebene[i+1]))
		}
		ebene = naechste
	}
	return ebene[0]
}

func quantenWMerklePfad(blaetter [][32]byte, i int) [][32]byte {
	var pfad [][32]byte
	ebene := append([][32]byte(nil), blaetter...)
	for len(ebene) > 1 {
		if len(ebene)%2 == 1 {
			ebene = append(ebene, ebene[len(ebene)-1])
		}
		pfad = append(pfad, ebene[i^1])
		naechste := make([][32]byte, 0, len(ebene)/2)
		for j := 0; j < len(ebene); j += 2 {
			naechste = append(naechste, quantenWKnoten(ebene[j], ebene[j+1]))
		}
		ebene = naechste
		i /= 2
	}
	return pfad
}

// quantenWMerkleTiefe: Laenge jedes Pfads bei n Blaettern.
func quantenWMerkleTiefe(n int) int {
	t := 0
	for m := n; m > 1; m = (m + 1) / 2 {
		t++
	}
	return t
}

func quantenWMerklePruefen(wurzel, blatt [32]byte, i int, pfad [][32]byte, n int) bool {
	if i < 0 || i >= n || len(pfad) != quantenWMerkleTiefe(n) {
		return false
	}
	x := blatt
	for _, nachbar := range pfad {
		if i%2 == 0 {
			x = quantenWKnoten(x, nachbar)
		} else {
			x = quantenWKnoten(nachbar, x)
		}
		i /= 2
	}
	return x == wurzel
}

// quantenWAnteil: ein Shamir-Anteil (x = Index des Validators, 1-basiert).
type quantenWAnteil struct {
	X int
	Y *big.Int
}

// quantenWZusammensetzen: Lagrange an der Stelle 0 ueber F_P.
func quantenWZusammensetzen(anteile []quantenWAnteil) *big.Int {
	s := new(big.Int)
	for i, a := range anteile {
		z, n := big.NewInt(1), big.NewInt(1)
		for j, b := range anteile {
			if i == j {
				continue
			}
			z.Mul(z, big.NewInt(int64(-b.X)))
			z.Mod(z, quantenWP)
			n.Mul(n, big.NewInt(int64(a.X-b.X)))
			n.Mod(n, quantenWP)
		}
		inv := new(big.Int).ModInverse(n, quantenWP)
		term := new(big.Int).Mul(a.Y, z)
		term.Mul(term, inv)
		s.Add(s, term)
		s.Mod(s, quantenWP)
	}
	return s
}

// quantenWZerlegen: v = Basis + 2*Bit + 4*Salz.
func quantenWZerlegen(v *big.Int) (basis, bit int) {
	return int(v.Bit(0)), int(v.Bit(1))
}

// quantenWKetteVor: m Schritte der Hash-Kette eines Validators. Fuer die
// Anfrage j legt er z(j) = H^(L-1-j)(Geheimnis) offen; H^(j+1)(z(j)) ist der
// Anker, H^(j-m)(z(j)) = z(m).
func quantenWKetteVor(x [32]byte, m int) [32]byte {
	for ; m > 0; m-- {
		x = quantenWHash(quantenWDomZK, x[:])
	}
	return x
}

// quantenWZufallSaat: Saat der Aufforderung j aus den Werten ALLER
// Validatoren der Note, in ihrer Reihenfolge.
func quantenWZufallSaat(K string, j int, werte [][32]byte) [32]byte {
	teile := [][]byte{quantenWDomZuf, []byte(K), []byte("|"), []byte(strconv.Itoa(j)), []byte("|")}
	for i := range werte {
		teile = append(teile, werte[i][:])
	}
	return quantenWHash(teile...)
}

// quantenWPfandFuer: max(quantenWPfand, ceil(Wert * k / n)) -- wer Stellen
// verbraucht, setzt so viel ein, wie diese Stellen vom Wert tragen.
func quantenWPfandFuer(wert int64, k, n int) int64 {
	if wert <= 0 || k <= 0 || n <= 0 {
		return quantenWPfand
	}
	z := new(big.Int).Mul(big.NewInt(wert), big.NewInt(int64(k)))
	z.Add(z, big.NewInt(int64(n-1)))
	z.Quo(z, big.NewInt(int64(n)))
	if !z.IsInt64() {
		return math.MaxInt64 // kein Konto deckt das: fail-closed
	}
	if v := z.Int64(); v > quantenWPfand {
		return v
	}
	return quantenWPfand
}

// quantenWAufforderung: k Stellen aus frei und je eine Basis, deterministisch
// aus K, Anfrage und Saat (quantenWZufallSaat; SHA-256-Strom, Verwerfen gegen
// Verzerrung, Fisher-Yates -- genau wie die Spezifikation).
func quantenWAufforderung(K, anfrageID string, saat []byte, frei []int, k int) ([]int, []int) {
	if k < 0 || k > len(frei) {
		return nil, nil // Aufrufer prueft und weist ab (fail-closed)
	}
	strom := quantenWHash(quantenWDomAuf, []byte(K), []byte("|"), []byte(anfrageID), []byte("|"), saat)
	var zaehler uint64
	var puffer []byte
	zahl := func(grenze uint64) uint64 {
		var max uint64 = ^uint64(0) - (^uint64(0)%grenze+1)%grenze // groesstes Vielfaches - 1
		for {
			for len(puffer) < 8 {
				var zb [8]byte
				binary.BigEndian.PutUint64(zb[:], zaehler)
				blk := quantenWHash(strom[:], zb[:])
				puffer = append(puffer, blk[:]...)
				zaehler++
			}
			r := binary.BigEndian.Uint64(puffer[:8])
			puffer = puffer[8:]
			if r <= max {
				return r % grenze
			}
		}
	}
	kand := append([]int(nil), frei...)
	sort.Ints(kand)
	for j := 0; j < k; j++ {
		m := j + int(zahl(uint64(len(kand)-j)))
		kand[j], kand[m] = kand[m], kand[j]
	}
	stellen := append([]int(nil), kand[:k]...)
	sort.Ints(stellen)
	basen := make([]int, k)
	for i := range basen {
		basen[i] = int(zahl(2))
	}
	return stellen, basen
}

// quantenWErgebnis eines Urteils.
type quantenWErgebnis int

const (
	quantenWUnentschieden quantenWErgebnis = iota
	quantenWEcht
	quantenWAbgewiesen
)

// quantenWUrteil: weniger als quantenWMinTreffer Treffer -> unentschieden;
// sonst echt, wenn Fehler * 100 <= tol * Treffer (tol der Note, ganzzahlig).
func quantenWUrteil(treffer, fehler, tol int) quantenWErgebnis {
	if treffer < quantenWMinTreffer {
		return quantenWUnentschieden
	}
	if fehler*100 <= tol*treffer {
		return quantenWEcht
	}
	return quantenWAbgewiesen
}

// quantenWOffenStelle: was je gepruefter Stelle offengelegt wird.
type quantenWOffenStelle struct {
	Anteile []quantenWAnteil
	Pfad    [][32]byte
}

var errQuantenW = errors.New("quanten_w")

func quantenWFehler(format string, args ...interface{}) error {
	return fmt.Errorf("%w: "+format, append([]interface{}{errQuantenW}, args...)...)
}

// quantenWAuswerten: jeder Knoten selbst -- t Anteile mit verschiedenen x
// aus 1..v, zusammensetzen, Wert < 2^126, Pruefsumme und Pfad gegen die
// Wurzel; dann Treffer und Fehler. Ein Fehler hier heisst: die Offenlegung
// ist ungueltig (nicht: die Note ist falsch).
func quantenWAuswerten(K string, n, t, v int, wurzel [32]byte, stellen, basen, antwort []int, offen []quantenWOffenStelle) (treffer, fehler int, err error) {
	if len(offen) != len(stellen) || len(antwort) != len(stellen) || len(basen) != len(stellen) {
		return 0, 0, quantenWFehler("Zahl der Stellen")
	}
	grenze := new(big.Int).Lsh(big.NewInt(1), quantenWSalzBits+2)
	for j, i := range stellen {
		o := offen[j]
		if len(o.Anteile) != t {
			return 0, 0, quantenWFehler("Stelle %d: %d Anteile, noetig %d", i, len(o.Anteile), t)
		}
		gesehen := map[int]bool{}
		for _, a := range o.Anteile {
			if a.X < 1 || a.X > v || gesehen[a.X] || a.Y == nil || a.Y.Sign() < 0 || a.Y.Cmp(quantenWP) >= 0 {
				return 0, 0, quantenWFehler("Stelle %d: Anteil ungueltig", i)
			}
			gesehen[a.X] = true
		}
		w := quantenWZusammensetzen(o.Anteile)
		if w.Cmp(grenze) >= 0 {
			return 0, 0, quantenWFehler("Stelle %d: Wert zu gross", i)
		}
		if !quantenWMerklePruefen(wurzel, quantenWPruefsumme(K, i, w), i, o.Pfad, n) {
			return 0, 0, quantenWFehler("Stelle %d: Pruefsumme oder Pfad passt nicht", i)
		}
		basis, bit := quantenWZerlegen(w)
		if basis == basen[j] {
			treffer++
			if antwort[j] != bit {
				fehler++
			}
		}
	}
	return treffer, fehler, nil
}

// quantenWBestaetigungNachricht: was Validator x (1-basiert) beim Praegen
// bestaetigt -- er haelt die Anteile zu genau dieser Wurzel, das ist der
// Anker seiner Hash-Kette, und das ist die Praegezeit. Die Zeit macht alte
// Bestaetigungen nach dem Praegefenster wertlos: eine tote Note laesst sich
// nicht mit denselben Ankern neu praegen (ihre Hash-Ketten waeren teils
// schon offen, der Zufall vorhersagbar).
func quantenWBestaetigungNachricht(K string, wurzel [32]byte, n, t int, validatoren []string, x int, anker [32]byte, zeit int64, tol int) string {
	return fmt.Sprintf("%s%s|%x|%d|%d|%s|%d|%x|%d|%d", quantenWDomBest, K, wurzel, n, t, strings.Join(validatoren, ","), x, anker, zeit, tol)
}
