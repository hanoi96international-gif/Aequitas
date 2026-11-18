package keeper

// QUANTENGELD VARIANTE S -- AUSGABE IN ABSCHNITTEN (Weg A, Stufe A2a)
//
// Spezifikation: forschung/quantengeld/s_kette.py (abschnitte, Praeparator,
// stellen_bit, stelle_laden_bit), Begruendung: forschung/quantengeld/
// DEZENTRAL.md 2, 7, 8. Pruefvektor: testdata/quanten_s_abschnitt_vektor.json
// (s_kette.py --vektor-abschnitte), quanten_s_abschnitt_test.go haelt beide
// gleich.
//
// WARUM: Die Pruefgroesse k (quantenWKTabelle) setzt voraus, dass weniger als
// t Validatoren hoechstens den Anteil (t-1)/v der Stellen kennen. Die erste
// Ausgabe praeparierte alle Stellen in EINEM Auftrag -- wer ihn absetzt, kennt
// den ganzen Token. Hier praepariert jeder Validator SEINEN Abschnitt selbst,
// teilt dessen Geheimnis selbst auf und schickt jedem anderen Validator dessen
// Anteile VERSCHLUESSELT (ECIES an seinen secp256k1-Schluessel) und
// UNTERSCHRIEBEN ueber den Ausgeber, der nur weiterleitet.
//
// STELLEN-BIT: das unterste Bit des Salzes (Bit 2 des Werts) sagt, zu welcher
// der zwei Annahmestellen die Stelle gehoert -- je Abschnitt genau zur Haelfte.
// Die Pruefsumme bindet es; die Kette sieht nur Salz und aendert sich nicht.
// Annahmestelle b bekommt nur die Werte mit Bit b und prueft selbst
// (quantenSOeffnungLaden). So sind die Stichproben disjunkt, und niemand kennt
// beide -- kennte die Inhaberin beide vor der Bindung, kaeme sie an beiden
// Stellen durch (s_kette.py, Selbsttest 5g).
//
// Hier stehen nur reine Funktionen ohne Netz; Dienst und Endpunkte folgen (A2b).

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
)

const (
	quantenSDomBuendel = "aequitas-quanten-s-buendel-v1"
	// Ein Buendel traegt hoechstens die Anteile eines ganzen Tokens
	// (quantenWNMax Stellen zu hoechstens 32 Hex-Zeichen), dazu Felder und den
	// Aufschlag von ECIES; als Hex doppelt so lang.
	quantenSBuendelHoechstens = 2 * (quantenWNMax*36 + 4096)
)

// quantenSAbschnitt: [von, bis) des Validators x (1-basiert) bei n Stellen und
// v Validatoren -- zusammenhaengend, hoechstens eine Stelle Unterschied
// (s_kette.abschnitte).
func quantenSAbschnitt(n, v, x int) (von, bis int, ok bool) {
	if v < 1 || n < v || x < 1 || x > v || n > quantenWNMax || v > quantenWVMax {
		return 0, 0, false
	}
	return (x - 1) * n / v, x * n / v, true
}

// quantenSStellenBit: Bit 2 des gesalzenen Werts (s_kette.stellen_bit).
func quantenSStellenBit(w *big.Int) int { return int(w.Bit(2)) }

// quantenSAbschnittWerte: zu Basen u und Werten tw eines Abschnitts die
// gesalzenen Werte w = u + 2 t + 4 (Stellen-Bit + 2 Salz) mit 123 Bit Salz.
// Die Stellen-Bits sind je Abschnitt genau zur Haelfte verteilt (bei
// ungerader Laenge eine mehr fuer Stelle 1), in zufaelliger Reihenfolge.
func quantenSAbschnittWerte(u, tw []int) ([]*big.Int, error) {
	n := len(u)
	if n == 0 || len(tw) != n {
		return nil, quantenWFehler("Abschnitt: Basen und Werte")
	}
	reihe := make([]int, n)
	for i := range reihe {
		reihe[i] = i
	}
	for i := n - 1; i > 0; i-- { // Fisher-Yates mit kryptographischem Zufall
		j, err := quantenSZufallInt(i + 1)
		if err != nil {
			return nil, err
		}
		reihe[i], reihe[j] = reihe[j], reihe[i]
	}
	bit := make([]int, n)
	for j, i := range reihe {
		if j >= n/2 {
			bit[i] = 1
		}
	}
	salzGrenze := new(big.Int).Lsh(big.NewInt(1), quantenWSalzBits-1)
	out := make([]*big.Int, n)
	for i := 0; i < n; i++ {
		if (u[i] != 0 && u[i] != 1) || (tw[i] != 0 && tw[i] != 1) {
			return nil, quantenWFehler("Abschnitt: Basis oder Wert %d", i)
		}
		s, err := rand.Int(rand.Reader, salzGrenze)
		if err != nil {
			return nil, err
		}
		w := new(big.Int).Lsh(s, 1)
		w.Add(w, big.NewInt(int64(bit[i])))
		w.Lsh(w, 2).Add(w, big.NewInt(int64(u[i]+2*tw[i])))
		out[i] = w
	}
	return out, nil
}

// quantenSTeilen: Shamir t-von-v ueber F_P (wie quantenSPraegen); Anteil des
// Validators x (1-basiert) steht an [x-1].
func quantenSTeilen(w *big.Int, t, v int) ([]*big.Int, error) {
	if t < 1 || t > v || v > quantenWVMax || w.Sign() < 0 || w.Cmp(quantenWP) >= 0 {
		return nil, quantenWFehler("Teilen: Schwelle oder Wert")
	}
	koeff := []*big.Int{w}
	for j := 1; j < t; j++ {
		c, err := rand.Int(rand.Reader, quantenWP)
		if err != nil {
			return nil, err
		}
		koeff = append(koeff, c)
	}
	out := make([]*big.Int, v)
	for x := 1; x <= v; x++ {
		y := new(big.Int)
		for j := len(koeff) - 1; j >= 0; j-- {
			y.Mul(y, big.NewInt(int64(x))).Add(y, koeff[j]).Mod(y, quantenWP)
		}
		out[x-1] = y
	}
	return out, nil
}

// quantenSSchluesselPruefen: ein oeffentlicher Schluessel (hex, unkomprimiert
// 65 Byte) muss zur Adresse gehoeren -- sonst koennte ein Ausgeber die Anteile
// eines Validators an sich selbst verschluesseln lassen.
func quantenSSchluesselPruefen(pubHex, adresse string) (*ecdsa.PublicKey, error) {
	roh, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(pubHex), "0x"))
	if err != nil || len(roh) != 65 {
		return nil, quantenWFehler("Schluessel: Form")
	}
	pub, err := crypto.UnmarshalPubkey(roh)
	if err != nil {
		return nil, quantenWFehler("Schluessel: kein Punkt der Kurve")
	}
	if !strings.EqualFold(crypto.PubkeyToAddress(*pub).Hex(), strings.TrimSpace(adresse)) {
		return nil, quantenWFehler("Schluessel gehoert nicht zur Adresse")
	}
	return pub, nil
}

// quantenSBuendelInhalt: die Anteile eines Abschnitts fuer EINEN Empfaenger.
type quantenSBuendelInhalt struct {
	Dom         string   `json:"dom"`
	K           string   `json:"k"`
	Von         int      `json:"von"`
	Bis         int      `json:"bis"`
	Praeparator int      `json:"praeparator"`
	Empfaenger  int      `json:"empfaenger"`
	Anteile     []string `json:"anteile"`
}

// quantenSBuendelNachricht: was der Praeparator ueber das Chiffrat
// unterschreibt -- der Ausgeber kann es weder lesen noch tauschen.
func quantenSBuendelNachricht(K string, praeparator, empfaenger int, chiffrat string) string {
	h := sha256.Sum256([]byte(chiffrat))
	return fmt.Sprintf("%s|chain:%d|%s|%d|%d|%x", quantenSDomBuendel, eingefroreneChainID, K, praeparator, empfaenger, h)
}

// quantenSBuendelVersiegeln: verschluesseln fuer den Empfaenger, unterschreiben
// vom Praeparator.
func quantenSBuendelVersiegeln(praep *ecdsa.PrivateKey, empf *ecdsa.PublicKey, in quantenSBuendelInhalt) (chiffrat, sig string, err error) {
	in.Dom = quantenSDomBuendel
	roh, err := json.Marshal(in)
	if err != nil {
		return "", "", err
	}
	c, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(empf), roh, nil, nil)
	if err != nil {
		return "", "", err
	}
	chiffrat = hex.EncodeToString(c)
	sig, err = quantenSUnterschreiben(praep, quantenSBuendelNachricht(in.K, in.Praeparator, in.Empfaenger, chiffrat))
	return chiffrat, sig, err
}

// quantenSBuendelOeffnen: Unterschrift des Praeparators pruefen, entschluesseln,
// streng lesen. Fail-closed: zu gross, fremde Unterschrift, fremder Empfaenger,
// anderes K oder anderer Abschnitt, falsche Zahl oder Form der Anteile.
func quantenSBuendelOeffnen(empf *ecdsa.PrivateKey, chiffrat, sig, praepAdresse, K string, praeparator, empfaenger, von, bis int) ([]*big.Int, error) {
	if len(chiffrat) == 0 || len(chiffrat) > quantenSBuendelHoechstens || !kleinHex(chiffrat) {
		return nil, quantenWFehler("Buendel: Form oder Groesse")
	}
	if !kanonischeSignatur(sig) || verifyPersonalSign(quantenSBuendelNachricht(K, praeparator, empfaenger, chiffrat), sig, praepAdresse) != nil {
		return nil, quantenWFehler("Buendel: nicht vom Praeparator %d unterschrieben", praeparator)
	}
	c, err := hex.DecodeString(chiffrat)
	if err != nil {
		return nil, quantenWFehler("Buendel: Hex")
	}
	roh, err := ecies.ImportECDSA(empf).Decrypt(c, nil, nil)
	if err != nil {
		return nil, quantenWFehler("Buendel: nicht fuer diesen Validator")
	}
	var in quantenSBuendelInhalt
	dec := json.NewDecoder(bytes.NewReader(roh))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || dec.More() {
		return nil, quantenWFehler("Buendel: Inhalt unlesbar")
	}
	if in.Dom != quantenSDomBuendel || in.K != K || in.Von != von || in.Bis != bis || in.Praeparator != praeparator ||
		in.Empfaenger != empfaenger || len(in.Anteile) != bis-von {
		return nil, quantenWFehler("Buendel: passt nicht zu K, Abschnitt oder Empfaenger")
	}
	out := make([]*big.Int, len(in.Anteile))
	for i, y := range in.Anteile {
		if len(y) == 0 || len(y) > 32 || !kleinHex(y) {
			return nil, quantenWFehler("Buendel: Anteil %d", i)
		}
		v, ok := new(big.Int).SetString(y, 16)
		if !ok || v.Cmp(quantenWP) >= 0 {
			return nil, quantenWFehler("Buendel: Anteil %d", i)
		}
		out[i] = v
	}
	return out, nil
}

// quantenSOeffnungLaden: die Annahmestelle b laedt ihre Stichprobe aus den
// Oeffnungen der Praeparatoren und prueft selbst (s_kette.stelle_laden_bit):
// die Blaetter ergeben die Wurzel, jeder Wert passt zu seinem Blatt, sein
// Stellen-Bit ist b, mindestens kSoll Stellen. Gibt (Basis, Wert) je Stelle.
func quantenSOeffnungLaden(nummer int, K string, n int, wurzel [32]byte, blaetter [][32]byte, oeffnung map[int]*big.Int, kSoll int) (map[int][2]int, error) {
	if (nummer != 0 && nummer != 1) || len(blaetter) != n || n > quantenWNMax || quantenWMerkleWurzel(blaetter) != wurzel {
		return nil, quantenWFehler("Ladung: Blaetter passen nicht zur Wurzel")
	}
	if kSoll <= 0 || len(oeffnung) < kSoll {
		return nil, quantenWFehler("Ladung: Stichprobe zu klein (%d Stellen, verlangt %d)", len(oeffnung), kSoll)
	}
	grenze := new(big.Int).Lsh(big.NewInt(1), quantenWSalzBits+2)
	out := make(map[int][2]int, len(oeffnung))
	for i, w := range oeffnung {
		if i < 0 || i >= n || w == nil || w.Sign() < 0 || w.Cmp(grenze) >= 0 || quantenWPruefsumme(K, i, w) != blaetter[i] {
			return nil, quantenWFehler("Ladung: Stelle %d passt nicht zu ihrem Blatt", i)
		}
		if quantenSStellenBit(w) != nummer {
			return nil, quantenWFehler("Ladung: Stelle %d gehoert zur anderen Annahmestelle", i)
		}
		b, t := quantenWZerlegen(w)
		out[i] = [2]int{b, t}
	}
	return out, nil
}
