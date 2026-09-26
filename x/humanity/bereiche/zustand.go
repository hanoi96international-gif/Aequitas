package bereiche

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Betraege in Mikro-AEQ (ganzzahlig, wie Decimal im Knoten).
const (
	Mikro        int64 = 1_000_000
	Grundbetrag        = 1_000 * Mikro // Registrierung: 1.000 AEQ
	gebuehrBps         = 10            // 0,1 % auf jede Ueberweisung, ganz ans Grundeinkommen
	grenzeFaktor       = 25            // Vermoegensgrenze = Durchschnitt x 25
)

// Konto eines Bereichs.
type Konto struct {
	Guthaben int64
	Mensch   bool
	Nonce    uint64
	// GEStand: Stand des kumulierten Grundeinkommens (Sammelblock.GEKumuliert),
	// bis zu dem dieses Konto ausgezahlt ist. Grundeinkommen wird so je Konto
	// in O(1) gutgeschrieben, wenn es handelt -- kein Durchlauf ueber alle
	// Menschen (loest nebenbei UBI_DISTRIBUTION_DESIGN.md's O(N)).
	GEStand int64
}

func (k Konto) leer() bool { return k == Konto{} }

func (k Konto) hash(addr string) Hash {
	if k.leer() {
		return Hash{}
	}
	var b []byte
	b = append(b, "konto|"...)
	b = append(b, addr...)
	b = binary.BigEndian.AppendUint64(b, uint64(k.Guthaben))
	if k.Mensch {
		b = append(b, 1)
	} else {
		b = append(b, 0)
	}
	b = binary.BigEndian.AppendUint64(b, k.Nonce)
	b = binary.BigEndian.AppendUint64(b, uint64(k.GEStand))
	return sha256.Sum256(b)
}

func kontoSchluessel(addr string) Hash { return sha256.Sum256([]byte("konto:" + addr)) }

func quittungSchluessel(id Hash) Hash { return sha256.Sum256(append([]byte("quittung:"), id[:]...)) }

var verbrauchtWert = sha256.Sum256([]byte("verbraucht"))

// Bereich eines Kontos.
func BereichVon(addr string, s int) int {
	h := sha256.Sum256([]byte("bereich:" + strings.ToLower(addr)))
	return int(binary.BigEndian.Uint64(h[:8]) % uint64(s))
}

// Zustand: was die Zustandsuebergangsfunktion liest und schreibt. Zwei
// Formen: der volle Zustand eines Bereichs und der Beweiszustand eines
// Fehlerbeweises, der nur die bewiesenen Konten kennt.
type Zustand interface {
	Konto(addr string) (Konto, error)
	SetzeKonto(addr string, k Konto)
	Verbraucht(id Hash) (bool, error)
	Verbrauche(id Hash)
	Wurzel() Hash
}

// VollZustand: alle Konten eines Bereichs.
type VollZustand struct {
	baum   *Baum
	konten map[string]Konto
}

func NeuerVollZustand() *VollZustand {
	return &VollZustand{baum: NeuerBaum(), konten: map[string]Konto{}}
}

func (z *VollZustand) Konto(addr string) (Konto, error) { return z.konten[addr], nil }
func (z *VollZustand) SetzeKonto(addr string, k Konto) {
	if k.leer() {
		delete(z.konten, addr)
	} else {
		z.konten[addr] = k
	}
	z.baum.Setze(kontoSchluessel(addr), k.hash(addr))
}
func (z *VollZustand) Verbraucht(id Hash) (bool, error) {
	return z.baum.Wert(quittungSchluessel(id)) == verbrauchtWert, nil
}
func (z *VollZustand) Verbrauche(id Hash) { z.baum.Setze(quittungSchluessel(id), verbrauchtWert) }
func (z *VollZustand) Wurzel() Hash       { return z.baum.Wurzel() }

// Kopie: fuer Ruecksetzen nach einem Fehlerbeweis und fuer Pruefer.
func (z *VollZustand) Kopie() *VollZustand {
	n := NeuerVollZustand()
	for k, v := range z.baum.werte {
		n.baum.werte[k] = v
	}
	for k, v := range z.baum.innen {
		n.baum.innen[k] = v
	}
	for k, v := range z.konten {
		n.konten[k] = v
	}
	return n
}

// Geldmenge: Summe aller Guthaben (fuer Tests und Teilsummen).
func (z *VollZustand) Geldmenge() (summe int64, menschen int) {
	for _, k := range z.konten {
		summe += k.Guthaben
		if k.Mensch {
			menschen++
		}
	}
	return
}

// Beweisfuer: Konto-/Quittungsdaten mit Merkle-Beweisen fuer einen
// Fehlerbeweis.
func (z *VollZustand) Beweisfuer(konten []string, quittungen []Hash) Vorzustand {
	v := Vorzustand{Wurzel: z.Wurzel()}
	for _, a := range konten {
		v.Konten = append(v.Konten, BewiesenesKonto{Addr: a, Konto: z.konten[a], Beweis: z.baum.Beweise(kontoSchluessel(a))})
	}
	for _, q := range quittungen {
		k := quittungSchluessel(q)
		v.Quittungen = append(v.Quittungen, BewieseneQuittung{ID: q, Verbraucht: z.baum.Wert(k) == verbrauchtWert, Beweis: z.baum.Beweise(k)})
	}
	return v
}

// ---- Transaktionen -----------------------------------------------------------

// Ueberweisung, vom Absender signiert (ed25519; Adresse = hex(oeffentlicher Schluessel)).
type Ueberweisung struct {
	Von, An  string
	Betrag   int64
	Nonce    uint64
	Ausstieg bool // Umtausch/Ausstieg aus dem System: An ist dann leer
	Sig      []byte
}

func (u Ueberweisung) signiertBytes() []byte {
	var b []byte
	b = append(b, "aequitas-bereich-ueberweisung|"...)
	b = append(b, u.Von...)
	b = append(b, '|')
	b = append(b, u.An...)
	b = binary.BigEndian.AppendUint64(b, uint64(u.Betrag))
	b = binary.BigEndian.AppendUint64(b, u.Nonce)
	if u.Ausstieg {
		b = append(b, 1)
	}
	return b
}

// Signiere mit dem privaten Schluessel des Absenders.
func (u *Ueberweisung) Signiere(priv ed25519.PrivateKey) {
	u.Sig = ed25519.Sign(priv, u.signiertBytes())
}

func (u Ueberweisung) signaturGueltig() bool {
	pub, err := hex.DecodeString(u.Von)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pub, u.signiertBytes(), u.Sig)
}

// Registrierung eines Menschen (der Beweis der Einmaligkeit liegt im
// Knoten, nicht hier): schafft Grundbetrag.
type Registrierung struct{ Addr string }

// Quittung: eine Ueberweisung in einen anderen Bereich. Belastet wird im
// Quellbereich, gutgeschrieben im Zielbereich -- genau einmal.
type Quittung struct {
	ID          Hash
	VonBereich  int
	NachBereich int
	An          string
	Betrag      int64
}

// Eingaben eines Bereichsblocks, in dieser Reihenfolge angewandt.
type Eingaben struct {
	Eingang        []Quittung
	Registrierung  []Registrierung
	Ueberweisungen []Ueberweisung
}

// Rahmen: was aus dem Sammelblock der Vorepoche kommt.
type Rahmen struct {
	S           int   // Zahl der Bereiche
	GEKumuliert int64 // Grundeinkommen je Mensch, seit Beginn aufsummiert
	Grenze      int64 // Vermoegensgrenze (0 = keine)
}

// Ergebnis der Ausfuehrung.
type Ergebnis struct {
	Wurzel     Hash
	Ausgang    []Quittung
	Teilsumme  Teilsumme
	Angewandt  []bool // je Ueberweisung: angewandt oder uebersprungen
	Beruehrt   []string
	Quittungen []Hash // beruehrte Quittungsschluessel (Eingang)
}

// Teilsumme: was ein Bereich je Block dem Sammelblock meldet.
type Teilsumme struct {
	Eingang      int64 // gutgeschriebene Quittungen
	Ausgang      int64 // ausgestellte Quittungen
	Gebuehren    int64 // ans Grundeinkommen (Gebuehren + Kappungen)
	GEAusgezahlt int64 // ausgezahltes Grundeinkommen
	Geschoepft   int64 // Registrierungen
	Ausstiege    int64 // aus dem System ausgetreten
	NeueMenschen int
}

var ErrFehltImBeweis = errors.New("Konto fehlt im Beweis")

// Ausfuehren: die Zustandsuebergangsfunktion eines Bereichsblocks. Rein und
// deterministisch; ungueltige Ueberweisungen werden uebersprungen, nicht
// abgelehnt -- genau wie im Knoten (zustand_ablehnung.go). Fehler nur, wenn
// der Zustand unvollstaendig ist (Beweiszustand) oder die Eingaben
// widerspruechlich sind.
func Ausfuehren(z Zustand, bereich int, r Rahmen, e Eingaben, blockID Hash) (Ergebnis, error) {
	var erg Ergebnis
	beruehrt := map[string]bool{}
	nimm := func(a string) (Konto, error) {
		beruehrt[a] = true
		return z.Konto(a)
	}
	// Grundeinkommen nachtragen, bevor ein Mensch etwas tut.
	geNachtrag := func(a string, k Konto) Konto {
		if k.Mensch && k.GEStand < r.GEKumuliert {
			d := r.GEKumuliert - k.GEStand
			k.Guthaben += d
			k.GEStand = r.GEKumuliert
			erg.Teilsumme.GEAusgezahlt += d
		}
		return k
	}
	// Kappung ueber der Grenze: der Ueberschuss geht ans Grundeinkommen.
	kappe := func(k Konto) Konto {
		if r.Grenze > 0 && k.Mensch && k.Guthaben > r.Grenze {
			erg.Teilsumme.Gebuehren += k.Guthaben - r.Grenze
			k.Guthaben = r.Grenze
		}
		return k
	}

	for _, q := range e.Eingang {
		if q.NachBereich != bereich || BereichVon(q.An, r.S) != bereich || q.Betrag <= 0 {
			return erg, fmt.Errorf("Quittung %x gehoert nicht in Bereich %d", q.ID[:4], bereich)
		}
		v, err := z.Verbraucht(q.ID)
		if err != nil {
			return erg, err
		}
		erg.Quittungen = append(erg.Quittungen, q.ID)
		if v {
			return erg, fmt.Errorf("Quittung %x schon eingeloest", q.ID[:4])
		}
		z.Verbrauche(q.ID)
		k, err := nimm(q.An)
		if err != nil {
			return erg, err
		}
		k = geNachtrag(q.An, k)
		k.Guthaben += q.Betrag
		z.SetzeKonto(q.An, kappe(k))
		erg.Teilsumme.Eingang += q.Betrag
	}
	for _, reg := range e.Registrierung {
		if BereichVon(reg.Addr, r.S) != bereich {
			return erg, fmt.Errorf("Registrierung %s gehoert nicht in Bereich %d", reg.Addr, bereich)
		}
		k, err := nimm(reg.Addr)
		if err != nil {
			return erg, err
		}
		if k.Mensch {
			continue // schon registriert: uebersprungen
		}
		k.Mensch = true
		k.GEStand = r.GEKumuliert
		k.Guthaben += Grundbetrag
		z.SetzeKonto(reg.Addr, k)
		erg.Teilsumme.Geschoepft += Grundbetrag
		erg.Teilsumme.NeueMenschen++
	}
	for i, u := range e.Ueberweisungen {
		ok, err := ueberweisen(z, bereich, r, u, i, blockID, nimm, geNachtrag, kappe, &erg)
		if err != nil {
			return erg, err
		}
		erg.Angewandt = append(erg.Angewandt, ok)
	}
	erg.Wurzel = z.Wurzel()
	for a := range beruehrt {
		erg.Beruehrt = append(erg.Beruehrt, a)
	}
	sort.Strings(erg.Beruehrt)
	return erg, nil
}

func ueberweisen(z Zustand, bereich int, r Rahmen, u Ueberweisung, i int, blockID Hash,
	nimm func(string) (Konto, error), geNachtrag func(string, Konto) Konto, kappe func(Konto) Konto, erg *Ergebnis) (bool, error) {
	if BereichVon(u.Von, r.S) != bereich {
		return false, fmt.Errorf("Ueberweisung von %s gehoert nicht in Bereich %d", u.Von, bereich)
	}
	if u.Betrag <= 0 || u.Von == u.An || (!u.Ausstieg && u.An == "") || !u.signaturGueltig() {
		return false, nil
	}
	von, err := nimm(u.Von)
	if err != nil {
		return false, err
	}
	von = geNachtrag(u.Von, von)
	gebuehr := u.Betrag * gebuehrBps / 10_000
	if u.Nonce != von.Nonce || von.Guthaben < u.Betrag+gebuehr {
		// Nachtrag des Grundeinkommens bleibt: er haengt nicht an der
		// Ueberweisung, und ihn zu verwerfen haette dieselbe Wirkung auf die
		// Teilsumme wie ihn zu behalten -- behalten ist einfacher zu pruefen.
		z.SetzeKonto(u.Von, von)
		return false, nil
	}
	von.Guthaben -= u.Betrag + gebuehr
	von.Nonce++
	z.SetzeKonto(u.Von, von)
	erg.Teilsumme.Gebuehren += gebuehr
	switch {
	case u.Ausstieg:
		erg.Teilsumme.Ausstiege += u.Betrag
	case BereichVon(u.An, r.S) == bereich:
		an, err := nimm(u.An)
		if err != nil {
			return false, err
		}
		an = geNachtrag(u.An, an)
		an.Guthaben += u.Betrag
		z.SetzeKonto(u.An, kappe(an))
	default:
		var b []byte
		b = append(b, blockID[:]...)
		b = binary.BigEndian.AppendUint64(b, uint64(i))
		q := Quittung{ID: sha256.Sum256(b), VonBereich: bereich, NachBereich: BereichVon(u.An, r.S), An: u.An, Betrag: u.Betrag}
		erg.Ausgang = append(erg.Ausgang, q)
		erg.Teilsumme.Ausgang += u.Betrag
	}
	return true, nil
}
