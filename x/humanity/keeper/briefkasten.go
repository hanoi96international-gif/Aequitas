package keeper

// BRIEFKASTEN (Quantengeld Weg B, Stufe B3; forschung/quantengeld/DEZENTRAL.md 18)
//
// Praeparatorinnen auf dem Handy sind nicht immer erreichbar. Sie tauschen
// Pakete -- verschluesselte Basen, Anteile, Oeffnungen -- ueber einen
// Briefkasten beim Knoten: ablegen, abholen, quittieren. Der Knoten sieht nur
// Chiffrat; er prueft, WER ablegt und WER abholt, und begrenzt alles.
//
//   - POST /api/briefkasten/ablegen {von, an, inhalt (hex), zeit, sig}: nur ein
//     registrierter Mensch (is_human) legt ab, unterschrieben mit seiner
//     Wallet (EIP-191 ueber Absender, Empfaenger, SHA-256 des Inhalts, Zeit).
//     Menschen sind nicht beliebig vermehrbar -- das ist der Schutz gegen
//     Fluten. Dieselbe Ablage zweimal: abgewiesen.
//   - POST /api/briefkasten/abholen {an, nach, zeit, sig}: nur der Empfaenger
//     (unterschrieben), hoechstens briefAbholenHoechstens Briefe bzw.
//     briefAbholenBytes je Aufruf, der Reihe nach ab nach.
//   - POST /api/briefkasten/quittieren {an, bis, zeit, sig}: der Empfaenger
//     loescht seine Briefe bis einschliesslich bis.
//
// GRENZEN: Inhalt hoechstens briefInhaltMax; je Absender hoechstens
// briefJeAbsender Briefe und briefBytesJeAbsender im Kasten; je Empfaenger
// briefJeEmpfaenger, davon je Absender briefJePaar; im ganzen Kasten
// briefBytesGesamt. Nach briefBehalten
// verfaellt ein Brief. Je Absender-IP briefJeIP Anfragen im Fenster.
// Unterschriebene Zeiten hoechstens briefZeitSpiel von der Uhr des Knotens.
//
// FAIL-CLOSED: Ohne Datenbank oder bei einem Lesefehler nimmt der Kasten
// nichts an (503) -- nie "ablegen ohne Pruefung".
//
// KEINE KONSENSREGEL: der Kasten ist ein Dienst des einzelnen Knotens, nicht
// Teil der Kette (keine StateRoot, kein Snapshot). Wer sicher gehen will,
// legt bei mehreren Knoten ab; der Empfaenger holt bei mehreren ab.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	briefDomAblegen    = "aequitas-briefkasten-ablegen-v1"
	briefDomAbholen    = "aequitas-briefkasten-abholen-v1"
	briefDomQuittieren = "aequitas-briefkasten-quittieren-v1"

	briefInhaltMax             = 64 << 10
	briefKoerperMax            = 2*briefInhaltMax + 4096
	briefZeitSpiel       int64 = 300
	briefBehalten        int64 = 8 * 86400
	briefJeAbsender            = 512
	briefBytesJeAbsender       = 32 << 20
	briefJeEmpfaenger          = 1024
	// je Absender und Empfaenger: ein Postfach (briefJeEmpfaenger) zu fuellen
	// braucht so mindestens 16 verschiedene Menschen. Das Protokoll braucht je
	// Token nur wenige Pakete zwischen zwei Praeparatorinnen.
	briefJePaar            = 64
	briefBytesGesamt       = 1 << 30
	briefAbholenHoechstens = 32
	briefAbholenBytes      = 2 << 20
	briefJeIP              = 60
)

// briefMu: Kontingente pruefen und ablegen in einem Schritt (ein Knoten).
var briefMu sync.Mutex

func briefAblegenNachricht(von, an, inhaltHash string, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%s|%d", briefDomAblegen, eingefroreneChainID, von, an, inhaltHash, zeit)
}

func briefAbholenNachricht(an string, nach, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%d|%d", briefDomAbholen, eingefroreneChainID, an, nach, zeit)
}

func briefQuittierenNachricht(an string, bis, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%d|%d", briefDomQuittieren, eingefroreneChainID, an, bis, zeit)
}

// briefAbweisung: Anfrage falsch (4xx); jeder andere Fehler ist 503.
type briefAbweisung struct {
	code int
	text string
}

func (e *briefAbweisung) Error() string { return e.text }

func briefFalsch(code int, format string, args ...interface{}) error {
	return &briefAbweisung{code: code, text: fmt.Sprintf(format, args...)}
}

// briefUnterschrift: kanonische EIP-191-Unterschrift von wer, Zeit im Spiel.
func briefUnterschrift(nachricht, sig, wer string, zeit, jetzt int64) error {
	if zeit <= 0 || zeit > jetzt+briefZeitSpiel || zeit < jetzt-briefZeitSpiel {
		return briefFalsch(http.StatusBadRequest, "zeit ausserhalb von %d s um die Uhr des Knotens", briefZeitSpiel)
	}
	if !kanonischeSignatur(sig) || verifyPersonalSign(nachricht, sig, wer) != nil {
		return briefFalsch(http.StatusForbidden, "Unterschrift passt nicht zu %s", kurzAdresse(wer))
	}
	return nil
}

// briefMensch: ist wer JETZT ein registrierter Mensch? Fehler = nicht lesbar.
func (cs *ChainState) briefMensch(wer string) (bool, error) {
	bekannt, mensch := func() (bool, bool) {
		cs.mu.RLock()
		defer cs.mu.RUnlock()
		acc, ok := cs.accounts.Get(wer)
		return ok, ok && acc.IsHuman
	}()
	if bekannt {
		return mensch, nil
	}
	var h bool
	switch err := cs.db.QueryRow(`SELECT is_human FROM chain_accounts WHERE lower(address) = $1`, wer).Scan(&h); {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, err
	}
	return h, nil
}

// briefAblegen: alle Pruefungen und das Ablegen. jetzt = Uhr des Knotens.
func (cs *ChainState) briefAblegen(von, an, inhaltHex string, zeit int64, sig string, jetzt int64) (int64, error) {
	if !kanonischeAdresse(von) || !kanonischeAdresse(an) || istSystemAdresse(an) {
		return 0, briefFalsch(http.StatusBadRequest, "von und an: kanonische Adressen")
	}
	inhalt, err := hex.DecodeString(inhaltHex)
	if err != nil || len(inhalt) == 0 || len(inhalt) > briefInhaltMax || hex.EncodeToString(inhalt) != inhaltHex {
		return 0, briefFalsch(http.StatusBadRequest, "inhalt: 1 bis %d Byte, hex klein", briefInhaltMax)
	}
	h := sha256.Sum256(inhalt)
	hHex := hex.EncodeToString(h[:])
	if err := briefUnterschrift(briefAblegenNachricht(von, an, hHex, zeit), sig, von, zeit, jetzt); err != nil {
		return 0, err
	}
	if cs.db == nil {
		return 0, errors.New("briefkasten: keine Datenbank")
	}
	mensch, err := cs.briefMensch(von)
	if err != nil {
		return 0, err
	}
	if !mensch {
		return 0, briefFalsch(http.StatusForbidden, "%s ist kein registrierter Mensch", kurzAdresse(von))
	}
	// Eine Ablage ist durch Absender, Empfaenger, Inhalt und Zeit bestimmt:
	// dieselbe Unterschrift legt nur einmal ab.
	schluessel := sha256.Sum256([]byte(von + "|" + an + "|" + hHex + "|" + fmt.Sprint(zeit)))

	briefMu.Lock()
	defer briefMu.Unlock()
	if _, err := cs.db.Exec(`DELETE FROM briefkasten WHERE abgelegt < $1`, jetzt-briefBehalten); err != nil {
		return 0, err
	}
	var nVon, nAn, nPaar int
	var bytesVon, bytesGesamt int64
	if err := cs.db.QueryRow(`SELECT
		(SELECT count(*) FROM briefkasten WHERE von = $1),
		(SELECT COALESCE(sum(groesse), 0) FROM briefkasten WHERE von = $1),
		(SELECT count(*) FROM briefkasten WHERE an = $2),
		(SELECT count(*) FROM briefkasten WHERE an = $2 AND von = $1),
		(SELECT COALESCE(sum(groesse), 0) FROM briefkasten)`, von, an).Scan(&nVon, &bytesVon, &nAn, &nPaar, &bytesGesamt); err != nil {
		return 0, err
	}
	switch {
	case nPaar >= briefJePaar:
		return 0, briefFalsch(http.StatusTooManyRequests, "zu viele Briefe dieses Absenders an diesen Empfaenger")
	case nVon >= briefJeAbsender || bytesVon+int64(len(inhalt)) > briefBytesJeAbsender:
		return 0, briefFalsch(http.StatusTooManyRequests, "Kontingent des Absenders voll")
	case nAn >= briefJeEmpfaenger:
		return 0, briefFalsch(http.StatusTooManyRequests, "Briefkasten des Empfaengers voll")
	case bytesGesamt+int64(len(inhalt)) > briefBytesGesamt:
		return 0, briefFalsch(http.StatusServiceUnavailable, "Briefkasten des Knotens voll")
	}
	var id int64
	err = cs.db.QueryRow(`INSERT INTO briefkasten (schluessel, von, an, inhalt, groesse, zeit, abgelegt)
		VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (schluessel) DO NOTHING RETURNING id`,
		hex.EncodeToString(schluessel[:]), von, an, inhalt, len(inhalt), zeit, jetzt).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, briefFalsch(http.StatusConflict, "schon abgelegt")
	}
	return id, err
}

type briefAusgabe struct {
	ID     int64  `json:"id"`
	Von    string `json:"von"`
	Inhalt string `json:"inhalt"`
	Zeit   int64  `json:"zeit"`
}

// briefAbholen: die Briefe an an nach id nach, begrenzt; mehr = es gibt weitere.
func (cs *ChainState) briefAbholen(an string, nach, zeit int64, sig string, jetzt int64) ([]briefAusgabe, bool, error) {
	if !kanonischeAdresse(an) || nach < 0 {
		return nil, false, briefFalsch(http.StatusBadRequest, "an kanonisch, nach >= 0")
	}
	if err := briefUnterschrift(briefAbholenNachricht(an, nach, zeit), sig, an, zeit, jetzt); err != nil {
		return nil, false, err
	}
	if cs.db == nil {
		return nil, false, errors.New("briefkasten: keine Datenbank")
	}
	rows, err := cs.db.Query(`SELECT id, von, inhalt, zeit FROM briefkasten
		WHERE an = $1 AND id > $2 AND abgelegt >= $3 ORDER BY id LIMIT $4`, an, nach, jetzt-briefBehalten, briefAbholenHoechstens+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []briefAusgabe
	bytes, mehr := 0, false
	for rows.Next() {
		var b briefAusgabe
		var inhalt []byte
		if err := rows.Scan(&b.ID, &b.Von, &inhalt, &b.Zeit); err != nil {
			return nil, false, err
		}
		if len(out) == briefAbholenHoechstens || bytes+len(inhalt) > briefAbholenBytes {
			mehr = true
			break
		}
		bytes += len(inhalt)
		b.Inhalt = hex.EncodeToString(inhalt)
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return out, mehr, nil
}

// briefQuittieren: die eigenen Briefe bis einschliesslich bis loeschen.
func (cs *ChainState) briefQuittieren(an string, bis, zeit int64, sig string, jetzt int64) (int64, error) {
	if !kanonischeAdresse(an) || bis < 0 {
		return 0, briefFalsch(http.StatusBadRequest, "an kanonisch, bis >= 0")
	}
	if err := briefUnterschrift(briefQuittierenNachricht(an, bis, zeit), sig, an, zeit, jetzt); err != nil {
		return 0, err
	}
	if cs.db == nil {
		return 0, errors.New("briefkasten: keine Datenbank")
	}
	res, err := cs.db.Exec(`DELETE FROM briefkasten WHERE an = $1 AND id <= $2`, an, bis)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// handleBriefkasten: POST /api/briefkasten/{ablegen,abholen,quittieren}.
func (a *APIServer) handleBriefkasten(art string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSONCORS(w)
		if r.Method != http.MethodPost {
			jsonError(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if !burstErlaubt("briefkasten:"+erneuerungsAbsender(r), briefJeIP, burstFenster) {
			jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
			return
		}
		var req struct {
			Von    string `json:"von"`
			An     string `json:"an"`
			Inhalt string `json:"inhalt"`
			Nach   int64  `json:"nach"`
			Bis    int64  `json:"bis"`
			Zeit   int64  `json:"zeit"`
			Sig    string `json:"sig"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, briefKoerperMax))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			jsonError(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		an := strings.ToLower(strings.TrimSpace(req.An))
		jetzt := time.Now().Unix()
		var antwort map[string]interface{}
		var err error
		switch art {
		case "ablegen":
			var id int64
			id, err = a.state.briefAblegen(strings.ToLower(strings.TrimSpace(req.Von)), an, req.Inhalt, req.Zeit, req.Sig, jetzt)
			antwort = map[string]interface{}{"ok": true, "id": id}
		case "abholen":
			var briefe []briefAusgabe
			var mehr bool
			briefe, mehr, err = a.state.briefAbholen(an, req.Nach, req.Zeit, req.Sig, jetzt)
			if briefe == nil {
				briefe = []briefAusgabe{}
			}
			antwort = map[string]interface{}{"briefe": briefe, "mehr": mehr}
		case "quittieren":
			var n int64
			n, err = a.state.briefQuittieren(an, req.Bis, req.Zeit, req.Sig, jetzt)
			antwort = map[string]interface{}{"ok": true, "geloescht": n}
		default:
			jsonError(w, "unknown", http.StatusNotFound)
			return
		}
		if err != nil {
			var ab *briefAbweisung
			if errors.As(err, &ab) {
				jsonError(w, ab.text, ab.code)
				return
			}
			fmt.Printf("[BRIEFKASTEN] %s gescheitert: %v\n", art, err)
			jsonError(w, "mailbox not available, please retry shortly", http.StatusServiceUnavailable)
			return
		}
		json.NewEncoder(w).Encode(antwort)
	}
}
