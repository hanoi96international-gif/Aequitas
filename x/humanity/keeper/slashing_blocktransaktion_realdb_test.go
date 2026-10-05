package keeper

import (
	"crypto/ecdsa"
	"fmt"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// Strafe fuer Doppelsignatur beim Nachspielen (slash_equivocation). Beweis
// und Sperre muessen in der Transaktion des Blocks stehen: ein
// zurueckgewiesener Block darf niemanden sperren, und die 50 AEQ beim
// zweiten Vergehen zieht jeder Knoten genau einmal ab -- auch der, der die
// Doppelsignatur selbst gesehen und schon vermerkt hat. Braucht
// equivocation_evidence und validator_penalties, also eine echte Datenbank.

type strafKnoten struct {
	t      *testing.T
	cs     *ChainState
	dag    *BlockDAG
	n      int
	key    *ecdsa.PrivateKey
	signer string
	op     string
}

func neuerStrafKnoten(t *testing.T) *strafKnoten {
	t.Helper()
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-strafe-realdb-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung -- DATABASE_URL pruefen")
	}
	for _, q := range []string{`DELETE FROM equivocation_evidence`, `DELETE FROM validator_penalties`, `DELETE FROM registered_nodes`} {
		if _, err := cs.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	cs.invalidatePenaltyCache()
	dag := newOrphanTestDAG()
	dag.state = cs
	dag.bootHeight = 0
	dag.replayedBlocks = make(map[string]bool)
	dag.replayFailures = make(map[string]replayFailureState)
	dag.stateRootMismatches = map[string]int{}
	dag.stateRootMismatchLastAt = map[string]int64{}

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	k := &strafKnoten{t: t, cs: cs, dag: dag, key: key,
		signer: strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex()), op: distTestAddr(1700)}
	if _, err := cs.db.Exec(`INSERT INTO registered_nodes (wallet_address, signing_address) VALUES ($1, $2)`, k.op, k.signer); err != nil {
		t.Fatal(err)
	}
	cs.mu.Lock()
	acc := &AccountState{Address: k.op, Balance: NewDecimal(100)}
	cs.accounts.Set(k.op, acc)
	err = cs.saveAccountToDB(acc)
	cs.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// doppelsignatur: zwei verschiedene, gueltig unterschriebene Bloecke auf
// dieselben Eltern; die Strafe traegt beide Koepfe als Beweis.
func (k *strafKnoten) doppelsignatur(zeit int64, eltern ...string) Transaction {
	k.t.Helper()
	a := signierterTestblock(k.t, k.key, 10, zeit, eltern, []Transaction{{Type: "transfer", Wallet: "0x1", To: "0x2", Amount: 1}})
	b := signierterTestblock(k.t, k.key, 10, zeit+1, eltern, []Transaction{{Type: "transfer", Wallet: "0x1", To: "0x3", Amount: 1}})
	return slashTx(k.signer, a, b, &Doppelbeweis{A: kopfVon(a), B: kopfVon(b)})
}

func (k *strafKnoten) block(txs ...Transaction) bool {
	k.t.Helper()
	k.n++
	b := &Block{Height: int64(k.n), Hash: fmt.Sprintf("strafe-%s-%d", k.t.Name(), k.n), Timestamp: nowUnix(), Transactions: txs}
	return k.dag.replayTransactions(b, true)
}

func (k *strafKnoten) beweise(tx Transaction) int {
	k.t.Helper()
	a, b := tx.BlockAHash, tx.BlockBHash
	if a > b {
		a, b = b, a
	}
	var n int
	if err := k.cs.db.QueryRow(`SELECT COUNT(*) FROM equivocation_evidence WHERE block_a_hash = $1 AND block_b_hash = $2`, a, b).Scan(&n); err != nil {
		k.t.Fatal(err)
	}
	return n
}

func (k *strafKnoten) vergehen() int {
	k.t.Helper()
	var n int
	k.cs.db.QueryRow(`SELECT offense_count FROM validator_penalties WHERE signing_address = $1`, k.signer).Scan(&n)
	return n
}

// guthaben: im Speicher und in der Datenbank -- beide muessen stimmen.
func (k *strafKnoten) guthaben() float64 {
	k.t.Helper()
	var db float64
	if err := k.cs.db.QueryRow(`SELECT balance FROM chain_accounts WHERE lower(address) = $1`, k.op).Scan(&db); err != nil {
		k.t.Fatal(err)
	}
	if mem := stand(k.cs, k.op); mem != db {
		k.t.Fatalf("Guthaben von %s: Speicher %.6f, Datenbank %.6f", k.op, mem, db)
	}
	return db
}

func gift() Transaction {
	return Transaction{Type: "gibt_es_nicht", Wallet: "0xgift", TxHash: "0xgift"}
}

// Missbrauch: ein Peer schickt eine echte Doppelsignatur in einem Block, der
// scheitert. Danach darf der Validator nicht gesperrt sein -- die anderen
// Knoten haben den Block ebenfalls verworfen. Erst der ehrliche Block sperrt.
func TestStrafe_ZurueckgewiesenerBlockSperrtNicht_RealDB(t *testing.T) {
	k := neuerStrafKnoten(t)
	erstes := k.doppelsignatur(nowUnix()-3600, "aa", "bb")

	if k.block(erstes, gift()) {
		t.Fatal("Block mit unbekannter Transaktion angenommen")
	}
	if n := k.beweise(erstes); n != 0 {
		t.Fatalf("nach dem Zurueckrollen stehen %d Beweise in equivocation_evidence", n)
	}
	if gesperrt, grund := k.cs.IsValidatorSuspended(k.signer, 0); gesperrt {
		t.Fatalf("Validator nach einem zurueckgewiesenen Block gesperrt: %s", grund)
	}

	if !k.block(erstes) {
		t.Fatal("ehrlicher Block abgelehnt")
	}
	if n := k.vergehen(); n != 1 {
		t.Fatalf("%d Vergehen statt 1", n)
	}
	if gesperrt, _ := k.cs.IsValidatorSuspended(k.signer, 0); !gesperrt {
		t.Fatal("nach dem ehrlichen Block nicht gesperrt (Zwischenspeicher nicht erneuert?)")
	}
}

// Das zweite Vergehen erst in einem Block, der scheitert, dann im ehrlichen:
// die 50 AEQ werden abgezogen -- vorher blieb der Beweis aus dem gescheiterten
// Block stehen, und der ehrliche zog nichts mehr ab.
func TestStrafe_ZweitesVergehenNachZurueckweisungZiehtAb_RealDB(t *testing.T) {
	k := neuerStrafKnoten(t)
	if !k.block(k.doppelsignatur(nowUnix()-5*86400, "aa", "bb")) {
		t.Fatal("erstes Vergehen abgelehnt")
	}
	zweites := k.doppelsignatur(nowUnix()-3600, "cc", "dd")

	if k.block(zweites, gift()) {
		t.Fatal("Block mit unbekannter Transaktion angenommen")
	}
	if got := k.guthaben(); got != 100 {
		t.Fatalf("nach dem Zurueckrollen %.6f statt 100", got)
	}
	if n, v := k.beweise(zweites), k.vergehen(); n != 0 || v != 1 {
		t.Fatalf("nach dem Zurueckrollen %d Beweise, %d Vergehen (erwartet 0, 1)", n, v)
	}

	if !k.block(zweites) {
		t.Fatal("ehrlicher Block abgelehnt")
	}
	if got := k.guthaben(); got != 100-equivocationSecondOffensePenaltyAEQ {
		t.Fatalf("nach dem zweiten Vergehen %.6f statt %.0f", got, 100-equivocationSecondOffensePenaltyAEQ)
	}
	if v := k.vergehen(); v != 2 {
		t.Fatalf("%d Vergehen statt 2", v)
	}
}

// Der Knoten, der die Doppelsignatur selbst sieht (AddPeerBlock), vermerkt
// sie, zieht die 50 AEQ ab und legt die Beweis-Transaktion in den Ausgang --
// in einer Transaktion. Seinen eigenen Block damit spielt er nie nach; ohne
// den Abzug bei der Erkennung fehlten ihm die 50 AEQ, die jeder andere beim
// Nachspielen abzieht. Spielt er den Block eines anderen mit demselben Paar
// nach, zieht er nicht noch einmal ab.
func TestStrafe_EntdeckerZiehtAbWieAlle_RealDB(t *testing.T) {
	k := neuerStrafKnoten(t)
	if !k.block(k.doppelsignatur(nowUnix()-5*86400, "aa", "bb")) {
		t.Fatal("erstes Vergehen abgelehnt")
	}
	zweites := k.doppelsignatur(nowUnix()-3600, "cc", "dd")
	if _, err := k.cs.db.Exec(`DELETE FROM pending_txs`); err != nil {
		t.Fatal(err)
	}
	n, betrag, err := k.cs.DoppelsignaturErkannt(k.signer, zweites.BlockAHash, zweites.BlockBHash, zweites.DetectedAt, zweites.Doppelbeweis)
	if err != nil || n != 2 || betrag != equivocationSecondOffensePenaltyAEQ {
		t.Fatalf("Erkennung: %d Vergehen, %.6f AEQ, %v", n, betrag, err)
	}
	if got := k.guthaben(); got != 100-equivocationSecondOffensePenaltyAEQ {
		t.Fatalf("nach der Erkennung %.6f statt %.0f", got, 100-equivocationSecondOffensePenaltyAEQ)
	}
	var imAusgang int
	k.cs.db.QueryRow(`SELECT COUNT(*) FROM pending_txs WHERE tx_json LIKE '%slash_equivocation%'`).Scan(&imAusgang)
	if imAusgang != 1 {
		t.Fatalf("%d Beweis-Transaktionen im Ausgang statt 1", imAusgang)
	}

	// Ein anderer Erkennender hat dasselbe Paar in seinen Block gelegt.
	if !k.block(zweites) {
		t.Fatal("Block mit der Strafe abgelehnt")
	}
	if got := k.guthaben(); got != 100-equivocationSecondOffensePenaltyAEQ {
		t.Fatalf("beim Nachspielen noch einmal abgezogen: %.6f", got)
	}
	if v := k.vergehen(); v != 2 {
		t.Fatalf("%d Vergehen statt 2", v)
	}
	if gesperrt, _ := k.cs.IsValidatorSuspended(k.signer, 0); !gesperrt {
		t.Fatal("nach dem zweiten Vergehen nicht gesperrt")
	}
}

// Gegenprobe zum Entdecker: dasselbe Paar ein zweites Mal erkannt (etwa ein
// dritter Block) bucht nichts und zaehlt nichts mehr.
func TestStrafe_ZweiteErkennungBuchtNichts_RealDB(t *testing.T) {
	k := neuerStrafKnoten(t)
	if !k.block(k.doppelsignatur(nowUnix()-5*86400, "aa", "bb")) {
		t.Fatal("erstes Vergehen abgelehnt")
	}
	zweites := k.doppelsignatur(nowUnix()-3600, "cc", "dd")
	for i := 0; i < 2; i++ {
		if _, _, err := k.cs.DoppelsignaturErkannt(k.signer, zweites.BlockAHash, zweites.BlockBHash, zweites.DetectedAt, zweites.Doppelbeweis); err != nil {
			t.Fatal(err)
		}
	}
	if got := k.guthaben(); got != 100-equivocationSecondOffensePenaltyAEQ {
		t.Fatalf("nach zweimal erkannt %.6f statt %.0f", got, 100-equivocationSecondOffensePenaltyAEQ)
	}
	if v := k.vergehen(); v != 2 {
		t.Fatalf("%d Vergehen statt 2", v)
	}
}

// Zweimal dieselbe Strafe in einem Block (zwei Erkennende, ein Produzent):
// genau einmal abgezogen, der Block geht durch. Vorher wartete der zweite
// Vermerk in seiner eigenen Transaktion auf die Zeilensperre des Blocks, bis
// statement_timeout den ganzen Block scheitern liess.
func TestStrafe_ZweimalDasselbePaarImBlock_RealDB(t *testing.T) {
	k := neuerStrafKnoten(t)
	if !k.block(k.doppelsignatur(nowUnix()-5*86400, "aa", "bb")) {
		t.Fatal("erstes Vergehen abgelehnt")
	}
	zweites := k.doppelsignatur(nowUnix()-3600, "cc", "dd")
	if !k.block(zweites, zweites) {
		t.Fatal("Block mit zweimal derselben Strafe abgelehnt")
	}
	if got := k.guthaben(); got != 100-equivocationSecondOffensePenaltyAEQ {
		t.Fatalf("nach zweimal derselben Strafe %.6f statt %.0f", got, 100-equivocationSecondOffensePenaltyAEQ)
	}
	if v := k.vergehen(); v != 2 {
		t.Fatalf("%d Vergehen statt 2", v)
	}
}
