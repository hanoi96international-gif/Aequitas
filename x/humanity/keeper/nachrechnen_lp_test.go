package keeper

import (
	"math"
	"testing"
)

var lpRegeln = []string{"lp_kein_halter", "lp_doppelt", "lp_anteil", "lp_empfaenger", "validator_kein_mensch"}

func lpNeu(vorher map[string]int64) map[string]int64 {
	m := map[string]int64{}
	for _, r := range lpRegeln {
		if d := erhaltungZaehler(r) - vorher[r]; d != 0 {
			m[r] = d
		}
	}
	return m
}

func lpZaehler() map[string]int64 {
	m := map[string]int64{}
	for _, r := range lpRegeln {
		m[r] = erhaltungZaehler(r)
	}
	return m
}

// lpAnlegen: drei Halter mit 1, 2 und 7 Anteilen, ein Topf von 100 AEQ.
// Ehrlich: 10, 20, 70.
func lpAnlegen(cs *ChainState) {
	for i, a := range erhaltungMenschen {
		cs.accounts.Set(a, &AccountState{Address: a, IsHuman: true, Balance: NewDecimal(100), LPShares: NewDecimal([]float64{1, 2, 7}[i])})
	}
	cs.humanCount = int64(len(erhaltungMenschen))
	cs.pool = &PoolState{ReserveAEQ: NewDecimal(1000), ReserveTUSD: NewDecimal(1000), TotalLPShares: NewDecimal(10)}
	cs.accounts.Set(lpPoolAddr, &AccountState{Address: lpPoolAddr, Balance: NewDecimal(100)})
}

func lpAktiv(t *testing.T) {
	wirtschaftAktivOverride.Store(1)
	t.Cleanup(func() { wirtschaftAktivOverride.Store(math.MaxInt64) })
}

// Gutfall: die echte LP-Runde des Erzeugers nach der Umstellung, in einem und
// in zwei Bloecken.
func TestNachrechnenLP_EchteRundeOhneAbweichung(t *testing.T) {
	lpAktiv(t)
	erzeuger := newTestState()
	lpAnlegen(erzeuger)
	var txs []Transaction
	erzeuger.ausgangOhneDB = func(t Transaction) { txs = append(txs, t) }
	at := nowUnix()
	if err := erzeuger.RunDailyDistributionAtomic(at); err != nil {
		t.Fatal(err)
	}
	var lp int
	for _, tx := range txs {
		if tx.Type == "lp_distribution" {
			lp++
		}
	}
	if lp != 3 {
		t.Fatalf("Vorbedingung: drei LP-Gutschriften erwartet, bekommen %+v", txs)
	}
	for _, teilung := range []int{len(txs), len(txs) - 3} {
		vorher := lpZaehler()
		dag, cs := nachspielKnoten(t, nil)
		cs.mu.Lock()
		lpAnlegen(cs)
		cs.mu.Unlock()
		if !dag.replayTransactions(erhaltungBlock(1, at+5, txs[:teilung]...), true) {
			t.Fatal("erster Block abgelehnt")
		}
		if teilung < len(txs) && !dag.replayTransactions(erhaltungBlock(2, at+6, txs[teilung:]...), true) {
			t.Fatal("zweiter Block abgelehnt")
		}
		if neu := lpNeu(vorher); len(neu) != 0 {
			t.Errorf("Teilung nach %d: ehrliche LP-Runde meldet %v", teilung, neu)
		}
	}
}

func lpAngriff(t *testing.T, aktiv bool, gutschriften ...Transaction) map[string]int64 {
	t.Helper()
	if aktiv {
		lpAktiv(t)
	}
	dag, cs := nachspielKnoten(t, nil)
	cs.mu.Lock()
	lpAnlegen(cs)
	cs.accounts.Set("0xb200000000000000000000000000000000000f01",
		&AccountState{Address: "0xb200000000000000000000000000000000000f01", Balance: NewDecimal(1)})
	cs.mu.Unlock()
	at := nowUnix()
	vorher := lpZaehler()
	txs := append(gutschriften,
		Transaction{Type: "lp_distribution_pool_zero", Amount: 0},
		Transaction{Type: "distribution_round_marker", DistributionAt: at})
	dag.replayTransactions(erhaltungBlock(1, at, txs...), true)
	return lpNeu(vorher)
}

func lpAn(a string, betrag float64) Transaction {
	return Transaction{Type: "lp_distribution", Wallet: a, Amount: betrag}
}

func TestNachrechnenLP_Angriffe(t *testing.T) {
	m := erhaltungMenschen
	fremd := "0xb200000000000000000000000000000000000f01"
	for _, f := range []struct {
		name  string
		txs   []Transaction
		regel string
	}{
		{"ehrlich", []Transaction{lpAn(m[0], 10), lpAn(m[1], 20), lpAn(m[2], 70)}, ""},
		{"umverteilt", []Transaction{lpAn(m[0], 70), lpAn(m[1], 20), lpAn(m[2], 10)}, "lp_anteil"},
		{"kein Halter", []Transaction{lpAn(m[0], 10), lpAn(m[1], 20), lpAn(fremd, 70)}, "lp_kein_halter"},
		{"ausgelassen", []Transaction{lpAn(m[0], 10), lpAn(m[1], 20)}, "lp_empfaenger"},
		{"doppelt", []Transaction{lpAn(m[0], 10), lpAn(m[1], 20), lpAn(m[2], 70), lpAn(m[2], 70)}, "lp_doppelt"},
	} {
		t.Run(f.name, func(t *testing.T) {
			neu := lpAngriff(t, true, f.txs...)
			if f.regel == "" {
				if len(neu) != 0 {
					t.Fatalf("ehrliche Runde meldet %v", neu)
				}
				return
			}
			if neu[f.regel] == 0 {
				t.Fatalf("%s nicht erkannt, gemeldet %v", f.regel, neu)
			}
		})
	}
}

// Vor der Umstellung teilte der Erzeuger mit Anteilen nach der Demurrage --
// keine Pruefung, die eine ehrliche alte Runde falsch melden koennte.
func TestNachrechnenLP_VorDerUmstellungKeinePruefung(t *testing.T) {
	m := erhaltungMenschen
	if neu := lpAngriff(t, false, lpAn(m[0], 70), lpAn(m[1], 20)); len(neu) != 0 {
		t.Fatalf("vor der Umstellung gemeldet: %v", neu)
	}
}

// Rueckrollen: die Bedachten eines abgelehnten Blocks bleiben nicht stehen.
func TestNachrechnenLP_RueckrollenEntferntBedachte(t *testing.T) {
	lpAktiv(t)
	m := erhaltungMenschen
	dag, cs := nachspielKnoten(t, nil)
	cs.mu.Lock()
	lpAnlegen(cs)
	cs.mu.Unlock()
	at := nowUnix()
	vorher := lpZaehler()
	if !dag.replayTransactions(erhaltungBlock(1, at, lpAn(m[0], 10)), true) {
		t.Fatal("erster Block abgelehnt")
	}
	if dag.replayTransactions(erhaltungBlock(2, at, lpAn(m[1], 20),
		Transaction{Type: "ubi_distribution", Amount: 1}), true) {
		t.Fatal("Vorbedingung: der Block muss abgelehnt werden")
	}
	if !dag.replayTransactions(erhaltungBlock(3, at, lpAn(m[1], 20), lpAn(m[2], 70),
		Transaction{Type: "lp_distribution_pool_zero"}), true) {
		t.Fatal("ehrlicher Block abgelehnt")
	}
	if neu := lpNeu(vorher); len(neu) != 0 {
		t.Fatalf("ehrlicher Rest nach Zurueckrollen meldet %v", neu)
	}
}

// Validatoren: der Erzeuger zahlt nur Betreibern, die Menschen sind.
func TestNachrechnenValidator_KeinMensch(t *testing.T) {
	cs := nachrechnenTestState()
	vorher := lpZaehler()
	for _, w := range []string{"0xmensch", "0xfrei"} {
		tx := Transaction{Type: "validator_distribution", Wallet: w, Amount: 1}
		cs.mu.Lock()
		if err := cs.nachrechnenTxLocked(&tx, 1790800000); err != nil {
			t.Fatal(err)
		}
		cs.mu.Unlock()
	}
	if neu := lpNeu(vorher); neu["validator_kein_mensch"] != 1 || len(neu) != 1 {
		t.Fatalf("genau ein validator_kein_mensch erwartet (0xfrei), gemeldet %v", neu)
	}
}
