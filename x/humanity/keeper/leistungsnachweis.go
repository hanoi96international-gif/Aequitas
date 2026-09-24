package keeper

import (
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// Leistungsnachweis fuer die Leiterrolle.
//
// Der Leiter nimmt ALLE Ueberweisungen des Netzes an: er prueft jede
// Signatur und schreibt jede Annahme in die Datenbank. Ein Validator auf
// schwacher Hardware wuerde als Leiter das ganze Netz ausbremsen. Deshalb
// misst jeder Knoten selbst, was er kann, und kuendigt das Ergebnis in
// seinen signierten Leitungsnachrichten an (LeitNachricht.Faehig):
//
//   - Signaturen je Sekunde: secp256k1-Wiederherstellung wie bei jeder
//     Ueberweisung, auf allen Kernen parallel.
//   - Datenbank: Dauer eines einzelnen Commits (Median), das ist der Takt,
//     in dem Annahmen dauerhaft werden.
//   - Kerne.
//
// Gewertet wird der BESTE je gemessene Wert: Last kann eine Messung nur
// verschlechtern, nie verbessern -- wer einmal gezeigt hat, dass seine
// Hardware es kann, verliert den Nachweis nicht, weil er gerade unter
// Volllast steht (sonst gaebe ein Leiter mitten in der Spitze ab).
//
// Folgen (leitung.go): wer den Nachweis nicht haelt, wird nur Leiter, wenn
// kein leiterfaehiger erreichbar ist (Notbetrieb), und gibt ab, sobald einer
// lebt. Der Nachweis entscheidet, WER leitet -- nie, OB jemand annimmt.
//
// Vertrauensmodell wie bei der Leitung insgesamt: Validatoren sind
// zugelassen und signieren; ein Validator, der ueber seine Hardware luegt,
// schadet der Geschwindigkeit, nicht der Sicherheit.
//
// Umgebung:
//
//	AEQUITAS_LEITER_FAEHIG=ja|nein    Betreiber entscheidet (ueberstimmt die Messung)
//	AEQUITAS_LEISTUNG_MIN_SIG=20000   Signaturen je Sekunde, mindestens
//	AEQUITAS_LEISTUNG_MAX_COMMIT_MS=20  Commit-Dauer, hoechstens
//	AEQUITAS_LEISTUNG_MIN_KERNE=4

const (
	leiterFaehigEnv    = "AEQUITAS_LEITER_FAEHIG"
	leistungMinSigEnv  = "AEQUITAS_LEISTUNG_MIN_SIG"
	leistungMaxComEnv  = "AEQUITAS_LEISTUNG_MAX_COMMIT_MS"
	leistungMinKernEnv = "AEQUITAS_LEISTUNG_MIN_KERNE"
)

// LeistungsSchwellen: was ein Leiter mindestens koennen muss.
type LeistungsSchwellen struct {
	MinSigProSek float64 `json:"min_signaturen_pro_sek"`
	MaxCommitMs  float64 `json:"max_commit_ms"`
	MinKerne     int     `json:"min_kerne"`
}

// Vorgaben: Ziel sind 10.000 Ueberweisungen je Sekunde. Die Signaturpruefung
// ist nur ein Teil der Arbeit je Ueberweisung (Profil: ~18 % der CPU), also
// verlangt der Nachweis das Doppelte des Ziels allein dafuer.
func leistungsVorgabe() LeistungsSchwellen {
	return LeistungsSchwellen{MinSigProSek: 20000, MaxCommitMs: 20, MinKerne: 4}
}

func leistungsSchwellenAusUmgebung() LeistungsSchwellen {
	s := leistungsVorgabe()
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(leistungMinSigEnv)), 64); err == nil && v >= 0 {
		s.MinSigProSek = v
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(leistungMaxComEnv)), 64); err == nil && v > 0 {
		s.MaxCommitMs = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(leistungMinKernEnv))); err == nil && v >= 0 {
		s.MinKerne = v
	}
	return s
}

// leiterFaehigZwang: "ja"/"nein" vom Betreiber, sonst "".
func leiterFaehigZwang() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(leiterFaehigEnv))) {
	case "ja", "1", "true", "yes":
		return "ja"
	case "nein", "0", "false", "no":
		return "nein"
	}
	return ""
}

// Leistungsmessung: ein Durchgang.
type Leistungsmessung struct {
	SigProSek float64 `json:"signaturen_pro_sek"`
	CommitMs  float64 `json:"commit_ms"` // Median; 0 = nicht gemessen
	Kerne     int     `json:"kerne"`
	ZeitMs    int64   `json:"zeit_ms"`
	Fehler    string  `json:"fehler,omitempty"`
}

type leistungsnachweis struct {
	mu        sync.Mutex
	schwellen LeistungsSchwellen
	zwang     string
	bester    Leistungsmessung // bester Wert je Groesse, nicht ein Durchgang
	letzte    Leistungsmessung
	messungen int
}

var leistung = &leistungsnachweis{schwellen: leistungsVorgabe()}

// uebernehmen: Ergebnis einrechnen (bester Wert je Groesse).
func (ln *leistungsnachweis) uebernehmen(m Leistungsmessung) {
	ln.mu.Lock()
	defer ln.mu.Unlock()
	ln.letzte = m
	ln.messungen++
	if m.SigProSek > ln.bester.SigProSek {
		ln.bester.SigProSek = m.SigProSek
	}
	if m.CommitMs > 0 && (ln.bester.CommitMs == 0 || m.CommitMs < ln.bester.CommitMs) {
		ln.bester.CommitMs = m.CommitMs
	}
	if m.Kerne > ln.bester.Kerne {
		ln.bester.Kerne = m.Kerne
	}
	ln.bester.ZeitMs = m.ZeitMs
}

// erfuellt und, falls nicht, warum.
func (ln *leistungsnachweis) erfuellt() (bool, string) {
	ln.mu.Lock()
	defer ln.mu.Unlock()
	switch ln.zwang {
	case "ja":
		return true, "vom Betreiber gesetzt (" + leiterFaehigEnv + "=ja)"
	case "nein":
		return false, "vom Betreiber gesetzt (" + leiterFaehigEnv + "=nein)"
	}
	if ln.messungen == 0 {
		return false, "noch nicht gemessen"
	}
	b, s := ln.bester, ln.schwellen
	var fehlt []string
	if b.SigProSek < s.MinSigProSek {
		fehlt = append(fehlt, fmt.Sprintf("Signaturen %.0f/s < %.0f/s", b.SigProSek, s.MinSigProSek))
	}
	if b.CommitMs == 0 {
		fehlt = append(fehlt, "Datenbank-Commit nicht messbar")
	} else if b.CommitMs > s.MaxCommitMs {
		fehlt = append(fehlt, fmt.Sprintf("Commit %.1f ms > %.1f ms", b.CommitMs, s.MaxCommitMs))
	}
	if b.Kerne < s.MinKerne {
		fehlt = append(fehlt, fmt.Sprintf("Kerne %d < %d", b.Kerne, s.MinKerne))
	}
	if len(fehlt) > 0 {
		return false, strings.Join(fehlt, "; ")
	}
	return true, "gemessen"
}

// leistungsnachweisErfuellt: haelt dieser Knoten den Nachweis?
func leistungsnachweisErfuellt() bool {
	ok, _ := leistung.erfuellt()
	return ok
}

// messeSignaturen: secp256k1-Wiederherstellungen je Sekunde, auf allen
// Kernen parallel, fuer die angegebene Dauer.
func messeSignaturen(dauer time.Duration) float64 {
	key, err := crypto.GenerateKey()
	if err != nil {
		return 0
	}
	hash := crypto.Keccak256([]byte("aequitas-leistungsnachweis"))
	sig, err := crypto.Sign(hash, key)
	if err != nil {
		return 0
	}
	var n atomic.Int64
	var wg sync.WaitGroup
	start := time.Now()
	ende := start.Add(dauer)
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(ende) {
				if _, err := crypto.SigToPub(hash, sig); err != nil {
					return
				}
				n.Add(1)
			}
		}()
	}
	wg.Wait()
	sek := time.Since(start).Seconds()
	if sek <= 0 {
		return 0
	}
	return float64(n.Load()) / sek
}

// messeCommit: Median der Dauer einzelner, dauerhaft geschriebener
// Commits (je ein Upsert in eigener Transaktion).
func messeCommit(db *sql.DB, anzahl int) (float64, error) {
	if db == nil {
		return 0, fmt.Errorf("keine Datenbank")
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS leistung_probe (id INT PRIMARY KEY, t BIGINT NOT NULL)`); err != nil {
		return 0, err
	}
	var dauern []float64
	for i := 0; i < anzahl; i++ {
		t0 := time.Now()
		if _, err := db.Exec(`INSERT INTO leistung_probe (id, t) VALUES (1, $1)
			ON CONFLICT (id) DO UPDATE SET t = EXCLUDED.t`, t0.UnixNano()); err != nil {
			return 0, err
		}
		dauern = append(dauern, float64(time.Since(t0).Microseconds())/1000)
	}
	sort.Float64s(dauern)
	return dauern[len(dauern)/2], nil
}

// leistungMessen: ein vollstaendiger Durchgang.
func leistungMessen(db *sql.DB) Leistungsmessung {
	m := Leistungsmessung{Kerne: runtime.NumCPU(), ZeitMs: time.Now().UnixMilli()}
	m.SigProSek = messeSignaturen(500 * time.Millisecond)
	c, err := messeCommit(db, 15)
	if err != nil {
		m.Fehler = err.Error()
	}
	m.CommitMs = c
	return m
}

var leistungGestartet atomic.Bool

// StarteLeistungsnachweis: misst einmal sofort (die Leitung braucht das
// Ergebnis beim Start) und danach alle zehn Minuten, solange der Nachweis
// nicht erfuellt ist -- ein Knoten, der gerade beim Start ausgelastet war,
// bekommt so eine neue Gelegenheit. Laeuft auch ohne Leitung, damit der
// Stand im Health-Endpunkt zeigt, ob ein Knoten Leiter sein KOENNTE.
func StarteLeistungsnachweis(cs *ChainState) {
	if cs == nil || !leistungGestartet.CompareAndSwap(false, true) {
		return
	}
	leistung.mu.Lock()
	leistung.schwellen = leistungsSchwellenAusUmgebung()
	leistung.zwang = leiterFaehigZwang()
	leistung.mu.Unlock()
	messen := func() {
		leistung.uebernehmen(leistungMessen(cs.db))
		ok, grund := leistung.erfuellt()
		fmt.Printf("[LEISTUNG] leiterfaehig=%v (%s)\n", ok, grund)
	}
	SafeCall("leistung-messen", messen)
	SafeGoroutine("leistung-nachmessen", func() {
		for {
			time.Sleep(10 * time.Minute)
			if ok, _ := leistung.erfuellt(); !ok {
				SafeCall("leistung-messen", messen)
			}
		}
	})
}

// LeistungsnachweisStand fuer /api/health/combined.
func LeistungsnachweisStand() map[string]interface{} {
	ok, grund := leistung.erfuellt()
	leistung.mu.Lock()
	defer leistung.mu.Unlock()
	return map[string]interface{}{
		"leiterfaehig": ok,
		"grund":        grund,
		"bester":       leistung.bester,
		"letzte":       leistung.letzte,
		"messungen":    leistung.messungen,
		"schwellen":    leistung.schwellen,
		"bedeutung": "Leistungsnachweis fuer die Leiterrolle: bester je gemessener Wert (Last verschlechtert " +
			"Messungen nur). Ohne Nachweis wird ein Validator nur Leiter, wenn kein leiterfaehiger erreichbar ist.",
	}
}
