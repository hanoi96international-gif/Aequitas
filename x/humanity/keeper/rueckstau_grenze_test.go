package keeper

import (
	"strings"
	"testing"
	"time"
)

func TestRueckstau_GrenzeLehntAbUndGibtFrei(t *testing.T) {
	t.Setenv("AEQUITAS_RUECKSTAU_MAX", "1000")
	alt := rueckstauMesserAn.Load()
	rueckstauMesserAn.Store(true)
	t.Cleanup(func() { rueckstauMesserAn.Store(alt); rueckstauAktuell.Store(0) })
	rueckstauGemessen.Store(time.Now().Unix())

	rueckstauAktuell.Store(999)
	if g := rueckstauGrund(); g != "" {
		t.Fatalf("unter der Grenze abgelehnt: %s", g)
	}
	rueckstauAktuell.Store(1001)
	g := rueckstauGrund()
	if !strings.Contains(g, "server busy") {
		t.Fatalf("ueber der Grenze angenommen: %q", g)
	}
	if r := admissionRefusalReason(); r == "" {
		t.Fatal("admissionRefusalReason laesst den Rueckstau durch")
	}
	// Veraltete Messung sperrt nicht (Messer haengt).
	rueckstauGemessen.Store(time.Now().Unix() - 60)
	if g := rueckstauGrund(); g != "" {
		t.Fatalf("veraltete Messung sperrt: %s", g)
	}
	// Vorgabe: zwei volle Bloecke; 0 schaltet ab.
	t.Setenv("AEQUITAS_RUECKSTAU_MAX", "")
	if rueckstauMax() != 2*int64(blockTxHartDeckel()) {
		t.Fatalf("Vorgabe %d", rueckstauMax())
	}
	t.Setenv("AEQUITAS_RUECKSTAU_MAX", "0")
	rueckstauGemessen.Store(time.Now().Unix())
	if g := rueckstauGrund(); g != "" {
		t.Fatalf("abgeschaltet und trotzdem abgelehnt: %s", g)
	}
}
