package keeper

// KANDIDATENPROBE: WER ZU LANGSAM IST, WIRD KEIN VALIDATOR.
//
// Ziel des Netzes sind 20.000-30.000 Ueberweisungen je Sekunde. Ein
// Validator, der das nicht mittraegt, bremst jeden Block, den er prueft oder
// baut. Deshalb misst der Knoten, bei dem sich ein neuer Validator anmeldet
// (/api/peers/register), dessen Leistung SELBST -- nicht, was der Kandidat
// ueber sich behauptet:
//
//  1. Die Anmeldung ist schon geprueft (registrierter Mensch, Bindung
//     unterschrieben, Signierschluessel nachgewiesen). Erst dann kostet der
//     Kandidat uns Arbeit.
//  2. Wir schicken ihm die Leistungsprobe (leistungsprobe.go): n Schluessel
//     ableiten, signieren, Absender wiederherstellen -- genau die Arbeit
//     einer Ueberweisung, aus einem Zufallswert, den er vorher nicht kennt.
//     Wir rechnen dasselbe nach und messen die Zeit bis zur Antwort, abzueglich
//     einer leeren Anfrage (Laufzeit).
//  3. Bestanden: Ergebnis richtig UND schnell genug fuer die Schwelle
//     (AEQUITAS_LEISTUNG_MIN_SIG). Sonst wird die Anmeldung abgewiesen,
//     mit Messwert und Grenze in der Antwort -- der Betreiber weiss, woran
//     es liegt.
//
// Fail-closed: nicht erreichbar, falsche Antwort, zu langsam, Probe noch
// nicht gelaufen -- alles heisst "nicht aufgenommen". Der Kandidat meldet
// sich alle 30 s neu (registerAndDiscover); liegt dann ein bestandenes
// Ergebnis vor, geht es weiter.
//
// Grenzen (AGENTS.md, "Was ist begrenzt?"):
//   - hoechstens kandidatInflightMax Proben gleichzeitig, netzweit von hier,
//   - je Kandidat eine Probe je kandidatWiederholAbstand nach einem Fehlschlag,
//   - hoechstens kandidatMaxEintraege gemerkte Ergebnisse (aelteste gehen),
//   - feste Probengroesse (probeAnzahl), Antwort hoechstens 4 KiB, 5 s Zeit,
//   - der Kandidat nimmt Proben nur von Blockproduzenten an, eine je 30 s.
//
// Was die Probe NICHT kann: den Arbeitsspeicher oder die Platte des
// Kandidaten sehen. Die prueft einrichten.sh vor dem Start (und der Knoten
// misst seinen Datenbank-Commit selbst, leistungsnachweis.go). Die Probe ist
// der Teil, den ein Kandidat nicht schoenreden kann.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	kandidatMaxEintraege     = 1024
	kandidatBestandenGueltig = 24 * time.Hour
	kandidatWiederholAbstand = 10 * time.Minute
	kandidatInflightMax      = 2
)

// kandidatKlient: Ziel-URL kommt vom Kandidaten -- also SSRF-Schutz
// (pinningDialer: nur oeffentliche Adressen, DNS einmal aufgeloest) und
// keine Umleitungen.
var kandidatKlient = &http.Client{
	Timeout:       5 * time.Second,
	Transport:     &http.Transport{DialContext: pinningDialer},
	CheckRedirect: ohneUmleitung,
}

type KandidatErgebnis struct {
	Status   string `json:"status"` // bestanden | nicht_bestanden | ausstehend
	Grund    string `json:"grund,omitempty"`
	DauerMs  int64  `json:"dauer_ms,omitempty"`
	GrenzeMs int64  `json:"grenze_ms,omitempty"`
	ZeitUnix int64  `json:"zeit"`
}

type kandidatenStand struct {
	mu       sync.Mutex
	e        map[string]KandidatErgebnis
	inflight int
}

var kandidaten = &kandidatenStand{e: map[string]KandidatErgebnis{}}

// kandidatProbeFn fuehrt die eigentliche Probe aus (in Tests ersetzt).
var kandidatProbeFn = func(dag *BlockDAG, url string) KandidatErgebnis {
	return dag.kandidatProben(url)
}

// pruefeOderStarte: liegt ein gueltiges "bestanden" vor? Wenn nicht und
// keine Probe laeuft oder gesperrt ist, wird eine gestartet (asynchron).
// Liefert das aktuelle Ergebnis; nur Status "bestanden" oeffnet das Tor.
func (k *kandidatenStand) pruefeOderStarte(addr, url string, jetzt time.Time, start func(addr, url string)) KandidatErgebnis {
	k.mu.Lock()
	defer k.mu.Unlock()
	e, ok := k.e[addr]
	alter := jetzt.Sub(time.Unix(e.ZeitUnix, 0))
	switch {
	case ok && e.Status == "bestanden" && alter < kandidatBestandenGueltig:
		return e
	case ok && e.Status == "ausstehend":
		return e
	case ok && e.Status == "nicht_bestanden" && alter < kandidatWiederholAbstand:
		return e
	}
	if k.inflight >= kandidatInflightMax {
		return KandidatErgebnis{Status: "ausstehend", Grund: "andere Proben laufen, naechste Anmeldung versucht es erneut", ZeitUnix: jetzt.Unix()}
	}
	k.platzLocked()
	k.inflight++
	neu := KandidatErgebnis{Status: "ausstehend", Grund: "Leistungsprobe laeuft", ZeitUnix: jetzt.Unix()}
	k.e[addr] = neu
	start(addr, url)
	return neu
}

// platzLocked: Obergrenze der gemerkten Ergebnisse; das aelteste Ergebnis
// geht, das keine laufende Probe ist.
func (k *kandidatenStand) platzLocked() {
	for len(k.e) >= kandidatMaxEintraege {
		alt, altZeit := "", int64(1<<62)
		for a, e := range k.e {
			if e.Status != "ausstehend" && e.ZeitUnix < altZeit {
				alt, altZeit = a, e.ZeitUnix
			}
		}
		if alt == "" {
			return // nur laufende Proben -- durch inflight ohnehin begrenzt
		}
		delete(k.e, alt)
	}
}

func (k *kandidatenStand) fertig(addr string, e KandidatErgebnis) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.inflight--
	k.e[addr] = e
}

func (k *kandidatenStand) lies(addr string) (KandidatErgebnis, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	e, ok := k.e[addr]
	return e, ok
}

// kandidatZugelassen: das Tor in handlePeerRegister. Nur der eigene
// Schluessel braucht keine Probe. Auch ein schon festgelegter Produzent muss
// bestehen: eine Eintragung in AUTHORIZED_VALIDATORS macht einen zu
// schwachen Server nicht schneller.
func (a *APIServer) kandidatZugelassen(addr, url string) KandidatErgebnis {
	jetzt := time.Now()
	if a.blockchain != nil && addr == a.blockchain.selfProposer {
		return KandidatErgebnis{Status: "bestanden", Grund: "dieser Knoten selbst", ZeitUnix: jetzt.Unix()}
	}
	if url == "" || !isAllowedPeerURL(url) {
		return KandidatErgebnis{Status: "nicht_bestanden", Grund: "keine oeffentliche Adresse (SELF_URL) -- ohne sie laesst sich die Leistung nicht messen", ZeitUnix: jetzt.Unix()}
	}
	dag := a.blockchain
	return kandidaten.pruefeOderStarte(addr, url, jetzt, func(addr, url string) {
		SafeGoroutine("kandidatenprobe", func() {
			e := KandidatErgebnis{Status: "nicht_bestanden", Grund: "Probe abgebrochen", ZeitUnix: time.Now().Unix()}
			defer func() { kandidaten.fertig(addr, e) }()
			e = kandidatProbeFn(dag, url)
			e.ZeitUnix = time.Now().Unix()
			fmt.Printf("[KANDIDAT] %s: %s (%s)\n", addr, e.Status, e.Grund)
		})
	})
}

func (dag *BlockDAG) istFesterProduzent(addr string) bool {
	return len(dag.produzentenFest) > 0 && dag.produzentenFest[addr]
}

// istProduzent: festgelegt ODER einer, dessen Bloecke dieser Knoten annimmt
// (authorizedValidators). Ein neuer Knoten ohne AUTHORIZED_VALIDATORS kennt
// die Produzenten nur so -- und muss deren Proben annehmen koennen.
func (dag *BlockDAG) istProduzent(addr string) bool {
	if dag.istFesterProduzent(addr) {
		return true
	}
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	return dag.authorizedValidators[addr]
}

// kandidatProben: Laufzeit messen, Aufgabe stellen, selbst nachrechnen.
func (dag *BlockDAG) kandidatProben(url string) KandidatErgebnis {
	if dag == nil || dag.signingKey == nil {
		return KandidatErgebnis{Status: "nicht_bestanden", Grund: "dieser Knoten kann keine Probe stellen (kein Signierschluessel)"}
	}
	von := strings.ToLower(dag.selfProposer)
	anfrage := func(n int, seed string) LeitNachricht {
		m := LeitNachricht{Art: leitArtProbe, Von: von, ZeitMs: time.Now().UnixMilli(), BlockHash: seed, Hoehe: int64(n)}
		dag.signiereLeitNachricht(&m)
		return m
	}
	_, rtt, err := probePostMit(kandidatKlient, url, anfrage(0, ""))
	if err != nil {
		return KandidatErgebnis{Status: "nicht_bestanden", Grund: "nicht erreichbar oder nimmt keine Probe an (Port 8080 offen? aktuelle Version?)"}
	}
	var s [16]byte
	if _, err := rand.Read(s[:]); err != nil {
		return KandidatErgebnis{Status: "nicht_bestanden", Grund: "Zufall nicht verfuegbar"}
	}
	seed := hex.EncodeToString(s[:])
	erhalten, dauer, err := probePostMit(kandidatKlient, url, anfrage(probeAnzahl, seed))
	if err != nil {
		return KandidatErgebnis{Status: "nicht_bestanden", Grund: "keine Antwort auf die Probe in 5 s"}
	}
	erwartet, err := leistungsprobeRechnen(s[:], probeAnzahl)
	if err != nil {
		return KandidatErgebnis{Status: "nicht_bestanden", Grund: "Probe hier nicht nachrechenbar"}
	}
	minSig := leistungsSchwellenAusUmgebung().MinSigProSek
	grenze := probeGrenze(probeAnzahl, minSig)
	rechen := dauer - rtt
	if rechen < 0 {
		rechen = 0
	}
	e := KandidatErgebnis{DauerMs: rechen.Milliseconds(), GrenzeMs: grenze.Milliseconds()}
	switch {
	case erwartet != erhalten:
		e.Status, e.Grund = "nicht_bestanden", "falsches Ergebnis"
	case !probeBestanden(erwartet, erhalten, dauer, rtt, probeAnzahl, minSig):
		e.Status = "nicht_bestanden"
		e.Grund = fmt.Sprintf("zu langsam: %d ms fuer %d Signaturen, erlaubt %d ms (mindestens %.0f je Sekunde)", e.DauerMs, probeAnzahl, e.GrenzeMs, minSig)
	default:
		e.Status, e.Grund = "bestanden", fmt.Sprintf("%d ms, erlaubt %d ms", e.DauerMs, e.GrenzeMs)
	}
	return e
}

// handleKandidatenprobe: GET /api/kandidatenprobe?signing_address=0x...
// Oeffentlich und nur lesend: der Stand der Probe fuer einen Kandidaten
// (einrichten.sh zeigt ihn an). Enthaelt nichts Geheimes.
func (a *APIServer) handleKandidatenprobe(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"GET required"}`, http.StatusMethodNotAllowed)
		return
	}
	addr := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("signing_address")))
	if !istHexAdresse(addr) {
		http.Error(w, `{"error":"signing_address: 0x + 40 hex"}`, http.StatusBadRequest)
		return
	}
	e, ok := kandidaten.lies(addr)
	if !ok {
		e = KandidatErgebnis{Status: "unbekannt", Grund: "noch keine Anmeldung mit diesem Schluessel"}
	}
	s := leistungsSchwellenAusUmgebung()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"signing_address": addr,
		"probe":           e,
		"schwellen":       s,
	})
}
