package keeper

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// Pruefer (ein festgelegter Produzent) und Kandidat, beide echt signierend.
func probeParteien(t *testing.T) (pruefer *BlockDAG, kandidat *APIServer) {
	t.Helper()
	pk, _ := crypto.GenerateKey()
	kk, _ := crypto.GenerateKey()
	pAddr := strings.ToLower(crypto.PubkeyToAddress(pk.PublicKey).Hex())
	kAddr := strings.ToLower(crypto.PubkeyToAddress(kk.PublicKey).Hex())
	pruefer = &BlockDAG{signingKey: pk, selfProposer: pAddr, produzentenFest: map[string]bool{pAddr: true}}
	kandidat = &APIServer{state: &ChainState{}, blockchain: &BlockDAG{signingKey: kk, selfProposer: kAddr, produzentenFest: map[string]bool{pAddr: true}}}
	return
}

func mitKlient(t *testing.T) {
	t.Helper()
	echt := kandidatKlient
	kandidatKlient = &http.Client{Timeout: 5 * time.Second} // httptest laeuft auf 127.0.0.1
	t.Cleanup(func() { kandidatKlient = echt })
}

func mitSchwelle(t *testing.T, minSig string) {
	t.Helper()
	t.Setenv(leistungMinSigEnv, minSig)
}

func frischerStand(t *testing.T) {
	t.Helper()
	alt := kandidaten
	kandidaten = &kandidatenStand{e: map[string]KandidatErgebnis{}}
	t.Cleanup(func() { kandidaten = alt })
}

func TestKandidatenprobe_EchterKandidatBesteht(t *testing.T) {
	mitKlient(t)
	mitSchwelle(t, "1") // hier geht es um das Protokoll, nicht um die Hardware des Testrechners
	pruefer, kandidat := probeParteien(t)
	srv := httptest.NewServer(http.HandlerFunc(kandidat.handleLeistungsprobe))
	defer srv.Close()

	e := pruefer.kandidatProben(srv.URL)
	if e.Status != "bestanden" {
		t.Fatalf("Status %q (%s), erwartet bestanden", e.Status, e.Grund)
	}
}

// Faelschung: ein Kandidat, der nicht rechnet, sondern irgendetwas
// zurueckgibt, faellt durch -- auch wenn er blitzschnell ist.
func TestKandidatenprobe_FalschesErgebnisFaelltDurch(t *testing.T) {
	mitKlient(t)
	mitSchwelle(t, "1")
	pruefer, _ := probeParteien(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"ergebnis": strings.Repeat("ab", 32)})
	}))
	defer srv.Close()

	if e := pruefer.kandidatProben(srv.URL); e.Status != "nicht_bestanden" {
		t.Fatalf("Status %q, erwartet nicht_bestanden fuer ein erfundenes Ergebnis", e.Status)
	}
}

// Zu langsam: richtiges Ergebnis, aber ueber der Grenze.
func TestKandidatenprobe_ZuLangsamFaelltDurch(t *testing.T) {
	mitKlient(t)
	mitSchwelle(t, "1000000000") // Grenze praktisch null
	pruefer, kandidat := probeParteien(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		kandidat.handleLeistungsprobe(w, r)
	}))
	defer srv.Close()

	e := pruefer.kandidatProben(srv.URL)
	if e.Status != "nicht_bestanden" || !strings.Contains(e.Grund, "zu langsam") {
		t.Fatalf("Status %q (%s), erwartet nicht_bestanden wegen Zeit", e.Status, e.Grund)
	}
}

// Nicht erreichbar heisst abgewiesen, nicht "durchgewunken".
func TestKandidatenprobe_UnerreichbarFaelltDurch(t *testing.T) {
	mitKlient(t)
	pruefer, _ := probeParteien(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	if e := pruefer.kandidatProben(srv.URL); e.Status != "nicht_bestanden" {
		t.Fatalf("Status %q, erwartet nicht_bestanden", e.Status)
	}
}

// Fremder Absender: der Kandidat rechnet nur fuer festgelegte Produzenten
// (sonst waere die Probe ein Hebel, jeden Knoten auszulasten).
func TestLeistungsprobe_FremderPrueferWirdAbgewiesen(t *testing.T) {
	_, kandidat := probeParteien(t)
	fk, _ := crypto.GenerateKey()
	fremd := &BlockDAG{signingKey: fk, selfProposer: strings.ToLower(crypto.PubkeyToAddress(fk.PublicKey).Hex())}
	m := LeitNachricht{Art: leitArtProbe, Von: fremd.selfProposer, ZeitMs: time.Now().UnixMilli(), BlockHash: strings.Repeat("00", 16), Hoehe: probeAnzahl}
	fremd.signiereLeitNachricht(&m)
	b, _ := json.Marshal(m)
	rec := httptest.NewRecorder()
	kandidat.handleLeistungsprobe(rec, httptest.NewRequest("POST", "/api/leistungsprobe", bytes.NewReader(b)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("Code %d, erwartet 403 fuer einen Pruefer, der kein Produzent ist", rec.Code)
	}
}

// Faelschung des Absenders: Von nennt den Produzenten, signiert hat ein
// anderer.
func TestLeistungsprobe_GefaelschterAbsenderWirdAbgewiesen(t *testing.T) {
	pruefer, kandidat := probeParteien(t)
	fk, _ := crypto.GenerateKey()
	m := LeitNachricht{Art: leitArtProbe, Von: pruefer.selfProposer, ZeitMs: time.Now().UnixMilli(), BlockHash: strings.Repeat("00", 16), Hoehe: probeAnzahl}
	(&BlockDAG{signingKey: fk}).signiereLeitNachricht(&m)
	b, _ := json.Marshal(m)
	rec := httptest.NewRecorder()
	kandidat.handleLeistungsprobe(rec, httptest.NewRequest("POST", "/api/leistungsprobe", bytes.NewReader(b)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("Code %d, erwartet 403 fuer eine fremde Signatur", rec.Code)
	}
}

// Solange keine bestandene Probe vorliegt, ist das Tor zu (fail-closed),
// und eine laufende Probe wird nicht ein zweites Mal gestartet.
func TestKandidatenStand_OhneErgebnisZuUndKeinDoppelstart(t *testing.T) {
	frischerStand(t)
	gestartet := 0
	start := func(string, string) { gestartet++ }
	jetzt := time.Now()
	for i := 0; i < 5; i++ {
		if e := kandidaten.pruefeOderStarte("0xaa", "http://1.2.3.4:8080", jetzt, start); e.Status == "bestanden" {
			t.Fatal("ohne Probe zugelassen")
		}
	}
	if gestartet != 1 {
		t.Fatalf("%d Proben gestartet, erwartet 1", gestartet)
	}
}

// Wiederholung: ein durchgefallener Kandidat loest vor Ablauf der Sperre
// keine neue Probe aus (jede Anmeldung alle 30 s waere sonst Arbeit fuer uns).
func TestKandidatenStand_DurchgefallenSperrtWiederholung(t *testing.T) {
	frischerStand(t)
	gestartet := 0
	start := func(string, string) { gestartet++ }
	jetzt := time.Now()
	kandidaten.pruefeOderStarte("0xbb", "http://1.2.3.4:8080", jetzt, start)
	kandidaten.fertig("0xbb", KandidatErgebnis{Status: "nicht_bestanden", ZeitUnix: jetzt.Unix()})
	if e := kandidaten.pruefeOderStarte("0xbb", "http://1.2.3.4:8080", jetzt.Add(time.Minute), start); e.Status != "nicht_bestanden" || gestartet != 1 {
		t.Fatalf("Status %q, %d Starts -- erwartet gesperrt", e.Status, gestartet)
	}
	if kandidaten.pruefeOderStarte("0xbb", "http://1.2.3.4:8080", jetzt.Add(kandidatWiederholAbstand+time.Second), start); gestartet != 2 {
		t.Fatalf("%d Starts nach Ablauf der Sperre, erwartet 2", gestartet)
	}
}

// Bestanden gilt nicht ewig: nach Ablauf wird neu gemessen.
func TestKandidatenStand_BestandenLaeuftAb(t *testing.T) {
	frischerStand(t)
	gestartet := 0
	start := func(string, string) { gestartet++ }
	jetzt := time.Now()
	kandidaten.pruefeOderStarte("0xcc", "u", jetzt, start)
	kandidaten.fertig("0xcc", KandidatErgebnis{Status: "bestanden", ZeitUnix: jetzt.Unix()})
	if e := kandidaten.pruefeOderStarte("0xcc", "u", jetzt.Add(time.Hour), start); e.Status != "bestanden" {
		t.Fatalf("Status %q, erwartet bestanden", e.Status)
	}
	if e := kandidaten.pruefeOderStarte("0xcc", "u", jetzt.Add(kandidatBestandenGueltig+time.Second), start); e.Status == "bestanden" || gestartet != 2 {
		t.Fatalf("Status %q, %d Starts -- erwartet neue Probe", e.Status, gestartet)
	}
}

// Obergrenzen: nie mehr als kandidatInflightMax Proben gleichzeitig, nie
// mehr als kandidatMaxEintraege gemerkte Ergebnisse.
func TestKandidatenStand_Obergrenzen(t *testing.T) {
	frischerStand(t)
	var mu sync.Mutex
	gestartet := 0
	start := func(string, string) { mu.Lock(); gestartet++; mu.Unlock() }
	jetzt := time.Now()
	for i := 0; i < 10; i++ {
		kandidaten.pruefeOderStarte(strings.Repeat("0", 3)+string(rune('a'+i)), "u", jetzt, start)
	}
	if gestartet != kandidatInflightMax {
		t.Fatalf("%d gleichzeitige Proben, erlaubt %d", gestartet, kandidatInflightMax)
	}
	frischerStand(t)
	for i := 0; i < kandidatMaxEintraege+50; i++ {
		addr := "0x" + strings.Repeat("0", 30) + time.Duration(i).String()
		kandidaten.pruefeOderStarte(addr, "u", jetzt, func(a, u string) {})
		kandidaten.fertig(addr, KandidatErgebnis{Status: "nicht_bestanden", ZeitUnix: jetzt.Unix() + int64(i)})
	}
	if n := len(kandidaten.e); n > kandidatMaxEintraege {
		t.Fatalf("%d Eintraege, Obergrenze %d", n, kandidatMaxEintraege)
	}
}

// Ohne oeffentliche Adresse keine Messung, also keine Aufnahme -- und keine
// Anfrage an eine interne Adresse (SSRF).
func TestKandidatZugelassen_InterneAdresseAbgewiesen(t *testing.T) {
	frischerStand(t)
	_, kandidat := probeParteien(t)
	for _, u := range []string{"", "http://127.0.0.1:8080", "http://10.0.0.5:8080", "http://169.254.169.254", "http://interner-dienst:8080"} {
		if e := kandidat.kandidatZugelassen("0x"+strings.Repeat("1", 40), u); e.Status != "nicht_bestanden" {
			t.Fatalf("%q: Status %q, erwartet nicht_bestanden", u, e.Status)
		}
	}
	if len(kandidaten.e) != 0 {
		t.Fatal("fuer eine interne Adresse wurde eine Probe gestartet")
	}
}

func TestHandleKandidatenprobe(t *testing.T) {
	frischerStand(t)
	a := &APIServer{}
	rec := httptest.NewRecorder()
	a.handleKandidatenprobe(rec, httptest.NewRequest("GET", "/api/kandidatenprobe?signing_address=kaputt", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("Code %d, erwartet 400", rec.Code)
	}
	addr := "0x" + strings.Repeat("a", 40)
	kandidaten.e[addr] = KandidatErgebnis{Status: "nicht_bestanden", Grund: "zu langsam", ZeitUnix: time.Now().Unix()}
	rec = httptest.NewRecorder()
	a.handleKandidatenprobe(rec, httptest.NewRequest("GET", "/api/kandidatenprobe?signing_address="+addr, nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"nicht_bestanden"`) || !strings.Contains(rec.Body.String(), "min_signaturen_pro_sek") {
		t.Fatalf("Antwort %d %s", rec.Code, rec.Body.String())
	}
}

// Ein neuer Knoten ohne AUTHORIZED_VALIDATORS kennt C1 nur als Produzenten,
// dessen Bloecke er annimmt -- auch dann muss er C1s Probe annehmen.
func TestLeistungsprobe_ProduzentAusDemNetzDarfPruefen(t *testing.T) {
	mitKlient(t)
	mitSchwelle(t, "1")
	pruefer, kandidat := probeParteien(t)
	kandidat.blockchain.produzentenFest = nil
	kandidat.blockchain.authorizedValidators = map[string]bool{pruefer.selfProposer: true}
	srv := httptest.NewServer(http.HandlerFunc(kandidat.handleLeistungsprobe))
	defer srv.Close()
	if e := pruefer.kandidatProben(srv.URL); e.Status != "bestanden" {
		t.Fatalf("Status %q (%s), erwartet bestanden", e.Status, e.Grund)
	}
}
