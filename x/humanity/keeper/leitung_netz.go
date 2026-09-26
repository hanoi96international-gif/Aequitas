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

var leitungKlient = &http.Client{Timeout: 3 * time.Second}

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
//
// konten: die Konten, die die Anfrage belastet (anfrageKonten, rpcKonten).
// Stufe 2: Ziel ist ihr Zustaendiger. Gehoeren sie verschiedenen, gibt es
// kein gemeinsames Ziel -- dann bearbeitet dieser Knoten selbst, und das Tor
// lehnt ab, was er nicht annimmt.
func (cs *ChainState) weiterleitungsZiel(r *http.Request, konten ...string) string {
	l := cs.leitung.Load()
	if l == nil || r.Header.Get(weitergeleitetKopf) != "" {
		return ""
	}
	if cs.nimmtAnFuer(konten...) {
		return ""
	}
	var addr, u string
	if len(konten) == 0 {
		addr, u = l.Leiter()
	} else {
		addr, u = l.Zustaendig(konten[0])
		for _, k := range konten[1:] {
			if a, _ := l.Zustaendig(k); a != addr {
				return ""
			}
		}
	}
	if addr == "" || u == "" || addr == l.ich {
		return ""
	}
	return u
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
	case pfad == "/api/recover-escrow":
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
		ziel := a.state.weiterleitungsZiel(r, anfrageKonten(r.URL.Path, body)...)
		if ziel == "" {
			r.Body = io.NopCloser(bytes.NewReader(body))
			h(w, r)
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
func validatorIPsFrei(l *Leitung) {
	var ips []string
	for _, u := range l.URLs() {
		if pu, err := url.Parse(u); err == nil {
			if ip := net.ParseIP(pu.Hostname()); ip != nil {
				ips = append(ips, ip.String())
			}
		}
	}
	rpcRateLimitFreiErgaenzen(ips)
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
