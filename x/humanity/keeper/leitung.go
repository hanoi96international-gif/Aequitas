package keeper

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
// # WER MITGLIED IST: OHNE HANDLISTE
//
// Die Validatoren, deren Mehrheit zaehlt (der Satz), pflegt niemand von
// Hand. Der Satz beginnt beim Genesis-Satz -- wie jede Kette mit einer
// Genesis beginnt -- und aendert sich danach nur ueber die Leitung selbst:
//
//   - Wer Validator werden will, registriert seinen Signierschluessel,
//     gebunden an einen registrierten Menschen (register-validator-key), und
//     meldet sich (Hallo). Der amtierende Leiter nimmt ihn auf. Niemand muss
//     zustimmen, niemand kann es verbieten.
//   - Wer laenger als EntfernenNach nichts von sich hoeren laesst, wird
//     entfernt -- sonst wuerden ausgefallene Validatoren irgendwann die
//     Mehrheit unerreichbar machen. Meldet er sich wieder, kommt er wieder
//     hinein.
//
// Damit dabei nie zwei Mehrheiten entstehen, gelten die Regeln, mit denen
// Raft seine Mitgliedschaft aendert (Ongaro, Dissertation Kap. 4, mit der
// Korrektur von 2015):
//   - je Aenderung genau EIN Validator mehr oder weniger: jede Mehrheit des
//     alten und jede des neuen Satzes haben einen Knoten gemeinsam;
//   - die naechste Aenderung erst, wenn die Mehrheit die vorige bestaetigt
//     hat (fest), und erst, wenn der Leiter in SEINER Amtszeit bestaetigt
//     wurde (der Stempel SatzTerm, entspricht Rafts No-op);
//   - jeder Knoten benutzt sofort den neuesten Satz, den er kennt;
//   - gewaehlt wird nur, wer einen mindestens so neuen Satz hat wie der
//     Waehler (Stempel, dann Version) -- ein bestaetigter Satz geht nie
//     verloren;
//   - Folger uebernehmen den Satz des Leiters, dessen Lease sie annehmen.
// Eine Aenderung, die nicht binnen AufnahmeFrist bestaetigt wird (der Neue
// meldet sich nicht mehr), nimmt der Leiter zurueck -- wieder genau ein
// Schritt.
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
	leitArtHallo       = "hallo"        // wer (noch) keine Lease bekommt -> alle
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
	// Stand des Satzes des Absenders: Stempel (Amtszeit, in der ein Leiter ihn
	// zuletzt bestaetigt hat) und Version. Siehe "Wer Mitglied ist".
	SatzTerm    uint64 `json:"satz_term"`
	SatzVersion uint64 `json:"satz_version"`
	// Nur in Quittungen: Mitglieder, von denen der Folger in der letzten
	// halben EntfernenNach gehoert hat. Der Leiter entfernt niemanden, den
	// ein Folger noch hoert.
	Lebend []string `json:"lebend,omitempty"`
	// Nur in Leases: der Satz selbst und die bekannten Adressen der
	// Mitglieder -- so erfaehrt jeder Folger, wen er bei einer Wahl fragt.
	Satz []string          `json:"satz,omitempty"`
	URLs map[string]string `json:"urls,omitempty"`
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
	// Mitgliedschaft: wer so lange nichts hoeren laesst, wird entfernt.
	EntfernenNach time.Duration
	// So lange hat eine Aenderung Zeit, bestaetigt zu werden.
	AufnahmeFrist time.Duration
	// So weit darf ein Neuer hinter dem Leiter liegen, um aufgenommen zu werden.
	AufnahmeToleranz int64
	// Zurueckgenommene Aufnahme: so lange nicht erneut versuchen.
	AufnahmeSperre time.Duration
	// So oft meldet sich jedes Mitglied bei allen anderen (Hallo) -- damit
	// jeder selbst beurteilen kann, ob ein Validator noch lebt.
	LebenszeichenAlle time.Duration
}

func leitVorgabe() LeitKonfig {
	return LeitKonfig{
		Takt:          time.Second,
		LeaseDauer:    6 * time.Second,
		FolgerFrist:   12 * time.Second,
		Staffel:       4 * time.Second,
		WechselAlle:   10 * time.Minute,
		HoeheToleranz: 2,

		EntfernenNach:    30 * time.Minute,
		AufnahmeFrist:    30 * time.Second,
		AufnahmeToleranz: 50,
		AufnahmeSperre:   10 * time.Minute,

		LebenszeichenAlle: 10 * time.Second,
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
	// Zugelassen: registrierter Validator (Signierschluessel an einen
	// registrierten Menschen gebunden). nil = alle (Tests).
	Zugelassen func(addr string) bool
	// Mensch: an welchen registrierten Menschen ist dieser Schluessel
	// gebunden? "" = unbekannt. Ein Mensch, eine Stimme: pro Mensch hoechstens
	// ein Mitglied. nil = jeder Schluessel ist sein eigener Mensch (Tests).
	Mensch func(addr string) string
	// Unbekannt: ein Leiter hat einen Validator aufgenommen, den dieser Knoten
	// (noch) nicht kennt -- Register bei den Peers nachfragen.
	Unbekannt func(addr string)
}

// LeitSpeicher: was einen Neustart ueberleben muss. Ohne votedFor koennte ein
// Knoten nach einem Neustart in derselben Amtszeit zweimal abstimmen.
type LeitSpeicher struct {
	Term      uint64 `json:"term"`
	Stimme    string `json:"stimme"`     // gewaehlt in Term
	Leiter    string `json:"leiter"`     // bekannter Leiter von Term
	WarLeiter bool   `json:"war_leiter"` // dieser Knoten leitete Term

	// Der Satz. Ohne ihn zaehlte ein Knoten nach einem Neustart wieder die
	// Mehrheit des Genesis-Satzes.
	Satz     *LeitSatz         `json:"satz,omitempty"`
	SatzFest bool              `json:"satz_fest,omitempty"`
	Vorher   *LeitSatz         `json:"vorher,omitempty"` // bestaetigter Vorgaenger einer offenen Aenderung
	URLs     map[string]string `json:"urls,omitempty"`
}

// LeitSatz: die Validatoren, deren Mehrheit zaehlt, mit Stand.
type LeitSatz struct {
	Mitglieder []string `json:"mitglieder"`
	Term       uint64   `json:"term"`
	Version    uint64   `json:"version"`
}

// neuerAls: zuerst der Stempel, dann die Version (wie Rafts "Log mindestens
// so aktuell": erst Term, dann Index).
func satzNeuerAls(t1, v1, t2, v2 uint64) bool {
	if t1 != t2 {
		return t1 > t2
	}
	return v1 > v2
}

type bewerberInfo struct {
	zeit   time.Time
	hoehe  int64
	faehig bool
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

	// Mitgliedschaft (siehe Kopf).
	satzTerm        uint64
	satzVersion     uint64
	fest            bool     // Satz von einer Mehrheit bestaetigt
	vorher          []string // bestaetigter Vorgaenger, solange eine Aenderung offen ist
	vorherTerm      uint64
	vorherVersion   uint64
	vorschlagSeit   time.Time
	bewerber        map[string]bewerberInfo
	gesperrt        map[string]time.Time // zurueckgenommene Aufnahmen
	letzteAenderung string

	term      uint64
	stimme    string
	rolle     leitRolle
	leiter    string
	warLeiter bool

	letzteLease  time.Time            // Folger: letzte gueltige Lease
	acks         map[string]time.Time // Leiter: Folger -> Sendezeit der bestaetigten Lease
	ackSatz      map[string]string    // ... und mit welchem Satz quittiert
	lebendBei    map[string][]string  // ... und wen der Folger noch hoert
	gehoert      map[string]time.Time // wann zuletzt irgendeine Nachricht kam
	stimmen      map[string]bool
	vorStimmen   map[string]bool // laufende Vorwahl
	vorwahlSeit  time.Time
	kandSeit     time.Time
	leiterSeit   time.Time
	letzterTakt  time.Time
	letzterHallo time.Time

	// Folger: der Satz, den der Leiter gesehenVon in gesehenTerm zuletzt
	// (glaubwuerdig) gezeigt hat.
	gesehenSatz []string
	gesehenVon  string
	gesehenTerm uint64

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
func satzHashVon(term, version uint64, satz []string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d/%d/%s", term, version, strings.Join(satz, ","))))
	return hex.EncodeToString(h[:8])
}

// setzeSatz: neuer Satz (schon normalisiert oder nicht). Bestaetigungen von
// Nicht-Mitgliedern verfallen.
func (l *Leitung) setzeSatz(satz []string, term, version uint64) {
	l.satz = normSatz(satz)
	l.satzTerm, l.satzVersion = term, version
	l.hash = satzHashVon(term, version, l.satz)
	for a := range l.acks {
		if !l.imSatz(a) {
			delete(l.acks, a)
		}
	}
}

func (l *Leitung) speicherSatz() (*LeitSatz, *LeitSatz) {
	akt := &LeitSatz{Mitglieder: append([]string(nil), l.satz...), Term: l.satzTerm, Version: l.satzVersion}
	var vor *LeitSatz
	if l.vorher != nil {
		vor = &LeitSatz{Mitglieder: append([]string(nil), l.vorher...), Term: l.vorherTerm, Version: l.vorherVersion}
	}
	return akt, vor
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
		ichFaehig: faehig,
		faehig:    map[string]bool{}, urls: map[string]string{},
		acks: map[string]time.Time{}, ackSatz: map[string]string{}, lebendBei: map[string][]string{}, stimmen: map[string]bool{}, gehoert: map[string]time.Time{},
		bewerber: map[string]bewerberInfo{}, gesperrt: map[string]time.Time{},
		gestartet: jetzt, letzteLease: jetzt,
	}
	if gespeichert.Satz != nil {
		l.setzeSatz(gespeichert.Satz.Mitglieder, gespeichert.Satz.Term, gespeichert.Satz.Version)
		l.fest = gespeichert.SatzFest
		if v := gespeichert.Vorher; v != nil {
			l.vorher, l.vorherTerm, l.vorherVersion = normSatz(v.Mitglieder), v.Term, v.Version
			l.vorschlagSeit = jetzt
		}
		for a, u := range gespeichert.URLs {
			l.urls[a] = u
		}
	} else {
		// Genesis: bestaetigt per Definition.
		l.setzeSatz(satz, 0, 0)
		l.fest = true
	}
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
	if l.leiter == l.ich && l.imSatz(l.ich) {
		// Wieder aufnehmen -- auch ohne Leistungsnachweis: dann gibt er ab,
		// sobald ein leiterfaehiger lebt (Takt). Bei zwei Validatoren stuende
		// das Netz sonst ganz. Bei drei und mehr nimmt er trotzdem erst an,
		// wenn eine Mehrheit seine Lease bestaetigt hat (DarfAnnehmen);
		// wer inzwischen einen neueren Term kennt, weist sie ab.
		l.rolle = leitLeiter
		l.leiterSeit = jetzt
		l.warLeiter = true
		// Stempel dieser Amtszeit (siehe werdeLeiter). Mitglieder unveraendert.
		l.setzeSatz(l.satz, l.term, l.satzVersion)
		if len(l.satz) <= 1 {
			l.fest = true
		}
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
		akt, vor := l.speicherSatz()
		urls := map[string]string{}
		for a, u := range l.urls {
			if l.imSatz(a) {
				urls[a] = u
			}
		}
		l.env.Speichern(LeitSpeicher{Term: l.term, Stimme: l.stimme, Leiter: l.leiter, WarLeiter: l.warLeiter,
			Satz: akt, SatzFest: l.fest, Vorher: vor, URLs: urls})
	}
}

func (l *Leitung) zugelassen(addr string) bool {
	return l.env.Zugelassen == nil || l.env.Zugelassen(addr)
}

func (l *Leitung) mensch(addr string) string {
	if l.env.Mensch == nil {
		return addr
	}
	return l.env.Mensch(addr)
}

// aufnehmbar: registriert, an einen bekannten Menschen gebunden, und dieser
// Mensch hat im Satz noch kein anderes Mitglied.
func (l *Leitung) aufnehmbar(a string, satz []string) bool {
	if !l.zugelassen(a) {
		return false
	}
	m := l.mensch(a)
	if m == "" {
		return false
	}
	for _, b := range satz {
		if b != a && l.mensch(b) == m {
			return false
		}
	}
	return true
}

// kuerzlichGehoert: hat dieser Knoten in der letzten halben EntfernenNach von
// a gehoert? (Seit dem eigenen Start: wer eben erst gestartet ist, weiss es
// nicht und gibt keine Auskunft.)
func (l *Leitung) kuerzlichGehoert(a string, jetzt time.Time) bool {
	halb := l.cfg.EntfernenNach / 2
	zuletzt := l.gehoert[a]
	if zuletzt.Before(l.gestartet) {
		zuletzt = l.gestartet
	}
	return jetzt.Sub(zuletzt) < halb
}

// satzwechselPlausibel: prueft ein Folger, bevor er den Satz eines Leiters
// uebernimmt. Ein Leiter, der luegt, soll so weder Unbekannte oder einen
// Menschen doppelt aufnehmen noch Lebende hinauswerfen koennen: uebernimmt
// eine Mehrheit den Satz nicht, gilt er nie als bestaetigt, und der Leiter
// nimmt ihn zurueck.
//
// Verglichen wird mit dem Satz, den DIESER Leiter in DIESER Amtszeit zuletzt
// gezeigt hat -- nicht mit dem eigenen: der kann nach einer Trennung aus
// einem anderen, nie bestaetigten Zweig stammen, und dann lehnte der Folger
// jeden Satz ab, und die Mitgliedschaft kaeme nie wieder zusammen.
func (l *Leitung) satzwechselPlausibel(m LeitNachricht, jetzt time.Time) bool {
	neu := normSatz(m.Satz)
	vorher := l.gesehenSatz
	if l.gesehenVon != m.Von || l.gesehenTerm != m.Term {
		vorher = nil
	}
	for _, a := range neu {
		if l.imSatz(a) || l.enthaelt(vorher, a) {
			continue
		}
		if !l.aufnehmbar(a, neu) {
			if l.env.Unbekannt != nil && (!l.zugelassen(a) || l.mensch(a) == "") {
				l.env.Unbekannt(a)
			}
			return false
		}
	}
	for _, a := range vorher {
		if l.enthaelt(neu, a) {
			continue
		}
		// Sich selbst hinauswerfen lassen, obwohl man die Lease gerade
		// bekommt? Nein -- fuer die Bestaetigung zaehlt man dann ohnehin
		// nicht mehr; und wer wirklich nur einseitig abgeschnitten ist, wird
		// nach seinem naechsten Lebenszeichen wieder aufgenommen.
		if a == l.ich || l.kuerzlichGehoert(a, jetzt) {
			return false
		}
	}
	return true
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
	// Der Leistungsnachweis entscheidet, wer Leiter WIRD, nicht, ob ein
	// amtierender annimmt: haelt er ihn nicht mehr, uebergibt er (Takt) --
	// bis dahin nimmt er weiter an, sonst stuende das Netz.
	if l.rolle != leitLeiter || l.abschliessen || l.frischZwei || !l.imSatz(l.ich) {
		return false
	}
	if len(l.satz) <= 1 {
		return true
	}
	if !l.mitWahl() {
		// Zwei Validatoren, keine Wahl (siehe Kopf): annehmen ohne
		// Bestaetigung -- aber nur, wenn feststeht, dass der andere diesen
		// Satz hat (sonst koennte er, noch im alten Satz zu dritt, einen
		// anderen mitwaehlen) oder der Vorgaenger selbst keine Wahl kannte.
		return l.fest || (l.vorher != nil && len(l.vorher) <= 2)
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
		Hoehe: l.hoehe(), Faehig: l.ichFaehig, SatzHash: l.hash,
		SatzTerm: l.satzTerm, SatzVersion: l.satzVersion}
}

// lease: die Lease traegt den Satz und die bekannten Adressen mit.
func (l *Leitung) lease(jetzt time.Time) LeitNachricht {
	m := l.basis(leitArtLease, jetzt)
	m.Satz = append([]string(nil), l.satz...)
	m.URLs = map[string]string{}
	for a, u := range l.urls {
		if l.imSatz(a) {
			m.URLs[a] = u
		}
	}
	return m
}

// faehigerLebt: hat dieser Knoten innerhalb der FolgerFrist von einem
// ANDEREN leiterfaehigen Validator gehoert?
func (l *Leitung) faehigerLebt(jetzt time.Time) bool {
	for a, f := range l.faehig {
		if a == l.ich || !f || !l.imSatz(a) {
			continue
		}
		if t, ok := l.gehoert[a]; ok && jetzt.Sub(t) < l.cfg.FolgerFrist {
			return true
		}
	}
	return false
}

// notbetrieb: weder dieser Knoten noch ein erreichbarer anderer haelt den
// Leistungsnachweis. Dann duerfen alle -- lieber langsam als gar nicht.
// Das beruehrt nur, WER Leiter wird; die Regeln, die zwei Annehmende
// ausschliessen (Lease, Mehrheit), gelten unveraendert.
func (l *Leitung) notbetrieb(jetzt time.Time) bool {
	return !l.ichFaehig && !l.faehigerLebt(jetzt)
}

// waehlbar: Kandidat mit Leistungsnachweis -- oder ohne, wenn auch dieser
// Knoten keinen leiterfaehigen erreicht (Notbetrieb aus SEINER Sicht; so
// gewinnt ein leiterfaehiger, sobald ihn eine Mehrheit sieht). Angekuendigt
// wird immer der ECHTE Nachweis: wuerden Knoten im Notbetrieb sich als
// leiterfaehig ausgeben, hielten die anderen den Notbetrieb fuer beendet,
// die ersten dann auch -- und es kippte hin und her.
//
// Und: sein Satz ist mindestens so neu wie der eigene -- sonst koennte ein
// Kandidat mit einem ueberholten Satz eine Mehrheit zusammenbekommen, die es
// im bestaetigten Satz nicht gibt.
func (l *Leitung) waehlbar(m LeitNachricht, jetzt time.Time) bool {
	return (m.Faehig || l.notbetrieb(jetzt)) && m.Hoehe >= l.hoehe()-l.cfg.HoeheToleranz &&
		!satzNeuerAls(l.satzTerm, l.satzVersion, m.SatzTerm, m.SatzVersion)
}

// effFaehig: darf dieser Knoten JETZT Leiter werden?
func (l *Leitung) effFaehig(jetzt time.Time) bool {
	return l.ichFaehig || l.notbetrieb(jetzt)
}

// nachfolger: der naechste leiterfaehige Validator nach addr, in fester
// Reihenfolge (alle: auch die ohne Nachweis, im Notbetrieb). Leer, wenn es
// keinen anderen gibt.
func (l *Leitung) nachfolger(addr string, alle bool) string {
	var kandidaten []string
	for _, a := range l.satz {
		if alle || l.faehig[a] {
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
func (l *Leitung) rang(jetzt time.Time) int {
	alle := l.notbetrieb(jetzt)
	a := l.leiter
	for r := 0; r < len(l.satz); r++ {
		a = l.nachfolger(a, alle)
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
	l.vorher = nil // eine offene Aenderung entscheidet jetzt der naechste Leiter
	l.fest = false // gilt nur fuer den Leiter, der es festgestellt hat
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
	// Stempel: dieser Satz gilt ab jetzt als "in meiner Amtszeit". Bestaetigt
	// ist er erst, wenn eine Mehrheit ihn mit diesem Stempel quittiert hat.
	// Die Folger uebernehmen ihn aus der Lease.
	l.vorher = nil
	l.setzeSatz(l.satz, l.term, l.satzVersion)
	// Bei zwei steht nach einer Uebergabe fest, dass der andere dieselben
	// Mitglieder hat (er hat sie mit der Uebergabe signiert).
	if len(l.satz) <= 1 || (beleg != nil && !l.mitWahl()) {
		l.fest = true
	}
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
			if n := l.nachfolger(l.ich, l.notbetrieb(jetzt)); n != "" && l.lebt(n, jetzt) {
				l.abschliessen = true
				l.uebergabeAn = n
			}
		}
		// Leistungsnachweis verloren: an einen leiterfaehigen abgeben, sobald
		// einer lebt. Gibt es keinen, bleibt dieser Leiter (Notbetrieb).
		if !l.abschliessen && !l.ichFaehig && (l.mitWahl() || l.cfg.ZweiWechseln) {
			if n := l.nachfolger(l.ich, false); n != "" && l.lebt(n, jetzt) {
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
		l.mitgliedschaft(jetzt)
		if jetzt.Sub(l.letzterTakt) >= l.cfg.Takt {
			l.letzterTakt = jetzt
			m := l.lease(jetzt)
			m.Uebergabe = l.beleg
			raus = append(raus, m)
		}
	case leitFolger:
		// Uebergabe wiederholen, bis der Neue seine erste Lease schickt.
		if l.offeneUebergabe != nil && jetzt.Sub(l.letzterTakt) >= l.cfg.Takt {
			l.letzterTakt = jetzt
			raus = append(raus, *l.offeneUebergabe)
		}
		// Kein Mitglied, oder seit einer Weile keine Lease (etwa nach einem
		// Neustart des Leiters, der die eigene Adresse nicht kennt): melden.
		// Mitglieder ausserdem regelmaessig: Lebenszeichen fuer alle.
		if ((!l.imSatz(l.ich) || jetzt.Sub(l.letzteLease) >= 2*l.cfg.Takt) &&
			jetzt.Sub(l.letzterHallo) >= 2*l.cfg.Takt) ||
			(l.cfg.LebenszeichenAlle > 0 && jetzt.Sub(l.letzterHallo) >= l.cfg.LebenszeichenAlle) {
			l.letzterHallo = jetzt
			raus = append(raus, l.basis(leitArtHallo, jetzt))
		}
		if !l.mitWahl() || !l.effFaehig(jetzt) || !l.imSatz(l.ich) {
			break
		}
		if l.vorStimmen != nil && len(l.vorStimmen) >= l.mehrheit() {
			raus = append(raus, l.kandidieren(jetzt)...)
			break
		}
		frist := l.cfg.FolgerFrist + time.Duration(l.rang(jetzt))*l.cfg.Staffel
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
	if m.Von == l.ich || m.Von == "" {
		return nil
	}
	// Gehoert wird auf Mitglieder und registrierte Validatoren. Den Satz
	// vergleicht jede Nachrichtenart selbst: Leases bringen ihren Satz mit,
	// Stimmen zaehlen nur von Mitgliedern des eigenen, eine Quittung nur mit
	// demselben Satz.
	if !l.imSatz(m.Von) && !l.zugelassen(m.Von) {
		return nil
	}
	if m.URL != "" {
		l.urls[m.Von] = m.URL
	}
	l.faehig[m.Von] = m.Faehig
	l.gehoert[m.Von] = jetzt
	if m.Art == leitArtHallo {
		return l.empfangeHallo(m, jetzt)
	}
	if l.frischZwei && m.Term <= l.term {
		l.frischZwei = false
	}

	switch m.Art {
	case leitArtLease:
		return l.empfangeLease(m, jetzt)
	case leitArtLeaseAck:
		// Die Lease-Zusage zaehlt unabhaengig vom Satz des Folgers (er waehlt
		// niemand anderen, solange sie gilt) -- sonst stuende die Annahme,
		// sobald ein Folger einen Satzwechsel ablehnt. Fuer die Bestaetigung
		// eines Satzes zaehlt nur eine Quittung MIT diesem Satz (quittiert).
		if l.rolle == leitLeiter && m.Term == l.term && m.Gewaehrt && l.imSatz(m.Von) {
			gesendet := time.UnixMilli(m.ZeitMs)
			if alt, ok := l.acks[m.Von]; !ok || gesendet.After(alt) {
				l.acks[m.Von] = gesendet
				l.ackSatz[m.Von] = m.SatzHash
				l.lebendBei[m.Von] = m.Lebend
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
			if l.rolle == leitFolger && l.vorStimmen != nil && m.Term == l.term+1 && m.Gewaehrt && l.imSatz(m.Von) {
				l.vorStimmen[m.Von] = true
				if len(l.vorStimmen) >= l.mehrheit() {
					return nil // Kandidatur im naechsten Takt, siehe unten
				}
			}
			return nil
		}
		if l.rolle == leitKandidat && m.Term == l.term && m.Gewaehrt && l.imSatz(m.Von) {
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
		if m.An == l.ich && m.Term == l.term && m.Von == l.leiter && m.SatzHash == l.hash && l.effFaehig(jetzt) {
			k := m
			l.wartet = &k
		}
		return nil
	}
	return nil
}

func (l *Leitung) empfangeLease(m LeitNachricht, jetzt time.Time) *LeitNachricht {
	// Der Leiter gehoert zu seinem eigenen Satz, und der Satz passt zu
	// seinem Hash -- sonst ist das keine Lease, auf die man sich einlaesst.
	if len(m.Satz) > 0 {
		if satzHashVon(m.SatzTerm, m.SatzVersion, normSatz(m.Satz)) != m.SatzHash ||
			!l.enthaelt(normSatz(m.Satz), m.Von) {
			return nil
		}
	} else if !l.imSatz(m.Von) || m.SatzHash != l.hash {
		return nil
	}
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
	// Den Satz des Leiters uebernehmen (wie Raft-Folger das Log des Leiters):
	// er ist der neueste, den eine Mehrheit ihn hat waehlen lassen.
	plausibel := len(m.Satz) > 0 && m.SatzHash != l.hash &&
		satzHashVon(m.SatzTerm, m.SatzVersion, normSatz(m.Satz)) == m.SatzHash &&
		l.satzwechselPlausibel(m, jetzt)
	if len(m.Satz) > 0 {
		// Merken, was dieser Leiter zeigt -- Massstab fuer seine naechste
		// Aenderung. Auch wenn er abgelehnt wurde: sonst gaelte die naechste
		// Luege wieder als "erster Satz, den ich von ihm sehe".
		if l.gesehenVon != m.Von || l.gesehenTerm != m.Term || plausibel {
			l.gesehenSatz = normSatz(m.Satz)
		}
		l.gesehenVon, l.gesehenTerm = m.Von, m.Term
	}
	if plausibel {
		l.setzeSatz(m.Satz, m.SatzTerm, m.SatzVersion)
		l.fest = false
		l.vorher = nil
		for a, u := range m.URLs {
			if a != l.ich && u != "" {
				l.urls[a] = u
			}
		}
		l.speichern()
	}
	ack = l.basis(leitArtLeaseAck, jetzt)
	ack.ZeitMs = m.ZeitMs
	ack.Term = l.term
	ack.Gewaehrt = true
	for _, a := range l.satz {
		if a != l.ich && a != m.Von && l.kuerzlichGehoert(a, jetzt) {
			ack.Lebend = append(ack.Lebend, a)
		}
	}
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
			l.waehlbar(m, jetzt)
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
	if !l.waehlbar(m, jetzt) {
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
		"satz":             l.satz,
		"satz_version":     l.satzVersion,
		"satz_stempel":     l.satzTerm,
		"satz_bestaetigt":  l.fest,
		"aenderung_offen":  l.vorher != nil,
		"letzte_aenderung": l.letzteAenderung,
		"bewerber":         len(l.bewerber),
		"mitglied":         l.imSatz(l.ich),
		"mit_wahl":         l.mitWahl(),
		"leiterfaehig":     l.ichFaehig,
		"notbetrieb":       l.notbetrieb(jetzt),
		"uebergabe_laeuft": l.abschliessen || l.offeneUebergabe != nil || l.wartet != nil,
		"satz_hash":        l.hash,
	}
}

// empfangeHallo: ein Validator meldet sich. Der Leiter merkt ihn fuer die
// Aufnahme vor und antwortet mit seiner Lease -- so erfaehrt der Neue den
// Leiter und den Satz, auch wenn er noch nicht dazugehoert.
func (l *Leitung) empfangeHallo(m LeitNachricht, jetzt time.Time) *LeitNachricht {
	if !l.zugelassen(m.Von) {
		return nil
	}
	l.bewerber[m.Von] = bewerberInfo{zeit: jetzt, hoehe: m.Hoehe, faehig: m.Faehig}
	if l.rolle != leitLeiter {
		return nil
	}
	antw := l.lease(jetzt)
	antw.Uebergabe = l.beleg
	return &antw
}

// quittiert: wie viele Mitglieder (ich eingeschlossen) haben eine Lease mit
// dem aktuellen Satz innerhalb der LeaseDauer quittiert?
func (l *Leitung) quittiert(jetzt time.Time) int {
	n := 0
	if l.imSatz(l.ich) {
		n = 1
	}
	for a, gesendet := range l.acks {
		if l.imSatz(a) && l.ackSatz[a] == l.hash && jetzt.Sub(gesendet) < l.cfg.LeaseDauer {
			n++
		}
	}
	return n
}

// mitgliedschaft: nur der Leiter, in jedem Takt. Bestaetigung feststellen,
// offene Aenderung zuruecknehmen, sonst hoechstens EINE neue Aenderung.
func (l *Leitung) mitgliedschaft(jetzt time.Time) {
	if l.abschliessen {
		return
	}
	// Quittungen zaehlen nur mit dem aktuellen Satz (und damit dem aktuellen
	// Stempel, also in dieser Amtszeit).
	quittiert := l.quittiert(jetzt) >= l.mehrheit()
	if !l.fest {
		if quittiert {
			l.fest = true
			l.vorher = nil
			l.speichern()
			return
		}
		frist := l.cfg.AufnahmeFrist
		if frist <= 0 {
			frist = leitVorgabe().AufnahmeFrist
		}
		if l.vorher != nil && jetzt.Sub(l.vorschlagSeit) >= frist {
			// Nicht bestaetigt: zuruecknehmen. Neuer Stand, damit der
			// zurueckgenommene Satz ueberall ueberholt ist.
			// Beide Richtungen sperren: eine abgelehnte Aufnahme wie eine
			// abgelehnte Entfernung nicht gleich wieder versuchen.
			for _, a := range l.satz {
				if !l.enthaelt(l.vorher, a) {
					l.gesperrt[a] = jetzt
				}
			}
			for _, a := range l.vorher {
				if !l.imSatz(a) {
					l.gesperrt[a] = jetzt
				}
			}
			l.letzteAenderung = fmt.Sprintf("zurueckgenommen (nicht bestaetigt binnen %s)", frist)
			alt := l.vorher
			l.vorher = nil
			l.setzeSatz(alt, l.term, l.satzVersion+1)
			l.fest = len(l.satz) <= 1
			l.letzterTakt = time.Time{}
			l.speichern()
		}
		return
	}
	// Bestaetigt, und zwar in dieser Amtszeit? (Rafts Korrektur von 2015:
	// erst dann darf ein neuer Leiter den Satz aendern.)
	if l.satzTerm != l.term || !quittiert {
		return
	}
	// Ausgefallene entfernen (nie sich selbst; EntfernenNach 0 = nie).
	for _, a := range l.satz {
		if a == l.ich || l.cfg.EntfernenNach <= 0 {
			continue
		}
		zuletzt := l.gehoert[a]
		if zuletzt.Before(l.leiterSeit) {
			zuletzt = l.leiterSeit
		}
		if jetzt.Sub(zuletzt) >= l.cfg.EntfernenNach && !l.hoertNochJemand(a, jetzt) {
			if t, ok := l.gesperrt[a]; ok && jetzt.Sub(t) < l.cfg.AufnahmeSperre {
				continue
			}
			neu := make([]string, 0, len(l.satz)-1)
			for _, b := range l.satz {
				if b != a {
					neu = append(neu, b)
				}
			}
			l.aendere(neu, "entfernt "+a+" (nichts gehoert seit "+l.cfg.EntfernenNach.String()+")", jetzt)
			return
		}
	}
	// Neue aufnehmen: registriert, gerade gemeldet, nicht weit zurueck.
	var kand []string
	for a, b := range l.bewerber {
		if jetzt.Sub(b.zeit) >= 3*l.cfg.Takt {
			if jetzt.Sub(b.zeit) >= l.cfg.EntfernenNach {
				delete(l.bewerber, a)
			}
			continue
		}
		if l.imSatz(a) || !l.aufnehmbar(a, l.satz) || b.hoehe < l.hoehe()-l.cfg.AufnahmeToleranz {
			continue
		}
		if t, ok := l.gesperrt[a]; ok && jetzt.Sub(t) < l.cfg.AufnahmeSperre {
			continue
		}
		kand = append(kand, a)
	}
	if len(kand) == 0 {
		return
	}
	sort.Strings(kand)
	l.aendere(append(append([]string(nil), l.satz...), kand[0]), "aufgenommen "+kand[0], jetzt)
}

// hoertNochJemand: meldet ein Folger (in einer frischen Quittung), dass er a
// noch hoert? Dann ist a nur vom Leiter abgeschnitten, nicht ausgefallen.
func (l *Leitung) hoertNochJemand(a string, jetzt time.Time) bool {
	for f, lebend := range l.lebendBei {
		if t, ok := l.acks[f]; !ok || jetzt.Sub(t) >= l.cfg.LeaseDauer {
			continue
		}
		if l.enthaelt(lebend, a) {
			return true
		}
	}
	return false
}

func (l *Leitung) enthaelt(satz []string, a string) bool {
	for _, b := range satz {
		if b == a {
			return true
		}
	}
	return false
}

// aendere: genau ein Mitglied mehr oder weniger. Gilt sofort (wie in Raft),
// bestaetigt erst mit der Mehrheit des NEUEN Satzes.
func (l *Leitung) aendere(neu []string, grund string, jetzt time.Time) {
	l.vorher = append([]string(nil), l.satz...)
	l.vorherTerm, l.vorherVersion = l.satzTerm, l.satzVersion
	l.vorschlagSeit = jetzt
	l.setzeSatz(neu, l.term, l.satzVersion+1)
	l.fest = len(l.satz) <= 1
	if l.fest {
		l.vorher = nil
	}
	l.letzteAenderung = grund
	l.letzterTakt = time.Time{} // Lease mit dem neuen Satz sofort
	l.speichern()
}
