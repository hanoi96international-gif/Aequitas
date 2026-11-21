package keeper

// BASEN y JE VALIDATOR VERSCHLUESSELT (forschung/quantengeld/DEZENTRAL.md 17, R8)
//
// Bisher stand y der Inhaberin im Klartext in der Bestellung, und der Ausgeber
// reichte es an die Validatoren weiter. Mit d = y XOR z auf der Kette kannte er
// damit z und aus c die gewaehlte Stelle, bevor die Inhaberin vorlegte -- also
// WO sie einloesen will (nicht x; einloesen oder binden kann er nicht).
//
// Jetzt verschluesselt die Inhaberin y je Abschnitt an den Schluessel des
// Validators, der diesen Abschnitt praepariert (ECIES wie die Buendel und
// "fuer_inhaberin"). Der Ausgeber reicht nur das Chiffrat weiter: in der
// Vormerkung (quanten_s_vormerken.go) und im Abschnitts-Auftrag
// (quanten_s_abschnitt_dienst.go). Der Validator oeffnet es mit seinem eigenen
// Schluessel.
//
// GEBUNDEN: Im Klartext stehen Domaene, Kette, Empfaenger (Adresse des
// Validators), n, v, Nummer X, der Hash der ganzen Validatorenliste und die
// Inhaberin. Der Validator prueft jedes Feld gegen den Auftrag. So laesst sich
// ein Chiffrat weder bei einem anderen Validator noch fuer einen anderen
// Abschnitt noch fuer eine andere Inhaberin verwenden (sonst koennte der
// Ausgeber als "Inhaberin" eines eigenen Tokens die Messung in ihren Basen
// verbrauchen und aus seinem Nachweis Teile von y lernen) -- und auch nicht in
// einem Auftrag, in dem der Ausgeber eigene Adressen als uebrige Validatoren
// nennt (dann bekaeme er Anteile und Zaehlungen zu ihren Basen; aus der
// Sicherheitspruefung von R8).
//
// GEGEN TAUSCH: Der Validator unterschreibt in seiner Antwort den Hash des
// Chiffrats, das er geoeffnet hat (quantenSAbschnittAntwortNachrichtAus,
// "|basen:"). Die Inhaberin vergleicht ihn mit ihrem eigenen Chiffrat. Hat der
// Ausgeber ein anderes eingesetzt (eigene Basen, an denselben Validator
// verschluesselt), faellt das beim Annehmen auf -- vor dem Binden.
//
// GRENZEN: Chiffrat hoechstens quantenSBasenChiffratMax Hex-Zeichen, nur
// Kleinbuchstaben-Hex, Inhalt streng (genau diese Felder, nichts danach).
// Geoeffnet wird erst nach allen billigen Pruefungen und nach der Unterschrift
// des Ausgebers. Fail-closed: jedes Abweichen ist ein Fehler, nie ein
// Rueckfall auf Klartext.
//
// EHRLICH: Wer misst, kennt y. Misst der Validator auf seinem eigenen Server
// (A5, Vormerken), bleibt y beim Validator. Im Hauptnetz misst heute ein
// GitHub-Lauf des Betreibers fuer C1; dort braucht der Lauf y im Klartext,
// darum nimmt `quanten-s-ausgabe.yml` (Weg rollen) weiter nur Klartext an.
// Der Anbieter des Quantenrechners sieht y in jedem Fall (DEZENTRAL.md 4).

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/ethereum/go-ethereum/crypto/ecies"
)

const (
	quantenSDomBasen = "aequitas-quanten-s-basen-v1"
	// quantenSBasenChiffratMax: Hex-Zeichen eines Chiffrats. Klartext: JSON mit
	// hoechstens quantenWNMax Bits und festen Feldern (unter 512 Byte);
	// ECIES legt 65 + 16 + 32 Byte dazu.
	quantenSBasenChiffratMax = 2 * (quantenWNMax + 1024)
)

// quantenSBasenInhalt: was im Chiffrat steht.
type quantenSBasenInhalt struct {
	Dom   string `json:"dom"`
	Chain int64  `json:"chain"`
	An    string `json:"an"`
	N     int    `json:"n"`
	V     int    `json:"v"`
	X     int    `json:"x"`
	// Validatoren: sha256 der Liste (klein, mit Komma verbunden).
	Validatoren string `json:"validatoren"`
	Inhaberin   string `json:"inhaberin"`
	Basen       string `json:"basen"`
}

// quantenSValidatorenHash: woran das Chiffrat die Validatorenliste bindet.
func quantenSValidatorenHash(validatoren []string) string {
	l := make([]string, len(validatoren))
	for i, a := range validatoren {
		l[i] = strings.ToLower(strings.TrimSpace(a))
	}
	return quantenSHashListe(l)
}

// quantenSBasenVerschluesseln: die Inhaberin verschluesselt y ihres Abschnitts
// X an dessen Validator validatoren[X-1] (oeffentlicher Schluessel pub). Der
// Schluessel muss zur Adresse gehoeren.
func quantenSBasenVerschluesseln(pubHex string, validatoren []string, n, x int, inhaberin, basen string) (string, error) {
	v := len(validatoren)
	if x < 1 || x > v {
		return "", quantenWFehler("Nummer %d bei %d Validatoren", x, v)
	}
	an := strings.ToLower(strings.TrimSpace(validatoren[x-1]))
	pub, err := quantenSSchluesselPruefen(pubHex, an)
	if err != nil {
		return "", err
	}
	von, bis, ok := quantenSAbschnitt(n, v, x)
	if !ok {
		return "", quantenWFehler("Abschnitte")
	}
	if _, ok := quantenSBitsLesen(basen, bis-von); !ok {
		return "", quantenWFehler("Basen: %d Bits verlangt, nur 0 und 1", bis-von)
	}
	roh, err := json.Marshal(quantenSBasenInhalt{Dom: quantenSDomBasen, Chain: eingefroreneChainID, An: an,
		N: n, V: v, X: x, Validatoren: quantenSValidatorenHash(validatoren), Inhaberin: strings.ToLower(inhaberin), Basen: basen})
	if err != nil {
		return "", err
	}
	c, err := ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(pub), roh, nil, nil)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(c), nil
}

// quantenSBasenChiffratForm: nur die Form (ohne Schluessel) -- fuer Bestellung
// und Ausgeber, die das Chiffrat nicht oeffnen koennen.
func quantenSBasenChiffratForm(c string) bool {
	return len(c) > 0 && len(c) <= quantenSBasenChiffratMax && len(c)%2 == 0 && kleinHex(c)
}

// quantenSBasenHash: was der Validator in seiner Antwort unterschreibt.
func quantenSBasenHash(c string) string {
	h := sha256.Sum256([]byte(c))
	return hex.EncodeToString(h[:])
}

// quantenSBasenOeffnen: der Validator (Schluessel key, Adresse an) oeffnet das
// Chiffrat und prueft jedes Feld gegen den Auftrag (Validatorenliste, Nummer
// x, Inhaberin). Heraus kommt y als Text.
func quantenSBasenOeffnen(key *ecdsa.PrivateKey, an string, c string, n int, validatoren []string, x int, inhaberin string) (string, error) {
	v := len(validatoren)
	if !quantenSBasenChiffratForm(c) {
		return "", quantenWFehler("verschluesselte Basen: Form oder Groesse")
	}
	von, bis, ok := quantenSAbschnitt(n, v, x)
	if !ok {
		return "", quantenWFehler("Abschnitte")
	}
	roh, _ := hex.DecodeString(c)
	klar, err := ecies.ImportECDSA(key).Decrypt(roh, nil, nil)
	if err != nil {
		return "", quantenWFehler("verschluesselte Basen: nicht fuer diesen Validator")
	}
	var in quantenSBasenInhalt
	dec := json.NewDecoder(bytes.NewReader(klar))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || dec.More() {
		return "", quantenWFehler("verschluesselte Basen: Inhalt unlesbar")
	}
	if in.Dom != quantenSDomBasen || in.Chain != eingefroreneChainID || in.An != strings.ToLower(an) {
		return "", quantenWFehler("verschluesselte Basen: Domaene, Kette oder Empfaenger")
	}
	if in.N != n || in.V != v || in.X != x || in.Validatoren != quantenSValidatorenHash(validatoren) {
		return "", quantenWFehler("verschluesselte Basen: fuer einen anderen Abschnitt")
	}
	if in.Inhaberin != strings.ToLower(strings.TrimSpace(inhaberin)) {
		return "", quantenWFehler("verschluesselte Basen: fuer eine andere Inhaberin")
	}
	if _, ok := quantenSBitsLesen(in.Basen, bis-von); !ok {
		return "", quantenWFehler("verschluesselte Basen: %d Bits verlangt, nur 0 und 1", bis-von)
	}
	return in.Basen, nil
}
