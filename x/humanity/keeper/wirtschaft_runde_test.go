package keeper

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// Eine Runde, wie RunDailyDistributionAtomic sie in einen Block legt:
// Abgaben, Grundeinkommen je Mensch, Endstand des Topfs, Marke am Ende.
func rundeTxs(at int64, ubiZeit int64) []Transaction {
	return []Transaction{
		{Type: "transfer", Wallet: "0xa", To: "0xb", Amount: 5}, // gehoert nicht dazu
		{Type: "umlauf", Wallet: "0xA", Amount: 0.166667, DistributionAt: at},
		{Type: "umlauf", Wallet: "0xb", Amount: 0.05, DistributionAt: at},
		{Type: "ubi_distribution", Wallet: "0xa", Amount: 0.108333, DistributionAt: ubiZeit},
		{Type: "ubi_distribution", Wallet: "0xb", Amount: 0.108333, DistributionAt: ubiZeit},
		{Type: "ubi_distribution_finalize", Amount: 0.000001, DistributionAt: at},
		{Type: "distribution_round_marker", DistributionAt: at},
	}
}

func TestRundenBuchungen_Zuordnung(t *testing.T) {
	// Grundeinkommen ohne eigene Rundenzeit (vor lebensnachweisAktiv) gehoert
	// zur Runde des Blocks; Adressen klein; Endstand und Marke ohne Adresse.
	bu := rundenBuchungen(rundeTxs(1000, 0))
	if len(bu) != 6 {
		t.Fatalf("%d Buchungen, erwartet 6: %+v", len(bu), bu)
	}
	for _, b := range bu {
		if b.zeit != 1000 {
			t.Fatalf("falsche Rundenzeit: %+v", b)
		}
	}
	if bu[0].adresse != "0xa" || bu[0].mikro != 166667 || bu[4].adresse != "" || bu[4].mikro != 1 {
		t.Fatalf("Zeilen falsch: %+v", bu)
	}
	// Ein Block ohne Runde: nichts. Eine Kappung ohne Runde im Block: nichts.
	if bu := rundenBuchungen([]Transaction{{Type: "transfer", Amount: 1}, {Type: "kappung", Wallet: "0xa", Amount: 2}}); len(bu) != 0 {
		t.Fatalf("Buchungen ohne Runde: %+v", bu)
	}
	// Unlesbare Betraege zaehlen nicht.
	for _, x := range []float64{math.NaN(), math.Inf(1), -1, 1e13} {
		if bu := rundenBuchungen([]Transaction{{Type: "umlauf", Wallet: "0xa", Amount: x, DistributionAt: 5}}); len(bu) != 0 {
			t.Fatalf("Betrag %v gezaehlt", x)
		}
	}
}

func TestRundeAuswerten_Rechnung(t *testing.T) {
	s := map[string]rundeSumme{}
	for _, b := range rundenBuchungen(rundeTxs(2000, 2000)) {
		x := s[b.art]
		if x.anzahl == 0 || b.mikro < x.kleinster {
			x.kleinster = b.mikro
		}
		if b.mikro > x.groesster {
			x.groesster = b.mikro
		}
		x.anzahl++
		x.summe += b.mikro
		x.vonBlock, x.bisBlock = 7, 7
		s[b.art] = x
	}
	vor := int64(10_000) // 0,01 AEQ Rest der Vorrunde
	a := rundeAuswerten(2000, s, 1000, &vor)
	if a["vollstaendig"] != true {
		t.Fatalf("Runde mit Marke und Endstand nicht vollstaendig: %v", a)
	}
	// Topf vor der Auszahlung = ausgezahlt + Rest = 0,216666 + 0,000001.
	if a["topf_vor_auszahlung"] != 0.216667 || a["rest_nach"] != 0.000001 {
		t.Fatalf("Topf %v, Rest %v", a["topf_vor_auszahlung"], a["rest_nach"])
	}
	// andere = 216667 - 10000 - 216667 (Abgaben) = -10000 Mikro.
	if a["andere_zufluesse"] != -0.01 {
		t.Fatalf("andere Zufluesse %v", a["andere_zufluesse"])
	}
	p := a["probe_menschen_mal_anteil"].(map[string]interface{})
	if p["gleich"] != true || p["menschen_mal_anteil_mikro"] != int64(216666) {
		t.Fatalf("Probe %v", p)
	}
	// Ohne Marke: nicht vollstaendig. Ohne Vorrunde: keine Differenz.
	delete(s, "distribution_round_marker")
	b := rundeAuswerten(2000, s, 0, nil)
	if b["vollstaendig"] != false {
		t.Fatal("Runde ohne Marke als vollstaendig gemeldet")
	}
	if _, da := b["andere_zufluesse"]; da {
		t.Fatal("Differenz ohne bekannte Vorrunde")
	}
}

func TestWirtschaftRunde_Eingaben(t *testing.T) {
	a := &APIServer{state: newTestState()}
	for _, q := range []string{"hoehe=-1", "hoehe=0x10", "zeit=abc", "hoehe=1&zeit=2", "hoehe=9999999999999", "zeit=0"} {
		rec := httptest.NewRecorder()
		a.handleWirtschaftRunde(rec, httptest.NewRequest(http.MethodGet, "/api/wirtschaft/runde?"+q, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: Status %d, erwartet 400", q, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	a.handleWirtschaftRunde(rec, httptest.NewRequest(http.MethodPost, "/api/wirtschaft/runde", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", rec.Code)
	}
	rec = httptest.NewRecorder() // ohne Datenbank: 503, keine Panik
	a.handleWirtschaftRunde(rec, httptest.NewRequest(http.MethodGet, "/api/wirtschaft/runde?hoehe=5", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("ohne Datenbank: %d", rec.Code)
	}
}

func TestWirtschaftRunde_RealDB(t *testing.T) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" {
		t.Skip("opt-in only: set AEQUITAS_TPS_BENCH=1 and DATABASE_URL (a disposable local Postgres) to run")
	}
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-runde-db-test.json")
	if !cs.useDB {
		t.Fatal("expected a live PostgreSQL connection")
	}
	cs.ensureRundenTabelle()
	if _, err := cs.db.Exec("DELETE FROM wirtschaft_runden_buchungen"); err != nil {
		t.Fatal(err)
	}
	a := &APIServer{state: cs}
	frage := func(q string) (int, map[string]interface{}) {
		t.Helper()
		rec := httptest.NewRecorder()
		a.handleWirtschaftRunde(rec, httptest.NewRequest(http.MethodGet, "/api/wirtschaft/runde?"+q, nil))
		var m map[string]interface{}
		json.Unmarshal(rec.Body.Bytes(), &m)
		return rec.Code, m
	}
	// Vorrunde (zeit 1000) mit Rest 0,01 AEQ.
	vor := []Transaction{
		{Type: "ubi_distribution", Wallet: "0xa", Amount: 1, DistributionAt: 1000},
		{Type: "ubi_distribution_finalize", Amount: 0.01, DistributionAt: 1000},
		{Type: "distribution_round_marker", DistributionAt: 1000},
	}
	if err := cs.schreibeRundenBuchungen(50, vor); err != nil {
		t.Fatal(err)
	}
	// Runde 2000 in Block 60 -- und dieselbe Runde in einem Geschwisterblock
	// (61) sowie derselbe Block zweimal: zaehlt einmal.
	for _, h := range []int64{60, 60, 61} {
		if err := cs.schreibeRundenBuchungen(h, rundeTxs(2000, 2000)); err != nil {
			t.Fatal(err)
		}
	}
	code, m := frage("zeit=2000")
	if code != http.StatusOK {
		t.Fatalf("Status %d: %v", code, m)
	}
	ubi := m["grundeinkommen"].(map[string]interface{})
	ab := m["abgaben"].(map[string]interface{})
	if ubi["menschen"] != 2.0 || ubi["summe_mikro"] != 216666.0 || ab["konten"] != 2.0 || ab["summe_mikro"] != 216667.0 {
		t.Fatalf("doppelt gezaehlt oder falsch: Grundeinkommen %v, Abgaben %v", ubi, ab)
	}
	if m["rest_vorrunde"] != 0.01 || m["vorrunde"] != 1000.0 || m["andere_zufluesse"] != -0.01 || m["vollstaendig"] != true {
		t.Fatalf("Vorrunde/Differenz: %v", m)
	}
	// Ohne Angabe: die juengste Runde. Ueber den Block: dieselbe Runde.
	if _, j := frage(""); j["zeit"] != 2000.0 {
		t.Fatalf("juengste Runde: %v", j["zeit"])
	}
	if _, h := frage("hoehe=60"); h["zeit"] != 2000.0 {
		t.Fatalf("ueber den Block: %v", h["zeit"])
	}
	// Der Geschwisterblock 61: seine Zeilen fielen als Doppel heraus. Fragt
	// jemand mit seiner Hoehe (etwa aus dem eigenen Kontoverlauf), findet die
	// Abfrage die Runde ueber den Block in der Datenbank -- ohne Doppel.
	if err := cs.SaveBlockToDB(&Block{Height: 61, Hash: fmt.Sprintf("0x%064d", 61), Timestamp: 2001, Transactions: rundeTxs(2000, 2000)}, true); err != nil {
		t.Fatal(err)
	}
	if _, h := frage("hoehe=61"); h["zeit"] != 2000.0 || h["grundeinkommen"].(map[string]interface{})["menschen"] != 2.0 {
		t.Fatalf("ueber den Geschwisterblock: %v", h)
	}
	// Unbekannte Zeit: 404.
	if code, _ := frage("zeit=1500"); code != http.StatusNotFound {
		t.Fatalf("unbekannte Runde: %d", code)
	}

	// Aeltere Runde, noch nicht im Index: aus den Bloecken nachgetragen --
	// nur die zusammenhaengenden Hoehen derselben Runde.
	for h, txs := range map[int64][]Transaction{
		500: {{Type: "transfer", Wallet: "0xa", To: "0xb", Amount: 1}},                                            // davor: keine Runde
		501: rundeTxs(3000, 3000)[:4],                                                                             // Abgaben + erstes Grundeinkommen
		502: append(rundeTxs(3000, 3000)[4:], Transaction{Type: "transfer", Wallet: "0xb", To: "0xa", Amount: 1}), // Rest der Runde
		503: rundeTxs(4000, 4000),                                                                                 // die naechste Runde: nicht mitnehmen
	} {
		blk := &Block{Height: h, Hash: fmt.Sprintf("0x%064d", h), Timestamp: 3000 + h, Transactions: txs}
		if err := cs.SaveBlockToDB(blk, true); err != nil {
			t.Fatal(err)
		}
	}
	code, n := frage("hoehe=502")
	if code != http.StatusOK {
		t.Fatalf("Nachtrag: Status %d: %v", code, n)
	}
	nu := n["grundeinkommen"].(map[string]interface{})
	if n["zeit"] != 3000.0 || nu["menschen"] != 2.0 || n["vollstaendig"] != true {
		t.Fatalf("nachgetragene Runde: %v", n)
	}
	b := n["bloecke"].(map[string]interface{})
	if b["von"] != 501.0 || b["bis"] != 502.0 {
		t.Fatalf("Bloecke der nachgetragenen Runde: %v", b)
	}
	if code, _ := frage("zeit=4000"); code != http.StatusNotFound {
		t.Fatal("die naechste Runde wurde beim Nachtragen mitgenommen")
	}
	// Ein gespeicherter, aber NICHT angewendeter Block (replayed = false,
	// etwa von einem Peer, dessen Pruefung scheiterte) liefert keine Runde --
	// auch nicht, wenn er eine erfundene Runde traegt.
	falsch := []Transaction{
		{Type: "ubi_distribution", Wallet: "0xdead", Amount: 999999, DistributionAt: 7000},
		{Type: "distribution_round_marker", DistributionAt: 7000},
	}
	if err := cs.SaveBlockToDB(&Block{Height: 700, Hash: fmt.Sprintf("0x%064d", 700), Timestamp: 7000, Transactions: falsch}, false); err != nil {
		t.Fatal(err)
	}
	if code, _ := frage("hoehe=700"); code != http.StatusNotFound {
		t.Fatalf("nicht angewendeter Block lieferte eine Runde: %d", code)
	}
	if code, _ := frage("zeit=7000"); code != http.StatusNotFound {
		t.Fatal("erfundene Runde aus einem nicht angewendeten Block im Index")
	}

	// Ein Block ohne Rundenbuchung: 404.
	if code, _ := frage("hoehe=500"); code != http.StatusNotFound {
		t.Fatalf("Block ohne Runde: %d", code)
	}

	// Grenze: Eine Runde ueber mehr als rundeNachtragHoehen Bloecke fuehrt zu
	// begrenzter Arbeit -- nachgetragen werden hoechstens so viele Hoehen, und
	// die Antwort sagt, dass die Runde unvollstaendig ist (keine Marke).
	for h := int64(1000); h < 1000+rundeNachtragHoehen+10; h++ {
		blk := &Block{Height: h, Hash: fmt.Sprintf("0x%064d", h), Timestamp: 5000, Transactions: []Transaction{
			{Type: "umlauf", Wallet: fmt.Sprintf("0x%040d", h), Amount: 1, DistributionAt: 5000}}}
		if err := cs.SaveBlockToDB(blk, true); err != nil {
			t.Fatal(err)
		}
	}
	code, g := frage("hoehe=1000")
	if code != http.StatusOK {
		t.Fatalf("lange Runde: %d %v", code, g)
	}
	// Gezaehlt werden GELESENE Hoehen, auch die eine davor (999, ohne Runde).
	if k := g["abgaben"].(map[string]interface{})["konten"].(float64); k > float64(rundeNachtragHoehen) || k < float64(rundeNachtragHoehen-1) || g["vollstaendig"] != false {
		t.Fatalf("Nachtrag nicht begrenzt: %v Konten, vollstaendig %v", k, g["vollstaendig"])
	}

	// Nur ein Nachtrag zugleich: laeuft schon einer, gibt es 503 statt
	// weiterer Last.
	if err := cs.SaveBlockToDB(&Block{Height: 2000, Hash: fmt.Sprintf("0x%064d", 2000), Timestamp: 6000,
		Transactions: rundeTxs(6000, 6000)}, true); err != nil {
		t.Fatal(err)
	}
	rundeNachtragMu.Lock()
	code, _ = frage("hoehe=2000")
	rundeNachtragMu.Unlock()
	if code != http.StatusServiceUnavailable {
		t.Fatalf("zweiter Nachtrag zugleich: %d, erwartet 503", code)
	}
	if code, _ := frage("hoehe=2000"); code != http.StatusOK {
		t.Fatalf("danach: %d", code)
	}
}
