package keeper

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/big"
	"os"
	"strings"
	"testing"
)

// Kettenseite der Quanten-Note (quanten_note.go): dieselben Angriffe wie die
// Spezifikation forschung/quantengeld/bruecke_kette.py, dazu der Gegenabgleich
// mit einer dort erzeugten Unterschrift.

type lamportTest struct {
	geheim [lamportBits][2][32]byte
	pub    []byte
}

func neuerLamport(t *testing.T) *lamportTest {
	t.Helper()
	k := &lamportTest{}
	for i := 0; i < lamportBits; i++ {
		for b := 0; b < 2; b++ {
			if _, err := rand.Read(k.geheim[i][b][:]); err != nil {
				t.Fatal(err)
			}
			h := sha256.Sum256(k.geheim[i][b][:])
			k.pub = append(k.pub, h[:]...)
		}
	}
	return k
}

func (k *lamportTest) unterschreiben(n []byte) []byte {
	d := sha256.Sum256(n)
	var sig []byte
	for i := 0; i < lamportBits; i++ {
		bit := int(d[i/8]>>(7-uint(i%8))) & 1
		sig = append(sig, k.geheim[i][bit][:]...)
	}
	return sig
}

func aeq(n int64) *big.Int { return new(big.Int).Mul(big.NewInt(n), big.NewInt(1e18)) }

const (
	qnAnna = "0x00000000000000000000000000000000000000a1"
	qnBen  = "0x00000000000000000000000000000000000000b2"
	qnDieb = "0x00000000000000000000000000000000000000d3"
)

// Die Unterschrift der Spezifikation (Python) gilt im Knoten -- und nur fuer
// genau ihre Nachricht.
func TestQuantenNote_SpezifikationGiltImKnoten(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_note_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct{ Oeffentlich, S, Ziel, Wei, Sig, Nachricht string }
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	pub, _ := hex.DecodeString(v.Oeffentlich)
	sig, _ := hex.DecodeString(v.Sig)
	wei, _ := new(big.Int).SetString(v.Wei, 10)
	if quantenSeriennummer(pub) != v.S {
		t.Fatal("Seriennummer weicht von der Spezifikation ab")
	}
	if string(quantenEinloeseNachricht(v.S, v.Ziel, wei)) != v.Nachricht {
		t.Fatalf("Nachricht weicht ab: %q", quantenEinloeseNachricht(v.S, v.Ziel, wei))
	}
	if !pruefeLamport(pub, []byte(v.Nachricht), sig) {
		t.Fatal("Unterschrift der Spezifikation im Knoten abgelehnt")
	}
	if pruefeLamport(pub, quantenEinloeseNachricht(v.S, qnDieb, wei), sig) {
		t.Fatal("Unterschrift gilt fuer ein anderes Ziel")
	}
}

func TestQuantenNote_GutfallUndAngriffe(t *testing.T) {
	reg := &quantenRegister{}
	konten := map[string]*big.Int{qnAnna: aeq(1000), qnBen: new(big.Int), qnDieb: new(big.Int)}
	summe := func(r *quantenRegister) *big.Int {
		s := new(big.Int).Set(r.summeGesperrt())
		for _, v := range konten {
			s.Add(s, v)
		}
		return s
	}
	start := summe(reg)
	sperren := func(wer string, wei *big.Int, k *lamportTest, frist, jetzt int64, rueck bool) (string, error) {
		neu, s, err := reg.quantenSperren(wer, konten[wer], wei, k.pub, frist, jetzt, rueck)
		if err == nil {
			reg = neu
			konten[wer] = new(big.Int).Sub(konten[wer], wei)
		}
		return s, err
	}
	einloesen := func(s, ziel string, wei *big.Int, pub, sig []byte) error {
		neu, betrag, err := reg.quantenEinloesen(s, ziel, wei, pub, sig)
		if err == nil {
			reg = neu
			if konten[ziel] == nil {
				konten[ziel] = new(big.Int)
			}
			konten[ziel] = new(big.Int).Add(konten[ziel], betrag)
		}
		return err
	}
	rueck := func(wer, s string, jetzt int64) error {
		neu, betrag, err := reg.quantenRueckholen(wer, s, jetzt)
		if err == nil {
			reg = neu
			konten[wer] = new(big.Int).Add(konten[wer], betrag)
		}
		return err
	}
	angriff := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Errorf("Angriff ging durch: %s", name)
		}
	}

	// Gutfall.
	n1 := neuerLamport(t)
	s1, err := sperren(qnAnna, aeq(300), n1, 100, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	sig1 := n1.unterschreiben(quantenEinloeseNachricht(s1, qnBen, aeq(300)))
	if err := einloesen(s1, qnBen, aeq(300), n1.pub, sig1); err != nil {
		t.Fatal(err)
	}
	if konten[qnBen].Cmp(aeq(300)) != 0 || summe(reg).Cmp(start) != 0 {
		t.Fatal("Gutfall: falsch gebucht")
	}

	// Einloesen.
	angriff("Wiederholung", einloesen(s1, qnBen, aeq(300), n1.pub, sig1))
	angriff("zweite gueltige Unterschrift an anderes Ziel", einloesen(s1, qnDieb, aeq(300), n1.pub, n1.unterschreiben(quantenEinloeseNachricht(s1, qnDieb, aeq(300)))))
	n2 := neuerLamport(t)
	s2, _ := sperren(qnAnna, aeq(200), n2, 100, 0, true)
	ok2 := n2.unterschreiben(quantenEinloeseNachricht(s2, qnBen, aeq(200)))
	angriff("abgefangene Unterschrift umlenken", einloesen(s2, qnDieb, aeq(200), n2.pub, ok2))
	angriff("Betrag erhoehen", einloesen(s2, qnBen, aeq(201), n2.pub, ok2))
	fremd := neuerLamport(t)
	angriff("fremder Schluessel", einloesen(s2, qnDieb, aeq(200), fremd.pub, fremd.unterschreiben(quantenEinloeseNachricht(s2, qnDieb, aeq(200)))))
	kaputt := append([]byte(nil), ok2...)
	kaputt[5] ^= 1
	angriff("ein Bit falsch", einloesen(s2, qnBen, aeq(200), n2.pub, kaputt))
	angriff("abgeschnitten", einloesen(s2, qnBen, aeq(200), n2.pub, ok2[:len(ok2)-1]))
	angriff("unbekannte Seriennummer", einloesen(strings.Repeat("00", 32), qnDieb, aeq(200), n2.pub, ok2))
	// Sperren.
	angriff("ohne Deckung", func() error { _, e := sperren(qnDieb, aeq(1), neuerLamport(t), 10, 0, false); return e }())
	angriff("Betrag 0", func() error { _, e := sperren(qnAnna, new(big.Int), neuerLamport(t), 10, 0, false); return e }())
	angriff("Seriennummer erneut", func() error { _, e := sperren(qnAnna, aeq(1), n2, 10, 0, false); return e }())
	angriff("verbrauchte Seriennummer erneut", func() error { _, e := sperren(qnAnna, aeq(1), n1, 10, 0, false); return e }())
	angriff("Schluessel in falscher Form", func() error {
		_, _, e := reg.quantenSperren(qnAnna, konten[qnAnna], aeq(1), []byte("x"), 10, 0, false)
		return e
	}())
	// Rueckholen.
	angriff("vor Fristablauf", rueck(qnAnna, s2, 50))
	angriff("durch Fremde", rueck(qnDieb, s2, 200))
	if err := rueck(qnAnna, s2, 200); err != nil {
		t.Fatalf("Rueckholung nach Frist: %v", err)
	}
	angriff("nach Rueckholung einloesen", einloesen(s2, qnBen, aeq(200), n2.pub, ok2))
	n3 := neuerLamport(t)
	s3, _ := sperren(qnAnna, aeq(100), n3, 1, 0, false)
	angriff("Inhaberpapier zurueckholen", rueck(qnAnna, s3, 1000))

	if summe(reg).Cmp(start) != 0 {
		t.Fatalf("Erhaltung verletzt: %s statt %s", summe(reg), start)
	}
}

// Solange das Verfahren der Lamport-Platzhalter ist, bleibt die Bruecke aus.
// Einschalten heisst: echtes Quanten-Einloeseverfahren UND Geldweg-Anschluss
// in einem eigenen, geprueften PR -- dieser Test muss dann bewusst angepasst
// werden.
func TestQuantenNote_SchlafendSolangePlatzhalter(t *testing.T) {
	if quantenNoteVerfahren == "lamport-platzhalter" && quantenNoteAbUnix != math.MaxInt64 {
		t.Fatal("Quanten-Note mit Platzhalter-Signatur eingeschaltet")
	}
	if quantenNoteAktiv(2_000_000_000) {
		t.Fatal("Quanten-Note aktiv")
	}
	for _, art := range []string{"quanten_sperren", "quanten_einloesen", "quanten_rueckholen"} {
		if bekannteTxArt(art, 2_000_000_000) {
			t.Fatalf("%s ist als Transaktionsart bekannt", art)
		}
	}
}
