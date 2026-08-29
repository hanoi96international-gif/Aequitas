package keeper

import (
	"encoding/json"
	"testing"
)

// eth_estimateGas gab fuer JEDE Anfrage 0x5B8D80 (6.000.000) zurueck. Eine
// Wallet uebernimmt diese Antwort als Gaslimit -- fuer eine Ueberweisung, die
// exakt 21.000 verbraucht, das 286-fache. Der Name der Methode ist
// "schaetze"; eine Konstante schaetzt nichts.
func TestEstimateGas_EinfacheUeberweisungIstKeineSchaetzung(t *testing.T) {
	s := &EVMRPCServer{}
	faelle := []struct {
		name     string
		aufruf   string
		erwartet string
	}{
		{"reine Ueberweisung", `{"from":"0xaa","to":"0xbb","value":"0x1"}`, "0x5208"},
		{"Ueberweisung mit leerem data", `{"to":"0xbb","data":"0x"}`, "0x5208"},
		{"Vertragsaufruf hat Daten", `{"to":"0xbb","data":"0xa9059cbb0000"}`, "0x5B8D80"},
		{"Vertragsaufruf ueber input", `{"to":"0xbb","input":"0xa9059cbb0000"}`, "0x5B8D80"},
		{"Erzeugung hat kein to", `{"data":"0x6080604052"}`, "0x5B8D80"},
	}
	for _, f := range faelle {
		t.Run(f.name, func(t *testing.T) {
			got, rpcErr := s.estimateGas([]json.RawMessage{json.RawMessage(f.aufruf)})
			if rpcErr != nil {
				t.Fatalf("unerwarteter Fehler: %v", rpcErr)
			}
			if got != f.erwartet {
				t.Fatalf("estimateGas(%s) = %v, erwartet %s -- %s", f.aufruf, got, f.erwartet,
					"eine Wallet setzt diesen Wert als Gaslimit ein")
			}
		})
	}
}

func TestEstimateGas_OhneParameterBleibtGrosszuegig(t *testing.T) {
	// Zu niedrig geschaetzt scheitert ein Aufruf mitten in der Ausfuehrung.
	// Wenn nichts bekannt ist, ist grosszuegig die sichere Richtung.
	s := &EVMRPCServer{}
	got, rpcErr := s.estimateGas(nil)
	if rpcErr != nil || got != "0x5B8D80" {
		t.Fatalf("ohne Parameter = %v (%v), erwartet 0x5B8D80", got, rpcErr)
	}
}

// oldestBlock stand fest auf "0x0" -- egal welcher Bereich angefragt wurde.
// Wer daraus eine Gebuehrenkurve baut, bezieht sie auf Block 0 statt auf die
// Gegenwart.
func TestFeeHistory_FormUndBereich(t *testing.T) {
	s := &EVMRPCServer{} // ohne DAG: hoechste Hoehe bleibt 0
	roh, rpcErr := s.feeHistory([]json.RawMessage{json.RawMessage(`"0x5"`)})
	if rpcErr != nil {
		t.Fatalf("unerwarteter Fehler: %v", rpcErr)
	}
	m, ok := roh.(map[string]interface{})
	if !ok {
		t.Fatalf("kein Objekt: %T", roh)
	}

	basis, _ := m["baseFeePerGas"].([]string)
	anteil, _ := m["gasUsedRatio"].([]float64)
	lohn, _ := m["reward"].([][]string)

	// Die Spezifikation verlangt EINEN baseFeePerGas mehr als die anderen
	// Reihen: den Wert fuer den naechsten, noch nicht erzeugten Block. Vorher
	// stand dort genau ein Eintrag, egal wie viele Bloecke gefragt waren.
	if len(anteil) != 5 || len(lohn) != 5 {
		t.Fatalf("fuer 5 angefragte Bloecke: gasUsedRatio=%d reward=%d, erwartet je 5",
			len(anteil), len(lohn))
	}
	if len(basis) != len(anteil)+1 {
		t.Fatalf("baseFeePerGas=%d, erwartet %d (eine mehr als gasUsedRatio -- der naechste Block)",
			len(basis), len(anteil)+1)
	}
	if _, da := m["oldestBlock"]; !da {
		t.Fatal("oldestBlock fehlt")
	}
	// Die Gebuehren selbst bleiben null: diese Kette erhebt keine. Das ist
	// keine Platzhalterangabe und darf nicht "repariert" werden.
	for i, b := range basis {
		if b != "0x0" {
			t.Fatalf("baseFeePerGas[%d]=%s -- die Kette ist gebuehrenfrei, hier gehoert 0x0 hin", i, b)
		}
	}
}

func TestFeeHistory_ObergrenzeGreift(t *testing.T) {
	// Ohne Deckel koennte ein einzelner Aufruf beliebig viel Speicher
	// anfordern -- go-ethereum zieht dieselbe Grenze bei 1024.
	s := &EVMRPCServer{}
	roh, _ := s.feeHistory([]json.RawMessage{json.RawMessage(`"0xffffff"`)})
	m := roh.(map[string]interface{})
	if n := len(m["gasUsedRatio"].([]float64)); n != 1024 {
		t.Fatalf("gasUsedRatio=%d, erwartet die Obergrenze 1024", n)
	}
}
