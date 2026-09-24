package keeper

import (
	"crypto/ecdsa"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestLeitNachricht_SignaturUndZeitfenster(t *testing.T) {
	key, _ := crypto.GenerateKey()
	dag := &BlockDAG{signingKey: key}
	von := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	jetzt := time.Now()
	m := LeitNachricht{Art: leitArtLease, Term: 3, Von: von, ZeitMs: jetzt.UnixMilli(), SatzHash: "x"}
	dag.signiereLeitNachricht(&m)
	if err := pruefeLeitNachricht(m, jetzt); err != nil {
		t.Fatalf("gueltige Nachricht abgewiesen: %v", err)
	}
	// Veraendert: Term hochgesetzt.
	f := m
	f.Term = 99
	if pruefeLeitNachricht(f, jetzt) == nil {
		t.Fatal("veraenderte Nachricht angenommen")
	}
	// Falscher Absender.
	f = m
	f.Von = "0x0000000000000000000000000000000000000001"
	if pruefeLeitNachricht(f, jetzt) == nil {
		t.Fatal("fremder Absender angenommen")
	}
	// Alt: ausserhalb des Zeitfensters (Wiedereinspielen).
	if pruefeLeitNachricht(m, jetzt.Add(time.Minute)) == nil {
		t.Fatal("alte Nachricht angenommen")
	}
	// Ohne Signatur.
	f = m
	f.Sig = ""
	if pruefeLeitNachricht(f, jetzt) == nil {
		t.Fatal("unsignierte Nachricht angenommen")
	}
}

func TestLeitungValidatorenAusUmgebung(t *testing.T) {
	satz, urls, err := leitungValidatorenAusUmgebung(
		" 0xBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=http://1.2.3.4:8080/ , 0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=http://5.6.7.8:8080")
	if err != nil || len(satz) != 2 || satz[0] != "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("satz %v err %v", satz, err)
	}
	if urls["0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"] != "http://1.2.3.4:8080" {
		t.Fatalf("urls %v", urls)
	}
	if _, _, err := leitungValidatorenAusUmgebung("keine-adresse=http://x"); err == nil {
		t.Fatal("ungueltige Adresse angenommen")
	}
}

// httpKnoten: ein Validator mit echtem HTTP-Server und echtem Schluessel.
type httpKnoten struct {
	key  *ecdsa.PrivateKey
	addr string
	cs   *ChainState
	dag  *BlockDAG
	srv  *httptest.Server
	l    *Leitung
	aus  atomic.Bool
}

type httpLeitungsNetz struct {
	t  *testing.T
	ks []*httpKnoten
}

// neuesHTTPLeitungsNetz: n Validatoren, alle registriert. genesis waehlt aus
// den (sortierten) Adressen den Genesis-Satz; die anderen kennen nur die
// URLs der Genesis-Validatoren als Peers -- wie ein neuer Validator, der nur
// seine Seeds kennt.
func neuesHTTPLeitungsNetz(t *testing.T, n int, genesisAnzahl int, cfg LeitKonfig) *httpLeitungsNetz {
	netz := &httpLeitungsNetz{t: t}
	for i := 0; i < n; i++ {
		key, _ := crypto.GenerateKey()
		k := &httpKnoten{key: key, addr: strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())}
		k.cs = newTestState()
		k.dag = &BlockDAG{signingKey: key, state: k.cs, blocks: map[string]*Block{}, replayedBlocks: map[string]bool{},
			authorizedValidators: map[string]bool{}}
		api := &APIServer{state: k.cs, blockchain: k.dag}
		mux := http.NewServeMux()
		mux.HandleFunc("/api/leitung", api.handleLeitung)
		addr := k.addr
		mux.HandleFunc("/rpc", func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"beim_leiter":"`+addr+`"}`)
		})
		k.srv = httptest.NewServer(mux)
		netz.ks = append(netz.ks, k)
	}
	sort.Slice(netz.ks, func(i, j int) bool { return netz.ks[i].addr < netz.ks[j].addr })
	var genesis, seeds []string
	for i, k := range netz.ks {
		if i < genesisAnzahl {
			genesis = append(genesis, k.addr)
			seeds = append(seeds, k.srv.URL)
		}
	}
	for i, k := range netz.ks {
		for _, o := range netz.ks {
			k.dag.authorizedValidators[o.addr] = true // alle registriert
		}
		env := LeitUmgebung{Hoehe: func() int64 { return 10 }, Entleert: func() bool { return true },
			Zugelassen: k.dag.istZugelassenerValidator}
		start := ""
		if len(genesis) > 0 {
			start = genesis[0]
		}
		g := genesis
		if i >= genesisAnzahl {
			g = nil // spaeter Hinzukommende kennen den Genesis-Satz nicht
		}
		k.l = NeueLeitung(k.addr, k.srv.URL, g, start, true, LeitSpeicher{}, cfg, env, time.Now())
		for j, o := range netz.ks {
			if j < genesisAnzahl && i < genesisAnzahl {
				k.l.SetzeURL(o.addr, o.srv.URL)
			}
		}
		k.cs.leitung.Store(k.l)
	}
	// Takt-Schleifen: wie leitungSchleife, schneller.
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	for _, k := range netz.ks {
		k := k
		go func() {
			tk := time.NewTicker(50 * time.Millisecond)
			defer tk.Stop()
			for {
				select {
				case <-stop:
					return
				case <-tk.C:
				}
				if k.aus.Load() {
					continue
				}
				k.dag.leitungVersenden(k.l, k.l.Takt(time.Now()), func() []string { return seeds })
			}
		}()
	}
	return netz
}

func (n *httpLeitungsNetz) annehmende() []int {
	var a []int
	for i, k := range n.ks {
		if !k.aus.Load() && k.cs.nimmtUeberweisungenAn() {
			a = append(a, i)
		}
	}
	return a
}

// beobachte: die ganze Zeit hoechstens einer annehmend.
func (n *httpLeitungsNetz) beobachte(d time.Duration) {
	ende := time.Now().Add(d)
	for time.Now().Before(ende) {
		if a := n.annehmende(); len(a) > 1 {
			n.t.Fatalf("ZWEI nehmen gleichzeitig an: %v", a)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// bisGenau: beobachten, bis genau einer (erfuellt ok) annimmt; hoechstens d.
func (n *httpLeitungsNetz) bisGenau(d time.Duration, ok func(i int) bool) int {
	ende := time.Now().Add(d)
	for time.Now().Before(ende) {
		a := n.annehmende()
		if len(a) > 1 {
			n.t.Fatalf("ZWEI nehmen gleichzeitig an: %v", a)
		}
		if len(a) == 1 && ok(a[0]) {
			return a[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	for i, k := range n.ks {
		n.t.Logf("%d: %v", i, k.l.Stand(time.Now()))
	}
	return -1
}

func (n *httpLeitungsNetz) ausfall(i int) {
	n.ks[i].aus.Store(true)
	n.ks[i].srv.Close()
}

func httpTestKonfig() LeitKonfig {
	return LeitKonfig{Takt: 100 * time.Millisecond, LeaseDauer: 800 * time.Millisecond,
		FolgerFrist: 1600 * time.Millisecond, Staffel: 600 * time.Millisecond, HoeheToleranz: 2,
		AufnahmeFrist: 3 * time.Second, AufnahmeToleranz: 50, AufnahmeSperre: 10 * time.Second}
}

// Drei Knoten mit echtem HTTP und echten Signaturen: der Startleiter nimmt
// an, faellt aus, ein anderer uebernimmt -- und nie nehmen zwei gleichzeitig
// an. Dazu die Weiterleitung: ein Folger schickt Annehmendes zum Leiter.
func TestLeitung_UeberHTTP(t *testing.T) {
	if testing.Short() {
		t.Skip("laeuft einige Sekunden in echter Zeit")
	}
	n := neuesHTTPLeitungsNetz(t, 3, 3, httpTestKonfig())
	n.beobachte(2 * time.Second)
	a := n.annehmende()
	if len(a) != 1 || a[0] != 0 {
		t.Fatalf("nach 2 s nehmen %v an, erwartet der Startleiter (kleinste Adresse)", a)
	}
	leiter := a[0]
	ks := n.ks

	// Weiterleitung: ein Folger schickt sendRawTransaction zum Leiter.
	folger := (leiter + 1) % 3
	req := httptest.NewRequest("POST", "/rpc", strings.NewReader(`{"method":"eth_sendRawTransaction"}`))
	ziel := ks[folger].cs.weiterleitungsZiel(req)
	if ziel != ks[leiter].srv.URL {
		t.Fatalf("Folger leitet an %q, erwartet %q", ziel, ks[leiter].srv.URL)
	}
	rec := httptest.NewRecorder()
	if !leiteWeiter(rec, req, ziel, []byte(`{"method":"eth_sendRawTransaction"}`)) || !strings.Contains(rec.Body.String(), ks[leiter].addr) {
		t.Fatalf("Weiterleitung kam nicht beim Leiter an: %q", rec.Body.String())
	}
	// Schon weitergeleitet: nie ein zweites Mal.
	req2 := httptest.NewRequest("POST", "/rpc", nil)
	req2.Header.Set(weitergeleitetKopf, "1")
	if ks[folger].cs.weiterleitungsZiel(req2) != "" {
		t.Fatal("eine schon weitergeleitete Anfrage wird erneut weitergeleitet")
	}

	// Leiter faellt aus. Uebernahme: FolgerFrist + Staffel + Vorwahl + Wahl
	// + erste Lease -- hier gut 3 s; 10 s Spielraum fuer den Race-Detector.
	n.ausfall(leiter)
	if n.bisGenau(10*time.Second, func(i int) bool { return i != leiter }) < 0 {
		t.Fatalf("10 s nach dem Ausfall nimmt keiner der beiden anderen an")
	}
}

// Dezentraler Beitritt ueber echtes HTTP: die Kette beginnt mit EINEM
// Validator. Zwei weitere kennen nur dessen URL (ihren Seed), setzen keine
// Liste, melden sich -- und werden aufgenommen. Danach faellt der
// Genesis-Validator aus, und die beiden Hinzugekommenen machen ohne ihn
// weiter.
func TestLeitung_BeitrittUeberHTTP(t *testing.T) {
	if testing.Short() {
		t.Skip("laeuft einige Sekunden in echter Zeit")
	}
	n := neuesHTTPLeitungsNetz(t, 3, 1, httpTestKonfig())
	ende := time.Now().Add(15 * time.Second)
	for {
		alle := true
		for _, k := range n.ks {
			st := k.l.Stand(time.Now())
			if st["mitglied"] != true || st["validatoren"] != 3 {
				alle = false
			}
		}
		if alle {
			break
		}
		if time.Now().After(ende) {
			for i, k := range n.ks {
				t.Logf("%d: %v", i, k.l.Stand(time.Now()))
			}
			t.Fatal("nach 15 s nicht alle drei aufgenommen")
		}
		n.beobachte(100 * time.Millisecond)
	}
	if a := n.annehmende(); len(a) != 1 || a[0] != 0 {
		t.Fatalf("nimmt %v an, erwartet der Genesis-Validator", a)
	}
	n.ausfall(0)
	if n.bisGenau(15*time.Second, func(i int) bool { return i != 0 }) < 0 {
		t.Fatal("ohne den Genesis-Validator uebernimmt keiner")
	}
}

func TestRpcSchreibt(t *testing.T) {
	if !rpcSchreibt([]byte(`{"jsonrpc":"2.0","method":"eth_sendRawTransaction","params":["0x"]}`)) {
		t.Fatal("sendRawTransaction gehoert zum Leiter")
	}
	if !rpcSchreibt([]byte(`[{"method":"eth_chainId"},{"method":"eth_getTransactionCount"}]`)) {
		t.Fatal("getTransactionCount gehoert zum Leiter (nur er kennt die vergebenen Nonces)")
	}
	if rpcSchreibt([]byte(`{"method":"eth_getBalance"}`)) {
		t.Fatal("Lesen bleibt beim Folger")
	}
}

// Swap & Co. gehen zum Leiter; GET und ein Knoten ohne Leitung bleiben lokal.
func TestZumLeiter(t *testing.T) {
	leiter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.Header.Get(weitergeleitetKopf) == "" {
			t.Error("Kopf fehlt beim Leiter")
		}
		io.WriteString(w, "leiter:"+string(b))
	}))
	defer leiter.Close()
	cs := newTestState()
	a := &APIServer{state: cs}
	lokal := func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		io.WriteString(w, "lokal:"+string(b))
	}
	h := a.zumLeiter(lokal)
	rufe := func(method string) string {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(method, "/api/swap", strings.NewReader("x")))
		return rec.Body.String()
	}
	if got := rufe("POST"); got != "lokal:x" {
		t.Fatalf("ohne Leitung: %q", got)
	}
	ich := "0x0000000000000000000000000000000000000002"
	anderer := "0x0000000000000000000000000000000000000001"
	l := NeueLeitung(ich, "", []string{ich, anderer}, anderer, true, LeitSpeicher{Term: 3, Leiter: anderer}, testKonfig(), LeitUmgebung{}, time.Now())
	l.SetzeURL(anderer, leiter.URL)
	cs.leitung.Store(l)
	if got := rufe("POST"); got != "leiter:x" {
		t.Fatalf("Folger mit Leitung: %q", got)
	}
	if got := rufe("GET"); got != "lokal:x" {
		t.Fatalf("GET: %q", got)
	}
}

// Mit echter Datenbank: der Leiter uebergibt erst, wenn nichts mehr offen ist;
// ein ueberholter Leiter verwirft genau das Unverteilte.
func TestLeitungEntleertUndVerwerfen_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-leitung-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	if !cs.leitungEntleert() {
		t.Fatal("leerer Knoten gilt nicht als entleert")
	}
	for i := 0; i < 3; i++ {
		tx := Transaction{Type: "transfer", Wallet: distTestAddr(720 + i), To: distTestAddr(820), Amount: 1, TxHash: "0xleitung-" + string(rune('a'+i))}
		if err := cs.SavePendingTx(tx); err != nil {
			t.Fatal(err)
		}
	}
	if cs.leitungEntleert() {
		t.Fatal("offene Annahmen im Ausgangskorb -- darf nicht uebergeben")
	}
	// Eine davon steht in einem gespeicherten Block.
	if _, err := cs.db.Exec(`UPDATE pending_txs SET included_at = 1, included_block_hash = '0xblock' WHERE tx_json LIKE '%leitung-a%'`); err != nil {
		t.Fatal(err)
	}
	cs.annahmenLaufend.Add(1)
	if cs.leitungEntleert() {
		t.Fatal("eine Annahme laeuft noch -- darf nicht uebergeben")
	}
	cs.annahmenLaufend.Add(-1)
	_, korb := cs.leitungUnverteiltVerwerfen()
	if korb != 2 {
		t.Fatalf("verworfen %d, erwartet genau die 2 unverteilten", korb)
	}
	var rest int
	cs.db.QueryRow(`SELECT count(*) FROM pending_txs`).Scan(&rest)
	if rest != 1 {
		t.Fatalf("es bleiben %d Zeilen, erwartet 1 (die im Block)", rest)
	}
	if !cs.leitungEntleert() {
		t.Fatal("nach dem Verwerfen nicht entleert")
	}
}
