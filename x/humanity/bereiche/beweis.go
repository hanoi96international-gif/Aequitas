package bereiche

import (
	"errors"
	"reflect"
)

// FEHLERBEWEIS.
//
// Wer einen falschen Bereichsblock findet, veroeffentlicht den Block und den
// Vorzustand der beruehrten Konten samt Merkle-Beweisen gegen die Wurzel vor
// dem Block. Jeder kann das billig nachrechnen: Beweise pruefen, den Block
// auf genau diesen Konten ausfuehren (Ausfuehren, dieselbe Funktion wie der
// Ausschuss), das Ergebnis mit dem Block vergleichen. Wer den Bereich nicht
// haelt, braucht dafuer nur den Beweis.

// BewiesenesKonto: ein Konto mit Beweis.
type BewiesenesKonto struct {
	Addr   string
	Konto  Konto
	Beweis Beweis
}

// BewieseneQuittung: Einloesestand einer Quittung mit Beweis.
type BewieseneQuittung struct {
	ID         Hash
	Verbraucht bool
	Beweis     Beweis
}

// Vorzustand: die beruehrten Teile des Zustands vor einem Block.
type Vorzustand struct {
	Wurzel     Hash
	Konten     []BewiesenesKonto
	Quittungen []BewieseneQuittung
}

// BeweisZustand: Zustand, der nur die bewiesenen Konten kennt.
type BeweisZustand struct {
	t          *Teilbaum
	konten     map[string]Konto
	quittungen map[Hash]bool
}

// NeuerBeweisZustand prueft alle Beweise gegen v.Wurzel.
func NeuerBeweisZustand(v Vorzustand) (*BeweisZustand, error) {
	var ks, ws []Hash
	var bws []Beweis
	bz := &BeweisZustand{konten: map[string]Konto{}, quittungen: map[Hash]bool{}}
	for _, k := range v.Konten {
		ks = append(ks, kontoSchluessel(k.Addr))
		ws = append(ws, k.Konto.hash(k.Addr))
		bws = append(bws, k.Beweis)
		bz.konten[k.Addr] = k.Konto
	}
	for _, q := range v.Quittungen {
		ks = append(ks, quittungSchluessel(q.ID))
		var w Hash
		if q.Verbraucht {
			w = verbrauchtWert
		}
		ws = append(ws, w)
		bws = append(bws, q.Beweis)
		bz.quittungen[q.ID] = q.Verbraucht
	}
	t, err := TeilbaumAus(v.Wurzel, ks, ws, bws)
	if err != nil {
		return nil, err
	}
	bz.t = t
	return bz, nil
}

func (z *BeweisZustand) Konto(addr string) (Konto, error) {
	k, ok := z.konten[addr]
	if !ok {
		return Konto{}, ErrFehltImBeweis
	}
	return k, nil
}
func (z *BeweisZustand) SetzeKonto(addr string, k Konto) {
	z.konten[addr] = k
	z.t.Setze(kontoSchluessel(addr), k.hash(addr))
}
func (z *BeweisZustand) Verbraucht(id Hash) (bool, error) {
	v, ok := z.quittungen[id]
	if !ok {
		return false, ErrFehltImBeweis
	}
	return v, nil
}
func (z *BeweisZustand) Verbrauche(id Hash) {
	z.quittungen[id] = true
	z.t.Setze(quittungSchluessel(id), verbrauchtWert)
}
func (z *BeweisZustand) Wurzel() Hash { return z.t.Wurzel() }

// Fehlerbeweis gegen einen Bereichsblock.
type Fehlerbeweis struct {
	Block BereichsBlock
	Vor   Vorzustand
}

var ErrBeweisUngueltig = errors.New("Fehlerbeweis ungueltig")

// PruefeFehlerbeweis: widerlegt fb den Block? r ist der Rahmen, den der
// PRUEFENDE fuer diese Epoche kennt (aus seinem Sammelblock) -- nicht der aus
// dem Beweis. Rueckgabe: (true, nil) -- der Block ist falsch; (false, nil) --
// der Block ist richtig; (_, ErrBeweisUngueltig) -- der Beweis taugt nichts
// (Beweise passen nicht, Konten fehlen).
func PruefeFehlerbeweis(fb Fehlerbeweis, r Rahmen) (bool, error) {
	b := fb.Block
	if fb.Vor.Wurzel != b.Vor {
		return false, ErrBeweisUngueltig
	}
	bz, err := NeuerBeweisZustand(fb.Vor)
	if err != nil {
		return false, ErrBeweisUngueltig
	}
	erg, err := Ausfuehren(bz, b.Bereich, r, b.Eingaben, b.EingabenID())
	if errors.Is(err, ErrFehltImBeweis) {
		return false, ErrBeweisUngueltig
	}
	if err != nil {
		return true, nil // die Eingaben selbst sind unzulaessig
	}
	falsch := erg.Wurzel != b.Nach || erg.Teilsumme != b.Teilsumme ||
		!reflect.DeepEqual(nilLeer(erg.Ausgang), nilLeer(b.Ausgang))
	return falsch, nil
}

func nilLeer(q []Quittung) []Quittung {
	if len(q) == 0 {
		return nil
	}
	return q
}

// ErstelleFehlerbeweis: wer den Bereich haelt (vor dem Block), baut den
// Beweis. Die beruehrten Konten ermittelt eine Probeausfuehrung auf einer
// Kopie.
func ErstelleFehlerbeweis(vor *VollZustand, b BereichsBlock, r Rahmen) Fehlerbeweis {
	probe := vor.Kopie()
	erg, _ := Ausfuehren(probe, b.Bereich, r, b.Eingaben, b.EingabenID())
	return Fehlerbeweis{Block: b, Vor: vor.Beweisfuer(erg.Beruehrt, erg.Quittungen)}
}
