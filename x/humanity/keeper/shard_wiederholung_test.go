package keeper

import (
	"testing"
	"time"
)

// Ein belegter Shard, der rechtzeitig frei wird, darf NICHT zum Rueckfall
// fuehren -- das ist der ganze Zweck.
func TestShardWiederholung_RettetEinenKurzBelegtenShard(t *testing.T) {
	t.Setenv(shardRetryVersucheEnv, "20")
	t.Setenv(shardRetryPauseEnv, "1000") // 1 ms, also bis zu 20 ms Fenster
	shardRetryZuruecksetzen()

	cs := &ChainState{accounts: newShardedAccounts()}
	cs.accounts.Set("0xa", &AccountState{Address: "0xa"})
	cs.accounts.Set("0xb", &AccountState{Address: "0xb"})

	// Halten und nach 5 ms freigeben -- gut innerhalb des Fensters.
	halt, ok := cs.accounts.TryLockAddrs("0xa")
	if !ok {
		t.Fatal("konnte den Shard nicht vorbelegen")
	}
	go func() { time.Sleep(5 * time.Millisecond); halt() }()

	start := time.Now()
	unlock, ok := cs.sperreMitKurzerWiederholung("0xa", "0xb")
	dauer := time.Since(start)
	if !ok {
		t.Fatal("aufgegeben, obwohl der Shard nach 5 ms frei wurde -- genau diesen " +
			"Fall soll das Wiederholen abfangen (ein Rueckfall kostet gemessen ~800 ms)")
	}
	unlock()
	if dauer > 60*time.Millisecond {
		t.Fatalf("hat %s gewartet -- das Fenster wird nicht eingehalten", dauer)
	}
	if shardRetryGerettet.Load() != 1 {
		t.Fatalf("Rettungszaehler steht auf %d, erwartet 1 -- ohne ihn ist nicht "+
			"erkennbar, ob der Schalter wirkt", shardRetryGerettet.Load())
	}
}

// Ein dauerhaft belegter Shard muss weiterhin zurueckfallen. Wuerde hier
// unbegrenzt gewartet, waere die Schutzwirkung des nicht-blockierenden
// Entwurfs weg und ein einziges heisses Konto koennte alles anhalten.
func TestShardWiederholung_GibtBeiDauerhaftemHaltAuf(t *testing.T) {
	t.Setenv(shardRetryVersucheEnv, "3")
	t.Setenv(shardRetryPauseEnv, "1000")
	shardRetryZuruecksetzen()

	cs := &ChainState{accounts: newShardedAccounts()}
	cs.accounts.Set("0xa", &AccountState{Address: "0xa"})
	halt, _ := cs.accounts.TryLockAddrs("0xa")
	defer halt()

	start := time.Now()
	_, ok := cs.sperreMitKurzerWiederholung("0xa")
	dauer := time.Since(start)
	if ok {
		t.Fatal("hat einen dauerhaft gehaltenen Shard erworben")
	}
	if dauer > 200*time.Millisecond {
		t.Fatalf("hat %s gewartet, obwohl 3 x 1 ms konfiguriert sind -- das Fenster "+
			"ist nicht gedeckelt, und ein heisses Konto koennte alles anhalten", dauer)
	}
	if shardRetryVergeblich.Load() != 1 {
		t.Fatalf("Vergeblich-Zaehler steht auf %d, erwartet 1", shardRetryVergeblich.Load())
	}
}

// Ausdrueckliche 0 schaltet ab und darf dann KEINE Pause kosten.
func TestShardWiederholung_NullSchaltetAbOhneZuWarten(t *testing.T) {
	t.Setenv(shardRetryVersucheEnv, "0")
	shardRetryZuruecksetzen()

	cs := &ChainState{accounts: newShardedAccounts()}
	cs.accounts.Set("0xa", &AccountState{Address: "0xa"})
	halt, _ := cs.accounts.TryLockAddrs("0xa")
	defer halt()

	start := time.Now()
	_, ok := cs.sperreMitKurzerWiederholung("0xa")
	if ok {
		t.Fatal("hat erworben, obwohl gehalten")
	}
	if d := time.Since(start); d > 2*time.Millisecond {
		t.Fatalf("abgeschaltet, hat aber %s gewartet", d)
	}
}

func shardRetryZuruecksetzen() {
	shardRetryGerettet.Store(0)
	shardRetryVergeblich.Store(0)
}
