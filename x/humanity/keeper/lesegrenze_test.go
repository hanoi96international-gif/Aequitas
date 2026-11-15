package keeper

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
