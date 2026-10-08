package keeper

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// Netz, Signaturen und Speicherung fuer die Leitung (leitung.go).
//
// # EINSCHALTEN
//
// Ohne AEQUITAS_LEITUNG=an aendert sich nichts: das Annahme-Tor arbeitet wie
// bisher allein mit ANNAHME_ROLLE. Mit ihr:
//
//	AEQUITAS_LEITUNG=an
//	AEQUITAS_LEITUNG_WECHSEL_MINUTEN=10        (Vorgabe; 0 = kein planmaessiger Wechsel)
//	AEQUITAS_LEITUNG_ZWEI_WECHSELN=1           (Wechsel auch bei nur zwei Validatoren)
//
// und NUR auf den Validatoren, mit denen eine Kette beginnt (Neustart bei
// null), gleich:
//
//	AEQUITAS_LEITUNG_GENESIS=0xAdresse1=http://IP1:8080,...
//
// Das ist der Genesis-Satz, wie jede Kette eine Genesis hat. Den ersten
// Term leitet die kleinste Adresse darin -- eine Regel, keine Wahl eines
// Betreibers. Danach pflegt niemand eine Liste: wer als Validator
// registriert ist (Signierschluessel an einen registrierten Menschen
// gebunden) und sich meldet, wird aufgenommen; wer lange schweigt, entfernt
// (leitung.go, "Wer Mitglied ist"). Ein spaeter hinzukommender Validator
// setzt nur AEQUITAS_LEITUNG=an: er meldet sich bei seinen Peers, erfaehrt
// den Leiter und wird aufgenommen.
//
// URLs moeglichst als http://IP:8080: dann erkennt der Leiter weitergeleitete
// Anfragen an der Absenderadresse und rechnet sie nicht auf die
// Ratenbegrenzung eines einzelnen Menschen (siehe rpc_frei.go). Validatoren
// melden ihre URL selbst (SELF_URL) in jeder signierten Nachricht.
//
// # GRENZE: ABSTURZ, NICHT BOSHEIT
//
// Wie Raft schuetzt das gegen Ausfaelle, Verluste und Netztrennungen -- nicht
// gegen einen Validator, der absichtlich luegt (etwa einen erfundenen hoeheren
// Term signiert). Das bleibt Sache der Validatorauswahl: ein Mensch, ein
// Validator, signierte Nachrichten, nachvollziehbar im Log.

const (
	leitungEnv        = "AEQUITAS_LEITUNG"
	leitungGenesisEnv = "AEQUITAS_LEITUNG_GENESIS"
	leitungWechselEnv = "AEQUITAS_LEITUNG_WECHSEL_MINUTEN"
	leitungZweiEnv    = "AEQUITAS_LEITUNG_ZWEI_WECHSELN"
	// Wie weit die Uhr eines Absenders von der eigenen abweichen darf.
	// Schuetzt gegen das Wiedereinspielen alter, gueltig signierter
	// Nachrichten.
	leitungZeitfenster = 30 * time.Second
	// Kennzeichnet eine weitergeleitete Anfrage -- nie ein zweites Mal
	// weiterleiten (sonst Kreisverkehr bei widerspruechlicher Sicht).
	weitergeleitetKopf = "X-Aequitas-Weitergeleitet"
)

func leitungAn() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(leitungEnv)), "an")
}

// leitungValidatorenAusUmgebung: "addr=url,addr=url" -> Adressen und URLs.
func leitungValidatorenAusUmgebung(roh string) ([]string, map[string]string, error) {
	urls := map[string]string{}
	var satz []string
	for _, teil := range strings.Split(roh, ",") {
		teil = strings.TrimSpace(teil)
		if teil == "" {
			continue
		}
		addr, u, _ := strings.Cut(teil, "=")
		addr = strings.ToLower(strings.TrimSpace(addr))
		if !strings.HasPrefix(addr, "0x") || len(addr) != 42 {
			return nil, nil, fmt.Errorf("%q ist keine Validator-Adresse", addr)
		}
		satz = append(satz, addr)
		if u = strings.TrimRight(strings.TrimSpace(u), "/"); u != "" {
			urls[addr] = u
		}
	}
	return normSatz(satz), urls, nil
}

// --- Signaturen -----------------------------------------------------------

func leitNachrichtHash(m LeitNachricht) []byte {
	m.Sig = ""
	b, _ := json.Marshal(m)
	return crypto.Keccak256([]byte("aequitas-leitung:"), b)
}

func (dag *BlockDAG) signiereLeitNachricht(m *LeitNachricht) {
	if dag.signingKey == nil {
		return
	}
	if sig, err := crypto.Sign(leitNachrichtHash(*m), dag.signingKey); err == nil {
		m.Sig = hex.EncodeToString(sig)
	}
}

// pruefeLeitNachricht: Signatur passt zum Absender, und die Nachricht ist
// frisch. Ob der Absender im Satz ist, prueft Leitung.Empfange.
func pruefeLeitNachricht(m LeitNachricht, jetzt time.Time) error {
	sig, err := hex.DecodeString(strings.TrimPrefix(m.Sig, "0x"))
	if err != nil || len(sig) != 65 {
		return fmt.Errorf("Signatur fehlt oder ist unlesbar")
	}
	pub, err := crypto.SigToPub(leitNachrichtHash(m), sig)
	if err != nil {
		return fmt.Errorf("Signatur nicht pruefbar: %v", err)
	}
	if !strings.EqualFold(crypto.PubkeyToAddress(*pub).Hex(), m.Von) {
		return fmt.Errorf("Signatur passt nicht zu %s", m.Von)
	}
	d := jetzt.Sub(time.UnixMilli(m.ZeitMs))
	if d > leitungZeitfenster || d < -leitungZeitfenster {
		return fmt.Errorf("Nachricht ausserhalb des Zeitfensters (%s)", d.Round(time.Second))
	}
	return nil
}

// --- Speicherung ------------------------------------------------------------
//
// Eigene Tabelle, nicht chain_config: eine Neusynchronisation ersetzt
// Zustand, und Term und abgegebene Stimme duerfen dabei nie verloren gehen --
// sonst koennte ein Knoten in derselben Amtszeit zweimal abstimmen.

func (cs *ChainState) leitungTabelle() {
	if cs.db != nil {
		cs.db.Exec(`CREATE TABLE IF NOT EXISTS leitung_zustand (id INT PRIMARY KEY, daten TEXT NOT NULL)`)
	}
}

func (cs *ChainState) leitungLaden() LeitSpeicher {
	var s LeitSpeicher
	if cs.db == nil {
		return s
	}
	var roh string
	if err := cs.db.QueryRow(`SELECT daten FROM leitung_zustand WHERE id = 1`).Scan(&roh); err == nil {
		json.Unmarshal([]byte(roh), &s)
	}
	return s
}

func (cs *ChainState) leitungSpeichern(s LeitSpeicher) {
	if cs.db == nil {
		return
	}
	b, _ := json.Marshal(s)
	if _, err := cs.db.Exec(`INSERT INTO leitung_zustand (id, daten) VALUES (1, $1)
		ON CONFLICT (id) DO UPDATE SET daten = EXCLUDED.daten`, string(b)); err != nil {
		// Ohne gespeicherte Stimme koennte ein Neustart doppelt abstimmen.
		// Lieber laut als still.
		fmt.Printf("[LEITUNG] ⚠ Zustand nicht gespeichert: %v\n", err)
	}
}

// --- Entleeren vor der Uebergabe --------------------------------------------

// leitungEntleert: nichts mehr in einer Annahme, nichts im WAL, nichts im
// Ausgangskorb, das nicht schon in einem gespeicherten Block steht.
func (cs *ChainState) leitungEntleert() bool {
	if cs.annahmenLaufend.Load() != 0 {
		return false
	}
	if cs.WALFlushQueueDepth() != 0 {
		return false
	}
	if cs.korb != nil && cs.korb.laenge() != 0 {
		return false // speicherkorb.go
	}
	if cs.walFlushSem != nil && len(cs.walFlushSem) != 0 {
		return false
	}
	if cs.db == nil {
		return true
	}
	var offen int
	if err := cs.db.QueryRow(`SELECT count(*) FROM pending_txs WHERE included_block_hash IS NULL`).Scan(&offen); err != nil {
		return false
	}
	return offen == 0
}

// --- Knoten-Anbindung ---------------------------------------------------------

// StarteLeitung baut die Leitung, wenn AEQUITAS_LEITUNG=an. Liefert nil sonst.
func StarteLeitung(dag *BlockDAG, cs *ChainState, selfURL string) *Leitung {
	StarteLeistungsnachweis(cs)
	if !leitungAn() || dag == nil || cs == nil {
		return nil
	}
	if dag.signingKey == nil {
		fmt.Println("[LEITUNG] ✗ Kein Signierschluessel -- Leitung bleibt aus")
		return nil
	}
	satz, urls, err := leitungValidatorenAusUmgebung(os.Getenv(leitungGenesisEnv))
	if err != nil {
		fmt.Printf("[LEITUNG] ✗ %s ungueltig (%v) -- Leitung bleibt aus\n", leitungGenesisEnv, err)
		return nil
	}
	ich := strings.ToLower(crypto.PubkeyToAddress(dag.signingKey.PublicKey).Hex())
	if url, ok := urls[ich]; ok && url != "" {
		selfURL = url
	}
	// Den ersten Term leitet die kleinste Adresse des Genesis-Satzes.
	start := ""
	if len(satz) > 0 {
		start = satz[0]
	}
	cfg := leitVorgabe()
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(leitungWechselEnv))); err == nil && v >= 0 {
		cfg.WechselAlle = time.Duration(v) * time.Minute
	}
	cfg.ZweiWechseln = os.Getenv(leitungZweiEnv) == "1"

	cs.leitungTabelle()
	env := LeitUmgebung{
		Hoehe:      func() int64 { return dag.heightSchnell.Load() },
		HatBlock:   dag.hatNachgespieltenBlock,
		Entleert:   cs.leitungEntleert,
		LetzterBlk: dag.letzterEigenerBlockHash,
		Speichern:  cs.leitungSpeichern,
		Ueberholt: func(term uint64) {
			// Unter der Sperre der Leitung aufgerufen -- die Arbeit gehoert
			// in eine eigene Goroutine.
			SafeGoroutine("leitung-ueberholt", func() { dag.leitungUeberholt(term) })
		},
		Zugelassen: dag.istZugelassenerValidator,
		Mensch:     dag.validatorMenschVon,
		Unbekannt:  func(string) { dag.validatorRegisterNachfragen() },
		Zuteilbar: func(a string) bool {
			bestanden, unbekannt := proben.geprueft(a, time.Now())
			return bestanden || unbekannt
		},
	}
	faehig := !cs.nurLesend.Load() && leistungsnachweisErfuellt()
	l := NeueLeitung(ich, selfURL, satz, start, faehig, cs.leitungLaden(), cfg, env, time.Now())
	for a, u := range urls {
		l.SetzeURL(a, u)
	}
	validatorIPsFrei(l)

	cs.leitung.Store(l)
	if n := cs.VorbehalteEinlesen(); n > 0 {
		fmt.Printf("[VORBEHALT] %d offene Vorbehalte eingelesen (Stufe 2)\n", n)
	}
	st := l.Stand(time.Now())
	fmt.Printf("[LEITUNG] ✓ an: Satz %v (Version %v, %v Validatoren, Wahl %v), Term %d, Leiter %s, dieser Knoten %s (Mitglied %v, leiterfaehig %v), Wechsel alle %s\n",
		st["satz"], st["satz_version"], st["validatoren"], st["mit_wahl"], l.Term(),
		func() string { a, _ := l.Leiter(); return a }(), ich, st["mitglied"], faehig, cfg.WechselAlle)
	SafeGoroutine("leitung-takt", func() { dag.leitungSchleife(l, cs) })
	return l
}

// Keine Weiterleitungen (Audit 2026-09-29, H3): die Leitungsadressen setzt
// der Betreiber, aber weiterleitungsKlient traegt den Authorization-Kopf des
// Nutzers mit. Ein Ziel, das umleitet, duerfte ihn nicht an eine dritte
// Adresse weiterreichen.
func ohneUmleitung(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }

var leitungKlient = &http.Client{Timeout: 3 * time.Second, CheckRedirect: ohneUmleitung}

var (
	leitungWeitergeleitet      atomic.Int64
	leitungWeiterleitungFehler atomic.Int64
)

func (dag *BlockDAG) leitungSchleife(l *Leitung, cs *ChainState) {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	letzteFaehigPruefung := time.Now()
	letzteKappung := time.Now()
	for range t.C {
		SafeCall("leitung-takt", func() {
			// Stufe 2 (kappung_verteilt.go): vorgemerkte Konten, fuer die
			// dieser Knoten zustaendig ist, einmal je Sekunde kappen.
			if verteilteAnnahmeAktiv(nowUnix()) && time.Since(letzteKappung) >= time.Second {
				letzteKappung = time.Now()
				cs.KappungenAbarbeiten()
				cs.VorbehalteAbarbeiten()
			}
			if time.Since(letzteFaehigPruefung) > time.Minute {
				letzteFaehigPruefung = time.Now()
				l.SetzeFaehig(!cs.nurLesend.Load() && leistungsnachweisErfuellt())
				validatorIPsFrei(l)
			}
			dag.leitungVersenden(l, l.Takt(time.Now()), dag.leitungPeers)
			if verteilteAnnahmeAktiv(nowUnix()) {
				dag.leistungsprobenPlanen(l)
			}
		})
	}
}

// leitungVersenden: signieren und an die bekannten Mitglieder schicken; ein
// Hallo zusaetzlich an alle Peers -- wer (noch) nicht dazugehoert, kennt die
// Mitglieder nicht, und der Leiter antwortet ihm mit seiner Lease.
func (dag *BlockDAG) leitungVersenden(l *Leitung, msgs []LeitNachricht, peers func() []string) {
	for _, m := range msgs {
		dag.signiereLeitNachricht(&m)
		ziele := l.URLs()
		if m.Art == leitArtHallo && peers != nil {
			for _, p := range peers() {
				ziele[p] = p
			}
		}
		gesendet := map[string]bool{}
		for _, u := range ziele {
			if u != "" && u != l.url && !gesendet[u] {
				gesendet[u] = true
				go dag.leitungSende(l, u, m)
			}
		}
	}
}

func (dag *BlockDAG) leitungSende(l *Leitung, basis string, m LeitNachricht) {
	defer func() { recover() }()
	b, _ := json.Marshal(m)
	resp, err := leitungKlient.Post(basis+"/api/leitung", "application/json", bytes.NewReader(b))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var antw LeitNachricht
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&antw); err != nil {
		return
	}
	if err := pruefeLeitNachricht(antw, time.Now()); err != nil {
		return
	}
	l.Empfange(antw, time.Now())
}

// handleLeitung: POST /api/leitung -- signierte Nachricht eines Validators.
func (a *APIServer) handleLeitung(w http.ResponseWriter, r *http.Request) {
	l := a.state.leitung.Load()
	if l == nil {
		http.Error(w, `{"error":"Leitung aus"}`, http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST"}`, http.StatusMethodNotAllowed)
		return
	}
	if !rpcRateLimitFrei(r) && rpcRateLimited(clientIP(r)) {
		http.Error(w, `{"error":"rate limited"}`, http.StatusTooManyRequests)
		return
	}
	var m LeitNachricht
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&m); err != nil {
		http.Error(w, `{"error":"unlesbar"}`, http.StatusBadRequest)
		return
	}
	if err := pruefeLeitNachricht(m, time.Now()); err != nil {
		http.Error(w, `{"error":"abgewiesen"}`, http.StatusForbidden)
		return
	}
	antw := l.Empfange(m, time.Now())
	if antw == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	a.blockchain.signiereLeitNachricht(antw)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(antw)
}

// hatNachgespieltenBlock: Block bekannt UND nachgespielt -- erst dann hat
// dieser Knoten alles, was der alte Leiter angenommen hat.
func (dag *BlockDAG) hatNachgespieltenBlock(hash string) bool {
	if hash == "" {
		return true
	}
	dag.mu.RLock()
	_, da := dag.blocks[hash]
	dag.mu.RUnlock()
	if !da {
		return false
	}
	dag.replayedMu.Lock()
	defer dag.replayedMu.Unlock()
	return dag.replayedBlocks[hash]
}

func (dag *BlockDAG) letzterEigenerBlockHash() string {
	if v, ok := dag.letzterEigenerBlock.Load().(string); ok {
		return v
	}
	return ""
}

// leitungUeberholtLaeuft verhindert, dass zwei Meldungen zwei Aufraeumarbeiten
// gleichzeitig starten.
var leitungUeberholtMu sync.Mutex

// leitungUeberholt: dieser Knoten war Leiter und hat die Leitung NICHT durch
// eigene Uebergabe verloren (Ausfall, Netztrennung). Was er in den letzten
// Sekunden angenommen, aber noch nicht in Bloecken verteilt hat, kennt der
// neue Leiter nicht -- und der hat inzwischen selbst angenommen. Diese
// Ueberweisungen duerfen nie mehr in einen Block (die Kontenstaende liefen
// sonst auseinander, genau der Fehler vom 15.09.2026), und der eigene Zustand,
// der sie schon enthaelt, muss vom neuen Leiter neu geholt werden.
//
// Fuer die Menschen dahinter heisst das: eine Ueberweisung, die in den letzten
// Sekunden vor dem Ausfall angenommen, aber noch in keinem Block war, gilt als
// nicht geschehen. Das ist dieselbe Regel wie bei jeder Kette: bestaetigt ist,
// was in einem Block steht.
func (dag *BlockDAG) leitungUeberholt(term uint64) {
	leitungUeberholtMu.Lock()
	defer leitungUeberholtMu.Unlock()
	cs := dag.state
	l := cs.leitung.Load()
	if l == nil {
		return
	}
	fmt.Printf("[LEITUNG] ⚠ Leitung von Term %d verloren, ohne sie zu uebergeben -- verwerfe unverteilte Annahmen und hole den Zustand neu\n", term)
	// 1. Keine Bloecke mehr aus dem eigenen Ausgangskorb.
	dag.resyncInProgress.Store(true)
	defer dag.resyncInProgress.Store(false)
	// 2.+3. WAL-Warteschlange und Ausgangskorb.
	verworfen, geloescht := cs.leitungUnverteiltVerwerfen()
	fmt.Printf("[LEITUNG] %d WAL-Eintraege und %d Ausgangskorb-Zeilen verworfen\n", verworfen, geloescht)
	// 4. Zustand vom neuen Leiter.
	addr, u := l.Leiter()
	for versuch := 0; versuch < 30 && (addr == "" || u == ""); versuch++ {
		time.Sleep(2 * time.Second)
		addr, u = l.Leiter()
	}
	if addr == "" || u == "" {
		fmt.Println("[LEITUNG] ✗ Neuer Leiter unbekannt -- Neusynchronisation beim naechsten Neustart")
		cs.setConfigValueDB(autoResyncPendingKey, "1")
		return
	}
	dag.resyncInProgress.Store(false) // PerformResync setzt es selbst
	if err := dag.PerformResync(u+"/api/snapshot", addr, u); err != nil {
		fmt.Printf("[LEITUNG] ✗ Neusynchronisation vom Leiter fehlgeschlagen (%v) -- beim naechsten Neustart\n", err)
		cs.setConfigValueDB(autoResyncPendingKey, "1")
		return
	}
	fmt.Println("[LEITUNG] ✓ Zustand vom neuen Leiter uebernommen")
}

// leitungUnverteiltVerwerfen: WAL-Warteschlange leeren, laufende Flushes
// abwarten, dann alles aus dem Ausgangskorb, was in keinem gespeicherten
// Block steht. Nur fuer den ueberholten Leiter (leitungUeberholt).
func (cs *ChainState) leitungUnverteiltVerwerfen() (wal int, korb int64) {
	cs.walFlushMu.Lock()
	wal = len(cs.walFlushQueue)
	cs.walFlushQueue = nil
	cs.walInSchlange = nil
	walWarteschlangeStand.Store(0) // sonst wiese die Annahme weiter ab (wal_annahme_druck.go)
	cs.walFlushMu.Unlock()
	for i := 0; i < 300 && cs.walFlushSem != nil && len(cs.walFlushSem) > 0; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	// Speicherkorb (speicherkorb.go): verwerfen UND die Marke dahinter
	// setzen -- sonst kaemen die verworfenen Ueberweisungen beim naechsten
	// Start aus dem WAL wieder in den Korb.
	if k := cs.korb; k != nil {
		// Einen laufenden Blockbau erst fertig werden lassen (hoechstens
		// 30 s) -- sonst legte er Genommenes nach dem Verwerfen zurueck.
		for i := 0; i < 300 && k.bauer.Load(); i++ {
			time.Sleep(100 * time.Millisecond)
		}
		n, bis := k.verwerfen()
		korb += int64(n)
		if bis > cs.korbBis.Load() {
			if err := cs.setConfigValueDB(speicherKorbMarke, strconv.FormatUint(bis, 10)); err != nil {
				fmt.Printf("[LEITUNG] ✗ Korb-Marke nicht gesetzt: %v\n", err)
			} else {
				cs.korbBis.Store(bis)
			}
		}
	}
	if cs.db != nil {
		if res, err := cs.db.Exec(`DELETE FROM pending_txs WHERE included_block_hash IS NULL`); err == nil {
			korb, _ = res.RowsAffected()
		} else {
			fmt.Printf("[LEITUNG] ✗ Ausgangskorb nicht geleert: %v\n", err)
		}
	}
	return wal, korb
}

// --- Weiterleitung --------------------------------------------------------------

// weiterleitungsZiel: an wen gehoert eine annehmende Anfrage, die dieser
// Knoten nicht annimmt? Leer = selbst bearbeiten (Leitung aus, selbst
// Leiter, Leiter unbekannt, oder schon einmal weitergeleitet).
//
// konten: die Konten, die die Anfrage belastet (anfrageKonten, rpcKonten).
// Stufe 2: Ziel ist ihr Zustaendiger. Gehoeren sie verschiedenen, gibt es
// kein gemeinsames Ziel -- dann bearbeitet dieser Knoten selbst, und das Tor
// lehnt ab, was er nicht annimmt.
// weiterleitungDenkbar: kann weiterleitungsZiel ueberhaupt ein Ziel liefern?
// Billig, ohne die Konten der Anfrage -- deren Ermittlung kostet je
// Ueberweisung eine Signatur-Wiederherstellung (rpcKonten).
func (cs *ChainState) weiterleitungDenkbar(r *http.Request) bool {
	return cs.leitung.Load() != nil && r.Header.Get(weitergeleitetKopf) == ""
}

func (cs *ChainState) weiterleitungsZiel(r *http.Request, konten ...string) string {
	_, u := cs.weiterleitungsZielMitAdresse(r, konten...)
	return u
}

// weiterleitungsZielMitAdresse: dasselbe, dazu die Adresse des Ziels (fuer
// den Nachweis, weiterleitung_nachweis.go).
func (cs *ChainState) weiterleitungsZielMitAdresse(r *http.Request, konten ...string) (string, string) {
	l := cs.leitung.Load()
	if l == nil || r.Header.Get(weitergeleitetKopf) != "" {
		return "", ""
	}
	if cs.nimmtAnFuer(konten...) {
		return "", ""
	}
	var addr, u string
	if len(konten) == 0 {
		addr, u = l.Leiter()
	} else {
		addr, u = l.Zustaendig(konten[0])
		for _, k := range konten[1:] {
			if a, _ := l.Zustaendig(k); a != addr {
				return "", ""
			}
		}
	}
	if addr == "" || u == "" || addr == l.ich {
		return "", ""
	}
	return addr, u
}

// anfrageKonten: welche Konten eine annehmende REST-Anfrage belastet.
func anfrageKonten(pfad string, body []byte) []string {
	var f struct {
		Wallet      string `json:"wallet"`
		Unternehmen string `json:"unternehmen"`
	}
	_ = json.Unmarshal(body, &f)
	w := strings.ToLower(strings.TrimSpace(f.Wallet))
	switch {
	case pfad == "/api/swap" || pfad == "/api/add-liquidity" || pfad == "/api/remove-liquidity":
		// Zum Zustaendigen des Kontos: ist er zugleich Leiter, tauscht er
		// direkt, sonst ueber den Vorbehalt (vorbehalt.go).
		return []string{w}
	case pfad == "/api/faucet":
		return []string{kontoFaucet}
	case strings.HasPrefix(pfad, "/api/unternehmen/"):
		return []string{strings.ToLower(strings.TrimSpace(f.Unternehmen))}
	case pfad == "/api/recover-escrow", pfad == "/api/set-guardian", pfad == "/api/confirm-alive",
		// Die Erneuerung belastet das erneuerte Konto (handleLivenessRenewal,
		// annahmeBeginnen(wallet)) -- im verteilten Term nimmt sie nur dessen
		// Zustaendiger an, nicht zwingend der Leiter (Sicherheitspruefung #300).
		pfad == "/api/liveness-renewal":
		return []string{w}
	}
	return nil
}

// rpcKonten: die Absender aller eth_sendRawTransaction und die Adressen
// aller eth_getTransactionCount in einem RPC-Koerper (einzeln oder Batch).
// Die Wiederherstellung des Absenders kostet beim zweiten Mal nichts
// (absender_cache.go).
func rpcKonten(body []byte) []string {
	var posten []json.RawMessage
	if len(body) > 0 && body[0] == '[' {
		if json.Unmarshal(body, &posten) != nil {
			return nil
		}
		// Zu grosse Buendel weist handleRPC ohnehin ab -- hier keine
		// Wiederherstellung dafuer. Ohne diese Grenze kostete ein 1-MB-Buendel
		// tausende secp256k1-Rechnungen, bevor die Buendelgrenze griff.
		if len(posten) > rpcMaxBuendel {
			return nil
		}
	} else {
		posten = []json.RawMessage{body}
	}
	gesehen := map[string]bool{}
	var out []string
	for _, p := range posten {
		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if json.Unmarshal(p, &req) != nil || len(req.Params) == 0 {
			continue
		}
		var a string
		switch req.Method {
		case "eth_sendRawTransaction":
			var raw string
			if json.Unmarshal(req.Params[0], &raw) == nil {
				if _, s, _, err := decodeAndRecoverSender(raw); err == nil {
					a = s
				}
			}
		case "eth_getTransactionCount":
			if json.Unmarshal(req.Params[0], &a) == nil {
				a = strings.ToLower(strings.TrimSpace(a))
			}
		}
		if a != "" && !gesehen[a] {
			gesehen[a] = true
			out = append(out, a)
		}
	}
	return out
}

var weiterleitungsKlient = &http.Client{Timeout: 20 * time.Second, CheckRedirect: ohneUmleitung}

// leiteWeiter schickt die Anfrage unveraendert an den Leiter und gibt dessen
// Antwort zurueck. false = hat nicht geklappt, selbst bearbeiten (das Tor
// antwortet dann mit einem wiederholbaren Fehler).
func leiteWeiter(w http.ResponseWriter, r *http.Request, ziel string, body []byte) bool {
	return leiteWeiterMit(w, r, ziel, body, nil)
}

// leiteWeiterMit: dasselbe mit zusaetzlichen Koepfen (der Nachweis,
// weiterleitung_nachweis.go).
func leiteWeiterMit(w http.ResponseWriter, r *http.Request, ziel string, body []byte, zusatz http.Header) bool {
	req, err := http.NewRequest(r.Method, ziel+r.URL.RequestURI(), bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	if auth := r.Header.Get("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	req.Header.Set(weitergeleitetKopf, "1")
	for k, v := range zusatz {
		req.Header[k] = v
	}
	resp, err := weiterleitungsKlient.Do(req)
	if err != nil {
		leitungWeiterleitungFehler.Add(1)
		return false
	}
	defer resp.Body.Close()
	for _, k := range []string{"Content-Type"} {
		if v := resp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, io.LimitReader(resp.Body, 8<<20))
	leitungWeitergeleitet.Add(1)
	return true
}

// zumLeiter: Swap, Liquiditaet und Faucet nehmen an (annahme_tor.go) --
// bei rotierendem Leiter gehoeren sie zu ihm wie Ueberweisungen.
func (a *APIServer) zumLeiter(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Die Nonce fuer Tausch und Liquiditaet kennt am frischesten, wer das
		// Konto gerade annimmt (Stufe 2): auch das GET dorthin.
		if r.Method == http.MethodGet && r.URL.Path == "/api/nonce" && a.state.leitung.Load() != nil {
			if ziel := a.state.weiterleitungsZiel(r, strings.ToLower(r.URL.Query().Get("wallet"))); ziel != "" && leiteWeiter(w, r, ziel, nil) {
				return
			}
			h(w, r)
			return
		}
		if r.Method != http.MethodPost {
			h(w, r)
			return
		}
		if a.state.leitung.Load() == nil {
			h(w, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, `{"error":"unlesbar"}`, http.StatusBadRequest)
			return
		}
		zielAdr, ziel := a.state.weiterleitungsZielMitAdresse(r, anfrageKonten(r.URL.Path, body)...)
		if ziel != "" && weiterleitungUnterschreiben(r.URL.Path) {
			// Was der Zustaendige nicht pruefen kann, wird nicht
			// unterschrieben: ein Koerper ueber seiner Lesegrenze liesse den
			// Nachweis scheitern, und die Anfrage zaehlte dort unter der
			// Adresse dieses Knotens -- ein Absender sperrte so alle hinter
			// ihm aus (Pruefung von #319, MEDIUM-18). Ohne IP als Absender
			// (INFO-23) gibt es keinen Nachweis: dann hier bearbeiten.
			if len(body) > weiterleitungKoerperMax {
				http.Error(w, `{"error":"request body too large"}`, http.StatusRequestEntityTooLarge)
				return
			}
			if net.ParseIP(clientIP(r)) == nil {
				ziel = ""
			}
		}
		if ziel == "" {
			r.Body = io.NopCloser(bytes.NewReader(body))
			h(w, r)
			return
		}
		if !rpcRateLimitFrei(r) && rpcRateLimited(clientIP(r)) {
			http.Error(w, `{"error":"rate limited"}`, http.StatusTooManyRequests)
			return
		}
		var nachweis http.Header
		if a.blockchain != nil {
			nachweis = weiterleitungNachweis(a.blockchain.GetSigningKey(), r, zielAdr, body, time.Now())
		}
		if leiteWeiterMit(w, r, ziel, body, nachweis) {
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		h(w, r)
	}
}

// rpcSchreibt: enthaelt ein RPC-Koerper etwas, das der Leiter beantworten
// muss? sendRawTransaction nimmt an; getTransactionCount gehoert dazu, weil
// nur der Leiter die gerade vergebenen Nonces kennt -- ein Folger naennte
// eine veraltete, und die naechste Ueberweisung scheiterte an "nonce too low".
func rpcSchreibt(body []byte) bool {
	return bytes.Contains(body, []byte(`"eth_sendRawTransaction"`)) ||
		bytes.Contains(body, []byte(`"eth_getTransactionCount"`))
}

// LeitungStand fuer /api/health/combined.
func (cs *ChainState) LeitungStand() map[string]interface{} {
	l := cs.leitung.Load()
	if l == nil {
		return map[string]interface{}{"an": false,
			"bedeutung": "Rotierender Leiter aus -- es nimmt an, wer nicht ANNAHME_ROLLE=nur_lesend hat (siehe annahme_tor)."}
	}
	s := l.Stand(time.Now())
	s["an"] = true
	s["weitergeleitet"] = leitungWeitergeleitet.Load()
	s["weiterleitung_fehler"] = leitungWeiterleitungFehler.Load()
	s["bedeutung"] = "Rotierender Leiter: genau einer nimmt an (leiter), die anderen leiten weiter. " +
		"Ab drei Validatoren waehlt die Mehrheit bei Ausfall einen neuen (mit_wahl)."
	return s
}

// istZugelassenerValidator: registrierter Validator -- Signierschluessel an
// einen registrierten Menschen gebunden (register-validator-key) oder aus
// dem Validator-Abgleich unter Peers.
func (dag *BlockDAG) istZugelassenerValidator(addr string) bool {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	return dag.authorizedValidators[strings.ToLower(addr)]
}

// leitungPeers: alle bekannten Knoten-URLs (Sync-Peers und Seeds).
func (dag *BlockDAG) leitungPeers() []string {
	dag.syncPeerMu.Lock()
	defer dag.syncPeerMu.Unlock()
	var out []string
	for p := range dag.activeSyncPeers {
		out = append(out, strings.TrimRight(p, "/"))
	}
	for p := range dag.trustedSeeds {
		out = append(out, strings.TrimRight(p, "/"))
	}
	return out
}

// validatorIPsFrei: weitergeleitete Anfragen kommen von den Validatoren
// selbst; die Ratenbegrenzung eines einzelnen Menschen darf sie nicht
// treffen (der weiterleitende Knoten hat seine eigene schon angewandt).
//
// NUR DER SATZ, NIE LOOPBACK, NEU AUFGEBAUT (Pruefung von #319, MEDIUM-2).
// Bisher kam jede URL, die irgendein zugelassener Validator in einer
// Leitungsnachricht ankuendigte, ungefiltert und fuer immer in die Liste.
// Wer 127.0.0.1 oder die Adresse eines vorgeschalteten Proxys ankuendigte,
// hob die Grenze je IP fuer jeden auf, der ueber diesen Weg kommt (/rpc,
// Leitung, Bindung, Erneuerung). Jetzt: nur Mitglieder des eigenen Satzes,
// keine Loopback-, unspezifizierte, Link-Local- oder Multicast-Adresse, und
// der Teil der Validatoren wird bei jedem Lauf ersetzt (wer den Satz
// verlaesst, faellt heraus).
//
// PRIVATE ADRESSEN NUR IN AUSDRUECKLICH GENANNTEN NETZEN. Vor dem Knoten
// steht ein Proxy im Docker-Netz (deploy/Caddyfile): ALLE Anfragen von aussen
// kommen von dessen privater Adresse. Stuende die in der Liste, gaelte die
// Grenze je IP fuer niemanden mehr. Private und CGNAT-Adressen (10/8,
// 172.16/12, 192.168/16, 100.64/10, fc00::/7) kommen deshalb nur hinein, wenn
// sie in einem Netz aus AEQUITAS_FREILISTE_NETZE liegen -- etwa
// 100.64.0.0/10 fuer Validatoren, die ein Tailscale-Netz teilen. Ein
// Schalter fuer ALLE privaten Netze haette das Docker-Netz des Proxys gleich
// mit geoeffnet (Pruefung von #320, LOW-2); zusaetzlich stellt
// rpcRateLimitFreiFuer keine Anfrage frei, die X-Forwarded-For, Forwarded
// oder X-Real-IP traegt. Ein genanntes Netz gilt nur, wenn es eng genug ist
// und kein Netz der eigenen Schnittstellen beruehrt: das Docker-Netz des
// Knotens samt Gateway (docker-proxy reicht Verbindungen ohne Kopf von dort
// weiter) darf nie freistellbar werden (Pruefung von #320, LOW-5).
//
// SONDERADRESSEN NIE (Pruefung von #320, INFO-1): nur globale Unicast-
// Adressen, und auch von denen nicht die Bereiche, die einen anderen Rechner
// oder ein Uebersetzungsnetz meinen (0/8, 198.18/15, 240/4, NAT64, SIIT,
// 6to4, Teredo, IPv4-kompatibel, Site-Local, ORCHID, SRv6; Pruefung von #320,
// INFO-1 und INFO-8).
//
// MELDUNGEN BEGRENZT (Pruefung von #320, LOW-1 und INFO-2): je Mitglied wird
// gemerkt, welche Adresse zuletzt als nicht freistellbar gemeldet wurde, und
// zwar nur fuer Mitglieder des aktuellen Satzes -- die Merkliste ist durch
// die Satzgroesse begrenzt, und eine Logzeile gibt es nur, wenn sich die
// Adresse eines Mitglieds aendert. Auch Namen (https://<domain>) werden
// gemeldet: sie werden nie aufgeloest und nie freigestellt.
func validatorIPsFrei(l *Leitung) {
	netze := freilisteNetze()
	nichtFrei.Lock()
	defer nichtFrei.Unlock()
	gemeldet := map[string]string{}
	var ips []string
	for a, u := range l.SatzURLs() {
		host := ""
		if pu, err := url.Parse(u); err == nil {
			host = pu.Hostname()
		}
		ip := net.ParseIP(host)
		if freistellbar(ip, netze) {
			ips = append(ips, ip.String())
			continue
		}
		gemeldet[a] = host
		if alt, schon := nichtFrei.gemeldet[a]; !schon || alt != host {
			nichtFreiLog(a, host)
		}
	}
	nichtFrei.gemeldet = gemeldet
	rpcRateLimitFreiValidatoren(ips)
}

// sonderNetze: global geroutet sieht keiner dieser Bereiche nach einem
// einzelnen anderen Validator aus.
var sonderNetze = mussNetze(
	"0.0.0.0/8", "192.0.0.0/24", "192.88.99.0/24", "198.18.0.0/15", "240.0.0.0/4",
	"::/96", "::ffff:0:0:0/96", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16",
	"2001::/32", "2001:2::/48", "2001:10::/28", "2001:20::/28",
	"fec0::/10", "100::/64", "5f00::/16", "3fff::/20",
)

func mussNetze(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}

func inNetzen(ip net.IP, netze []*net.IPNet) bool {
	for _, n := range netze {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// freistellbar: eine Adresse, von der ein anderer Validator weiterleiten
// kann -- nur globale Unicast-Adressen (nie Loopback, das waere dieser Rechner
// selbst und jeder lokale Proxy; nie unspezifiziert, Link-Local, Multicast
// oder Broadcast), keine Sonderbereiche, und privat oder CGNAT nur innerhalb
// der genannten Netze.
func freistellbar(ip net.IP, netze []*net.IPNet) bool {
	if ip == nil || !ip.IsGlobalUnicast() || inNetzen(ip, sonderNetze) {
		return false
	}
	return !isPrivateOrLoopback(ip.String()) || inNetzen(ip, netze)
}

var freilisteNetzeGewarnt atomic.Pointer[string]

// eigeneNetze: die Netze der eigenen Schnittstellen (im Container: das
// Docker-Netz). Variable fuer Tests.
var eigeneNetze = func() ([]*net.IPNet, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	var out []*net.IPNet
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			out = append(out, n)
		}
	}
	return out, nil
}

// freilisteNetzeWarnen: Variable fuer Tests.
var freilisteNetzeWarnen = func(text string) {
	fmt.Printf("[LEITUNG] ⚠ AEQUITAS_FREILISTE_NETZE: %s\n", text)
}

// netzPraefix: die Praefixlaenge, ein IPv4-Netz in IPv6-Schreibweise
// (::ffff:0:0/96 ist nach Go 0.0.0.0/0) auf die IPv4-Maske umgerechnet.
func netzPraefix(n *net.IPNet) (einsen int, v4 bool) {
	einsen, bits := n.Mask.Size()
	if n.IP.To4() != nil {
		if bits == 128 {
			einsen -= 96
		}
		return einsen, true
	}
	return einsen, false
}

// mitNetzmaske: mindestens ein Netz, das mehr als eine Adresse umfasst.
func mitNetzmaske(netze []*net.IPNet) bool {
	for _, n := range netze {
		if einsen, bits := n.Mask.Size(); einsen < bits {
			return true
		}
	}
	return false
}

func netzeBeruehren(a, b *net.IPNet) bool {
	return a.Contains(b.IP) || b.Contains(a.IP)
}

// freilisteNetze liest AEQUITAS_FREILISTE_NETZE (CIDR, durch Kommas
// getrennt). Verworfen -- und einmal je Wert im Log genannt -- wird, was
// kein Netz ist, was zu weit ist (IPv4 kuerzer als /8, IPv6 kuerzer als
// /16) und was ein Netz der eigenen Schnittstellen beruehrt (Pruefung von
// #320, LOW-5). Sind die Schnittstellen nicht lesbar, gilt keines
// (fail-closed).
func freilisteNetze() []*net.IPNet {
	roh := strings.TrimSpace(os.Getenv("AEQUITAS_FREILISTE_NETZE"))
	if roh == "" {
		return nil
	}
	eigene, err := eigeneNetze()
	if err == nil && !mitNetzmaske(eigene) {
		// Ohne ein einziges Netz mit echter Maske (lo hat immer 127.0.0.0/8)
		// ist die Liste nicht glaubwuerdig: wie nicht lesbar (Pruefung von
		// #320, INFO-12).
		err = fmt.Errorf("keine Schnittstelle mit Netzmaske")
	}
	var netze []*net.IPNet
	var verworfen []string
	for _, teil := range strings.Split(roh, ",") {
		if teil = strings.TrimSpace(teil); teil == "" {
			continue
		}
		_, n, perr := net.ParseCIDR(teil)
		if perr != nil {
			verworfen = append(verworfen, teil+" (kein Netz in CIDR-Schreibweise)")
			continue
		}
		if einsen, v4 := netzPraefix(n); (v4 && einsen < 8) || (!v4 && einsen < 16) {
			verworfen = append(verworfen, teil+" (zu weit)")
			continue
		}
		if err != nil {
			verworfen = append(verworfen, teil+" (eigene Schnittstellen nicht lesbar)")
			continue
		}
		beruehrt := ""
		for _, e := range eigene {
			if einsen, bits := e.Mask.Size(); einsen < bits && netzeBeruehren(n, e) {
				beruehrt = e.String()
				break
			}
		}
		if beruehrt != "" {
			verworfen = append(verworfen, teil+" (beruehrt das eigene Netz "+beruehrt+")")
			continue
		}
		netze = append(netze, n)
	}
	if len(verworfen) > 0 {
		text := strings.Join(verworfen, ", ") + " -- ignoriert"
		if alt := freilisteNetzeGewarnt.Load(); alt == nil || *alt != roh+"|"+text {
			merk := roh + "|" + text
			freilisteNetzeGewarnt.Store(&merk)
			freilisteNetzeWarnen(text)
		}
	}
	return netze
}

// nichtFrei: je Mitglied des Satzes die zuletzt gemeldete Adresse, die nicht
// freigestellt wurde (validatorIPsFrei).
var nichtFrei struct {
	sync.Mutex
	gemeldet map[string]string
}

var nichtFreiLog = func(validator, host string) {
	fmt.Println(nichtFreiText(validator, host))
}

// nichtFreiText: gekuerzt und in Anfuehrungszeichen -- der Name kommt von
// einem Peer (Pruefung von #320, INFO-9).
func nichtFreiText(validator, host string) string {
	if host == "" {
		host = "keine Adresse"
	}
	if len(host) > 100 {
		host = host[:100] + "..."
	}
	return fmt.Sprintf("[LEITUNG] ⚠ %s kuendigt %q an -- wird nicht von der Ratenbegrenzung freigestellt (kein IP-Literal, Loopback, Sonderadresse oder privat ausserhalb von AEQUITAS_FREILISTE_NETZE)",
		kurzAdresse(validator), host)
}

// merkeValidatorMensch: nur aus geprueften Bindungen (eigene Registrierung
// oder signierte Bindung eines Peers). Ein Mensch, eine Stimme: die Leitung
// nimmt pro Mensch hoechstens einen Schluessel auf -- auch wenn jemand auf
// zwei Knoten zwei verschiedene Schluessel registriert hat (jeder Knoten
// prueft UNIQUE(human_wallet) nur fuer sich).
func (dag *BlockDAG) merkeValidatorMensch(signing, mensch string) {
	signing, mensch = strings.ToLower(strings.TrimSpace(signing)), strings.ToLower(strings.TrimSpace(mensch))
	if signing != "" && mensch != "" {
		dag.validatorMenschen.Store(signing, mensch)
	}
}

// validatorMenschVon: "" = unbekannt (dann nimmt die Leitung ihn nicht auf).
func (dag *BlockDAG) validatorMenschVon(signing string) string {
	signing = strings.ToLower(strings.TrimSpace(signing))
	// Ab registerLeserAb aus dem Kettenregister -- nicht mehr aus dem, was
	// ein Peer im Abgleich erzaehlt hat (validator_register_leser.go).
	if registerLeserAktiv(nowUnix()) {
		return dag.menschAusRegister(signing)
	}
	if v, ok := dag.validatorMenschen.Load(signing); ok {
		return v.(string)
	}
	var h string
	if dag.state == nil || dag.state.db == nil {
		return ""
	}
	if err := dag.state.db.QueryRow(`SELECT human_wallet FROM validator_keys WHERE signing_address = $1`, signing).Scan(&h); err != nil {
		return ""
	}
	h = strings.ToLower(h)
	dag.merkeValidatorMensch(signing, h)
	return h
}

var validatorRegisterZuletzt atomic.Int64

// validatorRegisterNachfragen: hoechstens alle 30 s das Register bei den
// Peers abgleichen (ein Leiter hat jemanden aufgenommen, den dieser Knoten
// noch nicht kennt).
func (dag *BlockDAG) validatorRegisterNachfragen() {
	jetzt := time.Now().Unix()
	alt := validatorRegisterZuletzt.Load()
	if jetzt-alt < 30 || !validatorRegisterZuletzt.CompareAndSwap(alt, jetzt) {
		return
	}
	SafeGoroutine("leitung-register", dag.syncValidatorsFromAllPeers)
}
