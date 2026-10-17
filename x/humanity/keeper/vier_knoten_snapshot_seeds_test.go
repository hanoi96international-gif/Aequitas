package keeper

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// SNAPSHOT-RESYNC UND TRUSTED SEEDS MIT VIER ECHTEN KNOTEN -- Teil von
// TestVierKnoten_RealDB (vier_knoten_realdb_test.go: dasselbe Netz, dieselben
// Datenbanken, derselbe Takt). Was dort im Dateikopf als "nicht abgedeckt"
// stand:
//
//   - Snapshot-Resync ueber Loopback (snapshotResyncNachLoeschung): ein
//     Knoten faellt aus, verliert seine Datenbank, das Netz laeuft weiter. Er
//     kommt leer zurueck und holt den Zustand ueber dag.PerformResync --
//     denselben Weg wie leitungUeberholt und die Selbstheilung -- aus dem
//     echten GET /api/snapshot des Leiters, samt Checkpoint-Block
//     (SeedTrustedCheckpoint) und Brueckenschlag; den Rest per Push/Sync.
//     Der Abruf waehlt im Test ueber snapshotDialLokal statt pinningDialer
//     (snapshot.go); jede Pruefung am Snapshot bleibt.
//   - Missbrauch des Snapshots (snapshotMissbrauch): genau das, was
//     fetchAndValidateSnapshot und ResyncFromSnapshotURL pruefen --
//     Unterzeichner, Signatur ueber den ganzen Inhalt (auch die Hoehe),
//     fehlende Signatur, Alter, Pflicht eines erwarteten Unterzeichners. Eine
//     StateRoot traegt der Snapshot NICHT; gedeckt wird er allein durch die
//     Unterschrift des erwarteten Knotens. Das zeigt der Fall "manipuliert".
//   - Trusted Seeds (trustedSeedsFromSync): ein Knoten, der einen Peer in
//     PEER_NODES eintraegt (trustedSeedsSetzen, wie StartPeerDiscovery),
//     markiert dessen Sync-Bloecke als FromSync, ein Knoten ohne diesen
//     Eintrag nicht. Sichtbar ueber die Replay-Mauer (mauerBelegt): nur
//     FromSync gilt dort als Beleg. Signaturen und Deckung entbindet FromSync
//     nie -- auch ein Seed bringt keine gefaelschte Ueberweisung durch.

// vierKlientVon: ein Klient mit eigener Absender-IP. Der oeffentliche
// Snapshot ist je IP gedrosselt (handleSnapshot, 30 s, paketweit); die
// Knoten selbst holen ueber 127.0.0.1.
func vierKlientVon(ip string) *http.Client {
	return &http.Client{Timeout: 60 * time.Second, CheckRedirect: ohneUmleitung,
		Transport: &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			d := &net.Dialer{Timeout: 5 * time.Second, LocalAddr: &net.TCPAddr{IP: net.ParseIP(ip)}}
			return d.DialContext(ctx, network, addr)
		}}}
}

// vierServerAuf startet einen httptest-Server auf einer eigenen
// Loopback-Adresse -- ein Peer ausserhalb der vier Knoten.
func vierServerAuf(t *testing.T, ip string, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	ln, err := net.Listen("tcp", ip+":0")
	if err != nil {
		t.Fatalf("Listener auf %s: %v", ip, err)
	}
	srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// echterSnapshot holt den oeffentlichen Snapshot eines Knotens roh.
func echterSnapshot(t *testing.T, k *vierKnoten, klient *http.Client) *StateSnapshot {
	t.Helper()
	for versuch := 0; ; versuch++ {
		resp, err := klient.Get(k.srv.URL + "/api/snapshot")
		if err != nil {
			t.Fatalf("Snapshot von Knoten %d: %v", k.i, err)
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests && versuch < 2 {
			time.Sleep(snapshotRetryWait)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Snapshot von Knoten %d: HTTP %d %s", k.i, resp.StatusCode, raw)
		}
		var snap StateSnapshot
		if err := json.Unmarshal(raw, &snap); err != nil {
			t.Fatalf("Snapshot von Knoten %d: %v", k.i, err)
		}
		return &snap
	}
}

// snapshotSignieren: wie ExportSnapshot -- SHA256 ueber das JSON ohne
// Signatur.
func snapshotSignieren(t *testing.T, s *StateSnapshot, key *ecdsa.PrivateKey) {
	t.Helper()
	s.Signature = ""
	body, _ := json.Marshal(s)
	h := sha256.Sum256(body)
	sig, err := crypto.Sign(h[:], key)
	if err != nil {
		t.Fatal(err)
	}
	s.Signature = hex.EncodeToString(sig)
}

func vierKonto(cs *ChainState, addr string) (*AccountState, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	acc, ok := cs.accounts.Get(addr)
	if !ok {
		return nil, false
	}
	kopie := *acc
	return &kopie, true
}

// ------------------------------------------------------------- Snapshot: Missbrauch

func (n *vierKnotenNetz) snapshotMissbrauch(t *testing.T) {
	t.Helper()
	leiter := n.warteAufLeiter(t, -1, 30*time.Second)
	if leiter < 0 {
		t.Fatal("kein Leiter")
	}
	seed := n.ks[leiter]
	ziel := n.ks[(leiter+1)%len(n.ks)]
	fremderKnoten := n.ks[(leiter+2)%len(n.ks)]
	zielCS, zielDag, _ := ziel.teile()

	fremd, _ := crypto.GenerateKey()
	dieb := strings.ToLower(crypto.PubkeyToAddress(fremd.PublicKey).Hex())
	boeseAddr := strings.ToLower(crypto.PubkeyToAddress(n.boese.PublicKey).Hex())

	// Der echte, vom Seed unterschriebene Snapshot -- Grundlage jeder
	// Faelschung unten.
	echt := echterSnapshot(t, seed, vierKlientVon("127.0.0.98"))
	if echt.Signature == "" {
		t.Fatal("der Seed unterschreibt seinen Snapshot nicht")
	}
	kopie := func() *StateSnapshot {
		raw, _ := json.Marshal(echt)
		var s StateSnapshot
		if err := json.Unmarshal(raw, &s); err != nil {
			t.Fatal(err)
		}
		return &s
	}
	mitDieb := func(s *StateSnapshot) *StateSnapshot {
		s.Accounts = append(s.Accounts, &AccountState{Address: dieb, Balance: NewDecimal(1_000_000), IsHuman: true, LastActivityAt: nowUnix()})
		return s
	}

	manipuliert := mitDieb(kopie())
	ohneSig := kopie()
	ohneSig.Signature = ""
	neuSigniert := mitDieb(kopie())
	snapshotSignieren(t, neuSigniert, fremd)
	vomValidator := mitDieb(kopie())
	snapshotSignieren(t, vomValidator, n.boese)
	hoeher := kopie()
	hoeher.Height += 1_000_000 // die Hoehe ist mit unterschrieben (Rueckschritt-Schutz)

	ausliefern := map[string]*StateSnapshot{
		"unveraendert":  kopie(),
		"manipuliert":   manipuliert,
		"ohne-signatur": ohneSig,
		"neu-signiert":  neuSigniert,
		"vom-validator": vomValidator,
		"hoehe":         hoeher,
		"veraltet":      kopie(),
	}
	var mu sync.Mutex
	// Der boese Peer: liefert unter /<fall>/api/snapshot, was er will.
	boeserPeer := vierServerAuf(t, "127.0.0.21", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fall := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), "/api/snapshot")
		mu.Lock()
		s := ausliefern[fall]
		mu.Unlock()
		if s == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s)
	}))

	// Gegenprobe: unveraendert ueber denselben Umweg besteht die echte
	// Pruefung. Sonst bewiese jede Abweisung unten nur, dass der Umweg kaputt
	// ist.
	if _, err := fetchAndValidateSnapshot(boeserPeer.URL+"/unveraendert/api/snapshot", seed.addr); err != nil {
		t.Fatalf("Gegenprobe: der unveraenderte Snapshot des Seeds wird abgewiesen: %v", err)
	}

	faelle := []struct {
		name, url, signer, grund string
		vorher                   func(t *testing.T)
	}{
		{name: "anderer ehrlicher Knoten statt des erwarteten",
			url: fremderKnoten.srv.URL + "/api/snapshot", signer: seed.addr, grund: "expected " + seed.addr},
		{name: "nach dem Signieren veraendert",
			url: boeserPeer.URL + "/manipuliert/api/snapshot", signer: seed.addr, grund: "expected " + seed.addr},
		{name: "Signatur entfernt",
			url: boeserPeer.URL + "/ohne-signatur/api/snapshot", signer: seed.addr, grund: "no signature"},
		{name: "vom Angreifer neu signiert",
			url: boeserPeer.URL + "/neu-signiert/api/snapshot", signer: seed.addr, grund: "signed by " + dieb},
		{name: "von zugelassenem kompromittiertem Validator signiert",
			url: boeserPeer.URL + "/vom-validator/api/snapshot", signer: seed.addr, grund: "signed by " + boeseAddr},
		{name: "Hoehe nach dem Signieren angehoben",
			url: boeserPeer.URL + "/hoehe/api/snapshot", signer: seed.addr, grund: "expected " + seed.addr},
		{name: "ohne erwarteten Unterzeichner",
			url: seed.srv.URL + "/api/snapshot", signer: "", grund: "requires BOOTSTRAP_SIGNER"},
		{name: "echter Snapshot aber veraltet",
			url: boeserPeer.URL + "/veraltet/api/snapshot", signer: seed.addr, grund: "too old",
			vorher: func(t *testing.T) {
				// Ein echter Snapshot von gestern ist ebenso unterschrieben wie
				// einer von heute; nur das Alter haelt seine Wiederholung auf.
				// Statt einen Tag zu warten: die Grenze fuer diesen Fall auf
				// eine Sekunde (strenger, nicht lockerer).
				t.Setenv("SNAPSHOT_MAX_AGE_SECONDS", "1")
				time.Sleep(2100 * time.Millisecond)
			}},
	}

	// Der oeffentliche Snapshot ist je Absender-IP 30 s gedrosselt
	// (handleSnapshot), und die Knoten holen alle von 127.0.0.1 (Loopback-
	// Route). Im Fall LeiterFaelltAusUndZiehtNach hat der ueberholte Leiter
	// eben erst so geholt, der erste Fall hier laeuft also in die Drosselung
	// und PerformResync wartet sie ab. Frueher geschah das mit gesetztem
	// resyncInProgress -- 35 s ohne jeden Block, laenger als die
	// Finalitaetstiefe, und die eigenen letzten Bloecke blieben als toter
	// Zweig stehen; darum stand hier ein Warten von 31 s. Seit der Abruf vor
	// dem Tor laeuft (autoheal.go), ist das der Missbrauchsfall selbst: ein
	// Beobachter haelt fest, ob das Tor waehrend eines Falls je zu war. Keiner
	// der Snapshots ist gueltig, also darf es nie zu sein. Die Drosselung
	// selbst bleibt unangetastet.

	importHoeheVorher := zielCS.getConfigValueDB("snapshot_import_height")
	for _, f := range faelle {
		t.Run(f.name, func(t *testing.T) {
			if f.vorher != nil {
				f.vorher(t)
			}
			var torZu atomic.Bool
			stop := make(chan struct{})
			beobachtet := make(chan struct{})
			go func() {
				defer close(beobachtet)
				for {
					select {
					case <-stop:
						return
					case <-time.After(time.Millisecond):
						if zielDag.resyncInProgress.Load() {
							torZu.Store(true)
						}
					}
				}
			}()
			start := time.Now()
			err := zielDag.PerformResync(f.url, f.signer, seed.srv.URL)
			close(stop)
			<-beobachtet
			if torZu.Load() {
				t.Fatalf("Knoten %d war waehrend des Abrufs taub (resyncInProgress gesetzt, %s), obwohl der Snapshot ungueltig war",
					ziel.i, time.Since(start).Round(time.Millisecond))
			}
			if err == nil {
				t.Fatalf("Knoten %d hat den Snapshot uebernommen", ziel.i)
			}
			if !strings.Contains(err.Error(), f.grund) {
				t.Fatalf("Knoten %d weist ab, aber aus anderem Grund: %v (erwartet %q)", ziel.i, err, f.grund)
			}
			t.Logf("Knoten %d lehnt ab: %v", ziel.i, err)
			if zielDag.resyncInProgress.Load() {
				t.Fatal("resyncInProgress bleibt nach der Abweisung gesetzt")
			}
		})
	}

	// Keiner der Versuche hat den Zustand beruehrt.
	if _, hat := vierKonto(zielCS, dieb); hat {
		t.Fatalf("Knoten %d fuehrt das Konto des Diebs", ziel.i)
	}
	if h := zielCS.getConfigValueDB("snapshot_import_height"); h != importHoeheVorher {
		t.Fatalf("snapshot_import_height %q -> %q: ein abgewiesener Snapshot hat geschrieben", importHoeheVorher, h)
	}
	n.ueberweisen(t, 2)
	n.pruefeGleichstand(t, "nach den Snapshot-Faelschungen")
}

// ------------------------------------------------------------- Snapshot: Resync

func (n *vierKnotenNetz) snapshotResyncNachLoeschung(t *testing.T) {
	t.Helper()
	// Ein Heilungsruf waere hier ein Befund: ein frisch aus dem Snapshot
	// geholter Knoten muss ehrliche Bloecke nachspielen koennen.
	vorherHeilen, vorherAhnen := replayMauerHeilen, ahnenLeerlaufHeilen
	replayMauerHeilen = func(_ *BlockDAG, grund string) { t.Errorf("Replay-Mauer heilt nach dem Snapshot-Resync: %s", grund) }
	ahnenLeerlaufHeilen = func(_ *BlockDAG, grund string) { t.Errorf("Ahnen-Leerlauf heilt nach dem Snapshot-Resync: %s", grund) }
	defer func() { replayMauerHeilen, ahnenLeerlaufHeilen = vorherHeilen, vorherAhnen }()

	leiter := n.warteAufLeiter(t, -1, 30*time.Second)
	if leiter < 0 {
		t.Fatal("kein Leiter")
	}
	k := n.ks[(leiter+1)%len(n.ks)]
	t.Logf("Knoten %d faellt aus und verliert seine Datenbank", k.i)
	k.aus.Store(true)
	k.anhalten()

	// Das Netz laeuft ohne ihn weiter.
	n.ueberweisen(t, 10)
	ohne := n.pruefeGleichstand(t, "drei Knoten, der vierte ist geloescht")

	// Leer zurueck: neue Datenbank, nur das Schema -- keine Genesis-Konten,
	// keine Bloecke. k.aus bleibt gesetzt, bis der Zustand da ist: Pushes an
	// einen Knoten ohne Zustand scheiterten nur am Nachspielen.
	if got := vierDatenbankFrisch(t, k.dbName); got != k.dbURL {
		t.Fatalf("Datenbank-URL %s statt %s", got, k.dbURL)
	}
	n.starte(t, k)
	cs, dag, _ := k.teile()
	for _, s := range n.sender {
		if _, hat := vierKonto(cs, s.addr); hat {
			t.Fatalf("Knoten %d kennt nach dem Loeschen noch %s", k.i, s.addr)
		}
	}

	// Vom aktuellen Leiter, mit dessen Adresse als Unterzeichner -- genau
	// wie leitungUeberholt. Der Seed ist der Leiter, weil er der Knoten ist,
	// dem ein ueberholter Knoten in diesem Netz vertraut.
	quelle := n.ks[n.warteAufLeiter(t, k.i, 30*time.Second)]
	t.Logf("Resync von Knoten %d (%s)", quelle.i, quelle.addr)
	if err := dag.PerformResync(quelle.srv.URL+"/api/snapshot", quelle.addr, quelle.srv.URL); err != nil {
		t.Fatalf("Snapshot-Resync von Knoten %d: %v", quelle.i, err)
	}

	// Der Zustand kam aus dem Snapshot, nicht aus Bloecken: der Takt laeuft
	// noch nicht, HTTP ist zu -- und doch stehen die Absender mit ihren
	// Noncen da.
	var nonceSumme uint64
	for _, s := range n.sender {
		acc, hat := vierKonto(cs, s.addr)
		if !hat {
			t.Fatalf("Knoten %d: %s fehlt nach dem Snapshot", k.i, s.addr)
		}
		nonceSumme += uint64(acc.NaechsteNonce)
	}
	if nonceSumme == 0 {
		t.Fatal("nach dem Snapshot hat kein Absender eine Nonce -- der Zustand kam nicht aus dem Snapshot")
	}
	importHoehe := cs.getConfigValueDB("snapshot_import_height")
	if importHoehe == "" || importHoehe == "0" {
		t.Fatalf("snapshot_import_height %q -- ohne sie spielte der Knoten jeden Block ein zweites Mal", importHoehe)
	}
	t.Logf("Knoten %d nach dem Snapshot: Noncen-Summe %d (Netz: %d), Snapshot-Hoehe %s, Hoehe vorher im Netz %d",
		k.i, nonceSumme, n.gesamt.Load(), importHoehe, ohne.hoehe)

	k.aus.Store(false)
	n.taktStarten(k)
	mit := n.pruefeGleichstand(t, "der geloeschte Knoten ist ueber den Snapshot zurueck")
	if mit.hoehe < ohne.hoehe {
		t.Fatalf("Hoehe nach dem Resync %d < %d", mit.hoehe, ohne.hoehe)
	}
	// Und er traegt weiter mit.
	n.ueberweisen(t, 4)
	n.pruefeGleichstand(t, "nach dem Snapshot-Resync weiter mit vier Knoten")
}

// ------------------------------------------------------------- Trusted Seeds

// trustedSeedsFromSync: ein Peer ausserhalb des Netzes liefert per
// GET /api/blocks einen Block des zugelassenen, kompromittierten Validators
// mit einer Ueberweisung, die der Absender nie unterschrieben hat. Knoten A
// traegt diesen Peer als Seed ein (PEER_NODES), Knoten B nicht. Beide ziehen
// ihn replayMauerSchwelle-mal per doSyncOnce.
//
//   - Beide weisen den Block beim Nachspielen ab: FromSync entbindet keine
//     Signatur.
//   - A: der Block kam vom Seed -> FromSync -> die Replay-Mauer hat ihren
//     Beleg und ruft die Heilung (hier nur gezaehlt).
//   - B: derselbe Block vom selben Peer, aber kein Seed -> kein FromSync ->
//     kein Beleg, keine Heilung, "abgewehrt" steigt.
func (n *vierKnotenNetz) trustedSeedsFromSync(t *testing.T) {
	t.Helper()
	vorher := n.pruefeGleichstand(t, "vor den Seeds")
	leiter := n.warteAufLeiter(t, -1, 30*time.Second)
	// Weder A noch B ist der Leiter: A produziert mit einem Seed, der nie
	// "aufgeholt" meldet, womoeglich nicht -- der Leiter muss es.
	a := n.ks[(leiter+1)%len(n.ks)]
	b := n.ks[(leiter+2)%len(n.ks)]
	_, dagA, _ := a.teile()
	_, dagB, _ := b.teile()

	fremd, _ := crypto.GenerateKey()
	fremdAddr := strings.ToLower(crypto.PubkeyToAddress(fremd.PublicKey).Hex())
	boeseAddr := strings.ToLower(crypto.PubkeyToAddress(n.boese.PublicKey).Hex())
	opfer := n.sender[1]
	diebstahl := signiere(t, testSchluessel{key: fremd, addr: fremdAddr}, fremdAddr, aeqWei(500), n.nonce[1], chainIDJetzt())
	diebstahl.Wallet = opfer.addr
	angriff := vierBlock(t, n.boese, boeseAddr, spitzenBloecke(t, dagA), []Transaction{diebstahl})
	for _, e := range angriff.ParentHashes {
		if !dagB.hasBlockInMemory(e) {
			t.Fatalf("Elternblock %s fehlt bei Knoten %d -- der Block liefe als Waise, nicht ins Nachspielen", e, b.i)
		}
	}
	angriffJSON, _ := json.Marshal([]*Block{angriff})

	// Der Peer: liefert den Block auf jede Seitenanfrage (die Folgeseite nach
	// ihm ist leer) und auf die Abfrage per Hash. Alles andere: 404.
	peer := vierServerAuf(t, "127.0.0.22", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/blocks":
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("after_hash") != "" {
				w.Write([]byte("[]"))
				return
			}
			w.Write(angriffJSON)
		case r.Method == http.MethodPost && r.URL.Path == "/api/blocks/by-hash":
			w.Header().Set("Content-Type", "application/json")
			w.Write(angriffJSON)
		default:
			http.NotFound(w, r)
		}
	}))

	// A traegt ihn als Seed ein -- ueber denselben Weg wie StartPeerDiscovery
	// (PEER_NODES -> staticPeers -> trustedSeeds).
	t.Setenv("PEER_NODES", peer.URL)
	dagA.trustedSeedsSetzen(strings.TrimRight(NormalizeNodeURL(a.srv.URL), "/"))
	t.Setenv("PEER_NODES", "")
	defer dagA.trustedSeedsSetzen(strings.TrimRight(NormalizeNodeURL(a.srv.URL), "/")) // wieder leer
	if !dagA.isTrustedSyncSource(peer.URL) {
		t.Fatalf("Knoten %d: PEER_NODES=%s macht ihn nicht zum Seed", a.i, peer.URL)
	}
	if dagB.isTrustedSyncSource(peer.URL) {
		t.Fatalf("Knoten %d haelt den Peer fuer einen Seed, ohne ihn eingetragen zu haben", b.i)
	}
	// Kein anderer Knoten des Netzes ist bei A dadurch Seed geworden.
	for _, o := range n.ks {
		if o != a && dagA.isTrustedSyncSource(o.srv.URL) {
			t.Fatalf("Knoten %d haelt Knoten %d fuer einen Seed", a.i, o.i)
		}
	}

	var mu sync.Mutex
	heilungen := map[*BlockDAG][]string{}
	vorherHeilen := replayMauerHeilen
	replayMauerHeilen = func(d *BlockDAG, grund string) {
		mu.Lock()
		heilungen[d] = append(heilungen[d], grund)
		mu.Unlock()
	}
	defer func() { replayMauerHeilen = vorherHeilen }()

	abgewehrtB := dagB.replayMauer.abgewehrt.Load()
	abweisungenA, abweisungenB := dagA.replayMauer.abweisungen.Load(), dagB.replayMauer.abweisungen.Load()
	// Ein beim Nachspielen gescheiterter Block wird erst nach
	// replayBackoffFor(n) wieder nachgespielt (5 s, 10 s, ...; block.go) --
	// davor zaehlt ein Abruf nicht. Der Peer liefert ihn also ueber die Zeit
	// immer wieder, wie ein Seed, dessen Kette ihn fuehrt, bis jeder Knoten
	// ihn replayMauerSchwelle-mal nachgespielt und abgewiesen hat.
	//
	// Waehrenddessen erzeugt niemand (ruhe): sonst rueckt die Finalitaet
	// weiter, und B (kein Seed, kein FromSync) liesse den Block noch vor dem
	// dritten Nachspielen schon im Sync als "unter der Finalitaet" fallen --
	// A als Seed nicht (FromSync entbindet dort bis zum Stichtag,
	// fromsync_stichtag.go). Das ist ein anderer Unterschied als der, um den
	// es hier geht.
	n.ruhe.Store(true)
	defer n.ruhe.Store(false)
	ende := time.Now().Add(90 * time.Second)
	for time.Now().Before(ende) &&
		(dagA.replayMauer.abweisungen.Load()-abweisungenA < replayMauerSchwelle ||
			dagB.replayMauer.abweisungen.Load()-abweisungenB < replayMauerSchwelle) {
		dagB.doSyncOnce(peer.URL)
		dagA.doSyncOnce(peer.URL)
		time.Sleep(time.Second)
	}
	time.Sleep(500 * time.Millisecond) // loeseHeilungAus laeuft in einer Goroutine

	// Beide: abgewiesen, nichts verbucht.
	for _, k := range []*vierKnoten{a, b} {
		cs, d, _ := k.teile()
		if d.hasBlockInMemory(angriff.Hash) {
			t.Fatalf("Knoten %d fuehrt den Angriffsblock", k.i)
		}
		if acc, hat := vierKonto(cs, fremdAddr); hat && acc.Balance.Float() != 0 {
			t.Fatalf("Knoten %d: der Dieb hat %s AEQ", k.i, acc.Balance.String())
		}
	}
	if neu := dagA.replayMauer.abweisungen.Load() - abweisungenA; neu < replayMauerSchwelle {
		t.Fatalf("Knoten %d (Seed eingetragen) wies den Block nur %d-mal beim Nachspielen ab, erwartet %d -- der Test zeigt nichts",
			a.i, neu, replayMauerSchwelle)
	}
	if neu := dagB.replayMauer.abweisungen.Load() - abweisungenB; neu < replayMauerSchwelle {
		t.Fatalf("Knoten %d (kein Seed) wies den Block nur %d-mal beim Nachspielen ab, erwartet %d -- der Test zeigt nichts",
			b.i, neu, replayMauerSchwelle)
	}

	mu.Lock()
	beiA, beiB := heilungen[dagA], heilungen[dagB]
	mu.Unlock()
	// B: kein Seed, kein FromSync, kein Beleg.
	if len(beiB) != 0 {
		t.Fatalf("Knoten %d heilt nach Bloecken eines Peers, der kein Seed ist: %v", b.i, beiB)
	}
	if dagB.replayMauer.abgewehrt.Load() <= abgewehrtB {
		t.Fatalf("Knoten %d: Schwelle ohne Beleg nicht als abgewehrt gezaehlt", b.i)
	}
	// A: FromSync vom Seed ist der Beleg.
	if len(beiA) == 0 {
		t.Fatalf("Knoten %d hat den Block seines Seeds nicht als FromSync behandelt (keine Heilung mit Seed-Beleg)", a.i)
	}
	if !strings.Contains(beiA[0], "vom Seed per Sync geliefert") {
		t.Fatalf("Knoten %d heilt, aber nicht wegen FromSync: %s", a.i, beiA[0])
	}
	t.Logf("Seed-Block bei Knoten %d: FromSync, abgewiesen, Heilung (%s); derselbe Block bei Knoten %d: kein FromSync, abgewiesen, abgewehrt",
		a.i, beiA[0], b.i)

	nachher := n.pruefeGleichstand(t, "nach den Seeds")
	if nachher.nonces[opfer.addr] != vorher.nonces[opfer.addr] {
		t.Fatalf("Nonce des Opfers %d -> %d", vorher.nonces[opfer.addr], nachher.nonces[opfer.addr])
	}
}
