package keeper

import (
	"encoding/json"
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

// proveHerkunft haelt fest, welche Nullifier aus einem erfolgreichen
// /prove-Durchlauf dieses Knotens stammen.
var proveHerkunft sync.Map // nullifier (klein, ohne 0x) -> time.Time

// proveHerkunftTTL ist etwas grosszuegiger als die Gueltigkeit der
// Bescheinigung selbst (900 s im Proof-Server). Der Mensch soll nicht daran
// scheitern, dass er zwischen Beweis und Registrierung kurz ueberlegt hat.
const proveHerkunftTTL = 15 * time.Minute

func nullifierSchluessel(s string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x")
}

// merkeProveHerkunft liest den Nullifier aus einer erfolgreichen
// /prove-Antwort und haelt ihn fest.
//
// Fehler beim Auslesen sind bewusst still: die Antwort geht so oder so an den
// Aufrufer, und ein Proxy, der wegen einer Notiz eine gueltige Antwort
// verwirft, waere schlimmer als die Notiz wert ist. Fehlt sie, scheitert
// spaeter die Registrierung -- die umkehrbare Richtung.
func merkeProveHerkunft(respBody []byte) {
	var b struct {
		ZKNullifier string `json:"zkNullifier"`
	}
	if err := json.Unmarshal(respBody, &b); err != nil || b.ZKNullifier == "" {
		return
	}
	jetzt := time.Now()
	proveHerkunft.Store(nullifierSchluessel(b.ZKNullifier), jetzt)

	// Beim Schreiben aufraeumen statt per Zeitgeber: die Menge ist klein, und
	// ein Zeitgeber waere eine Goroutine mehr fuer nichts.
	proveHerkunft.Range(func(k, v any) bool {
		if t, ok := v.(time.Time); ok && jetzt.Sub(t) > proveHerkunftTTL {
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
// stammt und noch nicht verfallen ist.
func hatProveHerkunft(nullifier string) bool {
	v, ok := proveHerkunft.Load(nullifierSchluessel(nullifier))
	if !ok {
		return false
	}
	t, ok := v.(time.Time)
	return ok && time.Since(t) <= proveHerkunftTTL
}
