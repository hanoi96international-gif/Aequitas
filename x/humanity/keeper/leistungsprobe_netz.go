package keeper

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	mrand "math/rand"
	"net/http"
	"strings"
	"time"
)

const leitArtProbe = "leistungsprobe"

var probeZufall = mrand.New(mrand.NewSource(time.Now().UnixNano()))

// leistungsprobenPlanen: aus dem Leitungstakt -- jedes andere Mitglied zu
// zufaelligen Zeiten pruefen.
func (dag *BlockDAG) leistungsprobenPlanen(l *Leitung) {
	for a, u := range l.URLs() {
		if a == l.ich || u == "" || u == l.url {
			continue
		}
		if proben.faellig(a, time.Now(), probeZufall) {
			addr, url := a, u
			SafeGoroutine("leistungsprobe", func() { dag.leistungsprobeSenden(l, addr, url) })
		}
	}
}

func (dag *BlockDAG) probeAnfrage(l *Leitung, n int, seed string) LeitNachricht {
	m := LeitNachricht{Art: leitArtProbe, Von: l.ich, ZeitMs: time.Now().UnixMilli(), BlockHash: seed, Hoehe: int64(n)}
	dag.signiereLeitNachricht(&m)
	return m
}

func probePost(url string, m LeitNachricht) (string, time.Duration, error) {
	b, _ := json.Marshal(m)
	t0 := time.Now()
	resp, err := leitungKlient.Post(url+"/api/leistungsprobe", "application/json", bytes.NewReader(b))
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	var antw struct {
		Ergebnis string `json:"ergebnis"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&antw)
	return antw.Ergebnis, time.Since(t0), err
}

// leistungsprobeSenden: Laufzeit messen (leere Probe), dann die Aufgabe,
// dann selbst nachrechnen.
func (dag *BlockDAG) leistungsprobeSenden(l *Leitung, addr, url string) {
	_, rtt, err := probePost(url, dag.probeAnfrage(l, 0, ""))
	if err != nil {
		return // nicht erreichbar: das misst die Leitung (ausgefallen), nicht die Probe
	}
	var s [16]byte
	rand.Read(s[:])
	seed := hex.EncodeToString(s[:])
	erhalten, dauer, err := probePost(url, dag.probeAnfrage(l, probeAnzahl, seed))
	if err != nil {
		return
	}
	sb, _ := hex.DecodeString(seed)
	erwartet, err := leistungsprobeRechnen(sb, probeAnzahl)
	if err != nil {
		return
	}
	ok := probeBestanden(erwartet, erhalten, dauer, rtt, probeAnzahl, leistungsSchwellenAusUmgebung().MinSigProSek)
	proben.merke(strings.ToLower(addr), probeErgebnis{bestanden: ok, zeit: time.Now(), dauer: dauer})
}

// handleLeistungsprobe: POST /api/leistungsprobe -- nur von Mitgliedern,
// signiert, hoechstens eine je Pruefer alle 30 s.
func (a *APIServer) handleLeistungsprobe(w http.ResponseWriter, r *http.Request) {
	l := a.state.leitung.Load()
	if l == nil || r.Method != http.MethodPost {
		http.Error(w, `{"error":"nicht verfuegbar"}`, http.StatusNotFound)
		return
	}
	var m LeitNachricht
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&m); err != nil || m.Art != leitArtProbe {
		http.Error(w, `{"error":"unlesbar"}`, http.StatusBadRequest)
		return
	}
	if err := pruefeLeitNachricht(m, time.Now()); err != nil || !l.IstMitglied(strings.ToLower(m.Von)) {
		http.Error(w, `{"error":"abgewiesen"}`, http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if m.Hoehe == 0 {
		json.NewEncoder(w).Encode(map[string]string{"ergebnis": ""})
		return
	}
	if m.Hoehe != probeAnzahl || !proben.annehmen(strings.ToLower(m.Von), time.Now()) {
		http.Error(w, `{"error":"zu haeufig oder falsche Groesse"}`, http.StatusTooManyRequests)
		return
	}
	seed, err := hex.DecodeString(m.BlockHash)
	if err != nil || len(seed) != 16 {
		http.Error(w, `{"error":"Zufallswert"}`, http.StatusBadRequest)
		return
	}
	erg, err := leistungsprobeRechnen(seed, probeAnzahl)
	if err != nil {
		http.Error(w, `{"error":"Rechnung"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"ergebnis": erg})
}
