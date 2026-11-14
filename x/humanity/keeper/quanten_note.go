package keeper

// Quantengeld, Stufe 6 (docs/QUANTENSICHERHEIT.md, forschung/quantengeld/):
// die KETTENSEITE der Bruecke AEQ <-> Quanten-Note -- Pruefregeln im Knoten.
//
// Byte fuer Byte wie die ausfuehrbare Spezifikation
// forschung/quantengeld/bruecke_kette.py (Pruefvektor in
// testdata/quanten_note_vektor.json, dort erzeugt):
//
//   Seriennummer  s = SHA-256(oeffentlicher Schluessel der Note), 32 Byte hex
//   Einloesen     Einmal-Signatur ueber
//                 "aequitas-quanten-einloesen-v1|chain:1926|<s>|<ziel>|<wei>"
//
// Die Note ist (spaeter) ein Quanten-Signierzustand -- eine Einmal-Signatur
// (one-shot signature): Einloesen heisst genau EINE Unterschrift, dabei wird
// der Zustand verbraucht. Die Kette prueft klassisch. Bis das Quanten-
// verfahren feststeht, steht hier eine Lamport-Einmal-Signatur (SHA-256) an
// ihrer Stelle -- dieselbe Pruefung, hashbasiert und selbst quantensicher.
// Einen Lamport-Schluessel kann man kopieren, einen Quanten-Zustand nicht;
// darum erzwingt die KETTE "hoechstens eine Einloesung je Seriennummer"
// selbst (quantenEinloesenUrteil).
//
// SCHLAFEND: quantenNoteAbUnix = math.MaxInt64. Die Transaktionsarten sind
// unbekannt (bekannteTxArt), ein Block mit ihnen ist ungueltig. Eingeschaltet
// wird erst mit dem echten Quanten-Einloeseverfahren UND dem Anschluss an den
// Geldweg (Annahme, Nachspielen, Nachrechnen, Kein-Entzug) in einem eigenen,
// eigens geprueften PR. TestQuantenNote_SchlafendSolangePlatzhalter haelt das
// fest: solange das Verfahren der Lamport-Platzhalter ist, darf der Stichtag
// nicht gesetzt sein.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
)

const (
	// quantenNoteAbUnix: AUS. Siehe Kopf.
	quantenNoteAbUnix int64 = math.MaxInt64

	// quantenNoteVerfahren: welches Einmal-Signaturverfahren die Kette
	// prueft. "lamport-platzhalter" darf nie eingeschaltet werden.
	quantenNoteVerfahren = "lamport-platzhalter"

	quantenDomaeneEinloesen = "aequitas-quanten-einloesen-v1"

	lamportBits     = 256
	lamportPubBytes = 2 * lamportBits * 32
	lamportSigBytes = lamportBits * 32

	// quantenRegisterMax: wie bruecke_kette.py.
	quantenRegisterMax = 1_000_000
)

func quantenNoteAktiv(t int64) bool { return t >= quantenNoteAbUnix }

// quantenSeriennummer: s = SHA-256(oeffentlicher Schluessel).
func quantenSeriennummer(oeffentlich []byte) string {
	h := sha256.Sum256(oeffentlich)
	return hex.EncodeToString(h[:])
}

// quantenEinloeseNachricht: was die Note beim Einloesen unterschreibt.
func quantenEinloeseNachricht(s, ziel string, wei *big.Int) []byte {
	return []byte(fmt.Sprintf("%s|chain:%d|%s|%s|%s", quantenDomaeneEinloesen, eingefroreneChainID, s, ziel, wei.String()))
}

// pruefeLamport: Lamport-Einmal-Signatur ueber SHA-256(nachricht), Bit i
// (hoechstwertiges Bit zuerst) waehlt das Urbild 2i+bit.
func pruefeLamport(oeffentlich, nachricht, sig []byte) bool {
	if len(oeffentlich) != lamportPubBytes || len(sig) != lamportSigBytes {
		return false
	}
	d := sha256.Sum256(nachricht)
	for i := 0; i < lamportBits; i++ {
		bit := int(d[i/8]>>(7-uint(i%8))) & 1
		erwartet := oeffentlich[(2*i+bit)*32 : (2*i+bit+1)*32]
		h := sha256.Sum256(sig[i*32 : (i+1)*32])
		if string(h[:]) != string(erwartet) {
			return false
		}
	}
	return true
}

// ---- Register ----

type quantenGesperrt struct {
	S           string `json:"s"`
	Wei         string `json:"wei"`
	Sperrender  string `json:"von"`
	Ablauf      int64  `json:"ablauf"`
	Rueckholbar bool   `json:"rueckholbar,omitempty"`
}

// quantenRegister: unveraenderlich; jede Aenderung baut ein neues. Kanonisch
// nach Seriennummer sortiert.
type quantenRegister struct {
	Gesperrt   []quantenGesperrt `json:"gesperrt,omitempty"`
	Verbraucht []string          `json:"verbraucht,omitempty"`
}

var errQuantenNote = errors.New("Quanten-Note abgelehnt")

func (r *quantenRegister) finde(s string) (quantenGesperrt, int, bool) {
	i := sort.Search(len(r.Gesperrt), func(i int) bool { return r.Gesperrt[i].S >= s })
	if i < len(r.Gesperrt) && r.Gesperrt[i].S == s {
		return r.Gesperrt[i], i, true
	}
	return quantenGesperrt{}, i, false
}

func (r *quantenRegister) verbraucht(s string) bool {
	i := sort.SearchStrings(r.Verbraucht, s)
	return i < len(r.Verbraucht) && r.Verbraucht[i] == s
}

func (r *quantenRegister) kopie() *quantenRegister {
	return &quantenRegister{
		Gesperrt:   append([]quantenGesperrt(nil), r.Gesperrt...),
		Verbraucht: append([]string(nil), r.Verbraucht...),
	}
}

func (r *quantenRegister) mitVerbraucht(s string) {
	i := sort.SearchStrings(r.Verbraucht, s)
	r.Verbraucht = append(r.Verbraucht, "")
	copy(r.Verbraucht[i+1:], r.Verbraucht[i:])
	r.Verbraucht[i] = s
}

// quantenSperren: Regel 2 der Spezifikation. guthaben in Wei. Gibt das neue
// Register zurueck; der Geldweg bucht wei vom Sperrenden ab (Anschluss beim
// Einschalten).
func (r *quantenRegister) quantenSperren(wer string, guthaben, wei *big.Int, oeffentlich []byte, frist, jetzt int64, rueckholbar bool) (*quantenRegister, string, error) {
	if wei == nil || wei.Sign() <= 0 || guthaben == nil || guthaben.Cmp(wei) < 0 {
		return nil, "", fmt.Errorf("%w: keine Deckung oder Betrag ungueltig", errQuantenNote)
	}
	if !kanonischeAdresse(wer) {
		return nil, "", fmt.Errorf("%w: Adresse nicht kanonisch", errQuantenNote)
	}
	if len(oeffentlich) != lamportPubBytes {
		return nil, "", fmt.Errorf("%w: Schluessel in falscher Form", errQuantenNote)
	}
	if frist <= 0 || jetzt > math.MaxInt64-frist {
		return nil, "", fmt.Errorf("%w: Frist ungueltig", errQuantenNote)
	}
	s := quantenSeriennummer(oeffentlich)
	if _, _, ok := r.finde(s); ok || r.verbraucht(s) {
		return nil, "", fmt.Errorf("%w: Seriennummer nicht neu", errQuantenNote)
	}
	if len(r.Gesperrt)+len(r.Verbraucht) >= quantenRegisterMax {
		return nil, "", fmt.Errorf("%w: Register voll", errQuantenNote)
	}
	k := r.kopie()
	_, i, _ := k.finde(s)
	k.Gesperrt = append(k.Gesperrt, quantenGesperrt{})
	copy(k.Gesperrt[i+1:], k.Gesperrt[i:])
	k.Gesperrt[i] = quantenGesperrt{S: s, Wei: wei.String(), Sperrender: wer, Ablauf: jetzt + frist, Rueckholbar: rueckholbar}
	return k, s, nil
}

// quantenEinloesen: Regeln 3 und 4. Gibt das neue Register und den Betrag
// zurueck, den der Geldweg dem Ziel gutschreibt.
func (r *quantenRegister) quantenEinloesen(s, ziel string, wei *big.Int, oeffentlich, sig []byte) (*quantenRegister, *big.Int, error) {
	if r.verbraucht(s) {
		return nil, nil, fmt.Errorf("%w: schon eingeloest oder zurueckgeholt", errQuantenNote)
	}
	e, i, ok := r.finde(s)
	if !ok {
		return nil, nil, fmt.Errorf("%w: unbekannte Seriennummer", errQuantenNote)
	}
	if !kanonischeAdresse(ziel) {
		return nil, nil, fmt.Errorf("%w: Ziel nicht kanonisch", errQuantenNote)
	}
	betrag, _ := new(big.Int).SetString(e.Wei, 10)
	if wei == nil || betrag == nil || wei.Cmp(betrag) != 0 {
		return nil, nil, fmt.Errorf("%w: Betrag passt nicht", errQuantenNote)
	}
	if quantenSeriennummer(oeffentlich) != s {
		return nil, nil, fmt.Errorf("%w: Schluessel gehoert nicht zur Seriennummer", errQuantenNote)
	}
	if !pruefeLamport(oeffentlich, quantenEinloeseNachricht(s, ziel, wei), sig) {
		return nil, nil, fmt.Errorf("%w: Einmal-Signatur ungueltig", errQuantenNote)
	}
	k := r.kopie()
	k.Gesperrt = append(k.Gesperrt[:i], k.Gesperrt[i+1:]...)
	k.mitVerbraucht(s)
	return k, betrag, nil
}

// quantenRueckholen: Regel 5.
func (r *quantenRegister) quantenRueckholen(wer, s string, jetzt int64) (*quantenRegister, *big.Int, error) {
	e, i, ok := r.finde(s)
	if !ok {
		return nil, nil, fmt.Errorf("%w: nicht gesperrt", errQuantenNote)
	}
	if wer != e.Sperrender || !e.Rueckholbar || jetzt < e.Ablauf {
		return nil, nil, fmt.Errorf("%w: Rueckholung nicht erlaubt", errQuantenNote)
	}
	betrag, _ := new(big.Int).SetString(e.Wei, 10)
	k := r.kopie()
	k.Gesperrt = append(k.Gesperrt[:i], k.Gesperrt[i+1:]...)
	k.mitVerbraucht(s)
	return k, betrag, nil
}

// summeGesperrt: fuer die Erhaltung (Konten + gesperrt = konstant).
func (r *quantenRegister) summeGesperrt() *big.Int {
	sum := new(big.Int)
	for _, e := range r.Gesperrt {
		if v, ok := new(big.Int).SetString(e.Wei, 10); ok {
			sum.Add(sum, v)
		}
	}
	return sum
}
