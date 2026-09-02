package keeper

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Die Zusicherung: eine Statusabfrage wartet NIE auf die DAG-Sperre.
//
// Genau das ist am 02.09.2026 schiefgegangen -- der Knoten lief, haengte
// Bloecke an, hatte Tips 1, und antwortete trotzdem nicht, weil der
// Block-Burst die Schreibsperre hielt. Von aussen sah das wie ein Absturz aus.
func TestStatus_AntwortetAuchBeiGehaltenerSperre(t *testing.T) {
	dag := &BlockDAG{blocks: map[string]*Block{}, tips: map[string]bool{}}
	dag.setHeight(4711)

	// Schreibsperre halten, wie ein Block-Burst es tut.
	dag.mu.Lock()
	defer dag.mu.Unlock()

	fertig := make(chan struct{})
	go func() {
		defer close(fertig)
		if _, frei := dag.TryLatestBlock(); frei {
			t.Error("TryLatestBlock meldet frei, obwohl die Schreibsperre gehalten wird")
		}
		if h := dag.HeightSchnell(); h != 4711 {
			t.Errorf("HeightSchnell = %d, erwartet 4711 -- die Hoehe muss OHNE Sperre lesbar sein", h)
		}
	}()

	select {
	case <-fertig:
	case <-time.After(2 * time.Second):
		t.Fatal("die Statusabfrage hat sich an der gehaltenen Sperre angestellt -- genau das " +
			"laesst den Knoten von aussen tot wirken, waehrend er in Wahrheit arbeitet")
	}
}

// Der Zwischenspeicher muss den letzten guten Stand liefern und dabei die
// AKTUELLE Hoehe einsetzen -- an ihr liest man ab, ob der Knoten vorankommt.
func TestStatus_ZwischenspeicherLiefertAktuelleHoehe(t *testing.T) {
	statusZwischenspeicher = letzterGuterStatus{}
	if _, da := statusZwischenspeicher.holen(1); da {
		t.Fatal("ohne je einen Stand gemerkt zu haben darf nichts geliefert werden")
	}
	statusZwischenspeicher.merken(map[string]interface{}{"height": int64(100), "total_humans": 18})
	m, da := statusZwischenspeicher.holen(12345)
	if !da {
		t.Fatal("gemerkter Stand wird nicht geliefert")
	}
	if m["height"] != int64(12345) {
		t.Fatalf("height = %v, erwartet die AKTUELLE 12345 statt der gemerkten 100", m["height"])
	}
	if m["total_humans"] != 18 {
		t.Fatalf("die uebrigen Felder muessen aus dem gemerkten Stand kommen, sind aber %v", m["total_humans"])
	}
	if m["stand_veraltet"] != true {
		t.Fatal("eine veraltete Antwort MUSS als solche gekennzeichnet sein -- sonst " +
			"haelt der Leser alte Zahlen fuer aktuelle")
	}
}

// dag.height darf nur ueber setHeight geschrieben werden, sonst laeuft der
// sperrfreie Spiegel aus dem Takt und /api/status meldet eine falsche Hoehe.
func TestDagHeight_NurUeberSetHeight(t *testing.T) {
	roh, err := os.ReadFile("block.go")
	if err != nil {
		t.Fatalf("block.go nicht lesbar: %v", err)
	}
	muster := regexp.MustCompile(`(?m)^\s*dag\.height\s*=`)
	var funde []string
	for _, zeile := range strings.Split(string(roh), "\n") {
		// Die eine Zeile IM Setter ist die Ausnahme -- sie ist der Ort, an
		// dem geschrieben werden soll.
		if muster.MatchString(zeile) && !strings.Contains(zeile, "setHeight") &&
			strings.TrimSpace(zeile) != "dag.height = h" {
			funde = append(funde, strings.TrimSpace(zeile))
		}
	}
	if len(funde) > 0 {
		t.Fatalf("dag.height wird direkt zugewiesen statt ueber setHeight:\n  %s\n\n"+
			"setHeight haelt den sperrfreien Spiegel heightSchnell mit, aus dem "+
			"/api/status liest, wenn die DAG-Sperre belegt ist. Eine direkte Zuweisung "+
			"laesst den Spiegel zurueckfallen, und die Statusanzeige behauptet dann eine "+
			"Hoehe, die der Knoten laengst hinter sich hat", strings.Join(funde, "\n  "))
	}
}
