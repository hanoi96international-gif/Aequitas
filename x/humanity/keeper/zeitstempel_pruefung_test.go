package keeper

import "testing"

func TestZeitstempel_Zukunft(t *testing.T) {
	jetzt := zeitstempelPruefungAbUnix + 1000
	if g := zeitstempelZukunft(jetzt+zeitstempelZukunftToleranz, jetzt); g != "" {
		t.Fatalf("Toleranz muss reichen: %s", g)
	}
	if zeitstempelZukunft(jetzt+zeitstempelZukunftToleranz+1, jetzt) == "" {
		t.Fatal("ein Block aus der Zukunft muss abgewiesen werden")
	}
}

// Der Angriff aus K-3: ein Block wird vor eine Aktivierung datiert, um deren
// Pruefung abzuschalten. Nach dem Stichtag ist das nicht mehr moeglich.
func TestZeitstempel_RueckdatierungUeberAktivierungAbgewiesen(t *testing.T) {
	eltern := zeitstempelPruefungAbUnix + 3600
	if zeitstempelRueckdatiert(signierteUeberweisungenAbUnixFuerTest()-1, eltern) == "" {
		t.Fatal("Rueckdatierung vor eine Aktivierung muss abgewiesen werden")
	}
	if zeitstempelRueckdatiert(eltern-zeitstempelRueckToleranz-1, eltern) == "" {
		t.Fatal("mehr als die Toleranz zurueck muss abgewiesen werden")
	}
	if g := zeitstempelRueckdatiert(eltern-zeitstempelRueckToleranz, eltern); g != "" {
		t.Fatalf("Uhrabweichung innerhalb der Toleranz muss durchgehen: %s", g)
	}
	if g := zeitstempelRueckdatiert(eltern+5, eltern); g != "" {
		t.Fatalf("ein juengerer Block ist der Normalfall: %s", g)
	}
}

// Die Geschichte vor dem Stichtag bleibt nachspielbar.
func TestZeitstempel_AlteGeschichteUnberuehrt(t *testing.T) {
	eltern := zeitstempelPruefungAbUnix - 1
	if g := zeitstempelRueckdatiert(eltern-86400, eltern); g != "" {
		t.Fatalf("vor dem Stichtag darf die Regel nicht greifen: %s", g)
	}
}

// Irgendeine Aktivierung vor dem Stichtag; der Wert selbst ist egal, er muss
// nur weit vor dem Elternblock liegen.
func signierteUeberweisungenAbUnixFuerTest() int64 { return zeitstempelPruefungAbUnix - 30*86400 }
