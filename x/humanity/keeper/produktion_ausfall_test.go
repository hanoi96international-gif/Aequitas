package keeper

import (
	"os"
	"strings"
	"testing"
)

// Ein Instrument, das nur die erwarteten Ergebnisse beschreiben kann, ist
// keines. Diese Tests pruefen genau die Eigenschaft, auf die sich die naechste
// Entscheidung stuetzt: dass jeder der elf Abbruchgruende in ProduceBlock
// einen eigenen Zaehler bekommt und keiner still bleibt.

func TestProduktionsAusfall_ZaehltJedenGrundEinzeln(t *testing.T) {
	ProduktionsAusfaelleZuruecksetzen()
	t.Cleanup(ProduktionsAusfaelleZuruecksetzen)

	merkeProduktionsAusfall("sync_tor_saubere_zyklen")
	merkeProduktionsAusfall("sync_tor_saubere_zyklen")
	merkeProduktionsAusfall("ahne_wird_geholt")

	s := ProduktionsAusfaelle()
	if got := s["ausfaelle"].(int64); got != 3 {
		t.Fatalf("ausfaelle = %d, erwartet 3", got)
	}
	if got := s["letzter_grund"].(string); got != "ahne_wird_geholt" {
		t.Errorf("letzter_grund = %q, erwartet ahne_wird_geholt", got)
	}
	// Der teuerste Grund muss oben stehen -- sonst muss man ihn suchen, und
	// genau das Suchen im Log war der Anlass fuer dieses Instrument.
	liste, ok := s["nach_grund"].([]AusfallGrund)
	if !ok {
		t.Fatalf("nach_grund hat unerwarteten Typ %T", s["nach_grund"])
	}
	if len(liste) != 2 {
		t.Fatalf("nach_grund hat %d Eintraege, erwartet 2", len(liste))
	}
	if liste[0].Grund != "sync_tor_saubere_zyklen" || liste[0].Ticks != 2 {
		t.Errorf("der haeufigste Grund steht nicht oben: %+v", liste[0])
	}
}

func TestProduktionsAusfall_ErfolgsanteilIstVersucheZuErfolgen(t *testing.T) {
	ProduktionsAusfaelleZuruecksetzen()
	t.Cleanup(ProduktionsAusfaelleZuruecksetzen)

	for i := 0; i < 10; i++ {
		merkeProduktionsVersuch()
	}
	for i := 0; i < 4; i++ {
		merkeProduktionsErfolg()
	}
	s := ProduktionsAusfaelle()
	if got := s["erfolgsanteil"].(float64); got != 40 {
		t.Errorf("erfolgsanteil = %v, erwartet 40", got)
	}
}

func TestProduktionsAusfall_LeerTeiltNichtDurchNull(t *testing.T) {
	ProduktionsAusfaelleZuruecksetzen()
	t.Cleanup(ProduktionsAusfaelleZuruecksetzen)

	s := ProduktionsAusfaelle() // darf nicht panicken
	if got := s["erfolgsanteil"].(float64); got != 0 {
		t.Errorf("erfolgsanteil = %v auf leeren Zaehlern, erwartet 0", got)
	}
}

// DER EIGENTLICHE TEST. Ein neuer Abbruchgrund in ProduceBlock, der keinen
// Zaehler bekommt, ist genau die Luecke, wegen der es dieses Instrument gibt:
// der Tick faellt aus, das Log schreibt eine Zeile, die unter Last untergeht,
// und die Zaehler summieren sich nicht mehr zu dem, was fehlt.
func TestProduktionsAusfall_JedesNilInProduceBlockWirdGezaehlt(t *testing.T) {
	b, err := os.ReadFile("block.go")
	if err != nil {
		t.Fatalf("block.go nicht lesbar: %v", err)
	}
	zeilen := strings.Split(string(b), "\n")

	start := -1
	for i, z := range zeilen {
		if strings.HasPrefix(z, "func (dag *BlockDAG) ProduceBlock()") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatal("ProduceBlock nicht gefunden -- wenn es umbenannt wurde, diesen Test mitziehen")
	}
	ende := len(zeilen)
	for i := start + 1; i < len(zeilen); i++ {
		if zeilen[i] == "}" {
			ende = i
			break
		}
	}

	var ungezaehlt []int
	for i := start; i < ende; i++ {
		z := strings.TrimSpace(zeilen[i])
		if !strings.HasPrefix(z, "return nil") {
			continue
		}
		// Der Zaehler steht unmittelbar davor -- so wird er verdrahtet, und so
		// bleibt er beim Lesen an seinem Zweig sichtbar.
		if i > 0 && strings.Contains(zeilen[i-1], "merkeProduktionsAusfall(") {
			continue
		}
		ungezaehlt = append(ungezaehlt, i+1)
	}
	if len(ungezaehlt) > 0 {
		t.Errorf("%d Abbruchstelle(n) in ProduceBlock ohne Zaehler, Zeile(n) %v.\n"+
			"  Jede davon verwirft einen Tick, ohne dass irgendwo ablesbar waere, warum.\n"+
			"  Am 11.09.2026 fiel unter Last jeder zweite Tick aus und der Grund war aus\n"+
			"  dem Log nicht zu holen: tausende Zeilen laufen durch, eine 'skipping block\n"+
			"  production' geht darin unter. Ein merkeProduktionsAusfall(\"kurzer_grund\")\n"+
			"  direkt ueber dem return nil genuegt.", len(ungezaehlt), ungezaehlt)
	}
}

// EIN GLEICHZEITIGER NEUSTART DARF DAS NETZ NICHT EINFRIEREN.
//
// Am 12.09.2026 wurden beide Validatoren in derselben Sekunde neu gestartet.
// producedHeights war danach in beiden Prozessen leer, jeder eigene Block des
// Vorgaengers sah wie das Werk einer fremden Instanz aus, und BEIDE
// verweigerten die Produktion -- fuer immer, denn der Totmannschalter feuert
// nur, wenn ein Peer voraus ist. Die Uebernahme der eigenen Bloecke bis zur
// aktuellen Hoehe muss VOR der Zweite-Instanz-Pruefung stehen.
func TestProduceBlock_UebernimmtEigeneBloeckeDesVorgaengersVorDerPruefung(t *testing.T) {
	b, err := os.ReadFile("block.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	start := strings.Index(body, "func (dag *BlockDAG) ProduceBlock() *Block {")
	rumpf := body[start:]
	uebernahme := strings.Index(rumpf, "dag.uebernimmEigeneBloeckeDesVorgaengers(proposer)")
	pruefung := strings.Index(rumpf, "dag.state.HasBlockFromProposerAtHeight(proposer, maxParentHeight+1)")
	if uebernahme < 0 {
		t.Fatal("die Uebernahme der eigenen Bloecke des Vorgaengers ist weg -- ein gleichzeitiger " +
			"Neustart beider Validatoren friert dann das Netz ein (12.09.2026: 343 und 361 Ticks " +
			"'zweite_instanz_gleiche_hoehe' auf beiden Knoten, Hoehe stand)")
	}
	if pruefung < 0 || uebernahme > pruefung {
		t.Error("die Uebernahme steht hinter der Zweite-Instanz-Pruefung und kann sie nicht mehr entschaerfen")
	}
	// Und sie darf nur bis zur JETZIGEN Hoehe uebernehmen: was darueber
	// auftaucht, hat dieser Prozess nicht geschrieben.
	if !strings.Contains(body, "AND timestamp < $2") {
		t.Error("die Uebernahme ist nicht mehr auf Bloecke von VOR dem eigenen Start begrenzt -- damit wuerde auch " +
			"der Block einer wirklich zweiten Instanz als eigener gelten")
	}
}
