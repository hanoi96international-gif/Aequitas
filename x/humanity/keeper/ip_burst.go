package keeper

import (
	"strings"
	"sync"
	"time"
)

// Burst-Grenze je Absender-IP -- statt "eine Anfrage je N Sekunden".
//
// WARUM. /api/prove (3 s je IP), /api/register (10 s) und
// /api/humanity/credential (5 s) liessen je IP genau EINE Anfrage im Fenster
// zu. Fuer einen Angreifer mit einer Adresse ist das dicht. Fuer Menschen
// hinter derselben Adresse ist es ein Tor mit einer Person Platz: Mobilfunk
// steckt tausende Kunden hinter eine CGNAT-Adresse, und zehn Leute an einem
// Tisch im selben WLAN sind EINE Adresse. Die zweite Person bekam "too many
// requests -- please wait 10 seconds" -- nach Minuten vor der Kamera, mit
// einer Aufnahme, die sie danach wiederholen musste.
//
// WAS SICH AENDERT. Je IP sind jetzt N Anfragen je Fenster erlaubt (gleitend,
// nicht kalenderfest). Die Kosten je Anfrage bleiben gedeckelt, nur der
// Deckel kennt Gruppen. Die Grenzen je WALLET und je Nullifier bleiben
// unveraendert -- sie sind die eigentliche Missbrauchsgrenze, die IP-Grenze
// ist nur der Rechenzeitschutz.
//
// Speicher: je Schluessel hoechstens N Zeitstempel, hoechstens
// grenzenSchluesselHoechstens Schluessel (begrenzte_karte.go); die
// Aufraeumroutine von registerRateLimit (register.go) raeumt hier mit auf.

// ipBurst: key -> *ipBurstEintrag, hoechstens grenzenSchluesselHoechstens
// Schluessel (begrenzte_karte.go).
var ipBurst = neueBegrenzteKarte("ip_burst", grenzenSchluesselHoechstens, nil)

// erneuerungVon: die Zaehler weitergeleiteter Erneuerungen
// (erneuerungVonPraefix + "<folger>|<fuer>") in einer eigenen Karte. Den
// Absender fuer bestimmt dort der unterschreibende Folger: ein boeswilliges
// Mitglied des Satzes machte mit immer neuem fuer sonst ipBurst voll, und
// voll sperrt dort jeden neuen Absender an jedem Endpunkt
// (begrenzte_karte.go). So trifft es nur neue weitergeleitete Erneuerungen --
// wie walletRateLimit (Pruefung von #324, HIGH-1).
var erneuerungVon = neueBegrenzteKarte("erneuerung_weitergeleitet", grenzenSchluesselHoechstens, nil)

const erneuerungVonPraefix = "liveness-renewal-von:"

// burstKarte: in welcher Karte key zaehlt.
func burstKarte(key string) *begrenzteKarte {
	if strings.HasPrefix(key, erneuerungVonPraefix) {
		return erneuerungVon
	}
	return ipBurst
}

type ipBurstEintrag struct {
	mu     sync.Mutex
	zeiten []time.Time
	// zuletzt: die letzte Buchung, auch wenn sie erstattet wurde. Das
	// Aufraeumen haelt den Eintrag, solange sie im Fenster liegt -- sonst
	// verloere ein Folger, dessen gueltige Pruefungen alle erstattet werden,
	// bei voller Karte seinen Platz und gaelte als neuer Absender (Pruefung
	// von #319, LOW-39).
	zuletzt time.Time
}

// burstErlaubt meldet, ob fuer key noch eine Anfrage im Fenster frei ist, und
// bucht sie, wenn ja.
func burstErlaubt(key string, max int, fenster time.Duration) bool {
	_, ok := burstBuchen(key, max, fenster)
	return ok
}

// burstBuchen: wie burstErlaubt, liefert dazu den Zeitpunkt der Buchung (fuer
// burstErstatten). Die Zeit wird unter der Sperre genommen, damit die
// Eintraege der Reihe nach liegen.
func burstBuchen(key string, max int, fenster time.Duration) (time.Time, bool) {
	if max <= 0 {
		return time.Time{}, true
	}
	v, ok := burstKarte(key).LoadOrStore(key, &ipBurstEintrag{})
	if !ok {
		return time.Time{}, false // voll: ein neuer Absender wird begrenzt (begrenzte_karte.go)
	}
	e := v.(*ipBurstEintrag)
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	// Verfallene vorne wegschneiden.
	i := 0
	for i < len(e.zeiten) && now.Sub(e.zeiten[i]) >= fenster {
		i++
	}
	if i > 0 {
		e.zeiten = append(e.zeiten[:0], e.zeiten[i:]...)
	}
	if len(e.zeiten) >= max {
		return time.Time{}, false
	}
	e.zeiten = append(e.zeiten, now)
	e.zuletzt = now
	return now, true
}

// burstErstatten nimmt die Buchung von key zum Zeitpunkt zeit zurueck
// (erneuerungsGrenze: das Pruefbudget wird VOR der Pruefung gebucht, damit
// gleichzeitige Anfragen die Grenze nicht ueberholen, und ein gueltiger
// Nachweis bekommt seine Buchung zurueck -- Pruefung von #319, LOW-30). Genau
// diese, nicht die juengste: sonst verfiele eine fremde, aeltere Buchung
// frueher. Ist sie schon verfallen, gibt es nichts zu erstatten.
func burstErstatten(key string, zeit time.Time) {
	v, ok := burstKarte(key).Load(key)
	if !ok {
		return
	}
	e := v.(*ipBurstEintrag)
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := len(e.zeiten) - 1; i >= 0; i-- {
		if e.zeiten[i].Equal(zeit) {
			e.zeiten = append(e.zeiten[:i], e.zeiten[i+1:]...)
			return
		}
	}
}

// ipBurstAufraeumen entfernt Schluessel ohne Eintrag im Fenster (ipBurst und
// erneuerungVon).
func ipBurstAufraeumen(fenster time.Duration) {
	now := time.Now()
	for _, karte := range []*begrenzteKarte{ipBurst, erneuerungVon} {
		karte.Range(func(k, v any) bool {
			e := v.(*ipBurstEintrag)
			e.mu.Lock()
			leer := (len(e.zeiten) == 0 || now.Sub(e.zeiten[len(e.zeiten)-1]) >= fenster) && now.Sub(e.zuletzt) >= fenster
			e.mu.Unlock()
			if leer {
				karte.Delete(k)
			}
			return true
		})
	}
}

// Die Grenzen. Je Fenster von 60 s und Absender-IP:
const (
	burstProveJeIP      = 12 // /api/prove -- ein Beweis kostet den Proof-Server ~1 s CPU
	burstRegisterJeIP   = 12 // /api/register -- Groth16-Pruefung auf dem Knoten
	burstCredentialJeIP = 20 // /api/humanity/credential -- sequentieller Scan ueber chain_blocks
	burstFenster        = 60 * time.Second
	// /api/prove/get -- jeder Aufruf ist eine Anfrage an den Proof-Server,
	// und dort teilen sich ALLE Aufrufe dieses Knotens einen Zaehler (seine
	// IP). Ohne Grenze hier konnte eine einzige Adresse mit Abrufen erfundener
	// Kennungen diesen gemeinsamen Zaehler leeren -- und damit /prove fuer
	// alle anhalten (Audit 2026-09-29, H4).
	burstProveGetJeIP = 30
	// /api/liveness-renewal -- je Anfrage eine Abfrage im Coordinator-Register
	// und die Zulassung (Datenbank). Ein Coordinator erneuert hoechstens so
	// viele Menschen, wie am Tag registriert werden (Pruefung #314, LOW-3).
	// Weitergeleitete Erneuerungen mit gueltigem Nachweis zaehlen beim
	// Zustaendigen unter (Folger, Absender) in erneuerungVon
	// (weiterleitung_nachweis.go), alle anderen unter ihrer Verbindung.
	burstErneuerungJeIP = 30
	// Pruefungen von Weiterleitungsnachweisen (weiterleitung_nachweis.go) je
	// Absender, danach wird nicht mehr geprueft: deckelt die Kosten (Koerper
	// lesen, ecrecover) gefaelschter Nachweise -- auch gleichzeitiger, denn
	// gebucht wird vor der Pruefung. Ein gueltiger Nachweis bekommt seine
	// Buchung zurueck.
	burstNachweisPruefungJeIP = 600
)
