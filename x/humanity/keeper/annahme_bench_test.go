package keeper

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// Annahmepfad ohne Netz und ohne Datenbank: ein Buendel signierter
// Ueberweisungen durch handleRPC, so wie der Pruefstand es schickt (100 je
// Buendel, verschiedene Absender). Misst die CPU je Ueberweisung, die der
// RPC-Teil der Annahme kostet -- Parsen, Signatur, Nonce, Metadaten, Antwort.
//
//	go test ./x/humanity/keeper/ -run '^$' -bench BenchmarkAnnahmeBuendel -benchmem -cpuprofile cpu.out
func BenchmarkAnnahmeBuendel(b *testing.B) {
	annahmeBuendelMessen(b, newTestState())
}

// Wie oben, aber mit dem Annahmepfad von C1: Postgres, WAL, Bloecke aus dem
// Speicher, Flush in vier Teilen. Ohne Datenbank laeuft TransferAtomic ueber
// einen Testpfad (Zustand als JSON-Datei, Menschen zaehlen per Schleife), der
// in Produktion nie vorkommt und das Profil verdeckt.
//
//	AEQUITAS_TPS_BENCH=1 DATABASE_URL=postgres://... go test ./x/humanity/keeper/ \
//	  -run '^$' -bench BenchmarkAnnahmeBuendelDB -benchmem -cpuprofile cpu.out
func BenchmarkAnnahmeBuendelDB(b *testing.B) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" || os.Getenv("DATABASE_URL") == "" {
		b.Skip("opt-in: AEQUITAS_TPS_BENCH=1 und DATABASE_URL (wegwerfbare lokale Datenbank)")
	}
	b.Setenv("AEQUITAS_WAL_ENABLED", "1")
	b.Setenv("AEQUITAS_WAL_PATH", filepath.Join(b.TempDir(), "transfer.wal"))
	b.Setenv(speicherKorbEnv, "1")
	b.Setenv(walFlushTeileEnv, "4")
	cs := NewChainState("unused-annahme-bench.json")
	if !cs.useDB || cs.wal == nil {
		b.Fatal("Datenbank oder WAL nicht aktiv")
	}
	b.Cleanup(func() {
		cs.stopWALFlushWorkerForTest()
		_ = cs.wal.Close()
		cs.db.Close()
	})
	// Echte Blockerzeugung im Takt, wie auf dem Knoten: sonst greifen
	// Rueckstau- und Produktionsgrenze (admission_control.go) und weisen ab.
	dag := NewBlockchain("annahme-bench", cs)
	stopp := make(chan struct{})
	fertig := make(chan struct{})
	go func() {
		defer close(fertig)
		tick := time.NewTicker(200 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stopp:
				return
			case <-tick.C:
				dag.ProduceBlocksForTick()
			}
		}
	}()
	b.Cleanup(func() { close(stopp); <-fertig })
	time.Sleep(time.Second)
	annahmeBuendelMessenMit(b, cs, dag)
}

func annahmeBuendelMessen(b *testing.B, cs *ChainState) {
	annahmeBuendelMessenMit(b, cs, &BlockDAG{state: cs})
}

func annahmeBuendelMessenMit(b *testing.B, cs *ChainState, dag *BlockDAG) {
	const absender = 100
	// Wie auf C1 (AEQUITAS_RPC_QUIET_TX=1): keine Logzeile je Ueberweisung.
	altLeise := rpcQuietTx
	rpcQuietTx = true
	b.Cleanup(func() { rpcQuietTx = altLeise })
	inflightZuruecksetzen()
	b.Cleanup(inflightZuruecksetzen)
	// Wie auf C1 (seit 02.10.2026 hier): Wirtschaftsregeln und signierte
	// Ueberweisungen (Stufe 1.0) sind aktiv. TestMain schaltet beide fuer alle
	// anderen Tests ab (wirtschaft_main_test.go); ohne das hier mass dieser
	// Benchmark eine Annahme ohne Gebuehrenregeln und ohne Buchfuehrung.
	wirtschaftAktivOverride.Store(1)
	signierteUeberweisungenOverride.Store(1)
	b.Cleanup(func() {
		wirtschaftAktivOverride.Store(math.MaxInt64)
		signierteUeberweisungenOverride.Store(math.MaxInt64)
	})
	// Wie auf dem Pruefstand: der Lastgenerator ist freigestellt (nur die
	// Ratenbegrenzung je IP; Inflight- und Rueckstaugrenze gelten weiter).
	altFrei := rpcRateLimitFreiListe.Load()
	frei := map[string]bool{"192.0.2.1": true} // RemoteAddr von httptest
	rpcRateLimitFreiListe.Store(&frei)
	b.Cleanup(func() { rpcRateLimitFreiListe.Store(altFrei) })

	srv := NewEVMRPCServer(dag, cs)
	schluessel := make([]*ecdsa.PrivateKey, absender)
	for i := range schluessel {
		k, err := crypto.GenerateKey()
		if err != nil {
			b.Fatal(err)
		}
		schluessel[i] = k
		addr := strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
		// Freie Adressen duerfen hoechstens 250 AEQ halten
		// (pruefeEmpfaengerWirtschaft) -- also 200, wie im Pruefstand.
		acc := &AccountState{Address: addr, Balance: NewDecimal(200), LastActivityAt: nowUnix()}
		if cs.useDB {
			cs.mu.Lock()
			err = cs.saveAccountToDB(acc)
			cs.mu.Unlock()
			if err != nil {
				b.Fatal(err)
			}
		} else {
			cs.accounts.Set(addr, acc)
		}
	}
	signer := types.NewEIP155Signer(big.NewInt(1926))
	wert := new(big.Int).Exp(big.NewInt(10), big.NewInt(15), nil) // 0,001 AEQ

	buendel := func(nonce uint64) []byte {
		var sb strings.Builder
		sb.WriteByte('[')
		for i, k := range schluessel {
			// Im Kreis an den naechsten Absender wie der Lastgenerator auf C1
			// -- nicht an eine System-Adresse, die mit den Wirtschaftsregeln
			// jede Ueberweisung auf den langsamen Weg schickt.
			an := crypto.PubkeyToAddress(schluessel[(i+1)%len(schluessel)].PublicKey)
			tx, err := types.SignTx(types.NewTransaction(nonce, an, wert, 21000, big.NewInt(0), nil), signer, k)
			if err != nil {
				b.Fatal(err)
			}
			raw, _ := tx.MarshalBinary()
			if i > 0 {
				sb.WriteByte(',')
			}
			fmt.Fprintf(&sb, `{"jsonrpc":"2.0","id":%d,"method":"eth_sendRawTransaction","params":["0x%s"]}`, i, hex.EncodeToString(raw))
		}
		sb.WriteByte(']')
		return []byte(sb.String())
	}

	// Gutfall vorab: alle Posten angenommen.
	if w := annahmeAufruf(srv, buendel(0)); strings.Contains(w, `"error"`) {
		b.Fatalf("Gutfall abgelehnt: %.300s", w)
	}

	// Alle Buendel vorab signieren: das Signieren gehoert dem Absender, nicht
	// dem Knoten, und wuerde sonst im CPU-Profil mitgezaehlt.
	alle := make([][]byte, b.N)
	for n := range alle {
		alle[n] = buendel(uint64(n + 1))
	}
	b.ReportAllocs()
	abgelehnt := 0
	verfehltVorher := absenderVerfehlt.Load()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		w := annahmeAufruf(srv, alle[n])
		alle[n] = nil
		b.StopTimer()
		abgelehnt += strings.Count(w, `"error"`)
		b.StartTimer()
	}
	b.StopTimer()
	// Abgelehnte Posten sind billiger als angenommene -- ohne diese Zahl
	// waere ein schnellerer Lauf nicht von einem abweisenden zu unterscheiden.
	b.ReportMetric(float64(abgelehnt)/float64(b.N*absender), "abgelehnt/tx")
	// Signatur-Wiederherstellungen je Ueberweisung (Absender-Cache verfehlt):
	// mehr als 1 hiesse, dieselbe Signatur wird doppelt gerechnet.
	b.ReportMetric(float64(absenderVerfehlt.Load()-verfehltVorher)/float64(b.N*absender), "recover/tx")
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*absender)/1000, "us/tx")
}

func annahmeAufruf(srv *EVMRPCServer, body []byte) string {
	req := httptest.NewRequest("POST", "/rpc", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleRPC(w, req)
	return w.Body.String()
}

func addrFromHexForBench(s string) [20]byte {
	var out [20]byte
	b, _ := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	copy(out[:], b)
	return out
}
