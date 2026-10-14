package keeper

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// netzFall: vier Knoten an derselben Datenbank (Merker idempotent je Hash),
// derselbe Zustand, dieselben Bloecke; Knoten 0 leitet in Amtszeit 7.
type netzFall struct {
	*vorschlagFall
	knoten  []*BlockDAG
	urls    map[string]string
	aus     map[string]bool
	koepfMu sync.Mutex
}

func neuerNetzFall(t *testing.T) *netzFall {
	t.Helper()
	f := &netzFall{vorschlagFall: neuerVorschlagFall(t, true), urls: map[string]string{}, aus: map[string]bool{}}
	if _, err := f.dag.state.db.Exec(`DROP TABLE IF EXISTS checkpoint_zertifikate`); err != nil {
		t.Fatal(err)
	}
	checkpointZertTabelleDa.Delete(f.dag.state.db)
	alt := checkpointKopfLeser
	checkpointKopfLeser = func(*BlockDAG) kopfLeser {
		lies := f.koepfe.leser()
		return func(h []string) (map[string]blockKopf, error) {
			f.koepfMu.Lock()
			defer f.koepfMu.Unlock()
			return lies(h)
		}
	}
	t.Cleanup(func() { checkpointKopfLeser = alt })
	f.knoten = append(f.knoten, f.dag)
	for i := 1; i < 4; i++ {
		d := newOrphanTestDAG()
		d.state, d.blocks, d.replayedBlocks = f.dag.state, f.dag.blocks, f.dag.replayedBlocks
		d.signingKey, d.selfProposer = f.keys[i], f.adr[i]
		f.knoten = append(f.knoten, d)
	}
	for i, a := range f.adr {
		f.urls[a] = fmt.Sprintf("knoten%d", i)
	}
	return f
}

// senden: wie HTTP, aber direkt an checkpointEmpfangen des Knotens.
func (f *netzFall) senden(jetzt int64) checkpointSender {
	return func(url string, m CheckpointNachricht) ([]byte, error) {
		if f.aus[url] {
			return nil, errors.New("aus")
		}
		var i int
		fmt.Sscanf(url, "knoten%d", &i)
		antw, _, err := f.knoten[i].checkpointEmpfangen(m, f.adr[0], 7, jetzt)
		if err != nil {
			return nil, err
		}
		return json.Marshal(antw)
	}
}

func (f *netzFall) runde(t *testing.T, b *Block) (*CheckpointZertifikat, error) {
	t.Helper()
	jetzt := b.Timestamp + 200
	return f.dag.checkpointRunde(f.adr[0], 7, f.urls, b.Hash, jetzt, f.senden(jetzt))
}

// Zwei Runden: jeder Knoten hat danach denselben letzten Checkpoint; die
// zweite baut auf der ersten auf (Kette).
func TestCheckpointRunde_RealDB(t *testing.T) {
	f := neuerNetzFall(t)
	b1, b2 := f.blocks[1], f.blocks[2]
	z, err := f.runde(t, b1)
	if err != nil || z == nil {
		t.Fatalf("Runde 1: %v %v", z, err)
	}
	for i, d := range f.knoten {
		if c := d.letzterCheckpoint(); c == nil || c.Hash != b1.Hash {
			t.Fatalf("Knoten %d: letzter %v", i, c)
		}
	}
	z, err = f.runde(t, b2)
	if err != nil || z == nil || z.Hash != b2.Hash {
		t.Fatalf("Runde 2: %v %v", z, err)
	}
	for i, d := range f.knoten {
		if c := d.letzterCheckpoint(); c == nil || c.Hash != b2.Hash {
			t.Fatalf("Knoten %d: letzter %v", i, c)
		}
	}
	// Neustart: der letzte Checkpoint kommt aus der Datenbank.
	neu := newOrphanTestDAG()
	neu.state = f.dag.state
	if err := neu.checkpointLetzterLaden(); err != nil {
		t.Fatal(err)
	}
	if c := neu.letzterCheckpoint(); c == nil || c.Hash != b2.Hash {
		t.Fatalf("nach Neustart: %v", c)
	}
}

// Ein Block, der den letzten Checkpoint nicht in seiner Vergangenheit hat,
// bekommt kein Zertifikat -- schon der Leiter unterschreibt ihn nicht.
func TestCheckpointRunde_NurNachfolger_RealDB(t *testing.T) {
	f := neuerNetzFall(t)
	if z, err := f.runde(t, f.blocks[1]); err != nil || z == nil {
		t.Fatalf("Runde 1: %v %v", z, err)
	}
	if z, err := f.runde(t, f.seite); err == nil || z != nil {
		t.Fatalf("Seitenblock zertifiziert: %v %v", z, err)
	}
	if c := f.knoten[2].letzterCheckpoint(); c.Hash != f.blocks[1].Hash {
		t.Fatalf("letzter geaendert: %v", c)
	}
}

// Ausfall: einer aus -- Quorum (3 von 4) noch da; zwei aus -- kein
// Zertifikat, niemand speichert etwas.
func TestCheckpointRunde_Ausfall_RealDB(t *testing.T) {
	f := neuerNetzFall(t)
	f.aus["knoten3"] = true
	if z, err := f.runde(t, f.blocks[1]); err != nil || z == nil {
		t.Fatalf("einer aus: %v %v", z, err)
	}
	f.aus["knoten2"] = true
	z, err := f.runde(t, f.blocks[2])
	if err != nil || z != nil {
		t.Fatalf("zwei aus: %v %v", z, err)
	}
	if c := f.knoten[1].letzterCheckpoint(); c == nil || c.Hash != f.blocks[1].Hash {
		t.Fatalf("Knoten 1: %v", c)
	}
}

// Ein gefaelschtes oder fremdes Zertifikat wird von keinem Mitglied
// angenommen: falsche Unterschrift, zu wenige, Block ohne den letzten
// Checkpoint in seiner Vergangenheit (gueltig unterschrieben!).
func TestCheckpointZertifikat_Faelschung_RealDB(t *testing.T) {
	f := neuerNetzFall(t)
	if z, err := f.runde(t, f.blocks[1]); err != nil || z == nil {
		t.Fatalf("Runde 1: %v %v", z, err)
	}
	jetzt := f.seite.Timestamp + 200
	c := f.vorschlag(t, f.seite)
	echt := CheckpointZertifikat{KettenCheckpoint: c}
	for _, k := range f.keys[:3] {
		u, _ := checkpointUnterschreiben(c, k)
		echt.Unterschriften = append(echt.Unterschriften, u)
	}
	zertifikatOrdnen(&echt)
	kaputt := *zertifikatKopie(&echt)
	kaputt.Hash = f.blocks[2].Hash
	kaputt.Hoehe, kaputt.Zeit = f.blocks[2].Height, f.blocks[2].Timestamp
	wenige := *zertifikatKopie(&echt)
	wenige.Unterschriften = wenige.Unterschriften[:2]
	for name, z := range map[string]CheckpointZertifikat{"ohne Vorfahr": echt, "falsche Unterschrift": kaputt, "zu wenige": wenige} {
		z := z
		if _, _, err := f.knoten[3].checkpointEmpfangen(CheckpointNachricht{Art: "zertifikat", Zertifikat: &z}, f.adr[0], 7, jetzt); err == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
	if c := f.knoten[3].letzterCheckpoint(); c.Hash != f.blocks[1].Hash {
		t.Fatalf("letzter geaendert: %v", c)
	}
}

func TestCheckpointEmpfangen_Art(t *testing.T) {
	f := neuerVorschlagFall(t, false)
	v := f.leiterVorschlag(t, f.blocks[1])
	z := CheckpointZertifikat{KettenCheckpoint: v.KettenCheckpoint}
	for name, m := range map[string]CheckpointNachricht{
		"leer":      {},
		"unbekannt": {Art: "x", Vorschlag: &v},
		"beides":    {Art: "vorschlag", Vorschlag: &v, Zertifikat: &z},
		"ohne":      {Art: "zertifikat"},
	} {
		if _, status, err := f.dag.checkpointEmpfangen(m, f.adr[1], 7, v.Zeit+200); err == nil || status != http.StatusBadRequest {
			t.Fatalf("%s: %d %v", name, status, err)
		}
	}
	// Ohne Datenbank kein Vorschlag (fail-closed).
	if _, _, err := f.dag.checkpointEmpfangen(CheckpointNachricht{Art: "vorschlag", Vorschlag: &v}, f.adr[1], 7, v.Zeit+200); err == nil {
		t.Fatal("ohne Datenbank unterschrieben")
	}
}

func TestHandleCheckpoint_Grenzen(t *testing.T) {
	cs := &ChainState{}
	a := &APIServer{state: cs, blockchain: newOrphanTestDAG()}
	frage := func(methode, rumpf string) int {
		req := httptest.NewRequest(methode, "/api/checkpoint", strings.NewReader(rumpf))
		req.RemoteAddr = "10.9.8.7:1234"
		w := httptest.NewRecorder()
		a.handleCheckpoint(w, req)
		return w.Code
	}
	t.Setenv(checkpointsEnv, "")
	if c := frage("POST", "{}"); c != http.StatusNotFound {
		t.Fatalf("aus: %d", c)
	}
	t.Setenv(checkpointsEnv, "an")
	if c := frage("POST", "{}"); c != http.StatusNotFound {
		t.Fatalf("ohne Leitung: %d", c)
	}
	cs.leitung.Store(NeueLeitung("0x"+strings.Repeat("1", 40), "", []string{"0x" + strings.Repeat("1", 40)}, "", true, LeitSpeicher{}, testKonfig(), LeitUmgebung{}, time.Now()))
	if c := frage("GET", ""); c != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", c)
	}
	if c := frage("POST", `{"art":"x"}`); c != http.StatusBadRequest {
		t.Fatalf("Art: %d", c)
	}
	gross := `{"art":"zertifikat","zertifikat":{"hash":"` + strings.Repeat("a", checkpointRumpfGrenze) + `"}}`
	if c := frage("POST", gross); c != http.StatusBadRequest {
		t.Fatalf("zu gross: %d", c)
	}
	for i := 0; i < checkpointGleichzeitig; i++ {
		checkpointInArbeit <- struct{}{}
	}
	c := frage("POST", "{}")
	for i := 0; i < checkpointGleichzeitig; i++ {
		<-checkpointInArbeit
	}
	if c != http.StatusServiceUnavailable {
		t.Fatalf("ausgelastet: %d", c)
	}
}

func TestCheckpointWahl(t *testing.T) {
	d := newOrphanTestDAG()
	jetzt := int64(1_000_000)
	var vor string
	for i, alter := range []int64{900, 500, 300, 200, 100, 10} {
		h := hx(500 + i)
		b := &Block{Hash: h, Height: int64(i + 1), Timestamp: jetzt - alter}
		if vor != "" {
			b.ParentHashes = []string{vor}
		}
		d.blocks[h] = b
		vor = h
	}
	if got := d.checkpointWahl(jetzt); got != "" {
		t.Fatalf("ohne eigenen Block: %s", got)
	}
	d.letzterEigenerBlock.Store(vor)
	if got := d.checkpointWahl(jetzt); got != hx(503) {
		t.Fatalf("Wahl %s, erwartet der 200 s alte", got)
	}
	if got := d.checkpointWahl(jetzt + 600); got != "" {
		t.Fatalf("alles zu alt: %s", got)
	}
}
