package keeper

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// Unterschriebene Weiterleitung (Pruefung von #319, MEDIUM-7 und LOW-8).
//
// WARUM. Ein Folger zaehlt die Erneuerungen, die ihn erreichen, je Absender
// (erneuerungsGrenze) und leitet sie dann an den Zustaendigen weiter. Der
// darf sie nicht noch einmal unter der Adresse des Folgers zaehlen -- sonst
// fuellt ein Angreifer mit vielen Adressen (IPv6!) diesen einen Zaehler und
// sperrt jeden ehrlichen Coordinator hinter dem Folger aus. Woran erkennt der
// Zustaendige eine echte Weiterleitung? Die TCP-Adresse (Freiliste) reicht
// nicht: hinter einem Proxy, mit NAT oder einem Namen als URL steht der
// Folger nicht darin. Und der Kopf X-Aequitas-Weitergeleitet ist nur Text.
//
// WIE. Der Folger unterschreibt die Weiterleitung mit seinem Signierschluessel
// (derselbe wie fuer die Leitungsnachrichten): Methode, Pfad, Zeit und
// sha256 des Koerpers. Der Zustaendige nimmt sie nur von der Grenze aus, wenn
//   - die Unterschrift zu einem Validator passt, den seine Leitung kennt
//     (Mitglied des Satzes oder zugelassen, nicht er selbst),
//   - die Zeit im Fenster der Leitungsnachrichten liegt (30 s),
//   - er genau diese Weiterleitung noch nicht gesehen hat (jede nur einmal;
//     die Merkliste ist begrenzt, voll = nicht ausgenommen).
// Alles andere -- auch ein gefaelschter Kopf -- zaehlt wie eine direkte
// Anfrage dieser Adresse.
//
// Nur fuer die Pfade in weiterleitungUnterschreiben: eine Unterschrift kostet
// Rechenzeit, und Ueberweisungen werden zu Tausenden je Sekunde
// weitergeleitet.

const (
	weiterleitungNachweisKopf = "X-Aequitas-Weiterleitung"
	// Hoechstens so viele Weiterleitungen merkt sich ein Knoten gegen
	// Wiederholung (je eine Minute lang). Weit ueber dem, was Erneuerungen je
	// Minute brauchen; voll heisst: zaehlen statt ausnehmen.
	weiterleitungGemerktMax = 20000
	// Groesser als jeder gueltige Koerper der Erneuerung (8 KiB, siehe
	// handleLivenessRenewal) -- mehr wird fuer die Pruefung nicht gelesen.
	weiterleitungKoerperMax = 8 << 10
)

// weiterleitungsSchluessel: der Signierschluessel dieses Knotens, gesetzt von
// StarteLeitung. nil = keine Unterschrift (der Zustaendige zaehlt dann).
var weiterleitungsSchluessel atomic.Pointer[ecdsa.PrivateKey]

func weiterleitungUnterschreiben(pfad string) bool {
	return pfad == "/api/liveness-renewal"
}

func weiterleitungHash(methode, pfad string, zeitMs int64, koerper []byte) []byte {
	h := sha256.Sum256(koerper)
	return crypto.Keccak256([]byte("aequitas-weiterleitung:"), []byte(methode), []byte{0},
		[]byte(pfad), []byte{0}, []byte(strconv.FormatInt(zeitMs, 10)), []byte{0}, h[:])
}

// weiterleitungNachweisSetzen: in leiteWeiter, fuer die weitergeleitete
// Anfrage. Ohne Schluessel oder fuer andere Pfade nichts.
func weiterleitungNachweisSetzen(req *http.Request, pfad string, koerper []byte, jetzt time.Time) {
	k := weiterleitungsSchluessel.Load()
	if k == nil || !weiterleitungUnterschreiben(pfad) {
		return
	}
	zeit := jetzt.UnixMilli()
	sig, err := crypto.Sign(weiterleitungHash(req.Method, pfad, zeit, koerper), k)
	if err != nil {
		return
	}
	req.Header.Set(weiterleitungNachweisKopf, strconv.FormatInt(zeit, 10)+":"+hex.EncodeToString(sig))
}

// weiterleitungNachgewiesen: kommt r nachweislich von einem Validator, den
// die Leitung kennt? Liest dafuer den Koerper (hoechstens
// weiterleitungKoerperMax) und legt ihn fuer den naechsten Handler zurueck.
// Jeder Fehler heisst false -- dann zaehlt die Anfrage wie eine direkte.
func (cs *ChainState) weiterleitungNachgewiesen(r *http.Request, jetzt time.Time) bool {
	l := cs.leitung.Load()
	kopf := r.Header.Get(weiterleitungNachweisKopf)
	if l == nil || kopf == "" {
		return false
	}
	zeitText, sigText, ok := strings.Cut(kopf, ":")
	zeit, err := strconv.ParseInt(zeitText, 10, 64)
	if !ok || err != nil {
		return false
	}
	if d := jetzt.Sub(time.UnixMilli(zeit)); d > leitungZeitfenster || d < -leitungZeitfenster {
		return false
	}
	sig, err := hex.DecodeString(sigText)
	if err != nil || len(sig) != 65 {
		return false
	}
	koerper, err := io.ReadAll(io.LimitReader(r.Body, weiterleitungKoerperMax+1))
	r.Body = io.NopCloser(bytes.NewReader(koerper))
	if err != nil || len(koerper) > weiterleitungKoerperMax {
		return false
	}
	hash := weiterleitungHash(r.Method, r.URL.Path, zeit, koerper)
	pub, err := crypto.SigToPub(hash, sig)
	if err != nil {
		return false
	}
	if !l.KenntValidator(strings.ToLower(crypto.PubkeyToAddress(*pub).Hex())) {
		return false
	}
	// Der Hash, nicht die Unterschrift: eine Unterschrift laesst sich in eine
	// zweite, ebenso gueltige umformen (s und n-s).
	return weiterleitungErstmals(hex.EncodeToString(hash), jetzt)
}

var weiterleitungGemerkt struct {
	sync.Mutex
	bis map[string]time.Time
}

// weiterleitungErstmals bucht eine Weiterleitung; false, wenn sie schon
// gesehen wurde oder die Merkliste voll ist (fail-closed).
func weiterleitungErstmals(schluessel string, jetzt time.Time) bool {
	weiterleitungGemerkt.Lock()
	defer weiterleitungGemerkt.Unlock()
	if weiterleitungGemerkt.bis == nil {
		weiterleitungGemerkt.bis = map[string]time.Time{}
	}
	if bis, schon := weiterleitungGemerkt.bis[schluessel]; schon && jetzt.Before(bis) {
		return false
	}
	if len(weiterleitungGemerkt.bis) >= weiterleitungGemerktMax {
		for k, bis := range weiterleitungGemerkt.bis {
			if !jetzt.Before(bis) {
				delete(weiterleitungGemerkt.bis, k)
			}
		}
		if len(weiterleitungGemerkt.bis) >= weiterleitungGemerktMax {
			return false
		}
	}
	weiterleitungGemerkt.bis[schluessel] = jetzt.Add(2 * leitungZeitfenster)
	return true
}
