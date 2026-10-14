package keeper

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// CHECKPOINTS UEBER DIE LEITUNGSWEGE (OFFEN 10, Stufe C, Baustein 5b;
// docs/CHECKPOINT_FAHRPLAN.md).
//
// # EINSCHALTEN
//
// Nur mit AEQUITAS_CHECKPOINTS=an und eingeschalteter Leitung. Ohne den
// Schalter antwortet /api/checkpoint mit 404 und es laeuft nichts.
// Checkpoints sind noch KEINE Konsensregel: ein Zertifikat wird nur
// gespeichert und als "letzter Checkpoint" fuer die naechsten Vorschlaege
// benutzt (Kette). In die Kette kommt es erst mit Baustein 6.
//
// # ABLAUF JE SLOT (60 s)
//
//  1. Der Leiter (Leitung) waehlt einen Block, der zwischen 150 und 600 s
//     alt ist: vom eigenen juengsten Block den ersten Eltern entlang
//     zurueck (hoechstens checkpointWahlSchritte Schritte).
//  2. checkpointVorschlagErstellen: Slot dauerhaft gemerkt, Vorschlag
//     unterschrieben; der Leiter unterschreibt als Mitglied selbst mit.
//  3. POST /api/checkpoint {"art":"vorschlag"} an jedes Komiteemitglied mit
//     bekannter URL (hoechstens checkpointHoechstZiele, je 3 s). Die
//     Antwort ist die Unterschrift; der Sammler prueft jede.
//  4. Bei Quorum: das Zertifikat selbst annehmen und an dieselben Ziele
//     schicken ({"art":"zertifikat"}). Jedes Mitglied prueft es selbst
//     (checkpointGegenKette, Nachfolger des letzten) und speichert es.
//
// # GRENZEN
//
// Nur Validatoren (IP in der Freiliste der Leitung, rpc_frei.go) erreichen
// /api/checkpoint -- Fremde bekommen 403, bevor gelesen wird (Pruefung von
// 5b, MITTEL 1). Rumpf hoechstens 64 KiB, mit Lesefrist gelesen, BEVOR ein
// Platz belegt wird (sonst hielten vier langsame Ruempfe alle Plaetze);
// hoechstens checkpointGleichzeitig in Arbeit (sonst 503). Erst nach dem
// Laden des letzten Checkpoints (checkpointBereit) wird geantwortet -- sonst
// fehlte die Kettenpruefung (MITTEL 2). Datenbank-Abfragen mit Zeitgrenze.
// Ziele des Leiters nur http(s) mit Host. Die Tabelle haelt die juengsten
// checkpointAufbewahren Zertifikate.
//
// # WAS NICHT GESCHIEHT
//
// Ein Leiter, der keine Zertifikate zustande bringt, gibt die Leitung NICHT
// ab: solange Checkpoints keine Konsensregel sind, haengt an ihnen nichts,
// und ein systemischer Grund (weniger als vier Mitglieder, Komitee-Hash
// verschieden) liesse die Leitung sonst pendeln. checkpointLeiterStockt wird
// gemeldet (/api/health/combined), nicht befolgt -- bis Baustein 6.

const (
	checkpointsEnv           = "AEQUITAS_CHECKPOINTS"
	checkpointGleichzeitig   = 4
	checkpointHoechstZiele   = targetCommitteeSize
	checkpointWahlSchritte   = 5000
	checkpointWahlMindestAlt = 150
	checkpointRumpfGrenze    = 64 << 10
	checkpointLeseFrist      = 5 * time.Second
	checkpointAufbewahren    = 1440 // ein Tag bei einem je Minute
)

func checkpointsAn() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(checkpointsEnv)), "an")
}

// CheckpointNachricht: was ueber /api/checkpoint geht.
type CheckpointNachricht struct {
	Art        string                `json:"art"` // "vorschlag" | "zertifikat"
	Vorschlag  *CheckpointVorschlag  `json:"vorschlag,omitempty"`
	Zertifikat *CheckpointZertifikat `json:"zertifikat,omitempty"`
}

// --- letzter Checkpoint -------------------------------------------------------

var checkpointZertTabelleDa sync.Map

func checkpointZertifikatTabelle(db *sql.DB) error {
	if _, da := checkpointZertTabelleDa.Load(db); da {
		return nil
	}
	ctx, abbrechen := context.WithTimeout(context.Background(), 5*time.Second)
	defer abbrechen()
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS checkpoint_zertifikate (
		zeit  BIGINT PRIMARY KEY,
		hash  TEXT NOT NULL,
		hoehe BIGINT NOT NULL,
		daten TEXT NOT NULL
	)`)
	if err == nil {
		checkpointZertTabelleDa.Store(db, true)
	}
	return err
}

// checkpointLetzterLaden: der juengste gespeicherte Checkpoint (beim Start).
func (dag *BlockDAG) checkpointLetzterLaden() error {
	if dag.state == nil || dag.state.db == nil {
		return errors.New("keine Datenbank")
	}
	db := dag.state.db
	if err := checkpointZertifikatTabelle(db); err != nil {
		return err
	}
	ctx, abbrechen := context.WithTimeout(context.Background(), 5*time.Second)
	defer abbrechen()
	var daten string
	err := db.QueryRowContext(ctx, `SELECT daten FROM checkpoint_zertifikate ORDER BY zeit DESC LIMIT 1`).Scan(&daten)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var z CheckpointZertifikat
	if err := json.Unmarshal([]byte(daten), &z); err != nil {
		return err
	}
	dag.checkpointLetzter.Store(&z)
	return nil
}

func (dag *BlockDAG) letzterCheckpoint() *KettenCheckpoint {
	if z := dag.checkpointLetzter.Load(); z != nil {
		c := z.KettenCheckpoint
		return &c
	}
	return nil
}

// checkpointZertifikatAnnehmen: ein Zertifikat pruefen und als letzten
// Checkpoint speichern. Geprueft wie jedes fremde: Kette, Komitee,
// Unterschriften, und es muss den bisherigen letzten in seiner
// Vergangenheit haben.
//
// Ganz unter checkpointLetzterMu (Pruefung von 5b, NIEDRIG 3): zwei
// gleichzeitige Zertifikate auf verschiedenen Zweigen bestuenden sonst beide
// die Pruefung gegen denselben letzten.
func (dag *BlockDAG) checkpointZertifikatAnnehmen(z CheckpointZertifikat, jetzt int64) error {
	dag.checkpointLetzterMu.Lock()
	defer dag.checkpointLetzterMu.Unlock()
	letzter := dag.letzterCheckpoint()
	if letzter != nil && z.Hash == letzter.Hash {
		return nil // schon da
	}
	if err := dag.checkpointGegenKette(z, letzter, jetzt); err != nil {
		return err
	}
	if dag.state.db == nil {
		return errors.New("keine Datenbank")
	}
	if letzter != nil {
		gefunden, err := checkpointNachfolger(checkpointKopfLeser(dag), z.Hash, *letzter)
		if err != nil {
			return fmt.Errorf("Vorfahren nicht pruefbar: %v", err)
		}
		if !gefunden {
			return fmt.Errorf("letzter Checkpoint %s nicht in der Vergangenheit", kurzAdresse(letzter.Hash))
		}
	}
	if err := checkpointZertifikatTabelle(dag.state.db); err != nil {
		return err
	}
	daten, _ := json.Marshal(z)
	ctx, abbrechen := context.WithTimeout(context.Background(), 5*time.Second)
	defer abbrechen()
	res, err := dag.state.db.ExecContext(ctx, `INSERT INTO checkpoint_zertifikate (zeit, hash, hoehe, daten) VALUES ($1, $2, $3, $4) ON CONFLICT (zeit) DO NOTHING`,
		z.Zeit, z.Hash, z.Hoehe, string(daten))
	if err != nil {
		return fmt.Errorf("nicht speicherbar: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		// Schon gespeichert: derselbe Checkpoint (etwa nach einem Neustart
		// erneut empfangen) ist in Ordnung, ein anderer zur selben Zeit nicht.
		var hash string
		if err := dag.state.db.QueryRowContext(ctx, `SELECT hash FROM checkpoint_zertifikate WHERE zeit = $1`, z.Zeit).Scan(&hash); err != nil || hash != z.Hash {
			return fmt.Errorf("Zeit %d schon mit einem anderen Checkpoint belegt", z.Zeit)
		}
	}
	// Aufbewahren: die juengsten checkpointAufbewahren.
	dag.state.db.ExecContext(ctx, `DELETE FROM checkpoint_zertifikate WHERE zeit < (SELECT zeit FROM checkpoint_zertifikate ORDER BY zeit DESC OFFSET $1 LIMIT 1)`, checkpointAufbewahren-1)
	dag.checkpointLetzter.Store(zertifikatKopie(&z))
	return nil
}

// checkpointKopfLeser: woher die Vorfahren-Suche die Koepfe liest. Tests
// ersetzen ihn.
var checkpointKopfLeser = func(dag *BlockDAG) kopfLeser { return kopfLeserDB(dag.state.db) }

// --- Empfang -----------------------------------------------------------------

var checkpointInArbeit = make(chan struct{}, checkpointGleichzeitig)

// handleCheckpoint: POST /api/checkpoint.
func (a *APIServer) handleCheckpoint(w http.ResponseWriter, r *http.Request) {
	l := a.state.leitung.Load()
	if !checkpointsAn() || l == nil {
		http.Error(w, `{"error":"Checkpoints aus"}`, http.StatusNotFound)
		return
	}
	if !a.blockchain.checkpointBereit.Load() {
		http.Error(w, `{"error":"letzter Checkpoint nicht geladen"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST"}`, http.StatusMethodNotAllowed)
		return
	}
	// Nur der Leiter ruft das legitim auf: Fremde nie (vor dem Lesen).
	if !rpcRateLimitFrei(r) {
		http.Error(w, `{"error":"nur Validatoren"}`, http.StatusForbidden)
		return
	}
	// Erst lesen (mit Frist), dann einen Platz belegen -- wie
	// handleValidatorBindung (Pruefung von #326, B). Nach einem Fehler
	// bleibt die Frist (net/http verwirft den Rest des Rumpfs).
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(checkpointLeseFrist))
	rumpf, err := io.ReadAll(http.MaxBytesReader(w, r.Body, checkpointRumpfGrenze))
	if err != nil {
		http.Error(w, `{"error":"unlesbar"}`, http.StatusBadRequest)
		return
	}
	_ = rc.SetReadDeadline(time.Time{})
	select {
	case checkpointInArbeit <- struct{}{}:
		defer func() { <-checkpointInArbeit }()
	default:
		http.Error(w, `{"error":"ausgelastet"}`, http.StatusServiceUnavailable)
		return
	}
	var m CheckpointNachricht
	if err := json.Unmarshal(rumpf, &m); err != nil {
		http.Error(w, `{"error":"unlesbar"}`, http.StatusBadRequest)
		return
	}
	leiter, _ := l.Leiter()
	antw, status, err := a.blockchain.checkpointEmpfangen(m, leiter, l.Term(), time.Now().Unix())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(antw)
}

// checkpointEmpfangen: Antwort, HTTP-Status im Fehlerfall, Fehler.
func (dag *BlockDAG) checkpointEmpfangen(m CheckpointNachricht, leiter string, term uint64, jetzt int64) (interface{}, int, error) {
	switch {
	case m.Art == "vorschlag" && m.Vorschlag != nil && m.Zertifikat == nil:
		if dag.state == nil || dag.state.db == nil {
			return nil, http.StatusServiceUnavailable, errors.New("keine Datenbank")
		}
		u, err := dag.checkpointVorschlagAnnehmen(*m.Vorschlag, leiter, term, dag.letzterCheckpoint(), checkpointKopfLeser(dag), jetzt)
		if err != nil {
			return nil, http.StatusConflict, err
		}
		return u, 0, nil
	case m.Art == "zertifikat" && m.Zertifikat != nil && m.Vorschlag == nil:
		if err := dag.checkpointZertifikatAnnehmen(*m.Zertifikat, jetzt); err != nil {
			return nil, http.StatusConflict, err
		}
		return map[string]bool{"ok": true}, 0, nil
	}
	return nil, http.StatusBadRequest, errors.New("Art unbekannt")
}

// --- Leiter-Schleife -----------------------------------------------------------

// checkpointWahl: der Block fuer den Vorschlag -- vom eigenen juengsten Block
// den ersten Eltern entlang zurueck bis zum ersten, der mindestens
// checkpointWahlMindestAlt alt ist. "" wenn keiner im erlaubten Alter.
func (dag *BlockDAG) checkpointWahl(jetzt int64) string {
	start := dag.letzterEigenerBlockHash()
	if start == "" {
		return ""
	}
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	h := start
	for i := 0; i < checkpointWahlSchritte; i++ {
		b := dag.blocks[h]
		if b == nil {
			return ""
		}
		if alter := jetzt - b.Timestamp; alter >= checkpointWahlMindestAlt {
			if alter > checkpointHoechstensAlt {
				return ""
			}
			return h
		}
		if len(b.ParentHashes) == 0 {
			return ""
		}
		h = b.ParentHashes[0]
	}
	return ""
}

// checkpointStand: fuer /api/health/combined.
type checkpointStandDaten struct {
	mu          sync.Mutex
	slot        int64
	letzterFehl string
	vorschlaege int64
	zertifikate int64
	amtsantritt int64
	stockt      bool
}

func (dag *BlockDAG) CheckpointStand() map[string]interface{} {
	s := &dag.checkpointStand
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]interface{}{
		"an":             checkpointsAn(),
		"vorschlaege":    s.vorschlaege,
		"zertifikate":    s.zertifikate,
		"letzter_fehler": s.letzterFehl,
		"leiter_stockt":  s.stockt,
	}
	if c := dag.letzterCheckpoint(); c != nil {
		out["letzter"] = map[string]interface{}{"hoehe": c.Hoehe, "hash": c.Hash, "zeit": c.Zeit}
	}
	return out
}

// checkpointNetzStarten: Laden und Leiter-Schleife. Nur mit Schalter.
var checkpointNetzStarten = func(dag *BlockDAG, l *Leitung) {
	if !checkpointsAn() {
		return
	}
	if err := dag.checkpointLetzterLaden(); err != nil {
		fmt.Printf("[CHECKPOINT] ✗ letzter Checkpoint nicht lesbar: %v -- Checkpoints bleiben aus\n", err)
		return
	}
	dag.checkpointBereit.Store(true)
	fmt.Println("[CHECKPOINT] ✓ an (noch keine Konsensregel)")
	SafeGoroutine("checkpoint-leiter", func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for range t.C {
			SafeCall("checkpoint-leiter", func() { dag.checkpointTakt(l, time.Now().Unix()) })
		}
	})
}

// checkpointTakt: einmal je Slot, nur als Leiter.
func (dag *BlockDAG) checkpointTakt(l *Leitung, jetzt int64) {
	leiter, _ := l.Leiter()
	s := &dag.checkpointStand
	s.mu.Lock()
	if leiter != l.Ich() {
		s.amtsantritt = 0
		s.mu.Unlock()
		return
	}
	if s.amtsantritt == 0 {
		s.amtsantritt = jetzt
	}
	letzteZeit := int64(0)
	if c := dag.letzterCheckpoint(); c != nil {
		letzteZeit = c.Zeit
	}
	s.stockt = checkpointLeiterStockt(letzteZeit, s.amtsantritt, jetzt)
	slot := checkpointSlot(jetzt)
	if slot == s.slot {
		s.mu.Unlock()
		return
	}
	s.slot = slot
	s.mu.Unlock()

	fehl := func(err error) {
		s.mu.Lock()
		s.letzterFehl = err.Error()
		s.mu.Unlock()
	}
	hash := dag.checkpointWahl(jetzt)
	if hash == "" {
		fehl(errors.New("kein Block im erlaubten Alter"))
		return
	}
	z, err := dag.checkpointRunde(l.Ich(), l.Term(), l.URLs(), hash, jetzt, checkpointSenden)
	if err != nil {
		fehl(err)
		return
	}
	if z == nil {
		fehl(errors.New("kein Quorum"))
		return
	}
	s.mu.Lock()
	s.zertifikate++
	s.letzterFehl = ""
	s.mu.Unlock()
}

// checkpointSender: schickt eine Nachricht an eine URL und liefert die
// Antwort (begrenzt). Tests ersetzen ihn.
type checkpointSender func(url string, m CheckpointNachricht) ([]byte, error)

var checkpointKlient = &http.Client{Timeout: 3 * time.Second, CheckRedirect: ohneUmleitung}

func checkpointSenden(url string, m CheckpointNachricht) ([]byte, error) {
	b, _ := json.Marshal(m)
	resp, err := checkpointKlient.Post(url+"/api/checkpoint", "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	antw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return antw, nil
}

// checkpointRunde: Vorschlag, Unterschriften sammeln, bei Quorum das
// Zertifikat annehmen und verteilen. nil, nil: kein Quorum.
// ich/term: eigene Adresse und Amtszeit; urls: bekannte Validatoren.
func (dag *BlockDAG) checkpointRunde(ich string, term uint64, urls map[string]string, hash string, jetzt int64, senden checkpointSender) (*CheckpointZertifikat, error) {
	v, sammler, err := dag.checkpointVorschlagErstellen(hash, term, jetzt)
	if err != nil {
		return nil, err
	}
	s := &dag.checkpointStand
	s.mu.Lock()
	s.vorschlaege++
	s.mu.Unlock()

	var z *CheckpointZertifikat
	eigene, err := dag.checkpointVorschlagAnnehmen(v, ich, term, dag.letzterCheckpoint(), checkpointKopfLeser(dag), jetzt)
	if err != nil {
		return nil, fmt.Errorf("eigene Unterschrift: %v", err)
	}
	if z, err = sammler.hinzu(eigene); err != nil {
		return nil, err
	}
	// Ziele: Komiteemitglieder mit bekannter URL.
	ziele := map[string]string{}
	for a, u := range urls {
		if a != ich && sammler.komitee[a] && checkpointZielGueltig(u) && len(ziele) < checkpointHoechstZiele {
			ziele[a] = strings.TrimRight(u, "/")
		}
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	for a, u := range ziele {
		wg.Add(1)
		go func(a, u string) {
			defer wg.Done()
			defer func() { recover() }()
			antw, err := senden(u, CheckpointNachricht{Art: "vorschlag", Vorschlag: &v})
			if err != nil {
				return
			}
			var un CheckpointUnterschrift
			if json.Unmarshal(antw, &un) != nil || un.Von != a {
				return
			}
			if zz, err := sammler.hinzu(un); err == nil && zz != nil {
				mu.Lock()
				z = zz
				mu.Unlock()
			}
		}(a, u)
	}
	wg.Wait()
	if z == nil {
		return nil, nil
	}
	if err := dag.checkpointZertifikatAnnehmen(*z, jetzt); err != nil {
		return nil, fmt.Errorf("eigenes Zertifikat: %v", err)
	}
	for _, u := range ziele {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			defer func() { recover() }()
			senden(u, CheckpointNachricht{Art: "zertifikat", Zertifikat: z})
		}(u)
	}
	wg.Wait()
	return z, nil
}

// checkpointZielGueltig: nur http(s) mit Host, ohne Nutzerdaten, Abfrage
// oder Fragment. (Welche Hosts erlaubt sind, regelt die Leitung beim
// Setzen der URL -- OFFEN: zentral nur oeffentliche oder freigegebene.)
func checkpointZielGueltig(roh string) bool {
	u, err := url.Parse(roh)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}
