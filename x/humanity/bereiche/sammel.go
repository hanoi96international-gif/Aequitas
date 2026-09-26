package bereiche

import (
	"crypto/sha256"
	"encoding/binary"
)

// SAMMELBLOCK je Epoche: addiert die Teilsummen aller Bereiche
// deterministisch. Daraus: Geldmenge, Menschen, Grundeinkommen je Mensch,
// Vermoegensgrenze und die Zufallszahl der naechsten Losung.
//
// Grundeinkommen: die Gebuehren (und Kappungen) einer Epoche gehen in den
// Topf; je Mensch wird ganzzahlig verteilt, der Rest bleibt im Topf fuer die
// naechste Epoche. Ausgezahlt wird je Konto, wenn es handelt (Konto.GEStand),
// also in O(1) statt in einem Durchlauf ueber alle Menschen.
//
// Die Erhaltung, die der Sammelblock prueft:
//
//	Summe Geldmenge der Bereiche + offene Quittungen + Topf
//	  = Summe aller Registrierungen - Summe aller Ausstiege
type Sammelblock struct {
	Epoche         uint64
	Vorher         Hash
	Koepfe         []Hash // je Bereich der letzte Kopf der Epoche
	Geldmenge      int64
	Menschen       int
	OffeneQuittung int64
	Topf           int64 // eingegangen, noch nicht ausgezahlt
	NeuFuerGE      int64 // davon noch nicht je Mensch zugesagt
	GEJeMensch     int64
	GEKumuliert    int64
	Grenze         int64
	Geschoepft     int64 // seit Beginn
	Ausgestiegen   int64 // seit Beginn
}

// Hash des Sammelblocks.
func (s Sammelblock) Hash() Hash { return hashJSON(s) }

// Zufall fuer die Losung der naechsten Epoche.
func (s Sammelblock) Zufall() Hash {
	h := s.Hash()
	var b []byte
	b = append(b, "zufall|"...)
	b = append(b, h[:]...)
	b = binary.BigEndian.AppendUint64(b, s.Epoche)
	return sha256.Sum256(b)
}

// Rahmen fuer die naechste Epoche.
func (s Sammelblock) Rahmen(bereiche int) Rahmen {
	return Rahmen{S: bereiche, GEKumuliert: s.GEKumuliert, Grenze: s.Grenze}
}

// NaechsterSammelblock aus dem vorigen, den Bloecken der Epoche (alle
// Bereiche, in Bereichsreihenfolge) und den offenen Quittungen am Ende.
func NaechsterSammelblock(vor Sammelblock, geldmengen []int64, menschen []int, bloecke []BereichsBlock, offen int64) Sammelblock {
	s := Sammelblock{Epoche: vor.Epoche + 1, Vorher: vor.Hash(), Geschoepft: vor.Geschoepft, Ausgestiegen: vor.Ausgestiegen,
		GEKumuliert: vor.GEKumuliert, Topf: vor.Topf, NeuFuerGE: vor.NeuFuerGE}
	for _, g := range geldmengen {
		s.Geldmenge += g
	}
	for _, m := range menschen {
		s.Menschen += m
	}
	for _, b := range bloecke {
		t := b.Teilsumme
		s.Topf += t.Gebuehren - t.GEAusgezahlt
		s.NeuFuerGE += t.Gebuehren
		s.Geschoepft += t.Geschoepft
		s.Ausgestiegen += t.Ausstiege
		s.Koepfe = append(s.Koepfe, b.Kopf())
	}
	s.OffeneQuittung = offen
	if s.Menschen > 0 {
		s.GEJeMensch = s.NeuFuerGE / int64(s.Menschen)
		s.NeuFuerGE -= s.GEJeMensch * int64(s.Menschen)
		s.GEKumuliert += s.GEJeMensch
		s.Grenze = s.Geldmenge / int64(s.Menschen) * grenzeFaktor
	}
	return s
}

// Erhalten: gilt die Erhaltung der Geldmenge ueber das ganze Netz?
func (s Sammelblock) Erhalten() bool {
	return s.Geldmenge+s.OffeneQuittung+s.Topf == s.Geschoepft-s.Ausgestiegen
}
