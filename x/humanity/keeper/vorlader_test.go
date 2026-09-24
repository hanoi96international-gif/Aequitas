package keeper

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func vorladerTxs(n, ab int) ([]Transaction, []int64) {
	txs := make([]Transaction, n)
	ids := make([]int64, n)
	for i := range txs {
		txs[i] = Transaction{TxHash: fmt.Sprintf("0x%d", ab+i)}
		ids[i] = int64(ab + i)
	}
	return txs, ids
}

type freigabeSchreiber struct {
	mu  sync.Mutex
	ids []int64
}

func (f *freigabeSchreiber) freigeben(ids []int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids = append(f.ids, ids...)
}

func TestVorlader_NurEineOffeneVorladung(t *testing.T) {
	var v vorlader
	aufrufe := 0
	laden := func() ([]Transaction, []int64) { aufrufe++; return vorladerTxs(3, 1) }
	if !v.starten(laden) {
		t.Fatal("die erste Vorladung muss starten")
	}
	if v.starten(laden) {
		t.Fatal("eine zweite Vorladung neben einer offenen wuerde Zeilen doppelt beanspruchen")
	}
	vl := v.nehmen()
	<-vl.fertig
	if aufrufe != 1 {
		t.Fatalf("geladen %d mal, erwartet 1", aufrufe)
	}
	if v.nehmen() != nil {
		t.Fatal("nehmen muss den Platz leeren")
	}
}

// Fall 2: Der Deckel ist inzwischen kleiner. Der Block nimmt den Anfang, das
// Ende geht zurueck -- die Reihenfolge der Warteschlange bleibt erhalten.
func TestVorlader_KleinererDeckelGibtDasEndeZurueck(t *testing.T) {
	var v vorlader
	v.starten(func() ([]Transaction, []int64) { return vorladerTxs(10, 100) })
	var f freigabeSchreiber
	frisch := func(int) ([]Transaction, []int64) { t.Fatal("darf nicht frisch laden"); return nil, nil }
	txs, ids := v.nehmen().einloesen(4, f.freigeben, frisch)
	if len(txs) != 4 || len(ids) != 4 || ids[0] != 100 || ids[3] != 103 {
		t.Fatalf("Block traegt %v, erwartet die ersten vier (100..103)", ids)
	}
	if len(f.ids) != 6 || f.ids[0] != 104 || f.ids[5] != 109 {
		t.Fatalf("freigegeben %v, erwartet das Ende 104..109", f.ids)
	}
}

// Fall 3: Eine alte Vorladung geht zurueck und wird frisch geladen -- sie
// bleibt so weit unter der 10-Minuten-Frist des Aufraeumers, der eine
// beanspruchte Zeile ohne Block wieder oeffnet.
func TestVorlader_VeralteteVorladungWirdFrischGeladen(t *testing.T) {
	if vorladungHoechstalter >= 10*time.Minute {
		t.Fatalf("vorladungHoechstalter %s muss deutlich unter der Aufraeumer-Frist (10 min) liegen", vorladungHoechstalter)
	}
	var v vorlader
	v.starten(func() ([]Transaction, []int64) { return vorladerTxs(5, 1) })
	vl := v.nehmen()
	<-vl.fertig
	vl.geladenUm = time.Now().Add(-vorladungHoechstalter - time.Second)
	var f freigabeSchreiber
	frischGeladen := false
	txs, _ := vl.einloesen(5, f.freigeben, func(n int) ([]Transaction, []int64) {
		frischGeladen = true
		return vorladerTxs(2, 50)
	})
	if len(f.ids) != 5 {
		t.Fatalf("freigegeben %d, erwartet alle 5 der alten Vorladung", len(f.ids))
	}
	if !frischGeladen || len(txs) != 2 {
		t.Fatal("nach dem Verwerfen muss frisch geladen werden")
	}
}

// Fall 1: verwerfen wartet eine laufende Vorladung ab und gibt sie frei.
func TestVorlader_VerwerfenWartetUndGibtFrei(t *testing.T) {
	var v vorlader
	los := make(chan struct{})
	v.starten(func() ([]Transaction, []int64) { <-los; return vorladerTxs(3, 7) })
	var f freigabeSchreiber
	fertig := make(chan struct{})
	go func() { v.verwerfen(f.freigeben); close(fertig) }()
	select {
	case <-fertig:
		t.Fatal("verwerfen darf nicht zurueckkehren, bevor die Vorladung fertig ist -- sonst bliebe sie beansprucht")
	case <-time.After(20 * time.Millisecond):
	}
	close(los)
	<-fertig
	if len(f.ids) != 3 {
		t.Fatalf("freigegeben %v, erwartet 3 IDs", f.ids)
	}
	v.verwerfen(f.freigeben) // ohne offene Vorladung: nichts
	if len(f.ids) != 3 {
		t.Fatal("verwerfen ohne Vorladung darf nichts freigeben")
	}
}

func TestVorlader_NachDemAbschaltenKeineNeue(t *testing.T) {
	var v vorlader
	v.starten(func() ([]Transaction, []int64) { return vorladerTxs(2, 1) })
	var f freigabeSchreiber
	v.abschalten(f.freigeben)
	if len(f.ids) != 2 {
		t.Fatalf("beim Abschalten freigegeben %v, erwartet die offene Vorladung", f.ids)
	}
	if v.starten(func() ([]Transaction, []int64) { return vorladerTxs(1, 9) }) {
		t.Fatal("nach dem Herunterfahren darf nichts mehr beansprucht werden")
	}
}

// Ein Panic im Lader darf ProduceBlock nicht ewig warten lassen.
func TestVorlader_PanicImLaderSchliesstTrotzdem(t *testing.T) {
	var v vorlader
	v.starten(func() ([]Transaction, []int64) { panic("test") })
	vl := v.nehmen()
	select {
	case <-vl.fertig:
	case <-time.After(2 * time.Second):
		t.Fatal("nach einem Panic im Lader bleibt fertig offen -- ProduceBlock haengt")
	}
}

// ProduceBlock muss bei einem Abbruch die Vorladung mit freigeben (Fall 1).
// Ohne das kaeme der naechste Block mit den spaeteren Ueberweisungen eines
// Absenders vor dessen frueheren.
func TestProduceBlock_AbbruchVerwirftVorladung(t *testing.T) {
	b, err := os.ReadFile("block.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	start := strings.Index(body, "func (dag *BlockDAG) ProduceBlock() *Block {")
	if start < 0 {
		t.Fatal("ProduceBlock nicht gefunden")
	}
	rumpf := body[start:]
	ende := strings.Index(rumpf, "\n}\n")
	rumpf = rumpf[:ende]
	abbruch := strings.Index(rumpf, "if !blockGespeichert")
	if abbruch < 0 {
		t.Fatal("Abbruch-Freigabe nicht gefunden -- Test umhaengen, nicht loeschen")
	}
	stueck := rumpf[abbruch:]
	stueck = stueck[:strings.Index(stueck, "}()")]
	if !strings.Contains(stueck, "dag.vorlauf.verwerfen(") {
		t.Error("die Abbruch-Freigabe in ProduceBlock verwirft die Vorladung nicht mehr")
	}
	if !strings.Contains(rumpf, "dag.vorlauf.nehmen()") || !strings.Contains(rumpf, "dag.vorlauf.starten(") {
		t.Error("ProduceBlock nutzt die Vorladung nicht")
	}
}

// Mit echter Datenbank: Laden, Vorladen, Abbruch -- danach ist jede Zeile
// wieder offen und der naechste Block sieht sie in der Reihenfolge der
// Warteschlange.
func TestVorlader_AbbruchOeffnetAlleZeilen_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-vorlader-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}
	const anzahl = 8
	for i := 0; i < anzahl; i++ {
		tx := Transaction{
			Type: "transfer", Wallet: distTestAddr(710), To: distTestAddr(810),
			Amount: 1, TxHash: fmt.Sprintf("0xvorlader-%d", i),
		}
		if err := cs.SavePendingTx(tx); err != nil {
			t.Fatalf("Ausgangskorb fuellen: %v", err)
		}
	}
	var v vorlader
	_, ids := cs.LoadPendingTxsWithLimit(4) // der aktuelle Block
	if len(ids) != 4 {
		t.Fatalf("geladen %d, erwartet 4", len(ids))
	}
	v.starten(func() ([]Transaction, []int64) { return cs.LoadPendingTxsWithLimit(4) })
	vl := v.nehmen()
	<-vl.fertig
	if len(vl.ids) != 4 || vl.ids[0] <= ids[3] {
		t.Fatalf("Vorladung %v muss die naechsten vier NACH %v tragen", vl.ids, ids)
	}
	v.offen = vl // zurueck an ihren Platz, wie waehrend des Blockbaus

	// Abbruch, genau wie in ProduceBlock.
	cs.PendingTxIDsFreigeben(ids)
	v.verwerfen(cs.PendingTxIDsFreigeben)

	if offen := offeneAusgangskorbZeilen(t, cs); offen != anzahl {
		t.Fatalf("nach dem Abbruch %d von %d Zeilen offen -- die Vorladung haelt noch welche", offen, anzahl)
	}
	txs, _ := cs.LoadPendingTxsWithLimit(anzahl)
	for i, tx := range txs {
		if want := fmt.Sprintf("0xvorlader-%d", i); tx.TxHash != want {
			t.Fatalf("Position %d traegt %s, erwartet %s -- die Reihenfolge der Warteschlange ist verloren", i, tx.TxHash, want)
		}
	}
}
