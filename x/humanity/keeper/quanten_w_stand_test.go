package keeper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// /api/quanten-w/stand: Zahlen gegen das echte Register, Grenze der Liste,
// Zwischenspeicher, fremdes Urteil zaehlt nie als Erfolg, nur GET.

func TestQuantenWStand_EingeloestUndTotGezaehlt(t *testing.T) {
	qwuAktiv(t)
	ein := neueQsWelt(t, 128, 80)
	ein.bereit(qwT0)
	if a := ein.cs.quantenWStandRechnen(context.Background(), time.Unix(qwT0, 0)); a.TokenS != 1 || a.Beendet != 0 || len(a.Zuletzt) != 0 {
		t.Fatalf("lebender Token: %+v", a)
	}
	ein.einloesen(qwT0 + 50)
	a := ein.cs.quantenWStandRechnen(context.Background(), time.Unix(qwT0+100, 0))
	if a.TokenS != 0 || a.Beendet != 1 || a.Eingeloest != 1 || a.Tot != 0 || a.UrteilOffen != 0 {
		t.Fatalf("eingeloest: %+v", a)
	}
	if len(a.Zuletzt) != 1 || a.Zuletzt[0].K != ein.p.K || a.Zuletzt[0].Urteil != quantenWUrteilEingeloest {
		t.Fatalf("zuletzt: %+v", a.Zuletzt)
	}

	tot := neueQsWelt(t, 128, 80)
	tot.bereit(qwT0)
	tot.totMachen(qwT0 + 50)
	if a := tot.cs.quantenWStandRechnen(context.Background(), time.Unix(qwT0+1000, 0)); a.Tot != 1 || a.Eingeloest != 0 {
		t.Fatalf("tot: %+v", a)
	}
}

// Ein Register mit vielen Enden und einem unbekannten Urteil: die Liste ist
// begrenzt und nach Ende sortiert; das fremde Urteil zaehlt weder als
// eingeloest noch erscheint es in der Liste.
func TestQuantenWStand_GrenzeUndFremdesUrteil(t *testing.T) {
	w := neueQsWelt(t, 128, 80)
	w.bereit(qwT0)
	w.cs.mu.Lock()
	reg := w.cs.quantenWJetzt()
	for i := 0; i < 3*quantenWStandZuletzt; i++ {
		k := fmt.Sprintf("%064x", i+1)
		reg.Tote[k] = qwT0
		reg.Enden[k] = quantenWEnde{Ende: qwT0 + int64(i), Urteil: quantenWUrteilEingeloest}
	}
	fremd := fmt.Sprintf("%064x", 999)
	reg.Tote[fremd] = qwT0
	reg.Enden[fremd] = quantenWEnde{Ende: qwT0 + 10_000, Urteil: "echt"}
	ohne := fmt.Sprintf("%064x", 998)
	reg.Tote[ohne] = qwT0
	w.cs.mu.Unlock()

	a := w.cs.quantenWStandRechnen(context.Background(), time.Unix(qwT0, 0))
	if a.Eingeloest != 3*quantenWStandZuletzt || a.UrteilOffen != 2 || a.Beendet != 3*quantenWStandZuletzt+2 {
		t.Fatalf("Zaehlung: %+v", a)
	}
	if len(a.Zuletzt) != quantenWStandZuletzt {
		t.Fatalf("Liste nicht begrenzt: %d", len(a.Zuletzt))
	}
	for i, e := range a.Zuletzt {
		if e.K == fremd || e.K == ohne || e.Urteil != quantenWUrteilEingeloest {
			t.Fatalf("fremdes oder offenes Urteil gezeigt: %+v", e)
		}
		if i > 0 && e.Ende > a.Zuletzt[i-1].Ende {
			t.Fatal("nicht nach Ende sortiert")
		}
	}
}

// Innerhalb der Frische wird nicht neu gerechnet, danach schon; ein anderer
// Zustand bekommt nie die Antwort eines fremden.
func TestQuantenWStand_Zwischenspeicher(t *testing.T) {
	w := neueQsWelt(t, 128, 80)
	w.bereit(qwT0)
	t0 := time.Unix(qwT0, 0)
	a1 := w.cs.quantenWStand(context.Background(), t0)
	if a2 := w.cs.quantenWStand(context.Background(), t0.Add(quantenWStandFrische-time.Second)); a2 != a1 {
		t.Fatal("innerhalb der Frische neu gerechnet")
	}
	if a3 := w.cs.quantenWStand(context.Background(), t0.Add(quantenWStandFrische)); a3 == a1 {
		t.Fatal("nach der Frische nicht neu gerechnet")
	}
	if a4 := w.cs.quantenWStand(context.Background(), t0.Add(-time.Hour)); a4 == nil || a4.Stand != t0.Add(-time.Hour).Unix() {
		t.Fatal("Uhr rueckwaerts: alte Antwort weiter benutzt")
	}
	// Innerhalb der Frische der letzten Antwort (1 Token) fragt ein anderer
	// Zustand ohne Token: er muss seine eigene Zaehlung bekommen.
	anders := neueQsWelt(t, 128, 80)
	if b := anders.cs.quantenWStand(context.Background(), t0.Add(-time.Hour+time.Second)); b.TokenS != 0 {
		t.Fatalf("Antwort eines fremden Zustands: %+v", b)
	}
}

func TestQuantenWStand_HTTP(t *testing.T) {
	w := neueQsWelt(t, 128, 80)
	w.bereit(qwT0)
	a := &APIServer{state: w.cs}
	rec := httptest.NewRecorder()
	a.handleQuantenWStand(rec, httptest.NewRequest(http.MethodPost, "/api/quanten-w/stand", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	a.handleQuantenWStand(rec, httptest.NewRequest(http.MethodGet, "/api/quanten-w/stand", nil))
	var v map[string]interface{}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &v) != nil {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
	}
	for _, f := range []string{"aktiv", "noten_w", "token_s", "beendet", "eingeloest", "tot", "urteil_offen", "topf_mikro", "zuletzt", "stand"} {
		if _, ok := v[f]; !ok {
			t.Fatalf("Feld %s fehlt: %v", f, v)
		}
	}
}
