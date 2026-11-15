package keeper

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// B7: Grenze je Absender und Endpunkt -- nach lesenJeAbsender Anfragen 429,
// ein anderer Absender ist nicht betroffen.
func TestLesegrenze_JeAbsender(t *testing.T) {
	a := &APIServer{}
	n := 0
	h := a.lesenBegrenzt("/api/test-lesegrenze", func(w http.ResponseWriter, r *http.Request) { n++ })
	anfrage := func(ip string) int {
		r := httptest.NewRequest(http.MethodGet, "/api/test-lesegrenze", nil)
		r.RemoteAddr = ip + ":1234"
		w := httptest.NewRecorder()
		h(w, r)
		return w.Code
	}
	for i := 0; i < lesenJeAbsender; i++ {
		if c := anfrage("203.0.113.7"); c != http.StatusOK {
			t.Fatalf("Anfrage %d: %d", i, c)
		}
	}
	if c := anfrage("203.0.113.7"); c != http.StatusTooManyRequests {
		t.Fatalf("ueber der Grenze: %d", c)
	}
	if c := anfrage("203.0.113.8"); c != http.StatusOK {
		t.Fatalf("anderer Absender gesperrt: %d", c)
	}
	if n != lesenJeAbsender+1 {
		t.Fatalf("Handler %d mal gerufen", n)
	}
}

// B7: unbekannte Adresse nimmt keine Schreibsperre und legt kein Konto an;
// ungueltige Adresse wird abgewiesen.
func TestLesegrenze_UnbekannteAdresseOhneSperreUndOhneAnlegen(t *testing.T) {
	cs := newTestState()
	a := "0x00000000000000000000000000000000000000c3"
	cs.mu.RLock() // haelt eine Lesesperre: ein Schreib-Lock im Lesepfad wuerde hier haengen
	fertig := make(chan struct{})
	go func() {
		_ = cs.GetBalance(a)
		_ = cs.IsHuman(a)
		_ = cs.GetTUsdBalance(a)
		_ = cs.GetDemurrageStatus(a)
		close(fertig)
	}()
	<-fertig
	cs.mu.RUnlock()
	if _, ok := cs.accounts.Get(a); ok {
		t.Fatal("Lesepfad hat ein Konto angelegt")
	}
	for _, s := range []string{"", "0x123", "0xZZ00000000000000000000000000000000000000", "abc"} {
		if _, ok := lesbareAdresse(s); ok {
			t.Errorf("%q als Adresse angenommen", s)
		}
	}
	if _, ok := lesbareAdresse("0x00000000000000000000000000000000000000C3"); !ok {
		t.Error("Grossbuchstaben abgewiesen")
	}
}

// B16: /api/wirtschaft/konto mit vielen fremden Adressen laesst die
// Buchfuehrung nicht wachsen und setzt keinen Monatszaehler eines
// bestehenden Kontos zurueck.
func TestWirtschaftKonto_LegtKeinBuchAn(t *testing.T) {
	cs := newTestState()
	a := &APIServer{state: cs}
	wi := cs.wirt()
	jetzt := time.Now().Unix()
	alt := "0x00000000000000000000000000000000000000a1"
	wi.mu.Lock()
	vormonat := monatVon(jetzt - 40*86400)
	wi.buch[alt] = &buchKonto{Monat: vormonat, Ausgegeben: 7}
	vorher := len(wi.buch)
	wi.mu.Unlock()
	for i := 0; i < 500; i++ {
		addr := fmt.Sprintf("0x%040x", 0xb000+i)
		r := httptest.NewRequest(http.MethodGet, "/api/wirtschaft/konto?adresse="+addr, nil)
		w := httptest.NewRecorder()
		a.handleWirtschaftKonto(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d", addr, w.Code)
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/api/wirtschaft/konto?adresse="+alt, nil)
	a.handleWirtschaftKonto(httptest.NewRecorder(), r)
	wi.mu.Lock()
	defer wi.mu.Unlock()
	if len(wi.buch) != vorher {
		t.Fatalf("Buch gewachsen: %d -> %d", vorher, len(wi.buch))
	}
	if k := wi.buch[alt]; k.Monat != vormonat || k.Ausgegeben != 7 {
		t.Fatalf("Abfrage hat das Buchkonto veraendert: %+v", *k)
	}
	if v := wi.kontoAnsichtLocked(alt, jetzt); v.Ausgegeben != 0 || v.Monat != monatVon(jetzt) {
		t.Fatalf("Ansicht stellt den Monat nicht um: %+v", *v)
	}
}
