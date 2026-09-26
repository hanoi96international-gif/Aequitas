// Package bereiche ist Stufe 3 aus docs/SKALIERUNG_DEZENTRAL.md: die Konten
// werden in S Bereiche geteilt, jeder Bereich wird je Epoche von einem
// ausgelosten Ausschuss gerechnet, alle anderen uebernehmen das Ergebnis mit
// billigen Pruefungen -- und jeder kann einen falschen Bereichsblock mit
// einem Fehlerbeweis widerlegen.
//
// Das Paket ist reine, deterministische Rechnung ohne Netz und ohne
// Datenbank: Zustand, Losung, Bloecke, Quittungen, Sammelblock,
// Fehlerbeweis, Aktivierungsregel. Die Simulation vieler Knoten
// (simulation_test.go) prueft es gegen eine serielle Referenz, auch mit
// gekaperten Ausschuessen. Scharf geschaltet wird Stufe 3 erst ab
// genuegend Validatoren (Aktiv).
package bereiche

import (
	"crypto/sha256"
	"errors"
)

// SPARSE-MERKLE-BAUM.
//
// Jeder Bereich haelt seinen Zustand in einem Sparse-Merkle-Baum der Tiefe
// 256: der Schluessel ist sha256 der Kontoadresse, ein leeres Blatt hat den
// Hash null. Damit gibt es fuer jedes Konto -- auch fuer ein nicht
// vorhandenes -- einen kurzen Beweis gegen die Wurzel, und aus den Beweisen
// aller beruehrten Konten laesst sich die neue Wurzel berechnen, ohne den
// ganzen Zustand zu kennen. Genau das braucht der Fehlerbeweis: wer einen
// Bereichsblock widerlegen will, legt die beruehrten Konten vor, nicht den
// Bereich.

const tiefe = 256

// Hash ist ein 32-Byte-Wert.
type Hash [32]byte

var leer [tiefe + 1]Hash // leer[d]: Wurzel eines leeren Teilbaums der Hoehe d

func init() {
	for d := 1; d <= tiefe; d++ {
		leer[d] = knoten(leer[d-1], leer[d-1])
	}
}

func knoten(l, r Hash) Hash {
	if l == (Hash{}) && r == (Hash{}) {
		return Hash{}
	}
	var b [65]byte
	b[0] = 1
	copy(b[1:], l[:])
	copy(b[33:], r[:])
	return sha256.Sum256(b[:])
}

func bit(k Hash, i int) int { // i = 0 ist das hoechste Bit
	return int(k[i/8]>>(7-uint(i%8))) & 1
}

// Baum: nur die nicht leeren inneren Knoten werden gespeichert.
type Baum struct {
	werte map[Hash]Hash // Schluessel -> Blatthash
	innen map[pfad]Hash // (Hoehe, Praefix) -> Hash
}

type pfad struct {
	hoehe   int  // 0 = Blatt
	praefix Hash // Schluessel mit den unteren `hoehe` Bits auf null
}

func praefixVon(k Hash, hoehe int) Hash {
	// Bits ab Position tiefe-hoehe loeschen.
	p := k
	for i := tiefe - hoehe; i < tiefe; i++ {
		p[i/8] &^= 1 << (7 - uint(i%8))
	}
	return p
}

// NeuerBaum: leerer Zustand.
func NeuerBaum() *Baum { return &Baum{werte: map[Hash]Hash{}, innen: map[pfad]Hash{}} }

func (b *Baum) hashBei(hoehe int, praefix Hash) Hash {
	if h, ok := b.innen[pfad{hoehe, praefix}]; ok {
		return h
	}
	return leer[hoehe]
}

// Wurzel des Baums.
func (b *Baum) Wurzel() Hash { return b.hashBei(tiefe, Hash{}) }

// Setze: Blatt k auf wert (null = loeschen).
func (b *Baum) Setze(k, wert Hash) {
	if wert == (Hash{}) {
		delete(b.werte, k)
	} else {
		b.werte[k] = wert
	}
	h := wert
	for hoehe := 0; hoehe <= tiefe; hoehe++ {
		p := praefixVon(k, hoehe)
		if h == leer[hoehe] {
			delete(b.innen, pfad{hoehe, p})
		} else {
			b.innen[pfad{hoehe, p}] = h
		}
		if hoehe == tiefe {
			break
		}
		// Geschwister auf derselben Hoehe: das Bit an Position tiefe-1-hoehe kippen.
		pos := tiefe - 1 - hoehe
		g := p
		g[pos/8] ^= 1 << (7 - uint(pos%8))
		gh := b.hashBei(hoehe, g)
		if bit(k, pos) == 0 {
			h = knoten(h, gh)
		} else {
			h = knoten(gh, h)
		}
	}
}

// Wert: Blatthash von k (null, wenn leer).
func (b *Baum) Wert(k Hash) Hash { return b.werte[k] }

// Beweis: die Geschwister von unten nach oben; leere werden nur als Bit
// markiert (Nichtleer), damit Beweise kurz bleiben.
type Beweis struct {
	Nichtleer [tiefe / 8]byte
	Geschw    []Hash
}

// Beweise k gegen die aktuelle Wurzel.
func (b *Baum) Beweise(k Hash) Beweis {
	var bw Beweis
	for hoehe := 0; hoehe < tiefe; hoehe++ {
		pos := tiefe - 1 - hoehe
		g := praefixVon(k, hoehe)
		g[pos/8] ^= 1 << (7 - uint(pos%8))
		gh := b.hashBei(hoehe, g)
		if gh != leer[hoehe] {
			bw.Nichtleer[hoehe/8] |= 1 << uint(hoehe%8)
			bw.Geschw = append(bw.Geschw, gh)
		}
	}
	return bw
}

var ErrBeweis = errors.New("Merkle-Beweis passt nicht")

// geschwister: Beweis ausgepackt (alle 256 Geschwister).
func (bw Beweis) geschwister() ([tiefe]Hash, error) {
	var g [tiefe]Hash
	j := 0
	for hoehe := 0; hoehe < tiefe; hoehe++ {
		if bw.Nichtleer[hoehe/8]&(1<<uint(hoehe%8)) != 0 {
			if j >= len(bw.Geschw) {
				return g, ErrBeweis
			}
			g[hoehe] = bw.Geschw[j]
			j++
		} else {
			g[hoehe] = leer[hoehe]
		}
	}
	if j != len(bw.Geschw) {
		return g, ErrBeweis
	}
	return g, nil
}

// WurzelAus: die Wurzel, die sich aus Blatt wert bei k und dem Beweis ergibt.
func WurzelAus(k, wert Hash, bw Beweis) (Hash, error) {
	g, err := bw.geschwister()
	if err != nil {
		return Hash{}, err
	}
	h := wert
	for hoehe := 0; hoehe < tiefe; hoehe++ {
		if bit(k, tiefe-1-hoehe) == 0 {
			h = knoten(h, g[hoehe])
		} else {
			h = knoten(g[hoehe], h)
		}
	}
	return h, nil
}

// Teilbaum: ein Baum, der nur die beruehrten Blaetter samt Beweisen kennt --
// genug, um Aenderungen an genau diesen Blaettern nachzurechnen. Grundlage
// des Fehlerbeweises.
type Teilbaum struct {
	wurzel Hash
	b      *Baum
}

// TeilbaumAus: aus (Schluessel, Wert, Beweis)-Tripeln gegen wurzel. Jeder
// Beweis muss zur Wurzel passen.
func TeilbaumAus(wurzel Hash, schluessel []Hash, werte []Hash, beweise []Beweis) (*Teilbaum, error) {
	t := &Teilbaum{wurzel: wurzel, b: NeuerBaum()}
	for i, k := range schluessel {
		r, err := WurzelAus(k, werte[i], beweise[i])
		if err != nil {
			return nil, err
		}
		if r != wurzel {
			return nil, ErrBeweis
		}
		g, _ := beweise[i].geschwister()
		// Geschwister eintragen, dann das Blatt -- Setze rechnet damit hoch.
		for hoehe := 0; hoehe < tiefe; hoehe++ {
			pos := tiefe - 1 - hoehe
			gp := praefixVon(k, hoehe)
			gp[pos/8] ^= 1 << (7 - uint(pos%8))
			if _, schon := t.b.innen[pfad{hoehe, gp}]; !schon && g[hoehe] != leer[hoehe] {
				t.b.innen[pfad{hoehe, gp}] = g[hoehe]
			}
		}
		t.b.Setze(k, werte[i])
	}
	if len(schluessel) > 0 && t.b.Wurzel() != wurzel {
		return nil, ErrBeweis
	}
	return t, nil
}

// Setze und Wurzel wie beim vollen Baum -- nur fuer die bewiesenen Blaetter
// korrekt.
func (t *Teilbaum) Setze(k, wert Hash) { t.b.Setze(k, wert) }
func (t *Teilbaum) Wurzel() Hash       { return t.b.Wurzel() }
func (t *Teilbaum) Wert(k Hash) Hash   { return t.b.Wert(k) }
