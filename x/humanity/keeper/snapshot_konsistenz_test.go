package keeper

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// ExportSnapshot MUSS die exklusive Sperre nehmen, nicht die Lesesperre.
//
// # WARUM DAS SUBTIL IST UND EINEN WAECHTER BRAUCHT
//
// Die Lesesperre sieht hier voellig richtig aus: ein Snapshot liest ja nur.
// Genau deshalb stand sie jahrelang da, und genau deshalb kann sie jemand
// wieder einsetzen, um "unnoetiges Blockieren" zu vermeiden.
//
// Sie ist trotzdem falsch. Der WAL-Schnellpfad (transfer_wal.go) haelt fuer
// seine Dauer EBENFALLS nur cs.mu.RLock() und verlaesst sich zur
// Ausschliessung auf die Konten-Shards. Zwei Lesesperren schliessen einander
// nicht aus -- der Snapshot laeuft also mitten durch laufende Ueberweisungen
// und liefert manche Konten vor, manche nach derselben Ueberweisung.
//
// # WAS DAS GEKOSTET HAT
//
// Live am 02.09.2026: ein Knoten resynct, meldet Erfolg, und scheitert
// unmittelbar am naechsten Block, weil ein Konto im Snapshot 0 hatte, obwohl
// der Block daraus ueberweist. Er weist den Block ab, kommt nie darueber
// hinaus, sammelt Waisen, resynct erneut -- in denselben kaputten Zustand.
// Eine Endlosschleife, die den Primary tagelang instabil gemacht hat und wie
// ein Absturz aussah.
func TestExportSnapshot_NimmtDieExklusiveSperre(t *testing.T) {
	roh, err := os.ReadFile("snapshot.go")
	if err != nil {
		t.Fatalf("snapshot.go nicht lesbar: %v", err)
	}
	inhalt := strings.ReplaceAll(string(roh), "\r\n", "\n")

	start := strings.Index(inhalt, "func (cs *ChainState) ExportSnapshot")
	if start < 0 {
		t.Fatal("ExportSnapshot nicht gefunden -- wenn es umbenannt wurde, diesen Test " +
			"umhaengen statt loeschen")
	}
	ende := strings.Index(inhalt[start:], "\n}\n")
	if ende < 0 {
		ende = len(inhalt) - start
	}
	koerper := inhalt[start : start+ende]

	// Nur ausgefuehrte Zeilen zaehlen, nicht die Erklaerung darueber.
	code := regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(koerper, "")

	if strings.Contains(code, "cs.mu.RLock()") {
		t.Error("ExportSnapshot nimmt cs.mu.RLock().\n" +
			"  Das sieht richtig aus -- ein Snapshot liest ja nur -- ist es aber nicht:\n" +
			"  der WAL-Schnellpfad haelt fuer seine Dauer EBENFALLS nur RLock und schliesst\n" +
			"  ueber die Konten-Shards aus. Zwei Lesesperren schliessen einander nicht aus,\n" +
			"  der Snapshot laeuft also mitten durch laufende Ueberweisungen und liefert\n" +
			"  einen zerrissenen Zustand.\n" +
			"  Am 02.09.2026 hat genau das den Primary in eine Endlosschleife gebracht:\n" +
			"  resync -> naechster Block wegen 'insufficient balance' abgewiesen -> Waisen\n" +
			"  -> resync. Bitte cs.mu.Lock() lassen.")
	}
	if !strings.Contains(code, "cs.mu.Lock()") {
		t.Error("ExportSnapshot nimmt cs.mu ueberhaupt nicht exklusiv -- dann ist der " +
			"Snapshot kein Zustand zu einem Zeitpunkt, sondern eine Sammlung von Momenten")
	}
}
