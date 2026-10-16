package keeper

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// VIER ECHTE KNOTEN IN EINEM PROZESS (Beta-Bericht, Abschnitt 2: "Keine
// echten Tests mit 4 oder mehr Knoten", HOCH).
//
// Jeder Knoten ist so gebaut wie in cmd/aequitasd/main.go:
//
//	NewChainState (eigene Postgres-Datenbank aeq_vier_N)
//	NewBlockchain (eigener RELAYER_PRIVATE_KEY, gemeinsame AUTHORIZED_VALIDATORS)
//	LoadValidatorKeysIntoDAG, AnnahmeMessungBeginnen
//	NewAPIServer + serverKette(buildMux()) auf einem echten HTTP-Server
//	StarteLeitung (AEQUITAS_LEITUNG=an, gemeinsamer Genesis-Satz)
//
// Bloecke entstehen mit ProduceBlock, gehen per HTTPBroadcastBlock (echtes
// POST /api/blocks/push) an die anderen und werden zusaetzlich mit doSyncOnce
// (echtes GET /api/blocks...) nachgezogen. Ueberweisungen sind echte
// EIP-1559-Rohformen, eingereicht per POST /rpc eth_sendRawTransaction beim
// gerade gewaehlten Leiter.
//
// WAS DIESER TEST BEWUSST ANDERS MACHT ALS DER BETRIEB -- und warum keine
// dieser Stellen eine Pruefung abschaltet:
//
//   - httpSyncClient/txBatchClient sind fuer die Laufzeit des Tests ein
//     gewoehnlicher http.Client, leitungDialLokal ist an. Die Test-Server
//     lauschen auf 127.0.0.1, und pinningDialer weist Loopback zu Recht ab
//     (SSRF-Schutz). Genau so machen es push_reihenfolge_test.go,
//     eltern_grenze_test.go, challenge_orakel_test.go und leitung_dial_test.go;
//     der Schutz selbst hat eigene Tests. Keine Pruefung an einem Block, einer
//     Signatur, einer Nonce oder einer Deckung haengt am Klienten.
//   - Der Takt (Erzeugen, Schieben, Nachziehen) laeuft in einer Schleife des
//     Tests statt im block-production-ticker von main.go, damit er sich am
//     Testende anhalten laesst und der Lauf deterministisch endet. Er ruft
//     dieselben Funktionen.
//   - Die Leitung laeuft ueber leitungTaktStarten (der Haken existiert fuer
//     Tests) in einer anhaltbaren Schleife mit demselben Rumpf wie
//     leitungSchleife -- ohne Kappungen/Vorbehalte/Leistungsproben, die hinter
//     schlafenden Stichtagen liegen.
//
// PAKETWEITE ZUSTAENDE -- vier Knoten in einem Prozess teilen sie sich. Im
// Betrieb hat jeder Prozess seinen eigenen Satz. Der Test vermeidet die
// Faelle, in denen das die Aussage verfaelschen wuerde, statt sie
// abzuschalten:
//
//   - push_grenze.go:34/38 (blockPushPlaetze, blockPushJeIP): Push-Plaetze.
//     Behandelt mit vierDial (Absender-IP je Empfaenger); die globale Acht
//     bleibt geteilt, also strenger als im Betrieb.
//   - api.go:2005 (blockPushBreaker, je IP): der Angreifer schiebt von einer
//     eigenen IP (angreiferKlient).
//   - Die Replay-Mauer zaehlt seit 16.10.2026 je Knoten (dag.replayMauer),
//     nicht mehr paketweit -- kein Sonderfall mehr. pruefeFalscheBloecke
//     ersetzt die Heilung (os.Exit) durch einen Zaehler und verlangt, dass
//     Angriffsbloecke sie nie ausloesen.
//   - ahnen_unerreichbar.go:46 (ahnenLeerlaufFolgen): Runden ohne beschaffte
//     Vorfahren, ebenfalls mit os.Exit am Ende; im Prozess vierfach so schnell
//     voll. Mit vierDial (keine "busy"-Pushes, also kaum Waisen) wurde er in
//     keinem Lauf erreicht; ohne vierDial beendete er den ersten Versuch
//     nach zehn Sekunden.
//   - sync_blocks.go:225, tx_batch_transport.go:220 (httpSyncClient,
//     txBatchClient), leitung_netz.go:347 (leitungKlient): ein Klient fuer
//     alle -- siehe oben.
//   - sync_blocks.go:3511/3582/3646/3704: SELF_URL wird zur LAUFZEIT aus der
//     Umgebung gelesen; im Prozess gilt fuer alle vier die zuletzt gesetzte.
//     Betrifft nur den Hinweis auf die Rumpf-Quelle beim gestrippten Push und
//     die Resync-Schwelle, keine Pruefung.
//   - leistungsnachweis.go:249/257 (leistungGestartet): nur einmal je Prozess
//     gemessen; der Test setzt leistung.zwang wie AEQUITAS_LEITER_FAEHIG=ja.
//   - status_ohne_sperre.go:52 (statusZwischenspeicher): der "letzte gute"
//     /api/status ist paketweit -- ein beschaeftigter Knoten meldet dann den
//     Stand eines anderen. Der Test nimmt nur frisch gebaute Antworten.
//   - zustand_ablehnung.go:72, annahme_tor.go:100: reine Zaehler.
//
// NICHT ABGEDECKT:
//   - Snapshot-Resync: ein zurueckkehrender Ex-Leiter will den Zustand vom
//     neuen Leiter neu holen (leitung_netz.go:532), der Snapshot-Abruf geht
//     ueber pinningDialer und scheitert an Loopback (sync_blocks.go:205). Der
//     Knoten zieht im Test ueber Push und Sync nach, nicht ueber den Snapshot.
//   - libp2p-Gossip (p2p.go): die P2P-Knoten laufen ohne Start(), Bloecke
//     gehen nur ueber HTTP.
//   - Trusted Seeds (PRIMARY_NODE_URL): bewusst keine, damit FromSync nie
//     greift und jeder Block jede Pruefung durchlaeuft.
//   - Echte Prozessneustarts: "Neustart" heisst neue Instanz aus derselben
//     Datenbank; Hintergrundarbeiter der alten Instanz laufen gegen eine
//     geschlossene Datenbank aus (wie bei testKnoten).
func TestVierKnoten_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	if testing.Short() {
		t.Skip("laeuft in echter Zeit")
	}
	netz := neuesVierKnotenNetz(t)

	// (d) Validatorenliste: jeder meldet denselben Hash ueber /api/status.
	t.Run("ValidatorlisteGleich", func(t *testing.T) { netz.pruefeValidatorlisteGleich(t) })

	// (a) N Ueberweisungen beim Leiter, alle vier gleich.
	t.Run("UeberweisungenUndGleichstand", func(t *testing.T) {
		netz.ueberweisen(t, 12)
		netz.pruefeGleichstand(t, "nach den ersten Ueberweisungen")
	})

	// (c) Missbrauch: gefaelschte und fremd signierte Bloecke.
	t.Run("FalscheBloeckeAbgewiesen", func(t *testing.T) { netz.pruefeFalscheBloecke(t) })

	// (b) Der Leiter faellt aus: die anderen machen weiter, er zieht nach.
	t.Run("LeiterFaelltAusUndZiehtNach", func(t *testing.T) { netz.leiterAusfall(t) })
}

// ------------------------------------------------------------- Aufbau

const vierKnotenAnzahl = 4

type vierKnoten struct {
	i     int
	key   *ecdsa.PrivateKey
	addr  string
	dbURL string
	srv   *httptest.Server

	handler atomic.Pointer[http.Handler]
	aus     atomic.Bool // ausgefallen: kein HTTP, kein Takt

	mu  sync.Mutex // schuetzt cs/dag/l beim Neustart
	cs  *ChainState
	dag *BlockDAG
	l   *Leitung
	p2p *P2PNode

	stop chan struct{} // beendet Takt und Leitung dieser Instanz
	wg   sync.WaitGroup
}

func (k *vierKnoten) teile() (*ChainState, *BlockDAG, *Leitung) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.cs, k.dag, k.l
}

type vierKnotenNetz struct {
	t *testing.T
	// boese: ein fuenfter, ZUGELASSENER Validator, der nicht als Knoten
	// laeuft -- ein kompromittierter Schluessel. Er kann gueltige Koepfe
	// signieren; was er hineinschreibt, muss jeder Knoten selbst pruefen.
	boese  *ecdsa.PrivateKey
	ks     []*vierKnoten
	sender []testSchluessel
	nonce  []uint64
	empf   []string
	gesamt atomic.Int64 // angenommene Ueberweisungen
	// ruhe: kein Knoten erzeugt, alle ziehen nur nach. Mit vier Erzeugern
	// entstehen sonst laufend Zusammenfuehrungsbloecke (zwei Spitzen ->
	// jeder fuehrt zusammen -> wieder zwei Spitzen, so gewollt, main.go), und
	// ein Gleichstand laesst sich nie in einem Augenblick ablesen.
	ruhe    atomic.Bool
	erzeugt atomic.Int64
	ende    chan struct{}
}

// vierAdminURL: dieselbe Instanz wie DATABASE_URL, Datenbank "postgres".
func vierDBURL(t *testing.T, dbname string) string {
	t.Helper()
	u, err := url.Parse(os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("DATABASE_URL: %v", err)
	}
	u.Path = "/" + dbname
	return u.String()
}

// vierDatenbankFrisch legt aeq_vier_N leer neu an und das Schema hinein.
func vierDatenbankFrisch(t *testing.T, name string) string {
	t.Helper()
	admin, err := sql.Open("postgres", vierDBURL(t, "postgres"))
	if err != nil {
		t.Fatalf("Admin-Verbindung: %v", err)
	}
	defer admin.Close()
	if _, err := admin.Exec(`DROP DATABASE IF EXISTS ` + name + ` WITH (FORCE)`); err != nil {
		t.Fatalf("%s loeschen: %v", name, err)
	}
	if _, err := admin.Exec(`CREATE DATABASE ` + name); err != nil {
		t.Fatalf("%s anlegen: %v", name, err)
	}
	dbURL := vierDBURL(t, name)
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatalf("%s oeffnen: %v", name, err)
	}
	defer db.Close()
	// Nicht ensureRealDBSchema: das haengt an einem sync.Once fuer DATABASE_URL.
	(&ChainState{db: db, useDB: true}).initDB()
	return dbURL
}

// genesisKonten schreibt dieselben Startkonten in eine Datenbank -- auf allen
// vier Knoten identisch, wie ein Netz, das von derselben Genesis startet.
func genesisKonten(t *testing.T, dbURL string, adressen []string, guthaben float64) {
	t.Helper()
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cs := &ChainState{db: db, useDB: true}
	for _, a := range adressen {
		acc := &AccountState{Address: a, Balance: NewDecimal(guthaben), IsHuman: true, LastActivityAt: nowUnix()}
		if err := cs.saveAccountToDB(acc); err != nil {
			t.Fatalf("Genesis-Konto %s: %v", a, err)
		}
	}
}

func neuesVierKnotenNetz(t *testing.T) *vierKnotenNetz {
	t.Helper()
	netz := &vierKnotenNetz{t: t, ende: make(chan struct{})}

	// Die Regeln des Betriebs, nicht die des TestMain: signierte Ueberweisungen
	// und Wirtschaftsregeln nach ihren echten Stichtagen (0 = Konstante gilt).
	// TestMain schaltet beides fuer die alten Tests ab (wirtschaft_main_test.go).
	altSig, altWirt := signierteUeberweisungenOverride.Load(), wirtschaftAktivOverride.Load()
	signierteUeberweisungenOverride.Store(0)
	wirtschaftAktivOverride.Store(0)
	if !signierteUeberweisungenPflicht(nowUnix()) || !signierteUeberweisungenAufnehmen(nowUnix()) {
		t.Fatal("signierte Ueberweisungen sind nach der Uhr dieses Rechners nicht aktiv -- der Test prueft dann nicht, was er soll")
	}

	// Loopback-Klienten nur fuer diesen Test (siehe Dateikopf).
	echtSync, echtBatch := httpSyncClient, txBatchClient
	httpSyncClient = &http.Client{Timeout: 30 * time.Second, CheckRedirect: ohneUmleitung,
		Transport: &http.Transport{DialContext: vierDial, MaxIdleConnsPerHost: 4}}
	txBatchClient = httpSyncClient
	altDial := leitungDialLokal.Load() // leitung_dial_test.go setzt es per init()
	leitungDialLokal.Store(true)
	echtTakt := leitungTaktStarten

	// Schluessel: vier Validatoren, sortiert -- die kleinste Adresse leitet
	// den ersten Term (leitung_netz.go).
	for i := 0; i < vierKnotenAnzahl; i++ {
		key, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		netz.ks = append(netz.ks, &vierKnoten{key: key, addr: strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())})
	}
	sort.Slice(netz.ks, func(a, b int) bool { return netz.ks[a].addr < netz.ks[b].addr })

	boese, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	netz.boese = boese

	// Absender und Empfaenger der Ueberweisungen.
	var genesis []string
	for i := 0; i < 6; i++ {
		s := neuerTestSchluessel(t)
		netz.sender = append(netz.sender, s)
		netz.nonce = append(netz.nonce, 0)
		genesis = append(genesis, s.addr)
	}
	for i := 0; i < 3; i++ {
		netz.empf = append(netz.empf, fmt.Sprintf("0x7e%038x", i+1))
	}

	// Server zuerst: die URLs gehoeren in den Genesis-Satz der Leitung.
	var satz []string
	validatoren := []string{strings.ToLower(crypto.PubkeyToAddress(boese.PublicKey).Hex())}
	for i, k := range netz.ks {
		k := k
		k.i = i
		k.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if k.aus.Load() {
				// Ausgefallen: Verbindung ohne Antwort schliessen.
				if hj, ok := w.(http.Hijacker); ok {
					if c, _, err := hj.Hijack(); err == nil {
						c.Close()
						return
					}
				}
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			h := k.handler.Load()
			if h == nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			(*h).ServeHTTP(w, r)
		}))
		// Jeder Knoten auf seiner eigenen Loopback-Adresse (siehe vierDial).
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.%d:0", 11+i))
		if err != nil {
			t.Fatalf("Knoten %d: Listener: %v", i, err)
		}
		k.srv.Listener.Close()
		k.srv.Listener = ln
		k.srv.Start()
		k.dbURL = vierDatenbankFrisch(t, fmt.Sprintf("aeq_vier_%d", i))
		genesisKonten(t, k.dbURL, genesis, 1000)
		satz = append(satz, k.addr+"="+k.srv.URL)
		validatoren = append(validatoren, k.addr)
	}

	t.Setenv("AUTHORIZED_VALIDATORS", strings.Join(validatoren, ","))
	t.Setenv(leitungEnv, "an")
	t.Setenv(leitungGenesisEnv, strings.Join(satz, ","))
	t.Setenv(leiterFaehigEnv, "ja")
	t.Setenv("PRIMARY_NODE_URLS", "keine") // nie die oeffentlichen Seeds fragen
	t.Setenv("PRIMARY_NODE_URL", "")
	t.Setenv("PEER_NODES", "")
	t.Setenv(annahmeRolleEnv, "")
	// Der Leistungsnachweis ist paketweit und wird nur einmal je Prozess
	// gestartet (leistungsnachweis.go, leistungGestartet). Hat ein frueherer
	// Test ihn schon gestartet, wurde AEQUITAS_LEITER_FAEHIG damals gelesen.
	leistung.mu.Lock()
	altZwang := leistung.zwang
	leistung.zwang = "ja"
	leistung.mu.Unlock()

	// Leitungs-Takt anhaltbar (siehe Dateikopf).
	leitungTaktStarten = func(dag *BlockDAG, l *Leitung, cs *ChainState) {
		k := netz.knotenVon(dag)
		if k == nil {
			return
		}
		stop := k.stop
		k.wg.Add(1)
		go func() {
			defer k.wg.Done()
			tk := time.NewTicker(100 * time.Millisecond)
			defer tk.Stop()
			for {
				select {
				case <-stop:
					return
				case <-tk.C:
				}
				SafeCall("vier-leitung-takt", func() {
					l.SetzeStockt(cs.eigeneBloeckeStocken(time.Now().Unix()))
					dag.leitungVersenden(l, l.Takt(time.Now()), dag.leitungPeers)
				})
			}
		}()
	}

	t.Cleanup(func() {
		for _, k := range netz.ks {
			k.anhalten()
		}
		for _, k := range netz.ks {
			k.srv.Close()
		}
		leitungTaktStarten = echtTakt
		httpSyncClient, txBatchClient = echtSync, echtBatch
		leitungDialLokal.Store(altDial)
		leistung.mu.Lock()
		leistung.zwang = altZwang
		leistung.mu.Unlock()
		signierteUeberweisungenOverride.Store(altSig)
		wirtschaftAktivOverride.Store(altWirt)
	})

	for _, k := range netz.ks {
		netz.starte(t, k)
	}
	for _, k := range netz.ks {
		netz.taktStarten(k)
	}
	// Erster Leiter gewaehlt?
	if i := netz.warteAufLeiter(t, -1, 60*time.Second); i < 0 {
		t.Fatal("nach 60 s kein Leiter gewaehlt")
	}
	return netz
}

func (n *vierKnotenNetz) knotenVon(dag *BlockDAG) *vierKnoten {
	for _, k := range n.ks {
		k.mu.Lock()
		gleich := k.dag == dag
		k.mu.Unlock()
		if gleich {
			return k
		}
	}
	return nil
}

// starte baut (oder baut neu) die Instanz eines Knotens aus seiner
// Datenbank -- genau die Reihenfolge von main.go.
func (n *vierKnotenNetz) starte(t *testing.T, k *vierKnoten) {
	t.Helper()
	t.Setenv("DATABASE_URL", k.dbURL)
	t.Setenv("RELAYER_PRIVATE_KEY", hex.EncodeToString(crypto.FromECDSA(k.key)))
	t.Setenv("SELF_URL", k.srv.URL)
	cs := NewChainState(fmt.Sprintf("unused-vier-knoten-%d.json", k.i))
	if !cs.useDB {
		t.Fatalf("Knoten %d hat keine Datenbank", k.i)
	}
	if err := cs.PruefeWALRest(); err != nil {
		t.Fatalf("Knoten %d: %v", k.i, err)
	}
	dag := NewBlockchain(fmt.Sprintf("vier-%d", k.i), cs)
	if dag.selfProposer != k.addr {
		t.Fatalf("Knoten %d signiert als %s statt %s", k.i, dag.selfProposer, k.addr)
	}
	cs.LoadValidatorKeysIntoDAG(dag)
	cs.AnnahmeMessungBeginnen()
	// Ein echter libp2p-Knoten, wie main.go ihn baut (handleStatus liest
	// seine ID), auf einem freien Port und OHNE Start(): er waehlt sich nie
	// bei den oeffentlichen Bootstrap-Knoten ein. Bloecke laufen in diesem
	// Test ausschliesslich ueber HTTP, den Weg, um den es geht.
	t.Setenv("P2P_LISTEN_PORT", fmt.Sprint(freierPort(t)))
	p2p, err := NewP2PNode()
	if err != nil {
		t.Fatalf("Knoten %d: P2P: %v", k.i, err)
	}
	p2p.SetDAG(dag)
	api := NewAPIServer(dag, p2p, cs)
	var h http.Handler = serverKette(api.buildMux())

	// Peers: jeder kennt die drei anderen, wie nach der Peer-Anmeldung
	// (startSyncForPeer ohne seine Endlos-Schleife -- der Takt unten zieht nach).
	dag.syncPeerMu.Lock()
	for _, o := range n.ks {
		if o != k {
			dag.activeSyncPeers[o.srv.URL] = true
		}
	}
	dag.syncPeerMu.Unlock()

	k.mu.Lock()
	k.cs, k.dag, k.l, k.p2p = cs, dag, nil, p2p
	k.stop = make(chan struct{})
	k.mu.Unlock()
	k.handler.Store(&h)

	l := StarteLeitung(dag, cs, k.srv.URL)
	if l == nil {
		t.Fatalf("Knoten %d: Leitung nicht gestartet", k.i)
	}
	k.mu.Lock()
	k.l = l
	k.mu.Unlock()
}

// taktStarten: was der block-production-ticker in main.go tut, plus der
// regelmaessige Abgleich mit jedem Peer.
func (n *vierKnotenNetz) taktStarten(k *vierKnoten) {
	cs, dag, _ := k.teile()
	_ = cs
	k.mu.Lock()
	stop := k.stop
	k.mu.Unlock()
	k.wg.Add(1)
	go func() {
		defer k.wg.Done()
		// Versetzt wie verschiedene Rechner, nicht im Gleichschritt.
		select {
		case <-stop:
			return
		case <-time.After(time.Duration(k.i) * 100 * time.Millisecond):
		}
		tk := time.NewTicker(400 * time.Millisecond)
		defer tk.Stop()
		runde := 0
		for {
			select {
			case <-stop:
				return
			case <-tk.C:
			}
			runde++
			SafeCall("vier-takt", func() {
				if n.ruhe.Load() {
					// nur nachziehen
				} else if b := dag.ProduceBlock(); b != nil {
					n.erzeugt.Add(1)
					dag.HTTPBroadcastBlock(b)
				}
				if runde%3 == 0 || n.ruhe.Load() {
					for _, o := range n.ks {
						if o != k && !o.aus.Load() {
							dag.doSyncOnce(o.srv.URL)
						}
					}
				}
			})
		}
	}()
}

// anhalten: Takt und Leitung dieser Instanz enden, die Datenbank wird
// geschlossen (wie testKnoten). Die Instanz ist danach tot.
func (k *vierKnoten) anhalten() {
	k.mu.Lock()
	stop := k.stop
	cs := k.cs
	p2p := k.p2p
	k.stop = nil
	k.mu.Unlock()
	if stop == nil {
		return
	}
	close(stop)
	k.wg.Wait()
	// Laufende Pushes an diesen Knoten koennen noch einen Moment brauchen.
	pushFertigWarten(3 * time.Second)
	if p2p != nil {
		p2p.host.Close()
	}
	if cs != nil {
		cs.stopEVMMirrorFlushWorkerForTest()
		if cs.db != nil {
			cs.db.Close()
		}
	}
}

// vierDial verbindet von der Adresse des ZIELS aus (127.0.0.11 -> 127.0.0.11).
//
// WARUM. Die Push-Plaetze sind paketweit (push_grenze.go, blockPushPlaetze
// und blockPushJeIP): im Betrieb hat jeder Knoten seine eigenen acht Plaetze
// und je Absender-IP zwei; im selben Prozess teilen sich alle vier Knoten
// einen Satz, und alle Absender hiessen 127.0.0.1 -- dann hatte das ganze Netz
// zusammen ZWEI Push-Plaetze, und die Haelfte aller Pushes kam als "busy"
// zurueck. Mit dieser Bindung zaehlt jeder Empfaenger seine Absender unter
// einer eigenen IP, wie ein eigener Prozess. Die globale Grenze von acht
// bleibt geteilt -- strenger als im Betrieb, nie lockerer.
func vierDial(ctx context.Context, network, addr string) (net.Conn, error) {
	d := &net.Dialer{Timeout: 5 * time.Second}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			d.LocalAddr = &net.TCPAddr{IP: ip}
		}
	}
	return d.DialContext(ctx, network, addr)
}

func freierPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// pushFertigWarten: kurz warten, bis keine Push-Warteschlange mehr laeuft.
func pushFertigWarten(max time.Duration) {
	ende := time.Now().Add(max)
	for time.Now().Before(ende) {
		time.Sleep(50 * time.Millisecond)
	}
}

// ------------------------------------------------------------- Leiter

// leiterIndex: welcher Knoten nimmt gerade an (-1: keiner, -2: mehrere).
func (n *vierKnotenNetz) leiterIndex() int {
	gefunden := -1
	for i, k := range n.ks {
		if k.aus.Load() {
			continue
		}
		cs, _, _ := k.teile()
		if cs != nil && cs.nimmtUeberweisungenAn() {
			if gefunden >= 0 {
				return -2
			}
			gefunden = i
		}
	}
	return gefunden
}

// warteAufLeiter: bis genau ein Knoten annimmt (nicht nicht), hoechstens d.
// Zwei gleichzeitig Annehmende brechen den Test sofort ab.
func (n *vierKnotenNetz) warteAufLeiter(t *testing.T, nicht int, d time.Duration) int {
	t.Helper()
	ende := time.Now().Add(d)
	for time.Now().Before(ende) {
		i := n.leiterIndex()
		if i == -2 {
			t.Fatal("ZWEI Knoten nehmen gleichzeitig an")
		}
		if i >= 0 && i != nicht {
			return i
		}
		time.Sleep(50 * time.Millisecond)
	}
	for i, k := range n.ks {
		_, _, l := k.teile()
		if l != nil {
			t.Logf("Knoten %d (aus=%v): %v", i, k.aus.Load(), l.Stand(time.Now()))
		}
	}
	return -1
}

// ------------------------------------------------------------- Ueberweisungen

// rpc schickt eine JSON-RPC-Anfrage an einen Knoten.
func vierRPC(t *testing.T, basis, methode string, params ...interface{}) (json.RawMessage, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": methode, "params": params})
	resp, err := httpSyncClient.Post(basis+"/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err.Error()
	}
	defer resp.Body.Close()
	var out struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, raw)
	}
	if out.Error != nil {
		return nil, out.Error.Message
	}
	return out.Result, ""
}

func vierRohUeberweisung(t *testing.T, von testSchluessel, an string, wei *big.Int, nonce uint64) string {
	t.Helper()
	to := common.HexToAddress(an)
	cid := big.NewInt(chainIDJetzt())
	stx, err := types.SignNewTx(von.key, types.LatestSignerForChainID(cid), &types.DynamicFeeTx{
		ChainID: cid, Nonce: nonce, To: &to, Value: wei, Gas: 21000,
		GasFeeCap: big.NewInt(1), GasTipCap: big.NewInt(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := stx.MarshalBinary()
	return "0x" + hex.EncodeToString(raw)
}

// ueberweisen reicht anzahl Ueberweisungen beim aktuellen Leiter ein.
func (n *vierKnotenNetz) ueberweisen(t *testing.T, anzahl int) {
	t.Helper()
	for j := 0; j < anzahl; j++ {
		s := j % len(n.sender)
		wei := new(big.Int).Mul(big.NewInt(int64(1+j%5)), new(big.Int).Div(weiPerAEQ, big.NewInt(100))) // 0,01..0,05 AEQ
		roh := vierRohUeberweisung(t, n.sender[s], n.empf[j%len(n.empf)], wei, n.nonce[s])
		ende := time.Now().Add(30 * time.Second)
		for {
			i := n.warteAufLeiter(t, -1, 30*time.Second)
			if i < 0 {
				t.Fatalf("Ueberweisung %d: kein Leiter", j)
			}
			_, fehler := vierRPC(t, n.ks[i].srv.URL, "eth_sendRawTransaction", roh)
			if fehler == "" {
				n.nonce[s]++
				n.gesamt.Add(1)
				break
			}
			if time.Now().After(ende) {
				t.Fatalf("Ueberweisung %d beim Leiter %d abgelehnt: %s", j, i, fehler)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}

// ------------------------------------------------------------- Gleichstand

type vierStand struct {
	hoehe    int64
	spitzen  string
	root     string
	guthaben map[string]string
	nonces   map[string]uint64
	ausgang  int
}

func (n *vierKnotenNetz) standVon(k *vierKnoten) vierStand {
	cs, dag, _ := k.teile()
	st := vierStand{guthaben: map[string]string{}, nonces: map[string]uint64{}}
	st.hoehe = dag.heightSchnell.Load()
	tips := dag.GetTips()
	sort.Strings(tips)
	st.spitzen = strings.Join(tips, ",")
	st.root = cs.StateRoot()
	adressen := append([]string{}, n.empf...)
	for _, s := range n.sender {
		adressen = append(adressen, s.addr)
	}
	cs.mu.RLock()
	for _, a := range adressen {
		if acc, ok := cs.accounts.Get(a); ok {
			st.guthaben[a] = acc.Balance.String()
			st.nonces[a] = uint64(acc.NaechsteNonce)
		}
	}
	cs.mu.RUnlock()
	return st
}

// pruefeGleichstand: Erzeugung anhalten, nachziehen lassen, bis alle vier
// dasselbe sagen -- hoechstens 90 s.
func (n *vierKnotenNetz) pruefeGleichstand(t *testing.T, wann string) vierStand {
	t.Helper()
	// Erst alles Angenommene verblocken lassen, dann die Erzeugung anhalten.
	ende := time.Now().Add(60 * time.Second)
	for !n.ausgangLeer() {
		if time.Now().After(ende) {
			t.Fatalf("%s: der Ausgang des Leiters wird binnen 60 s nicht leer", wann)
		}
		time.Sleep(100 * time.Millisecond)
	}
	n.ruhe.Store(true)
	defer n.ruhe.Store(false)
	ende = time.Now().Add(90 * time.Second)
	var letzte []vierStand
	for time.Now().Before(ende) {
		letzte = letzte[:0]
		for _, k := range n.ks {
			if k.aus.Load() {
				continue
			}
			letzte = append(letzte, n.standVon(k))
		}
		if vierGleich(letzte) && n.alleUeberweisungenVerbucht(letzte[0]) {
			t.Logf("%s: %d Knoten gleich -- Hoehe %d, StateRoot %s, %d Ueberweisungen verbucht, %d Bloecke erzeugt",
				wann, len(letzte), letzte[0].hoehe, letzte[0].root, n.gesamt.Load(), n.erzeugt.Load())
			return letzte[0]
		}
		time.Sleep(500 * time.Millisecond)
	}
	for i, s := range letzte {
		t.Logf("Stand %d: Hoehe %d Spitzen %s Root %s Guthaben %v Nonces %v", i, s.hoehe, s.spitzen, s.root, s.guthaben, s.nonces)
	}
	t.Fatalf("%s: nach 90 s keine Einigkeit unter den Knoten", wann)
	return vierStand{}
}

// ausgangLeer: kein lebender Knoten hat noch Angenommenes, das in keinem
// Block steht.
func (n *vierKnotenNetz) ausgangLeer() bool {
	for _, k := range n.ks {
		if k.aus.Load() {
			continue
		}
		cs, _, _ := k.teile()
		var offen int
		if err := cs.db.QueryRow(`SELECT COUNT(*) FROM pending_txs WHERE included_at = 0`).Scan(&offen); err != nil || offen > 0 {
			return false
		}
	}
	return true
}

func vierGleich(st []vierStand) bool {
	if len(st) == 0 {
		return false
	}
	for _, s := range st[1:] {
		if s.hoehe != st[0].hoehe || s.spitzen != st[0].spitzen ||
			s.root != st[0].root || len(s.guthaben) != len(st[0].guthaben) {
			return false
		}
		for a, g := range st[0].guthaben {
			if s.guthaben[a] != g || s.nonces[a] != st[0].nonces[a] {
				return false
			}
		}
	}
	return true
}

// alleUeberweisungenVerbucht: die Summe der Nonces ist die Zahl der
// angenommenen Ueberweisungen -- erst dann ist nichts mehr unterwegs.
func (n *vierKnotenNetz) alleUeberweisungenVerbucht(st vierStand) bool {
	var summe uint64
	for _, s := range n.sender {
		summe += st.nonces[s.addr]
	}
	return summe == uint64(n.gesamt.Load())
}

// ------------------------------------------------------------- (d)

// frischerStatus: /api/status, aber nur eine frisch gebaute Antwort. Ist der
// Knoten beschaeftigt, liefert handleStatus den letzten guten Stand aus einem
// PAKETWEITEN Zwischenspeicher (status_ohne_sperre.go:52) -- im selben Prozess
// kann das der Stand eines anderen Knotens oder eines frueheren Tests sein.
func frischerStatus(t *testing.T, k *vierKnoten) map[string]interface{} {
	t.Helper()
	ende := time.Now().Add(30 * time.Second)
	for {
		resp, err := httpSyncClient.Get(k.srv.URL + "/api/status")
		if err != nil {
			t.Fatalf("Knoten %d /api/status: %v", k.i, err)
		}
		var st map[string]interface{}
		err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&st)
		resp.Body.Close()
		if err != nil {
			t.Fatalf("Knoten %d /api/status: %v", k.i, err)
		}
		if veraltet, _ := st["stand_veraltet"].(bool); !veraltet {
			return st
		}
		if time.Now().After(ende) {
			t.Fatalf("Knoten %d liefert 30 s lang nur den zwischengespeicherten Stand", k.i)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (n *vierKnotenNetz) pruefeValidatorlisteGleich(t *testing.T) {
	t.Helper()
	var erster string
	for i, k := range n.ks {
		st := frischerStatus(t, k)
		h, _ := st["validator_liste_hash"].(string)
		if h == "" {
			t.Fatalf("Knoten %d meldet keinen validator_liste_hash: %v", i, st)
		}
		_, dag, _ := k.teile()
		if h != dag.ValidatorListeHash() {
			t.Fatalf("Knoten %d: /api/status %s, intern %s", i, h, dag.ValidatorListeHash())
		}
		if i == 0 {
			erster = h
		} else if h != erster {
			t.Fatalf("Validatorenliste verschieden: Knoten 0 %s, Knoten %d %s", erster, i, h)
		}
	}
	t.Logf("Validatorenliste bei allen vier gleich: %s", erster)
}

// ------------------------------------------------------------- (c)

// vierBlock baut einen Block so, wie ProduceBlock ihn baut, und signiert ihn
// mit key. proposer darf von der Adresse von key abweichen (Faelschung).
func vierBlock(t *testing.T, key *ecdsa.PrivateKey, proposer string, eltern []*Block, txs []Transaction) *Block {
	t.Helper()
	var maxH int64 = -1
	var hashes []string
	var zeit int64
	for _, e := range eltern {
		hashes = append(hashes, e.Hash)
		if e.Height > maxH {
			maxH = e.Height
		}
		if e.Timestamp > zeit {
			zeit = e.Timestamp
		}
	}
	sort.Strings(hashes)
	if jetzt := nowUnix(); jetzt > zeit {
		zeit = jetzt
	}
	b := &Block{
		Height: maxH + 1, Timestamp: zeit, ParentHashes: hashes, Proposer: proposer,
		Humans: 0, Transactions: txs, StateRoot: "boese", ProducedAtMs: time.Now().UnixMilli(),
	}
	b.merkeTransaktionenJSON()
	b.TxRoot = txBatchRootJSON(b.transaktionenJSON(), len(b.Transactions))
	b.Hash = calculateBlockHash(b)
	h := common.HexToHash(b.Hash)
	sig, err := crypto.Sign(h[:], key)
	if err != nil {
		t.Fatal(err)
	}
	b.Signature = hex.EncodeToString(sig)
	return b
}

// angreiferKlient: eigene Absender-IP, damit die Abweisungen des Angreifers
// nicht der IP eines ehrlichen Absenders angerechnet werden (die Push-Sperre
// je IP ist paketweit, api.go blockPushBreaker).
var angreiferKlient = &http.Client{Timeout: 10 * time.Second, CheckRedirect: ohneUmleitung,
	Transport: &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		d := &net.Dialer{Timeout: 5 * time.Second, LocalAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.99")}}
		return d.DialContext(ctx, network, addr)
	}}}

// schiebe: POST /api/blocks/push wie ein Peer. Liefert ok und den Grund.
func vierSchiebe(t *testing.T, basis string, b *Block) (bool, string) {
	t.Helper()
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := angreiferKlient.Post(basis+"/api/blocks/push", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Push an %s: %v", basis, err)
	}
	defer resp.Body.Close()
	var antw blockPushResponse
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err := json.Unmarshal(raw, &antw); err != nil {
		return false, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, raw)
	}
	return antw.OK, antw.Reason
}

// spitzenBloecke: die aktuellen Spitzen eines Knotens als Bloecke.
func spitzenBloecke(t *testing.T, dag *BlockDAG) []*Block {
	t.Helper()
	var out []*Block
	dag.mu.RLock()
	for h := range dag.tips {
		if b := dag.blocks[h]; b != nil {
			kopie := *b
			out = append(out, &kopie)
		}
	}
	dag.mu.RUnlock()
	if len(out) == 0 {
		t.Fatal("keine Spitzen")
	}
	return out
}

// eingereichteUeberweisung: eine Ueberweisung, die schon in einem Block steht
// (fuer den Wiederholungsangriff) -- aus der Datenbank eines Knotens.
func eingereichteUeberweisung(t *testing.T, dag *BlockDAG) Transaction {
	t.Helper()
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	for _, b := range dag.blocks {
		for _, tx := range b.Transactions {
			if tx.Type == "transfer" && tx.Roh != "" {
				return tx
			}
		}
	}
	t.Fatal("kein verbuchter Transfer im Speicher-DAG")
	return Transaction{}
}

func (n *vierKnotenNetz) pruefeFalscheBloecke(t *testing.T) {
	t.Helper()
	vorher := n.pruefeGleichstand(t, "vor den Faelschungen")

	_, dag0, _ := n.ks[0].teile()
	eltern := spitzenBloecke(t, dag0)
	fremd, _ := crypto.GenerateKey()
	fremdAddr := strings.ToLower(crypto.PubkeyToAddress(fremd.PublicKey).Hex())
	boeseAddr := strings.ToLower(crypto.PubkeyToAddress(n.boese.PublicKey).Hex())
	opfer := n.sender[0]

	// Eine Ueberweisung vom Opfer an den Angreifer, die das Opfer nie
	// unterschrieben hat: Rohform vom Angreifer signiert, Opfer als Absender.
	diebstahl := signiere(t, testSchluessel{key: fremd, addr: fremdAddr}, fremdAddr, aeqWei(500), n.nonce[0], chainIDJetzt())
	diebstahl.Wallet = opfer.addr
	// Eine echte, schon verbuchte Ueberweisung noch einmal (Nonce verbraucht).
	wiederholt := eingereichteUeberweisung(t, dag0)

	manipuliert := vierBlock(t, n.boese, boeseAddr, eltern, nil)
	manipuliert.Transactions = []Transaction{diebstahl} // nach dem Signieren: Hash passt nicht mehr

	aktuelleEltern := func() []*Block {
		_, d, _ := n.ks[0].teile()
		return spitzenBloecke(t, d)
	}
	type fall struct {
		name  string
		bau   func() *Block
		ziele []*vierKnoten
		// replay: der Kopf ist echt, abgewiesen wird erst beim Nachspielen.
		// Diese Abweisungen zaehlen in der Replay-Mauer (replay_mauer.go).
		// Der Angreifer schiebt jeden solchen Block replayMauerSchwelle-mal an
		// jeden Knoten; die Heilung darf trotzdem nie anspringen, weil kein
		// Seed und kein anderer Erzeuger den Block fuehrt.
		replay bool
		b      *Block
	}
	faelle := []*fall{
		{name: "fremd signiert (Schluessel nicht zugelassen)", ziele: n.ks,
			bau: func() *Block { return vierBlock(t, fremd, fremdAddr, eltern, []Transaction{diebstahl}) }},
		{name: "falscher Erzeuger (zugelassene Adresse, fremde Signatur)", ziele: n.ks,
			bau: func() *Block { return vierBlock(t, fremd, n.ks[1].addr, eltern, []Transaction{diebstahl}) }},
		{name: "nach dem Signieren veraendert", ziele: n.ks, bau: func() *Block { return manipuliert }},
		{name: "zugelassener Erzeuger, Ueberweisung nicht vom Absender signiert", ziele: n.ks, replay: true,
			bau: func() *Block { return vierBlock(t, n.boese, boeseAddr, aktuelleEltern(), []Transaction{diebstahl}) }},
		{name: "zugelassener Erzeuger, verbrauchte Ueberweisung wiederholt", ziele: n.ks, replay: true,
			bau: func() *Block { return vierBlock(t, n.boese, boeseAddr, aktuelleEltern(), []Transaction{wiederholt}) }},
	}
	// Die Heilung endet in os.Exit -- hier nur zaehlen. Kein Angriffsblock
	// darf sie ausloesen.
	var heilungen atomic.Int64
	vorherHeilen := replayMauerHeilen
	replayMauerHeilen = func(_ *BlockDAG, grund string) {
		heilungen.Add(1)
		t.Errorf("Replay-Mauer heilt nach einem Angriffsblock: %s", grund)
	}
	defer func() { replayMauerHeilen = vorherHeilen }()

	for _, f := range faelle {
		f.b = f.bau()
		versuche := 1
		if f.replay {
			versuche = replayMauerSchwelle
		}
		for _, k := range f.ziele {
			for v := 0; v < versuche; v++ {
				ok, grund := vierSchiebe(t, k.srv.URL, f.b)
				if ok {
					t.Fatalf("%s: Knoten %d hat den Block angenommen", f.name, k.i)
				}
				_, dag, _ := k.teile()
				if dag.hasBlockInMemory(f.b.Hash) {
					t.Fatalf("%s: Knoten %d fuehrt den Block (%s) trotz Ablehnung", f.name, k.i, grund)
				}
				t.Logf("%s: Knoten %d lehnt ab (%s)", f.name, k.i, grund)
			}
		}
		if f.replay {
			// Ehrliche Bloecke setzen die Mauer zurueck (merkeBlockErfolg)
			// und heben die Hoehe fuer den naechsten Fall.
			n.ueberweisen(t, 1)
			n.pruefeGleichstand(t, "nach "+f.name)
		}
	}

	nachher := n.pruefeGleichstand(t, "nach den Faelschungen")
	// loeseHeilungAus prueft den Beleg in einer eigenen Goroutine.
	time.Sleep(500 * time.Millisecond)
	if h := heilungen.Load(); h != 0 {
		t.Fatalf("Replay-Mauer hat %d-mal geheilt", h)
	}

	// MISSBRAUCH der Replay-Mauer (bis 16.10.2026 je HOEHE gezaehlt): der
	// zugelassene Erzeuger schiebt replayMauerSchwelle VERSCHIEDENE Bloecke
	// derselben Hoehe an jeden Knoten, ohne ehrlichen Block dazwischen. Der
	// alte Zaehler loeste damit Heilung samt os.Exit aus.
	mauerEltern := aktuelleEltern()
	varianten := [][]Transaction{{diebstahl}, {wiederholt}, {diebstahl, wiederholt}}
	vorAbweisungen := make([]int64, len(n.ks))
	for i, k := range n.ks {
		_, d, _ := k.teile()
		vorAbweisungen[i] = d.replayMauer.abweisungen.Load()
	}
	var mauerBloecke []*Block
	for _, txs := range varianten[:replayMauerSchwelle] {
		b := vierBlock(t, n.boese, boeseAddr, mauerEltern, txs)
		mauerBloecke = append(mauerBloecke, b)
		for _, k := range n.ks {
			if ok, _ := vierSchiebe(t, k.srv.URL, b); ok {
				t.Fatalf("Mauer-Angriff: Knoten %d hat Block #%d angenommen", k.i, b.Height)
			}
		}
	}
	for i := 1; i < len(mauerBloecke); i++ {
		if mauerBloecke[i].Height != mauerBloecke[0].Height || mauerBloecke[i].Hash == mauerBloecke[0].Hash {
			t.Fatalf("Mauer-Angriff braucht verschiedene Bloecke derselben Hoehe: #%d %s vs #%d %s",
				mauerBloecke[0].Height, mauerBloecke[0].Hash, mauerBloecke[i].Height, mauerBloecke[i].Hash)
		}
	}
	time.Sleep(500 * time.Millisecond) // loeseHeilungAus laeuft in einer Goroutine
	// Gegenprobe: jeder Knoten hat sie wirklich beim Nachspielen abgewiesen
	// -- sonst bewiese "keine Heilung" nichts.
	for i, k := range n.ks {
		_, d, _ := k.teile()
		if neu := d.replayMauer.abweisungen.Load() - vorAbweisungen[i]; neu < replayMauerSchwelle {
			t.Fatalf("Mauer-Angriff: Knoten %d wies nur %d Bloecke beim Nachspielen ab, erwartet %d -- "+
				"der Test zeigt den Angriff nicht", k.i, neu, replayMauerSchwelle)
		}
	}
	if h := heilungen.Load(); h != 0 {
		t.Fatalf("Mauer-Angriff: Replay-Mauer hat %d-mal geheilt", h)
	}
	t.Logf("Mauer-Angriff: %d verschiedene Bloecke der Hoehe %d an jedem Knoten abgewiesen, keine Heilung",
		len(mauerBloecke), mauerBloecke[0].Height)
	n.ueberweisen(t, 1)
	nachher = n.pruefeGleichstand(t, "nach dem Mauer-Angriff")
	// Kein Knoten hat irgendetwas vom Angreifer verbucht: das Opfer verlor nur
	// durch eigene, echte Ueberweisungen, der Angreifer bekam nichts.
	for i, k := range n.ks {
		cs, dag, _ := k.teile()
		for _, f := range faelle {
			if dag.hasBlockInMemory(f.b.Hash) {
				t.Fatalf("Knoten %d fuehrt am Ende den Block %q", i, f.name)
			}
		}
		cs.mu.RLock()
		acc, hat := cs.accounts.Get(fremdAddr)
		cs.mu.RUnlock()
		if hat && acc.Balance.Float() != 0 {
			t.Fatalf("Knoten %d: der Angreifer hat %s AEQ", i, acc.Balance.String())
		}
	}
	if nachher.nonces[opfer.addr] != uint64(n.nonce[0]) {
		t.Fatalf("Nonce des Opfers %d, erwartet %d", nachher.nonces[opfer.addr], n.nonce[0])
	}
	t.Logf("alle Faelschungen abgewiesen; StateRoot vorher %s, nachher %s (dazwischen nur ehrliche Ueberweisungen)",
		vorher.root, nachher.root)
}

// ------------------------------------------------------------- (b)

func (n *vierKnotenNetz) leiterAusfall(t *testing.T) {
	t.Helper()
	alt := n.warteAufLeiter(t, -1, 30*time.Second)
	if alt < 0 {
		t.Fatal("kein Leiter vor dem Ausfall")
	}
	k := n.ks[alt]
	t.Logf("Leiter ist Knoten %d (%s) -- er faellt jetzt aus", alt, k.addr)
	k.aus.Store(true)
	k.anhalten()

	neu := n.warteAufLeiter(t, alt, 90*time.Second)
	if neu < 0 {
		t.Fatal("nach dem Ausfall des Leiters uebernimmt binnen 90 s niemand")
	}
	t.Logf("Knoten %d uebernimmt die Leitung", neu)
	n.ueberweisen(t, 8)
	ohne := n.pruefeGleichstand(t, "drei Knoten ohne den alten Leiter")

	// Rueckkehr: dieselbe Datenbank, neue Instanz -- ein Neustart.
	k.aus.Store(false)
	n.starte(t, k)
	n.taktStarten(k)
	mit := n.pruefeGleichstand(t, "der alte Leiter ist zurueck und hat nachgezogen")
	if mit.hoehe < ohne.hoehe {
		t.Fatalf("Hoehe nach der Rueckkehr %d < %d", mit.hoehe, ohne.hoehe)
	}
	// Und es geht mit allen vier weiter.
	n.ueberweisen(t, 4)
	n.pruefeGleichstand(t, "nach der Rueckkehr weiter mit vier Knoten")

	// Ein Folger faellt aus und kommt zurueck.
	folger := (n.warteAufLeiter(t, -1, 30*time.Second) + 1) % len(n.ks)
	f := n.ks[folger]
	f.aus.Store(true)
	f.anhalten()
	n.ueberweisen(t, 4)
	n.pruefeGleichstand(t, "ohne einen Folger")
	f.aus.Store(false)
	n.starte(t, f)
	n.taktStarten(f)
	n.pruefeGleichstand(t, "der Folger ist zurueck")
}
