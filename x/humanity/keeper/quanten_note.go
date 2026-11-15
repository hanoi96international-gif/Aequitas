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

	// Quantum Lightning (Zhandry 2025, BAUVORGABE §8), Einweg-Bruecke --
	// wie forschung/quantengeld/labor.py, Pruefvektor
	// testdata/quanten_praegen_vektor.json.
	quantenDomaeneSerie  = "aequitas-quanten-serie-v1"
	quantenSerieMaxBytes = 1024
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
	// Lightning: angemeldete Lightning-Noten (Einweg, Inhaberpapier),
	// kanonisch nach Schluessel sortiert. Nie wieder entfernt.
	Lightning []quantenLightning `json:"lightning,omitempty"`
}

// quantenLightning: eine angemeldete Lightning-Seriennummer. K =
// SHA-256(Domaene | Seriennummer); die Seriennummer selbst steht nicht in
// der Kette (nur ihr Hash), der Inhaber zeigt sie mit der Note vor.
type quantenLightning struct {
	K   string `json:"k"`
	Wei string `json:"wei"`
	Von string `json:"von"`
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
		Lightning:  append([]quantenLightning(nil), r.Lightning...),
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
	if r.groesse() >= quantenRegisterMax {
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

func (r *quantenRegister) groesse() int {
	return len(r.Gesperrt) + len(r.Verbraucht) + len(r.Lightning)
}

// ---- Quantum Lightning: Praegen ist frei, Anmelden kostet Deckung ----
//
// Jeder darf eine Note praegen (Gen ist oeffentlich, kein Schluessel, keine
// Bank). Wert bekommt sie erst, wenn ihre Seriennummer hier angemeldet ist:
// der Anmeldende sperrt wei AEQ, die Kette traegt K = SHA-256(Domaene |
// Anmelder | Seriennummer) ein. Je Anmelder jede Seriennummer genau einmal;
// keine Rueckholung, kein Einloesen (Einweg, Version 1). Der Empfaenger
// einer Note prueft selbst: K angemeldet, h < Gruppenordnung? Dann der
// Quantentest (Variante B, 128 Tests). Eine zweite Anmeldung derselben Note
// unter einem anderen Anmelder schafft kein Geld: die Note ist EIN Zustand
// und wird mit genau einer Anmeldung weitergegeben; die andere Deckung
// bleibt gesperrt.
// Die Kette sieht nie einen Quantenzustand -- nur Hashes und Betraege.

// quantenSerieKanonisch: big-endian, kuerzeste Form (die Null ist 0x00).
// Sonst haette dieselbe Note mehrere Schreibweisen und liesse sich mehrfach
// anmelden.
func quantenSerieKanonisch(serie []byte) bool {
	return len(serie) > 0 && len(serie) <= quantenSerieMaxBytes && (len(serie) == 1 || serie[0] != 0)
}

// quantenSerieSchluessel: K = SHA-256(Domaene | "|" | Anmelder | "|" |
// Seriennummer). Der Anmelder (kanonische Adresse, feste Laenge) gehoert
// dazu: sonst koennte jeder, der eine Anmeldung im Mempool sieht, dieselbe
// Seriennummer mit 1 Wei zuerst anmelden und die echte blockieren.
func quantenSerieSchluessel(von string, serie []byte) string {
	h := sha256.New()
	h.Write([]byte(quantenDomaeneSerie))
	h.Write([]byte("|"))
	h.Write([]byte(von))
	h.Write([]byte("|"))
	h.Write(serie)
	return hex.EncodeToString(h.Sum(nil))
}

func (r *quantenRegister) findeLightning(k string) (quantenLightning, int, bool) {
	i := sort.Search(len(r.Lightning), func(i int) bool { return r.Lightning[i].K >= k })
	if i < len(r.Lightning) && r.Lightning[i].K == k {
		return r.Lightning[i], i, true
	}
	return quantenLightning{}, i, false
}

// quantenPraegen: meldet eine Lightning-Seriennummer an. guthaben in Wei;
// der Geldweg bucht wei vom Anmeldenden ab (Anschluss beim Einschalten).
func (r *quantenRegister) quantenPraegen(wer string, guthaben, wei *big.Int, serie []byte) (*quantenRegister, string, error) {
	if !quantenSerieKanonisch(serie) {
		return nil, "", fmt.Errorf("%w: Seriennummer in falscher Form", errQuantenNote)
	}
	if wei == nil || wei.Sign() <= 0 || guthaben == nil || guthaben.Cmp(wei) < 0 {
		return nil, "", fmt.Errorf("%w: keine Deckung oder Betrag ungueltig", errQuantenNote)
	}
	if !kanonischeAdresse(wer) {
		return nil, "", fmt.Errorf("%w: Adresse nicht kanonisch", errQuantenNote)
	}
	k := quantenSerieSchluessel(wer, serie)
	if _, _, ok := r.findeLightning(k); ok {
		return nil, "", fmt.Errorf("%w: Seriennummer schon angemeldet", errQuantenNote)
	}
	if r.groesse() >= quantenRegisterMax {
		return nil, "", fmt.Errorf("%w: Register voll", errQuantenNote)
	}
	n := r.kopie()
	_, i, _ := n.findeLightning(k)
	n.Lightning = append(n.Lightning, quantenLightning{})
	copy(n.Lightning[i+1:], n.Lightning[i:])
	n.Lightning[i] = quantenLightning{K: k, Wei: wei.String(), Von: wer}
	return n, k, nil
}

// quantenSerieWert: was eine vorgezeigte (Anmelder, Seriennummer) laut Kette wert ist;
// 0 = nicht angemeldet (die Note ist nichts wert, egal wie gut sie aussieht).
func (r *quantenRegister) quantenSerieWert(von string, serie []byte) *big.Int {
	if !quantenSerieKanonisch(serie) || !kanonischeAdresse(von) {
		return new(big.Int)
	}
	e, _, ok := r.findeLightning(quantenSerieSchluessel(von, serie))
	if !ok {
		return new(big.Int)
	}
	v, ok := new(big.Int).SetString(e.Wei, 10)
	if !ok {
		return new(big.Int)
	}
	return v
}

// summeGesperrt: fuer die Erhaltung (Konten + gesperrt = konstant).
func (r *quantenRegister) summeGesperrt() *big.Int {
	sum := new(big.Int)
	for _, e := range r.Gesperrt {
		if v, ok := new(big.Int).SetString(e.Wei, 10); ok {
			sum.Add(sum, v)
		}
	}
	for _, e := range r.Lightning {
		if v, ok := new(big.Int).SetString(e.Wei, 10); ok {
			sum.Add(sum, v)
		}
	}
	return sum
}
