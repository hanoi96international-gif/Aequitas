package bereiche

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
)

// BereichsBlock: was ein Ausschuss fuer seinen Bereich erzeugt und mit >= 2/3
// unterschreibt.
type BereichsBlock struct {
	Bereich   int
	Epoche    uint64
	Nr        uint64 // fortlaufend je Bereich
	Vor, Nach Hash
	Rahmen    Rahmen
	Eingaben  Eingaben
	Ausgang   []Quittung
	Teilsumme Teilsumme
	// Geldmenge des Bereichs nach dem Block -- fuer die billige Pruefung der
	// Erhaltung ueber die Kopfkette.
	Geldmenge int64
	Menschen  int
	Sig       map[string][]byte // Ausschussmitglied -> Signatur ueber Kopf()
	// PruefSig: der zweite, unabhaengig geloste Pruefausschuss -- noetig,
	// bevor Ausstiege ausgezahlt werden.
	PruefSig map[string][]byte
}

func hashJSON(v interface{}) Hash {
	b, _ := json.Marshal(v)
	return sha256.Sum256(b)
}

// EingabenID: haengt nur an dem, was VOR der Ausfuehrung feststeht -- daraus
// werden die Quittungs-IDs abgeleitet.
func (b BereichsBlock) EingabenID() Hash {
	var x []byte
	x = append(x, "bereichsblock-eingaben|"...)
	x = binary.BigEndian.AppendUint64(x, uint64(b.Bereich))
	x = binary.BigEndian.AppendUint64(x, b.Epoche)
	x = binary.BigEndian.AppendUint64(x, b.Nr)
	x = append(x, b.Vor[:]...)
	h := hashJSON(b.Eingaben)
	x = append(x, h[:]...)
	return sha256.Sum256(x)
}

// Kopf: was unterschrieben wird.
func (b BereichsBlock) Kopf() Hash {
	k := struct {
		ID        Hash
		Nach      Hash
		Rahmen    Rahmen
		Ausgang   Hash
		Teilsumme Teilsumme
		Geldmenge int64
		Menschen  int
	}{b.EingabenID(), b.Nach, b.Rahmen, hashJSON(b.Ausgang), b.Teilsumme, b.Geldmenge, b.Menschen}
	return hashJSON(k)
}

// Unterschreibe als Ausschussmitglied.
func (b *BereichsBlock) Unterschreibe(addr string, priv ed25519.PrivateKey) {
	if b.Sig == nil {
		b.Sig = map[string][]byte{}
	}
	k := b.Kopf()
	b.Sig[addr] = ed25519.Sign(priv, k[:])
}

// PruefUnterschrift als Mitglied des Pruefausschusses.
func (b *BereichsBlock) PruefUnterschrift(addr string, priv ed25519.PrivateKey) {
	if b.PruefSig == nil {
		b.PruefSig = map[string][]byte{}
	}
	k := b.Kopf()
	b.PruefSig[addr] = append(append([]byte{}, "pruef:"...), ed25519.Sign(priv, append([]byte("pruef:"), k[:]...))...)
}

// Mehrheit: mindestens zwei Drittel.
func Mehrheit(m int) int { return (2*m + 2) / 3 }

func zaehle(sig map[string][]byte, ausschuss []string, nachricht []byte, praefix string) int {
	n := 0
	for _, a := range ausschuss {
		s, ok := sig[a]
		if !ok || len(s) < len(praefix) || string(s[:len(praefix)]) != praefix {
			continue
		}
		pub, err := pubAus(a)
		if err == nil && ed25519.Verify(pub, nachricht, s[len(praefix):]) {
			n++
		}
	}
	return n
}

// BestaetigtVon: haben >= 2/3 des Ausschusses unterschrieben?
func (b BereichsBlock) BestaetigtVon(ausschuss []string) bool {
	k := b.Kopf()
	return zaehle(b.Sig, ausschuss, k[:], "") >= Mehrheit(len(ausschuss))
}

// GeprueftVon: haben >= 2/3 des Pruefausschusses unterschrieben?
func (b BereichsBlock) GeprueftVon(pruefer []string) bool {
	k := b.Kopf()
	return zaehle(b.PruefSig, pruefer, append([]byte("pruef:"), k[:]...), "pruef:") >= Mehrheit(len(pruefer))
}

// BilligPruefen: was JEDER Knoten mit einem Bereichsblock tut, ohne den
// Bereich zu halten. vorher: der letzte angenommene Block des Bereichs (nil
// fuer den ersten). Prueft Unterschriften, Kette, Rahmen und Erhaltung der
// Geldmenge ueber die Kopfkette. "Kein Konto unter null" und die StateRoot
// selbst kann nur pruefen, wer den Bereich haelt -- dafuer gibt es den
// Fehlerbeweis.
func BilligPruefen(b BereichsBlock, vorher *BereichsBlock, ausschuss []string, r Rahmen) error {
	if !b.BestaetigtVon(ausschuss) {
		return fmt.Errorf("Bereich %d Block %d: weniger als 2/3 des Ausschusses", b.Bereich, b.Nr)
	}
	if b.Rahmen != r {
		return fmt.Errorf("Bereich %d Block %d: falscher Rahmen", b.Bereich, b.Nr)
	}
	vg, vm, vn := int64(0), 0, uint64(0)
	var vw Hash
	if vorher != nil {
		vg, vm, vn, vw = vorher.Geldmenge, vorher.Menschen, vorher.Nr, vorher.Nach
		if b.Nr != vn+1 || b.Vor != vw {
			return fmt.Errorf("Bereich %d Block %d: Kette gebrochen", b.Bereich, b.Nr)
		}
	}
	var aus int64
	for _, q := range b.Ausgang {
		if q.VonBereich != b.Bereich || q.Betrag <= 0 {
			return fmt.Errorf("Bereich %d: fremde Quittung", b.Bereich)
		}
		aus += q.Betrag
	}
	var ein int64
	for _, q := range b.Eingaben.Eingang {
		ein += q.Betrag
	}
	t := b.Teilsumme
	if aus != t.Ausgang || ein != t.Eingang {
		return fmt.Errorf("Bereich %d Block %d: Quittungssummen passen nicht", b.Bereich, b.Nr)
	}
	soll := vg + t.Eingang + t.Geschoepft + t.GEAusgezahlt - t.Ausgang - t.Gebuehren - t.Ausstiege
	if b.Geldmenge != soll || b.Geldmenge < 0 {
		return fmt.Errorf("Bereich %d Block %d: Geldmenge nicht erhalten (%d statt %d)", b.Bereich, b.Nr, b.Geldmenge, soll)
	}
	if b.Menschen != vm+t.NeueMenschen {
		return fmt.Errorf("Bereich %d Block %d: Menschen passen nicht", b.Bereich, b.Nr)
	}
	return nil
}
