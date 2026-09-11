package keeper

import "testing"

// Die Schranke wurde am 11.09.2026 von 8.000 auf 20.000 gehoben: das brachte
// 3.454 -> 4.653 Ketten-TPS und senkte die Ablehnungsquote von 79,5 % auf 0,0.
// Fest verdrahten darf man diese 20.000 aber nicht -- was sie wirklich
// begrenzt, ist Speicher (3,6 GB im Knoten bei 15.900 gleichzeitigen Posten).
// Auf der kleinen VM, auf der jemand seinen ersten Validator startet, waere
// das der Kernel-OOM. Diese Tests halten die Ableitung fest.

func TestInflightVorgabe_WaechstMitDemSpeicherlimit(t *testing.T) {
	faelle := []struct {
		limit    string
		erwartet int64
		warum    string
	}{
		{"5GiB", 20000, "die Produktionsboxen: der live gemessene Wert"},
		{"2GiB", 8000, "halb so viel Speicher, halb so grosse Schranke"},
		{"512MiB", 2000, "sehr kleine VM -- die Untergrenze greift"},
		{"64GiB", 40000, "grosse Maschine -- die Obergrenze greift, darueber wurde nie gemessen"},
	}
	for _, f := range faelle {
		t.Run(f.limit, func(t *testing.T) {
			t.Setenv("GOMEMLIMIT", f.limit)
			t.Setenv(inflightGrenzeEnv, "")
			if got := inflightGrenze(); got != f.erwartet {
				t.Errorf("inflightGrenze() = %d bei GOMEMLIMIT=%s, erwartet %d (%s)",
					got, f.limit, f.erwartet, f.warum)
			}
		})
	}
}

// Ohne GOMEMLIMIT ist der Kernel die einzige Bremse, und der bremst durch
// Beenden -- am 05.09. und 07.09.2026 genau so geschehen, beide Male mit
// OOMKilled=false und damit unsichtbar fuer Docker. Wer kein Limit setzt,
// bekommt darum keine groessere Schranke geschenkt.
func TestInflightVorgabe_OhneSpeicherlimitBleibtEsBeimAltenWert(t *testing.T) {
	t.Setenv("GOMEMLIMIT", "")
	t.Setenv(inflightGrenzeEnv, "")
	if got := inflightGrenze(); got != inflightVorgabeOhneL {
		t.Errorf("inflightGrenze() = %d ohne GOMEMLIMIT, erwartet %d", got, inflightVorgabeOhneL)
	}
}

// Ein Tippfehler in einer Umgebungsvariablen darf einen Schutz niemals
// entschaerfen -- dieselbe Regel, die evm_rpc.go fuer den Ratenbegrenzer
// aufstellt. Auch nicht ueber den Umweg eines unlesbaren GOMEMLIMIT.
func TestInflightVorgabe_UnsinnEntschaerftNicht(t *testing.T) {
	for _, roh := range []string{"viel", "-5GiB", "0", "5 Gigabyte", "GiB"} {
		t.Run(roh, func(t *testing.T) {
			t.Setenv("GOMEMLIMIT", roh)
			t.Setenv(inflightGrenzeEnv, "")
			if got := inflightGrenze(); got != inflightVorgabeOhneL {
				t.Errorf("inflightGrenze() = %d bei GOMEMLIMIT=%q, erwartet den vorsichtigen "+
					"Wert %d -- ein unlesbares Limit darf nicht in eine groessere Schranke "+
					"uebersetzt werden", got, roh, inflightVorgabeOhneL)
			}
		})
	}
	// Und die ausdrueckliche Abschaltung muss weiter moeglich bleiben.
	t.Setenv("GOMEMLIMIT", "5GiB")
	t.Setenv(inflightGrenzeEnv, "0")
	if got := inflightGrenze(); got != 0 {
		t.Errorf("inflightGrenze() = %d bei ausdruecklicher 0, erwartet 0 (bewusstes Abschalten)", got)
	}
	// Ein unbrauchbarer ausdruecklicher Wert dagegen faellt auf die Vorgabe.
	t.Setenv(inflightGrenzeEnv, "achtzehntausend")
	if got := inflightGrenze(); got != 20000 {
		t.Errorf("inflightGrenze() = %d bei unbrauchbarem Wert, erwartet die speicherabhaengige Vorgabe 20000", got)
	}
}
