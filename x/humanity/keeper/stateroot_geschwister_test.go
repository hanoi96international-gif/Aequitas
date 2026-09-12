package keeper

import (
	"os"
	"strings"
	"testing"
)

// DER FEHLALARM, DER DIE PRODUKTION ANHIELT.
//
// Zwei Validatoren, die an derselben Hoehe produzieren, ergeben IMMER
// verschiedene StateRoots: collectUnreplayedAncestors laeuft nur den eigenen
// Elternpfad, die Transaktionen des Geschwisterblocks bleiben im angesammelten
// Zustand des pruefenden Knotens stehen. Der Code weiss das -- der Kommentar an
// der Zaehlstelle sagt "will ALWAYS produce different StateRoots" -- und
// behandelt es korrekt als Warnung statt als Ablehnung.
//
// Trotzdem benutzte autoheal.go genau diesen Zaehler als Divergenzbeweis. Am
// 11.09.2026 unter Last: 917 Abweichungen in zehn Minuten, ein ausgeloester
// Resync, 41 Sekunden ohne Blockproduktion -- waehrend beide Knoten
// nachweislich gleich waren (identische Hoehe, identische Geldmenge, an Hoehe
// 6273700 byte-identischer Blockhash UND StateRoot). Im Leerlauf faellt es
// nicht auf: leere Bloecke ergeben denselben Root. Der Fehlalarm entsteht
// genau dann, wenn Last anliegt.

func quelleLesen(t *testing.T, datei string) string {
	t.Helper()
	b, err := os.ReadFile(datei)
	if err != nil {
		t.Fatalf("%s nicht lesbar: %v", datei, err)
	}
	return string(b)
}

func TestStateRoot_SelbstheilungNimmtNurEchteAbweichungen(t *testing.T) {
	body := quelleLesen(t, "autoheal.go")

	if strings.Contains(body, "TotalStateRootMismatches() < autoHealMismatchThreshold") {
		t.Error("die Selbstheilung loest wieder ueber TotalStateRootMismatches aus. Diese Zahl " +
			"besteht unter Last fast vollstaendig aus dem Normalfall eines DAG mit zwei " +
			"Produzenten -- gemessen 917 in zehn Minuten, waehrend beide Knoten byte-identisch " +
			"waren. Ein Signal, das unter Last zwangslaeufig anschlaegt, darf keine " +
			"Produktionsunterbrechung ausloesen. EchteStateRootAbweichungen() verwenden.")
	}
	if !strings.Contains(body, "EchteStateRootAbweichungen()") {
		t.Error("EchteStateRootAbweichungen wird nicht mehr benutzt -- dann stuetzt sich der " +
			"Resync wieder auf ein Signal, das den Normalfall mitzaehlt.")
	}
}

func TestStateRoot_GeschwisterWerdenGetrenntGezaehlt(t *testing.T) {
	body := quelleLesen(t, "block.go")

	if !strings.Contains(body, "geschwisterbedingt := dag.ownsProducedHeight(block.Height)") {
		t.Fatal("die Unterscheidung ist weg. Ohne sie landet jede Abweichung im selben Topf, " +
			"und der Topf wird unter Last vom Normalfall geflutet. Die Frage, die zaehlt: hat " +
			"dieser Knoten an derselben Hoehe selbst produziert? Dann stecken die eigenen " +
			"Transaktionen im lokalen Zustand und der Vergleich kann gar nicht aufgehen.")
	}
	// Der Alarm darf nicht mehr an der Gesamtzahl haengen.
	if strings.Contains(body, "alert := dag.stateRootMismatches[block.Proposer] >= 5") {
		t.Error("die Alarmschwelle haengt wieder an der Gesamtzahl statt an den echten " +
			"Abweichungen -- damit meldet das Log unter Last dauerhaft Divergenz, wo keine ist.")
	}
	// Und die Diagnose muss erhalten bleiben: die Gesamtzahl ist weiterhin
	// interessant, sie taugt nur nicht als Ausloeser.
	if !strings.Contains(body, "dag.stateRootMismatches[block.Proposer]++") {
		t.Error("die Gesamtzahl wird nicht mehr gefuehrt. Sie ist als Diagnose wertvoll -- " +
			"nur eben nicht als Entscheidungsgrundlage.")
	}
}

// Ein Zaehler, der nie zurueckgesetzt wird, meldet irgendwann Divergenz aus
// reiner Ansammlung. Der Ruecksetzer auf Uebereinstimmung muss beide Toepfe
// treffen, sonst bleibt der echte stehen.
func TestStateRoot_UebereinstimmungSetztBeideZaehlerZurueck(t *testing.T) {
	body := quelleLesen(t, "block.go")

	i := strings.Index(body, "dag.stateRootMismatches[block.Proposer] = 0")
	if i < 0 {
		t.Fatal("der Ruecksetzer auf Uebereinstimmung ist weg")
	}
	rest := body[i : i+400]
	for _, feld := range []string{"dag.stateRootEcht[block.Proposer] = 0", "dag.stateRootGeschwister[block.Proposer] = 0"} {
		if !strings.Contains(rest, feld) {
			t.Errorf("%s fehlt beim Ruecksetzen. Ein Zaehler, der nur waechst, meldet "+
				"irgendwann Divergenz aus reiner Ansammlung.", feld)
		}
	}
}

// Der zweite Fehlalarm derselben Klasse, einen Tag spaeter: unter Last
// ueberspringen sich die Validatoren, ich produziere nicht an der Hoehe des
// fremden Blocks, und der Geschwister-Filter greift nicht. Mein Zustand
// enthaelt aber Tausende angenommene, noch nicht verblockte Ueberweisungen --
// der Root des fremden Blocks kann sie nicht kennen. Die Selbstheilung loeste
// einen Resync aus, und die Hoehe stand 15 Sekunden.
func TestStateRoot_LokaleUeberweisungenMachenDenVergleichUnscharf(t *testing.T) {
	body := quelleLesen(t, "block.go")
	if !strings.Contains(body, "letzteEigeneUeberweisungNs.Load()") {
		t.Fatal("die Zaehlstelle prueft nicht mehr, ob dieser Knoten gerade Ueberweisungen " +
			"annimmt. Unter Last liegt dann jede fremde Abweichung im Topf der echten, und " +
			"die Selbstheilung haelt die Kette an -- am 12.09.2026 fuer 15 Sekunden bei " +
			"11.000 angenommenen Ueberweisungen je Sekunde.")
	}
	i := strings.Index(body, "geschwisterbedingt := dag.ownsProducedHeight(block.Height)")
	if i < 0 || !strings.Contains(body[i:i+120], "lokaleUnschaerfe") {
		t.Error("die lokale Unschaerfe fliesst nicht in geschwisterbedingt ein")
	}
	st := quelleLesen(t, "state.go")
	if !strings.Contains(st, "letzteEigeneUeberweisungNs.Store(time.Now().UnixNano())") {
		t.Error("TransferAtomic setzt den Zeitstempel nicht mehr -- die Unschaerfe waere dann nie erkennbar")
	}
}

// Die dritte Pruefung derselben Familie: der Vergleich EINES kanonischen
// Block-Hashes mit dem des Primary. In einem DAG mit zwei Bloecken je Hoehe
// waehlt jeder Knoten seinen kanonischen selbst, und unter Last faellt die
// Wahl verschieden aus -- beide Bloecke liegen trotzdem in beiden DAGs. Ein
// Fork heisst: der Primary kennt meinen Block NICHT. Nur das darf einen
// Resync ausloesen, und nur wiederholt.
func TestForkPruefung_FragtDenPrimaryNachDemHashUndBrauchtDreiTreffer(t *testing.T) {
	body := quelleLesen(t, "autoheal.go")
	i := strings.Index(body, "func (dag *BlockDAG) runChainDivergenceCheckOnce(")
	if i < 0 {
		t.Fatal("runChainDivergenceCheckOnce nicht gefunden")
	}
	rumpf := body[i:]
	if j := strings.Index(rumpf[1:], "\nfunc "); j > 0 {
		rumpf = rumpf[:j]
	}
	if !strings.Contains(rumpf, "fetchPrimaryHasBlock(primaryURL, localBlock.Hash)") {
		t.Error("die Divergenzpruefung fragt den Primary nicht mehr nach unserem Block per Hash. " +
			"Ein abweichender kanonischer Hash allein ist in einem DAG kein Fork -- am 12.09.2026 " +
			"warf genau das C2 mitten im Aufholen per Resync ganz zurueck.")
	}
	if !strings.Contains(rumpf, "chainDivergenceFolge.Add(1); n < 3") {
		t.Error("der Resync verlangt nicht mehr drei aufeinanderfolgende Treffer -- ein einzelner " +
			"Netz- oder Ordnungsmoment darf keinen Resync ausloesen")
	}
}

// Nach drei Anlaeufen in zwei Tagen: der StateRoot loest keinen Resync mehr
// aus. Er hasht den Nachzustand einschliesslich aller lokal angenommenen,
// noch nicht verblockten Ueberweisungen -- zwei Knoten mit verschiedenen
// Mempools haben verschiedene Roots, und jeder Filter dagegen hatte eine
// Luecke, die unter Last zu einem Resync fuehrte.
func TestStateRoot_LoestKeinenResyncMehrAus(t *testing.T) {
	body := quelleLesen(t, "autoheal.go")
	i := strings.Index(body, "echt := dag.EchteStateRootAbweichungen()")
	if i < 0 {
		t.Fatal("die Zaehlstelle ist weg")
	}
	rest := body[i : i+2500]
	schalter := strings.Index(rest, `os.Getenv("AEQUITAS_AUTOHEAL_STATEROOT") != "1"`)
	ausloeser := strings.Index(rest, "dag.triggerAutoResync(")
	if schalter < 0 || ausloeser < 0 || schalter > ausloeser {
		t.Error("der StateRoot-Resync ist wieder scharf, ohne ausdruecklichen Schalter davor. " +
			"Das hat am 12.09.2026 chain_accounts unter Last truncated und den Knoten angehalten.")
	}
}
