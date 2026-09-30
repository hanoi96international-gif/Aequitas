package keeper

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Kam dieser Nullifier wirklich aus einer geprueften Registrierung?
//
// DIE LUECKE, DIE DAS SCHLIESST
//
// Die gesamte biometrische Kette -- Aufnahme, drei Vergleichsdienste, Quorum,
// Bescheinigung -- haengt an /api/prove. Geld entsteht aber an /api/register,
// und dort wurde bis zum 26.08.2026 NICHTS davon geprueft: der Aufrufer
// reichte pA/pB/pC/pubSignals ein, der Vertrag prueste nur, dass der Beweis
// gueltig IST -- nicht, woher er kam.
//
// Groth16-Proving-Keys sind konstruktionsbedingt oeffentlich, und dieser liegt
// im Repo. Wer sich eine bio_hash wuerfelt, lokal einen Beweis erzeugt und
// direkt an /api/register geht, praegt 1.000 AEQ. Und nochmal. Die
// Gesichtspruefung war damit beratend, nicht bindend:
// BIO_ATTESTATION_MODE=required haertete einen Pfad, den niemand benutzen
// muss.
//
// Dieselbe Fehlerklasse wie BIO_ATTESTATION_MODE=off am 25.08.2026 -- ein Tor,
// das an der falschen Tuer steht.
//
// WIE DIE PRUEFUNG FUNKTIONIERT
//
// Der Knoten ist selbst der Proxy zum Proof-Server (handleProveProxy). Er
// sieht beide Enden: die Anfrage mit der Bescheinigung und die Antwort mit dem
// Nullifier. Kam eine Antwort mit Status 200 zurueck, hat der Proof-Server die
// Bescheinigung geprueft -- denn dort steht BIO_ATTESTATION_MODE=required, und
// ohne gueltige Bescheinigung antwortet er 403.
//
// Der Knoten merkt sich also: DIESER Nullifier ist durch die Pruefung
// gekommen. /api/register nimmt nur solche.
//
// WARUM NUR IM ARBEITSSPEICHER
//
// Es ist kein Kettenzustand, sondern eine kurzlebige Herkunftsnotiz zwischen
// zwei Aufrufen, die Sekunden auseinanderliegen. Sie in die Datenbank oder gar
// in den Konsens zu heben, brauchte Replikation und Wanderungspfade fuer etwas,
// das in 15 Minuten verfaellt -- und wuerde die Kette um Zustand erweitern,
// der nichts ueber die Kette aussagt.
//
// Der Preis ist eine Knotenbindung: wer bei Knoten A beweist, muss bei Knoten
// A registrieren. Die App zeigt ohnehin auf genau eine Adresse. Faellt der
// Knoten zwischen beiden Aufrufen aus, scheitert die Registrierung mit einer
// klaren Meldung und der Mensch versucht es erneut -- die umkehrbare
// Richtung.
//
// AN DIE WALLET GEBUNDEN (30.09.2026)
//
// Bis hierhin merkte sich der Knoten nur den Nullifier. Im Beweis steckt die
// Wallet aber nur als PRIVATER Eingang (commitment = Poseidon(bio, wallet,
// salt), Circuit v3); welche Wallet, sieht niemand. Wer eine /prove-Antwort
// abfing, bevor sie benutzt war (boesartige App, mitgelesener Verkehr),
// konnte sie mit der eigenen Signatur fuer die EIGENE Wallet einreichen: der
// Nullifier hatte Herkunft, die Signatur passte zur eigenen Wallet, der
// Beweis war gueltig. Ein fremdes Gesicht, ein eigenes Konto.
//
// Die Wallet steht im Rumpf der /prove-Anfrage. Der Proof-Server erzeugt den
// Beweis genau fuer sie, und die Bescheinigung des Coordinators deckt genau
// (bio, wallet) ab (bio_attestation.js). Der Knoten merkt sich deshalb das
// Paar (Nullifier, Wallet), und /api/register nimmt den Nullifier nur fuer
// diese Wallet. Voll schliesst das erst Circuit v4 (Wallet als oeffentliches
// Signal), siehe docs/V8_ENTWURF.md (v).

// proveHerkunft haelt fest, welche Nullifier aus einem erfolgreichen
// /prove-Durchlauf dieses Knotens stammen -- und fuer welche Wallet.
var proveHerkunft sync.Map // kanonischer Nullifier -> herkunft

type herkunft struct {
	zeit   time.Time
	wallet string // klein, mit 0x
}

// proveHerkunftTTL ist etwas grosszuegiger als die Gueltigkeit der
// Bescheinigung selbst (900 s im Proof-Server). Der Mensch soll nicht daran
// scheitern, dass er zwischen Beweis und Registrierung kurz ueberlegt hat.
const proveHerkunftTTL = 15 * time.Minute

func nullifierSchluessel(s string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x")
}

// herkunftsSchluessel: die kanonische Zahl des Nullifiers (nullifier_canonical.go),
// nicht seine Schreibweise. Bis 29.09.2026 stand hier nullifierSchluessel -- es
// entfernte nur "0x". Damit galt eine /prove-Herkunft fuer N auch fuer "0x"+N,
// eine ANDERE Zahl, und zusammen mit der fehlenden Bindung an pubSignals
// (registerOnV7) liess sich mit einem ehrlichen /prove eine zweite Wallet mit
// selbst erzeugtem Beweis registrieren. Unlesbares ergibt "" und damit nie
// eine Herkunft.
func herkunftsSchluessel(s string) string {
	c, err := canonicalNullifier(s)
	if err != nil {
		return ""
	}
	return c
}

// merkeProveHerkunft liest die Wallet aus der /prove-ANFRAGE (fuer sie hat
// der Proof-Server den Beweis erzeugt) und den Nullifier aus der
// erfolgreichen ANTWORT und haelt das Paar fest.
//
// Fehler beim Auslesen sind bewusst still: die Antwort geht so oder so an den
// Aufrufer, und ein Proxy, der wegen einer Notiz eine gueltige Antwort
// verwirft, waere schlimmer als die Notiz wert ist. Fehlt sie, scheitert
// spaeter die Registrierung -- die umkehrbare Richtung. Ohne gueltige Wallet
// in der Anfrage wird nichts gemerkt (der Proof-Server haette sie ohnehin
// abgelehnt).
func merkeProveHerkunft(reqBody, respBody []byte) {
	wallet, ok := eindeutigeWallet(reqBody)
	if !ok || !isValidWalletAddr(wallet) {
		return
	}
	var b struct {
		ZKNullifier string `json:"zkNullifier"`
	}
	if err := json.Unmarshal(respBody, &b); err != nil || b.ZKNullifier == "" {
		return
	}
	schluessel := herkunftsSchluessel(b.ZKNullifier)
	if schluessel == "" {
		return
	}
	jetzt := time.Now()
	proveHerkunft.Store(schluessel, herkunft{zeit: jetzt, wallet: wallet})

	// Beim Schreiben aufraeumen statt per Zeitgeber: die Menge ist klein, und
	// ein Zeitgeber waere eine Goroutine mehr fuer nichts.
	proveHerkunft.Range(func(k, v any) bool {
		if h, ok := v.(herkunft); !ok || jetzt.Sub(h.zeit) > proveHerkunftTTL {
			proveHerkunft.Delete(k)
		}
		return true
	})
}

// proveHerkunftVerlangt sagt, ob /api/register eine Herkunft nachweisen muss.
//
// Voreinstellung AN. Ein Tor, das gebaut, aber nicht eingeschaltet ist,
// schuetzt niemanden -- und dieses hier steht vor der Stelle, an der Geld
// entsteht.
//
// Abschaltbar ueber REQUIRE_PROVE_PROVENANCE=false, fuer den Fall, dass ein
// Betreiber ohne eigenen Proof-Server fahren will. Wer das tut, hat dann
// wieder den Zustand von vor dem 26.08.2026, und das steht hier, damit es
// eine bewusste Entscheidung bleibt.
func proveHerkunftVerlangt() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("REQUIRE_PROVE_PROVENANCE")), "false")
}

// hatProveHerkunft prueft, ob dieser Nullifier aus einem /prove dieses Knotens
// FUER DIESE WALLET stammt und noch nicht verfallen ist.
func hatProveHerkunft(nullifier, wallet string) bool {
	schluessel := herkunftsSchluessel(nullifier)
	w := strings.ToLower(strings.TrimSpace(wallet))
	if schluessel == "" || !isValidWalletAddr(w) {
		return false
	}
	v, ok := proveHerkunft.Load(schluessel)
	if !ok {
		return false
	}
	h, ok := v.(herkunft)
	return ok && h.wallet == w && time.Since(h.zeit) <= proveHerkunftTTL
}

// eindeutigeWallet liest die Wallet aus dem /prove-Rumpf so, wie der
// Proof-Server sie liest -- und nur, wenn es genau EINE gibt.
//
// Sicherheitspruefung 30.09.2026: Go's encoding/json ordnet Schluessel ohne
// Ruecksicht auf Gross-/Kleinschreibung zu, und der letzte gewinnt. Der
// Proof-Server (express.json, JSON.parse) liest req.body.wallet genau so
// geschrieben. Ein Rumpf {"wallet":OPFER, ..., "Wallet":ANGREIFER} liess den
// Proof-Server Bescheinigung und Beweis fuer das Opfer pruefen, waehrend
// dieser Knoten die Herkunft fuer den Angreifer notierte -- mit einer
// mitgelesenen /prove-Anfrage haette er das Gesicht des Opfers auf seine
// Wallet registriert. Deshalb: genau ein Schluessel, der wie "wallet"
// aussieht, genau so geschrieben, als Zeichenkette. Alles andere gilt als
// mehrdeutig und bekommt keine Herkunft (und wird am Proxy abgewiesen).
func eindeutigeWallet(body []byte) (string, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return "", false
	}
	gefunden := 0
	var wallet string
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return "", false
		}
		schluessel, ok := t.(string)
		if !ok {
			return "", false
		}
		var wert json.RawMessage
		if err := dec.Decode(&wert); err != nil {
			return "", false
		}
		if !strings.EqualFold(schluessel, "wallet") {
			continue
		}
		gefunden++
		if schluessel != "wallet" {
			return "", false
		}
		if err := json.Unmarshal(wert, &wallet); err != nil {
			return "", false
		}
	}
	if t, err := dec.Token(); err != nil || t != json.Delim('}') || gefunden != 1 {
		return "", false
	}
	// Nichts hinter dem Objekt (JSON.parse wuerde es ablehnen, Go liesse es stehen).
	if _, err := dec.Token(); err != io.EOF {
		return "", false
	}
	return strings.ToLower(strings.TrimSpace(wallet)), true
}
