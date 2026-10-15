package keeper

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Missbrauch und Grenzen (Beta-Pruefung 14.10.2026): gemerkte Peers sind
// begrenzt, verfallen, gehen durch den SSRF-Filter und werden nie Seeds.
func TestPeerVorrat_GrenzenUndFilter_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := testKnoten(t, "unused-peer-vorrat.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	cs.peerVorratTabelle()
	cs.db.Exec(`DELETE FROM bekannte_peers`)
	t.Cleanup(func() { cs.db.Exec(`DELETE FROM bekannte_peers`) })
	peerVorratGeschriebenMu.Lock()
	peerVorratGeschrieben = map[string]time.Time{}
	peerVorratGeschriebenMu.Unlock()

	jetzt := time.Unix(2_000_000_000, 0)
	// Flut: 1000 verschiedene oeffentliche URLs -- es bleiben 64.
	for i := 0; i < 1000; i++ {
		cs.peerMerken(fmt.Sprintf("http://8.%d.%d.%d:8080", i/65536%256, i/256%256, i%256), jetzt.Add(time.Duration(i)*time.Second))
	}
	var n int
	cs.db.QueryRow(`SELECT COUNT(*) FROM bekannte_peers`).Scan(&n)
	if n != peerVorratGrenze {
		t.Fatalf("%d Eintraege, Grenze %d", n, peerVorratGrenze)
	}
	// Private, Loopback, ohne Schema, zu lang: nie gemerkt.
	for _, u := range []string{"http://127.0.0.1:8080", "http://10.0.0.5:8080", "http://192.168.1.2", "http://localhost:8080",
		"file:///etc/passwd", "https://" + strings.Repeat("a", 300) + ".de"} {
		cs.peerMerken(u, jetzt.Add(time.Hour*3))
		var c int
		cs.db.QueryRow(`SELECT COUNT(*) FROM bekannte_peers WHERE url = $1`, u).Scan(&c)
		if c != 0 {
			t.Fatalf("%q gemerkt", u)
		}
	}
	// Laden: hoechstens peerVorratStart, juengste zuerst, ohne selbst.
	spaet := jetzt.Add(1000 * time.Second)
	geladen := cs.peerVorratLaden("http://8.0.3.231:8080", spaet)
	if len(geladen) != peerVorratStart || geladen[0] != "http://8.0.3.230:8080" {
		t.Fatalf("geladen: %v", geladen)
	}
	for _, u := range geladen {
		if u == "http://8.0.3.231:8080" {
			t.Fatal("eigene Adresse geladen")
		}
	}
	// Nach der TTL: nichts.
	if g := cs.peerVorratLaden("", spaet.Add(peerVorratTTL+time.Hour)); len(g) != 0 {
		t.Fatalf("abgelaufene Peers geladen: %v", g)
	}
	// Schreibabstand: ein zweites Merken desselben Peers kurz danach
	// aendert nichts.
	u := "http://8.8.8.8:8080"
	cs.peerMerken(u, jetzt)
	cs.peerMerken(u, jetzt.Add(time.Minute))
	var z int64
	cs.db.QueryRow(`SELECT zuletzt FROM bekannte_peers WHERE url = $1`, u).Scan(&z)
	if z != 0 && z != jetzt.Unix() {
		t.Fatalf("Schreibabstand nicht eingehalten: %d", z)
	}
}

// Gemerkte Peers werden nie trustedSeeds (ihre Bloecke bekaemen sonst die
// FromSync-Ausnahmen).
func TestPeerVorrat_NieSeed_RealDB(t *testing.T) {
	skipUnlessRealDBBenchEnv(t)
	cs := testKnoten(t, "unused-peer-vorrat-seed.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	cs.peerVorratTabelle()
	cs.db.Exec(`DELETE FROM bekannte_peers`)
	t.Cleanup(func() { cs.db.Exec(`DELETE FROM bekannte_peers`) })
	cs.db.Exec(`INSERT INTO bekannte_peers (url, zuletzt) VALUES ('http://9.9.9.9:8080', $1)`, time.Now().Unix())
	dag := &BlockDAG{state: cs, activeSyncPeers: map[string]bool{}, trustedSeeds: map[string]bool{}}
	geladen := cs.peerVorratLaden("", time.Now())
	if len(geladen) != 1 {
		t.Fatalf("geladen: %v", geladen)
	}
	if dag.isTrustedSyncSource(geladen[0]) {
		t.Fatal("gemerkter Peer gilt als Seed")
	}
}

func TestLeitung_MitgliedURLsNurSatzOhneSelbst(t *testing.T) {
	a, b := "0x"+strings.Repeat("1", 40), "0x"+strings.Repeat("2", 40)
	l := NeueLeitung(a, "http://8.8.4.4:8080", []string{a, b}, a, true, LeitSpeicher{}, testKonfig(), LeitUmgebung{}, time.Now())
	l.SetzeURL(b, "http://8.8.8.8:8080/")
	l.SetzeURL("0x"+strings.Repeat("3", 40), "http://1.1.1.1:8080") // kein Mitglied
	got := l.MitgliedURLs()
	if len(got) != 1 || got[0] != "http://8.8.8.8:8080" {
		t.Fatalf("MitgliedURLs: %v", got)
	}
}

// Missbrauch (Pruefung der gemerkten Peers, MITTEL): ein Leiter, der die
// URLs reihum wechselt, belegt hoechstens leitungPeerPlaetze Abgleiche.
func TestLeitungPeers_HoechstensVierPlaetze(t *testing.T) {
	alt := leitungSyncStarten
	t.Cleanup(func() { leitungSyncStarten = alt })
	leitungSyncStarten = func(dag *BlockDAG, u string) {
		dag.syncPeerMu.Lock()
		dag.activeSyncPeers[u] = true
		dag.syncPeerMu.Unlock()
	}
	dag := &BlockDAG{activeSyncPeers: map[string]bool{}, trustedSeeds: map[string]bool{}}
	for runde := 0; runde < 25; runde++ {
		var urls []string
		for i := 0; i < 20; i++ {
			urls = append(urls, fmt.Sprintf("http://8.8.%d.%d:8080", runde, i))
		}
		dag.leitungPeersStarten(urls)
	}
	if n := len(dag.activeSyncPeers); n != leitungPeerPlaetze {
		t.Fatalf("%d Abgleiche aus der Leitung, Grenze %d", n, leitungPeerPlaetze)
	}
	// Endet einer (syncPeerAufgebenNach), wird der Platz wieder frei.
	for u := range dag.activeSyncPeers {
		delete(dag.activeSyncPeers, u)
		break
	}
	dag.leitungPeersStarten([]string{"http://8.8.200.1:8080"})
	if !dag.activeSyncPeers["http://8.8.200.1:8080"] {
		t.Fatal("freier Platz wird nicht wieder vergeben")
	}
}

// Ein Abgleich, der dauernd scheitert, zaehlt seine Fehlschlaege -- die
// Grundlage dafuer, dass syncWithNode endet und den Platz freigibt.
func TestSyncTakt_FehlFolge(t *testing.T) {
	st := &syncTaktStand{backoff: time.Second}
	for i := 0; i < syncPeerAufgebenNach; i++ {
		st.runde(func() {}, func() bool { return false }, func(time.Duration) {})
	}
	if st.fehlFolge != syncPeerAufgebenNach {
		t.Fatalf("fehlFolge %d", st.fehlFolge)
	}
	st.runde(func() {}, func() bool { return true }, func(time.Duration) {})
	if st.fehlFolge != 0 {
		t.Fatal("ein gelungener Takt setzt die Folge nicht zurueck")
	}
}
