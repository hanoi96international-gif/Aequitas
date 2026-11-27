package keeper

// WEG C, Schritt C2b-1 (forschung/quantengeld/WEG_C.md): die Meldung der
// gemessenen Pulse und die oeffentliche Teilmenge, im Knoten.
//
// Ueber eine echte Quantenleitung kommt nur ein Teil der Pulse bei der
// Inhaberin an. Sie meldet zuerst die Menge Lambda der Pulse, die sie
// gemessen hat. Erst DANACH zieht der Zufall eines spaeteren Blocks daraus die
// Teilmenge von hoechstens quantenWNMax Stellen, die den Token bildet. Weil
// die Teilmenge nach der Meldung gezogen wird, kann eine Betruegerin den
// Anteil freier Stellen (Mehrphotonen-Pulse) darin nicht erhoehen; die
// Pruefgroesse rechnet damit (quanten_s_leitung.go).
//
// Spezifikation: s_kette.py (s3_meldung, s3_teilmenge, s3_kodieren,
// s3_dekodieren); Pruefvektoren testdata/quanten_s3_vektor.json und
// testdata/quanten_s3_meldung_vektor.json (CI vergleicht sie mit der
// Spezifikation).
//
// AUF DER KETTE: hoechstens quantenS3MeldungMax Pulse, als Luecken kodiert
// (LEB128, genau eine Form, Hex). 65.536 Pulse sind hoechstens 512 KiB Hex
// und fuer jeden Knoten in Millisekunden gelesen und sortiert.
//
// NOCH NICHT IM KONSENS: nur Rechnung (wie C1). Die Transaktion, die eine
// Meldung bucht, kommt hinter einem eigenen Stichtag.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
)

const (
	quantenS3PulsMax       = 1 << 26 // Pulse je Ausgabe (Kents Handsender: 4e7 in 4 s)
	quantenS3MeldungMax    = 1 << 16 // gemeldete Pulse auf der Kette
	quantenS3MeldungHexMax = 2 * 4 * quantenS3MeldungMax
)

var quantenS3DomTeil = []byte("aequitas-quanten-s3-teilmenge-v1|")

var errQuantenS3 = errors.New("S3")

func quantenS3Fehler(format string, a ...interface{}) error {
	return fmt.Errorf("%w: "+format, append([]interface{}{errQuantenS3}, a...)...)
}

// quantenS3Kodieren: die eine Form einer Meldung (aufsteigend, verschieden).
func quantenS3Kodieren(lam []int) (string, error) {
	if len(lam) < 1 || len(lam) > quantenS3MeldungMax || lam[0] < 0 || lam[len(lam)-1] >= quantenS3PulsMax {
		return "", quantenS3Fehler("Kodierung: Form")
	}
	aus := make([]byte, 0, 2*len(lam))
	vorher := -1
	for _, i := range lam {
		if i <= vorher {
			return "", quantenS3Fehler("Kodierung: nicht aufsteigend")
		}
		g := i - vorher - 1
		for {
			b := byte(g & 0x7f)
			g >>= 7
			if g != 0 {
				aus = append(aus, b|0x80)
				continue
			}
			aus = append(aus, b)
			break
		}
		vorher = i
	}
	return hex.EncodeToString(aus), nil
}

// quantenS3Dekodieren: streng und fail-closed; Groesse vor dem Lesen.
func quantenS3Dekodieren(text string) ([]int, error) {
	if len(text) < 2 || len(text) > quantenS3MeldungHexMax || len(text)%2 != 0 {
		return nil, quantenS3Fehler("Meldung: Laenge")
	}
	for i := 0; i < len(text); i++ {
		if c := text[i]; !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return nil, quantenS3Fehler("Meldung: kein Hex")
		}
	}
	roh, err := hex.DecodeString(text)
	if err != nil {
		return nil, quantenS3Fehler("Meldung: kein Hex")
	}
	lam := make([]int, 0, len(roh))
	vorher, g, schritt, gruppen := -1, 0, 0, 0
	for _, b := range roh {
		g |= int(b&0x7f) << schritt
		schritt += 7
		gruppen++
		if b&0x80 != 0 {
			if gruppen == 4 {
				return nil, quantenS3Fehler("Meldung: Zahl zu lang")
			}
			continue
		}
		if gruppen > 1 && b == 0 {
			return nil, quantenS3Fehler("Meldung: Gruppe zu viel")
		}
		i := vorher + 1 + g
		if i >= quantenS3PulsMax || len(lam) == quantenS3MeldungMax {
			return nil, quantenS3Fehler("Meldung: Puls oder Zahl zu gross")
		}
		lam = append(lam, i)
		vorher, g, schritt, gruppen = i, 0, 0, 0
	}
	if gruppen != 0 {
		return nil, quantenS3Fehler("Meldung: abgebrochen")
	}
	return lam, nil
}

// quantenS3MeldungPruefen wie s_kette.s3_meldung: verschiedene Pulse in
// 0..nPuls-1 (aufsteigend, wie die Kodierung sie liefert), mindestens
// gammaDetPpm/10^6 von nPuls.
func quantenS3MeldungPruefen(nPuls, gammaDetPpm int, lam []int) error {
	if nPuls < 1 || nPuls > quantenS3PulsMax || gammaDetPpm < 1 || gammaDetPpm > 1_000_000 {
		return quantenS3Fehler("Meldung: Groessen")
	}
	vorher := -1
	for _, i := range lam {
		if i <= vorher || i >= nPuls {
			return quantenS3Fehler("Meldung: doppelter oder fremder Puls")
		}
		vorher = i
	}
	if int64(len(lam))*1_000_000 < int64(gammaDetPpm)*int64(nPuls) {
		return quantenS3Fehler("Meldung: %d Pulse, verlangt %d ppm von %d", len(lam), gammaDetPpm, nPuls)
	}
	return nil
}

// quantenS3Teilmenge wie s_kette.s3_teilmenge: die nMax Pulse der Meldung mit
// den kleinsten SHA-256(dom|K|"|"|zufall|"|"|i), aufsteigend sortiert. Der
// Zufall muss aus einem Block NACH der Meldung stammen.
func quantenS3Teilmenge(K string, lam []int, zufall []byte, meldungHoehe, zufallHoehe int64, nMax int) ([]int, error) {
	if zufallHoehe <= meldungHoehe {
		return nil, quantenS3Fehler("Teilmenge: Zufall nicht nach der Meldung")
	}
	if len(zufall) != 32 || nMax < 1 || nMax > quantenWNMax || len(lam) > quantenS3MeldungMax {
		return nil, quantenS3Fehler("Teilmenge: Form")
	}
	aus := quantenS3Rang(K, lam, zufall, nMax)
	sort.Ints(aus)
	return aus, nil
}

// MEHRERE SENDER (C2b-2b-4, s_kette.s3_abschnitt_pulse, WEG_C.md
// „Designfrage“): Sender x (1..v) schickt die Pulse [(x-1) N/v, x N/v). Wer
// einen Sender bedient, kennt die Werte seines Abschnitts. ANGRIFF ohne
// Abschnitte: Die Inhaberin weiss, welcher Puls von welchem Sender kommt; im
// Bund mit einem Sender meldete sie bevorzugt dessen Pulse, und der Token
// bestuende fast nur aus Stellen, die die Faelscherseite kennt. Darum meldet
// JEDER Abschnitt fuer sich mindestens gamma_det seiner Pulse, und die
// Teilmenge zieht aus jedem Abschnitt GLEICH viele. Mit v = 1 genau wie bisher.

// quantenS3AbschnittPulse: [von, bis) der Pulse von Sender x (1-basiert).
func quantenS3AbschnittPulse(nPuls, v, x int) (von, bis int, ok bool) {
	if v < 1 || v > quantenWVMax || x < 1 || x > v || nPuls < v || nPuls > quantenS3PulsMax {
		return 0, 0, false
	}
	return (x - 1) * nPuls / v, x * nPuls / v, true
}

// quantenS3MeldungPruefenAbschnitte wie s_kette.s3_meldung_abschnitte.
func quantenS3MeldungPruefenAbschnitte(nPuls, gammaDetPpm int, lam []int, v int) error {
	if err := quantenS3MeldungPruefen(nPuls, gammaDetPpm, lam); err != nil {
		return err
	}
	j := 0
	for x := 1; x <= v; x++ {
		von, bis, ok := quantenS3AbschnittPulse(nPuls, v, x)
		if !ok {
			return quantenS3Fehler("Meldung: Abschnitt %d von %d", x, v)
		}
		zahl := 0
		for ; j < len(lam) && lam[j] < bis; j++ { // lam aufsteigend (geprueft)
			if lam[j] >= von {
				zahl++
			}
		}
		if int64(zahl)*1_000_000 < int64(gammaDetPpm)*int64(bis-von) {
			return quantenS3Fehler("Meldung: Abschnitt %d mit %d von %d Pulsen unter gamma_det", x, zahl, bis-von)
		}
	}
	return nil
}

// quantenS3TeilmengeAbschnitte wie s_kette.s3_teilmenge_abschnitte: aus jedem
// Abschnitt je = min(kleinste Zahl gemeldeter Pulse eines Abschnitts,
// nMax / v) Pulse, je Abschnitt nach demselben Rang wie quantenS3Teilmenge.
// Aufsteigend; Stelle (x-1)·je .. x·je-1 des Tokens liegt im Abschnitt von
// Sender x. lam aufsteigend und verschieden (wie die Kodierung liefert).
func quantenS3TeilmengeAbschnitte(K string, lam []int, zufall []byte, meldungHoehe, zufallHoehe int64, nPuls, v, nMax int) ([]int, error) {
	if zufallHoehe <= meldungHoehe {
		return nil, quantenS3Fehler("Teilmenge: Zufall nicht nach der Meldung")
	}
	if len(zufall) != 32 || nMax < 1 || nMax > quantenWNMax || v < 1 || v > nMax || len(lam) > quantenS3MeldungMax {
		return nil, quantenS3Fehler("Teilmenge: Form")
	}
	teile := make([][]int, v)
	j := 0
	for x := 1; x <= v; x++ {
		von, bis, ok := quantenS3AbschnittPulse(nPuls, v, x)
		if !ok {
			return nil, quantenS3Fehler("Teilmenge: Abschnitt %d von %d", x, v)
		}
		for ; j < len(lam) && lam[j] < bis; j++ {
			if lam[j] >= von {
				teile[x-1] = append(teile[x-1], lam[j])
			}
		}
	}
	je := nMax / v
	for _, t := range teile {
		je = min(je, len(t))
	}
	if je < 1 {
		return nil, quantenS3Fehler("Teilmenge: ein Abschnitt ohne gemeldeten Puls")
	}
	aus := make([]int, 0, je*v)
	for _, t := range teile {
		aus = append(aus, quantenS3Rang(K, t, zufall, je)...)
	}
	sort.Ints(aus)
	return aus, nil
}

// quantenS3Rang: die nMax Pulse mit den kleinsten SHA-256(dom|K|"|"|zufall|"|"|i)
// (Gleichstand: kleinerer Puls), unsortiert.
func quantenS3Rang(K string, lam []int, zufall []byte, nMax int) []int {
	type rang struct {
		i int
		h [32]byte
	}
	r := make([]rang, len(lam))
	puffer := make([]byte, 0, len(quantenS3DomTeil)+len(K)+1+32+1+20)
	for j, i := range lam {
		puffer = puffer[:0]
		puffer = append(puffer, quantenS3DomTeil...)
		puffer = append(puffer, K...)
		puffer = append(puffer, '|')
		puffer = append(puffer, zufall...)
		puffer = append(puffer, '|')
		puffer = strconv.AppendInt(puffer, int64(i), 10)
		r[j] = rang{i: i, h: sha256.Sum256(puffer)}
	}
	sort.Slice(r, func(a, b int) bool {
		if c := bytes.Compare(r[a].h[:], r[b].h[:]); c != 0 {
			return c < 0
		}
		return r[a].i < r[b].i
	})
	if len(r) > nMax {
		r = r[:nMax]
	}
	aus := make([]int, len(r))
	for j := range r {
		aus[j] = r[j].i
	}
	return aus
}
