package zkgueltigkeit

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc"
	"github.com/consensys/gnark-crypto/ecc/bn254/twistededwards/eddsa"
	tedwards "github.com/consensys/gnark-crypto/ecc/twistededwards"
	"github.com/consensys/gnark/frontend"
)

// Zustand ausserhalb des Schaltkreises: Konten im MiMC-Merkle-Baum.
type Zustand struct {
	Tiefe  int
	Konten []Konto
	ebenen [][]fr.Element // ebenen[0] = Blaetter
}

// Konto ausserhalb des Schaltkreises.
type Konto struct {
	Schluessel *eddsa.PrivateKey
	Guthaben   uint64
	Nonce      uint64
}

func mimcVon(xs ...fr.Element) fr.Element {
	h := mimc.NewMiMC()
	for _, x := range xs {
		b := x.Bytes()
		h.Write(b[:])
	}
	var r fr.Element
	r.SetBytes(h.Sum(nil))
	return r
}

func feld(v uint64) fr.Element {
	var e fr.Element
	e.SetUint64(v)
	return e
}

func (k Konto) blatt() fr.Element {
	return mimcVon(k.Schluessel.PublicKey.A.X, k.Schluessel.PublicKey.A.Y, feld(k.Guthaben), feld(k.Nonce))
}

// NeuerZustand: 2^tiefe Konten mit je guthaben.
func NeuerZustand(tiefe int, guthaben uint64) (*Zustand, error) {
	z := &Zustand{Tiefe: tiefe}
	for i := 0; i < 1<<tiefe; i++ {
		k, err := eddsa.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		z.Konten = append(z.Konten, Konto{Schluessel: k, Guthaben: guthaben})
	}
	z.neuBerechnen()
	return z, nil
}

func (z *Zustand) neuBerechnen() {
	z.ebenen = make([][]fr.Element, z.Tiefe+1)
	for _, k := range z.Konten {
		z.ebenen[0] = append(z.ebenen[0], k.blatt())
	}
	for d := 1; d <= z.Tiefe; d++ {
		unten := z.ebenen[d-1]
		for i := 0; i < len(unten); i += 2 {
			z.ebenen[d] = append(z.ebenen[d], mimcVon(unten[i], unten[i+1]))
		}
	}
}

func (z *Zustand) setze(i int, k Konto) {
	z.Konten[i] = k
	z.ebenen[0][i] = k.blatt()
	for d := 1; d <= z.Tiefe; d++ {
		j := (i >> d) << 1
		z.ebenen[d][i>>d] = mimcVon(z.ebenen[d-1][j], z.ebenen[d-1][j+1])
	}
}

// Wurzel des Baums.
func (z *Zustand) Wurzel() fr.Element { return z.ebenen[z.Tiefe][0] }

func (z *Zustand) pfad(i int) []frontend.Variable {
	var p []frontend.Variable
	for d := 0; d < z.Tiefe; d++ {
		p = append(p, z.ebenen[d][(i>>d)^1])
	}
	return p
}

// Auftrag: was ein Mensch ueberweisen will.
type Auftrag struct {
	Von, An int
	Betrag  uint64
}

// Nachricht, die der Absender signiert.
func nachricht(von, an int, betrag, nonce uint64) fr.Element {
	return mimcVon(feld(uint64(von)), feld(uint64(an)), feld(betrag), feld(nonce))
}

// Buendel wendet die Auftraege an und liefert den Zeugen fuer den Beweis.
// Aendert den Zustand.
func (z *Zustand) Buendel(auftraege []Auftrag) (*Kreis, error) {
	k := NeuerKreis(z.Tiefe, len(auftraege))
	k.AltWurzel = z.Wurzel()
	for i, a := range auftraege {
		von, an := z.Konten[a.Von], z.Konten[a.An]
		if a.Von == a.An || von.Guthaben < a.Betrag {
			return nil, fmt.Errorf("Auftrag %d ungueltig", i)
		}
		n := nachricht(a.Von, a.An, a.Betrag, von.Nonce)
		nb := n.Bytes()
		sig, err := von.Schluessel.Sign(nb[:], mimc.NewMiMC())
		if err != nil {
			return nil, err
		}
		tx := &k.Txs[i]
		tx.VonIndex, tx.AnIndex, tx.Betrag = a.Von, a.An, a.Betrag
		tx.Von = kontoKreis(von)
		tx.VonPfad = z.pfad(a.Von)
		tx.Sig.Assign(tedwards.BN254, sig)

		von.Guthaben -= a.Betrag
		von.Nonce++
		z.setze(a.Von, von)

		an = z.Konten[a.An]
		tx.An = kontoKreis(an)
		tx.AnPfad = z.pfad(a.An)
		an.Guthaben += a.Betrag
		z.setze(a.An, an)
	}
	k.NeuWurzel = z.Wurzel()
	return k, nil
}

func kontoKreis(k Konto) KontoKreis {
	var kk KontoKreis
	kk.Schluessel.Assign(tedwards.BN254, k.Schluessel.PublicKey.Bytes())
	kk.Guthaben = new(big.Int).SetUint64(k.Guthaben)
	kk.Nonce = new(big.Int).SetUint64(k.Nonce)
	return kk
}
