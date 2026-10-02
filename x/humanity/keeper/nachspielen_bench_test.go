package keeper

import (
	"crypto/ecdsa"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// WAS BRAUCHT EIN VALIDATOR OHNE LEITERROLLE?
//
// Der Leiter nimmt an und baut Bloecke (BenchmarkAnnahmeBuendelDB, Pruefstand
// auf C1). Jeder andere Validator bekommt die fertigen Bloecke und rechnet sie
// selbst nach: Signaturen, Nonces, Deckung, Gebuehren, StateRoot, Speichern.
// Wie viel CPU das je Ueberweisung kostet und ob mehr Kerne helfen, war nie
// gemessen -- C2 ist aus, und die Mindestausstattung (8 Kerne) stammt aus der
// Leiterrolle (leistungsnachweis.go).
//
// Hier, in einem Prozess: Knoten A (eigene Datenbank) nimmt Buendel an und
// erzeugt Bloecke wie auf C1. Knoten B (eigene Datenbank, dieselben
// Anfangskonten) bekommt die Bloecke so, wie sie ueber das Netz kommen (JSON,
// AddPeerBlock) und spielt sie nach. Gemessen wird NUR das Nachspielen:
// Wandzeit und Prozess-CPU je Ueberweisung, daraus die Kerne im Mittel. Mit
// -cpu 2,4,8 zeigt sich, ob mehr Kerne das Nachspielen schneller machen.
//
// Zwei Faelle, weil das Nachspielen sie verschieden behandelt
// (replay_parallel.go, collectDisjointTransferBatch):
//
//	gebuehr  -- freie Adressen, jede Ueberweisung traegt 0,1 % Gebuehr:
//	            der serielle Pfad.
//	mensch   -- registrierte Menschen im Monatsfreibetrag (gebuehrMitWirtschaft):
//	            keine Gebuehr, der parallele Pfad.
//
// Jede Signatur stellt B selbst wieder her (pruefeUeberweisungenImBlock, ohne
// Zwischenspeicher). Der Absender-Cache des Prozesses (absender_cache.go) wird
// trotzdem vor dem Nachspielen geleert, damit nichts von A bei B ankommt.
//
//	AEQUITAS_TPS_BENCH=1 DATABASE_URL=postgres://.../irgendeine \
//	  go test ./x/humanity/keeper/ -run '^$' -bench BenchmarkNachspielenDB \
//	  -benchtime 25x -cpu 2,4,8
//
// DATABASE_URL zeigt auf einen wegwerfbaren Server: der Benchmark legt je
// Lauf zwei frische Datenbanken an und loescht sie danach wieder.
func BenchmarkNachspielenDB(b *testing.B) {
	if os.Getenv("AEQUITAS_TPS_BENCH") != "1" || os.Getenv("DATABASE_URL") == "" {
		b.Skip("opt-in: AEQUITAS_TPS_BENCH=1 und DATABASE_URL (wegwerfbarer Postgres-Server)")
	}
	b.Run("gebuehr", func(b *testing.B) { nachspielenMessen(b, false) })
	b.Run("mensch", func(b *testing.B) { nachspielenMessen(b, true) })
}

// frischeDatenbank legt auf dem Server von basis eine leere Datenbank an und
// gibt ihre URL zurueck; sie wird mit dem Benchmark wieder geloescht.
func frischeDatenbank(b *testing.B, basis, name string) string {
	b.Helper()
	u, err := url.Parse(basis)
	if err != nil {
		b.Fatal(err)
	}
	admin, err := sql.Open("postgres", basis)
	if err != nil {
		b.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Exec("DROP DATABASE IF EXISTS " + name); err != nil {
		b.Fatal(err)
	}
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if a, err := sql.Open("postgres", basis); err == nil {
			_, _ = a.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
			a.Close()
		}
	})
	u.Path = "/" + name
	return u.String()
}

func schluesselHex(k *ecdsa.PrivateKey) string {
	return "0x" + hex.EncodeToString(crypto.FromECDSA(k))
}

func prozessCPU() time.Duration {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

func nachspielenMessen(b *testing.B, mensch bool) {
	// 16 Gruppen zu je 100 Absendern senden gleichzeitig, damit Bloecke mit
	// tausenden Ueberweisungen entstehen wie auf C1 (Lauf 20: im Mittel 6.707
	// je Block). b.N Buendel je Gruppe.
	const gruppen = 16
	const absender = 100 * gruppen
	basis := os.Getenv("DATABASE_URL")
	lauf := fmt.Sprintf("nachspielen_%d", time.Now().UnixNano()%1_000_000_000)
	dbA := frischeDatenbank(b, basis, lauf+"_a")
	dbB := frischeDatenbank(b, basis, lauf+"_b")

	// Wie auf C1: Wirtschaftsregeln und signierte Ueberweisungen (Stufe 1.0)
	// sind aktiv. TestMain schaltet beide fuer alle anderen Tests ab
	// (wirtschaft_main_test.go) -- ohne das hier pruefte B keine Signatur und
	// jede Ueberweisung truege die alte Gebuehr.
	wirtschaftAktivOverride.Store(1)
	signierteUeberweisungenOverride.Store(1)
	b.Cleanup(func() {
		wirtschaftAktivOverride.Store(math.MaxInt64)
		signierteUeberweisungenOverride.Store(math.MaxInt64)
	})

	altLeise := rpcQuietTx
	rpcQuietTx = true
	b.Cleanup(func() { rpcQuietTx = altLeise })
	inflightZuruecksetzen()
	b.Cleanup(inflightZuruecksetzen)
	altFrei := rpcRateLimitFreiListe.Load()
	frei := map[string]bool{"192.0.2.1": true} // RemoteAddr von httptest
	rpcRateLimitFreiListe.Store(&frei)
	b.Cleanup(func() { rpcRateLimitFreiListe.Store(altFrei) })

	// ── Knoten A: der Leiter, wie auf C1 ────────────────────────────────
	leiterKey, _ := crypto.GenerateKey()
	folgerKey, _ := crypto.GenerateKey()
	leiterAdr := strings.ToLower(crypto.PubkeyToAddress(leiterKey.PublicKey).Hex())
	b.Setenv("AEQUITAS_WAL_ENABLED", "1")
	b.Setenv(speicherKorbEnv, "1")
	b.Setenv(walFlushTeileEnv, "4")
	b.Setenv("AUTHORIZED_VALIDATORS", leiterAdr)
	b.Setenv("BOOTSTRAP_SIGNER", leiterAdr)
	b.Setenv("DATABASE_URL", dbA)
	b.Setenv("RELAYER_PRIVATE_KEY", schluesselHex(leiterKey))
	b.Setenv("AEQUITAS_WAL_PATH", filepath.Join(b.TempDir(), "leiter.wal"))
	csA := NewChainState("unused-nachspielen-a.json")
	if !csA.useDB || csA.wal == nil {
		b.Fatal("Leiter: Datenbank oder WAL nicht aktiv")
	}
	dagA := NewBlockchain("nachspielen-leiter", csA)
	srvA := NewEVMRPCServer(dagA, csA)

	schluessel := make([]*ecdsa.PrivateKey, absender)
	konten := make([]AccountState, absender)
	jetzt := nowUnix()
	for i := range schluessel {
		k, _ := crypto.GenerateKey()
		schluessel[i] = k
		konten[i] = AccountState{
			Address:        strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex()),
			Balance:        NewDecimal(200), // freie Adressen duerfen hoechstens 250 halten
			LastActivityAt: jetzt,
			IsHuman:        mensch,
		}
	}
	anlegen := func(cs *ChainState) {
		for i := range konten {
			acc := konten[i]
			cs.mu.Lock()
			err := cs.saveAccountToDB(&acc)
			cs.mu.Unlock()
			if err != nil {
				b.Fatal(err)
			}
		}
	}
	anlegen(csA)

	stopp := make(chan struct{})
	fertig := make(chan struct{})
	go func() {
		defer close(fertig)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stopp:
				return
			case <-tick.C:
				dagA.ProduceBlocksForTick()
			}
		}
	}()
	leiterAnhalten := func() {
		select {
		case <-stopp:
		default:
			close(stopp)
			<-fertig
		}
	}
	b.Cleanup(func() {
		leiterAnhalten()
		csA.stopWALFlushWorkerForTest()
		_ = csA.wal.Close()
		csA.db.Close()
	})
	time.Sleep(time.Second)

	signer := types.NewEIP155Signer(big.NewInt(1926))
	wert := new(big.Int).Exp(big.NewInt(10), big.NewInt(15), nil) // 0,001 AEQ
	buendel := func(g int, nonce uint64) []byte {
		var sb strings.Builder
		sb.WriteByte('[')
		gruppe := schluessel[g*100 : (g+1)*100]
		for i, k := range gruppe {
			// Im Kreis an den naechsten der Gruppe: Mensch an Mensch bzw.
			// freie Adresse an freie Adresse (keine System-Adresse, die
			// jeden Schnellpfad abschaltet).
			an := crypto.PubkeyToAddress(gruppe[(i+1)%len(gruppe)].PublicKey)
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

	// Annehmen: b.N Buendel. Abgelehnte Posten (Rueckstau, Inflight) werden
	// mit derselben Nonce wiederholt, bis jede Nonce eines Absenders sitzt --
	// sonst stuenden Luecken in den Bloecken und das Nachspielen saehe nur
	// einen Teil.
	// Alle Buendel vorab signieren: das Signieren gehoert den Absendern.
	alle := make([][][]byte, gruppen)
	for g := range alle {
		alle[g] = make([][]byte, b.N)
		for n := range alle[g] {
			alle[g][n] = buendel(g, uint64(n))
		}
	}
	fehlerKanal := make(chan string, gruppen)
	var wg sync.WaitGroup
	for g := 0; g < gruppen; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for n := 0; n < b.N; n++ {
				for versuch := 0; ; versuch++ {
					w := annahmeAufruf(srvA, alle[g][n])
					// Sitzt eine Nonce schon (frueherer Versuch), meldet der
					// Knoten "nonce too low" -- das zaehlt als angenommen.
					if strings.Count(w, `"error"`) == strings.Count(w, "nonce too low") {
						break
					}
					if !strings.Contains(w, "-32005") || versuch >= 600 {
						fehlerKanal <- fmt.Sprintf("Gruppe %d, Buendel %d (Versuch %d): %.300s", g, n, versuch, w)
						return
					}
					time.Sleep(50 * time.Millisecond)
				}
			}
		}(g)
	}
	wg.Wait()
	close(fehlerKanal)
	for f := range fehlerKanal {
		b.Fatal(f)
	}
	angenommen := gruppen * 100 * b.N

	// Warten, bis jede angenommene Ueberweisung in einem Block steht.
	frist := time.Now().Add(2 * time.Minute)
	var bloecke []*Block
	for {
		bloecke = bloecke[:0]
		inBloecken := 0
		for _, blk := range dagA.GetBlocks() {
			if blk.Height == 0 {
				continue
			}
			bloecke = append(bloecke, blk)
			for _, tx := range blk.Transactions {
				if tx.Type == "transfer" {
					inBloecken++
				}
			}
		}
		if inBloecken >= angenommen {
			break
		}
		if time.Now().After(frist) {
			b.Fatalf("nach 2 Minuten erst %d von %d Ueberweisungen in Bloecken", inBloecken, angenommen)
		}
		time.Sleep(200 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond) // letzter Block gespeichert
	leiterAnhalten()
	sort.Slice(bloecke, func(i, j int) bool {
		if bloecke[i].Height != bloecke[j].Height {
			return bloecke[i].Height < bloecke[j].Height
		}
		return bloecke[i].Hash < bloecke[j].Hash
	})
	// Wie ueber das Netz: als JSON, damit B nichts mit A teilt.
	draht := make([][]byte, len(bloecke))
	ueberweisungen := 0
	for i, blk := range bloecke {
		d, err := json.Marshal(blk)
		if err != nil {
			b.Fatal(err)
		}
		draht[i] = d
		for _, tx := range blk.Transactions {
			if tx.Type == "transfer" {
				ueberweisungen++
			}
		}
	}
	stateRootA := csA.StateRoot()

	// ── Knoten B: der Validator ohne Leiterrolle ─────────────────────────
	os.Setenv("DATABASE_URL", dbB)
	os.Setenv("RELAYER_PRIVATE_KEY", schluesselHex(folgerKey))
	os.Setenv("AEQUITAS_WAL_PATH", filepath.Join(b.TempDir(), "folger.wal"))
	csB := NewChainState("unused-nachspielen-b.json")
	if !csB.useDB {
		b.Fatal("Folger: Datenbank nicht aktiv")
	}
	dagB := NewBlockchain("nachspielen-folger", csB)
	_ = NewEVMRPCServer(dagB, csB)
	b.Cleanup(func() {
		if csB.wal != nil {
			csB.stopWALFlushWorkerForTest()
			_ = csB.wal.Close()
		}
		csB.db.Close()
	})
	anlegen(csB)
	empfangen := make([]*Block, len(draht))
	for i, d := range draht {
		var blk Block
		if err := json.Unmarshal(d, &blk); err != nil {
			b.Fatal(err)
		}
		empfangen[i] = &blk
	}
	absenderSpeicher = neuerAbsenderCacheVerteilt()
	ReplayPhasenZuruecksetzen()

	// ── Gemessen: nur das Nachspielen ────────────────────────────────────
	b.ResetTimer()
	cpuVorher := prozessCPU()
	start := time.Now()
	for i, blk := range empfangen {
		if !dagB.AddPeerBlock(blk) {
			b.StopTimer()
			b.Fatalf("Folger lehnt Block %d (Hoehe %d, %d tx) ab", i, blk.Height, len(blk.Transactions))
		}
	}
	wand := time.Since(start)
	cpu := prozessCPU() - cpuVorher
	b.StopTimer()

	if got := csB.StateRoot(); got != stateRootA {
		b.Fatalf("StateRoot nach dem Nachspielen: Folger %s, Leiter %s", got, stateRootA)
	}
	if ueberweisungen == 0 {
		b.Fatal("keine Ueberweisungen in den Bloecken")
	}
	// Wo die Zeit des Nachspielens bleibt (replay_phasen_stats.go).
	if phasen, err := json.Marshal(ReplayPhasenStand()); err == nil {
		b.Logf("replay_phasen %s", phasen)
	}
	if pfad, err := json.Marshal(ReplayPfadStand()); err == nil {
		b.Logf("replay_pfad %s", pfad)
	}
	gebuehren := 0
	for _, blk := range empfangen {
		for _, tx := range blk.Transactions {
			if tx.Type == "transfer" && tx.Gebuehr != 0 {
				gebuehren++
			}
		}
	}
	b.Logf("ueberweisungen mit gebuehr: %d von %d", gebuehren, ueberweisungen)
	b.ReportMetric(float64(wand.Microseconds())/float64(ueberweisungen), "us/tx")
	b.ReportMetric(float64(cpu.Microseconds())/float64(ueberweisungen), "cpu-us/tx")
	b.ReportMetric(cpu.Seconds()/wand.Seconds(), "kerne")
	b.ReportMetric(float64(ueberweisungen)/wand.Seconds(), "tx/s")
	b.ReportMetric(float64(ueberweisungen)/float64(len(empfangen)), "tx/block")
	b.ReportMetric(float64(len(empfangen)), "bloecke")
}
