package keeper

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// Unterschriebene Weiterleitung (Pruefungen von #319: MEDIUM-7, LOW-8,
// MEDIUM-11, INFO-14).
//
// WARUM. Ein Folger zaehlt die Erneuerungen, die ihn erreichen, je Absender
// (erneuerungsGrenze) und leitet sie dann an den Zustaendigen weiter. Zaehlte
// der Zustaendige sie unter der Adresse des Folgers, fuellte ein Angreifer
// mit vielen Adressen (IPv6!) diesen einen Zaehler und sperrte jeden
// ehrlichen Coordinator hinter dem Folger aus. Woran erkennt der Zustaendige,
// fuer wen eine Weiterleitung gilt? Die TCP-Adresse (Freiliste) reicht nicht:
// hinter einem Proxy, mit NAT oder einem Namen als URL steht der Folger nicht
// darin. Und der Kopf X-Aequitas-Weitergeleitet ist nur Text.
//
// WIE. Der Folger unterschreibt die Weiterleitung mit seinem Signierschluessel
// (derselbe wie fuer Bloecke und Leitungsnachrichten): Methode, Pfad, Zeit,
// die Adresse, unter der ER gezaehlt hat ("fuer"), die Adresse des
// Zustaendigen, eine Zufallszahl (zwei gleiche Anfragen in derselben
// Millisekunde sind sonst dieselbe Weiterleitung, und die zweite galte als
// Wiederholung -- Pruefung von #319, MEDIUM-18) und sha256 des Koerpers.
// Einen Koerper ueber weiterleitungKoerperMax unterschreibt er nicht
// (zumLeiter antwortet 413). Der Zustaendige prueft
//   - die Unterschrift passt zu einem Mitglied des Satzes seiner Leitung
//     (nicht er selbst, nicht ein bloss zugelassener Validator -- Pruefung
//     von #319, LOW-31),
//   - das Ziel ist er selbst (sonst gaelte ein mitgehoerter Nachweis bei
//     jedem anderen Knoten noch einmal),
//   - die Zeit liegt im Fenster der Leitungsnachrichten (30 s),
//   - er hat genau diese Weiterleitung noch nicht gesehen.
// Dann zaehlt er sie unter (Folger, fuer) -- mit derselben Grenze wie eine
// direkte Anfrage. Es gibt keinen gemeinsamen Vorrat, den ein Angreifer
// leeren koennte: wer ueber einen Folger kommt, verbraucht nur sein eigenes
// Budget. Alles andere -- auch ein gefaelschter Kopf -- zaehlt wie eine
// direkte Anfrage der TCP-Adresse.
//
// VERTRAUEN. Ein Mitglied des Satzes bestimmt "fuer" selbst und kann so
// beliebig viele Zaehler aufmachen -- dasselbe Vertrauen, das ihm die
// Freiliste schon gibt (docs/OFFEN.md, Ratenbegrenzung; Pruefung von #319,
// LOW-31). Darum nur der Satz, kein bloss zugelassener Validator.
//
// Die Merkliste schuetzt nur vor doppelter Arbeit und vor mitgehoerten
// Wiederholungen. Sie ist ein Ring fester Groesse; voll heisst: den aeltesten
// Eintrag verdraengen (eine Wiederholung zaehlte dann unter dem Absender,
// nicht frei).
//
// Nur fuer die Pfade in weiterleitungUnterschreiben: eine Unterschrift kostet
// Rechenzeit, und Ueberweisungen werden zu Tausenden je Sekunde
// weitergeleitet.

const (
	weiterleitungNachweisKopf = "X-Aequitas-Weiterleitung"
	// Groesser als jeder gueltige Koerper der Erneuerung (8 KiB, siehe
	// handleLivenessRenewal) -- mehr wird fuer die Pruefung nicht gelesen.
	weiterleitungKoerperMax = 8 << 10
	// Eine secp256k1-Unterschrift (65 Byte) in Hex.
	weiterleitungSigLaenge = 130
	// Die Zufallszahl: 16 Byte in Hex.
	weiterleitungNonceLaenge = 32
	// Laenger ist kein gueltiger Kopf (Zeit, IPv6, Zufallszahl,
	// Unterschrift): vor jedem Zerlegen abgewiesen (Pruefung von #319,
	// LOW-19).
	weiterleitungKopfMax = 256
)

// weiterleitungGemerktMax: so viele Weiterleitungen merkt sich ein Knoten
// gegen Wiederholung. Variable nur fuer Tests.
var weiterleitungGemerktMax = 20000

// weiterleitungPruefungen zaehlt die Unterschriftspruefungen (ecrecover),
// weiterleitungAbgelehnt die abgelehnten Nachweise je Grund -- fuer Tests
// und /api/health/combined: Uhrabweichung oder ein Folger, den die Leitung
// (noch) nicht kennt, sind sonst unsichtbar (Pruefung von #319, LOW-24).
var (
	weiterleitungPruefungen atomic.Int64
	weiterleitungAbgelehnt  [weiterleitungGruende]atomic.Int64
	weiterleitungAnerkannt  atomic.Int64
	weiterleitungGrundNamen = [weiterleitungGruende]string{"format", "zeit", "koerper", "unbekannt", "wiederholung"}
)

const (
	grundFormat = iota
	grundZeit
	grundKoerper
	grundUnbekannt
	grundWiederholung
	weiterleitungGruende
)

func weiterleitungAbweisen(grund int) string {
	weiterleitungAbgelehnt[grund].Add(1)
	return ""
}

// WeiterleitungNachweisStand fuer /api/health/combined.
func WeiterleitungNachweisStand() map[string]interface{} {
	abgelehnt := map[string]int64{}
	for i, name := range weiterleitungGrundNamen {
		abgelehnt[name] = weiterleitungAbgelehnt[i].Load()
	}
	return map[string]interface{}{
		"anerkannt": weiterleitungAnerkannt.Load(),
		"abgelehnt": abgelehnt,
		"bedeutung": "Unterschriebene Weiterleitungen von Erneuerungen beim Zustaendigen. " +
			"zeit: Uhren weichen mehr als 30 s ab; unbekannt: Folger nicht im Satz oder " +
			"zugelassen, falsches Ziel oder Faelschung; wiederholung: schon gesehen.",
	}
}

func weiterleitungUnterschreiben(pfad string) bool {
	return pfad == "/api/liveness-renewal"
}

func weiterleitungHash(methode, pfad string, zeitMs int64, fuer, ziel, nonce string, koerper []byte) []byte {
	h := sha256.Sum256(koerper)
	return crypto.Keccak256([]byte("aequitas-weiterleitung-v3:"), []byte(methode), []byte{0},
		[]byte(pfad), []byte{0}, []byte(strconv.FormatInt(zeitMs, 10)), []byte{0},
		[]byte(fuer), []byte{0}, []byte(ziel), []byte{0}, []byte(nonce), []byte{0}, h[:])
}

// weiterleitungNachweis: die Koepfe fuer die weitergeleitete Anfrage, oder
// nil (kein Schluessel, kein Ziel, anderer Pfad, Koerper zu gross, keine IP
// als Absender, kein Zufall) -- dann leitet zumLeiter nicht weiter
// (Pruefung von #319, INFO-26).
func weiterleitungNachweis(k *ecdsa.PrivateKey, r *http.Request, ziel string, koerper []byte, jetzt time.Time) http.Header {
	if k == nil || ziel == "" || !weiterleitungUnterschreiben(r.URL.Path) || len(koerper) > weiterleitungKoerperMax {
		return nil
	}
	ip := net.ParseIP(clientIP(r))
	if ip == nil {
		return nil
	}
	var zufall [weiterleitungNonceLaenge / 2]byte
	if _, err := rand.Read(zufall[:]); err != nil {
		return nil
	}
	fuer, zeit, nonce := ip.String(), jetzt.UnixMilli(), hex.EncodeToString(zufall[:])
	sig, err := crypto.Sign(weiterleitungHash(r.Method, r.URL.Path, zeit, fuer, ziel, nonce, koerper), k)
	if err != nil {
		return nil
	}
	h := http.Header{}
	h.Set(weiterleitungNachweisKopf, strconv.FormatInt(zeit, 10)+";"+fuer+";"+nonce+";"+hex.EncodeToString(sig))
	return h
}

// weiterleitungFuer: kommt r nachweislich von einem Validator, den die
// Leitung kennt, und fuer welchen Absender? Liefert den Schluessel, unter dem
// der Zustaendige zaehlt, oder "" -- dann zaehlt die Anfrage wie eine
// direkte. Liest dafuer den Koerper (hoechstens weiterleitungKoerperMax) und
// legt ihn fuer den naechsten Handler zurueck.
func (cs *ChainState) weiterleitungFuer(r *http.Request, jetzt time.Time) string {
	l := cs.leitung.Load()
	kopf := r.Header.Get(weiterleitungNachweisKopf)
	if l == nil || kopf == "" {
		return ""
	}
	if len(kopf) > weiterleitungKopfMax {
		return weiterleitungAbweisen(grundFormat)
	}
	teile := strings.SplitN(kopf, ";", 5)
	if len(teile) != 4 || len(teile[2]) != weiterleitungNonceLaenge || len(teile[3]) != weiterleitungSigLaenge {
		return weiterleitungAbweisen(grundFormat)
	}
	if _, err := hex.DecodeString(teile[2]); err != nil {
		return weiterleitungAbweisen(grundFormat)
	}
	zeit, err := strconv.ParseInt(teile[0], 10, 64)
	if err != nil {
		return weiterleitungAbweisen(grundFormat)
	}
	if d := jetzt.Sub(time.UnixMilli(zeit)); d > leitungZeitfenster || d < -leitungZeitfenster {
		return weiterleitungAbweisen(grundZeit)
	}
	ip := net.ParseIP(teile[1])
	if ip == nil || ip.String() != teile[1] {
		return weiterleitungAbweisen(grundFormat)
	}
	sig, err := hex.DecodeString(teile[3])
	if err != nil {
		return weiterleitungAbweisen(grundFormat)
	}
	koerper, err := io.ReadAll(io.LimitReader(r.Body, weiterleitungKoerperMax+1))
	r.Body = io.NopCloser(bytes.NewReader(koerper))
	if err != nil || len(koerper) > weiterleitungKoerperMax {
		return weiterleitungAbweisen(grundKoerper)
	}
	hash := weiterleitungHash(r.Method, r.URL.Path, zeit, teile[1], l.Ich(), teile[2], koerper)
	weiterleitungPruefungen.Add(1)
	pub, err := crypto.SigToPub(hash, sig)
	if err != nil {
		return weiterleitungAbweisen(grundUnbekannt)
	}
	folger := strings.ToLower(crypto.PubkeyToAddress(*pub).Hex())
	if !l.SatzMitglied(folger) {
		return weiterleitungAbweisen(grundUnbekannt)
	}
	// Der Hash, nicht die Unterschrift: eine Unterschrift laesst sich in eine
	// zweite, ebenso gueltige umformen (s und n-s).
	if !weiterleitungErstmals(hex.EncodeToString(hash), jetzt) {
		return weiterleitungAbweisen(grundWiederholung)
	}
	weiterleitungAnerkannt.Add(1)
	return folger + "|" + teile[1]
}

type weiterleitungEintrag struct {
	bis   time.Time
	platz int
}

var weiterleitungGemerkt struct {
	sync.Mutex
	eintraege map[string]weiterleitungEintrag
	ring      []string
	kopf      int
}

// weiterleitungErstmals bucht eine Weiterleitung; false, wenn sie schon
// gesehen wurde. Ist der Ring voll, verdraengt sie die aelteste (O(1)).
func weiterleitungErstmals(schluessel string, jetzt time.Time) bool {
	m := &weiterleitungGemerkt
	m.Lock()
	defer m.Unlock()
	if m.eintraege == nil {
		m.eintraege = map[string]weiterleitungEintrag{}
	}
	if e, schon := m.eintraege[schluessel]; schon && jetzt.Before(e.bis) {
		return false
	}
	platz := len(m.ring)
	if platz < weiterleitungGemerktMax {
		m.ring = append(m.ring, schluessel)
	} else {
		platz = m.kopf % len(m.ring)
		if alt := m.ring[platz]; m.eintraege[alt].platz == platz {
			delete(m.eintraege, alt)
		}
		m.ring[platz] = schluessel
		m.kopf = platz + 1
	}
	// Zwei Fenster: eine Nachricht, deren Zeit bis zu 30 s voraus liegt, ist
	// bis 30 s nach ihrer Zeit gueltig.
	m.eintraege[schluessel] = weiterleitungEintrag{bis: jetzt.Add(2 * leitungZeitfenster), platz: platz}
	return true
}
