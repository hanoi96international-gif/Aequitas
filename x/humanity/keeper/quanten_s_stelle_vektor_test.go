package keeper

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
)

// PRUEFVEKTOR FUER DIE APP ALS ANNAHMESTELLE (DEZENTRAL.md 17, R6).
//
// Die App (Aequitas-App, lib/quantenSStelle.ts) muss genau so rechnen wie
// quantenSAnnahmeLauf (quanten_s_rollen.go): Vorlage gegen die Inhaberin auf
// der Kette, Blaetter vom Validator gegen die Wurzel, Oeffnung fuer diese
// Stelle (ECIES wie go-ethereum) gegen die Blaetter, Stellen-Bit, Stichprobe
// und Salze gegen die Bindung der Ausgabe, Vorab-Urteil wie die Kette, dann
// die Anmeldung unterschreiben. Form wie im Hauptnetz: ein Validator (t = v =
// 1), n = 1.104, Toleranz 2 % (k = 552).
//
// Fest sind alle Schluessel (nur Tests!) und alle Werte (aus einem
// SHA-256-Zaehler). Die Chiffrate (Oeffnung fuer die Stelle, x fuer die
// Inhaberin) sind zufaellig und darum einmal erzeugt und gespeichert. Neu
// schreiben: AEQUITAS_VEKTOR_SCHREIBEN=1 go test -run TestQuantenSStelleVektor .
// Danach nach Aequitas-App/lib/__tests__/vektoren/ kopieren.

type qssVektor struct {
	Hinweis             string                 `json:"hinweis"`
	ChainID             int64                  `json:"chain_id"`
	StelleSchluessel    string                 `json:"stelle_schluessel_nur_test"`
	Stelle              string                 `json:"stelle"`
	StellePub           string                 `json:"stelle_pub"`
	AndereStelle        string                 `json:"andere_stelle"`
	Inhaberin           string                 `json:"inhaberin"`
	Validator           string                 `json:"validator"`
	K                   string                 `json:"k"`
	B                   int                    `json:"b"`
	Stand               map[string]interface{} `json:"stand"`
	Abschnitt           QuantenSPaketAbschnitt `json:"abschnitt"`
	Oeffnung            string                 `json:"oeffnung"`
	Vorlage             QuantenSVorlage        `json:"vorlage"`
	Probe               []int                  `json:"probe"`
	Salze               []string               `json:"salze"`
	Treffer             int                    `json:"treffer"`
	Fehler              int                    `json:"fehler"`
	Vorab               string                 `json:"vorab"`
	AnmeldenNachricht   string                 `json:"anmelden_nachricht"`
	AnmeldenSig         string                 `json:"anmelden_sig"`
	FalscheOeffnung     string                 `json:"falsche_oeffnung"` // fuer die andere Stelle verschluesselt
	FuerInhaberinGeheim string                 `json:"fuer_inhaberin_nur_test"`
	// Nachweis nach dem Ende (quanten_s_nachweis.go): was der Validator nach
	// dem Einloesen offenlegt, und was ein Pruefer daraus nachrechnet.
	Nachweis        QuantenSNachweis        `json:"nachweis"`
	NachweisBericht QuantenSNachweisBericht `json:"nachweis_bericht"`
}

const qssVektorPfad = "testdata/quanten_s_stelle_vektor.json"

// qssStrom: feste Pseudozufallsbits aus einem SHA-256-Zaehler (nur Test).
type qssStrom struct {
	zaehler uint64
	puffer  []byte
}

func (s *qssStrom) byte_() byte {
	if len(s.puffer) == 0 {
		var z [8]byte
		binary.BigEndian.PutUint64(z[:], s.zaehler)
		s.zaehler++
		h := sha256.Sum256(append([]byte("qss-vektor|"), z[:]...))
		s.puffer = h[:]
	}
	b := s.puffer[0]
	s.puffer = s.puffer[1:]
	return b
}

func (s *qssStrom) bit() int { return int(s.byte_() & 1) }

func qssVektorBauen(t *testing.T, gespeichert *qssVektor) qssVektor {
	t.Helper()
	const n, b, tol, z = 1104, 1, 2, 1
	stelleKey, _ := crypto.HexToECDSA(strings.Repeat("81", 32))
	andereKey, _ := crypto.HexToECDSA(strings.Repeat("82", 32))
	inhKey, _ := crypto.HexToECDSA(strings.Repeat("51", 32))
	valKey, _ := crypto.HexToECDSA(strings.Repeat("61", 32))
	v := qssVektor{Hinweis: "nur fuer Tests: feste Schluessel und Werte, nie fuer echte Werte", ChainID: eingefroreneChainID,
		StelleSchluessel: strings.Repeat("81", 32), Stelle: quantenSAdresse(stelleKey), StellePub: quantenSPubHex(stelleKey),
		AndereStelle: quantenSAdresse(andereKey), Inhaberin: quantenSAdresse(inhKey), Validator: quantenSAdresse(valKey),
		K: strings.Repeat("5e", 32), B: b}
	st := &qssStrom{}
	u, tw, bit, y, x := make([]int, n), make([]int, n), make([]int, n), make([]int, n), make([]int, n)
	werte := make([]*big.Int, n)
	blaetter := make([][32]byte, n)
	reihe := make([]int, n)
	for i := range reihe {
		reihe[i] = i
	}
	for i := n - 1; i > 0; i-- { // Stellen-Bits genau zur Haelfte (wie quantenSAbschnittWerte)
		j := int(binary.BigEndian.Uint16([]byte{st.byte_(), st.byte_()})) % (i + 1)
		reihe[i], reihe[j] = reihe[j], reihe[i]
	}
	for j, i := range reihe {
		if j >= n/2 {
			bit[i] = 1
		}
	}
	for i := 0; i < n; i++ {
		u[i], tw[i], y[i] = st.bit(), st.bit(), st.bit()
		salz := make([]byte, 15) // 120 Bit < 2^123
		for k := range salz {
			salz[k] = st.byte_()
		}
		w := new(big.Int).SetBytes(salz)
		w.Lsh(w, 1).Add(w, big.NewInt(int64(bit[i])))
		w.Lsh(w, 2).Add(w, big.NewInt(int64(u[i]+2*tw[i])))
		werte[i] = w
		blaetter[i] = quantenWPruefsumme(v.K, i, w)
		// Messung der Inhaberin in ihren Basen y: bei passender Basis der Wert
		// (an wenigen Stellen falsch -- Messfehler), sonst Zufall.
		if y[i] == u[i] {
			x[i] = tw[i]
			if i%97 == 5 {
				x[i] ^= 1
			}
		} else {
			x[i] = st.bit()
		}
	}
	wurzel := quantenWMerkleWurzel(blaetter)
	// Salze und Teil-Hashes je Stelle (ein Abschnitt), Proben der Ausgabe.
	var salze [2][32]byte
	var teile [2][32]byte
	var proben [2][32]byte
	stellenVon := func(s int) []int {
		var p []int
		for i := 0; i < n; i++ {
			if bit[i] == s {
				p = append(p, i)
			}
		}
		return p
	}
	for s := 0; s < 2; s++ {
		for k := range salze[s] {
			salze[s][k] = st.byte_()
		}
		teile[s] = quantenSProbeTeilHash(v.K, s, 1, salze[s], stellenVon(s))
		h, err := quantenSProbeHashV2(v.K, s, [][32]byte{teile[s]})
		if err != nil {
			t.Fatal(err)
		}
		proben[s] = h
	}
	d := make([]int, n)
	for i := range d {
		d[i] = y[i] ^ z
	}
	c := b ^ z
	v.Stand = map[string]interface{}{"k": v.K, "art": "s", "n": n, "t": 1, "v": 1, "tol": tol, "inhaber": v.Inhaberin,
		"stellen": []string{v.AndereStelle, v.Stelle}, "gebunden": true, "gewaehlt": true, "fehl": 0, "wert_mikro": 1000000,
		"probe_art": 2, "wurzel": hex.EncodeToString(wurzel[:]), "proben": []string{hex.EncodeToString(proben[0][:]), hex.EncodeToString(proben[1][:])},
		"d": qsBitsText(d), "c": c, "aktiv": true}
	// Abschnitt des Validators (fuer-inhaberin), unterschrieben.
	ab := QuantenSPaketAbschnitt{X: 1, Von: 0, Bis: n, BuendelHash: quantenSHashListe([]string{"", "buendel"}),
		BuendelSigHash: quantenSHashListe([]string{"", "sig"}), Zaehlung: [][2]int{{n / 4, 3}, {n / 4, 3}}}
	for i := range blaetter {
		ab.Blaetter = append(ab.Blaetter, hex.EncodeToString(blaetter[i][:]))
	}
	for s := 0; s < 2; s++ {
		ab.Teile = append(ab.Teile, hex.EncodeToString(teile[s][:]))
	}
	oeffnungFuer := func(key *ecdsa.PublicKey, s int) string {
		w := map[string]string{}
		for _, i := range stellenVon(s) {
			w[strconv.Itoa(i)] = werte[i].Text(16)
		}
		roh, _ := json.Marshal(map[string]interface{}{"dom": quantenSDomOeffnung, "k": v.K, "b": s, "x": 1, "von": 0, "bis": n, "werte": w,
			"salz": hex.EncodeToString(salze[s][:])})
		ch, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(key), roh, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return hex.EncodeToString(ch)
	}
	if gespeichert != nil {
		ab.FuerInhaberin, v.Oeffnung, v.FalscheOeffnung = gespeichert.Abschnitt.FuerInhaberin, gespeichert.Oeffnung, gespeichert.FalscheOeffnung
	} else {
		roh, _ := json.Marshal(map[string]interface{}{"dom": quantenSDomFuerInhaberin, "k": v.K, "von": 0, "bis": n, "x": qsBitsText(x)})
		ch, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(&inhKey.PublicKey), roh, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		ab.FuerInhaberin = hex.EncodeToString(ch)
		v.Oeffnung = oeffnungFuer(&stelleKey.PublicKey, b)
		v.FalscheOeffnung = oeffnungFuer(&andereKey.PublicKey, b) // richtiger Inhalt, falscher Schluessel
	}
	m := quantenSAbschnittAntwortNachrichtAus(v.K, 1, 0, n, quantenSHashListe(ab.Blaetter), ab.FuerInhaberin, ab.BuendelHash, ab.BuendelSigHash, ab.Teile, ab.Zaehlung, ab.BasenHash)
	var err error
	if ab.Sig, err = quantenSUnterschreiben(valKey, m); err != nil {
		t.Fatal(err)
	}
	v.Abschnitt = ab
	// Vorlage der Inhaberin fuer diese Stelle.
	vl := quantenSVorlageText(v.K, b, x)
	sig, err := quantenSUnterschreiben(inhKey, vl)
	if err != nil {
		t.Fatal(err)
	}
	v.Vorlage = QuantenSVorlage{K: v.K, B: b, X: qsBitsText(x), Vorlage: sig}
	// Was die Stelle laedt und vorab rechnet (wie quantenSAnnahmeLauf).
	v.Probe = stellenVon(b)
	sort.Ints(v.Probe)
	v.Salze = []string{hex.EncodeToString(salze[b][:])}
	for _, i := range v.Probe {
		if d[i]^b^c == u[i] {
			v.Treffer++
			if x[i] != tw[i] {
				v.Fehler++
			}
		}
	}
	v.Vorab = "abgewiesen"
	if quantenWUrteil(v.Treffer, v.Fehler, tol) == quantenWEcht {
		v.Vorab = "besteht"
	}
	v.AnmeldenNachricht = quantenWSAnmeldenNachricht(v.Stelle, v.K, strconv.Itoa(b), qsBitsText(x), v.Probe, 0)
	if v.AnmeldenSig, err = quantenSUnterschreiben(stelleKey, v.AnmeldenNachricht); err != nil {
		t.Fatal(err)
	}
	v.FuerInhaberinGeheim = strings.Repeat("51", 32)
	// Nachweis (deterministisch: EIP-191 mit RFC 6979).
	cc, bb := c, b
	nw := QuantenSNachweis{Dom: quantenSDomNachweis, K: v.K, N: n, T: 1, X: 1, Von: 0, Bis: n, Validator: v.Validator,
		Validatoren: []string{v.Validator}, Wurzel: hex.EncodeToString(wurzel[:]),
		Proben: []string{hex.EncodeToString(proben[0][:]), hex.EncodeToString(proben[1][:])},
		Salze:  []string{hex.EncodeToString(salze[0][:]), hex.EncodeToString(salze[1][:])}, XInhaberin: qsBitsText(x),
		D: qsBitsText(d), C: &cc, Stelle: &bb, Zeit: 1795180000}
	for _, w := range werte {
		nw.Werte = append(nw.Werte, w.Text(16))
	}
	nm, err := quantenSNachweisNachricht(nw)
	if err != nil {
		t.Fatal(err)
	}
	if nw.Sig, err = quantenSUnterschreiben(valKey, nm); err != nil {
		t.Fatal(err)
	}
	v.Nachweis = nw
	ber, err := quantenSNachweisePruefen(v.K, []string{v.Validator}, []QuantenSNachweis{nw}, &wurzel, nw.Proben)
	if err != nil {
		t.Fatal(err)
	}
	v.NachweisBericht = *ber
	return v
}

func TestQuantenSStelleVektor(t *testing.T) {
	if os.Getenv("AEQUITAS_VEKTOR_SCHREIBEN") == "1" {
		w := qssVektorBauen(t, nil)
		roh, _ := json.MarshalIndent(w, "", " ")
		if err := os.WriteFile(qssVektorPfad, append(roh, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	roh, err := os.ReadFile(qssVektorPfad)
	if err != nil {
		t.Fatal(err)
	}
	var g qssVektor
	if err := json.Unmarshal(roh, &g); err != nil {
		t.Fatal(err)
	}
	soll := qssVektorBauen(t, &g)
	a, _ := json.Marshal(soll)
	b, _ := json.Marshal(g)
	if string(a) != string(b) {
		t.Fatal("Vektor weicht von dem ab, was der Knoten rechnet")
	}
	// Derselbe Weg wie die Annahmestelle (quantenSAnnahmeLauf), Schritt fuer
	// Schritt mit den Funktionen des Knotens.
	key, _ := crypto.HexToECDSA(g.StelleSchluessel)
	n := 1104
	if !kanonischeSignatur(g.Vorlage.Vorlage) || verifyPersonalSign(quantenSVorlageText(g.K, g.B, mustBits(t, g.Vorlage.X, n)), g.Vorlage.Vorlage, g.Inhaberin) != nil {
		t.Fatal("Vorlage")
	}
	if err := quantenSPaketAbschnittPruefen(g.Abschnitt, g.K, n, 1, 1, g.Validator); err != nil {
		t.Fatal(err)
	}
	var blaetter [][32]byte
	for _, h := range g.Abschnitt.Blaetter {
		bl, _ := quantenWHex32(h)
		blaetter = append(blaetter, bl)
	}
	wurzel, _ := quantenWHex32(g.Stand["wurzel"].(string))
	if quantenWMerkleWurzel(blaetter) != wurzel {
		t.Fatal("Wurzel")
	}
	oe, salz, err := quantenSOeffnungEntschluesselnV2(key, g.Oeffnung, g.K, g.B, 1, 0, n)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := quantenSOeffnungEntschluesselnV2(key, g.FalscheOeffnung, g.K, g.B, 1, 0, n); err == nil {
		t.Fatal("Oeffnung fuer die andere Stelle gelesen")
	}
	geladen, err := quantenSOeffnungLaden(g.B, g.K, n, wurzel, blaetter, oe, quantenWKNote(2, 1, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(geladen) != len(g.Probe) || hex.EncodeToString(salz[:]) != g.Salze[0] {
		t.Fatal("Stichprobe oder Salz")
	}
	proben := g.Stand["proben"].([]interface{})
	pb, _ := quantenWHex32(proben[g.B].(string))
	if h, err := quantenSProbeV2Pruefen(g.K, g.B, n, 1, g.Probe, [][32]byte{salz}); err != nil || h != pb {
		t.Fatal("Stichprobe passt nicht zur Bindung")
	}
	if g.Vorab != "besteht" || g.Fehler == 0 || g.Treffer < quantenWKNote(2, 1, 1)/3 {
		t.Fatalf("Vektor soll eine ehrliche Vorlage mit wenigen Messfehlern sein: %s, %d/%d", g.Vorab, g.Fehler, g.Treffer)
	}
	if verifyPersonalSign(g.AnmeldenNachricht, g.AnmeldenSig, g.Stelle) != nil || !kanonischeSignatur(g.AnmeldenSig) {
		t.Fatal("Anmeldung")
	}
}

func mustBits(t *testing.T, s string, n int) []int {
	t.Helper()
	b, err := quantenSBitsText(s, n)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
