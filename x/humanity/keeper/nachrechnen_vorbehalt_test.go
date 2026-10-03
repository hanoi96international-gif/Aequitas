package keeper

import (
	"testing"
)

func vorbehaltZaehler() map[string]int64 {
	m := map[string]int64{}
	for _, r := range []string{"vorbehalt_unbekannt", "vorbehalt_fremd", "vorbehalt_betrag",
		"vorbehalt_mindestbetrag", "vorbehalt_unlesbar", "tausch_ergebnis", "lp_anteile"} {
		m[r] = erhaltungZaehler(r)
	}
	return m
}

// vorbehaltAusfuehrungFuerTest: A nimmt den Vorbehalt an, B (Leiter) fuehrt
// ihn aus, C hat nur den ersten Schritt nachgespielt -- C kennt den
// unterschriebenen Auftrag und steht vor der Ausfuehrung. Zurueck: C und die
// ehrliche Ausfuehrung aus B's Block.
func vorbehaltAusfuehrungFuerTest(t *testing.T, tmpl Transaction, minOut float64) (*annahmeKnoten, Transaction) {
	t.Helper()
	a, b, c := vorbehaltsKnoten(t, "a"), vorbehaltsKnoten(t, "b"), vorbehaltsKnoten(t, "c")
	if _, err := a.cs.VorbehaltAtomic(tmpl, minOut); err != nil {
		t.Fatal(err)
	}
	blkA := a.block(t, 1)
	for _, k := range []*annahmeKnoten{b, c} {
		if !k.dag.replayTransactions(blkA, true) {
			t.Fatalf("%s lehnte den Vorbehalt ab", k.name)
		}
	}
	if n := b.cs.VorbehalteAbarbeiten(); n != 1 {
		t.Fatalf("B fuehrte %d Vorbehalte aus statt 1", n)
	}
	blkB := b.block(t, 2)
	for _, tx := range blkB.Transactions {
		if tx.Type == "vorbehalt_ausfuehrung" {
			return c, tx
		}
	}
	t.Fatal("keine Ausfuehrung in B's Block")
	return nil, Transaction{}
}

func vorbehaltPruefe(t *testing.T, k *annahmeKnoten, tx Transaction) map[string]int64 {
	t.Helper()
	vorher := vorbehaltZaehler()
	k.cs.mu.Lock()
	err := k.cs.nachrechnenTxLocked(&tx, nowUnix())
	k.cs.mu.Unlock()
	if err != nil {
		t.Fatalf("im Beobachtungsmodus darf nichts abgelehnt werden: %v", err)
	}
	neu := map[string]int64{}
	for r, v := range vorher {
		if d := erhaltungZaehler(r) - v; d != 0 {
			neu[r] = d
		}
	}
	return neu
}

func kopieMitVorbehalt(tx Transaction) Transaction {
	k := tx
	vb := *tx.Vorbehalt
	k.Vorbehalt = &vb
	return k
}

// Gutfall und Missbrauch beim Tausch: die ehrliche Ausfuehrung bleibt ohne
// Meldung; ein erfundenes Ergebnis, ein groesserer Betrag als
// unterschrieben, eine fremde Wallet, eine unbekannte Referenz und ein
// Ergebnis unter dem Mindestbetrag fallen auf.
func TestNachrechnenVorbehalt_Tausch(t *testing.T) {
	c, ehrlich := vorbehaltAusfuehrungFuerTest(t,
		Transaction{Type: "swap_aeq_tusd", Wallet: "0xvx", Amount: 1_000, TxHash: "0xvbn1"}, 900)
	if ehrlich.Vorbehalt == nil || ehrlich.Vorbehalt.Erstattet {
		t.Fatalf("Vorbedingung: ehrliche Ausfuehrung erwartet, bekam %+v", ehrlich.Vorbehalt)
	}
	if got := vorbehaltPruefe(t, c, ehrlich); len(got) != 0 {
		t.Fatalf("ehrliche Ausfuehrung gemeldet: %v", got)
	}

	erfunden := kopieMitVorbehalt(ehrlich)
	erfunden.AmountOut *= 10
	if got := vorbehaltPruefe(t, c, erfunden); got["tausch_ergebnis"] != 1 {
		t.Fatalf("erfundenes Tauschergebnis nicht erkannt: %v", got)
	}

	mehr := kopieMitVorbehalt(ehrlich)
	mehr.Amount = 1_900
	if got := vorbehaltPruefe(t, c, mehr); got["vorbehalt_betrag"] != 1 {
		t.Fatalf("Betrag ueber dem unterschriebenen nicht erkannt: %v", got)
	}

	fremd := kopieMitVorbehalt(ehrlich)
	fremd.Wallet = "0xvz"
	if got := vorbehaltPruefe(t, c, fremd); got["vorbehalt_fremd"] != 1 {
		t.Fatalf("fremde Wallet nicht erkannt: %v", got)
	}

	andereArt := kopieMitVorbehalt(ehrlich)
	andereArt.Vorbehalt.Art = "swap_tusd_aeq"
	if got := vorbehaltPruefe(t, c, andereArt); got["vorbehalt_fremd"] != 1 {
		t.Fatalf("andere Art nicht erkannt: %v", got)
	}

	unbekannt := kopieMitVorbehalt(ehrlich)
	unbekannt.Vorbehalt.Ref = "0xgibtesnicht"
	if got := vorbehaltPruefe(t, c, unbekannt); got["vorbehalt_unbekannt"] != 1 {
		t.Fatalf("unbekannte Referenz nicht erkannt: %v", got)
	}

	zuWenig := kopieMitVorbehalt(ehrlich)
	zuWenig.AmountOut = 800
	if got := vorbehaltPruefe(t, c, zuWenig); got["vorbehalt_mindestbetrag"] != 1 {
		t.Fatalf("Ergebnis unter dem Mindestbetrag nicht erkannt: %v", got)
	}

	// Eine Erstattung bucht nur zurueck: keine Meldung, auch ohne Betraege.
	erstattet := kopieMitVorbehalt(ehrlich)
	erstattet.Vorbehalt.Erstattet = true
	erstattet.Amount, erstattet.AmountOut = 0, 0
	if got := vorbehaltPruefe(t, c, erstattet); len(got) != 0 {
		t.Fatalf("Erstattung gemeldet: %v", got)
	}
}

// Einlage ueber einen Vorbehalt: erfundene LP-Anteile und ein anderer
// zweiter Betrag fallen auf.
func TestNachrechnenVorbehalt_Einlage(t *testing.T) {
	c, ehrlich := vorbehaltAusfuehrungFuerTest(t,
		Transaction{Type: "add_liquidity", Wallet: "0xvx", Amount: 100, AmountOut: 100, TxHash: "0xvbn2"}, 0)
	if got := vorbehaltPruefe(t, c, ehrlich); len(got) != 0 {
		t.Fatalf("ehrliche Einlage gemeldet: %v", got)
	}
	erfunden := kopieMitVorbehalt(ehrlich)
	erfunden.LPShares *= 50
	if got := vorbehaltPruefe(t, c, erfunden); got["lp_anteile"] != 1 {
		t.Fatalf("erfundene LP-Anteile nicht erkannt: %v", got)
	}
	anders := kopieMitVorbehalt(ehrlich)
	anders.AmountOut = 400
	if got := vorbehaltPruefe(t, c, anders); got["vorbehalt_betrag"] != 1 {
		t.Fatalf("anderer zweiter Betrag nicht erkannt: %v", got)
	}
}

// Im strengen Modus lehnt der Nachspielende eine Ausfuehrung mit erfundenem
// Tauschergebnis ab; der Pool bleibt unberuehrt.
func TestNachrechnenVorbehalt_StrengLehntAb(t *testing.T) {
	c, ehrlich := vorbehaltAusfuehrungFuerTest(t,
		Transaction{Type: "swap_aeq_tusd", Wallet: "0xvx", Amount: 1_000, TxHash: "0xvbn3"}, 0)
	nachrechnenStrengOverride.Store(1)
	t.Cleanup(func() { nachrechnenStrengOverride.Store(0) })

	erfunden := kopieMitVorbehalt(ehrlich)
	erfunden.AmountOut *= 10
	vorher := c.cs.pool.ReserveTUSD
	blk := &Block{Height: 2, Hash: "0xvb-streng-erfunden", Timestamp: nowUnix(), Transactions: []Transaction{erfunden}}
	if c.dag.replayTransactions(blk, true) {
		t.Fatal("Ausfuehrung mit erfundenem Ergebnis angenommen")
	}
	if c.cs.pool.ReserveTUSD != vorher {
		t.Fatalf("Pool veraendert: %v -> %v", vorher, c.cs.pool.ReserveTUSD)
	}
	blkEhrlich := &Block{Height: 2, Hash: "0xvb-streng-ehrlich", Timestamp: nowUnix(), Transactions: []Transaction{ehrlich}}
	if !c.dag.replayTransactions(blkEhrlich, true) {
		t.Fatal("ehrliche Ausfuehrung im strengen Modus abgelehnt")
	}
}

// pool_correction auf der V8-Kette: weder erzeugen noch nachspielen.
func TestPoolCorrection_AufV8Gesperrt(t *testing.T) {
	zurueck := _setVertragForTest(&vertragKonfig{version: vertragVersionV8})
	defer zurueck()
	dag, cs := newDeterminismTestDAG()
	cs.mu.Lock()
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(1000), ReserveTUSD: NewDecimal(1000), TotalLPShares: NewDecimal(100)}
	cs.mu.Unlock()

	if err := cs.CorrectPhantomSupplyAtomic(999, 999); err == nil {
		t.Fatal("pool_correction auf V8 erzeugt")
	}
	blk := &Block{Height: 1, Hash: "0xpc-v8", Timestamp: nowUnix(),
		Transactions: []Transaction{{Type: "pool_correction", Amount: 999, AmountOut: 999}}}
	if dag.replayTransactions(blk, true) {
		t.Fatal("pool_correction auf V8 nachgespielt")
	}
	if cs.pool.ReserveAEQ != NewDecimal(1000) || cs.pool.ReserveTUSD != NewDecimal(1000) {
		t.Fatalf("Reserven veraendert: %v/%v", cs.pool.ReserveAEQ, cs.pool.ReserveTUSD)
	}
}
