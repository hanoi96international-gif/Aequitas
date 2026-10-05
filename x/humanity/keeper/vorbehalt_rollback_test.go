package keeper

import "testing"

// Ein zurueckgewiesener Block darf keinen Vorbehalt hinterlassen -- weder in
// cs.vorbehalte noch auf dem Vorbehaltskonto. Sonst lehnt der Knoten den
// ehrlichen Block mit demselben Vorbehalt spaeter ab ("existiert schon") oder
// ueberspringt dessen Ausfuehrung ("deckt die Rueckbuchung nicht") und laeuft
// den anderen davon.

// feindlicherBlock: die Transaktionen eines ehrlichen Blocks und dahinter eine,
// die das Nachspielen des ganzen Blocks scheitern laesst.
func feindlicherBlock(ehrlich *Block, hash string) *Block {
	txs := append(append([]Transaction{}, ehrlich.Transactions...), Transaction{Type: "gibt_es_nicht", Wallet: "0xvz", TxHash: hash + "-gift"})
	return &Block{Height: ehrlich.Height, Hash: hash, Timestamp: ehrlich.Timestamp, Transactions: txs}
}

func offenBei(cs *ChainState, konto string) bool {
	offeneVorbehalteMu.Lock()
	defer offeneVorbehalteMu.Unlock()
	_, ok := cs.vorbehalte[konto]
	return ok
}

// Schritt 1 in einem Block, der scheitert: danach kein offener Vorbehalt und
// kein Einsatz auf dem Vorbehaltskonto; der ehrliche Block geht durch.
func TestVorbehalt_ZurueckgewiesenerBlockHinterlaesstKeinenVorbehalt(t *testing.T) {
	a, b := vorbehaltsKnoten(t, "a"), vorbehaltsKnoten(t, "b")
	id, err := a.cs.VorbehaltAtomic(Transaction{Type: "swap_aeq_tusd", Wallet: "0xvx", Amount: 1_000, TxHash: "0xvbrb1"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	v := vorbehaltsKonto(id)
	blkA := a.block(t, 1)
	rootVorher := b.cs.StateRoot()

	if b.dag.replayTransactions(feindlicherBlock(blkA, "0xvbrb-feind1"), true) {
		t.Fatal("B nahm den Block mit der unbekannten Transaktion an")
	}
	if vAcc, ok := b.cs.accounts.Get(v); ok && (!vAcc.Balance.IsZero() || !vAcc.TUsdBalance.IsZero() || !vAcc.LPShares.IsZero()) {
		t.Fatalf("nach dem Zurueckrollen liegt noch ein Einsatz auf %s: %+v", v, vAcc)
	}
	if offenBei(b.cs, v) {
		t.Fatal("nach dem Zurueckrollen steht der Vorbehalt noch in cs.vorbehalte")
	}
	if got := stand(b.cs, "0xvx"); got != 2_000 {
		t.Fatalf("X hat nach dem Zurueckrollen %.6f statt 2.000", got)
	}
	if got := b.cs.StateRoot(); got != rootVorher {
		t.Fatalf("StateRoot nach dem Zurueckrollen %s, vorher %s", got, rootVorher)
	}
	// Der Leiter darf keinen Vorbehalt ausfuehren, den es nicht gibt.
	if n := b.cs.VorbehalteAbarbeiten(); n != 0 {
		t.Fatalf("B fuehrte %d Vorbehalte aus einem zurueckgewiesenen Block aus", n)
	}

	if !b.dag.replayTransactions(blkA, true) {
		t.Fatal("B lehnte danach den ehrlichen Block ab")
	}
	if n := b.cs.VorbehalteAbarbeiten(); n != 1 {
		t.Fatalf("B fuehrte %d Vorbehalte aus statt 1", n)
	}
	blkB := b.block(t, 2)
	if !a.dag.replayTransactions(blkB, true) {
		t.Fatal("A lehnte B's Block ab")
	}
	gleicherZustand(t, a, b)
}

// Schritt 2 in einem Block, der scheitert: der Vorbehalt bleibt offen, der
// Einsatz auf dem Vorbehaltskonto; der ehrliche Block zahlt danach aus.
func TestVorbehalt_ZurueckgewieseneAusfuehrungBleibtOffen(t *testing.T) {
	a, b, c := vorbehaltsKnoten(t, "a"), vorbehaltsKnoten(t, "b"), vorbehaltsKnoten(t, "c")
	id, err := a.cs.VorbehaltAtomic(Transaction{Type: "swap_aeq_tusd", Wallet: "0xvx", Amount: 1_000, TxHash: "0xvbrb2"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	v := vorbehaltsKonto(id)
	blkA := a.block(t, 1)
	for _, k := range []*annahmeKnoten{b, c} {
		if !k.dag.replayTransactions(blkA, true) {
			t.Fatalf("%s lehnte A's Block ab", k.name)
		}
	}
	if n := b.cs.VorbehalteAbarbeiten(); n != 1 {
		t.Fatalf("B fuehrte %d Vorbehalte aus statt 1", n)
	}
	blkB := b.block(t, 2)
	einsatz := stand(c.cs, v)
	if einsatz <= 0 {
		t.Fatalf("kein Einsatz auf %s bei C", v)
	}

	if c.dag.replayTransactions(feindlicherBlock(blkB, "0xvbrb-feind2"), true) {
		t.Fatal("C nahm den Block mit der unbekannten Transaktion an")
	}
	if got := stand(c.cs, v); got != einsatz {
		t.Fatalf("Einsatz auf %s nach dem Zurueckrollen %.6f statt %.6f", v, got, einsatz)
	}
	if !offenBei(c.cs, v) {
		t.Fatal("nach dem Zurueckrollen fehlt der offene Vorbehalt in cs.vorbehalte")
	}
	if got := stand(c.cs, "0xvx"); got != 2_000-1_020 {
		t.Fatalf("X hat nach dem Zurueckrollen %.6f statt 980", got)
	}

	if !c.dag.replayTransactions(blkB, true) {
		t.Fatal("C lehnte danach B's ehrlichen Block ab")
	}
	if offenBei(c.cs, v) {
		t.Fatal("nach der Ausfuehrung steht der Vorbehalt noch offen")
	}
	if !a.dag.replayTransactions(blkB, true) {
		t.Fatal("A lehnte B's Block ab")
	}
	gleicherZustand(t, a, b, c)
	x, _ := c.cs.accounts.Get("0xvx")
	if x.TUsdBalance.Float() <= 500 {
		t.Fatalf("X hat bei C keinen tUSD erhalten: %v", x.TUsdBalance)
	}
}

// Sicherung direkt: der Teil-Snapshot stellt genau die genannten
// Vorbehaltskonten her (auch "gab es nicht"), der volle die ganze Karte. Eine
// Ueberweisung nennt keines und sichert nichts.
func TestVorbehaltSicherung_TeilUndVoll(t *testing.T) {
	_, cs := newDeterminismTestDAG()
	o1 := offenerVorbehalt{wallet: "0xvx", art: "swap_aeq_tusd", betrag: 10}
	o2 := offenerVorbehalt{wallet: "0xvz", art: "add_liquidity", betrag: 1, betrag2: 1}
	cs.vorbehalte = map[string]offenerVorbehalt{"vorbehalt:a": o1}

	if s := cs.vorbehaltSichern([]string{"0xvx", "0xvz", ubiPoolAddr}, false); s.eintraege != nil {
		t.Fatalf("Ueberweisung sicherte Vorbehalte: %+v", s.eintraege)
	}

	s := cs.vorbehaltSichern([]string{"0xvx", "vorbehalt:a", "vorbehalt:c"}, false)
	delete(cs.vorbehalte, "vorbehalt:a")
	cs.vorbehalte["vorbehalt:c"] = o2
	cs.vorbehalte["vorbehalt:d"] = o2 // nicht genannt: bleibt
	cs.vorbehaltZurueck(s)
	if got, ok := cs.vorbehalte["vorbehalt:a"]; !ok || got != o1 {
		t.Fatalf("vorbehalt:a nicht wiederhergestellt: %+v %v", got, ok)
	}
	if _, ok := cs.vorbehalte["vorbehalt:c"]; ok {
		t.Fatal("vorbehalt:c gab es vor dem Snapshot nicht und steht noch da")
	}
	if _, ok := cs.vorbehalte["vorbehalt:d"]; !ok {
		t.Fatal("vorbehalt:d war nicht im Snapshot und darf nicht verschwinden")
	}

	delete(cs.vorbehalte, "vorbehalt:d")
	v := cs.vorbehaltSichern(nil, true)
	delete(cs.vorbehalte, "vorbehalt:a")
	cs.vorbehalte["vorbehalt:e"] = o2
	cs.vorbehaltZurueck(v)
	if len(cs.vorbehalte) != 1 || cs.vorbehalte["vorbehalt:a"] != o1 {
		t.Fatalf("voller Snapshot nicht wiederhergestellt: %+v", cs.vorbehalte)
	}
}
