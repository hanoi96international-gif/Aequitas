package main

import (
	"strings"
	"testing"
	"time"
)

func TestAuswerten_Perzentile(t *testing.T) {
	var d []float64
	for i := 1; i <= 100; i++ {
		d = append(d, float64(i))
	}
	e := Auswerten("/x", d, time.Second)
	if e.P50 != 51 || e.P90 != 90 || e.P99 != 99 || e.Hoechst != 100 || e.JeSekunde != 100 {
		t.Fatalf("Perzentile falsch: %+v", e)
	}
}

func TestMessen_EchtePlatte(t *testing.T) {
	e, err := Messen(t.TempDir(), 50, 256)
	if err != nil {
		t.Fatal(err)
	}
	if e.Anzahl != 50 || e.P50 <= 0 || e.Hoechst < e.P99 || e.P99 < e.P50 {
		t.Fatalf("unplausibel: %+v", e)
	}
	if b := Bericht(e, 64); !strings.Contains(b, "Einschaetzung") {
		t.Fatalf("Bericht ohne Einschaetzung: %s", b)
	}
}
