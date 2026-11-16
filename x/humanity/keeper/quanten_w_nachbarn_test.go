package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Validator-Dienst: Nachbarn des Knotens statt nur QUANTEN_W_PEERS.
// Nachbarn sind Fremde -- nur oeffentliche Adressen, keine Weiterleitung,
// alle gleichzeitig unter einer Frist, und nur vom Validator unterschriebene
// Anteile zaehlen.

// qwAnfrageOffen: praegen, Anfrage, Zufall, Antwort -- danach steht die
// Offenlegung an (k = 16: ein Teil).
func qwAnfrageOffen(t *testing.T, q *qwDienste) {
	t.Helper()
	w := q.w
	q.praegen(t)
	q.zeit += 10
	w.annehmen(w.anfrageTx(w.anna, 0, 16, "", q.zeit), q.zeit+1)
	q.zeit += 2
	q.takt()
	a := w.anfrage(w.cs)
	if a == nil || a.Basen == nil {
		t.Fatal("Zufall nicht vollstaendig")
	}
	w.annehmen(w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen)), q.zeit+1)
	q.zeit += 2
}

// anteilServer: ein Validator, der nach verzoegerung seine unterschriebenen
// Anteile ausgibt.
func anteilServer(t *testing.T, d *quantenWDienst, verzoegerung time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		time.Sleep(verzoegerung)
		index, _ := strconv.Atoi(r.URL.Query().Get("index"))
		ab, _ := strconv.Atoi(r.URL.Query().Get("ab"))
		out, err := d.anteile(r.URL.Query().Get("k"), index, ab)
		if err == nil {
			err = d.anteilUnterschreiben(&out)
		}
		if err != nil {
			http.Error(rw, err.Error(), http.StatusConflict)
			return
		}
		json.NewEncoder(rw).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// haengenderServer: antwortet nie (bis der Abrufer aufgibt).
func haengenderServer(t *testing.T) *httptest.Server {
	t.Helper()
	stop := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-stop:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(stop) }) // laeuft vor srv.Close
	return srv
}

// zaehlServer: zaehlt Verbindungen und Anfragen.
func zaehlServer(t *testing.T) (*httptest.Server, *atomic.Int64, *atomic.Int64) {
	t.Helper()
	var verbindungen, anfragen atomic.Int64
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		anfragen.Add(1)
		http.Error(rw, "nein", http.StatusConflict)
	}))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			verbindungen.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, &verbindungen, &anfragen
}

// Die Unterschrift bindet alles: Note, Nummer, Teil, Validator, Stellen,
// Anteile, Pfade -- und nur Validator X der Note kann sie geben.
func TestQuantenWDienst_AnteilUnterschrift(t *testing.T) {
	q := neueQwDienste(t, 200)
	e := &quantenWTresorEintrag{Validatoren: q.w.val, T: 3}
	echt := func() QuantenWAnteilAusgabe {
		a := QuantenWAnteilAusgabe{K: q.w.p.K, Index: 1, Ab: 64, X: 2, Stellen: []int{3, 5}, Y: []string{"a1", "b2"},
			Pfade: [][]string{{"00", "01"}, {"10", "11"}}}
		if err := q.d[1].anteilUnterschreiben(&a); err != nil {
			t.Fatal(err)
		}
		return a
	}
	if !quantenWAnteilEcht(e, echt()) {
		t.Fatal("Gegenprobe: echte Ausgabe abgewiesen")
	}
	fremd, _ := neuerSchluessel(t)
	fremdDienst, err := neuerQuantenWDienst(q.w.cs, fremd, &quantenWSpeicherAblage{}, nil, q.einreichen)
	if err != nil {
		t.Fatal(err)
	}
	for name, f := range map[string]func(a *QuantenWAnteilAusgabe){
		"andere Note":          func(a *QuantenWAnteilAusgabe) { a.K = strings.Repeat("e", 64) },
		"andere Nummer":        func(a *QuantenWAnteilAusgabe) { a.Index = 2 },
		"anderer Teil":         func(a *QuantenWAnteilAusgabe) { a.Ab = 0 },
		"anderer Validator":    func(a *QuantenWAnteilAusgabe) { a.X = 3 },
		"andere Stelle":        func(a *QuantenWAnteilAusgabe) { a.Stellen[1] = 6 },
		"anderer Anteil":       func(a *QuantenWAnteilAusgabe) { a.Y[0] = "a2" },
		"Anteile verschoben":   func(a *QuantenWAnteilAusgabe) { a.Y = []string{"a1 b2"} },
		"anderer Pfad":         func(a *QuantenWAnteilAusgabe) { a.Pfade[1][0] = "12" },
		"ohne Unterschrift":    func(a *QuantenWAnteilAusgabe) { a.Sig = "" },
		"kaputte Unterschrift": func(a *QuantenWAnteilAusgabe) { a.Sig = "0x1234" },
		"X = 0":                func(a *QuantenWAnteilAusgabe) { a.X = 0 },
		"X zu gross":           func(a *QuantenWAnteilAusgabe) { a.X = len(q.w.val) + 1 },
		"X negativ":            func(a *QuantenWAnteilAusgabe) { a.X = -1 },
		// Validator 3 unterschreibt im Namen von 2.
		"fremder Validator": func(a *QuantenWAnteilAusgabe) {
			a.Sig = ""
			if err := q.d[2].anteilUnterschreiben(a); err != nil {
				t.Fatal(err)
			}
		},
		// Ein Nachbar, der kein Validator der Note ist.
		"Fremder": func(a *QuantenWAnteilAusgabe) {
			a.Sig = ""
			if err := fremdDienst.anteilUnterschreiben(a); err != nil {
				t.Fatal(err)
			}
		},
	} {
		a := echt()
		f(&a)
		if quantenWAnteilEcht(e, a) {
			t.Errorf("%s: als echt angenommen", name)
		}
	}
}

// Die API gibt die Anteile unterschrieben heraus.
func TestQuantenWDienst_AnteileAPIUnterschrieben(t *testing.T) {
	q := neueQwDienste(t, 200)
	qwAnfrageOffen(t, q)
	d := q.d[1]
	q.w.cs.quantenWDienstZeiger.Store(d)
	quantenWAbOverride.Store(nowUnix() - 86400) // die API prueft gegen die Uhr
	a := &APIServer{state: q.w.cs}
	rr := httptest.NewRecorder()
	a.handleQuantenWAnteile(rr, httptest.NewRequest(http.MethodGet, "/api/quanten-w/validator/anteile?k="+q.w.p.K+"&index=0&ab=0", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("Status %d: %s", rr.Code, rr.Body.String())
	}
	var out QuantenWAnteilAusgabe
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	e := d.eintrag(q.w.p.K)
	if out.X != 2 || len(out.Stellen) != 16 || !quantenWAnteilEcht(e, out) {
		t.Fatalf("Ausgabe nicht unterschrieben: X %d, %d Stellen, Sig %q", out.X, len(out.Stellen), out.Sig)
	}
}

// Gutfall: ohne QUANTEN_W_PEERS, nur mit den Nachbarn des Knotens -- die
// Offenlegung in drei Teilen laeuft durch.
func TestQuantenWDienst_NachbarnDesKnotens(t *testing.T) {
	q := neueQwDienste(t, 400)
	w := q.w
	for x, d := range q.d {
		var nachbarn []string
		for y, s := range q.srv {
			if y != x {
				nachbarn = append(nachbarn, s.URL)
			}
		}
		d.peers = nil
		d.nachbarn = func() []string { return nachbarn }
		// httptest liegt auf Loopback: hier zugelassen, ueber den
		// gewoehnlichen Dialer (die Filter prueft NachbarnMissbrauch).
		d.erlaubt = func(string) bool { return true }
		d.fremd = quantenWPeerKlient(nil)
	}
	q.praegen(t)
	q.zeit += 10
	w.annehmen(w.anfrageTx(w.anna, 0, 150, "", q.zeit), q.zeit+1)
	q.zeit += 2
	q.takt()
	a := w.anfrage(w.cs)
	w.annehmen(w.antwortTx(w.anna, 0, w.p.ehrlich(a.Stellen, a.Basen)), q.zeit+1)
	q.zeit += 2
	q.takt()
	if w.anfrage(w.cs) != nil || w.stand(w.cs, w.anna) != 100_000_000 {
		t.Fatalf("ueber die Nachbarn nicht offengelegt: %+v", w.anfrage(w.cs))
	}
}

// Missbrauch der Nachbarliste: nur oeffentliche Adressen, ohne Doppelte,
// begrenzt; die Adressen des Betreibers zuerst und so, wie er sie setzt.
func TestQuantenWDienst_NachbarnFilter(t *testing.T) {
	q := neueQwDienste(t, 200)
	d := q.d[0]
	d.peers = []string{"http://127.0.0.1:9/", "http://100.64.0.7:8080"}
	nachbarn := []string{
		"http://127.0.0.1:1",
		"http://10.0.0.1:8080",
		"http://192.168.1.1",
		"http://169.254.169.254",
		"http://100.64.0.8:8080", // CGNAT ohne Freiliste
		"https://[::1]:8443",
		"https://[fe80::1%25eth0]:8443",
		"http://[64:ff9b::7f00:1]:80", // NAT64 auf 127.0.0.1
		"http://0.0.0.0:80",
		"ftp://1.2.3.4",
		"file:///etc/passwd",
		"http://example.com", // http mit Namen: DNS-Rebinding
		"",
		"   ",
		"http://127.0.0.1:9",      // schon vom Betreiber
		"https://1.2.3.4:8443/",   // erlaubt
		"https://1.2.3.4:8443",    // doppelt
		"https://knoten.example/", // erlaubt (Name mit https)
	}
	d.nachbarn = func() []string { return nachbarn }
	got := d.peerListe()
	want := []quantenWPeer{
		{url: "http://127.0.0.1:9"}, {url: "http://100.64.0.7:8080"},
		{url: "https://1.2.3.4:8443", fremd: true}, {url: "https://knoten.example", fremd: true},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("Liste\n %v\nstatt\n %v", got, want)
	}
	// Grenze: viele Nachbarn, hoechstens quantenWPeersHoechstens.
	var viele []string
	for i := 0; i < 3*quantenWPeersHoechstens; i++ {
		viele = append(viele, fmt.Sprintf("https://1.2.3.%d:8443", i))
	}
	d.nachbarn = func() []string { return viele }
	if got := d.peerListe(); len(got) != quantenWPeersHoechstens || got[0].fremd || !got[2].fremd {
		t.Fatalf("%d Peers (Grenze %d): %v", len(got), quantenWPeersHoechstens, got[:3])
	}
	// Auch die Adressen des Betreibers: hoechstens quantenWPeersHoechstens.
	d.peers, d.nachbarn = viele, nil
	if got := d.peerListe(); len(got) != quantenWPeersHoechstens || got[0].fremd {
		t.Fatalf("%d Peers des Betreibers (Grenze %d)", len(got), quantenWPeersHoechstens)
	}
	// Ohne Nachbarn und Betreiber: nichts (und kein Abruf).
	d.peers, d.nachbarn = nil, nil
	if got := d.peerListe(); len(got) != 0 {
		t.Fatalf("Peers ohne Quelle: %v", got)
	}
}

// Ein Name, der auf Loopback zeigt, kommt durch den Filter (https mit Name),
// aber nie bis zur Verbindung: der Klient fuer Nachbarn loest einmal auf und
// verbindet nur zu oeffentlichen Adressen.
func TestQuantenWDienst_NachbarNameAufLoopback(t *testing.T) {
	q := neueQwDienste(t, 200)
	d := q.d[0]
	srv, verbindungen, _ := zaehlServer(t)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	ziel := "https://localhost:" + port
	if !d.erlaubt(ziel) {
		t.Fatal("Voraussetzung: der Filter laesst https mit Namen durch")
	}
	d.peers = nil
	d.nachbarn = func() []string { return []string{ziel} }
	e := &quantenWTresorEintrag{Validatoren: q.w.val, T: 3}
	eigen := QuantenWAnteilAusgabe{K: q.w.p.K, X: 1}
	if out := d.nachbarAnteile(context.Background(), q.w.p.K, e, eigen); len(out) != 0 {
		t.Fatalf("Anteile von Loopback: %v", out)
	}
	if n := verbindungen.Load(); n != 0 {
		t.Fatalf("%d Verbindungen zu Loopback ueber einen Namen", n)
	}
	// Gegenprobe: der gewoehnliche Dialer haette verbunden.
	d.fremd = quantenWPeerKlient(nil)
	d.nachbarAnteile(context.Background(), q.w.p.K, e, eigen)
	if verbindungen.Load() == 0 {
		t.Fatal("Gegenprobe: ohne pinningDialer keine Verbindung -- der Test prueft nichts")
	}
}

// Keine Weiterleitung, weder bei Nachbarn noch bei den Adressen des
// Betreibers: eine erlaubte Adresse duerfte sonst ins interne Netz zeigen.
func TestQuantenWDienst_NachbarLeitetUm(t *testing.T) {
	q := neueQwDienste(t, 200)
	d := q.d[0]
	innen, _, anfragen := zaehlServer(t)
	um := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		http.Redirect(rw, r, innen.URL+r.URL.RequestURI(), http.StatusFound)
	}))
	t.Cleanup(um.Close)
	e := &quantenWTresorEintrag{Validatoren: q.w.val, T: 3}
	eigen := QuantenWAnteilAusgabe{K: q.w.p.K, X: 1}
	d.erlaubt = func(string) bool { return true }
	d.fremd = quantenWPeerKlient(nil) // wie im Betrieb, nur ohne Loopback-Sperre
	for name, setze := range map[string]func(){
		"Nachbar":   func() { d.peers, d.nachbarn = nil, func() []string { return []string{um.URL} } },
		"Betreiber": func() { d.peers, d.nachbarn = []string{um.URL}, nil },
	} {
		setze()
		d.nachbarAnteile(context.Background(), q.w.p.K, e, eigen)
		if n := anfragen.Load(); n != 0 {
			t.Fatalf("%s: Weiterleitung gefolgt (%d Anfragen innen)", name, n)
		}
	}
}

// Ein Fremder, der schneller antwortet, mit erfundenen Anteilen fuer jeden
// anderen Validator (unterschrieben mit seinem eigenen Schluessel oder gar
// nicht) oder mit einer alten, echt unterschriebenen Ausgabe fuer andere
// Stellen (S: neue Stichprobe nach einer Frist, gleiche Nummer): ohne die
// Pruefungen belegte er jeden Platz, und keine Auswahl passte je. Mit ihnen
// zaehlen nur die echten.
func TestQuantenWDienst_NachbarFaelschtAnteile(t *testing.T) {
	q := neueQwDienste(t, 200)
	w := q.w
	qwAnfrageOffen(t, q)
	fremd, _ := neuerSchluessel(t)
	fremdDienst, err := neuerQuantenWDienst(w.cs, fremd, &quantenWSpeicherAblage{}, nil, q.einreichen)
	if err != nil {
		t.Fatal(err)
	}
	a := w.anfrage(w.cs)
	var faelscher []string
	for x := 1; x <= len(w.val); x++ {
		for _, art := range []string{"fremd", "ohne", "alt"} {
			xx, art := x, art
			srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				out := QuantenWAnteilAusgabe{K: w.p.K, Index: 0, Ab: 0, X: xx, Stellen: append([]int(nil), a.Stellen[:16]...)}
				for range out.Stellen {
					out.Y = append(out.Y, "1")
					out.Pfade = append(out.Pfade, []string{strings.Repeat("0", 64)})
				}
				switch art {
				case "fremd":
					fremdDienst.anteilUnterschreiben(&out)
				case "alt":
					out.Stellen[0] = (out.Stellen[0] + 1) % w.p.n
					q.d[xx-1].anteilUnterschreiben(&out)
				}
				json.NewEncoder(rw).Encode(out)
			}))
			t.Cleanup(srv.Close)
			faelscher = append(faelscher, srv.URL)
		}
	}
	for x, d := range q.d {
		peers := append([]string(nil), faelscher...)
		for y, dy := range q.d {
			if y != x {
				peers = append(peers, anteilServer(t, dy, 150*time.Millisecond).URL)
			}
		}
		d.peers = peers
	}
	q.takt()
	if w.anfrage(w.cs) != nil || w.stand(w.cs, w.anna) != 100_000_000 {
		t.Fatalf("gefaelschte Anteile haben die Offenlegung verhindert: %+v", w.anfrage(w.cs))
	}
}

// Nachbarn, die nicht antworten, halten den Takt nicht auf: alle
// gleichzeitig unter einer Frist; sind t Validatoren beisammen, wird nur
// noch kurz nachgewartet.
func TestQuantenWDienst_LangsameNachbarn(t *testing.T) {
	q := neueQwDienste(t, 200)
	qwAnfrageOffen(t, q)
	d := q.d[0]
	e := d.eintrag(q.w.p.K)
	eigen, err := d.anteile(q.w.p.K, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var haengend []string
	for i := 0; i < 6; i++ {
		haengend = append(haengend, haengenderServer(t).URL)
	}
	// Nur haengende: nach peerZeit zurueck (nacheinander waeren es 6 x).
	d.peerZeit, d.peers = 300*time.Millisecond, haengend
	start := time.Now()
	if out := d.nachbarAnteile(context.Background(), q.w.p.K, e, eigen); len(out) != 0 {
		t.Fatalf("Anteile von haengenden Nachbarn: %v", out)
	}
	if dauer := time.Since(start); dauer > 3*d.peerZeit {
		t.Fatalf("haengende Nachbarn: %v (Frist %v)", dauer, d.peerZeit)
	}
	// Zwei Validatoren antworten (mit dem eigenen t = 3), der Rest haengt:
	// zurueck nach nachlauf, nicht erst nach der Frist.
	d.peerZeit, d.nachlauf = 20*time.Second, 200*time.Millisecond
	d.peers = append([]string{q.srv[1].URL, q.srv[2].URL}, haengend...)
	start = time.Now()
	out := d.nachbarAnteile(context.Background(), q.w.p.K, e, eigen)
	if len(out) != 2 {
		t.Fatalf("%d Ausgaben statt 2", len(out))
	}
	if dauer := time.Since(start); dauer > 5*time.Second {
		t.Fatalf("auf haengende Nachbarn gewartet, obwohl t beisammen war: %v", dauer)
	}
	// Derselbe Validator unter zwei Adressen zaehlt einmal (sonst fuellten
	// Doppelte die Auswahlen, die quantenWSammeln ohnehin verwirft).
	d.peerZeit, d.nachlauf = 5*time.Second, time.Second
	d.peers = []string{q.srv[1].URL, anteilServer(t, q.d[1], 0).URL, q.srv[2].URL, anteilServer(t, q.d[2], 0).URL}
	if out := d.nachbarAnteile(context.Background(), q.w.p.K, e, eigen); len(out) != 2 || out[0].X == out[1].X {
		t.Fatalf("Doppelte gezaehlt: %d Ausgaben", len(out))
	}
	// Alle Validatoren da: sofort zurueck, ohne auf haengende zu warten.
	d.nachlauf = 20 * time.Second
	d.peers = append(append([]string(nil), haengend[:3]...), q.srv[1].URL, q.srv[2].URL, q.srv[3].URL, q.srv[4].URL)
	start = time.Now()
	if out := d.nachbarAnteile(context.Background(), q.w.p.K, e, eigen); len(out) != 4 {
		t.Fatalf("%d Ausgaben statt 4", len(out))
	}
	if dauer := time.Since(start); dauer > 5*time.Second {
		t.Fatalf("alle da und trotzdem gewartet: %v", dauer)
	}
}

// StarteQuantenWDienst (wie in main.go) gibt die Nachbarn des Knotens an
// den Dienst weiter -- gefiltert, die Adressen des Betreibers vorn.
func TestQuantenWDienst_StartMitNachbarn(t *testing.T) {
	cs := newTestState()
	key, _ := neuerSchluessel(t)
	t.Setenv("QUANTEN_W_PEERS", "http://10.1.2.3:8080")
	ctx, halt := context.WithCancel(context.Background())
	t.Cleanup(halt)
	nachbarn := func() []string { return []string{"http://127.0.0.1:9", "https://5.6.7.8:8443"} }
	if err := cs.StarteQuantenWDienst(ctx, key, "http://127.0.0.1:1", nil, nachbarn); err != nil {
		t.Fatal(err)
	}
	d := cs.quantenWDienstJetzt()
	if d == nil {
		t.Fatal("kein Dienst")
	}
	want := []quantenWPeer{{url: "http://10.1.2.3:8080"}, {url: "https://5.6.7.8:8443", fremd: true}}
	if got := d.peerListe(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("Peers %v statt %v", got, want)
	}
}
