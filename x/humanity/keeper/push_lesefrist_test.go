package keeper

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// pushLeseServer: dieselbe Kette und dieselben Server-Fristen wie im
// Betrieb, der Handler liest wie handleBlockPush (Platz, Lesefrist, Rumpf).
func pushLeseServer(t *testing.T) (*httptest.Server, chan error) {
	t.Helper()
	ergebnis := make(chan error, 4)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		frei, ok := blockPushPlatz("198.51.100.77")
		if !ok {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			ergebnis <- fmt.Errorf("busy")
			return
		}
		defer frei()
		pushMitLeseFrist(w, r)
		_, _, err := readPushBody(w, r, maxBlockStreamBytes)
		ergebnis <- err
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewUnstartedServer(serverKette(h))
	srv.Config.ReadTimeout = 30 * time.Second
	srv.Config.WriteTimeout = 60 * time.Second
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, ergebnis
}

func pushLeseFristKuerzen(t *testing.T) {
	t.Helper()
	anlauf, rate := pushLeseAnlauf, pushMindestRate
	pushLeseAnlauf, pushMindestRate = 500*time.Millisecond, 32<<10
	t.Cleanup(func() { pushLeseAnlauf, pushMindestRate = anlauf, rate })
}

// MISSBRAUCH: wer einen Push-Platz belegt und nur tropft, verliert ihn nach
// dem Anlauf -- nicht erst nach der ReadTimeout des Servers (30 s).
func TestPushLesefrist_TropfenVerliertDenPlatz(t *testing.T) {
	pushLeseFristKuerzen(t)
	srv, ergebnis := pushLeseServer(t)
	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	start := time.Now()
	fmt.Fprintf(conn, "POST /api/blocks/push HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: 100000\r\n\r\n")
	stopp := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			select {
			case <-stopp:
				return
			case <-time.After(100 * time.Millisecond):
			}
			if _, err := conn.Write([]byte("{")); err != nil {
				return
			}
		}
	}()
	defer close(stopp)
	select {
	case err := <-ergebnis:
		dauer := time.Since(start)
		if err == nil {
			t.Fatal("tropfender Push wurde vollstaendig gelesen")
		}
		if dauer > 3*time.Second {
			t.Fatalf("Platz erst nach %v frei -- die Mindestrate greift nicht", dauer)
		}
		t.Logf("tropfender Push nach %v abgebrochen: %v", dauer.Round(time.Millisecond), err)
	case <-time.After(10 * time.Second):
		t.Fatal("tropfender Push haelt den Platz laenger als 10 s")
	}
	// Der Platz ist wieder frei.
	for i := 0; i < 50 && len(blockPushPlaetze) != 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if n := len(blockPushPlaetze); n != 0 {
		t.Fatalf("%d Plaetze nach dem Abbruch noch belegt", n)
	}
}

// Gegenprobe: ein zuegiger Push, auch ein grosser, geht durch.
func TestPushLesefrist_ZuegigerPushGehtDurch(t *testing.T) {
	pushLeseFristKuerzen(t)
	srv, ergebnis := pushLeseServer(t)
	rumpf := bytes.Repeat([]byte("x"), 4<<20)
	resp, err := http.Post(srv.URL+"/api/blocks/push", "application/json", bytes.NewReader(rumpf))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if err := <-ergebnis; err != nil {
		t.Fatalf("zuegiger Push abgebrochen: %v", err)
	}
}

// Der echte Handler setzt die Frist, bevor er den Rumpf liest.
func TestPushLesefrist_HandlerSetztSie(t *testing.T) {
	roh, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`(?s)func \(a \*APIServer\) handleBlockPush.*?pushMitLeseFrist\(w, r\)\s*\n\s*body, tooLarge, err := readPushBody`).Match(roh) {
		t.Fatal("handleBlockPush liest den Rumpf ohne pushMitLeseFrist")
	}
}
