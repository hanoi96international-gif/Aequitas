package keeper

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"
)

// ROTIERENDER LEITER: WER GERADE UEBERWEISUNGEN ANNIMMT.
//
// # WARUM
//
// Seit dem 22.09.2026 nimmt genau ein Knoten Ueberweisungen an (annahme_tor.go):
// nehmen zwei gleichzeitig an, laufen die Kontenstaende auseinander, sobald ein
// Konto leerlaeuft -- belegt in zwei_produzenten_realdb_test.go. Dieser eine
// Knoten war aber fest eingestellt. Am 24.09.2026 lief sein Abo aus, und das
// Netz nahm stundenlang keine einzige Ueberweisung an. Ein Netz, das an einem
// Rechner haengt, ist nicht dezentral.
//
// Solana loest das mit einem Leiter, der nach festem Plan wechselt: in jedem
// Moment nimmt genau einer an, aber niemand dauerhaft. Dasselbe hier:
//
//   - Der Leiter einer AMTSZEIT (Term) nimmt an. Alle anderen leiten weiter.
//   - Planmaessiger Wechsel (WechselAlle): der Leiter hoert auf anzunehmen,
//     wartet, bis alles Angenommene in seinen Bloecken steht, und uebergibt
//     mit dem Hash seines letzten Blocks. Der Naechste nimmt erst an, wenn er
//     diesen Block nachgespielt hat. Es gibt keinen Moment mit zwei Leitern.
//   - Ausfall: hoert die Mehrheit laenger als FolgerFrist nichts vom Leiter,
//     waehlt sie den naechsten leiterfaehigen Validator. Ein Leiter nimmt nur
//     an, solange eine Mehrheit seine Lease innerhalb von LeaseDauer
//     bestaetigt hat, und ein Folger stimmt fuer niemand anderen, solange er
//     innerhalb von FolgerFrist eine Lease gesehen hat. Da FolgerFrist >
//     LeaseDauer, hat ein abgeschnittener Leiter aufgehoert, bevor irgendwo
//     ein neuer gewaehlt werden kann -- auch bei einer Netztrennung.
//
// Das ist der Wahl- und Lease-Teil von Raft. Die Daten selbst laufen weiter
// ueber die Bloecke; ein Protokoll-Log gibt es hier nicht.
//
// # MIT ZWEI VALIDATOREN
//
// Eine Mehrheit von zwei ist zwei. Ist einer weg, gibt es keine Mehrheit --
// und aus der Ferne ist ein toter Knoten nicht von einer Netztrennung zu
// unterscheiden. Eine automatische Uebernahme waere hier genau die Gefahr,
// die das Annahme-Tor beseitigt. Deshalb bei zwei Validatoren:
//   - keine Wahl, keine automatische Uebernahme;
//   - der Leiter nimmt ohne Lease-Bestaetigung an (sonst stuende das Netz,
//     sobald der ANDERE ausfaellt -- schlechter als heute);
//   - ein planmaessiger Wechsel nur, wenn ausdruecklich eingeschaltet.
// Wirkliche Ausfallsicherheit beginnt bei drei Validatoren.
//
// # WAS DIESE DATEI IST
//
// Reine Logik, ohne Netz, ohne Datenbank, mit eingespeister Uhr: damit laesst
// sie sich mit Ausfaellen, Verlusten und Netztrennungen simulieren
// (leitung_sim_test.go). Netz, Signaturen und Speicherung: leitung_netz.go.

// Nachrichtenarten.
const (
	leitArtLease       = "lease"        // Leiter -> alle: ich leite Term T
	leitArtLeaseAck    = "lease_ack"    // Folger -> Leiter
	leitArtStimmeBitte = "stimme_bitte" // Kandidat -> alle
	leitArtStimme      = "stimme"       // Antwort auf stimme_bitte
	leitArtUebergabe   = "uebergabe"    // alter Leiter -> naechster
)

// LeitNachricht ist alles, was zwischen Validatoren fuer die Leitung
// ausgetauscht wird. Signiert und geprueft in leitung_netz.go.
type LeitNachricht struct {
	Art       string `json:"art"`
	Term      uint64 `json:"term"`
	Von       string `json:"von"`
	URL       string `json:"url,omitempty"`
	ZeitMs    int64  `json:"zeit_ms"` // Sendezeit des Absenders (Lease-Rechnung)
	An        string `json:"an,omitempty"`
	Gewaehrt  bool   `json:"gewaehrt,omitempty"`
	Hoehe     int64  `json:"hoehe"`
	BlockHash string `json:"block_hash,omitempty"`
	Faehig    bool   `json:"faehig"`
	SatzHash  string `json:"satz_hash"`
	// Die Lease eines neuen Leiters nach planmaessigem Wechsel traegt die
	// Uebergabe des alten mit -- der Beleg, dass der alte aufgehoert hat.
	Uebergabe *LeitNachricht `json:"uebergabe,omitempty"`
	// Vorwahl (PreVote): "wuerdest du mich waehlen?" -- ohne dass irgendwer
	// seinen Term hochzaehlt. Siehe vorwahl() unten.
	Vorwahl bool   `json:"vorwahl,omitempty"`
	Sig     string `json:"sig,omitempty"`
}

// LeitKonfig: Zeiten. FolgerFrist muss groesser sein als LeaseDauer plus
// der moegliche Uhrengang -- sonst ist die Sicherheit bei Netztrennung weg.
type LeitKonfig struct {
	Takt        time.Duration // wie oft der Leiter seine Lease erneuert
	LeaseDauer  time.Duration // so lange traegt eine Mehrheits-Bestaetigung
	FolgerFrist time.Duration // so lange stimmt ein Folger fuer niemand anderen
	Staffel     time.Duration // Abstand der Kandidaturen nach Rang
	WechselAlle time.Duration // planmaessiger Wechsel; 0 = aus
	// Bei zwei Validatoren planmaessig wechseln? (siehe oben)
	ZweiWechseln bool
	// Hoechstens so viele Bloecke darf ein Kandidat hinter dem Waehler liegen.
	HoeheToleranz int64
}

func leitVorgabe() LeitKonfig {
	return LeitKonfig{
		Takt:          time.Second,
		LeaseDauer:    6 * time.Second,
		FolgerFrist:   12 * time.Second,
		Staffel:       4 * time.Second,
		WechselAlle:   60 * time.Minute,
		HoeheToleranz: 2,
	}
}

type leitRolle int

const (
	leitFolger leitRolle = iota
	leitKandidat
	leitLeiter
)

func (r leitRolle) String() string {
	switch r {
	case leitKandidat:
		return "kandidat"
	case leitLeiter:
		return "leiter"
	}
	return "folger"
}

// LeitUmgebung verbindet die Logik mit dem Knoten. Alle Felder duerfen in
// Tests durch Attrappen ersetzt werden.
type LeitUmgebung struct {
	Hoehe      func() int64       // eigene Kettenhoehe
	HatBlock   func(string) bool  // Block vorhanden UND nachgespielt
	Entleert   func() bool        // nichts mehr in Annahme, WAL, Ausgangskorb
	LetzterBlk func() string      // Hash des letzten eigenen Blocks
	Speichern  func(LeitSpeicher) // dauerhaft, VOR jeder Antwort
	// Ueberholt: dieser Knoten war Leiter und hat die Leitung NICHT durch
	// eigene Uebergabe verloren. Was er angenommen, aber noch nicht in
	// Bloecken verteilt hat, kennt der neue Leiter nicht.
	Ueberholt func(term uint64)
}

// LeitSpeicher: was einen Neustart ueberleben muss. Ohne votedFor koennte ein
// Knoten nach einem Neustart in derselben Amtszeit zweimal abstimmen.
type LeitSpeicher struct {
	Term      uint64 `json:"term"`
	Stimme    string `json:"stimme"`     // gewaehlt in Term
	Leiter    string `json:"leiter"`     // bekannter Leiter von Term
	WarLeiter bool   `json:"war_leiter"` // dieser Knoten leitete Term
}

// Leitung ist die Zustandsmaschine eines Knotens.
type Leitung struct {
	mu   sync.Mutex
	cfg  LeitKonfig
	ich  string
	url  string
	satz []string // sortierte Validator-Adressen (alle, mit Stimmrecht)
	hash string
	env  LeitUmgebung

	ichFaehig bool
	faehig    map[string]bool   // angekuendigte Leiterfaehigkeit
	urls      map[string]string // Adresse -> URL, aus signierten Nachrichten

	term      uint64
	stimme    string
	rolle     leitRolle
	leiter    string
	warLeiter bool

	letzteLease time.Time            // Folger: letzte gueltige Lease
	acks        map[string]time.Time // Leiter: Folger -> Sendezeit der bestaetigten Lease
	gehoert     map[string]time.Time // wann zuletzt irgendeine Nachricht kam
	stimmen     map[string]bool
	vorStimmen  map[string]bool // laufende Vorwahl
	vorwahlSeit time.Time
	kandSeit    time.Time
	leiterSeit  time.Time
	letzterTakt time.Time

	// Planmaessiger Wechsel, Seite des alten Leiters.
	abschliessen bool
	uebergabeAn  string
	// Seite des neuen: empfangene Uebergabe, wartet auf den Block.
	wartet *LeitNachricht
	// Beleg fuer die eigenen Leases nach Uebernahme.
	beleg *LeitNachricht
	// Alter Leiter: Uebergabe, die wiederholt wird, bis der Neue sich meldet.
	// Geht sie verloren, stuende das Netz bei zwei Validatoren sonst still.
	offeneUebergabe *LeitNachricht
	// Frischer Knoten (nichts gespeichert) bei zwei Validatoren: nimmt erst
	// an, wenn er vom anderen gehoert hat, dass der keinen hoeheren Term
	// kennt. Sonst erklaerte sich ein Knoten mit geloeschter Datenbank zum
	// Leiter von Term 1, waehrend der andere laengst Term 7 leitet.
	frischZwei bool

	gestartet time.Time
}

// satzHashVon: Validatoren, die sich in der Zusammensetzung nicht einig
// sind, zaehlen verschiedene Mehrheiten -- sie duerfen einander nicht
// zuhoeren.
func satzHashVon(satz []string) string {
	h := sha256.Sum256([]byte(strings.Join(satz, ",")))
	return hex.EncodeToString(h[:8])
}

func normSatz(satz []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range satz {
		a = strings.ToLower(strings.TrimSpace(a))
		if a != "" && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	sort.Strings(out)
	return out
}

// NeueLeitung baut die Logik. startLeiter leitet Term 1, solange nichts
// gespeichert ist -- auf allen Knoten gleich eingestellt.
func NeueLeitung(ich, url string, satz []string, startLeiter string, faehig bool,
	gespeichert LeitSpeicher, cfg LeitKonfig, env LeitUmgebung, jetzt time.Time) *Leitung {
	l := &Leitung{
		cfg: cfg, ich: strings.ToLower(ich), url: url, env: env,
		satz: normSatz(satz), ichFaehig: faehig,
		faehig: map[string]bool{}, urls: map[string]string{},
		acks: map[string]time.Time{}, stimmen: map[string]bool{}, gehoert: map[string]time.Time{},
		gestartet: jetzt, letzteLease: jetzt,
	}
	l.hash = satzHashVon(l.satz)
	l.faehig[l.ich] = faehig
	if url != "" {
		l.urls[l.ich] = url
	}
	startLeiter = strings.ToLower(strings.TrimSpace(startLeiter))
	if gespeichert.Term == 0 {
		l.frischZwei = len(l.satz) == 2
		l.term = 1
		l.leiter = startLeiter
		if startLeiter != "" {
			l.faehig[startLeiter] = true
		}
	} else {
		l.term = gespeichert.Term
		l.stimme = gespeichert.Stimme
		l.leiter = gespeichert.Leiter
		l.warLeiter = gespeichert.WarLeiter
	}
	if l.leiter == l.ich && l.ichFaehig && l.imSatz(l.ich) {
		// Wieder aufnehmen. Bei drei und mehr nimmt er trotzdem erst an,
		// wenn eine Mehrheit seine Lease bestaetigt hat (DarfAnnehmen);
		// wer inzwischen einen neueren Term kennt, weist sie ab.
		l.rolle = leitLeiter
		l.leiterSeit = jetzt
		l.warLeiter = true
	}
	return l
}

func (l *Leitung) imSatz(a string) bool {
	i := sort.SearchStrings(l.satz, a)
	return i < len(l.satz) && l.satz[i] == a
}

func (l *Leitung) mehrheit() int { return len(l.satz)/2 + 1 }

// mitWahl: ab drei Validatoren gibt es Wahl, Lease und Uebernahme.
func (l *Leitung) mitWahl() bool { return len(l.satz) >= 3 }

func (l *Leitung) speichern() {
	if l.env.Speichern != nil {
		l.env.Speichern(LeitSpeicher{Term: l.term, Stimme: l.stimme, Leiter: l.leiter, WarLeiter: l.warLeiter})
	}
}

func (l *Leitung) hoehe() int64 {
	if l.env.Hoehe == nil {
		return 0
	}
	return l.env.Hoehe()
}

// DarfAnnehmen: nimmt dieser Knoten JETZT Ueberweisungen an?
func (l *Leitung) DarfAnnehmen(jetzt time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.darfAnnehmen(jetzt)
}

func (l *Leitung) darfAnnehmen(jetzt time.Time) bool {
	if l.rolle != leitLeiter || l.abschliessen || !l.ichFaehig || l.frischZwei {
		return false
	}
	if len(l.satz) <= 1 {
		return true
	}
	if !l.mitWahl() {
		return true // zwei Validatoren: siehe Kopf
	}
	// Mehrheit (ich eingeschlossen) hat eine Lease bestaetigt, die ich
	// hoechstens LeaseDauer vor jetzt ABGESCHICKT habe.
	n := 1
	for _, gesendet := range l.acks {
		if jetzt.Sub(gesendet) < l.cfg.LeaseDauer {
			n++
		}
	}
	return n >= l.mehrheit()
}

// Leiter liefert Adresse und URL des bekannten Leiters (leer, wenn keiner).
func (l *Leitung) Leiter() (string, string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.leiter, l.urls[l.leiter]
}

// Term liefert die aktuelle Amtszeit.
func (l *Leitung) Term() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.term
}

// URLs liefert die bekannten Adressen der anderen Validatoren.
func (l *Leitung) URLs() map[string]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := map[string]string{}
	for a, u := range l.urls {
		if a != l.ich {
			out[a] = u
		}
	}
	return out
}

// SetzeFaehig: der eigene Leistungsnachweis hat sich geaendert.
func (l *Leitung) SetzeFaehig(f bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ichFaehig = f
	l.faehig[l.ich] = f
}

// SetzeURL fuer einen Validator aus der Konfiguration.
func (l *Leitung) SetzeURL(addr, url string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if a := strings.ToLower(addr); url != "" && l.imSatz(a) {
		l.urls[a] = url
	}
}

func (l *Leitung) basis(art string, jetzt time.Time) LeitNachricht {
	return LeitNachricht{Art: art, Term: l.term, Von: l.ich, URL: l.url, ZeitMs: jetzt.UnixMilli(),
		Hoehe: l.hoehe(), Faehig: l.ichFaehig, SatzHash: l.hash}
}

// nachfolger: der naechste leiterfaehige Validator nach addr, in fester
// Reihenfolge. Leer, wenn es keinen anderen gibt.
func (l *Leitung) nachfolger(addr string) string {
	var kandidaten []string
	for _, a := range l.satz {
		if l.faehig[a] {
			kandidaten = append(kandidaten, a)
		}
	}
	if len(kandidaten) == 0 {
		return ""
	}
	i := sort.SearchStrings(kandidaten, addr)
	if i < len(kandidaten) && kandidaten[i] == addr {
		i++
	}
	n := kandidaten[i%len(kandidaten)]
	if n == addr {
		return ""
	}
	return n
}

// rang: wie viele leiterfaehige Validatoren nach dem ausgefallenen Leiter
// vor diesem Knoten an der Reihe sind. Gestaffelte Kandidatur verhindert,
// dass alle gleichzeitig antreten und sich die Stimmen teilen.
func (l *Leitung) rang() int {
	a := l.leiter
	for r := 0; r < len(l.satz); r++ {
		a = l.nachfolger(a)
		if a == "" {
			return len(l.satz)
		}
		if a == l.ich {
			return r
		}
	}
	return len(l.satz)
}

func (l *Leitung) werdeFolger(term uint64, leiter string, jetzt time.Time) {
	if l.rolle == leitLeiter && term > l.term && l.warLeiter && l.env.Ueberholt != nil {
		// Leitung verloren, ohne sie selbst zu uebergeben.
		defer l.env.Ueberholt(l.term)
	}
	if term > l.term {
		l.term = term
		l.stimme = ""
		l.warLeiter = false
	}
	l.rolle = leitFolger
	l.leiter = leiter
	l.letzteLease = jetzt
	l.abschliessen = false
	l.uebergabeAn = ""
	l.acks = map[string]time.Time{}
	l.stimmen = map[string]bool{}
	l.beleg = nil
	l.speichern()
}

func (l *Leitung) werdeLeiter(jetzt time.Time, beleg *LeitNachricht) {
	l.letzterTakt = time.Time{} // erste Lease sofort
	l.offeneUebergabe = nil
	l.rolle = leitLeiter
	l.leiter = l.ich
	l.warLeiter = true
	l.leiterSeit = jetzt
	l.acks = map[string]time.Time{}
	l.stimmen = map[string]bool{}
	l.abschliessen = false
	l.beleg = beleg
	l.wartet = nil
	l.speichern()
}

// Takt: regelmaessig aufrufen (alle paar hundert Millisekunden). Liefert
// die zu versendenden Nachrichten.
func (l *Leitung) Takt(jetzt time.Time) []LeitNachricht {
	l.mu.Lock()
	defer l.mu.Unlock()
	var raus []LeitNachricht

	// Neuer Leiter nach Uebergabe: sobald der Block da ist, uebernehmen.
	if l.wartet != nil && l.wartet.Term == l.term {
		if l.env.HatBlock == nil || l.wartet.BlockHash == "" || l.env.HatBlock(l.wartet.BlockHash) {
			beleg := *l.wartet
			l.term++
			l.stimme = l.ich
			l.werdeLeiter(jetzt, &beleg)
		}
	}

	switch l.rolle {
	case leitLeiter:
		// Planmaessiger Wechsel?
		if !l.abschliessen && l.cfg.WechselAlle > 0 && (l.mitWahl() || l.cfg.ZweiWechseln) &&
			jetzt.Sub(l.leiterSeit) >= l.cfg.WechselAlle {
			if n := l.nachfolger(l.ich); n != "" && l.lebt(n, jetzt) {
				l.abschliessen = true
				l.uebergabeAn = n
			}
		}
		if l.abschliessen && (l.env.Entleert == nil || l.env.Entleert()) {
			ue := l.basis(leitArtUebergabe, jetzt)
			ue.An = l.uebergabeAn
			if l.env.LetzterBlk != nil {
				ue.BlockHash = l.env.LetzterBlk()
			}
			raus = append(raus, ue)
			// Ab hier ist der Naechste Leiter von term+1 -- sobald er den
			// Block hat. Dieser Knoten folgt und wartet auf dessen Lease.
			// Er hat SELBST uebergeben: nicht "ueberholt".
			naechster := l.uebergabeAn
			l.warLeiter = false
			l.rolle = leitFolger
			l.abschliessen = false
			l.uebergabeAn = ""
			l.term++
			l.stimme = naechster
			l.leiter = naechster
			l.letzteLease = jetzt
			l.acks = map[string]time.Time{}
			k := ue
			l.offeneUebergabe = &k
			l.letzterTakt = jetzt
			l.speichern()
			return raus
		}
		if jetzt.Sub(l.letzterTakt) >= l.cfg.Takt {
			l.letzterTakt = jetzt
			m := l.basis(leitArtLease, jetzt)
			m.Uebergabe = l.beleg
			raus = append(raus, m)
		}
	case leitFolger:
		// Uebergabe wiederholen, bis der Neue seine erste Lease schickt.
		if l.offeneUebergabe != nil && jetzt.Sub(l.letzterTakt) >= l.cfg.Takt {
			l.letzterTakt = jetzt
			raus = append(raus, *l.offeneUebergabe)
		}
		if !l.mitWahl() || !l.ichFaehig || !l.imSatz(l.ich) {
			break
		}
		if l.vorStimmen != nil && len(l.vorStimmen) >= l.mehrheit() {
			raus = append(raus, l.kandidieren(jetzt)...)
			break
		}
		frist := l.cfg.FolgerFrist + time.Duration(l.rang())*l.cfg.Staffel
		if jetzt.Sub(l.letzteLease) >= frist &&
			(l.vorStimmen == nil || jetzt.Sub(l.vorwahlSeit) >= l.cfg.FolgerFrist) {
			raus = append(raus, l.vorwahl(jetzt))
		}
	case leitKandidat:
		// Keine Mehrheit in angemessener Zeit: zurueck in die Vorwahl, nicht
		// gleich den naechsten Term -- sonst zaehlt ein abgeschnittener
		// Kandidat doch wieder hoch.
		if jetzt.Sub(l.kandSeit) >= l.cfg.FolgerFrist {
			l.rolle = leitFolger
			l.vorStimmen = nil
			l.stimmen = map[string]bool{}
			// Neu anstellen, GESTAFFELT nach Rang: sonst traten zwei, die sich
			// die Stimmen geteilt hatten, im selben Takt wieder an, teilten sie
			// wieder -- endlos (gemessen: Term 6, kein Leiter, ueber HTTP).
			// Der Rang ist je Knoten verschieden, also laufen sie auseinander.
			l.letzteLease = jetzt
		}
	}
	return raus
}

func (l *Leitung) lebt(addr string, jetzt time.Time) bool {
	if l.mitWahl() {
		t, ok := l.acks[addr]
		return ok && jetzt.Sub(t) < l.cfg.LeaseDauer
	}
	t, ok := l.gehoert[addr]
	return ok && jetzt.Sub(t) < l.cfg.LeaseDauer
}

// vorwahl (PreVote, wie in Raft): erst fragen, ob eine Mehrheit waehlen
// WUERDE, und nur dann den Term hochzaehlen. Ohne sie zaehlt ein Knoten, der
// kurz abgeschnitten war, seinen Term immer weiter hoch -- und setzt nach
// seiner Rueckkehr einen gesunden Leiter ab, einfach weil sein Term hoeher
// ist. Das Netz stockte dann ohne jeden Grund.
func (l *Leitung) vorwahl(jetzt time.Time) LeitNachricht {
	l.vorStimmen = map[string]bool{l.ich: true}
	l.vorwahlSeit = jetzt
	m := l.basis(leitArtStimmeBitte, jetzt)
	m.Term = l.term + 1
	m.Vorwahl = true
	return m
}

func (l *Leitung) kandidieren(jetzt time.Time) []LeitNachricht {
	l.vorStimmen = nil
	l.term++
	l.rolle = leitKandidat
	l.stimme = l.ich
	l.leiter = ""
	l.kandSeit = jetzt
	l.stimmen = map[string]bool{l.ich: true}
	l.speichern()
	return []LeitNachricht{l.basis(leitArtStimmeBitte, jetzt)}
}

// Empfange verarbeitet eine GEPRUEFTE Nachricht (Signatur, Absender im
// Satz). Liefert die Antwort (oder nil).
func (l *Leitung) Empfange(m LeitNachricht, jetzt time.Time) *LeitNachricht {
	l.mu.Lock()
	defer l.mu.Unlock()
	if m.SatzHash != l.hash || !l.imSatz(m.Von) || m.Von == l.ich {
		return nil
	}
	if m.URL != "" {
		l.urls[m.Von] = m.URL
	}
	l.faehig[m.Von] = m.Faehig
	l.gehoert[m.Von] = jetzt
	if l.frischZwei && m.Term <= l.term {
		l.frischZwei = false
	}

	switch m.Art {
	case leitArtLease:
		return l.empfangeLease(m, jetzt)
	case leitArtLeaseAck:
		if l.rolle == leitLeiter && m.Term == l.term && m.Gewaehrt {
			gesendet := time.UnixMilli(m.ZeitMs)
			if alt, ok := l.acks[m.Von]; !ok || gesendet.After(alt) {
				l.acks[m.Von] = gesendet
			}
		} else if m.Term > l.term {
			l.werdeFolger(m.Term, "", jetzt)
		}
		return nil
	case leitArtStimmeBitte:
		return l.empfangeStimmeBitte(m, jetzt)
	case leitArtStimme:
		if m.Term > l.term && !m.Vorwahl {
			l.werdeFolger(m.Term, "", jetzt)
			return nil
		}
		if m.Vorwahl {
			// Antwort auf die Vorwahl: gilt fuer term+1, solange nichts
			// dazwischenkam.
			if l.rolle == leitFolger && l.vorStimmen != nil && m.Term == l.term+1 && m.Gewaehrt {
				l.vorStimmen[m.Von] = true
				if len(l.vorStimmen) >= l.mehrheit() {
					return nil // Kandidatur im naechsten Takt, siehe unten
				}
			}
			return nil
		}
		if l.rolle == leitKandidat && m.Term == l.term && m.Gewaehrt {
			l.stimmen[m.Von] = true
			if len(l.stimmen) >= l.mehrheit() {
				l.werdeLeiter(jetzt, nil)
			}
		}
		return nil
	case leitArtUebergabe:
		// Nur vom Leiter des laufenden Terms, nur an mich. Der Absender hat
		// seinen Term schon hochgezaehlt, als er schickte -- sein m.Term ist
		// der ALTE (basis vor dem Hochzaehlen gebaut).
		if m.An == l.ich && m.Term == l.term && m.Von == l.leiter && l.ichFaehig {
			k := m
			l.wartet = &k
		}
		return nil
	}
	return nil
}

func (l *Leitung) empfangeLease(m LeitNachricht, jetzt time.Time) *LeitNachricht {
	ack := l.basis(leitArtLeaseAck, jetzt)
	// Der Leiter rechnet mit SEINER Sendezeit.
	ack.ZeitMs = m.ZeitMs
	if m.Term < l.term {
		// Ueberholter Leiter: er erfaehrt den neuen Term aus der Antwort.
		ack.Gewaehrt = false
		return &ack
	}
	if m.Term == l.term && l.leiter != "" && l.leiter != m.Von {
		// Zwei Leiter in einem Term darf es nicht geben; nicht bestaetigen.
		ack.Gewaehrt = false
		return &ack
	}
	if m.Term > l.term || l.leiter == "" || l.rolle != leitFolger {
		if l.rolle == leitLeiter && m.Term == l.term {
			ack.Gewaehrt = false
			return &ack
		}
		l.werdeFolger(m.Term, m.Von, jetzt)
	}
	l.letzteLease = jetzt
	l.leiter = m.Von
	if l.offeneUebergabe != nil && m.Term > l.offeneUebergabe.Term {
		l.offeneUebergabe = nil // der Neue hat uebernommen
	}
	ack.Term = l.term
	ack.Gewaehrt = true
	return &ack
}

func (l *Leitung) empfangeStimmeBitte(m LeitNachricht, jetzt time.Time) *LeitNachricht {
	antw := l.basis(leitArtStimme, jetzt)
	antw.An = m.Von
	if m.Vorwahl {
		// Keinerlei Zustandsaenderung: dieselben Bedingungen wie bei der
		// echten Wahl, nur als Auskunft.
		antw.Vorwahl = true
		antw.Term = m.Term
		leaseFrisch := l.rolle == leitFolger && l.leiter != "" && jetzt.Sub(l.letzteLease) < l.cfg.FolgerFrist
		antw.Gewaehrt = m.Term > l.term && !l.darfAnnehmen(jetzt) && !leaseFrisch &&
			m.Faehig && m.Hoehe >= l.hoehe()-l.cfg.HoeheToleranz
		return &antw
	}
	if m.Term < l.term {
		return &antw
	}
	// Solange ein Leiter lebt (Lease frisch), stimmt ein Folger fuer
	// niemand anderen -- die Lease-Zusage. Auch ein Leiter stimmt nicht.
	if l.rolle == leitLeiter || (l.rolle == leitFolger && l.leiter != "" && jetzt.Sub(l.letzteLease) < l.cfg.FolgerFrist) {
		return &antw
	}
	if !m.Faehig || m.Hoehe < l.hoehe()-l.cfg.HoeheToleranz {
		return &antw
	}
	if m.Term > l.term {
		l.term = m.Term
		l.stimme = ""
		l.rolle = leitFolger
		l.leiter = ""
	}
	if l.stimme == "" || l.stimme == m.Von {
		l.stimme = m.Von
		l.letzteLease = jetzt // nicht selbst sofort antreten
		l.speichern()
		antw.Term = l.term
		antw.Gewaehrt = true
	}
	return &antw
}

// Stand fuer /api/health/combined.
func (l *Leitung) Stand(jetzt time.Time) map[string]interface{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	return map[string]interface{}{
		"term":             l.term,
		"rolle":            l.rolle.String(),
		"leiter":           l.leiter,
		"leiter_url":       l.urls[l.leiter],
		"nimmt_an":         l.darfAnnehmen(jetzt),
		"validatoren":      len(l.satz),
		"mit_wahl":         l.mitWahl(),
		"leiterfaehig":     l.ichFaehig,
		"uebergabe_laeuft": l.abschliessen || l.offeneUebergabe != nil || l.wartet != nil,
		"satz_hash":        l.hash,
	}
}
