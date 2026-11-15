package keeper

// OFFLINE-BARGELD -- Stufe 1a: Regeln und Register (docs/OFFLINE_BARGELD.md).
//
// Mit AEQ ohne Netz bezahlen, von Handy zu Handy. Die Sicherheit kommt aus
// drei Schichten:
//
//  1. Ein Geraeteschluessel (P-256, Android Keystore/StrongBox) verlaesst den
//     Chip nie -- jede Zahlung ist genau einem Geraet zuordenbar. Der Chip
//     VERHINDERT keine Doppelausgabe (Android hat keinen Hardware-Zaehler fuer
//     Apps); er macht sie beweisbar.
//  2. Zwei verschiedene Zahlungen mit demselben (Geraet, Zaehler) -- oder mehr
//     ausgegeben, als das Pfand deckt -- sind ein Beweis, den jeder Knoten
//     beim Einloesen selbst erkennt.
//  3. Ein Mensch, ein Konto: der Betrueger haftet mit Pfand, Konto und einer
//     Schuld; er kann kein zweites Konto aufmachen.
//
// DIESE DATEI enthaelt nur die Regeln (reine Funktionen ueber dem Register)
// und das Register selbst. Keine Transaktionsart ist eingehaengt; der
// Stichtag steht auf "nie" (offlineAbUnix). Die Einbindung in Annahme,
// Nachspielen, Snapshot und Rueckroll kommt als eigene Stufe mit eigener
// Sicherheitspruefung.
//
// BETRAEGE in Mikro-AEQ (int64), wie Decimal.Micro().

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
	"sync/atomic"
)

const (
	// offlineAbUnix: Stichtag (math.MaxInt64 = schlafend). Gesetzt wird er
	// erst nach Stufe 1b, eigener Sicherheitspruefung und Freigabe.
	offlineAbUnix int64 = math.MaxInt64

	offlinePfandMinMik   int64 = 1_000_000  // 1 AEQ
	offlinePfandMaxMik   int64 = 50_000_000 // 50 AEQ
	offlineZahlungMaxMik int64 = 20_000_000 // 20 AEQ

	// offlineZertifikatHoechstens: laengste Gueltigkeit eines Pfand-Zertifikats.
	offlineZertifikatHoechstens int64 = 72 * 3600
	// offlineEinloeseFrist: so lange nach dem Ende des laengsten Zertifikats
	// bleibt das Pfand fuer Einloesungen stehen.
	offlineEinloeseFrist int64 = 14 * 86400

	// Speichergrenzen (jede Liste im Register ist begrenzt).
	offlineZaehlerJePfand         = 1000 // eingeloeste Zaehler je Pfand
	offlineZaehlerMax       int64 = 1 << 31
	offlineArchivJeMensch         = 8   // abgeschlossene Pfaender je Mensch, die spaete Einloesungen noch pruefen
	offlineSchuldenJeMensch       = 256 // offene Forderungen gegen einen Menschen

	offlineDomaeneZahlung    = "aequitas-offline-zahlung-v1"
	offlineDomaeneZertifikat = "aequitas-offline-zertifikat-v1"
	offlineKonfigSchluessel  = "offline_bargeld"
	offlineGeraetHexLaenge   = 130 // 0x04 || X || Y, unkomprimiert
	offlineSignaturHexLaenge = 128 // r || s, je 32 Byte
)

// offlineTopfAddr: das Pfand liegt hier -- eine Adresse ohne Schluessel, wie
// der Quantengeld-Topf.
var offlineTopfAddr = func() string {
	h := sha256.Sum256([]byte("aequitas-offline-topf-v1"))
	return "0x" + hex.EncodeToString(h[12:])
}()

// offlineAbOverride: nur fuer Tests (0 = Konstante gilt).
var offlineAbOverride atomic.Int64

func offlineAb() int64 {
	if v := offlineAbOverride.Load(); v != 0 {
		return v
	}
	return offlineAbUnix
}

func offlineAktiv(t int64) bool { return t >= offlineAb() }

// ------------------------------------------------------------ Saetze

// offlineZahlungNachricht: was der Geraeteschluessel fuer eine Zahlung
// unterschreibt. Alle Felder klein, Betrag in Mikro-AEQ.
func offlineZahlungNachricht(geraet string, zaehler int64, empfaenger string, betragMik int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%d|%s|%d", offlineDomaeneZahlung, eingefroreneChainID,
		strings.ToLower(geraet), zaehler, strings.ToLower(empfaenger), betragMik)
}

// offlineZertifikatNachricht: was ein Erzeuger fuer ein Pfand-Zertifikat
// unterschreibt (Stufe 2; hier, damit der Satz an einer Stelle steht).
func offlineZertifikatNachricht(geraet, mensch string, pfandMik, ausgezahltMik, gueltigBis int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%d|%d", offlineDomaeneZertifikat, eingefroreneChainID,
		strings.ToLower(geraet), strings.ToLower(mensch), pfandMik, ausgezahltMik, gueltigBis)
}

// offlineGeraetLesen: oeffentlicher P-256-Schluessel, unkomprimiert, als
// 130 Hex-Zeichen klein; der Punkt muss auf der Kurve liegen.
func offlineGeraetLesen(h string) (*ecdsa.PublicKey, bool) {
	if len(h) != offlineGeraetHexLaenge || !kleinHex(h) || !strings.HasPrefix(h, "04") {
		return nil, false
	}
	b, err := hex.DecodeString(h)
	if err != nil {
		return nil, false
	}
	x := new(big.Int).SetBytes(b[1:33])
	y := new(big.Int).SetBytes(b[33:65])
	k := elliptic.P256()
	if !k.IsOnCurve(x, y) {
		return nil, false
	}
	return &ecdsa.PublicKey{Curve: k, X: x, Y: y}, true
}

// offlineSignaturPruefen: ECDSA P-256 ueber SHA-256 der Nachricht, r||s.
func offlineSignaturPruefen(geraet, nachricht, sigHex string) bool {
	pub, ok := offlineGeraetLesen(geraet)
	if !ok || len(sigHex) != offlineSignaturHexLaenge || !kleinHex(sigHex) {
		return false
	}
	b, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}
	r := new(big.Int).SetBytes(b[:32])
	s := new(big.Int).SetBytes(b[32:])
	if r.Sign() <= 0 || s.Sign() <= 0 {
		return false
	}
	d := sha256.Sum256([]byte(nachricht))
	return ecdsa.Verify(pub, d[:], r, s)
}

// ------------------------------------------------------------ Register

type offlineSchuld struct {
	Glaeubiger string `json:"glaeubiger"`
	Mikro      int64  `json:"mikro"`
}

type offlinePfand struct {
	Mensch          string `json:"mensch"`
	Geraet          string `json:"geraet"`
	PfandMikro      int64  `json:"pfand"`
	AusgezahltMikro int64  `json:"ausgezahlt"`
	Seit            int64  `json:"seit"`
	Gekuendigt      int64  `json:"gekuendigt,omitempty"`
	Abgeschlossen   int64  `json:"abgeschlossen,omitempty"`
	Betrug          bool   `json:"betrug,omitempty"`
	// Zaehler: eingeloester Zaehler -> SHA-256 der Zahlung (Hex).
	Zaehler map[int64]string `json:"zaehler,omitempty"`
}

func (p *offlinePfand) verfuegbar() int64 {
	if p.Abgeschlossen != 0 {
		return 0
	}
	if v := p.PfandMikro - p.AusgezahltMikro; v > 0 {
		return v
	}
	return 0
}

type offlineRegister struct {
	// Pfaender: Geraet -> Pfand (aktive, gekuendigte und archivierte).
	Pfaender map[string]*offlinePfand `json:"pfaender,omitempty"`
	// Aktiv: Mensch -> Geraet seines nicht abgeschlossenen Pfands.
	Aktiv map[string]string `json:"aktiv,omitempty"`
	// Gesperrt: Menschen, die nach einem Betrug nie wieder offline bezahlen.
	Gesperrt map[string]bool `json:"gesperrt,omitempty"`
	// Schulden: Schuldner -> offene Forderungen (aelteste zuerst).
	Schulden map[string][]offlineSchuld `json:"schulden,omitempty"`
}

func (r *offlineRegister) leer() bool {
	return len(r.Pfaender) == 0 && len(r.Aktiv) == 0 && len(r.Gesperrt) == 0 && len(r.Schulden) == 0
}

// kodiert: deterministisch (encoding/json sortiert Map-Schluessel).
func (r *offlineRegister) kodiert() string {
	if r.leer() {
		return ""
	}
	b, _ := json.Marshal(r)
	return string(b)
}

func (r *offlineRegister) kopie() *offlineRegister {
	n, err := offlineRegisterLesen(r.kodiert())
	if err != nil {
		// kodiert() stammt aus einem gueltigen Register; das kann nicht scheitern.
		panic("offlineRegister.kopie: " + err.Error())
	}
	return n
}

func (r *offlineRegister) stateRootTeil() string {
	if r.leer() {
		return ""
	}
	h := sha256.Sum256([]byte(r.kodiert()))
	return "|offline:" + hex.EncodeToString(h[:])
}

// offlineRegisterLesen: streng -- jede Form, die die Kette nie schreibt, ist
// ein Fehler (ein Register von aussen, etwa aus einem Snapshot, wird nicht
// geglaubt).
func offlineRegisterLesen(s string) (*offlineRegister, error) {
	r := &offlineRegister{}
	if s == "" {
		return r, nil
	}
	if err := json.Unmarshal([]byte(s), r); err != nil {
		return nil, fmt.Errorf("Offline-Register nicht lesbar: %w", err)
	}
	for g, p := range r.Pfaender {
		if p == nil || p.Geraet != g {
			return nil, fmt.Errorf("Offline-Register: Pfand %s nicht stimmig", kurzAdresse(g))
		}
		if _, ok := offlineGeraetLesen(g); !ok || !kanonischeAdresse(p.Mensch) {
			return nil, errors.New("Offline-Register: Geraet oder Mensch nicht gueltig")
		}
		if p.PfandMikro < offlinePfandMinMik || p.PfandMikro > offlinePfandMaxMik || p.AusgezahltMikro < 0 ||
			p.AusgezahltMikro > p.PfandMikro || len(p.Zaehler) > offlineZaehlerJePfand {
			return nil, errors.New("Offline-Register: Betraege oder Zaehler ausserhalb der Grenzen")
		}
		for n, d := range p.Zaehler {
			if n < 1 || n > offlineZaehlerMax || !istHash64(d) {
				return nil, errors.New("Offline-Register: Zaehler nicht gueltig")
			}
		}
	}
	for m, g := range r.Aktiv {
		p := r.Pfaender[g]
		if p == nil || p.Mensch != m || p.Abgeschlossen != 0 {
			return nil, errors.New("Offline-Register: aktives Pfand nicht stimmig")
		}
	}
	for m, ss := range r.Schulden {
		if !kanonischeAdresse(m) || len(ss) == 0 || len(ss) > offlineSchuldenJeMensch {
			return nil, errors.New("Offline-Register: Schulden nicht gueltig")
		}
		for _, x := range ss {
			if !kanonischeAdresse(x.Glaeubiger) || x.Mikro <= 0 {
				return nil, errors.New("Offline-Register: Forderung nicht gueltig")
			}
		}
	}
	return r, nil
}

func (r *offlineRegister) init() {
	if r.Pfaender == nil {
		r.Pfaender = map[string]*offlinePfand{}
	}
	if r.Aktiv == nil {
		r.Aktiv = map[string]string{}
	}
	if r.Gesperrt == nil {
		r.Gesperrt = map[string]bool{}
	}
	if r.Schulden == nil {
		r.Schulden = map[string][]offlineSchuld{}
	}
}

// archivBegrenzen: hoechstens offlineArchivJeMensch abgeschlossene Pfaender
// je Mensch bleiben (fuer spaete Einloesungen); die aeltesten fallen heraus.
func (r *offlineRegister) archivBegrenzen(mensch string) {
	var alte []*offlinePfand
	for _, p := range r.Pfaender {
		if p.Mensch == mensch && p.Abgeschlossen != 0 {
			alte = append(alte, p)
		}
	}
	if len(alte) <= offlineArchivJeMensch {
		return
	}
	sort.Slice(alte, func(i, j int) bool {
		if alte[i].Abgeschlossen != alte[j].Abgeschlossen {
			return alte[i].Abgeschlossen < alte[j].Abgeschlossen
		}
		return alte[i].Geraet < alte[j].Geraet
	})
	for _, p := range alte[:len(alte)-offlineArchivJeMensch] {
		delete(r.Pfaender, p.Geraet)
	}
}

// ------------------------------------------------------------ Regeln

// offlineKeinKonto: Adressen, die weder Pfand halten noch Offline-Zahlungen
// empfangen. istSystemAdresse kennt die beiden Toepfe (Offline, Quantengeld)
// nicht -- das dort zu aendern wirkt auf andere Wege der Kette und kommt mit
// Stufe 1b und eigener Pruefung; hier werden beide ausdruecklich genannt.
func offlineKeinKonto(a string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	return istSystemAdresse(a) || a == offlineTopfAddr || a == strings.ToLower(quantenTopfAddr)
}

// offlineAbgelehnt: eine Regel ist verletzt -- die Transaktion gilt nicht
// (nie ein Fehler beim Lesen, der als "gilt" durchginge).
type offlineAbgelehnt struct{ regel, text string }

func (e *offlineAbgelehnt) Error() string { return e.regel + ": " + e.text }

func offlineNein(regel, format string, args ...interface{}) error {
	return &offlineAbgelehnt{regel: regel, text: fmt.Sprintf(format, args...)}
}

// offlinePfandAnlegen: ein registrierter Mensch sperrt betragMik im Topf und
// bindet ihn an geraet. Die Ueberweisung an den Topf bucht der Aufrufer.
func offlinePfandAnlegen(reg *offlineRegister, mensch string, istMensch bool, geraet string, betragMik, blockZeit int64) (*offlineRegister, error) {
	mensch = strings.ToLower(mensch)
	if !offlineAktiv(blockZeit) {
		return nil, offlineNein("offline_inaktiv", "Offline-Bargeld ist noch nicht aktiv")
	}
	if !kanonischeAdresse(mensch) || offlineKeinKonto(mensch) || !istMensch {
		return nil, offlineNein("offline_mensch", "nur ein registrierter Mensch legt ein Pfand an")
	}
	if _, ok := offlineGeraetLesen(geraet); !ok {
		return nil, offlineNein("offline_form", "Geraeteschluessel nicht gueltig (P-256, unkomprimiert)")
	}
	if betragMik < offlinePfandMinMik || betragMik > offlinePfandMaxMik {
		return nil, offlineNein("offline_grenze", "Pfand muss zwischen %d und %d Mikro-AEQ liegen", offlinePfandMinMik, offlinePfandMaxMik)
	}
	if reg.Gesperrt[mensch] {
		return nil, offlineNein("offline_gesperrt", "nach einem Betrug kein Offline-Bezahlen mehr")
	}
	if len(reg.Schulden[mensch]) > 0 {
		return nil, offlineNein("offline_schuld", "erst offene Forderungen begleichen")
	}
	if _, da := reg.Aktiv[mensch]; da {
		return nil, offlineNein("offline_doppelt", "es gibt schon ein Pfand")
	}
	if _, da := reg.Pfaender[geraet]; da {
		return nil, offlineNein("offline_doppelt", "dieser Geraeteschluessel war schon an ein Pfand gebunden")
	}
	n := reg.kopie()
	n.init()
	n.Pfaender[geraet] = &offlinePfand{Mensch: mensch, Geraet: geraet, PfandMikro: betragMik, Seit: blockZeit}
	n.Aktiv[mensch] = geraet
	return n, nil
}

// offlineEinloesung: was eine Einloesung bewirkt. Der Aufrufer bucht:
// AusTopf vom Topf an Empfaenger; VomKonto (hoechstens so viel, wie der
// Schuldner hat) vom Schuldner an Empfaenger; den Rest traegt
// offlineSchuldEintragen als Forderung ein.
type offlineEinloesung struct {
	Mensch     string // Schuldner (Halter des Pfands)
	Empfaenger string
	AusTopf    int64
	Rest       int64 // nicht vom Pfand gedeckt: vom Konto, sonst Schuld
	Betrug     bool
}

// offlineEinloesen: eine Zahlung von geraet an empfaenger einloesen.
// Dieselbe Pruefung fuer Annahme und Nachspielen.
func offlineEinloesen(reg *offlineRegister, geraet string, zaehler int64, empfaenger string, betragMik int64, sigHex string, blockZeit int64) (*offlineRegister, offlineEinloesung, error) {
	empfaenger = strings.ToLower(empfaenger)
	if !offlineAktiv(blockZeit) {
		return nil, offlineEinloesung{}, offlineNein("offline_inaktiv", "Offline-Bargeld ist noch nicht aktiv")
	}
	if !kanonischeAdresse(empfaenger) || offlineKeinKonto(empfaenger) {
		return nil, offlineEinloesung{}, offlineNein("offline_form", "Empfaenger nicht gueltig")
	}
	if zaehler < 1 || zaehler > offlineZaehlerMax || betragMik <= 0 || betragMik > offlineZahlungMaxMik {
		return nil, offlineEinloesung{}, offlineNein("offline_grenze", "Zaehler oder Betrag ausserhalb der Grenzen")
	}
	p := reg.Pfaender[geraet]
	if p == nil {
		return nil, offlineEinloesung{}, offlineNein("offline_unbekannt", "kein Pfand fuer dieses Geraet")
	}
	if empfaenger == p.Mensch {
		return nil, offlineEinloesung{}, offlineNein("offline_form", "an sich selbst zahlt man nicht offline")
	}
	nachricht := offlineZahlungNachricht(geraet, zaehler, empfaenger, betragMik)
	if !offlineSignaturPruefen(geraet, nachricht, sigHex) {
		return nil, offlineEinloesung{}, offlineNein("offline_unterschrift", "Unterschrift des Geraets gilt nicht fuer diese Zahlung")
	}
	d := sha256.Sum256([]byte(nachricht))
	digest := hex.EncodeToString(d[:])
	betrug := false
	if alt, da := p.Zaehler[zaehler]; da {
		if alt == digest {
			return nil, offlineEinloesung{}, offlineNein("offline_doppelt", "diese Zahlung ist schon eingeloest")
		}
		// Zwei verschiedene Zahlungen mit demselben Zaehler: nur der Chip von
		// geraet kann beide unterschrieben haben -- der Beweis.
		betrug = true
	} else if len(p.Zaehler) >= offlineZaehlerJePfand {
		return nil, offlineEinloesung{}, offlineNein("offline_voll", "dieses Pfand hat die Hoechstzahl an Einloesungen erreicht")
	}

	n := reg.kopie()
	n.init()
	q := n.Pfaender[geraet]
	if !betrug {
		if q.Zaehler == nil {
			q.Zaehler = map[int64]string{}
		}
		q.Zaehler[zaehler] = digest
	}
	aus := betragMik
	if v := q.verfuegbar(); aus > v {
		aus = v
	}
	rest := betragMik - aus
	// Mehr ausgegeben, als das Pfand deckt, ist ebenfalls Betrug (die App des
	// Zahlenden laesst das nie zu) -- ausser das Pfand ist schon
	// abgeschlossen: dann kam der Empfaenger nur zu spaet.
	if rest > 0 && q.Abgeschlossen == 0 {
		betrug = true
	}
	q.AusgezahltMikro += aus
	if betrug {
		q.Betrug = true
		n.Gesperrt[q.Mensch] = true
	}
	return n, offlineEinloesung{Mensch: q.Mensch, Empfaenger: empfaenger, AusTopf: aus, Rest: rest, Betrug: betrug}, nil
}

// offlineSchuldEintragen: was weder Pfand noch Konto decken, wird eine
// Forderung des Empfaengers gegen den Schuldner. Die Liste ist begrenzt; ist
// sie voll, wird die neue Forderung an die letzte angehaengt (kein Betrag
// geht verloren, nur die Zahl der Eintraege bleibt fest).
func offlineSchuldEintragen(reg *offlineRegister, schuldner, glaeubiger string, mikro int64) *offlineRegister {
	if mikro <= 0 {
		return reg
	}
	n := reg.kopie()
	n.init()
	ss := n.Schulden[schuldner]
	for i := range ss {
		if ss[i].Glaeubiger == glaeubiger {
			ss[i].Mikro += mikro
			n.Schulden[schuldner] = ss
			return n
		}
	}
	if len(ss) >= offlineSchuldenJeMensch {
		ss[len(ss)-1].Mikro += mikro // Grenze: zusammenfassen statt wachsen
		n.Schulden[schuldner] = ss
		return n
	}
	n.Schulden[schuldner] = append(ss, offlineSchuld{Glaeubiger: glaeubiger, Mikro: mikro})
	return n
}

// offlineKuendigen: der Halter kuendigt sein Pfand. Danach nimmt die App
// kein neues Zertifikat mehr an; Einloesungen gehen weiter bis zum Abschluss.
func offlineKuendigen(reg *offlineRegister, mensch string, blockZeit int64) (*offlineRegister, error) {
	mensch = strings.ToLower(mensch)
	if !offlineAktiv(blockZeit) {
		return nil, offlineNein("offline_inaktiv", "Offline-Bargeld ist noch nicht aktiv")
	}
	g, da := reg.Aktiv[mensch]
	if !da {
		return nil, offlineNein("offline_unbekannt", "kein Pfand")
	}
	if reg.Pfaender[g].Gekuendigt != 0 {
		return nil, offlineNein("offline_doppelt", "schon gekuendigt")
	}
	n := reg.kopie()
	n.Pfaender[g].Gekuendigt = blockZeit
	return n, nil
}

// offlineAbschlussAb: ab wann ein gekuendigtes Pfand abgeschlossen werden
// darf -- das laengste Zertifikat plus die Einloesefrist.
func offlineAbschlussAb(gekuendigt int64) int64 {
	return gekuendigt + offlineZertifikatHoechstens + offlineEinloeseFrist
}

// offlineAbschluss: was beim Abschluss an wen geht. Der Rest des Pfands
// begleicht zuerst offene Forderungen gegen den Halter (aelteste zuerst),
// dann geht er an den Halter.
type offlineAuszahlung struct {
	An    string
	Mikro int64
}

func offlineAbschliessen(reg *offlineRegister, mensch string, blockZeit int64) (*offlineRegister, []offlineAuszahlung, error) {
	mensch = strings.ToLower(mensch)
	if !offlineAktiv(blockZeit) {
		return nil, nil, offlineNein("offline_inaktiv", "Offline-Bargeld ist noch nicht aktiv")
	}
	g, da := reg.Aktiv[mensch]
	if !da {
		return nil, nil, offlineNein("offline_unbekannt", "kein Pfand")
	}
	p := reg.Pfaender[g]
	if p.Gekuendigt == 0 {
		return nil, nil, offlineNein("offline_frist", "erst kuendigen")
	}
	if blockZeit < offlineAbschlussAb(p.Gekuendigt) {
		return nil, nil, offlineNein("offline_frist", "Abschluss erst ab %d", offlineAbschlussAb(p.Gekuendigt))
	}
	n := reg.kopie()
	n.init()
	q := n.Pfaender[g]
	rest := q.verfuegbar()
	var aus []offlineAuszahlung
	ss := n.Schulden[mensch]
	for len(ss) > 0 && rest > 0 {
		zahl := ss[0].Mikro
		if zahl > rest {
			zahl = rest
		}
		aus = append(aus, offlineAuszahlung{An: ss[0].Glaeubiger, Mikro: zahl})
		rest -= zahl
		ss[0].Mikro -= zahl
		if ss[0].Mikro == 0 {
			ss = ss[1:]
		}
	}
	if len(ss) == 0 {
		delete(n.Schulden, mensch)
	} else {
		n.Schulden[mensch] = ss
	}
	if rest > 0 {
		aus = append(aus, offlineAuszahlung{An: mensch, Mikro: rest})
	}
	q.AusgezahltMikro = q.PfandMikro
	q.Abgeschlossen = blockZeit
	delete(n.Aktiv, mensch)
	n.archivBegrenzen(mensch)
	return n, aus, nil
}
