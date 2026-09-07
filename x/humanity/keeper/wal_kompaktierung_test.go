package keeper

import "testing"

// Die Zwei-Runden-Regel ist die ganze Sicherheit dieser Funktion: ein Record
// wird ERST geschrieben und DANN eingereiht, also gibt es ein Fenster, in dem
// er in keiner Warteschlange steht und trotzdem gebraucht wird. Wer hier zu
// frueh kuerzt, verliert genau die Records, die ein Absturz braeuchte.

func TestWALKompaktierung_OhneWALPassiertNichts(t *testing.T) {
	cs := &ChainState{} // cs.wal ist nil
	if got := cs.walKompaktierungsRunde(42); got != 0 {
		t.Errorf("Runde ohne WAL gab %d zurueck, erwartet 0", got)
	}
	// starteWALKompaktierung darf ebenfalls nicht panicken.
	cs.starteWALKompaktierung()
}

func TestWALKompaktierungsStand_LeerIstNull(t *testing.T) {
	s := WALKompaktierungsStand()
	for _, k := range []string{"laeufe", "freigegeben_mb", "fehler"} {
		if got, ok := s[k].(int64); !ok || got < 0 {
			t.Errorf("%s = %v, erwartet eine nicht negative Zahl", k, s[k])
		}
	}
	if got := s["intervall_minuten"].(int64); got != 5 {
		t.Errorf("intervall_minuten = %d, erwartet 5", got)
	}
	// Die Groessenschwelle muss gross genug sein, dass eine kleine Datei nicht
	// dauernd neu geschrieben wird -- das kostet mehr, als es spart.
	if got := s["ab_groesse_mb"].(int64); got < 128 {
		t.Errorf("ab_groesse_mb = %d, das ist zu klein fuer eine Neuschreibung je Runde", got)
	}
}
