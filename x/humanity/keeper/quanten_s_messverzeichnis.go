package keeper

// QUANTENGELD VARIANTE S -- MESSQUELLE ALS VERZEICHNIS (Betrieb, A5)
//
// Im Pilot liegen alle Messungen eines Validators in EINER Datei
// (quantenSMessDatei, von hardware_ibm.py --s-abschnitte), die vor dem Start
// geschrieben wird. Im Betrieb misst der Validator, waehrend sein Dienst
// laeuft: Die Inhaberin schickt ihm ihre Basen y fuer seinen Abschnitt, er
// misst (hardware_ibm.py --s-abschnitt), dann kommt der Auftrag des
// Ausgebers. Wuerden Messen und Dienst dieselbe Datei lesen und neu schreiben,
// koennte ein schon verbrauchter Abschnitt zurueckkommen -- dieselbe Messung
// zweimal.
//
// Darum hier je gemessenem Abschnitt eine eigene Datei, benannt nach den
// Basen der Inhaberin: <sha256(y als Text aus 0 und 1)>.json mit genau einem
// Datensatz (quantenSMessDatensatz). Der Messende legt nur NEUE Dateien an
// (os.link, nie ueberschreiben). Der Dienst liest genau die eine Datei zu den
// verlangten Basen, prueft sie streng, ENTFERNT sie und verwendet sie erst
// danach. Die Einmaligkeit haengt am Dateisystem (nur ein Entfernen gelingt),
// nicht an einer Absprache zwischen zwei Programmen.
//
// Fail-closed: keine Datei, keine gewoehnliche Datei (z. B. ein Verweis), zu
// gross, unbekannte Felder, Text nach dem Datensatz, andere Basen als der Name
// sagt, kaputte Bits oder ein gescheitertes Entfernen -- dann keine Messung.
// Eine abgewiesene Datei bleibt liegen, damit der Betreiber sie ansehen kann.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// quantenSMessEintragMax: ein Datensatz hat vier Bitketten von hoechstens
// quantenWNMax Zeichen; 1 MiB laesst reichlich Platz und begrenzt das Lesen.
const quantenSMessEintragMax = 1 << 20

type quantenSMessVerzeichnis struct {
	mu  sync.Mutex
	dir string
	// Vormerkungen je Tag (quanten_s_vormerken.go), unter mu.
	vormerkTag    int64
	vormerkGesamt int
	vormerkJe     map[string]int
}

// quantenSMessName: Dateiname des Abschnitts zu den Basen yText (gleich in
// hardware_ibm.py s_messquelle_name).
func quantenSMessName(yText string) string {
	h := sha256.Sum256([]byte(yText))
	return hex.EncodeToString(h[:]) + ".json"
}

func (m *quantenSMessVerzeichnis) abschnitt(y []int) ([]int, []int, []int, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(y) == 0 {
		return nil, nil, nil, "", quantenWFehler("keine Basen")
	}
	soll := qsBitsText(y)
	pfad := filepath.Join(m.dir, quantenSMessName(soll))
	info, err := os.Lstat(pfad)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil, "", quantenWFehler("keine Messung zu diesen Basen")
	}
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, nil, "", quantenWFehler("Messquelle: Eintrag ist keine gewoehnliche Datei")
	}
	if info.Size() > quantenSMessEintragMax {
		return nil, nil, nil, "", quantenWFehler("Messquelle: Eintrag zu gross")
	}
	roh, err := quantenSMessEintragLesen(pfad, info)
	if err != nil {
		return nil, nil, nil, "", err
	}
	var d quantenSMessDatensatz
	dec := json.NewDecoder(bytes.NewReader(roh))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil || dec.More() {
		return nil, nil, nil, "", quantenWFehler("Messquelle: Eintrag unlesbar")
	}
	if d.Y != soll {
		return nil, nil, nil, "", quantenWFehler("Messquelle: Eintrag passt nicht zu seinem Namen")
	}
	u, ok1 := quantenSBitsLesen(d.U, len(y))
	t, ok2 := quantenSBitsLesen(d.T, len(y))
	x, ok3 := quantenSBitsLesen(d.X, len(y))
	if !ok1 || !ok2 || !ok3 {
		return nil, nil, nil, "", quantenWFehler("Messquelle: Eintrag kaputt")
	}
	// Erst entfernen, dann verwenden: gelingt das Entfernen nicht (oder war
	// ein anderer schneller), wird nichts verwendet.
	if err := os.Remove(pfad); err != nil {
		return nil, nil, nil, "", quantenWFehler("Messquelle: Eintrag nicht entfernbar")
	}
	return u, t, x, d.Wo + " " + d.Job, nil
}

// quantenSMessEintragLesen: liest die Datei, die Lstat gesehen hat -- wurde
// sie dazwischen ausgetauscht, wird nichts gelesen.
func quantenSMessEintragLesen(pfad string, info fs.FileInfo) ([]byte, error) {
	f, err := os.Open(pfad)
	if err != nil {
		return nil, quantenWFehler("Messquelle nicht lesbar")
	}
	defer f.Close()
	offen, err := f.Stat()
	if err != nil || !os.SameFile(info, offen) {
		return nil, quantenWFehler("Messquelle: Eintrag ausgetauscht")
	}
	roh, err := io.ReadAll(io.LimitReader(f, quantenSMessEintragMax+1))
	if err != nil || len(roh) > quantenSMessEintragMax {
		return nil, quantenWFehler("Messquelle: Eintrag zu gross")
	}
	return roh, nil
}
