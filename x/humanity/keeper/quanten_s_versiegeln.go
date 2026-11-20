package keeper

// QUANTENGELD VARIANTE S -- VERSIEGELTE MESSUNG (Transport zum Validator)
//
// Die Messung eines Abschnitts (u, t, y, x) ist die Praeparation des Tokens.
// Wer sie hat und zugleich Inhaberin ist, koennte an beiden Annahmestellen
// bestehen. Der Mess-Job des Workflows (quanten-s-ausgabe.yml, Umgebung
// quantenhardware) versiegelt sie darum mit forschung/quantengeld/
// s_versiegeln.py auf den Schluessel des Validators, der den Abschnitt
// praepariert. Durch GitHub (Job-Ausgaben) geht nur das Chiffrat; erst der
// Knoten des Validators oeffnet es hier wieder und legt den Datensatz in sein
// Messverzeichnis (QuantenSMessungOeffnen, aufgerufen von
// deploy/quanten-s-ausgabe-abschnitt.sh).
//
// Verfahren: ECIES wie go-ethereum (crypto/ecies), Inhalt
// {"dom": quantenSMessungDom, "an": <Adresse>, "messung": <Datensatz>}.
// Fail-closed: falscher Schluessel, veraendertes Chiffrat, fremde Domaene,
// fremder Empfaenger, unbekannte oder fehlende Felder, kaputte Bits, andere
// Laenge oder andere Basen als verlangt -- dann kein Datensatz.

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
)

const quantenSMessungDom = "aequitas-qs-messung-v1"

// quantenSVersiegeltMax: Hex-Text des Chiffrats; ein Datensatz hat hoechstens
// quantenSMessEintragMax Byte (Hex doppelt, dazu Huelle und ECIES).
const quantenSVersiegeltMax = 2*quantenSMessEintragMax + 4096

type quantenSVersiegelt struct {
	Dom     string                `json:"dom"`
	An      string                `json:"an"`
	Messung quantenSMessDatensatz `json:"messung"`
}

// quantenSMessungOeffnen: der Datensatz aus dem Chiffrat, nur fuer diesen
// Validator, nur in n Stellen und nur zu den Basen mit sha256 = name (Hex,
// wie quantenSMessName ohne ".json").
func quantenSMessungOeffnen(key *ecdsa.PrivateKey, chiffratHex string, n int, name string) (*quantenSMessDatensatz, error) {
	if key == nil {
		return nil, errors.New("kein Schluessel")
	}
	if n < 2 || n > quantenWNMax {
		return nil, errors.New("n")
	}
	if len(chiffratHex) > quantenSVersiegeltMax {
		return nil, errors.New("Chiffrat zu gross")
	}
	text := strings.TrimSpace(chiffratHex)
	roh, err := hex.DecodeString(text)
	if err != nil {
		return nil, errors.New("Chiffrat: kein Hex")
	}
	klar, err := ecies.ImportECDSA(key).Decrypt(roh, nil, nil)
	if err != nil {
		return nil, errors.New("Chiffrat nicht fuer diesen Schluessel oder veraendert")
	}
	var v quantenSVersiegelt
	if err := quantenSStrengLesen(klar, &v); err != nil {
		return nil, err
	}
	if v.Dom != quantenSMessungDom {
		return nil, errors.New("Inhalt: fremde Domaene")
	}
	if v.An != quantenSAdresse(key) {
		return nil, errors.New("Inhalt: fuer einen anderen Validator")
	}
	m := v.Messung
	for _, b := range []string{m.U, m.T, m.Y, m.X} {
		if _, ok := quantenSBitsLesen(b, n); !ok {
			return nil, fmt.Errorf("Messung: keine %d Bits", n)
		}
	}
	if h := sha256.Sum256([]byte(m.Y)); hex.EncodeToString(h[:]) != name {
		return nil, errors.New("Messung: andere Basen als verlangt")
	}
	if len(m.Wo) > 512 || len(m.Job) > 512 {
		return nil, errors.New("Messung: wo oder job zu lang")
	}
	return &m, nil
}

// quantenSStrengLesen: genau ein JSON-Wert, keine unbekannten Felder (in der
// Huelle und im Datensatz), und wo und job vorhanden (sonst stuende "" fuer
// ein fehlendes Feld). Fehlen dom, an oder eine Bitkette, faellt das an deren
// eigener Pruefung auf.
func quantenSStrengLesen(roh []byte, ziel *quantenSVersiegelt) error {
	var v struct {
		Dom     string `json:"dom"`
		An      string `json:"an"`
		Messung struct {
			U   string  `json:"u"`
			T   string  `json:"t"`
			Y   string  `json:"y"`
			X   string  `json:"x"`
			Wo  *string `json:"wo"`
			Job *string `json:"job"`
		} `json:"messung"`
	}
	dec := json.NewDecoder(bytes.NewReader(roh))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil || dec.More() {
		return errors.New("Inhalt unlesbar (genau dom, an, messung mit u, t, y, x, wo, job)")
	}
	m := v.Messung
	if m.Wo == nil || m.Job == nil {
		return errors.New("Messung: wo und job fehlen")
	}
	*ziel = quantenSVersiegelt{Dom: v.Dom, An: v.An, Messung: quantenSMessDatensatz{U: m.U, T: m.T, Y: m.Y, X: m.X, Wo: *m.Wo, Job: *m.Job}}
	return nil
}

// QuantenSMessungOeffnen: `aequitasd quanten-s-messung-oeffnen --n N --name H
// --verzeichnis D` < Chiffrat (Hex). Legt den Datensatz als D/H.json ab (wie
// ihn das Messverzeichnis liest) und gibt nur den Namen aus.
//
// Das Ablegen tut der Knotenbenutzer selbst (im Container), nicht root auf dem
// Server: Das Messverzeichnis gehoert dem Knotenbenutzer, und ein root, der
// darin Dateien anlegt, aendert und verlinkt, folgte einem dort untergeschobenen
// Verweis. Hier: das Verzeichnis darf kein Verweis sein; erst unter eigenem
// Zufallsnamen (O_EXCL, 0600, auf die Platte), dann per Link auf den
// endgueltigen Namen -- nie ueberschrieben, nie halb sichtbar, nie einem
// Verweis gefolgt.
func QuantenSMessungOeffnen(schluesselHex string, ein io.Reader, n int, name, verzeichnis string, aus io.Writer) error {
	key, err := crypto.HexToECDSA(quantenSSchluesselText(schluesselHex))
	if err != nil {
		return errors.New("Knotenschluessel unlesbar") // ohne den Schluessel im Text
	}
	// Der Name wird erst Teil eines Pfads, nachdem quantenSMessungOeffnen ihn
	// als sha256 der Basen bestaetigt hat (64 kleine Hex-Zeichen).
	name = strings.ToLower(strings.TrimSpace(name))
	if info, err := os.Lstat(verzeichnis); err != nil || !info.IsDir() {
		return errors.New("Messverzeichnis: kein Verzeichnis (oder ein Verweis)")
	}
	// Hoechstens ein Byte ueber der Grenze lesen; quantenSMessungOeffnen
	// weist dann ab, bevor es entschluesselt.
	roh, err := io.ReadAll(io.LimitReader(ein, quantenSVersiegeltMax+1))
	if err != nil {
		return err
	}
	m, err := quantenSMessungOeffnen(key, string(roh), n, name)
	if err != nil {
		return err
	}
	text, err := json.Marshal(m)
	if err != nil {
		return err
	}
	neu, err := quantenSDateiNeu(verzeichnis, filepath.Join(verzeichnis, name+".json"), append(text, '\n'))
	if err != nil {
		return err
	}
	if !neu {
		return errors.New("zu diesen Basen liegt schon eine Messung (nicht ueberschrieben)")
	}
	_, err = fmt.Fprintln(aus, name+".json")
	return err
}

// quantenSZufallsName: Name der Zwischendatei (im Test ersetzbar, um einen
// untergeschobenen Verweis unter genau diesem Namen zu pruefen).
var quantenSZufallsName = func() (string, error) {
	var z [8]byte
	if _, err := rand.Read(z[:]); err != nil {
		return "", err
	}
	return ".neu-" + hex.EncodeToString(z[:]), nil
}

// quantenSDateiNeu: roh als neue Datei ziel in dir -- erst unter eigenem
// Zufallsnamen (O_EXCL: scheitert auch an einem Verweis), 0600, auf die Platte,
// dann per Link auf ziel (scheitert, wenn es ziel schon gibt, auch als
// Verweis). false: ziel war schon vergeben. Wie quantenSVormerkAnlegen.
func quantenSDateiNeu(dir, ziel string, roh []byte) (bool, error) {
	zname, err := quantenSZufallsName()
	if err != nil {
		return false, err
	}
	tmp := filepath.Join(dir, zname)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, errors.New("Messung anlegen: nicht moeglich")
	}
	defer os.Remove(tmp)
	_, werr := f.Write(roh)
	serr := f.Sync()
	cerr := f.Close()
	if werr != nil || serr != nil || cerr != nil {
		return false, errors.New("Messung schreiben: nicht moeglich")
	}
	if err := os.Link(tmp, ziel); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return false, nil
		}
		return false, errors.New("Messung ablegen: nicht moeglich")
	}
	return true, nil
}
