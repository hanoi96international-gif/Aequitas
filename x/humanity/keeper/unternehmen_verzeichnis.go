package keeper

// Verzeichnis, Bürgschaften und Austreten für Unternehmen
// (docs/UNTERNEHMEN_KONZEPT.md 10.2 Nr. 5 und 12, WIRTSCHAFT_ZAHLENPRUEFUNG.md 2.3).
//
// # WARUM
//
// Ein Unternehmen lässt sich auf der Kette nicht als Unternehmen erkennen, nur
// an seinem Verhalten. Damit Menschen trotzdem finden, wo sie AEQ ausgeben
// können, braucht es drei Dinge, und keins davon bringt Geld:
//
//   - Verzeichniseintrag: Ort, Annahme-Regel des Ladens ("bis 20 % des
//     Einkaufs") und Webseite -- selbst angegeben, von einem Verantwortlichen
//     unterschrieben. Ob die Webseite wirklich zum Laden gehört, prüft jede App
//     selbst über https://<seite>/.well-known/aequitas.txt; der Knoten ruft
//     keine fremde Adresse ab.
//   - Bürgschaft: ein verifizierter Mensch, der dort selbst bezahlt hat und
//     nicht verantwortlich ist, bestätigt "diesen Betrieb gibt es". Höchstens
//     buergschaftenJeJahr je Mensch in 365 Tagen, je Unternehmen einmal im
//     Jahr. Ein Hinweis, kein Beweis -- der Beweis ist Kundschaft über die Zeit.
//   - Austreten: ein Mitinhaber trägt sich aus. Die Gründerin (erster
//     Eintrag) nicht: an ihr hängen Gründungsphase und Erstes-Unternehmen-Regel
//     (wirtschaft2.go); gäbe sie ihren Platz ab, könnte sie die Vorteile mit
//     jedem neuen Unternehmen erneut nutzen. Sie schließt stattdessen.
//
// # KONSENS
//
// Alle drei sind Transaktionen mit Nachweis (auftrag_nachweis.go): jeder Knoten
// prüft die Unterschrift und den Zustand beim Nachspielen selbst. Nichts davon
// ändert Kontostände, Liegegeld oder StateRoot. Aktiv erst ab
// unternehmenVerzeichnisAbUnix -- ein Knoten ohne diese Arten weist Blöcke mit
// ihnen ab (Whitelist in block.go), darum müssen vorher alle Knoten
// ausgerollt sein.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// 2026-11-01T00:00:00Z: nach Stufe 2 (15.10.), mit Zeit für das Ausrollen.
const unternehmenVerzeichnisAbUnix int64 = 1793491200

// unternehmenVerzeichnisOverride: nur für Tests (0 = Konstante gilt).
var unternehmenVerzeichnisOverride atomic.Int64

func unternehmenVerzeichnisAktiv(unix int64) bool {
	a := unternehmenVerzeichnisAbUnix
	if o := unternehmenVerzeichnisOverride.Load(); o != 0 {
		a = o
	}
	return unix >= a
}

const (
	buergschaftenJeJahr     = 5
	buergschaftFensterSek   = 365 * 86400
	buergenGespeichertJeUnt = 200 // die neuesten; gezählt werden alle
	verzeichnisOrtMax       = 60
	verzeichnisAnnahmeMax   = 80
	verzeichnisWebseiteMax  = 100
)

// buergeEintrag: eine Bürgschaft im Register des Unternehmens. Gehalten
// werden die der letzten 365 Tage; gezählt (BuergenAnzahl) werden alle.
type buergeEintrag struct {
	M  string `json:"m"`
	At int64  `json:"at"`
}

// Nur https und ein Rechnername, kein Pfad, kein Port, keine Anmeldedaten:
// die App baut daraus https://<host>/.well-known/aequitas.txt.
var webseiteMuster = regexp.MustCompile(`^https://([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

func normText(s string, max int) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '|' {
			return ' '
		}
		return r
	}, s))
	if r := []rune(s); len(r) > max {
		s = string(r[:max])
	}
	return s
}

// normWebseite: "" bleibt "", sonst die normalisierte Adresse oder Fehler.
func normWebseite(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, "/")
	if s == "" {
		return "", true
	}
	if len(s) > verzeichnisWebseiteMax || !webseiteMuster.MatchString(s) {
		return "", false
	}
	return s, true
}

func unternehmenVerzeichnisNachricht(u, v, ort, annahme, webseite string, zeit int64) string {
	return fmt.Sprintf("Aequitas: Verzeichniseintrag\nUnternehmen: %s\nVerantwortlich: %s\nOrt: %s\nAnnahme: %s\nWebseite: %s\nZeit: %d",
		u, v, ort, annahme, webseite, zeit)
}

func unternehmenBuergschaftNachricht(u, m string, zeit int64) string {
	return fmt.Sprintf("Aequitas: Buergschaft fuer ein Unternehmen\nUnternehmen: %s\nMensch: %s\nZeit: %d", u, m, zeit)
}

func unternehmenAustretenNachricht(u, m string, zeit int64) string {
	return fmt.Sprintf("Aequitas: Als Verantwortliche austreten\nUnternehmen: %s\nVerantwortlich: %s\nZeit: %d", u, m, zeit)
}

// applyUnternehmenVerzeichnisLocked: Eintrag setzen. zeit ist die
// unterschriebene Zeit; sie muss neuer sein als die des bisherigen Eintrags,
// damit eine alte Unterschrift einen neueren Eintrag nicht überschreibt.
func (cs *ChainState) applyUnternehmenVerzeichnisLocked(ctx context.Context, unternehmen, verantwortlich, ort, annahme, webseite string, zeit, at int64) error {
	if !unternehmenVerzeichnisAktiv(at) {
		return fmt.Errorf("unternehmen_verzeichnis vor der Aktivierung: %w", ErrZustandLehntAb)
	}
	u, ok1 := normAdresse(unternehmen)
	v, ok2 := normAdresse(verantwortlich)
	web, ok3 := normWebseite(webseite)
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("unternehmen_verzeichnis: ungueltige Adresse oder Webseite: %w", ErrZustandLehntAb)
	}
	if ort != normText(ort, verzeichnisOrtMax) || annahme != normText(annahme, verzeichnisAnnahmeMax) {
		return fmt.Errorf("unternehmen_verzeichnis: Text nicht normalisiert: %w", ErrZustandLehntAb)
	}
	w := cs.wirt()
	w.mu.Lock()
	e := w.offenesUnternehmenLocked(u)
	switch {
	case e == nil:
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_verzeichnis: %s ist kein offenes Unternehmen: %w", u, ErrZustandLehntAb)
	case !e.istVerantwortlich(v):
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_verzeichnis: %s ist nicht verantwortlich: %w", v, ErrZustandLehntAb)
	case zeit <= e.VerzeichnisZeit:
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_verzeichnis: Unterschrift aelter als der bisherige Eintrag: %w", ErrZustandLehntAb)
	}
	e.Ort, e.Annahme, e.Webseite, e.VerzeichnisZeit = ort, annahme, web, zeit
	cp := e.kopie()
	w.mu.Unlock()
	return cs.speichereUnternehmen(ctx, cp)
}

// applyUnternehmenBuergschaftLocked: Mensch m bürgt für Unternehmen u.
//
// Geprüft wird nur, was jeder Knoten aus dem Register kennt -- auch einer,
// der von einem Snapshot gestartet ist: Mensch, nicht verantwortlich, je
// Unternehmen einmal und insgesamt höchstens buergschaftenJeJahr in 365 Tagen.
// "Hat dort bezahlt" steht in der Buchführung, die nach einem Snapshot fehlt;
// das prüft deshalb nur die Annahme (hatDortBezahlt). Ein Produzent, der es
// übergeht, bläht eine Anzeige auf, kein Konto.
func (cs *ChainState) applyUnternehmenBuergschaftLocked(ctx context.Context, unternehmen, mensch string, at int64) error {
	if !unternehmenVerzeichnisAktiv(at) {
		return fmt.Errorf("unternehmen_buergschaft vor der Aktivierung: %w", ErrZustandLehntAb)
	}
	u, ok1 := normAdresse(unternehmen)
	m, ok2 := normAdresse(mensch)
	if !ok1 || !ok2 {
		return fmt.Errorf("unternehmen_buergschaft: ungueltige Adressen: %w", ErrZustandLehntAb)
	}
	if err := cs.pruefeMenschLocked(ctx, m); err != nil {
		return err
	}
	w := cs.wirt()
	w.mu.Lock()
	e := w.offenesUnternehmenLocked(u)
	if e == nil {
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_buergschaft: %s ist kein offenes Unternehmen: %w", u, ErrZustandLehntAb)
	}
	if e.istVerantwortlich(m) {
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_buergschaft: Verantwortliche buergen nicht fuer sich selbst: %w", ErrZustandLehntAb)
	}
	if err := w.buergschaftErlaubtLocked(u, m, at); err != nil {
		w.mu.Unlock()
		return err
	}
	frisch := e.Buergen[:0:0]
	for _, b := range e.Buergen {
		if at-b.At < buergschaftFensterSek {
			frisch = append(frisch, b)
		}
	}
	e.Buergen = append(frisch, buergeEintrag{M: m, At: at})
	e.BuergenAnzahl++
	cp := e.kopie()
	w.mu.Unlock()
	return cs.speichereUnternehmen(ctx, cp)
}

// buergschaftErlaubtLocked: je Unternehmen einmal in 365 Tagen, insgesamt
// höchstens buergschaftenJeJahr. Nur aus dem Register. w.mu gehalten.
func (w *wirtschaft) buergschaftErlaubtLocked(u, m string, at int64) error {
	n := 0
	for adr, e := range w.unternehmen {
		for _, b := range e.Buergen {
			if b.M != m || at-b.At >= buergschaftFensterSek {
				continue
			}
			if adr == u {
				return fmt.Errorf("unternehmen_buergschaft: %s hat fuer %s schon in diesem Jahr gebuergt: %w", m, u, ErrZustandLehntAb)
			}
			n++
		}
	}
	if n >= buergschaftenJeJahr {
		return fmt.Errorf("unternehmen_buergschaft: hoechstens %d Buergschaften in 365 Tagen: %w", buergschaftenJeJahr, ErrZustandLehntAb)
	}
	return nil
}

// hatDortBezahlt: Mensch m hat in diesem oder dem vorigen Quartal bei u
// eingekauft (Buchführung, wie für den Umsatz gezählt). Nur lesend -- die
// Annahme prüft damit, das Nachspielen nicht (siehe oben).
func (cs *ChainState) hatDortBezahlt(m, u string, at int64) bool {
	w := cs.wirt()
	w.mu.Lock()
	defer w.mu.Unlock()
	k := w.buch[m]
	if k == nil {
		return false
	}
	q := quartalVon(at)
	switch {
	case k.GzQuartal == q:
		return k.Gezaehlt[u] > 0 || k.GezaehltVorher[u] > 0
	case naechstesQuartal(k.GzQuartal) == q:
		return k.Gezaehlt[u] > 0
	}
	return false
}

// applyUnternehmenAustretenLocked: Mitinhaber m trägt sich aus u aus.
func (cs *ChainState) applyUnternehmenAustretenLocked(ctx context.Context, unternehmen, mensch string, at int64) error {
	if !unternehmenVerzeichnisAktiv(at) {
		return fmt.Errorf("unternehmen_austreten vor der Aktivierung: %w", ErrZustandLehntAb)
	}
	u, ok1 := normAdresse(unternehmen)
	m, ok2 := normAdresse(mensch)
	if !ok1 || !ok2 {
		return fmt.Errorf("unternehmen_austreten: ungueltige Adressen: %w", ErrZustandLehntAb)
	}
	w := cs.wirt()
	w.mu.Lock()
	e := w.offenesUnternehmenLocked(u)
	switch {
	case e == nil:
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_austreten: %s ist kein offenes Unternehmen: %w", u, ErrZustandLehntAb)
	case !e.istVerantwortlich(m):
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_austreten: %s ist nicht verantwortlich: %w", m, ErrZustandLehntAb)
	case e.Verantwortliche[0] == m:
		w.mu.Unlock()
		return fmt.Errorf("unternehmen_austreten: die Gruenderin tritt nicht aus, sie schliesst: %w", ErrZustandLehntAb)
	}
	rest := make([]string, 0, len(e.Verantwortliche)-1)
	for _, v := range e.Verantwortliche {
		if v != m {
			rest = append(rest, v)
		}
	}
	e.Verantwortliche = rest
	cp := e.kopie()
	w.mu.Unlock()
	return cs.speichereUnternehmen(ctx, cp)
}

// verzeichnisDaten: was in der Spalte wirtschaft_unternehmen.verzeichnis steht.
type verzeichnisDaten struct {
	Ort             string          `json:"ort,omitempty"`
	Annahme         string          `json:"annahme,omitempty"`
	Webseite        string          `json:"webseite,omitempty"`
	VerzeichnisZeit int64           `json:"zeit,omitempty"`
	Buergen         []buergeEintrag `json:"buergen,omitempty"`
	BuergenAnzahl   int             `json:"buergen_anzahl,omitempty"`
}

func verzeichnisJSON(e *unternehmenEintrag) string {
	d := verzeichnisDaten{e.Ort, e.Annahme, e.Webseite, e.VerzeichnisZeit, e.Buergen, e.BuergenAnzahl}
	if d.Ort == "" && d.Annahme == "" && d.Webseite == "" && d.VerzeichnisZeit == 0 && len(d.Buergen) == 0 && d.BuergenAnzahl == 0 {
		return ""
	}
	b, _ := json.Marshal(d)
	return string(b)
}

func verzeichnisAusJSON(e *unternehmenEintrag, s string) {
	if s == "" {
		return
	}
	var d verzeichnisDaten
	if json.Unmarshal([]byte(s), &d) != nil {
		return
	}
	e.Ort, e.Annahme, e.Webseite, e.VerzeichnisZeit = d.Ort, d.Annahme, d.Webseite, d.VerzeichnisZeit
	e.Buergen, e.BuergenAnzahl = d.Buergen, d.BuergenAnzahl
}

// ------------------------------------------------------------ Annahme (HTTP)

// POST /api/unternehmen/verzeichnis
// {unternehmen, verantwortlich, ort, annahme, webseite, zeit, sig}
func (a *APIServer) handleUnternehmenVerzeichnis(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Unternehmen    string `json:"unternehmen"`
		Verantwortlich string `json:"verantwortlich"`
		Ort            string `json:"ort"`
		Annahme        string `json:"annahme"`
		Webseite       string `json:"webseite"`
		Zeit           int64  `json:"zeit"`
		Sig            string `json:"sig"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	now := time.Now().Unix()
	u, ok1 := normAdresse(req.Unternehmen)
	v, ok2 := normAdresse(req.Verantwortlich)
	web, ok3 := normWebseite(req.Webseite)
	if !ok1 || !ok2 || !unternehmenVerzeichnisAktiv(now) {
		jsonError(w, "invalid addresses or not active yet", http.StatusBadRequest)
		return
	}
	if !ok3 {
		jsonError(w, "webseite must be https://host without path", http.StatusBadRequest)
		return
	}
	if !zeitFrisch(req.Zeit) {
		jsonError(w, "zeit expired or in the future", http.StatusBadRequest)
		return
	}
	ort := normText(req.Ort, verzeichnisOrtMax)
	annahme := normText(req.Annahme, verzeichnisAnnahmeMax)
	if err := verifyPersonalSign(unternehmenVerzeichnisNachricht(u, v, ort, annahme, web, req.Zeit), req.Sig, v); err != nil {
		jsonError(w, "signature invalid: "+err.Error(), http.StatusForbidden)
		return
	}
	tx := Transaction{Type: "unternehmen_verzeichnis", Wallet: u, To: v, Ort: ort, Annahme: annahme, Webseite: web,
		Nachweis: nachweisFuerAnnahme(Auftragsnachweis{Sig: req.Sig, Zeit: req.Zeit})}
	a.unternehmenEinreichen(w, []string{u}, tx, func(ctx context.Context) error {
		return a.state.applyUnternehmenVerzeichnisLocked(ctx, u, v, ort, annahme, web, req.Zeit, now)
	})
}

// POST /api/unternehmen/buergschaft {unternehmen, mensch, zeit, sig}
func (a *APIServer) handleUnternehmenBuergschaft(w http.ResponseWriter, r *http.Request) {
	a.unternehmenEinfacherAuftrag(w, r, "unternehmen_buergschaft", unternehmenBuergschaftNachricht,
		func(ctx context.Context, u, m string, at int64) error {
			if !a.state.hatDortBezahlt(m, u, at) {
				return fmt.Errorf("unternehmen_buergschaft: %s hat dort in diesem oder dem vorigen Quartal nicht bezahlt: %w", m, ErrZustandLehntAb)
			}
			return a.state.applyUnternehmenBuergschaftLocked(ctx, u, m, at)
		})
}

// POST /api/unternehmen/austreten {unternehmen, mensch, zeit, sig}
func (a *APIServer) handleUnternehmenAustreten(w http.ResponseWriter, r *http.Request) {
	a.unternehmenEinfacherAuftrag(w, r, "unternehmen_austreten", unternehmenAustretenNachricht,
		func(ctx context.Context, u, m string, at int64) error {
			return a.state.applyUnternehmenAustretenLocked(ctx, u, m, at)
		})
}

// unternehmenEinfacherAuftrag: ein Mensch unterschreibt (unternehmen, mensch, zeit).
func (a *APIServer) unternehmenEinfacherAuftrag(w http.ResponseWriter, r *http.Request, typ string,
	nachricht func(u, m string, zeit int64) string, anwenden func(ctx context.Context, u, m string, at int64) error) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Unternehmen string `json:"unternehmen"`
		Mensch      string `json:"mensch"`
		Zeit        int64  `json:"zeit"`
		Sig         string `json:"sig"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	now := time.Now().Unix()
	u, ok1 := normAdresse(req.Unternehmen)
	m, ok2 := normAdresse(req.Mensch)
	if !ok1 || !ok2 || !unternehmenVerzeichnisAktiv(now) {
		jsonError(w, "invalid addresses or not active yet", http.StatusBadRequest)
		return
	}
	if !zeitFrisch(req.Zeit) {
		jsonError(w, "zeit expired or in the future", http.StatusBadRequest)
		return
	}
	if err := verifyPersonalSign(nachricht(u, m, req.Zeit), req.Sig, m); err != nil {
		jsonError(w, "signature invalid: "+err.Error(), http.StatusForbidden)
		return
	}
	tx := Transaction{Type: typ, Wallet: u, To: m,
		Nachweis: nachweisFuerAnnahme(Auftragsnachweis{Sig: req.Sig, Zeit: req.Zeit})}
	a.unternehmenEinreichen(w, []string{u, m}, tx, func(ctx context.Context) error {
		return anwenden(ctx, u, m, now)
	})
}
