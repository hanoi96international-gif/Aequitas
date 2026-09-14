package keeper

import (
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
// Speicher: je Schluessel hoechstens N Zeitstempel; die Aufraeumroutine von
// registerRateLimit (register.go) raeumt hier mit auf.

var ipBurst sync.Map // key -> *ipBurstEintrag

type ipBurstEintrag struct {
	mu     sync.Mutex
	zeiten []time.Time
}

// burstErlaubt meldet, ob fuer key noch eine Anfrage im Fenster frei ist, und
// bucht sie, wenn ja.
func burstErlaubt(key string, max int, fenster time.Duration) bool {
	if max <= 0 {
		return true
	}
	now := time.Now()
	v, _ := ipBurst.LoadOrStore(key, &ipBurstEintrag{})
	e := v.(*ipBurstEintrag)
	e.mu.Lock()
	defer e.mu.Unlock()
	// Verfallene vorne wegschneiden.
	i := 0
	for i < len(e.zeiten) && now.Sub(e.zeiten[i]) >= fenster {
		i++
	}
	if i > 0 {
		e.zeiten = append(e.zeiten[:0], e.zeiten[i:]...)
	}
	if len(e.zeiten) >= max {
		return false
	}
	e.zeiten = append(e.zeiten, now)
	return true
}

// ipBurstAufraeumen entfernt Schluessel ohne Eintrag im Fenster.
func ipBurstAufraeumen(fenster time.Duration) {
	now := time.Now()
	ipBurst.Range(func(k, v any) bool {
		e := v.(*ipBurstEintrag)
		e.mu.Lock()
		leer := len(e.zeiten) == 0 || now.Sub(e.zeiten[len(e.zeiten)-1]) >= fenster
		e.mu.Unlock()
		if leer {
			ipBurst.Delete(k)
		}
		return true
	})
}

// Die Grenzen. Je Fenster von 60 s und Absender-IP:
const (
	burstProveJeIP      = 12 // /api/prove -- ein Beweis kostet den Proof-Server ~1 s CPU
	burstRegisterJeIP   = 12 // /api/register -- Groth16-Pruefung auf dem Knoten
	burstCredentialJeIP = 20 // /api/humanity/credential -- sequentieller Scan ueber chain_blocks
	burstFenster        = 60 * time.Second
)
