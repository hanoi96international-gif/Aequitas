package keeper

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Audit B1 (14.11.2026): Seit der Coordinator dem Umzug "pq:"-Eintraege
// anhaengt, wies der Knoten den Antrag mit 400 ab ("only v3: and v3k:") und
// haette die PQ-Unterschriften ohnehin nicht in die Transaktion uebernommen.
// Jetzt: angenommen, durchgereicht, und ab dem Stichtag beim Pruefen Pflicht.
func TestUmzugAntrag_PQEintraege(t *testing.T) {
	f := neuerUzFall(t)
	coord, zeuge := pqUmgebung(t, 1) // PQ von Anfang an Pflicht
	f.coord, f.zeuge = coord.ed, zeuge.ed
	f.cs.annehmendAusdruecklich.Store(true)
	a := &APIServer{state: f.cs}
	jetzt := nowUnix()
	antrag := f.antragMit(f.neuKey, f.neu, f.alt, uzN, jetzt, "", umzugPruefungStreng)
	bindungText := string(umzugBindungNachricht("test", uzN, f.neu, jetzt, umzugPruefungStreng))
	aussageText := string(umzugAussageNachricht("test", uzN, jetzt))
	pqGut := []string{"pq:" + coord.pqUnterschrift(t, bindungText), "pq:" + zeuge.pqUnterschrift(t, aussageText)}

	ip := 0
	post := func(eintraege []string) (int, string) {
		t.Helper()
		ip++
		body := map[string]interface{}{"neu": f.neu, "alt": f.alt, "nullifier": uzN, "zeit": jetzt, "modus": "test",
			"bescheinigung": eintraege, "pruefung": umzugPruefungStreng, "sig": antrag.Umzug.Sig}
		roh, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, "/api/umzug/antrag", bytes.NewReader(roh))
		r.RemoteAddr = "198.51.100." + string(rune('0'+ip)) + ":1"
		w := httptest.NewRecorder()
		a.handleUmzugAntrag(w, r)
		return w.Code, w.Body.String()
	}
	v3 := []string{"v3:" + antrag.BioBescheinigung.Signaturen[0], "v3:" + antrag.BioBescheinigung.Signaturen[1]}

	// Missbrauch / Fehlform: kaputter pq:-Eintrag -> 400 vor jeder Pruefung.
	if c, b := post(append(append([]string{}, v3...), "pq:abc")); c != http.StatusBadRequest {
		t.Fatalf("kaputter pq:-Eintrag: %d %s", c, b)
	}
	// Unbekannte Art bleibt abgewiesen.
	if c, b := post(append(append([]string{}, v3...), "v9:abc")); c != http.StatusBadRequest {
		t.Fatalf("unbekannter Eintrag: %d %s", c, b)
	}
	// PQ Pflicht, aber keine pq:-Eintraege: die Kette lehnt ab (nicht 400).
	if c, b := post(v3); c != http.StatusConflict || !strings.Contains(b, "umzug_bescheinigung") {
		t.Fatalf("ohne PQ nach dem Stichtag: %d %s", c, b)
	}
	// PQ-Unterschrift eines fremden Schluessels: abgelehnt.
	fremd := neuerPQ(t)
	falsch := []string{pqGut[0], "pq:" + zeuge.ed.pub + strings.TrimPrefix(fremd.pqUnterschrift(t, aussageText), fremd.ed.pub)}
	if c, b := post(append(append([]string{}, v3...), falsch...)); c != http.StatusConflict {
		t.Fatalf("fremde PQ-Unterschrift: %d %s", c, b)
	}
	// Gutfall: v3 + pq -> angenommen, und die Transaktion traegt die PQ-Teile.
	if c, b := post(append(append([]string{}, v3...), pqGut...)); c != http.StatusOK {
		t.Fatalf("gueltiger Antrag mit pq: abgewiesen: %d %s", c, b)
	}
}

// Die Grenze des Antrags traegt Bindung und sieben Zeugen mit PQ, die der
// anderen Umzugswege bleibt eng.
func TestUmzugAntrag_KoerperGrenze(t *testing.T) {
	eintrag := len(`"pq:"`) + pqEintragLaenge + 1
	if 8*eintrag+4096 > umzugAntragKoerperMax {
		t.Fatalf("acht pq:-Eintraege (%d Byte) passen nicht in umzugAntragKoerperMax", 8*eintrag)
	}
	if umzugAntragKoerperMax > zumLeiterRumpfHoechstens {
		t.Fatal("der Antrag geht ueber die Weiterleitung zum Leiter -- groesser als deren Grenze hilft nicht")
	}
	if umzugKoerperMax >= umzugAntragKoerperMax {
		t.Fatal("die uebrigen Umzugswege brauchen keine PQ-Grenze")
	}
}
