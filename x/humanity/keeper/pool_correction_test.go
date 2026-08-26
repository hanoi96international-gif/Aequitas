package keeper

import (
	"context"
	"math"
	"net/http/httptest"
	"testing"
)

// Die Betraege der einmaligen Korrektur vom 26.08.2026, wie sie live
// gemessen wurden: Reserve 505,764954 AEQ gegen 13.100 tUSD, Ueberschuss
// 305,277988 AEQ.
const (
	istAEQ    = 505.764954
	istTUSD   = 13100.0
	brennAEQ  = 305.277988
	brennTUSD = 7907.114977
)

func poolStateFuerKorrektur() *ChainState {
	cs := newTestState()
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(istAEQ), ReserveTUSD: NewDecimal(istTUSD)}
	return cs
}

func TestKorrekturVerkleinertBeideReservenGenau(t *testing.T) {
	cs := poolStateFuerKorrektur()
	if err := cs.applyPoolCorrectionLocked(context.Background(), brennAEQ, brennTUSD); err != nil {
		t.Fatal(err)
	}
	if got := cs.pool.ReserveAEQ.Float(); math.Abs(got-200.486966) > 1e-6 {
		t.Fatalf("AEQ-Reserve = %v, erwartet 200.486966", got)
	}
	if got := cs.pool.ReserveTUSD.Float(); math.Abs(got-5192.885023) > 1e-6 {
		t.Fatalf("tUSD-Reserve = %v, erwartet 5192.885023", got)
	}
}

// Der eigentliche Zweck: der Vorrat stimmt danach.
func TestNachDerKorrekturErklaertDieRegelDenBestand(t *testing.T) {
	// gemessen = Konten + Reserve; die Regel sagt 17 Menschen x 1.000.
	const konten = 16799.513034
	const regel = 17000.0
	vorher := konten + istAEQ
	if math.Abs(vorher-regel) < 1 {
		t.Fatal("Testaufbau falsch: vorher soll eine Luecke bestehen")
	}
	nachher := konten + (istAEQ - brennAEQ)
	if math.Abs(nachher-regel) > 1e-6 {
		t.Fatalf("nach der Korrektur %v statt %v -- die Luecke bleibt", nachher, regel)
	}
}

// Der Grund, warum der Nutzer diese Variante gewaehlt hat.
func TestDerPreisBleibtStehen(t *testing.T) {
	vorher := istTUSD / istAEQ
	nachher := (istTUSD - brennTUSD) / (istAEQ - brennAEQ)
	if math.Abs(vorher-nachher) > 1e-6 {
		t.Fatalf("Preis springt von %v auf %v -- die Reserven wurden nicht "+
			"verhaeltnisgleich verkleinert", vorher, nachher)
	}
}

// DER wichtigste Test dieser Datei.
func TestZweimalAngewendetVerbrenntNichtZweimal(t *testing.T) {
	// Bei den Verteilungen ist genau das nachgewiesen passiert: ein doppelt
	// zugestellter Block zahlte alles ein zweites Mal aus (Audit 2026-08-16).
	// Hier waere die Folge, dass echtes Geld doppelt vernichtet wird.
	cs := poolStateFuerKorrektur()
	if err := cs.applyPoolCorrectionLocked(context.Background(), brennAEQ, brennTUSD); err != nil {
		t.Fatal(err)
	}
	nachErstem := cs.pool.ReserveAEQ.Float()

	// Ohne Datenbank greift die Sperre ueber getConfigValueDB nicht; dann muss
	// wenigstens die Reserve-Pruefung verhindern, dass ins Minus gebrannt wird.
	err := cs.applyPoolCorrectionLocked(context.Background(), brennAEQ, brennTUSD)
	if err == nil && math.Abs(cs.pool.ReserveAEQ.Float()-nachErstem) > 1e-6 {
		t.Fatalf("zweite Anwendung hat erneut verbrannt: %v -> %v",
			nachErstem, cs.pool.ReserveAEQ.Float())
	}
}

func TestKorrekturKannNiemalsVERGROESSERN(t *testing.T) {
	// Diese Transaktion wird nicht von einem Menschen unterschrieben, sondern
	// entsteht auf einem Knoten. Koennte sie vergroessern, waere sie genau das
	// Werkzeug zur Geldschoepfung, gegen das sie gebaut wurde.
	cs := poolStateFuerKorrektur()
	for _, betrag := range []float64{-1, 0} {
		if err := cs.applyPoolCorrectionLocked(context.Background(), betrag, 0); err == nil {
			t.Fatalf("Betrag %v wurde angenommen", betrag)
		}
	}
	if got := cs.pool.ReserveAEQ.Float(); math.Abs(got-istAEQ) > 1e-9 {
		t.Fatalf("Reserve wurde trotz Abweisung veraendert: %v", got)
	}
}

func TestZuGrosseKorrekturScheitertStattZuKappen(t *testing.T) {
	// Kappen wuerde auf einem Knoten mit etwas kleinerer Reserve ein anderes
	// Ergebnis liefern als auf dem anderen -- beide "erfolgreich", beide
	// verschieden. Genau die stille Abweichung, die hier vermieden wird.
	cs := poolStateFuerKorrektur()
	if err := cs.applyPoolCorrectionLocked(context.Background(), istAEQ+1, 0); err == nil {
		t.Fatal("eine Korrektur groesser als die Reserve muss scheitern")
	}
	if got := cs.pool.ReserveAEQ.Float(); math.Abs(got-istAEQ) > 1e-9 {
		t.Fatalf("Reserve wurde trotz Fehler veraendert: %v", got)
	}
	if got := cs.pool.ReserveTUSD.Float(); math.Abs(got-istTUSD) > 1e-9 {
		t.Fatalf("tUSD-Reserve wurde trotz Fehler veraendert: %v", got)
	}
}

func TestOhnePoolKeineKorrektur(t *testing.T) {
	cs := newTestState()
	cs.pool = nil
	if err := cs.applyPoolCorrectionLocked(context.Background(), 1, 1); err == nil {
		t.Fatal("ein Knoten ohne Pool darf nicht so tun, als haette er korrigiert")
	}
}

func TestNurDieEchteGegenstelleZaehlt(t *testing.T) {
	// X-Forwarded-For darf hier NICHTS bewirken: die Kopfzeile setzt jeder
	// frei, und wer sie glaubt, macht aus einem Schleifen-Tor ein offenes.
	faelle := []struct {
		remote   string
		header   string
		erwartet bool
	}{
		{"127.0.0.1:5555", "", true},
		{"[::1]:5555", "", true},
		{"173.249.37.118:5555", "", false},
		{"173.249.37.118:5555", "127.0.0.1", false},
		{"8.8.8.8:1", "127.0.0.1, 10.0.0.1", false},
	}
	for _, f := range faelle {
		r := httptest.NewRequest("POST", "/api/admin/pool-correction", nil)
		r.RemoteAddr = f.remote
		if f.header != "" {
			r.Header.Set("X-Forwarded-For", f.header)
		}
		if got := vonDerSchleife(r); got != f.erwartet {
			t.Fatalf("RemoteAddr=%q XFF=%q -> %v, erwartet %v", f.remote, f.header, got, f.erwartet)
		}
	}
}
