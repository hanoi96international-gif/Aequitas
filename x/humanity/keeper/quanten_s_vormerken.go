package keeper

// QUANTENGELD VARIANTE S -- BASEN BEIM VALIDATOR VORMERKEN (A5, Betrieb)
//
// Ein Validator praepariert seinen Abschnitt in den Messbasen y der
// Inhaberin. Die Messung auf dem Quantenrechner dauert Minuten (Warteschlange
// beim Anbieter) und laeuft auf seinem Server, nicht im Knoten
// (deploy/validator/quanten-messen.sh). Der Abschnitts-Auftrag
// (quanten_s_abschnitt_dienst.go) findet die Messung darum schon fertig im
// Messverzeichnis vor -- oder weist ab ("keine Messung zu diesen Basen").
//
// Bisher musste die Inhaberin dem Betreiber y von Hand schicken. Jetzt:
//
//  1. VORMERKEN (POST /api/quanten-w/validator/vormerken, vom Ausgeber
//     unterschrieben, nur ein Mensch mit Deckung): y fuer GENAU den Abschnitt
//     dieses Validators. Der Knoten legt eine neue Datei
//     <Messverzeichnis>/vorgemerkt/<sha256(y)>.basen an (nie ueberschrieben).
//     Antwort: "vorgemerkt", "wartet" (liegt schon vor) oder "gemessen"
//     (die Messung ist da). Wiederholen ist sicher und zaehlt nicht.
//  2. MESSEN auf dem Server (quanten-messen.sh --warteschlange, als
//     Knotenbenutzer, mit Tagesgrenze): liest die Datei streng, misst, legt
//     die Messung ab, entfernt die Vormerkung.
//  3. Das Werkzeug (aequitasd quanten-s-ausgabe --messen-warten) fragt
//     begrenzt nach, bis jeder Validator "gemessen" meldet, und schickt erst
//     dann die Abschnitts-Auftraege.
//
// Jede Messung kostet Rechenzeit auf dem Konto des Validators. Darum begrenzt:
// hoechstens quantenSVormerkOffenHoechstens wartende Vormerkungen,
// quantenSVormerkJePraegerTag je Ausgeber und quantenSVormerkJeTag insgesamt
// je Tag (UTC), Unterschrift hoechstens quantenSVormerkAlt alt. Die Zaehler
// leben im Speicher (ein Neustart setzt sie zurueck); unabhaengig davon misst
// der Server hoechstens QS_JE_TAG Abschnitte am Tag (Zaehler ausserhalb des
// Verzeichnisses, das der Knoten beschreiben kann).
//
// Die Kette aendert sich nicht; die Vormerkung ist keine Zusage und
// verbraucht nichts beim Validator ausser der Messung.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	quantenSDomVormerkung = "aequitas-quanten-s-vormerkung-v1"
	// Unterverzeichnis des Messverzeichnisses und Endung der Vormerkungen
	// (gleich in hardware_ibm.py).
	quantenSVormerkVerzeichnis = "vorgemerkt"
	quantenSVormerkEndung      = ".basen"
	// Grenzen (siehe Kopf).
	quantenSVormerkOffenHoechstens       = 8
	quantenSVormerkJePraegerTag          = 8
	quantenSVormerkJeTag                 = 32
	quantenSVormerkAlt             int64 = 600
	// Beim Zaehlen der wartenden Vormerkungen werden hoechstens so viele
	// Eintraege gelesen; sind es mehr, gilt das Verzeichnis als voll.
	quantenSVormerkLesenHoechstens = 4 * quantenSVormerkOffenHoechstens
)

// QuantenSVormerkung: y der Inhaberin fuer Abschnitt X, vom Ausgeber unterschrieben.
type QuantenSVormerkung struct {
	N           int      `json:"n"`
	X           int      `json:"x"`
	Validatoren []string `json:"validatoren"`
	Basen       string   `json:"basen"`
	Zeit        int64    `json:"zeit"`
	Praeger     string   `json:"praeger"`
	PraegerSig  string   `json:"praeger_sig"`
}

// QuantenSVormerkAntwort: Stand beim Validator.
type QuantenSVormerkAntwort struct {
	Validator string `json:"validator"`
	X         int    `json:"x"`
	Status    string `json:"status"` // vorgemerkt | wartet | gemessen
}

// quantenSVormerkDatei: Inhalt einer Vormerkung (hardware_ibm.py liest genau
// diese Felder).
type quantenSVormerkDatei struct {
	Basen   string `json:"basen"`
	Praeger string `json:"praeger"`
	Zeit    int64  `json:"zeit"`
}

const (
	quantenSVorgemerkt = "vorgemerkt"
	quantenSWartet     = "wartet"
	quantenSGemessen   = "gemessen"
)

// quantenSVormerkNachricht: was der Ausgeber unterschreibt.
func quantenSVormerkNachricht(q QuantenSVormerkung) string {
	hb := sha256.Sum256([]byte(q.Basen))
	return fmt.Sprintf("%s|chain:%d|%s|%d|%d|%s|%x|%d", quantenSDomVormerkung, eingefroreneChainID,
		strings.ToLower(q.Praeger), q.N, q.X, strings.Join(q.Validatoren, ","), hb, q.Zeit)
}

// vormerkungPruefen: alles, was ohne Verzeichnis geprueft werden kann.
func (d *quantenWDienst) vormerkungPruefen(q *QuantenSVormerkung) error {
	v := len(q.Validatoren)
	if v < 1 || v > quantenWVMax {
		return quantenWFehler("Zahl der Validatoren")
	}
	gesehen := map[string]bool{}
	for _, a := range q.Validatoren {
		if !kanonischeAdresse(a) || gesehen[a] {
			return quantenWFehler("Validator nicht kanonisch oder doppelt")
		}
		gesehen[a] = true
	}
	if q.X < 1 || q.X > v || q.Validatoren[q.X-1] != d.adresse {
		return quantenWFehler("Nummer %d gehoert nicht zu diesem Validator", q.X)
	}
	if q.N < 2 || q.N > quantenWNMax {
		return quantenWFehler("n")
	}
	von, bis, ok := quantenSAbschnitt(q.N, v, q.X)
	if !ok {
		return quantenWFehler("Abschnitte")
	}
	if len(q.Basen) != bis-von {
		return quantenWFehler("Basen: %d statt %d", len(q.Basen), bis-von)
	}
	if _, ok := quantenSBitsLesen(q.Basen, bis-von); !ok {
		return quantenWFehler("Basen: nur 0 und 1")
	}
	q.Praeger = strings.ToLower(strings.TrimSpace(q.Praeger))
	if !kanonischeAdresse(q.Praeger) || !kanonischeSignatur(q.PraegerSig) ||
		verifyPersonalSign(quantenSVormerkNachricht(*q), q.PraegerSig, q.Praeger) != nil {
		return quantenWFehler("Vormerkung nicht vom Ausgeber unterschrieben")
	}
	if !d.praegerTauglich(q.Praeger) {
		return quantenWFehler("Ausgeber ist kein Mensch mit Deckung")
	}
	jetzt := d.jetzt()
	if q.Zeit < jetzt-quantenSVormerkAlt || q.Zeit > jetzt+quantenWVoraus {
		return quantenWFehler("Zeit ausserhalb des Fensters")
	}
	return nil
}

// vormerken: pruefen, dann im eigenen Messverzeichnis vormerken.
func (d *quantenWDienst) vormerken(q QuantenSVormerkung) (QuantenSVormerkAntwort, error) {
	if err := d.vormerkungPruefen(&q); err != nil {
		return QuantenSVormerkAntwort{}, err
	}
	mv, ok := d.quelle().(*quantenSMessVerzeichnis)
	if !ok || mv == nil {
		return QuantenSVormerkAntwort{}, quantenWFehler("dieser Validator nimmt keine Vormerkungen an (Messquelle ist kein Verzeichnis)")
	}
	st, err := mv.vormerken(q.Basen, q.Praeger, q.Zeit, d.jetzt())
	if err != nil {
		return QuantenSVormerkAntwort{}, err
	}
	return QuantenSVormerkAntwort{Validator: d.adresse, X: q.X, Status: st}, nil
}

// vormerken: legt <dir>/vorgemerkt/<sha256(y)>.basen an -- oder meldet, dass
// die Vormerkung schon liegt oder die Messung schon da ist. Grenzen unter
// derselben Sperre wie das Anlegen; nur ein neues Anlegen zaehlt.
func (m *quantenSMessVerzeichnis) vormerken(yText, praeger string, zeit, jetzt int64) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := quantenSMessName(yText)
	switch info, err := os.Lstat(filepath.Join(m.dir, name)); {
	case err == nil && info.Mode().IsRegular():
		return quantenSGemessen, nil
	case err == nil:
		return "", quantenWFehler("Messquelle: Eintrag ist keine gewoehnliche Datei")
	case !errors.Is(err, fs.ErrNotExist):
		return "", quantenWFehler("Messquelle nicht lesbar")
	}
	vdir := filepath.Join(m.dir, quantenSVormerkVerzeichnis)
	if err := os.Mkdir(vdir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", fmt.Errorf("Vormerkungen: Verzeichnis: %w", err)
	}
	if info, err := os.Lstat(vdir); err != nil || !info.IsDir() {
		return "", quantenWFehler("Vormerkungen: %s ist kein Verzeichnis", quantenSVormerkVerzeichnis)
	}
	ziel := filepath.Join(vdir, strings.TrimSuffix(name, ".json")+quantenSVormerkEndung)
	switch _, err := os.Lstat(ziel); {
	case err == nil:
		return quantenSWartet, nil
	case !errors.Is(err, fs.ErrNotExist):
		return "", quantenWFehler("Vormerkungen nicht lesbar")
	}
	offen, err := quantenSVormerkOffen(vdir)
	if err != nil {
		return "", err
	}
	if offen >= quantenSVormerkOffenHoechstens {
		return "", quantenWFehler("zu viele wartende Vormerkungen")
	}
	if tag := jetzt / 86400; tag != m.vormerkTag || m.vormerkJe == nil {
		m.vormerkTag, m.vormerkGesamt, m.vormerkJe = tag, 0, map[string]int{}
	}
	if m.vormerkGesamt >= quantenSVormerkJeTag || m.vormerkJe[praeger] >= quantenSVormerkJePraegerTag {
		return "", quantenWFehler("Tagesgrenze fuer Vormerkungen erreicht")
	}
	roh, err := json.Marshal(quantenSVormerkDatei{Basen: yText, Praeger: praeger, Zeit: zeit})
	if err != nil {
		return "", err
	}
	neu, err := quantenSVormerkAnlegen(vdir, ziel, roh)
	if err != nil {
		return "", err
	}
	if !neu {
		return quantenSWartet, nil
	}
	m.vormerkGesamt++
	m.vormerkJe[praeger]++
	return quantenSVorgemerkt, nil
}

// quantenSVormerkOffen: wartende Vormerkungen; mehr Eintraege als
// quantenSVormerkLesenHoechstens gelten als voll (fail-closed).
func quantenSVormerkOffen(vdir string) (int, error) {
	f, err := os.Open(vdir)
	if err != nil {
		return 0, quantenWFehler("Vormerkungen nicht lesbar")
	}
	defer f.Close()
	eintraege, err := f.ReadDir(quantenSVormerkLesenHoechstens)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, quantenWFehler("Vormerkungen nicht lesbar")
	}
	if len(eintraege) >= quantenSVormerkLesenHoechstens {
		return quantenSVormerkOffenHoechstens, nil
	}
	offen := 0
	for _, e := range eintraege {
		if strings.HasSuffix(e.Name(), quantenSVormerkEndung) {
			offen++
		}
	}
	return offen, nil
}

// quantenSVormerkAnlegen: erst unter einem eigenen Zufallsnamen (O_EXCL,
// 0600, auf die Platte), dann per Link auf den endgueltigen Namen -- nie
// ueberschrieben, nie halb sichtbar. false: der Name war schon vergeben.
func quantenSVormerkAnlegen(vdir, ziel string, roh []byte) (bool, error) {
	var z [8]byte
	if _, err := rand.Read(z[:]); err != nil {
		return false, err
	}
	tmp := filepath.Join(vdir, ".neu-"+hex.EncodeToString(z[:]))
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, fmt.Errorf("Vormerkung anlegen: %w", err)
	}
	defer os.Remove(tmp)
	if _, err := f.Write(roh); err != nil {
		f.Close()
		return false, fmt.Errorf("Vormerkung schreiben: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return false, fmt.Errorf("Vormerkung schreiben: %w", err)
	}
	if err := f.Close(); err != nil {
		return false, fmt.Errorf("Vormerkung schreiben: %w", err)
	}
	if err := os.Link(tmp, ziel); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return false, nil
		}
		return false, fmt.Errorf("Vormerkung anlegen: %w", err)
	}
	return true, nil
}

// ------------------------------------------------------------ Endpunkt

func (a *APIServer) handleQuantenSVormerken(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	d := a.state.quantenWDienstJetzt()
	if d == nil || !quantenWAktiv(time.Now().Unix()) {
		jsonError(w, "quantum money validator service not active", http.StatusConflict)
		return
	}
	if !burstErlaubt("quanten-w-dienst:"+erneuerungsAbsender(r), burstQuantenWDienstJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	var q QuantenSVormerkung
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, quantenSAbschnittKoerperMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&q); err != nil || dec.More() {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	out, err := d.vormerken(q)
	if err != nil {
		if errors.Is(err, errQuantenW) {
			jsonError(w, "rejected: "+err.Error(), http.StatusConflict)
			return
		}
		fmt.Printf("[quanten_s] Vormerken gescheitert: %v\n", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(out)
}

// ------------------------------------------------------------ Werkzeug

// quantenSVormerkNetz: der Weg zum Vormerken bei Validator x (1-basiert).
type quantenSVormerkNetz interface {
	vormerken(ctx context.Context, x int, q QuantenSVormerkung) (QuantenSVormerkAntwort, error)
}

const (
	// So oft fragt das Werkzeug nach, ob die Messungen da sind; Wiederholen
	// zaehlt beim Validator nicht gegen die Tagesgrenze.
	quantenSVormerkTakt = 20 * time.Second
	// Laenger wartet das Werkzeug nicht (Warteschlange beim Anbieter).
	quantenSMessenWartenMax = 4 * time.Hour
)

// quantenSBasenZiehen: zufaellige Messbasen der Inhaberin je Abschnitt.
func quantenSBasenZiehen(n, v int) ([]string, error) {
	if n < 2 || n > quantenWNMax || v < 1 || v > quantenWVMax {
		return nil, fmt.Errorf("Basen ziehen: n = %d, v = %d", n, v)
	}
	var out []string
	for x := 1; x <= v; x++ {
		von, bis, ok := quantenSAbschnitt(n, v, x)
		if !ok {
			return nil, fmt.Errorf("Abschnitte: n = %d, v = %d", n, v)
		}
		roh := make([]byte, bis-von)
		if _, err := rand.Read(roh); err != nil {
			return nil, err
		}
		b := make([]byte, len(roh))
		for i, r := range roh {
			b[i] = '0' + r&1
		}
		out = append(out, string(b))
	}
	return out, nil
}

// quantenSMessungenAbwarten: merkt bei jedem Validator y seines Abschnitts
// vor und fragt alle takt nach, bis alle "gemessen" melden -- hoechstens
// warten lang. Eine Ablehnung bricht sofort ab; dann ist nichts ausgegeben.
// Der Stand ist eine Auskunft, keine Zusage: luegt ein Validator "gemessen",
// weist er danach den Abschnitts-Auftrag ab.
func quantenSMessungenAbwarten(ctx context.Context, key *ecdsa.PrivateKey, basen QuantenSBasen, validatoren []string, netz quantenSVormerkNetz,
	warten, takt time.Duration, jetzt func() int64, log io.Writer) error {
	v := len(validatoren)
	if v < 1 || len(basen.Basen) != v {
		return fmt.Errorf("Vormerken: %d Abschnitte fuer %d Validatoren", len(basen.Basen), v)
	}
	if warten <= 0 || warten > quantenSMessenWartenMax || takt <= 0 {
		return fmt.Errorf("Vormerken: Wartezeit %s (hoechstens %s)", warten, quantenSMessenWartenMax)
	}
	ausgeber := quantenSAdresse(key)
	beginn := time.Now()
	ende := beginn.Add(warten)
	stand := make([]string, v)
	for {
		offen := 0
		for x := 1; x <= v; x++ {
			if stand[x-1] == quantenSGemessen {
				continue
			}
			q := QuantenSVormerkung{N: basen.N, X: x, Validatoren: validatoren, Basen: basen.Basen[x-1], Zeit: jetzt(), Praeger: ausgeber}
			sig, err := quantenSUnterschreiben(key, quantenSVormerkNachricht(q))
			if err != nil {
				return err
			}
			q.PraegerSig = sig
			ans, err := netz.vormerken(ctx, x, q)
			if err != nil {
				return fmt.Errorf("Validator %d: Vormerken: %w -- nichts ausgegeben", x, err)
			}
			if ans.X != x || !strings.EqualFold(ans.Validator, validatoren[x-1]) {
				return fmt.Errorf("Validator %d: Antwort auf das Vormerken passt nicht", x)
			}
			switch ans.Status {
			case quantenSGemessen:
			case quantenSVorgemerkt, quantenSWartet:
				offen++
			default:
				return fmt.Errorf("Validator %d: unbekannter Stand %q", x, ans.Status)
			}
			if ans.Status != stand[x-1] {
				fmt.Fprintf(log, "Validator %d: %s (nach %s)\n", x, ans.Status, time.Since(beginn).Round(time.Second))
				stand[x-1] = ans.Status
			}
		}
		if offen == 0 {
			return nil
		}
		if !time.Now().Add(takt).Before(ende) {
			return fmt.Errorf("nach %s haben %d von %d Validatoren noch keine Messung -- nichts ausgegeben", warten, offen, v)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(takt):
		}
	}
}

func (m *quantenSMehrNetz) vormerken(ctx context.Context, x int, q QuantenSVormerkung) (QuantenSVormerkAntwort, error) {
	h, err := m.validator(x)
	if err != nil {
		return QuantenSVormerkAntwort{}, err
	}
	roh, err := json.Marshal(q)
	if err != nil {
		return QuantenSVormerkAntwort{}, err
	}
	code, body, err := h.anfrage(ctx, http.MethodPost, "/api/quanten-w/validator/vormerken", roh)
	if err != nil {
		return QuantenSVormerkAntwort{}, err
	}
	if code != http.StatusOK {
		return QuantenSVormerkAntwort{}, fmt.Errorf("HTTP %d: %s", code, quantenSFehlertext(body))
	}
	var a QuantenSVormerkAntwort
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return QuantenSVormerkAntwort{}, errors.New("Antwort ist kein passendes JSON")
	}
	return a, nil
}
