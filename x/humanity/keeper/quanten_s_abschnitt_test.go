package keeper

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
)

// Pruefvektor aus der Spezifikation (s_kette.py --vektor-abschnitte): Grenzen,
// Stellen-Bit, Blaetter, Wurzel und Stichproben sind Byte fuer Byte gleich.
func TestQuantenSAbschnitt_Vektor(t *testing.T) {
	roh, err := os.ReadFile("testdata/quanten_s_abschnitt_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		K                 string              `json:"K"`
		N                 int                 `json:"n"`
		NVal              int                 `json:"n_val"`
		Abschnitte        [][2]int            `json:"abschnitte"`
		AbschnitteWeitere map[string][][2]int `json:"abschnitte_weitere"`
		Werte             []string            `json:"werte"`
		StellenBit        []int               `json:"stellen_bit"`
		Blaetter          []string            `json:"blaetter"`
		Wurzel            string              `json:"wurzel"`
		Stichprobe        [2][]int            `json:"stichprobe"`
	}
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	pruefeGrenzen := func(n, nv int, soll [][2]int) {
		t.Helper()
		if len(soll) != nv {
			t.Fatalf("%d/%d: %d Abschnitte im Vektor", n, nv, len(soll))
		}
		for x := 1; x <= nv; x++ {
			von, bis, ok := quantenSAbschnitt(n, nv, x)
			if !ok || von != soll[x-1][0] || bis != soll[x-1][1] {
				t.Fatalf("%d/%d, Validator %d: [%d, %d) statt %v", n, nv, x, von, bis, soll[x-1])
			}
		}
	}
	pruefeGrenzen(v.N, v.NVal, v.Abschnitte)
	for schl, soll := range v.AbschnitteWeitere {
		var n, nv int
		if _, err := fmt.Sscanf(schl, "%d/%d", &n, &nv); err != nil {
			t.Fatal(err)
		}
		pruefeGrenzen(n, nv, soll)
	}
	werte := make([]*big.Int, v.N)
	blaetter := make([][32]byte, v.N)
	for i := 0; i < v.N; i++ {
		w, ok := new(big.Int).SetString(v.Werte[i], 10)
		if !ok {
			t.Fatalf("Wert %d", i)
		}
		werte[i] = w
		if quantenSStellenBit(w) != v.StellenBit[i] {
			t.Fatalf("Stellen-Bit %d", i)
		}
		b, _ := quantenWHex32(v.Blaetter[i])
		blaetter[i] = b
		if quantenWPruefsumme(v.K, i, w) != b {
			t.Fatalf("Blatt %d", i)
		}
	}
	wurzel, _ := quantenWHex32(v.Wurzel)
	if quantenWMerkleWurzel(blaetter) != wurzel {
		t.Fatal("Wurzel")
	}
	for b := 0; b < 2; b++ {
		oe := map[int]*big.Int{}
		for _, i := range v.Stichprobe[b] {
			oe[i] = werte[i]
		}
		geladen, err := quantenSOeffnungLaden(b, v.K, v.N, wurzel, blaetter, oe, len(oe))
		if err != nil || len(geladen) != len(oe) {
			t.Fatalf("Stelle %d laedt nicht: %v", b, err)
		}
		for i, ut := range geladen {
			bb, tt := quantenWZerlegen(werte[i])
			if ut != [2]int{bb, tt} {
				t.Fatalf("Stelle %d, %d: falsch zerlegt", b, i)
			}
		}
		// Die Stichprobe der anderen Stelle: abgewiesen.
		fremd := map[int]*big.Int{}
		for _, i := range v.Stichprobe[1-b] {
			fremd[i] = werte[i]
		}
		if _, err := quantenSOeffnungLaden(b, v.K, v.N, wurzel, blaetter, fremd, 1); err == nil {
			t.Fatalf("Stelle %d laedt die Stichprobe der anderen", b)
		}
	}
}

func TestQuantenSAbschnitt_Grenzen(t *testing.T) {
	for _, f := range [][3]int{{3, 4, 1}, {10, 0, 1}, {10, 3, 0}, {10, 3, 4}, {quantenWNMax + 1, 4, 1}, {200, quantenWVMax + 1, 1}} {
		if _, _, ok := quantenSAbschnitt(f[0], f[1], f[2]); ok {
			t.Fatalf("%v angenommen", f)
		}
	}
	// Jede Stelle genau einmal, hoechstens eine Stelle Unterschied.
	for _, f := range [][2]int{{10400, 4}, {3920, 10}, {7, 7}, {101, 6}} {
		n, nv := f[0], f[1]
		naechste, mi, ma := 0, n, 0
		for x := 1; x <= nv; x++ {
			von, bis, ok := quantenSAbschnitt(n, nv, x)
			if !ok || von != naechste {
				t.Fatalf("%d/%d: Luecke vor Validator %d", n, nv, x)
			}
			naechste = bis
			mi, ma = min(mi, bis-von), max(ma, bis-von)
		}
		if naechste != n || ma-mi > 1 {
			t.Fatalf("%d/%d: deckt nicht alles oder ungleich", n, nv)
		}
	}
}

func TestQuantenSAbschnittWerte(t *testing.T) {
	const n = 101
	u, tw := make([]int, n), make([]int, n)
	for i := range u {
		u[i], tw[i] = i%2, (i/2)%2
	}
	ws, err := quantenSAbschnittWerte(u, tw)
	if err != nil {
		t.Fatal(err)
	}
	grenze := new(big.Int).Lsh(big.NewInt(1), quantenWSalzBits+2)
	einsen := 0
	for i, w := range ws {
		b, tt := quantenWZerlegen(w)
		if b != u[i] || tt != tw[i] || w.Cmp(grenze) >= 0 {
			t.Fatalf("Stelle %d: Basis, Wert oder Groesse", i)
		}
		einsen += quantenSStellenBit(w)
	}
	if einsen != n-n/2 {
		t.Fatalf("Stellen-Bit: %d Einsen bei %d Stellen, erwartet %d", einsen, n, n-n/2)
	}
	ws2, _ := quantenSAbschnittWerte(u, tw)
	if ws[0].Cmp(ws2[0]) == 0 && ws[1].Cmp(ws2[1]) == 0 {
		t.Fatal("zweimal dieselben Werte -- kein Zufall")
	}
	for _, f := range []func() error{
		func() error { _, err := quantenSAbschnittWerte(nil, nil); return err },
		func() error { _, err := quantenSAbschnittWerte([]int{0}, []int{0, 1}); return err },
		func() error { _, err := quantenSAbschnittWerte([]int{2}, []int{0}); return err },
		func() error { _, err := quantenSAbschnittWerte([]int{0}, []int{-1}); return err },
	} {
		if f() == nil {
			t.Fatal("ungueltige Eingabe angenommen")
		}
	}
}

func TestQuantenSTeilen(t *testing.T) {
	w, _ := new(big.Int).SetString("123456789abcdef0123456789abcdef", 16)
	a, err := quantenSTeilen(w, 3, 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, wahl := range [][]int{{1, 2, 3}, {1, 3, 5}, {2, 4, 5}, {5, 4, 1}} {
		var an []quantenWAnteil
		for _, x := range wahl {
			an = append(an, quantenWAnteil{X: x, Y: a[x-1]})
		}
		if quantenWZusammensetzen(an).Cmp(w) != 0 {
			t.Fatalf("%v setzt nicht zusammen", wahl)
		}
	}
	if quantenWZusammensetzen([]quantenWAnteil{{X: 1, Y: a[0]}, {X: 2, Y: a[1]}}).Cmp(w) == 0 {
		t.Fatal("t-1 Anteile ergeben den Wert")
	}
	for _, f := range [][2]int{{0, 3}, {4, 3}, {2, quantenWVMax + 1}} {
		if _, err := quantenSTeilen(w, f[0], f[1]); err == nil {
			t.Fatalf("Schwelle %v angenommen", f)
		}
	}
	if _, err := quantenSTeilen(new(big.Int).Set(quantenWP), 2, 3); err == nil {
		t.Fatal("Wert >= P angenommen")
	}
}

func qsSchluessel(t *testing.T) (*big.Int, string, string, []byte) {
	t.Helper()
	k, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k.D, strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex()), hex.EncodeToString(crypto.FromECDSAPub(&k.PublicKey)), crypto.FromECDSA(k)
}

func TestQuantenSSchluesselPruefen(t *testing.T) {
	_, adr, pub, _ := qsSchluessel(t)
	_, adr2, _, _ := qsSchluessel(t)
	if _, err := quantenSSchluesselPruefen(pub, adr); err != nil {
		t.Fatal(err)
	}
	if _, err := quantenSSchluesselPruefen("0x"+pub, strings.ToUpper(adr[:2])+adr[2:]); err != nil {
		t.Fatalf("mit 0x: %v", err)
	}
	for name, f := range map[string][2]string{
		"fremde Adresse": {pub, adr2},
		"kurz":           {pub[:128], adr},
		"kein Hex":       {strings.Repeat("z", 130), adr},
		"kein Punkt":     {"04" + strings.Repeat("00", 64), adr},
	} {
		if _, err := quantenSSchluesselPruefen(f[0], f[1]); err == nil {
			t.Fatalf("%s angenommen", name)
		}
	}
}

func TestQuantenSBuendel(t *testing.T) {
	praep, _ := crypto.GenerateKey()
	empf, _ := crypto.GenerateKey()
	dritter, _ := crypto.GenerateKey()
	praepAdr := quantenSAdresse(praep)
	K := strings.Repeat("ab", 32)
	anteile := []string{"1", "ff", strings.Repeat("7", 32)} // 0x77..77 < P = 2^127-1
	in := quantenSBuendelInhalt{K: K, Von: 10, Bis: 13, Praeparator: 2, Empfaenger: 3, Anteile: anteile}
	c, sig, err := quantenSBuendelVersiegeln(praep, &empf.PublicKey, in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := quantenSBuendelOeffnen(empf, c, sig, praepAdr, K, 2, 3, 10, 13)
	if err != nil || len(got) != 3 || got[1].Text(16) != "ff" {
		t.Fatalf("Gutfall: %v %v", got, err)
	}
	// Der Ausgeber (oder wer mitliest) kann nichts lesen.
	if _, err := ecies.ImportECDSA(dritter).Decrypt(qsHex(t, c), nil, nil); err == nil {
		t.Fatal("ein Dritter entschluesselt das Buendel")
	}
	// Missbrauch: alles abgewiesen.
	kaputt := []byte(c)
	if kaputt[100] == 'a' {
		kaputt[100] = 'b'
	} else {
		kaputt[100] = 'a'
	}
	sigDritter, _ := quantenSUnterschreiben(dritter, quantenSBuendelNachricht(K, 2, 3, c))
	for name, f := range map[string]func() error{
		"anderer Empfaenger": func() error { _, err := quantenSBuendelOeffnen(dritter, c, sig, praepAdr, K, 2, 3, 10, 13); return err },
		"Chiffrat getauscht": func() error {
			_, err := quantenSBuendelOeffnen(empf, string(kaputt), sig, praepAdr, K, 2, 3, 10, 13)
			return err
		},
		"fremde Unterschrift": func() error {
			_, err := quantenSBuendelOeffnen(empf, c, sigDritter, praepAdr, K, 2, 3, 10, 13)
			return err
		},
		"anderer Praeparator": func() error { _, err := quantenSBuendelOeffnen(empf, c, sig, praepAdr, K, 1, 3, 10, 13); return err },
		"anderes K": func() error {
			_, err := quantenSBuendelOeffnen(empf, c, sig, praepAdr, strings.Repeat("cd", 32), 2, 3, 10, 13)
			return err
		},
		"anderer Abschnitt":      func() error { _, err := quantenSBuendelOeffnen(empf, c, sig, praepAdr, K, 2, 3, 11, 14); return err },
		"anderer Empfaenger-Nr.": func() error { _, err := quantenSBuendelOeffnen(empf, c, sig, praepAdr, K, 2, 4, 10, 13); return err },
		"zu gross": func() error {
			_, err := quantenSBuendelOeffnen(empf, strings.Repeat("a", quantenSBuendelHoechstens+2), sig, praepAdr, K, 2, 3, 10, 13)
			return err
		},
		"Grossbuchstaben": func() error {
			_, err := quantenSBuendelOeffnen(empf, strings.ToUpper(c), sig, praepAdr, K, 2, 3, 10, 13)
			return err
		},
	} {
		if f() == nil {
			t.Fatalf("%s angenommen", name)
		}
	}
	// Ein richtig versiegeltes Buendel mit falschem Inhalt: unbekanntes Feld,
	// Anteil >= P, falsche Zahl der Anteile, fremde Domaene.
	roh := func(s string) (string, string) {
		cc, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(&empf.PublicKey), []byte(s), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		ch := hex.EncodeToString(cc)
		sg, _ := quantenSUnterschreiben(praep, quantenSBuendelNachricht(K, 2, 3, ch))
		return ch, sg
	}
	basis := `"k":"` + K + `","von":10,"bis":13,"praeparator":2,"empfaenger":3`
	for name, s := range map[string]string{
		"unbekanntes Feld": `{"dom":"` + quantenSDomBuendel + `",` + basis + `,"anteile":["1","2","3"],"mehr":1}`,
		"Anteil >= P":      `{"dom":"` + quantenSDomBuendel + `",` + basis + `,"anteile":["1","2","` + quantenWP.Text(16) + `"]}`,
		"zu wenige":        `{"dom":"` + quantenSDomBuendel + `",` + basis + `,"anteile":["1","2"]}`,
		"fremde Domaene":   `{"dom":"x",` + basis + `,"anteile":["1","2","3"]}`,
		"Anteil Gross":     `{"dom":"` + quantenSDomBuendel + `",` + basis + `,"anteile":["1","2","AB"]}`,
		"zwei Objekte":     `{"dom":"` + quantenSDomBuendel + `",` + basis + `,"anteile":["1","2","3"]}{}`,
	} {
		ch, sg := roh(s)
		if _, err := quantenSBuendelOeffnen(empf, ch, sg, praepAdr, K, 2, 3, 10, 13); err == nil {
			t.Fatalf("%s angenommen", name)
		}
	}
}

func qsHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestQuantenSOeffnungLaden_Missbrauch(t *testing.T) {
	const n = 64
	K := strings.Repeat("ef", 32)
	u, tw := make([]int, n), make([]int, n)
	for i := range u {
		u[i], tw[i] = (i*7)%2, (i*3/2)%2
	}
	ws, err := quantenSAbschnittWerte(u, tw)
	if err != nil {
		t.Fatal(err)
	}
	blaetter := make([][32]byte, n)
	for i, w := range ws {
		blaetter[i] = quantenWPruefsumme(K, i, w)
	}
	wurzel := quantenWMerkleWurzel(blaetter)
	oe := [2]map[int]*big.Int{{}, {}}
	for i, w := range ws {
		oe[quantenSStellenBit(w)][i] = w
	}
	if len(oe[0]) != n/2 || len(oe[1]) != n/2 {
		t.Fatalf("nicht je zur Haelfte: %d/%d", len(oe[0]), len(oe[1]))
	}
	if _, err := quantenSOeffnungLaden(0, K, n, wurzel, blaetter, oe[0], n/2); err != nil {
		t.Fatal(err)
	}
	kopie := func(m map[int]*big.Int) map[int]*big.Int {
		out := map[int]*big.Int{}
		for k, v := range m {
			out[k] = new(big.Int).Set(v)
		}
		return out
	}
	var einer, eins int
	for i := range oe[0] {
		einer = i
		break
	}
	for i := range oe[1] {
		eins = i
		break
	}
	fremd := kopie(oe[0])
	fremd[eins] = oe[1][eins]
	falsch := kopie(oe[0])
	falsch[einer].Xor(falsch[einer], big.NewInt(1))
	zuGross := kopie(oe[0])
	zuGross[einer] = new(big.Int).Lsh(big.NewInt(1), quantenWSalzBits+2)
	aussen := kopie(oe[0])
	aussen[n] = oe[0][einer]
	ohne := kopie(oe[0])
	ohne[einer] = nil
	andereBlaetter := append([][32]byte(nil), blaetter...)
	andereBlaetter[0][0] ^= 1
	for name, f := range map[string]func() error{
		"Stelle der anderen": func() error { _, err := quantenSOeffnungLaden(0, K, n, wurzel, blaetter, fremd, 1); return err },
		"falscher Wert":      func() error { _, err := quantenSOeffnungLaden(0, K, n, wurzel, blaetter, falsch, 1); return err },
		"Wert zu gross":      func() error { _, err := quantenSOeffnungLaden(0, K, n, wurzel, blaetter, zuGross, 1); return err },
		"Stelle ausserhalb":  func() error { _, err := quantenSOeffnungLaden(0, K, n, wurzel, blaetter, aussen, 1); return err },
		"Wert fehlt":         func() error { _, err := quantenSOeffnungLaden(0, K, n, wurzel, blaetter, ohne, 1); return err },
		"zu klein":           func() error { _, err := quantenSOeffnungLaden(0, K, n, wurzel, blaetter, oe[0], n/2+1); return err },
		"fremde Wurzel":      func() error { _, err := quantenSOeffnungLaden(0, K, n, [32]byte{1}, blaetter, oe[0], 1); return err },
		"Blaetter getauscht": func() error { _, err := quantenSOeffnungLaden(0, K, n, wurzel, andereBlaetter, oe[0], 1); return err },
		"Stelle 2":           func() error { _, err := quantenSOeffnungLaden(2, K, n, wurzel, blaetter, oe[0], 1); return err },
		"anderes K": func() error {
			_, err := quantenSOeffnungLaden(0, strings.Repeat("aa", 32), n, wurzel, blaetter, oe[0], 1)
			return err
		},
		"Zahl der Blaetter":   func() error { _, err := quantenSOeffnungLaden(0, K, n+1, wurzel, blaetter, oe[0], 1); return err },
		"Mindestgroesse null": func() error { _, err := quantenSOeffnungLaden(0, K, n, wurzel, blaetter, oe[0], 0); return err },
	} {
		if f() == nil {
			t.Fatalf("%s angenommen", name)
		}
	}
}
