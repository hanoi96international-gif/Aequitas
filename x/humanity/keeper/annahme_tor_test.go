package keeper

import (
	"testing"
)

// Die Sperre selbst: Voreinstellung aendert nichts, nur_lesend lehnt ab.

func TestAnnahmeTor_OhneKonfigurationNimmtAn(t *testing.T) {
	t.Setenv(annahmeRolleEnv, "")
	cs := newTestState()
	cs.nurLesend.Store(annahmeRolleAusUmgebung())
	if !cs.nimmtUeberweisungenAn() {
		t.Fatal("ohne Konfiguration muss ein Knoten annehmen -- ein Tor, das beim Einschalten " +
			"die halbe Kette stillegen koennte, gehoert nicht per Vorgabe scharf")
	}
	if err := cs.pruefeAnnahmeTor(); err != nil {
		t.Fatalf("unerwartet abgelehnt: %v", err)
	}
}

func TestAnnahmeTor_NurLesendLehntAb(t *testing.T) {
	t.Setenv(annahmeRolleEnv, "nur_lesend")
	cs := newTestState()
	cs.nurLesend.Store(annahmeRolleAusUmgebung())
	if cs.nimmtUeberweisungenAn() {
		t.Fatal("nur_lesend muss ablehnen")
	}
	vorher := abgelehnteUeberweisungen.Load()
	err := cs.pruefeAnnahmeTor()
	if err == nil {
		t.Fatal("erwartet eine Ablehnung")
	}
	if abgelehnteUeberweisungen.Load() != vorher+1 {
		t.Fatal("die Ablehnung wurde nicht gezaehlt -- eine Sperre, die niemand zaehlt, " +
			"merkt man erst, wenn sich jemand beschwert")
	}
	// Die Meldung landet bei einem Menschen in der App: sie muss den Ausweg nennen.
	if got := err.Error(); len(got) < 40 || !contains(got, annahmeRolleEnv) {
		t.Fatalf("die Meldung nennt den Grund nicht: %q", got)
	}
}

// Gross- und Kleinschreibung darf nicht ueber die Rolle einer Box entscheiden.
func TestAnnahmeTor_SchreibweiseEgal(t *testing.T) {
	cs := newTestState()
	for _, v := range []string{"nur_lesend", "NUR_LESEND", " Nur_Lesend "} {
		t.Setenv(annahmeRolleEnv, v)
		cs.nurLesend.Store(annahmeRolleAusUmgebung())
		if cs.nimmtUeberweisungenAn() {
			t.Fatalf("%q haette ablehnen muessen", v)
		}
	}
	for _, v := range []string{"annehmend", "", "ja", "irgendwas"} {
		t.Setenv(annahmeRolleEnv, v)
		cs.nurLesend.Store(annahmeRolleAusUmgebung())
		if !cs.nimmtUeberweisungenAn() {
			t.Fatalf("%q haette annehmen muessen -- nur das ausdrueckliche nur_lesend sperrt", v)
		}
	}
}

// Die Sperre muss VOR jeder Zustandsaenderung greifen: eine abgewiesene
// Ueberweisung darf kein Guthaben bewegen und keine Ausgangskorb-Zeile
// hinterlassen.
func TestAnnahmeTor_AbgelehnteUeberweisungAendertNichts(t *testing.T) {
	cs := newTestState()
	cs.SetzeNurLesend(true)
	uhrSetzen(cs, "0xacct-a", 1000, nowUnix())
	uhrSetzen(cs, "0xacct-b", 0, nowUnix())

	vorherA := abzugVon(cs, "0xacct-a")
	vorherB := abzugVon(cs, "0xacct-b")

	_, _, err := cs.TransferAtomic("0xacct-a", "0xacct-b", 100,
		Transaction{Type: "transfer", Wallet: "0xacct-a", To: "0xacct-b", Amount: 100, TxHash: "0xtor"})
	if err == nil {
		t.Fatal("ein nur lesender Knoten hat eine Ueberweisung angenommen")
	}
	if abzugVon(cs, "0xacct-a") != vorherA || abzugVon(cs, "0xacct-b") != vorherB {
		t.Fatal("die abgelehnte Ueberweisung hat Zustand veraendert -- die Sperre greift zu spaet")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
