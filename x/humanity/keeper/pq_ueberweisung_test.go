package keeper

import (
	"bytes"
	"crypto/mldsa"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Stufe 3 (pq_ueberweisung.go): quantensichere Ueberweisungen. Gutfall,
// Wiederholung, und jede Faelschung, die ein Erzeuger in einen Block
// schreiben koennte -- jeder Knoten prueft selbst, ein Verstoss macht den
// ganzen Block ungueltig.

type pqKontoSchluessel struct {
	sk   *mldsa.PrivateKey
	pub  string
	addr string
}

func neuerPQKontoSchluessel(t *testing.T, seedByte byte) pqKontoSchluessel {
	t.Helper()
	sk, err := mldsa.NewPrivateKey(mldsa.MLDSA65(), bytes.Repeat([]byte{seedByte}, 32))
	if err != nil {
		t.Fatal(err)
	}
	pub := sk.PublicKey().Bytes()
	return pqKontoSchluessel{sk: sk, pub: hex.EncodeToString(pub), addr: pqAdresseAusSchluessel(pub)}
}

func pqUnterschreibe(t *testing.T, k pqKontoSchluessel, msg string) string {
	t.Helper()
	sig, err := k.sk.SignDeterministic([]byte(msg), nil)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(sig)
}

// pqTransfer: eine vollstaendig gueltige PQ-Ueberweisung.
func pqTransfer(t *testing.T, von pqKontoSchluessel, an string, wei *big.Int, nonce uint64) Transaction {
	t.Helper()
	w := wei.String()
	msg := pqUeberweisungNachricht(von.addr, an, w, nonce)
	return Transaction{Type: "transfer", Wallet: von.addr, To: an, Amount: betragAusWei(wei), TxHash: pqUeberweisungTxHash(msg),
		PQ: &PQUeberweisung{Pub: von.pub, Sig: pqUnterschreibe(t, von, msg), Nonce: nonce, Wei: w}}
}

func pqAktivFuerTest(t *testing.T) {
	t.Helper()
	vorher := pqUeberweisungAbOverride.Load()
	pqUeberweisungAbOverride.Store(1)
	t.Cleanup(func() { pqUeberweisungAbOverride.Store(vorher) })
}

func TestPQUeberweisung_NachspielenPrueftUndSetztNonce(t *testing.T) {
	pqAktivFuerTest(t)
	a := neuerPQKontoSchluessel(t, 0x11)
	b := neuerTestSchluessel(t)
	dag, cs := nachspielKnoten(t, map[string]float64{a.addr: 1000})

	tx := pqTransfer(t, a, b.addr, aeqWei(10), 0)
	if ok := dag.replayTransactions(testBlock(1, tx), true); !ok {
		t.Fatal("gueltige PQ-Ueberweisung abgelehnt")
	}
	if got := kontoVon(t, cs, a.addr).NaechsteNonce; got != 1 {
		t.Fatalf("NaechsteNonce %d statt 1", got)
	}
	if got := kontoVon(t, cs, b.addr).Balance.Float(); got != 10 {
		t.Fatalf("Empfaenger hat %.6f statt 10", got)
	}
	// Wiederholung derselben Ueberweisung in einem neuen Block.
	if ok := dag.replayTransactions(testBlock(2, tx), true); ok {
		t.Fatal("wiederholte PQ-Ueberweisung (Nonce verbraucht) angenommen")
	}
	if got := kontoVon(t, cs, b.addr).Balance.Float(); got != 10 {
		t.Fatalf("nach der Wiederholung hat der Empfaenger %.6f", got)
	}
}

func TestPQUeberweisung_FaelschungenMachenDenBlockUngueltig(t *testing.T) {
	pqAktivFuerTest(t)
	opfer := neuerPQKontoSchluessel(t, 0x21)
	angreifer := neuerPQKontoSchluessel(t, 0x22)
	ziel := neuerTestSchluessel(t)
	gut := func() Transaction { return pqTransfer(t, opfer, ziel.addr, aeqWei(100), 0) }

	falle := map[string]func() Transaction{
		"Unterschrift des Angreifers mit Schluessel des Opfers": func() Transaction {
			tx := gut()
			tx.PQ.Sig = pqUnterschreibe(t, angreifer, pqUeberweisungNachricht(opfer.addr, ziel.addr, tx.PQ.Wei, 0))
			return tx
		},
		"Schluessel des Angreifers, Absender Opfer": func() Transaction {
			tx := pqTransfer(t, angreifer, ziel.addr, aeqWei(100), 0)
			tx.Wallet = opfer.addr
			return tx
		},
		"Empfaenger nach dem Unterschreiben getauscht": func() Transaction {
			tx := gut()
			tx.To = angreifer.addr
			return tx
		},
		"Betrag im Block erhoeht": func() Transaction {
			tx := gut()
			tx.Amount = 900
			return tx
		},
		"Wei erhoeht, Betrag passend": func() Transaction {
			tx := gut()
			tx.PQ.Wei = aeqWei(900).String()
			tx.Amount = 900
			return tx
		},
		"Nonce geaendert": func() Transaction {
			tx := gut()
			tx.PQ.Nonce = 7
			return tx
		},
		"TxHash geaendert": func() Transaction {
			tx := gut()
			tx.TxHash = "0x" + strings.Repeat("ab", 32)
			return tx
		},
		"Rohform und PQ zugleich": func() Transaction {
			tx := gut()
			tx.Roh = "0x02"
			return tx
		},
		"Wei mit fuehrender Null": func() Transaction {
			tx := gut()
			tx.PQ.Wei = "0" + tx.PQ.Wei
			return tx
		},
		"Schluessel gross geschrieben": func() Transaction {
			tx := gut()
			tx.PQ.Pub = strings.ToUpper(tx.PQ.Pub)
			return tx
		},
		"Unterschrift abgeschnitten": func() Transaction {
			tx := gut()
			tx.PQ.Sig = tx.PQ.Sig[:len(tx.PQ.Sig)-2]
			return tx
		},
		"Unterschrift ein Bit falsch": func() Transaction {
			tx := gut()
			b := []byte(tx.PQ.Sig)
			if b[10] == 'a' {
				b[10] = 'b'
			} else {
				b[10] = 'a'
			}
			tx.PQ.Sig = string(b)
			return tx
		},
		"Absender gross geschrieben": func() Transaction {
			tx := gut()
			tx.Wallet = strings.ToUpper(tx.Wallet[:2]) + strings.ToUpper(tx.Wallet[2:])
			return tx
		},
	}
	for name, mach := range falle {
		dag, cs := nachspielKnoten(t, map[string]float64{opfer.addr: 1000})
		if ok := dag.replayTransactions(testBlock(1, mach()), true); ok {
			t.Errorf("%s: Block angenommen", name)
			continue
		}
		if got := kontoVon(t, cs, opfer.addr).Balance.Float(); got != 1000 {
			t.Errorf("%s: Opfer hat %.6f statt 1000", name, got)
		}
		if got := kontoVon(t, cs, angreifer.addr).Balance.Float() + kontoVon(t, cs, ziel.addr).Balance.Float(); got != 0 {
			t.Errorf("%s: Geld ist geflossen (%.6f)", name, got)
		}
	}
}

// Ein secp256k1-Schluessel kann von einer PQ-Adresse nichts ausgeben, und ein
// PQ-Nachweis gehoert an keine andere Art.
func TestPQUeberweisung_KeinAndererWegVonDerPQAdresse(t *testing.T) {
	pqAktivFuerTest(t)
	wechselAb(t, math.MaxInt64)
	pq := neuerPQKontoSchluessel(t, 0x31)
	dieb := neuerTestSchluessel(t)
	dag, cs := nachspielKnoten(t, map[string]float64{pq.addr: 1000})
	roh := signiere(t, dieb, dieb.addr, aeqWei(500), 0, 1926)
	roh.Wallet = pq.addr
	if ok := dag.replayTransactions(testBlock(1, roh), true); ok {
		t.Fatal("EVM-Rohform hat von einer PQ-Adresse ausgegeben")
	}
	if got := kontoVon(t, cs, pq.addr).Balance.Float(); got != 1000 {
		t.Fatalf("PQ-Adresse hat %.6f statt 1000", got)
	}

	dag2, _ := nachspielKnoten(t, map[string]float64{pq.addr: 1000})
	falscheArt := pqTransfer(t, pq, dieb.addr, aeqWei(1), 0)
	falscheArt.Type = "faucet"
	if ok := dag2.replayTransactions(testBlock(1, falscheArt), true); ok {
		t.Fatal("PQ-Nachweis an einer anderen Art angenommen")
	}
}

func TestPQUeberweisung_VorDemStichtagUngueltig(t *testing.T) {
	vorher := pqUeberweisungAbOverride.Load()
	pqUeberweisungAbOverride.Store(math.MaxInt64 - 1)
	defer pqUeberweisungAbOverride.Store(vorher)
	a := neuerPQKontoSchluessel(t, 0x41)
	b := neuerTestSchluessel(t)
	dag, cs := nachspielKnoten(t, map[string]float64{a.addr: 1000})
	if ok := dag.replayTransactions(testBlock(1, pqTransfer(t, a, b.addr, aeqWei(1), 0)), true); ok {
		t.Fatal("PQ-Ueberweisung vor dem Stichtag angenommen")
	}
	if got := kontoVon(t, cs, b.addr).Balance.Float(); got != 0 {
		t.Fatalf("Empfaenger hat %.6f", got)
	}
}

func TestPQUeberweisung_NoncenImBlockMuessenSteigen(t *testing.T) {
	pqAktivFuerTest(t)
	a := neuerPQKontoSchluessel(t, 0x51)
	b := neuerTestSchluessel(t)
	dag, _ := nachspielKnoten(t, map[string]float64{a.addr: 1000})
	if ok := dag.replayTransactions(testBlock(1, pqTransfer(t, a, b.addr, aeqWei(1), 5), pqTransfer(t, a, b.addr, aeqWei(1), 3)), true); ok {
		t.Fatal("fallende Nonce angenommen")
	}
	dag2, cs2 := nachspielKnoten(t, map[string]float64{a.addr: 1000})
	if ok := dag2.replayTransactions(testBlock(1, pqTransfer(t, a, b.addr, aeqWei(1), 5), pqTransfer(t, a, b.addr, aeqWei(1), 6)), true); !ok {
		t.Fatal("steigende Noncen abgelehnt")
	}
	if got := kontoVon(t, cs2, a.addr).NaechsteNonce; got != 7 {
		t.Fatalf("NaechsteNonce %d statt 7", got)
	}
}

// Annahme (TransferAtomic) und Nachspielen setzen dieselbe Nonce und
// dieselben Kontostaende.
func TestPQUeberweisung_AnnahmeGleichNachspielen(t *testing.T) {
	pqAktivFuerTest(t)
	signierteUeberweisungenOverride.Store(1)
	t.Cleanup(func() { signierteUeberweisungenOverride.Store(math.MaxInt64) })
	a := neuerPQKontoSchluessel(t, 0x61)
	b := neuerTestSchluessel(t)
	tx := pqTransfer(t, a, b.addr, aeqWei(10), 4)

	annehmer := newTestState()
	annehmer.mu.Lock()
	annehmer.accounts.Set(a.addr, &AccountState{Address: a.addr, Balance: NewDecimal(1000)})
	annehmer.mu.Unlock()
	if _, _, err := annehmer.TransferAtomic(a.addr, b.addr, tx.Amount, tx); err != nil {
		t.Fatalf("Annahme: %v", err)
	}
	if got := kontoVon(t, annehmer, a.addr).NaechsteNonce; got != 5 {
		t.Fatalf("Annahme: NaechsteNonce %d statt 5", got)
	}
	if _, _, err := annehmer.TransferAtomic(a.addr, b.addr, tx.Amount, tx); err == nil {
		t.Fatal("Annahme nimmt dieselbe Nonce zweimal an")
	}

	dag, nachspieler := nachspielKnoten(t, map[string]float64{a.addr: 1000})
	if ok := dag.replayTransactions(testBlock(1, tx), true); !ok {
		t.Fatal("Nachspielen lehnt die angenommene PQ-Ueberweisung ab")
	}
	if got := kontoVon(t, nachspieler, a.addr).NaechsteNonce; got != 5 {
		t.Fatalf("Nachspielen: NaechsteNonce %d statt 5", got)
	}
	if x, y := kontoVon(t, annehmer, b.addr).Balance.Float(), kontoVon(t, nachspieler, b.addr).Balance.Float(); x != y {
		t.Fatalf("Empfaenger: Annahme %.9f, Nachspielen %.9f", x, y)
	}
}

func TestPQUeberweisung_Endpunkt(t *testing.T) {
	a := neuerPQKontoSchluessel(t, 0x71)
	b := neuerTestSchluessel(t)
	cs := newTestState()
	cs.mu.Lock()
	cs.accounts.Set(a.addr, &AccountState{Address: a.addr, Balance: NewDecimal(1000)})
	cs.mu.Unlock()
	api := &APIServer{state: cs}
	tx := pqTransfer(t, a, b.addr, aeqWei(10), 0)
	post := func(body map[string]interface{}) int {
		roh, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		api.handlePQUeberweisung(w, httptest.NewRequest(http.MethodPost, "/api/pq/ueberweisung", bytes.NewReader(roh)))
		return w.Code
	}
	body := map[string]interface{}{"von": a.addr, "an": b.addr, "wei": tx.PQ.Wei, "nonce": 0, "pub": a.pub, "sig": tx.PQ.Sig}

	vorher := pqUeberweisungAbOverride.Load()
	pqUeberweisungAbOverride.Store(math.MaxInt64 - 1)
	if c := post(body); c != http.StatusConflict {
		t.Fatalf("vor dem Stichtag: %d", c)
	}
	pqUeberweisungAbOverride.Store(1)
	t.Cleanup(func() { pqUeberweisungAbOverride.Store(vorher) })

	falsch := map[string]interface{}{}
	for k, v := range body {
		falsch[k] = v
	}
	falsch["an"] = a.addr
	if c := post(falsch); c != http.StatusBadRequest {
		t.Fatalf("an sich selbst: %d", c)
	}
	falsch["an"] = neuerTestSchluessel(t).addr
	if c := post(falsch); c != http.StatusForbidden {
		t.Fatalf("anderer Empfaenger als unterschrieben: %d", c)
	}
	falsch["an"], falsch["wei"] = b.addr, "abc"
	if c := post(falsch); c != http.StatusBadRequest {
		t.Fatalf("Unsinn als Betrag: %d", c)
	}
	falsch["wei"], falsch["extra"] = tx.PQ.Wei, 1
	if c := post(falsch); c != http.StatusBadRequest {
		t.Fatalf("unbekanntes Feld: %d", c)
	}
	if c := post(body); c != http.StatusOK {
		t.Fatalf("gueltige PQ-Ueberweisung: %d", c)
	}
	if c := post(body); c == http.StatusOK {
		t.Fatal("Wiederholung angenommen")
	}
	if got := cs.GetBalance(b.addr); got != 10 {
		t.Fatalf("Empfaenger hat %.6f statt 10", got)
	}
}

// Pruefvektor, byte-gleich in der App (lib/__tests__/pq.test.ts): Seed 0x11*32.
func TestPQUeberweisung_PruefvektorFuerDieApp(t *testing.T) {
	a := neuerPQKontoSchluessel(t, 0x11)
	if a.addr != pqAdresseAusSchluessel(func() []byte { b, _ := hex.DecodeString(a.pub); return b }()) {
		t.Fatal("Adresse nicht reproduzierbar")
	}
	// Gegengerechnet mit Python cryptography (MLDSA65 from_seed_bytes).
	if a.addr != "0x6b2b299810ed00feb61d8a1cc5b91b5caa93926b" {
		t.Fatalf("Adresse zu Seed 0x11*32: %s", a.addr)
	}
	if got, want := pqUeberweisungNachricht("0xa", "0xb", "5", 7), "aequitas-pq-ueberweisung-v1|chain:1926|0xa|0xb|5|7"; got != want {
		t.Fatalf("%q != %q", got, want)
	}
}

// Ein Block traegt hoechstens pqUeberweisungenJeBlock PQ-Ueberweisungen:
// der Erzeuger schneidet in der Reihenfolge ab, jeder Knoten weist mehr ab.
func TestPQUeberweisung_HoechstensJeBlock(t *testing.T) {
	pqAktivFuerTest(t)
	a := neuerPQKontoSchluessel(t, 0x81)
	b := neuerTestSchluessel(t)
	txs := make([]Transaction, 0, pqUeberweisungenJeBlock+2)
	txs = append(txs, Transaction{Type: "faucet", Wallet: b.addr})
	for i := 0; i <= pqUeberweisungenJeBlock; i++ {
		txs = append(txs, pqTransfer(t, a, b.addr, aeqWei(1), uint64(i)))
	}
	if n := pqUeberweisungGrenze(txs); n != pqUeberweisungenJeBlock+1 {
		t.Fatalf("Grenze %d, erwartet %d (die 501. PQ-Ueberweisung in den naechsten Block)", n, pqUeberweisungenJeBlock+1)
	}
	if n := pqUeberweisungGrenze(txs[:pqUeberweisungenJeBlock+1]); n != pqUeberweisungenJeBlock+1 {
		t.Fatalf("genau 500: Grenze %d", n)
	}
	dag, cs := nachspielKnoten(t, map[string]float64{a.addr: 10000})
	if ok := dag.replayTransactions(testBlock(1, txs[1:]...), true); ok {
		t.Fatal("Block mit 501 PQ-Ueberweisungen angenommen")
	}
	if got := kontoVon(t, cs, b.addr).Balance.Float(); got != 0 {
		t.Fatalf("Empfaenger hat %.6f", got)
	}
	dag2, cs2 := nachspielKnoten(t, map[string]float64{a.addr: 10000})
	if ok := dag2.replayTransactions(testBlock(1, txs[1:pqUeberweisungenJeBlock+1]...), true); !ok {
		t.Fatal("Block mit genau 500 PQ-Ueberweisungen abgelehnt")
	}
	if got := kontoVon(t, cs2, a.addr).NaechsteNonce; got != pqUeberweisungenJeBlock {
		t.Fatalf("NaechsteNonce %d", got)
	}
}

// Die App (lib/tresor.ts, @noble/post-quantum, deterministisch) unterschreibt
// so, dass JEDER Knoten die Ueberweisung annimmt: Vektor von der App erzeugt
// (testdata/pq_ueberweisung_app_vektor.json, Seed 0x11*32).
func TestPQUeberweisung_UnterschriftDerAppGiltImKnoten(t *testing.T) {
	pqAktivFuerTest(t)
	roh, err := os.ReadFile("testdata/pq_ueberweisung_app_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Adresse, An, Wei, Pub, Sig string
		Nonce                      uint64
	}
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	wei, _ := new(big.Int).SetString(v.Wei, 10)
	tx := Transaction{Type: "transfer", Wallet: v.Adresse, To: v.An, Amount: betragAusWei(wei),
		TxHash: pqUeberweisungTxHash(pqUeberweisungNachricht(v.Adresse, v.An, v.Wei, v.Nonce)),
		PQ:     &PQUeberweisung{Pub: v.Pub, Sig: v.Sig, Nonce: v.Nonce, Wei: v.Wei}}
	if _, err := pruefeUeberweisungsRoh(&tx, nowUnix()); err != nil {
		t.Fatalf("Unterschrift der App abgelehnt: %v", err)
	}
	if neuerPQKontoSchluessel(t, 0x11).addr != v.Adresse {
		t.Fatal("App und Knoten leiten verschiedene Adressen ab")
	}
}

// Sicherheitspruefung von #488: eine PQ-Ueberweisung passt nur in Bloecke ab
// dem Stichtag -- der Erzeuger waehlt eine Blockzeit danach oder laesst sie
// fuer den naechsten Block liegen.
func TestPQUeberweisung_FensterAbStichtag(t *testing.T) {
	vorher := pqUeberweisungAbOverride.Load()
	pqUeberweisungAbOverride.Store(2_000_000_000)
	defer pqUeberweisungAbOverride.Store(vorher)
	a := neuerPQKontoSchluessel(t, 0xa1)
	tx := pqTransfer(t, a, neuerTestSchluessel(t).addr, aeqWei(1), 0)
	if err := blockTauglich(&tx, 2_000_000_000-1); err == nil {
		t.Fatal("vor dem Stichtag tauglich")
	}
	if err := blockTauglich(&tx, 2_000_000_000); err != nil {
		t.Fatalf("zum Stichtag nicht tauglich: %v", err)
	}
	if f := auftragsFenster(&tx, 2_000_000_100); f.von != 2_000_000_000 {
		t.Fatalf("Fenster beginnt %d", f.von)
	}
	// Ein Block, dessen Zeit nur vor dem Stichtag laege, nimmt sie nicht auf.
	if n, _, err := blockZeitPraefix([]Transaction{{Type: "faucet", Wallet: a.addr}, tx}, 2_000_000_000-10, 2_000_000_000-20); err != nil || n != 1 {
		t.Fatalf("Praefix %d (%v), erwartet 1", n, err)
	}
}
