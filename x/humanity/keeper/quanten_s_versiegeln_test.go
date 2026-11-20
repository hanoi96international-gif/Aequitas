package keeper

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
)

// Der Vektor kommt aus forschung/quantengeld/s_versiegeln.py
// (--vektor-schreiben, fester ephemerer Schluessel und IV); der Selbsttest
// dort rechnet ihn byte-genau nach. Hier liest Go ihn: dieselbe Huelle, dasselbe
// ECIES wie go-ethereum.

type qsvVektor struct {
	An         string                `json:"an"`
	Chiffrat   string                `json:"chiffrat"`
	Klartext   string                `json:"klartext"`
	Messung    quantenSMessDatensatz `json:"messung"`
	Name       string                `json:"name"`
	Schluessel string                `json:"validator_schluessel_nur_test"`
	Pub        string                `json:"validator_pub"`
}

func qsvLesen(t *testing.T) qsvVektor {
	t.Helper()
	roh, err := os.ReadFile("testdata/quanten_s_versiegelt_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v qsvVektor
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func qsvVerschluesseln(t *testing.T, pub *ecies.PublicKey, klar string) string {
	t.Helper()
	c, err := ecies.Encrypt(rand.Reader, pub, []byte(klar), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(c)
}

func TestQuantenSMessungOeffnenVektor(t *testing.T) {
	v := qsvLesen(t)
	key, err := crypto.HexToECDSA(v.Schluessel)
	if err != nil {
		t.Fatal(err)
	}
	if quantenSAdresse(key) != v.An || hex.EncodeToString(crypto.FromECDSAPub(&key.PublicKey)) != v.Pub {
		t.Fatalf("Adresse oder Schluessel aus Python weichen ab: %s %s", quantenSAdresse(key), v.An)
	}
	roh, _ := hex.DecodeString(v.Chiffrat)
	klar, err := ecies.ImportECDSA(key).Decrypt(roh, nil, nil)
	if err != nil || string(klar) != v.Klartext {
		t.Fatalf("Python-Chiffrat nicht gelesen: %v", err)
	}
	var aus bytes.Buffer
	dir := t.TempDir()
	if err := QuantenSMessungOeffnen(v.Schluessel, strings.NewReader(v.Chiffrat+"\n"), 8, v.Name, dir, &aus); err != nil {
		t.Fatal(err)
	}
	// Ausgegeben wird nur der Name, nie der Inhalt.
	if aus.String() != v.Name+".json\n" || quantenSMessName(v.Messung.Y) != v.Name+".json" {
		t.Fatalf("Ausgabe %q", aus.String())
	}
	pfad := filepath.Join(dir, v.Name+".json")
	info, err := os.Lstat(pfad)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("abgelegt: %v %v", info, err)
	}
	if e, _ := os.ReadDir(dir); len(e) != 1 {
		t.Fatalf("Reste im Verzeichnis: %v", e)
	}
	// Genau das, was der Dienst liest: abschnitt() nimmt die Datei an und
	// entfernt sie.
	mv := &quantenSMessVerzeichnis{dir: dir}
	y, _ := quantenSBitsLesen(v.Messung.Y, 8)
	u, tt, x, wo, err := mv.abschnitt(y)
	if err != nil || qsBitsText(u) != v.Messung.U || qsBitsText(tt) != v.Messung.T || qsBitsText(x) != v.Messung.X || wo != "Pruefvektor vektor-1" {
		t.Fatalf("Dienst liest die abgelegte Messung nicht: %v %v", wo, err)
	}
}

func TestQuantenSMessungAblegenMissbrauch(t *testing.T) {
	v := qsvLesen(t)
	lauf := func(dir, name, chiffrat string) (string, error) {
		var aus bytes.Buffer
		err := QuantenSMessungOeffnen(v.Schluessel, strings.NewReader(chiffrat), 8, name, dir, &aus)
		return aus.String(), err
	}
	leer := func(t *testing.T, dir string, ausser ...string) {
		t.Helper()
		e, _ := os.ReadDir(dir)
		if len(e) != len(ausser) {
			t.Fatalf("Reste im Verzeichnis: %v", e)
		}
	}
	// Es liegt schon eine Messung: nicht ueberschrieben.
	dir := t.TempDir()
	ziel := filepath.Join(dir, v.Name+".json")
	os.WriteFile(ziel, []byte("alt"), 0o600)
	if aus, err := lauf(dir, v.Name, v.Chiffrat); err == nil || aus != "" {
		t.Fatal("vorhandene Messung ueberschrieben")
	}
	if roh, _ := os.ReadFile(ziel); string(roh) != "alt" {
		t.Fatal("vorhandene Messung veraendert")
	}
	leer(t, dir, "alt")
	// Unter dem Zielnamen liegt ein Verweis: ihm wird nicht gefolgt.
	dir = t.TempDir()
	draussen := filepath.Join(t.TempDir(), "opfer")
	os.WriteFile(draussen, []byte("opfer"), 0o644)
	os.Symlink(draussen, filepath.Join(dir, v.Name+".json"))
	if _, err := lauf(dir, v.Name, v.Chiffrat); err == nil {
		t.Fatal("einem Verweis gefolgt")
	}
	if roh, _ := os.ReadFile(draussen); string(roh) != "opfer" {
		t.Fatal("Ziel des Verweises beschrieben")
	}
	leer(t, dir, "verweis")
	// Das Verzeichnis selbst ist ein Verweis.
	echt := t.TempDir()
	verweis := filepath.Join(t.TempDir(), "verz")
	os.Symlink(echt, verweis)
	if _, err := lauf(verweis, v.Name, v.Chiffrat); err == nil {
		t.Fatal("in ein verlinktes Verzeichnis abgelegt")
	}
	leer(t, echt)
	if _, err := lauf(filepath.Join(echt, "fehlt"), v.Name, v.Chiffrat); err == nil {
		t.Fatal("ohne Verzeichnis abgelegt")
	}
	// Der Name wird Teil des Pfads: nur 64 Hex-Zeichen.
	for _, name := range []string{"../" + v.Name[3:], v.Name[:63] + "/", "", strings.ToUpper(v.Name[:1]) + "x" + v.Name[2:]} {
		dir = t.TempDir()
		if _, err := lauf(dir, name, v.Chiffrat); err == nil {
			t.Fatalf("Name %q angenommen", name)
		}
		leer(t, dir)
	}
	// Unter dem Namen der Zwischendatei liegt ein Verweis (vom Knotenbenutzer
	// untergeschoben): O_EXCL legt nicht an, dem Verweis wird nicht gefolgt.
	dir = t.TempDir()
	opfer := filepath.Join(t.TempDir(), "opfer")
	os.WriteFile(opfer, []byte("opfer"), 0o644)
	os.Symlink(opfer, filepath.Join(dir, ".neu-fest"))
	alt := quantenSZufallsName
	quantenSZufallsName = func() (string, error) { return ".neu-fest", nil }
	_, err := lauf(dir, v.Name, v.Chiffrat)
	quantenSZufallsName = alt
	if err == nil {
		t.Fatal("ueber einen Verweis als Zwischendatei abgelegt")
	}
	if roh, _ := os.ReadFile(opfer); string(roh) != "opfer" {
		t.Fatal("Ziel des Verweises beschrieben")
	}
	if _, err := os.Lstat(filepath.Join(dir, v.Name+".json")); err == nil {
		t.Fatal("trotzdem unter dem Namen abgelegt")
	}
	// Scheitert das Oeffnen, wird nichts angelegt.
	dir = t.TempDir()
	if _, err := lauf(dir, v.Name, "04"+v.Chiffrat[2:len(v.Chiffrat)-2]+"00"); err == nil {
		t.Fatal("veraendertes Chiffrat abgelegt")
	}
	leer(t, dir)
}

func TestQuantenSMessungOeffnenMissbrauch(t *testing.T) {
	v := qsvLesen(t)
	key, _ := crypto.HexToECDSA(v.Schluessel)
	pub := ecies.ImportECDSAPublic(&key.PublicKey)
	huelle := func(dom, an, messung string) string {
		return `{"dom":"` + dom + `","an":"` + an + `","messung":` + messung + `}`
	}
	gut := `{"u":"01101001","t":"11000101","y":"10011100","x":"01011101","wo":"w","job":"j"}`
	if _, err := quantenSMessungOeffnen(key, qsvVerschluesseln(t, pub, huelle(quantenSMessungDom, v.An, gut)), 8, v.Name); err != nil {
		t.Fatalf("Gutfall: %v", err)
	}
	anders, _ := crypto.HexToECDSA(strings.Repeat("72", 32))
	fremd := ecies.ImportECDSAPublic(&anders.PublicKey)
	kippen := func(i int) string {
		roh, _ := hex.DecodeString(v.Chiffrat)
		if i < 0 {
			i += len(roh)
		}
		roh[i] ^= 1
		return hex.EncodeToString(roh)
	}
	faelle := []struct {
		was     string
		chiffre string
		n       int
		name    string
	}{
		{"fuer einen anderen Schluessel", qsvVerschluesseln(t, fremd, huelle(quantenSMessungDom, quantenSAdresse(anders), gut)), 8, v.Name},
		{"Punkt veraendert", kippen(10), 8, v.Name},
		{"IV veraendert", kippen(66), 8, v.Name},
		{"Chiffrat veraendert", kippen(90), 8, v.Name},
		{"Tag veraendert", kippen(-1), 8, v.Name},
		{"abgeschnitten", v.Chiffrat[:len(v.Chiffrat)-2], 8, v.Name},
		{"kein Hex", "zz" + v.Chiffrat[2:], 8, v.Name},
		{"zu gross", strings.Repeat("00", quantenSVersiegeltMax/2+1), 8, v.Name},
		{"fremde Domaene", qsvVerschluesseln(t, pub, huelle("aequitas-qs-messung-v0", v.An, gut)), 8, v.Name},
		{"anderer Empfaenger", qsvVerschluesseln(t, pub, huelle(quantenSMessungDom, quantenSAdresse(anders), gut)), 8, v.Name},
		{"Feld zu viel", qsvVerschluesseln(t, pub, huelle(quantenSMessungDom, v.An, strings.Replace(gut, `"job":"j"`, `"job":"j","z":"1"`, 1))), 8, v.Name},
		{"job fehlt", qsvVerschluesseln(t, pub, huelle(quantenSMessungDom, v.An, strings.Replace(gut, `,"job":"j"`, ``, 1))), 8, v.Name},
		{"wo fehlt", qsvVerschluesseln(t, pub, huelle(quantenSMessungDom, v.An, strings.Replace(gut, `,"wo":"w"`, ``, 1))), 8, v.Name},
		{"n zu klein", qsvVerschluesseln(t, pub, huelle(quantenSMessungDom, v.An, `{"u":"0","t":"1","y":"1","x":"0","wo":"w","job":"j"}`)), 1, "6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b"},
		{"Huelle mit Feld zu viel", qsvVerschluesseln(t, pub, strings.TrimSuffix(huelle(quantenSMessungDom, v.An, gut), "}")+`,"z":1}`), 8, v.Name},
		{"Huelle ohne Empfaenger", qsvVerschluesseln(t, pub, `{"dom":"`+quantenSMessungDom+`","messung":`+gut+`}`), 8, v.Name},
		{"zwei Werte", qsvVerschluesseln(t, pub, huelle(quantenSMessungDom, v.An, gut)+" {}"), 8, v.Name},
		{"keine Bits", qsvVerschluesseln(t, pub, huelle(quantenSMessungDom, v.An, strings.Replace(gut, `"x":"01011101"`, `"x":"0101110a"`, 1))), 8, v.Name},
		{"andere Laenge", qsvVerschluesseln(t, pub, huelle(quantenSMessungDom, v.An, gut)), 9, v.Name},
		{"andere Basen", qsvVerschluesseln(t, pub, huelle(quantenSMessungDom, v.An, strings.Replace(gut, `"y":"10011100"`, `"y":"10011101"`, 1))), 8, v.Name},
		{"Name kein Hash", v.Chiffrat, 8, "abc"},
		{"wo zu lang", qsvVerschluesseln(t, pub, huelle(quantenSMessungDom, v.An, strings.Replace(gut, `"wo":"w"`, `"wo":"`+strings.Repeat("w", 513)+`"`, 1))), 8, v.Name},
		{"job zu lang", qsvVerschluesseln(t, pub, huelle(quantenSMessungDom, v.An, strings.Replace(gut, `"job":"j"`, `"job":"`+strings.Repeat("j", 513)+`"`, 1))), 8, v.Name},
	}
	for _, f := range faelle {
		if m, err := quantenSMessungOeffnen(key, f.chiffre, f.n, f.name); err == nil {
			t.Errorf("%s: angenommen (%+v)", f.was, m)
		}
	}
	if _, err := quantenSMessungOeffnen(nil, v.Chiffrat, 8, v.Name); err == nil {
		t.Error("ohne Schluessel angenommen")
	}
	// Ueber den Befehl: ein unlesbarer Schluessel taucht nicht im Fehler auf.
	var aus bytes.Buffer
	dir := t.TempDir()
	if err := QuantenSMessungOeffnen("geheim-kaputt", strings.NewReader(v.Chiffrat), 8, v.Name, dir, &aus); err == nil || strings.Contains(err.Error(), "geheim") {
		t.Fatalf("Schluesselfehler: %v", err)
	}
	// Die Grenze greift vor dem Entschluesseln (eigene Meldung), auch wenn der
	// Text danach ein gueltiges Chiffrat waere.
	if err := QuantenSMessungOeffnen(v.Schluessel, strings.NewReader(v.Chiffrat+strings.Repeat(" ", quantenSVersiegeltMax)), 8, v.Name, dir, &aus); err == nil || !strings.Contains(err.Error(), "zu gross") {
		t.Fatalf("zu grosse Eingabe: %v", err)
	}
	if _, err := quantenSMessungOeffnen(key, strings.Repeat("00", quantenSVersiegeltMax/2+1), 8, v.Name); err == nil || !strings.Contains(err.Error(), "zu gross") {
		t.Fatalf("zu grosses Chiffrat: %v", err)
	}
	if aus.Len() != 0 {
		t.Fatal("im Fehlerfall etwas ausgegeben")
	}
	if e, _ := os.ReadDir(dir); len(e) != 0 {
		t.Fatalf("im Fehlerfall etwas abgelegt: %v", e)
	}
}
