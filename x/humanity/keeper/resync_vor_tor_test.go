package keeper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// PerformResync: Abruf vor dem Tor, Austausch dahinter (autoheal.go,
// resync_tor.go). Befund aus dem Vier-Knoten-Test: das Tor war fuer den
// ganzen Abruf zu, samt 429-Wartezeiten -- der Knoten nahm und erzeugte
// minutenlang keinen Block, auch wenn der Abruf am Ende scheiterte.

const resyncTestSigner = "0x0000000000000000000000000000000000000001"

// resyncTestServer: ein Peer, dessen /api/snapshot erst antwortet, wenn der
// Test es erlaubt (frei), und dann einen von einem fremden Schluessel
// unterschriebenen -- also gefaelschten -- Snapshot liefert. drosseln
// Antworten vorher sind 429.
func resyncTestServer(t *testing.T, drosseln int32, frei <-chan struct{}) (*httptest.Server, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	dieb, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	var anfragen, gleichzeitig, maxGleichzeitig atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := gleichzeitig.Add(1)
		defer gleichzeitig.Add(-1)
		for {
			m := maxGleichzeitig.Load()
			if n <= m || maxGleichzeitig.CompareAndSwap(m, n) {
				break
			}
		}
		if anfragen.Add(1) <= drosseln {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if frei != nil {
			<-frei
		}
		s := &StateSnapshot{Timestamp: time.Now().Unix(), Height: 10}
		snapshotSignieren(t, s, dieb)
		_ = json.NewEncoder(w).Encode(s)
	}))
	t.Cleanup(srv.Close)
	return srv, &anfragen, &maxGleichzeitig
}

// resyncFreigabe: der Kanal, auf den resyncTestServer wartet, und ein
// mehrfach aufrufbares Schliessen.
func resyncFreigabe() (<-chan struct{}, func()) {
	frei := make(chan struct{})
	var einmal sync.Once
	return frei, func() { einmal.Do(func() { close(frei) }) }
}

func resyncLokalErlauben(t *testing.T) {
	t.Helper()
	alt := snapshotDialLokal.Load()
	snapshotDialLokal.Store(true)
	t.Cleanup(func() { snapshotDialLokal.Store(alt) })
}

// Missbrauch: ein Peer, der /api/snapshot lange drosselt und dann langsam
// antwortet. Waehrend der ganzen Zeit muss der Knoten Bloecke annehmen und
// erzeugen koennen -- und der gefaelschte Snapshot, der am Ende kommt, wird
// trotzdem abgewiesen, ohne dass das Tor je zu war.
func TestPerformResync_DrosselnderPeerMachtKnotenNichtTaub(t *testing.T) {
	resyncLokalErlauben(t)
	altWarten := snapshotRetryWarten
	snapshotRetryWarten = 150 * time.Millisecond
	defer func() { snapshotRetryWarten = altWarten }()

	frei, freigeben := resyncFreigabe()
	srv, anfragen, _ := resyncTestServer(t, 3, frei) // drei 429, dann langsam
	t.Cleanup(freigeben)                             // vor srv.Close: ein haengender Abruf haelt sonst den Server

	dag := newGhostdagTestDAG()
	var torJeZu atomic.Bool
	stop := make(chan struct{})
	beobachter := make(chan struct{})
	go func() {
		defer close(beobachter)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if dag.resyncInProgress.Load() {
				torJeZu.Store(true)
			}
			time.Sleep(time.Millisecond)
		}
	}()

	fertig := make(chan error, 1)
	go func() { fertig <- dag.PerformResync(srv.URL+"/api/snapshot", resyncTestSigner, "") }()

	// Warten, bis der Peer alle drei 429 verteilt hat und die vierte Anfrage
	// haengt -- das sind mehrere Wartezeiten.
	frist := time.Now().Add(10 * time.Second)
	for anfragen.Load() < 4 {
		if time.Now().After(frist) {
			t.Fatalf("nur %d Anfragen beim Peer", anfragen.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Jetzt, mitten im Abruf: Annahme und Erzeugung sind offen.
	for i := 0; i < 20; i++ {
		offen, verlassen := dag.resyncTorBetreten()
		if !offen {
			t.Fatal("waehrend des Snapshot-Abrufs ist das Tor zu -- der Knoten nimmt und erzeugt keine Bloecke")
		}
		verlassen()
		time.Sleep(5 * time.Millisecond)
	}
	freigeben()

	select {
	case err := <-fertig:
		if err == nil {
			t.Fatal("ein von einem fremden Schluessel unterschriebener Snapshot wurde uebernommen")
		}
		if !strings.Contains(err.Error(), "expected "+resyncTestSigner) {
			t.Fatalf("abgewiesen, aber aus anderem Grund: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("PerformResync kehrt nicht zurueck")
	}
	close(stop)
	<-beobachter
	if torJeZu.Load() {
		t.Fatal("das Tor war zu, obwohl der Snapshot nie gueltig war")
	}
	if dag.resyncInProgress.Load() || dag.resyncTorDrin.Load() != 0 {
		t.Fatalf("Tor nach dem Abbruch: zu=%v, drin=%d", dag.resyncInProgress.Load(), dag.resyncTorDrin.Load())
	}
}

// Missbrauch: zwei PerformResync gleichzeitig (zwei Waechter feuern, oder
// leitungUeberholt neben der Selbstheilung). Es laeuft hoechstens einer --
// der zweite holt nicht einmal, solange der erste noch holt; zwei
// Zustandswechsel nebeneinander sind damit ausgeschlossen.
func TestPerformResync_HoechstensEinerZugleich(t *testing.T) {
	resyncLokalErlauben(t)
	frei, freigeben := resyncFreigabe()
	srv, anfragen, maxGleichzeitig := resyncTestServer(t, 0, frei)
	t.Cleanup(freigeben)

	dag := newGhostdagTestDAG()
	const n = 4
	var wg sync.WaitGroup
	fehler := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fehler <- dag.PerformResync(srv.URL+"/api/snapshot", resyncTestSigner, "")
		}()
	}
	// Der erste haengt beim Peer; die anderen duerfen ihn nicht erreichen.
	frist := time.Now().Add(5 * time.Second)
	for anfragen.Load() < 1 {
		if time.Now().After(frist) {
			t.Fatal("keine Anfrage beim Peer")
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	if a := anfragen.Load(); a != 1 {
		t.Fatalf("%d Resyncs holen gleichzeitig", a)
	}
	freigeben()
	wg.Wait()
	close(fehler)
	for err := range fehler {
		if err == nil {
			t.Fatal("gefaelschter Snapshot uebernommen")
		}
	}
	if m := maxGleichzeitig.Load(); m != 1 {
		t.Fatalf("hoechstens %d Abrufe gleichzeitig, erwartet 1", m)
	}
	if anfragen.Load() != n {
		t.Fatalf("%d Abrufe, erwartet %d nacheinander", anfragen.Load(), n)
	}
}

// Das Tor wartet auf einen Durchgang, der die Pruefung schon hinter sich hat,
// und bricht nach der Frist ab, statt parallel zu ihm zu tauschen.
func TestResyncTor_WartetAufLaufendenDurchgang(t *testing.T) {
	dag := newGhostdagTestDAG()
	offen, verlassen := dag.resyncTorBetreten()
	if !offen {
		t.Fatal("Tor ohne Resync zu")
	}
	oeffnen := dag.resyncTorSchliessen()
	if o, _ := dag.resyncTorBetreten(); o {
		t.Fatal("nach dem Schliessen kommt ein neuer Durchgang hinein")
	}
	// Frist abgelaufen, der Durchgang laeuft noch: Abbruch.
	if err := dag.warteBisResyncTorLeer(50 * time.Millisecond); err == nil {
		t.Fatal("Tor gilt als leer, obwohl ein Durchgang laeuft")
	}
	// Der Durchgang endet waehrend des Wartens: dann leer.
	go func() {
		time.Sleep(50 * time.Millisecond)
		verlassen()
	}()
	if err := dag.warteBisResyncTorLeer(5 * time.Second); err != nil {
		t.Fatalf("Tor nach dem Ende des Durchgangs: %v", err)
	}
	oeffnen()
	if dag.resyncInProgress.Load() {
		t.Fatal("Tor nach dem letzten Halter noch zu")
	}
}

// PerformResync mit einem GUELTIG unterschriebenen Snapshot, waehrend ein
// Blockdurchgang die Torpruefung schon hinter sich hat: kein Austausch
// parallel zu ihm. Nach der Frist bricht der Resync ab -- und der Zustand
// ist unberuehrt.
func TestPerformResync_TauschtNichtNebenLaufendemDurchgang(t *testing.T) {
	resyncLokalErlauben(t)
	altMax := resyncTorLeerMax
	resyncTorLeerMax = 200 * time.Millisecond
	defer func() { resyncTorLeerMax = altMax }()

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	const neu = "0x00000000000000000000000000000000000000aa"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := &StateSnapshot{Timestamp: time.Now().Unix(), Height: 10,
			Accounts: []*AccountState{{Address: neu, Balance: NewDecimal(5)}}}
		snapshotSignieren(t, s, key)
		_ = json.NewEncoder(w).Encode(s)
	}))
	defer srv.Close()

	dag := newGhostdagTestDAG()
	dag.state = &ChainState{}
	offen, verlassen := dag.resyncTorBetreten()
	if !offen {
		t.Fatal("Tor ohne Resync zu")
	}
	err = dag.PerformResync(srv.URL+"/api/snapshot", signer, "")
	verlassen()
	if err == nil || !strings.Contains(err.Error(), "pass(es) running") {
		t.Fatalf("Resync neben einem laufenden Durchgang: %v, erwartet Abbruch", err)
	}
	if dag.state.accounts != nil {
		if _, da := dag.state.accounts.Get(neu); da {
			t.Fatal("der Snapshot wurde neben dem laufenden Durchgang angewendet")
		}
	}
	if dag.resyncInProgress.Load() {
		t.Fatal("Tor nach dem Abbruch noch zu")
	}
}

// Der knappste Wettlauf: ein Resync schliesst das Tor genau zwischen den
// beiden Schritten eines Durchgangs. Dann muss mindestens eine Seite die
// andere sehen -- entweder der Durchgang das geschlossene Tor, oder der
// Resync den laufenden Durchgang. Beides verpasst hiesse: Austausch und
// Blockdurchgang zugleich.
func TestResyncTor_SchliessenMittenImBetreten(t *testing.T) {
	dag := newGhostdagTestDAG()
	var oeffnen func()
	var resyncSahDurchgang bool
	resyncTorZwischenHaken = func() {
		oeffnen = dag.resyncTorSchliessen()
		resyncSahDurchgang = dag.warteBisResyncTorLeer(20*time.Millisecond) != nil
	}
	offen, verlassen := dag.resyncTorBetreten()
	resyncTorZwischenHaken = nil
	if offen {
		verlassen()
	}
	if oeffnen != nil {
		oeffnen()
	}
	if offen && !resyncSahDurchgang {
		t.Fatal("Durchgang und Resync haben einander verpasst -- beide liefen zugleich")
	}
	if !resyncSahDurchgang {
		t.Fatal("der Resync hat den bereits gezaehlten Durchgang nicht gesehen")
	}
	if offen {
		t.Fatal("der Durchgang lief, obwohl das Tor vor seiner Pruefung zu war")
	}
}

// Zwei Halter (leitungUeberholt und sein PerformResync): das Tor oeffnet erst,
// wenn beide gehen; ein doppelter Aufruf derselben Oeffnung zaehlt einmal.
func TestResyncTor_ZweiHalter(t *testing.T) {
	dag := newGhostdagTestDAG()
	a := dag.resyncTorSchliessen()
	b := dag.resyncTorSchliessen()
	b()
	b()
	if !dag.resyncInProgress.Load() {
		t.Fatal("der innere Halter hat das Tor des aeusseren geoeffnet")
	}
	a()
	if dag.resyncInProgress.Load() {
		t.Fatal("Tor nach beiden Haltern noch zu")
	}
}

// Was sich zwischen Abruf und Austausch aendern kann, prueft der Austausch
// selbst noch einmal: ein inzwischen zu alter Snapshot wird nicht angewendet,
// und ohne gepruefte Herkunft gar keiner.
func TestWendeResyncSnapshotAn_PrueftErneut(t *testing.T) {
	cs := &ChainState{}
	if err := cs.wendeResyncSnapshotAn(nil); err == nil {
		t.Fatal("Austausch ohne geprueften Snapshot")
	}
	if err := cs.wendeResyncSnapshotAn(&resyncSnapshot{snap: &StateSnapshot{Timestamp: time.Now().Unix()}}); err == nil {
		t.Fatal("Austausch ohne Unterzeichner")
	}
	t.Setenv("SNAPSHOT_MAX_AGE_SECONDS", "60")
	alt := &resyncSnapshot{snap: &StateSnapshot{Timestamp: time.Now().Unix() - 120, Height: 5}, signer: resyncTestSigner}
	if err := cs.wendeResyncSnapshotAn(alt); err == nil || !strings.Contains(err.Error(), "too old") {
		t.Fatalf("waehrend des Abrufs veralteter Snapshot: %v", err)
	}
}
