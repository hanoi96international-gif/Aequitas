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
// bisher allein mit ANNAHME_ROLLE. Mit ihr, auf ALLEN Validatoren gleich:
//
//	AEQUITAS_LEITUNG=an
//	AEQUITAS_LEITUNG_VALIDATOREN=0xAdresse1=http://IP1:8080,0xAdresse2=http://IP2:8080,...
//	AEQUITAS_LEITUNG_START=0xAdresse1          (leitet Term 1; nur beim allerersten Start)
//	AEQUITAS_LEITUNG_WECHSEL_MINUTEN=60        (0 = kein planmaessiger Wechsel)
//	AEQUITAS_LEITUNG_ZWEI_WECHSELN=1           (Wechsel auch bei nur zwei Validatoren)
//
// Die Validatorliste bestimmt die Mehrheit. Sie MUSS auf allen Knoten gleich
// sein -- jeder Knoten prueft das ueber einen Hash in jeder Nachricht und
// hoert Knoten mit anderer Liste nicht zu (sonst zaehlten zwei Knoten
// verschiedene Mehrheiten). Dasselbe Vertrauensmodell wie
// AUTHORIZED_VALIDATORS und VALIDATOR_LABELS.
//
// URLs moeglichst als http://IP:8080: dann erkennt der Leiter weitergeleitete
// Anfragen an der Absenderadresse und rechnet sie nicht auf die
// Ratenbegrenzung eines einzelnen Menschen (siehe rpc_frei.go).
//
// # GRENZE: ABSTURZ, NICHT BOSHEIT
//
// Wie Raft schuetzt das gegen Ausfaelle, Verluste und Netztrennungen -- nicht
// gegen einen Validator, der absichtlich luegt (etwa einen erfundenen hoeheren
// Term signiert). Das bleibt Sache der Validatorauswahl: ein Mensch, ein
// Validator, signierte Nachrichten, nachvollziehbar im Log.

const (
	leitungEnv            = "AEQUITAS_LEITUNG"
	leitungValidatorenEnv = "AEQUITAS_LEITUNG_VALIDATOREN"
	leitungStartEnv       = "AEQUITAS_LEITUNG_START"
	leitungWechselEnv     = "AEQUITAS_LEITUNG_WECHSEL_MINUTEN"
	leitungZweiEnv        = "AEQUITAS_LEITUNG_ZWEI_WECHSELN"
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
	satz, urls, err := leitungValidatorenAusUmgebung(os.Getenv(leitungValidatorenEnv))
	if err != nil || len(satz) == 0 {
		fmt.Printf("[LEITUNG] ✗ %s ungueltig (%v) -- Leitung bleibt aus\n", leitungValidatorenEnv, err)
		return nil
	}
	ich := strings.ToLower(crypto.PubkeyToAddress(dag.signingKey.PublicKey).Hex())
	if url, ok := urls[ich]; ok && url != "" {
		selfURL = url
	}
	start := strings.ToLower(strings.TrimSpace(os.Getenv(leitungStartEnv)))
	if start == "" {
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
	}
	faehig := !cs.nurLesend.Load() && leistungsnachweisErfuellt()
	l := NeueLeitung(ich, selfURL, satz, start, faehig, cs.leitungLaden(), cfg, env, time.Now())
	for a, u := range urls {
		l.SetzeURL(a, u)
	}
	// Weitergeleitete Anfragen kommen von den Validatoren selbst; die
	// Ratenbegrenzung eines einzelnen Menschen darf sie nicht treffen (der
	// weiterleitende Knoten hat seine eigene schon angewandt).
	var ips []string
	for _, u := range urls {
		if pu, err := url.Parse(u); err == nil {
			if ip := net.ParseIP(pu.Hostname()); ip != nil {
				ips = append(ips, ip.String())
			}
		}
	}
	rpcRateLimitFreiErgaenzen(ips)

	cs.leitung.Store(l)
	fmt.Printf("[LEITUNG] ✓ an: %d Validatoren (Mehrheit %d, Wahl %v), Term %d, Leiter %s, dieser Knoten %s (leiterfaehig %v), Wechsel alle %s\n",
		len(satz), len(satz)/2+1, len(satz) >= 3, l.Term(), func() string { a, _ := l.Leiter(); return a }(), ich, faehig, cfg.WechselAlle)
	SafeGoroutine("leitung-takt", func() { dag.leitungSchleife(l, cs) })
	return l
}

var leitungKlient = &http.Client{Timeout: 3 * time.Second}

var (
	leitungWeitergeleitet      atomic.Int64
	leitungWeiterleitungFehler atomic.Int64
)

func (dag *BlockDAG) leitungSchleife(l *Leitung, cs *ChainState) {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	letzteFaehigPruefung := time.Now()
	for range t.C {
		SafeCall("leitung-takt", func() {
			if time.Since(letzteFaehigPruefung) > time.Minute {
				letzteFaehigPruefung = time.Now()
				l.SetzeFaehig(!cs.nurLesend.Load() && leistungsnachweisErfuellt())
			}
			for _, m := range l.Takt(time.Now()) {
				dag.signiereLeitNachricht(&m)
				for _, u := range l.URLs() {
					go dag.leitungSende(l, u, m)
				}
			}
		})
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
	cs.walFlushMu.Unlock()
	for i := 0; i < 300 && cs.walFlushSem != nil && len(cs.walFlushSem) > 0; i++ {
		time.Sleep(100 * time.Millisecond)
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
func (cs *ChainState) weiterleitungsZiel(r *http.Request) string {
	l := cs.leitung.Load()
	if l == nil || r.Header.Get(weitergeleitetKopf) != "" {
		return ""
	}
	if cs.nimmtUeberweisungenAn() {
		return ""
	}
	addr, u := l.Leiter()
	if addr == "" || u == "" || addr == l.ich {
		return ""
	}
	return u
}

var weiterleitungsKlient = &http.Client{Timeout: 20 * time.Second}

// leiteWeiter schickt die Anfrage unveraendert an den Leiter und gibt dessen
// Antwort zurueck. false = hat nicht geklappt, selbst bearbeiten (das Tor
// antwortet dann mit einem wiederholbaren Fehler).
func leiteWeiter(w http.ResponseWriter, r *http.Request, ziel string, body []byte) bool {
	req, err := http.NewRequest(r.Method, ziel+r.URL.RequestURI(), bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	if auth := r.Header.Get("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	req.Header.Set(weitergeleitetKopf, "1")
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
		if r.Method != http.MethodPost {
			h(w, r)
			return
		}
		ziel := a.state.weiterleitungsZiel(r)
		if ziel == "" {
			h(w, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, `{"error":"unlesbar"}`, http.StatusBadRequest)
			return
		}
		if !rpcRateLimitFrei(r) && rpcRateLimited(clientIP(r)) {
			http.Error(w, `{"error":"rate limited"}`, http.StatusTooManyRequests)
			return
		}
		if leiteWeiter(w, r, ziel, body) {
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
