package keeper

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
)

// PRUEFVEKTOR FUER DIE APP ALS INHABERIN (R4b, DEZENTRAL.md 17).
//
// Die App (Aequitas-App, lib/quantenS.ts) muss genau so rechnen wie das
// Werkzeug der Inhaberin (quanten_s_rollen.go): x aus fuer_inhaberin lesen
// (ECIES wie go-ethereum), die Unterschrift jedes Validators ueber seinen
// Abschnitt pruefen, Merkle-Wurzel und Proben nachrechnen, Bindung, Wahl und
// Vorlage unterschreiben. testdata/quanten_s_inhaberin_vektor.json haelt das
// fest; derselbe Inhalt liegt in der App (lib/__tests__/vektoren/).
//
// Fest sind alle Schluessel (nur Tests!), K, die Werte und die Blaetter. Die
// Chiffrate fuer die Inhaberin sind zufaellig (ECIES) und darum einmal erzeugt
// und gespeichert; dieser Test prueft, dass Go sie liest und dass alle anderen
// Felder sich aus den festen Eingaben genau so ergeben. Neu schreiben:
// AEQUITAS_VEKTOR_SCHREIBEN=1 go test -run TestQuantenSInhaberinVektor .

type qsiVektorAbschnitt struct {
	Abschnitt           QuantenSPaketAbschnitt `json:"abschnitt"`
	X                   string                 `json:"x"`
	Nachricht           string                 `json:"nachricht"`
	Validator           string                 `json:"validator"`
	ValidatorSchluessel string                 `json:"validator_schluessel_nur_test"`
	BlattHashe          string                 `json:"blaetter_hash"`
}

type qsiVektor struct {
	Hinweis             string               `json:"hinweis"`
	ChainID             int64                `json:"chain_id"`
	InhaberinSchluessel string               `json:"inhaberin_schluessel_nur_test"`
	Inhaberin           string               `json:"inhaberin"`
	InhaberinPub        string               `json:"inhaberin_pub"`
	K                   string               `json:"k"`
	N                   int                  `json:"n"`
	V                   int                  `json:"v"`
	Abschnitte          []qsiVektorAbschnitt `json:"abschnitte"`
	Wurzel              string               `json:"wurzel"`
	Proben              [2]string            `json:"proben"`
	X                   string               `json:"x"`
	Y                   string               `json:"y"`
	Z                   int                  `json:"z"`
	D                   string               `json:"d"`
	BindenNachricht     string               `json:"binden_nachricht"`
	BindenSig           string               `json:"binden_sig"`
	Stelle              int                  `json:"stelle"`
	C                   string               `json:"c"`
	WahlNachricht       string               `json:"wahl_nachricht"`
	WahlSig             string               `json:"wahl_sig"`
	VorlageText         string               `json:"vorlage_text"`
	Vorlage             QuantenSVorlage      `json:"vorlage"`
	FalschFuerInhaberin string               `json:"falsch_fuer_inhaberin"` // fuer einen anderen Schluessel
}

const qsiVektorPfad = "testdata/quanten_s_inhaberin_vektor.json"

func qsiSchluessel(t *testing.T, b byte) string {
	t.Helper()
	return strings.Repeat(hex.EncodeToString([]byte{b}), 32)
}

// qsiVektorBauen: alle festen Felder; fi = gespeicherte Chiffrate (nil = neu).
func qsiVektorBauen(t *testing.T, fi []string, falsch string) qsiVektor {
	t.Helper()
	inh, err := crypto.HexToECDSA(qsiSchluessel(t, 0x51))
	if err != nil {
		t.Fatal(err)
	}
	val := []string{qsiSchluessel(t, 0x61), qsiSchluessel(t, 0x62)}
	v := qsiVektor{Hinweis: "nur fuer Tests: feste Schluessel, nie fuer echte Werte", ChainID: eingefroreneChainID,
		InhaberinSchluessel: qsiSchluessel(t, 0x51), Inhaberin: quantenSAdresse(inh), InhaberinPub: quantenSPubHex(inh),
		K: strings.Repeat("ab", 16) + strings.Repeat("cd", 16), N: 10, V: 2, Z: 1, Stelle: 1}
	xBits := []int{1, 0, 1, 1, 0, 0, 1, 0, 1, 1}
	yBits := []int{0, 1, 1, 0, 1, 0, 0, 1, 1, 0}
	var blaetter [][32]byte
	for i := 0; i < v.N; i++ {
		blaetter = append(blaetter, quantenWHash([]byte("blatt|"), []byte{byte(i)}))
	}
	var teile [2][][32]byte
	for xi := 1; xi <= v.V; xi++ {
		key, err := crypto.HexToECDSA(val[xi-1])
		if err != nil {
			t.Fatal(err)
		}
		von, bis, _ := quantenSAbschnitt(v.N, v.V, xi)
		ab := QuantenSPaketAbschnitt{X: xi, Von: von, Bis: bis, BuendelHash: quantenSHashListe([]string{"", "buendel"}),
			BuendelSigHash: quantenSHashListe([]string{"", "sig"}), Zaehlung: [][2]int{{2 + xi, 0}, {1 + xi, 0}}}
		for i := von; i < bis; i++ {
			ab.Blaetter = append(ab.Blaetter, hex.EncodeToString(blaetter[i][:]))
		}
		for s := 0; s < 2; s++ {
			h := quantenWHash([]byte("teil|"), []byte{byte(xi), byte(s)})
			ab.Teile = append(ab.Teile, hex.EncodeToString(h[:]))
			teile[s] = append(teile[s], h)
		}
		xs := qsBitsText(xBits[von:bis])
		if fi != nil {
			ab.FuerInhaberin = fi[xi-1]
		} else {
			roh, _ := json.Marshal(map[string]interface{}{"dom": quantenSDomFuerInhaberin, "k": v.K, "von": von, "bis": bis, "x": xs})
			c, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(&inh.PublicKey), roh, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			ab.FuerInhaberin = hex.EncodeToString(c)
		}
		m := quantenSAbschnittAntwortNachrichtAus(v.K, xi, von, bis, quantenSHashListe(ab.Blaetter), ab.FuerInhaberin, ab.BuendelHash, ab.BuendelSigHash, ab.Teile, ab.Zaehlung, ab.BasenHash)
		if ab.Sig, err = quantenSUnterschreiben(key, m); err != nil {
			t.Fatal(err)
		}
		v.Abschnitte = append(v.Abschnitte, qsiVektorAbschnitt{Abschnitt: ab, X: xs, Nachricht: m, Validator: quantenSAdresse(key),
			ValidatorSchluessel: val[xi-1], BlattHashe: quantenSHashListe(ab.Blaetter)})
	}
	w := quantenWMerkleWurzel(blaetter)
	v.Wurzel = hex.EncodeToString(w[:])
	for s := 0; s < 2; s++ {
		h, err := quantenSProbeHashV2(v.K, s, teile[s])
		if err != nil {
			t.Fatal(err)
		}
		v.Proben[s] = hex.EncodeToString(h[:])
	}
	v.X, v.Y = qsBitsText(xBits), qsBitsText(yBits)
	d := make([]int, v.N)
	for i := range yBits {
		d[i] = yBits[i] ^ v.Z
	}
	v.D = qsBitsText(d)
	v.BindenNachricht = quantenWSBindenNachricht(v.Inhaberin, v.K, v.D)
	if v.BindenSig, err = quantenSUnterschreiben(inh, v.BindenNachricht); err != nil {
		t.Fatal(err)
	}
	v.C = string(rune('0' + (v.Stelle ^ v.Z)))
	v.WahlNachricht = quantenWSWahlNachricht(v.Inhaberin, v.K, v.C)
	if v.WahlSig, err = quantenSUnterschreiben(inh, v.WahlNachricht); err != nil {
		t.Fatal(err)
	}
	v.VorlageText = quantenSVorlageText(v.K, v.Stelle, xBits)
	sig, err := quantenSUnterschreiben(inh, v.VorlageText)
	if err != nil {
		t.Fatal(err)
	}
	v.Vorlage = QuantenSVorlage{K: v.K, B: v.Stelle, X: v.X, Vorlage: sig}
	v.FalschFuerInhaberin = falsch
	if falsch == "" {
		anders, _ := crypto.HexToECDSA(qsiSchluessel(t, 0x52))
		roh, _ := json.Marshal(map[string]interface{}{"dom": quantenSDomFuerInhaberin, "k": v.K, "von": 0, "bis": 5, "x": "10110"})
		c, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(&anders.PublicKey), roh, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		v.FalschFuerInhaberin = hex.EncodeToString(c)
	}
	return v
}

func TestQuantenSInhaberinVektor(t *testing.T) {
	if os.Getenv("AEQUITAS_VEKTOR_SCHREIBEN") == "1" {
		v := qsiVektorBauen(t, nil, "")
		roh, _ := json.MarshalIndent(v, "", "  ")
		if err := os.WriteFile(qsiVektorPfad, append(roh, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("geschrieben:", qsiVektorPfad)
	}
	roh, err := os.ReadFile(qsiVektorPfad)
	if err != nil {
		t.Fatal(err)
	}
	var gelesen qsiVektor
	dec := json.NewDecoder(strings.NewReader(string(roh)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&gelesen); err != nil {
		t.Fatal(err)
	}
	var fi []string
	for _, a := range gelesen.Abschnitte {
		fi = append(fi, a.Abschnitt.FuerInhaberin)
	}
	soll := qsiVektorBauen(t, fi, gelesen.FalschFuerInhaberin)
	a, _ := json.Marshal(soll)
	b, _ := json.Marshal(gelesen)
	if string(a) != string(b) {
		t.Fatalf("Vektor weicht von dem ab, was der Knoten rechnet:\n%s\n%s", a, b)
	}
	// Go liest die gespeicherten Chiffrate und prueft die Unterschriften --
	// derselbe Weg wie quantenSInhaberinAnnehmen.
	inh, _ := crypto.HexToECDSA(gelesen.InhaberinSchluessel)
	for i, ab := range gelesen.Abschnitte {
		if err := quantenSPaketAbschnittPruefen(ab.Abschnitt, gelesen.K, gelesen.N, gelesen.V, i+1, ab.Validator); err != nil {
			t.Fatal(err)
		}
		xs, err := quantenSFuerInhaberinOeffnen(inh, ab.Abschnitt.FuerInhaberin, gelesen.K, ab.Abschnitt.Von, ab.Abschnitt.Bis)
		if err != nil || qsBitsText(xs) != ab.X {
			t.Fatalf("Abschnitt %d: %v %v", i+1, xs, err)
		}
	}
	if _, err := quantenSFuerInhaberinOeffnen(inh, gelesen.FalschFuerInhaberin, gelesen.K, 0, 5); err == nil {
		t.Fatal("Chiffrat fuer einen anderen Schluessel gelesen")
	}
	for _, p := range [][2]string{{gelesen.BindenNachricht, gelesen.BindenSig}, {gelesen.WahlNachricht, gelesen.WahlSig}, {gelesen.VorlageText, gelesen.Vorlage.Vorlage}} {
		if !kanonischeSignatur(p[1]) || verifyPersonalSign(p[0], p[1], gelesen.Inhaberin) != nil {
			t.Fatalf("Unterschrift der Inhaberin: %q", p[0])
		}
	}
}
