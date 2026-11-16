package keeper

// QUANTENGELD VARIANTE W -- STUFE W2c: DER VALIDATOR-DIENST (schlafend)
//
// Die Regeln der Kette (quanten_w_kette.go) sagen, was eine gueltige
// Bestaetigung, ein gueltiger Zufall und eine gueltige Offenlegung ist. Sie
// sagen nicht, WER das einreicht. Bis hierhin gab es dafuer nur Testcode. Dies
// ist die Seite des Validators: er verwahrt seine Anteile, bestaetigt das
// Praegen und tut danach von selbst, was die Note von ihm braucht.
// SCHLAFEND wie die Kette: vor quantenWAb antwortet jeder Endpunkt 409, und
// der Takt tut nichts.
//
// # WAS DER VALIDATOR VERWAHRT (der Tresor)
//
// Je Note: seine Shamir-Anteile aller Stellen, das Geheimnis seiner
// Hash-Kette und die Blaetter (gesalzene Pruefsummen) der Note. Anteile und
// Geheimnis liegen verschluesselt in der eigenen Datenbank (AES-256-GCM,
// Schluessel aus dem Signierschluessel abgeleitet) -- ein Abzug der Datenbank
// allein gibt sie nicht her. Die Tabelle quanten_w_tresor ist KEIN Konsens:
// nicht in der StateRoot, nicht im Snapshot, kein anderer Knoten sieht sie.
// Geht sie verloren, kann dieser Validator fuer seine Noten weder Zufall noch
// Anteile liefern (ohne Zufall von ALLEN bleibt jede Anfrage offen) -- sie
// gehoert in die Sicherung des Betreibers.
//
// # DIE DREI EINGAENGE VON AUSSEN
//
//   - POST /api/quanten-w/validator/hinterlegen: ein Praeger hinterlegt die
//     Anteile dieses Validators und bekommt Anker und Bestaetigung. Geprueft,
//     bevor irgendetwas unterschrieben oder gespeichert wird: Form und
//     Grenzen, dieser Knoten steht genau einmal in der Liste, die Blaetter
//     ergeben die Wurzel, die Unterschrift des Praegers bindet GENAU diesen
//     Inhalt, der Praeger ist ein Mensch mit Mindestdeckung, K ist frei.
//     Wer zuerst hinterlegt, haelt K; ein zweiter Inhalt fuer dasselbe K wird
//     abgewiesen -- dieser Validator unterschreibt je K genau EINE Wurzel und
//     EINEN Anker. Ungepraegte Hinterlegungen verfallen nach dem
//     Praegefenster. Begrenzt: Koerper, Eintraege gesamt, offene je Praeger.
//   - GET /api/quanten-w/validator/anteile: die Anteile dieses Validators fuer
//     die Stellen der laufenden Anfrage -- erst wenn die ANTWORT des
//     Anfragenden final in der Kette steht (siehe unten). Danach sind sie
//     ohnehin fuer die Offenlegung bestimmt und oeffentlich.
//   - Der Takt liest nur den eigenen Zustand und reicht ueber dieselben
//     geprueften Wege ein wie jeder andere (POST /api/quanten-w/<art>).
//
// # NICHTS VOR DER FINALITAET
//
// Zwei Dinge gibt der Validator heraus, und beide erst, wenn ihr Anlass
// nicht mehr aus der Kette fallen kann (reif):
//
//   - den Wert seiner Hash-Kette, wenn die ANFRAGE final ist. Sonst koennte
//     der Anfragende alle Werte sehen, seine Anfrage in einer anderen Fassung
//     (andere Stellenzahl) neu unterbringen und sich so unter mehreren
//     Aufforderungen die guenstigste aussuchen.
//   - seine Anteile, wenn die ANTWORT final ist. Sonst koennte der Anfragende
//     die Anteile sehen, die Antwort zuruecknehmen und die richtige
//     einreichen -- jede Faelschung ginge durch.
//
// "Final": der Stand wurde bei DAG-Hoehe h zum ersten Mal gesehen, steht
// unveraendert da, und der finalisierte Checkpoint hat h erreicht (darunter
// ordnet kein Knoten mehr um, finality.go). Das kostet Sekunden; die Fristen
// der Regeln (600 s Antwort, 1800 s Offenlegung) sind dafuer bemessen.
//
// # WAS DER VALIDATOR NICHT PRUEFEN KANN
//
// Ob seine Anteile zu den Blaettern passen: ein einzelner Anteil sagt nichts
// ueber das Geheimnis. Ein Praeger, der falsche Anteile hinterlegt, macht
// seine eigene Note unpruefbar (jede Offenlegung scheitert an der Pruefsumme,
// das Pfand des Anfragenden kommt nach der Frist zurueck). Er schadet nur
// sich. Wer einer Note trauen will, prueft sie -- dafuer ist die Pruefung da.

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	quantenWDomHinterlegen = "aequitas-quanten-w-hinterlegen-v1"
	quantenWDomTresor      = "aequitas-quanten-w-tresor-v1|"

	// quantenWTresorHoechstens: Eintraege im Tresor gesamt -- die lebenden
	// Noten des Registers (quantenWNotenHoechstens) und ebenso viele
	// ungepraegte. Voll = keine neue Hinterlegung (fail-closed).
	quantenWTresorHoechstens = 2 * quantenWNotenHoechstens
	// quantenWTresorJeWallet: ungepraegte Hinterlegungen je Praeger.
	quantenWTresorJeWallet = 4
	// quantenWHinterlegenMax: Koerper einer Hinterlegung. 2048 Stellen mit
	// Anteil (bis 32 Hex) und Blatt (64 Hex) sind ~215 KB JSON.
	quantenWHinterlegenMax = 320 << 10
	// quantenWAnteileAntwortMax: Antwort eines anderen Validators auf die
	// Frage nach seinen Anteilen. 64 Stellen mit Anteil und 11 Pfadknoten
	// sind ~52 KB JSON.
	quantenWAnteileAntwortMax = 128 << 10
	burstQuantenWValJeIP      = 20

	quantenWValTakt = 5 * time.Second
	// quantenWValStaffel: der Validator mit Rang x wartet (x-1) Staffeln,
	// bevor er eine Offenlegung oder Frist einreicht, die jeder einreichen
	// darf -- sonst reichten alle im selben Augenblick dasselbe ein.
	quantenWValStaffel int64 = 10
	// quantenWValVersuche: so viele Auswahlen von t Validatoren probiert der
	// Dienst je Takt, wenn Anteile nicht zu den Blaettern passen.
	quantenWValVersuche = 50
	// quantenWSichtHoechstens: gemerkte Sichtungen (je Note hoechstens zwei
	// laufende). Voll = nichts wird reif (fail-closed).
	quantenWSichtHoechstens = 4 * quantenWTresorHoechstens
)

// quantenWTresorEintrag: was dieser Validator zu einer Note verwahrt.
type quantenWTresorEintrag struct {
	K           string
	X           int // Rang dieses Validators in der Note, 1-basiert
	N, T        int
	Wurzel      [32]byte
	Validatoren []string
	Zeit        int64 // Praegezeit, die die Bestaetigung bindet
	Praeger     string
	Inhalt      string // SHA-256 ueber Anteile und Blaetter (Hex)
	Geheim      [32]byte
	Anteile     []*big.Int
	Blaetter    [][32]byte
	Angelegt    int64
}

func (e *quantenWTresorEintrag) anker() [32]byte {
	return quantenWKetteVor(e.Geheim, quantenWZufallL)
}

// kettenWert: z(j) = H^(L-1-j)(Geheimnis), siehe quantenWKetteVor.
func (e *quantenWTresorEintrag) kettenWert(j int) ([32]byte, bool) {
	if j < 0 || j >= quantenWZufallL {
		return [32]byte{}, false
	}
	return quantenWKetteVor(e.Geheim, quantenWZufallL-1-j), true
}

// quantenWSicht: wann ein Stand zum ersten Mal gesehen wurde.
type quantenWSicht struct {
	hoehe  int64 // DAG-Hoehe bei der ersten Sichtung
	reifAb int64 // Unix-Zeit, zu der er zum ersten Mal reif war (0 = noch nicht)
}

// quantenWValidator: der Dienst eines Knotens mit Signierschluessel.
type quantenWValidator struct {
	cs  *ChainState
	key *ecdsa.PrivateKey
	ich string

	// Von aussen gesetzt (APIServer im Betrieb, Tests direkt).
	hoehe       func() int64                                                      // DAG-Hoehe
	final       func() int64                                                      // finalisierte Hoehe
	einreichen  func(art string, q *QuantenWAngaben) error                        // ueber den geprueften Annahmeweg
	urls        func() map[string]string                                          // Signieradresse -> URL der anderen Validatoren
	holeAnteile func(basis, k string, index int) (*quantenWAnteileAntwort, error) // GET beim anderen Validator

	mu       sync.Mutex
	tresor   map[string]*quantenWTresorEintrag
	sicht    map[string]*quantenWSicht
	gemeldet map[string]int64 // Art|K|Anfrage -> letzte Fehlermeldung (Unix)
}

// quantenWAnteileAntwort: GET /api/quanten-w/validator/anteile.
type quantenWAnteileAntwort struct {
	K       string     `json:"k"`
	Index   int        `json:"index"`
	X       int        `json:"x"`
	Stellen []int      `json:"stellen"`
	Anteile []string   `json:"anteile"` // je Stelle Y hexadezimal
	Pfade   [][]string `json:"pfade"`   // je Stelle der Merkle-Pfad
}

// neuerQuantenWValidator: der Dienst fuer diesen Schluessel; laedt den Tresor
// aus der Datenbank (wenn es eine gibt).
func neuerQuantenWValidator(cs *ChainState, key *ecdsa.PrivateKey) (*quantenWValidator, error) {
	if cs == nil || key == nil {
		return nil, errors.New("quanten_w: Validator-Dienst ohne Zustand oder Schluessel")
	}
	v := &quantenWValidator{cs: cs, key: key, ich: strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex()),
		tresor: map[string]*quantenWTresorEintrag{}, sicht: map[string]*quantenWSicht{}, gemeldet: map[string]int64{}}
	v.hoehe = func() int64 { return 0 }
	v.final = func() int64 { return -1 } // ohne Quelle wird nichts reif
	v.einreichen = func(string, *QuantenWAngaben) error { return errors.New("kein Annahmeweg gesetzt") }
	v.urls = func() map[string]string { return nil }
	v.holeAnteile = quantenWHoleAnteileHTTP
	if err := v.tresorLaden(); err != nil {
		return nil, err
	}
	return v, nil
}

// ------------------------------------------------------------ Tresor

func (v *quantenWValidator) tresorSchluessel() []byte {
	h := sha256.Sum256(append([]byte(quantenWDomTresor), crypto.FromECDSA(v.key)...))
	return h[:]
}

// tresorVersiegeln: Geheimnis und Anteile, AES-256-GCM, an K gebunden.
func (v *quantenWValidator) tresorVersiegeln(e *quantenWTresorEintrag) ([]byte, error) {
	klar := make([]byte, 0, 32+16*len(e.Anteile))
	klar = append(klar, e.Geheim[:]...)
	for _, y := range e.Anteile {
		var b [16]byte
		y.FillBytes(b[:])
		klar = append(klar, b[:]...)
	}
	blk, err := aes.NewCipher(v.tresorSchluessel())
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, klar, []byte(e.K)), nil
}

func (v *quantenWValidator) tresorOeffnen(e *quantenWTresorEintrag, siegel []byte) error {
	blk, err := aes.NewCipher(v.tresorSchluessel())
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(blk)
	if err != nil {
		return err
	}
	if len(siegel) < gcm.NonceSize() {
		return errors.New("Siegel zu kurz")
	}
	klar, err := gcm.Open(nil, siegel[:gcm.NonceSize()], siegel[gcm.NonceSize():], []byte(e.K))
	if err != nil {
		return errors.New("Siegel passt nicht zu diesem Schluessel")
	}
	if len(klar) != 32+16*e.N {
		return errors.New("Siegel: falsche Laenge")
	}
	copy(e.Geheim[:], klar[:32])
	e.Anteile = make([]*big.Int, e.N)
	for i := range e.Anteile {
		y := new(big.Int).SetBytes(klar[32+16*i : 48+16*i])
		if y.Cmp(quantenWP) >= 0 {
			return errors.New("Siegel: Anteil ausserhalb des Koerpers")
		}
		e.Anteile[i] = y
	}
	return nil
}

func quantenWTresorSchema(exec func(string)) {
	// quanten_w_tresor (quanten_w_validator.go): die Anteile DIESES
	// Validators. Kein Konsens, nicht im Snapshot.
	exec(`CREATE TABLE IF NOT EXISTS quanten_w_tresor (
k TEXT PRIMARY KEY,
x INT NOT NULL,
n INT NOT NULL,
t INT NOT NULL,
wurzel TEXT NOT NULL,
validatoren TEXT NOT NULL,
zeit BIGINT NOT NULL,
praeger TEXT NOT NULL,
inhalt TEXT NOT NULL,
siegel BYTEA NOT NULL,
blaetter BYTEA NOT NULL,
angelegt BIGINT NOT NULL
)`)
}

func (v *quantenWValidator) tresorLaden() error {
	if v.cs.db == nil {
		return nil
	}
	rows, err := v.cs.db.Query(`SELECT k, x, n, t, wurzel, validatoren, zeit, praeger, inhalt, siegel, blaetter, angelegt
		FROM quanten_w_tresor ORDER BY k LIMIT $1`, quantenWTresorHoechstens+1)
	if err != nil {
		return fmt.Errorf("quanten_w_tresor nicht lesbar: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		e := &quantenWTresorEintrag{}
		var wurzel, validatoren string
		var siegel, blaetter []byte
		if err := rows.Scan(&e.K, &e.X, &e.N, &e.T, &wurzel, &validatoren, &e.Zeit, &e.Praeger, &e.Inhalt, &siegel, &blaetter, &e.Angelegt); err != nil {
			return fmt.Errorf("quanten_w_tresor nicht lesbar: %w", err)
		}
		if len(v.tresor) >= quantenWTresorHoechstens {
			return fmt.Errorf("quanten_w_tresor: mehr als %d Eintraege", quantenWTresorHoechstens)
		}
		w, ok := quantenWHex32(wurzel)
		if !ok || !istHash64(e.K) || e.N <= 0 || e.N > quantenWNMax || len(blaetter) != 32*e.N ||
			json.Unmarshal([]byte(validatoren), &e.Validatoren) != nil || e.X < 1 || e.X > len(e.Validatoren) {
			return fmt.Errorf("quanten_w_tresor: Eintrag %.12s hat keine gueltige Form", e.K)
		}
		e.Wurzel = w
		e.Blaetter = make([][32]byte, e.N)
		for i := range e.Blaetter {
			copy(e.Blaetter[i][:], blaetter[32*i:32*i+32])
		}
		// Ein Eintrag, der sich mit diesem Schluessel nicht oeffnen laesst
		// (Schluessel gewechselt), wird gemeldet und uebergangen, nicht
		// geloescht: mit dem alten Schluessel ist er wieder lesbar.
		if err := v.tresorOeffnen(e, siegel); err != nil {
			fmt.Printf("[QUANTEN-W] ⚠ Tresor-Eintrag %.12s nicht lesbar (%v) -- fuer diese Note kann dieser Knoten nichts liefern\n", e.K, err)
			continue
		}
		v.tresor[e.K] = e
	}
	return rows.Err()
}

func (v *quantenWValidator) tresorSpeichern(e *quantenWTresorEintrag) error {
	if v.cs.db == nil {
		return nil
	}
	siegel, err := v.tresorVersiegeln(e)
	if err != nil {
		return err
	}
	blaetter := make([]byte, 0, 32*len(e.Blaetter))
	for i := range e.Blaetter {
		blaetter = append(blaetter, e.Blaetter[i][:]...)
	}
	validatoren, _ := json.Marshal(e.Validatoren)
	// Ohne ON CONFLICT: ein zweiter Inhalt fuer dasselbe K scheitert hier
	// noch einmal, auch wenn zwei Prozesse dieselbe Datenbank teilten.
	_, err = v.cs.db.Exec(`INSERT INTO quanten_w_tresor (k, x, n, t, wurzel, validatoren, zeit, praeger, inhalt, siegel, blaetter, angelegt)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		e.K, e.X, e.N, e.T, hex.EncodeToString(e.Wurzel[:]), string(validatoren), e.Zeit, e.Praeger, e.Inhalt, siegel, blaetter, e.Angelegt)
	return err
}

// tresorLoeschenLocked: der Eintrag ist nicht mehr noetig (Note tot oder nie
// gepraegt). Erst die Datenbank, dann der Speicher: scheitert das Loeschen,
// bleibt er und wird im naechsten Takt wieder versucht.
func (v *quantenWValidator) tresorLoeschenLocked(k, grund string) {
	if v.cs.db != nil {
		if _, err := v.cs.db.Exec(`DELETE FROM quanten_w_tresor WHERE k = $1`, k); err != nil {
			fmt.Printf("[QUANTEN-W] ⚠ Tresor-Eintrag %.12s nicht geloescht: %v\n", k, err)
			return
		}
	}
	delete(v.tresor, k)
	fmt.Printf("[QUANTEN-W] Tresor-Eintrag %.12s entfernt (%s)\n", k, grund)
}

// ------------------------------------------------------------ Hinterlegen

// QuantenWHinterlegung: Koerper von POST /api/quanten-w/validator/hinterlegen.
type QuantenWHinterlegung struct {
	Wallet      string   `json:"wallet"`
	Zeit        int64    `json:"zeit"`
	K           string   `json:"k"`
	N           int      `json:"n"`
	T           int      `json:"t"`
	Wurzel      string   `json:"wurzel"`
	Validatoren []string `json:"validatoren"`
	Anteile     []string `json:"anteile"`  // je Stelle der Anteil DIESES Validators, Hex
	Blaetter    []string `json:"blaetter"` // je Stelle die Pruefsumme, 64 Hex
	WalletSig   string   `json:"wallet_sig,omitempty"`
	PQ          *PQKonto `json:"pq,omitempty"`
}

// QuantenWBestaetigung: die Antwort -- was der Praeger in quanten_w_praegen
// fuer diesen Validator eintraegt.
type QuantenWBestaetigung struct {
	Validator    string `json:"validator"`
	X            int    `json:"x"`
	Anker        string `json:"anker"`
	Bestaetigung string `json:"bestaetigung"`
	Zeit         int64  `json:"zeit"`
}

// quantenWInhalt: bindet die hinterlegten Anteile und Blaetter (in der
// Reihenfolge der Stellen) an die Unterschrift des Praegers.
func quantenWInhalt(anteile []*big.Int, blaetter [][32]byte) string {
	h := sha256.New()
	for _, y := range anteile {
		var b [16]byte
		y.FillBytes(b[:])
		h.Write(b[:])
	}
	for i := range blaetter {
		h.Write(blaetter[i][:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

func quantenWHinterlegenNachricht(wallet, K string, n, t int, wurzel string, validatoren []string, x int, inhalt string, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%d|%s|%s|%d|%s|%d", quantenWDomHinterlegen, eingefroreneChainID,
		strings.ToLower(wallet), K, n, t, wurzel, strings.Join(validatoren, ","), x, inhalt, zeit)
}

// quantenWValAbweisung: eine Ablehnung mit HTTP-Status fuer den Aufrufer.
type quantenWValAbweisung struct {
	status int
	text   string
}

func (e *quantenWValAbweisung) Error() string { return e.text }

func quantenWValNein(status int, format string, args ...interface{}) error {
	return &quantenWValAbweisung{status: status, text: fmt.Sprintf(format, args...)}
}

// hinterlegen prueft eine Hinterlegung vollstaendig, speichert sie und
// bestaetigt. jetzt = Uhr dieses Knotens (Unix).
func (v *quantenWValidator) hinterlegen(h *QuantenWHinterlegung, jetzt int64) (*QuantenWBestaetigung, error) {
	wallet := strings.ToLower(strings.TrimSpace(h.Wallet))
	K := strings.ToLower(strings.TrimSpace(h.K))
	// Form und Grenzen -- billig, vor jeder Rechnung.
	if !kanonischeAdresse(wallet) || istSystemAdresse(wallet) || !istHash64(K) {
		return nil, quantenWValNein(http.StatusBadRequest, "wallet oder k nicht in gueltiger Form")
	}
	wurzel, ok := quantenWHex32(h.Wurzel)
	if !ok || len(h.Validatoren) == 0 || len(h.Validatoren) > quantenWVMax {
		return nil, quantenWValNein(http.StatusBadRequest, "wurzel oder validatoren nicht in gueltiger Form")
	}
	x := 0
	for i, a := range h.Validatoren {
		if !kanonischeAdresse(a) {
			return nil, quantenWValNein(http.StatusBadRequest, "validator %d nicht kanonisch", i+1)
		}
		if a == v.ich {
			if x != 0 {
				return nil, quantenWValNein(http.StatusBadRequest, "dieser Validator steht mehrfach in der Liste")
			}
			x = i + 1
		}
	}
	if x == 0 {
		return nil, quantenWValNein(http.StatusBadRequest, "dieser Knoten (%s) ist kein Validator der Note", kurzAdresse(v.ich))
	}
	if err := quantenWPraegenPruefen(false, K, h.N, h.T, h.Validatoren, len(h.Validatoren), len(h.Validatoren)); err != nil {
		return nil, quantenWValNein(http.StatusBadRequest, "%v", err)
	}
	if len(h.Anteile) != h.N || len(h.Blaetter) != h.N {
		return nil, quantenWValNein(http.StatusBadRequest, "anteile und blaetter: je Stelle genau einer (n = %d)", h.N)
	}
	if jetzt-h.Zeit > quantenWVoraus || h.Zeit-jetzt > quantenWVoraus {
		return nil, quantenWValNein(http.StatusBadRequest, "zeit weicht mehr als %d s von der Uhr dieses Knotens ab", quantenWVoraus)
	}
	anteile := make([]*big.Int, h.N)
	blaetter := make([][32]byte, h.N)
	for i := 0; i < h.N; i++ {
		s := h.Anteile[i]
		if len(s) == 0 || len(s) > 32 || !kleinHex(s) {
			return nil, quantenWValNein(http.StatusBadRequest, "anteil %d nicht in gueltiger Form", i)
		}
		y, ok := new(big.Int).SetString(s, 16)
		if !ok || y.Cmp(quantenWP) >= 0 {
			return nil, quantenWValNein(http.StatusBadRequest, "anteil %d ausserhalb des Koerpers", i)
		}
		b, ok := quantenWHex32(h.Blaetter[i])
		if !ok {
			return nil, quantenWValNein(http.StatusBadRequest, "blatt %d nicht in gueltiger Form", i)
		}
		anteile[i], blaetter[i] = y, b
	}
	if quantenWMerkleWurzel(blaetter) != wurzel {
		return nil, quantenWValNein(http.StatusBadRequest, "die Blaetter ergeben nicht die Wurzel")
	}
	inhalt := quantenWInhalt(anteile, blaetter)
	// Die Unterschrift des Praegers ueber genau diesen Inhalt.
	sig := &QuantenWAngaben{WalletSig: h.WalletSig, PQ: h.PQ}
	if err := quantenWUnterschrift(sig, quantenWHinterlegenNachricht(wallet, K, h.N, h.T, h.Wurzel, h.Validatoren, x, inhalt, h.Zeit), wallet, jetzt); err != nil {
		return nil, quantenWValNein(http.StatusForbidden, "Unterschrift des Praegers: %v", err)
	}
	// Der Praeger: ein Mensch, der die Mindestdeckung hat.
	v.cs.ensureAccountLoaded(wallet)
	v.cs.mu.RLock()
	acc, hat := v.cs.accounts.Get(wallet)
	mensch := hat && acc.IsHuman
	var guthaben int64
	if hat {
		guthaben = acc.Balance.Micro()
	}
	reg := v.cs.quantenWJetzt()
	_, tot := reg.Tote[K]
	lebt := reg.Noten[K] != nil
	v.cs.mu.RUnlock()
	if !mensch || guthaben < quantenMindestDeckungMik {
		return nil, quantenWValNein(http.StatusForbidden, "nur ein Mensch mit mindestens %d Mikro-AEQ hinterlegt", quantenMindestDeckungMik)
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	if alt := v.tresor[K]; alt != nil {
		// Dieselbe Hinterlegung noch einmal: dieselbe Antwort. Alles andere
		// ist ein zweiter Inhalt fuer dasselbe K.
		if alt.Inhalt == inhalt && alt.Praeger == wallet && alt.Zeit == h.Zeit && alt.N == h.N && alt.T == h.T &&
			alt.Wurzel == wurzel && alt.X == x && strings.Join(alt.Validatoren, ",") == strings.Join(h.Validatoren, ",") {
			return v.bestaetigen(alt)
		}
		return nil, quantenWValNein(http.StatusConflict, "fuer k ist schon etwas anderes hinterlegt")
	}
	if tot || lebt {
		return nil, quantenWValNein(http.StatusConflict, "die Note %s gibt es schon", K[:12])
	}
	if len(v.tresor) >= quantenWTresorHoechstens {
		return nil, quantenWValNein(http.StatusServiceUnavailable, "der Tresor dieses Validators ist voll")
	}
	offen := 0
	for _, e := range v.tresor {
		if e.Praeger == wallet && reg.Noten[e.K] == nil {
			offen++
		}
	}
	if offen >= quantenWTresorJeWallet {
		return nil, quantenWValNein(http.StatusTooManyRequests, "schon %d ungepraegte Hinterlegungen dieses Praegers", offen)
	}
	e := &quantenWTresorEintrag{K: K, X: x, N: h.N, T: h.T, Wurzel: wurzel, Validatoren: append([]string(nil), h.Validatoren...),
		Zeit: h.Zeit, Praeger: wallet, Inhalt: inhalt, Anteile: anteile, Blaetter: blaetter, Angelegt: jetzt}
	if _, err := io.ReadFull(rand.Reader, e.Geheim[:]); err != nil {
		return nil, fmt.Errorf("quanten_w: kein Zufall fuer die Hash-Kette: %w", err)
	}
	// Erst speichern, dann bestaetigen: eine Bestaetigung, zu der dieser
	// Knoten nach einem Absturz nichts mehr haette, bliebe eine Note ohne
	// Zufall.
	if err := v.tresorSpeichern(e); err != nil {
		return nil, fmt.Errorf("quanten_w: Tresor nicht geschrieben: %w", err)
	}
	v.tresor[K] = e
	return v.bestaetigen(e)
}

// bestaetigen: die Unterschrift dieses Validators ueber Wurzel, Anker und
// Praegezeit (quantenWBestaetigungNachricht) -- je Eintrag immer dieselbe.
func (v *quantenWValidator) bestaetigen(e *quantenWTresorEintrag) (*QuantenWBestaetigung, error) {
	anker := e.anker()
	msg := quantenWBestaetigungNachricht(e.K, e.Wurzel, e.N, e.T, e.Validatoren, e.X, anker, e.Zeit)
	sig, err := crypto.Sign(accounts.TextHash([]byte(msg)), v.key)
	if err != nil {
		return nil, fmt.Errorf("quanten_w: Bestaetigung nicht unterschrieben: %w", err)
	}
	sig[64] += 27
	return &QuantenWBestaetigung{Validator: v.ich, X: e.X, Anker: hex.EncodeToString(anker[:]), Bestaetigung: hexutil.Encode(sig), Zeit: e.Zeit}, nil
}

// ------------------------------------------------------------ Reife

// reifLocked: ist der Stand unter diesem Schluessel final? Die erste Frage
// merkt die DAG-Hoehe; reif ist er, wenn der finalisierte Checkpoint sie
// erreicht hat. Rueckgabe: reif, und seit wann (Unix).
func (v *quantenWValidator) reifLocked(schluessel string, jetzt int64) (bool, int64) {
	s := v.sicht[schluessel]
	if s == nil {
		if len(v.sicht) >= quantenWSichtHoechstens {
			return false, 0
		}
		v.sicht[schluessel] = &quantenWSicht{hoehe: v.hoehe()}
		return false, 0
	}
	if s.reifAb == 0 {
		if v.final() < s.hoehe {
			return false, 0
		}
		s.reifAb = jetzt
	}
	return true, s.reifAb
}

func quantenWAnfrageSchluessel(K string, a *quantenWAnfrage) string {
	return fmt.Sprintf("z|%s|%d|%s|%d|%d|%d|%s", K, a.Index, a.Von, a.K, a.Zeit, a.Pfand, a.EinloesenAn)
}

func quantenWAntwortSchluessel(K string, a *quantenWAnfrage) string {
	var sb strings.Builder
	for _, b := range a.Antwort {
		sb.WriteByte(byte('0' + b))
	}
	return fmt.Sprintf("o|%s|%d|%s|%d|%v|%v|%s|%d", K, a.Index, a.Von, a.Gezogen, a.Stellen, a.Basen, sb.String(), a.AntwortZeit)
}

// ------------------------------------------------------------ Anteile

// anteileFuer: die Anteile dieses Validators fuer die laufende Anfrage --
// nur wenn die Antwort final in der Kette steht. Auch der eigene Takt geht
// durch diese Pruefung.
func (v *quantenWValidator) anteileFuer(K string, index int, jetzt int64) (*quantenWAnteileAntwort, error) {
	v.cs.mu.RLock()
	var a *quantenWAnfrage
	var note *quantenWNote
	if e := v.cs.quantenWJetzt().Noten[K]; e != nil {
		note = e.note
		a = e.note.Anfrage
	}
	v.cs.mu.RUnlock()
	v.mu.Lock()
	defer v.mu.Unlock()
	e := v.tresor[K]
	if e == nil || note == nil || !v.passtLocked(e, note) {
		return nil, quantenWValNein(http.StatusNotFound, "fuer diese Note verwahrt dieser Validator nichts")
	}
	if a == nil || a.Index != index || a.Antwort == nil {
		return nil, quantenWValNein(http.StatusConflict, "keine beantwortete Anfrage %d an dieser Note", index)
	}
	if reif, _ := v.reifLocked(quantenWAntwortSchluessel(K, a), jetzt); !reif {
		return nil, quantenWValNein(http.StatusTooEarly, "die Antwort ist noch nicht final")
	}
	out := &quantenWAnteileAntwort{K: K, Index: index, X: e.X, Stellen: append([]int(nil), a.Stellen...)}
	for _, s := range a.Stellen {
		if s < 0 || s >= e.N {
			return nil, quantenWValNein(http.StatusConflict, "Stelle ausserhalb der Note")
		}
		out.Anteile = append(out.Anteile, e.Anteile[s].Text(16))
		var pfad []string
		for _, p := range quantenWMerklePfad(e.Blaetter, s) {
			pfad = append(pfad, hex.EncodeToString(p[:]))
		}
		out.Pfade = append(out.Pfade, pfad)
	}
	return out, nil
}

// passtLocked: gehoert die Note im Register zu diesem Tresor-Eintrag?
func (v *quantenWValidator) passtLocked(e *quantenWTresorEintrag, note *quantenWNote) bool {
	return note.N == e.N && note.T == e.T && note.Wurzel == e.Wurzel && e.X >= 1 && e.X <= len(note.Validatoren) &&
		note.Validatoren[e.X-1] == v.ich && strings.Join(note.Validatoren, ",") == strings.Join(e.Validatoren, ",")
}

// quantenWHoleAnteileHTTP: die Anteile eines anderen Validators, ueber den
// Klienten der Leitung (SSRF-Schutz, 3 s, keine Umleitung).
func quantenWHoleAnteileHTTP(basis, k string, index int) (*quantenWAnteileAntwort, error) {
	u := strings.TrimRight(basis, "/") + "/api/quanten-w/validator/anteile?k=" + url.QueryEscape(k) + "&index=" + strconv.Itoa(index)
	resp, err := leitungKlient.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	roh, err := io.ReadAll(io.LimitReader(resp.Body, quantenWAnteileAntwortMax+1))
	if err != nil {
		return nil, err
	}
	if len(roh) > quantenWAnteileAntwortMax {
		return nil, errors.New("Antwort zu gross")
	}
	var out quantenWAnteileAntwort
	if err := json.Unmarshal(roh, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ------------------------------------------------------------ Takt

// melden: ein Fehlschlag, hoechstens einmal je Minute und Auftrag -- der
// Takt kommt alle paar Sekunden wieder. Die Karte ist durch den Tresor
// begrenzt (je Note ein laufender Auftrag) und wird im Takt geleert.
func (v *quantenWValidator) melden(auf *quantenWAuftrag, jetzt int64, format string, args ...interface{}) {
	sk := fmt.Sprintf("%s|%s|%d", auf.art, auf.k, auf.index)
	v.mu.Lock()
	zuletzt, bekannt := v.gemeldet[sk]
	if bekannt && jetzt-zuletzt < 60 {
		v.mu.Unlock()
		return
	}
	if !bekannt && len(v.gemeldet) >= quantenWSichtHoechstens {
		v.mu.Unlock()
		return
	}
	v.gemeldet[sk] = jetzt
	v.mu.Unlock()
	fmt.Printf("[QUANTEN-W] "+format+"\n", args...)
}

// quantenWAuftrag: was der Takt nach dem Lesen (ohne Sperre) einreicht.
type quantenWAuftrag struct {
	art   string
	k     string
	index int
	q     *QuantenWAngaben // nil bei der Offenlegung: erst sammeln
	// Offenlegung
	eintrag *quantenWTresorEintrag
	anfrage *quantenWAnfrage
}

// takt: ein Durchgang. Liest den eigenen Zustand, raeumt den Tresor auf und
// reicht ein, was faellig und reif ist. jetzt = Uhr dieses Knotens (Unix).
// Rueckgabe: wie viele Einreichungen angenommen wurden (fuer Tests).
func (v *quantenWValidator) takt(jetzt int64) int {
	if !quantenWAktiv(jetzt) {
		return 0
	}
	v.cs.mu.RLock()
	reg := v.cs.quantenWJetzt()
	v.cs.mu.RUnlock()

	var auftraege []quantenWAuftrag
	v.mu.Lock()
	gesehen := map[string]bool{}
	schluesselK := make([]string, 0, len(v.tresor))
	for k := range v.tresor {
		schluesselK = append(schluesselK, k)
	}
	sort.Strings(schluesselK)
	for _, K := range schluesselK {
		e := v.tresor[K]
		eintrag := reg.Noten[K]
		if eintrag == nil {
			if _, tot := reg.Tote[K]; tot {
				v.tresorLoeschenLocked(K, "Note tot")
			} else if jetzt > e.Zeit+quantenWAlt+quantenWVoraus+quantenWBuchSpiel {
				// Die Bestaetigung gilt nicht mehr: gepraegt wird diese
				// Hinterlegung nie.
				v.tresorLoeschenLocked(K, "nie gepraegt, Praegefenster vorbei")
			}
			continue
		}
		note := eintrag.note
		if !v.passtLocked(e, note) {
			// K wurde mit anderem Inhalt gepraegt (ohne diesen Validator):
			// der Eintrag gehoert zu keiner Note.
			if jetzt > e.Zeit+quantenWAlt+quantenWVoraus+quantenWBuchSpiel {
				v.tresorLoeschenLocked(K, "K anders gepraegt")
			}
			continue
		}
		a := note.Anfrage
		if a == nil {
			continue
		}
		switch {
		case a.Basen == nil:
			// Zufall: mein Wert fuer diese Anfrage, wenn sie final ist.
			sk := quantenWAnfrageSchluessel(K, a)
			gesehen[sk] = true
			if note.ZIndex[e.X-1] >= a.Index {
				continue
			}
			if reif, _ := v.reifLocked(sk, jetzt); !reif {
				continue
			}
			z, ok := e.kettenWert(a.Index)
			if !ok {
				continue
			}
			auftraege = append(auftraege, quantenWAuftrag{art: quantenWZufallTxArt, k: K, index: a.Index,
				q: &QuantenWAngaben{K: K, Index: a.Index, Zufall: []QuantenWZufall{{V: e.X, Z: hex.EncodeToString(z[:])}}}})
		case a.Antwort == nil:
			// Keine Antwort bis zur Frist: durchgefallen. Jeder darf das
			// einreichen; gestaffelt nach Rang.
			if jetzt > a.Gezogen+quantenWFristAntwort+int64(e.X-1)*quantenWValStaffel {
				auftraege = append(auftraege, quantenWAuftrag{art: quantenWFristTxArt, k: K, index: a.Index, q: &QuantenWAngaben{K: K, Index: a.Index}})
			}
		default:
			sk := quantenWAntwortSchluessel(K, a)
			gesehen[sk] = true
			if jetzt > a.AntwortZeit+quantenWFristOffen {
				if jetzt > a.AntwortZeit+quantenWFristOffen+int64(e.X-1)*quantenWValStaffel {
					auftraege = append(auftraege, quantenWAuftrag{art: quantenWFristTxArt, k: K, index: a.Index, q: &QuantenWAngaben{K: K, Index: a.Index}})
				}
				continue
			}
			reif, seit := v.reifLocked(sk, jetzt)
			if !reif || jetzt < seit+int64(e.X-1)*quantenWValStaffel {
				continue
			}
			auftraege = append(auftraege, quantenWAuftrag{art: quantenWOffenlegenTxArt, k: K, index: a.Index, eintrag: e, anfrage: a})
		}
	}
	// Sichtungen, deren Stand es nicht mehr gibt, vergessen.
	for sk := range v.sicht {
		if !gesehen[sk] {
			delete(v.sicht, sk)
		}
	}
	for sk, zuletzt := range v.gemeldet {
		if jetzt-zuletzt > 600 {
			delete(v.gemeldet, sk)
		}
	}
	v.mu.Unlock()

	angenommen := 0
	for i := range auftraege {
		auf := &auftraege[i]
		if auf.art == quantenWOffenlegenTxArt {
			q, err := v.offenlegungSammeln(auf.eintrag, auf.anfrage, jetzt)
			if err != nil {
				v.melden(auf, jetzt, "Offenlegung fuer %.12s (Anfrage %d) noch nicht moeglich: %v", auf.k, auf.index, err)
				continue
			}
			auf.q = q
		}
		if err := v.einreichen(auf.art, auf.q); err != nil {
			v.melden(auf, jetzt, "%s fuer %.12s (Anfrage %d) nicht angenommen: %v", auf.art, auf.k, auf.index, err)
			continue
		}
		angenommen++
		fmt.Printf("[QUANTEN-W] ✓ %s fuer %.12s (Anfrage %d) eingereicht\n", auf.art, auf.k, auf.index)
	}
	return angenommen
}

// offenlegungSammeln: t Anteile je Stelle -- die eigenen und die anderer
// Validatoren der Note -- und der Merkle-Pfad. Jede Auswahl wird gegen die
// eigenen Blaetter geprueft, bevor sie eingereicht wird: was ein anderer
// Validator liefert, wird nicht geglaubt.
func (v *quantenWValidator) offenlegungSammeln(e *quantenWTresorEintrag, a *quantenWAnfrage, jetzt int64) (*QuantenWAngaben, error) {
	eigene, err := v.anteileFuer(e.K, a.Index, jetzt)
	if err != nil {
		return nil, err
	}
	// je Validator-Rang die Anteile der Stellen
	von := map[int][]*big.Int{}
	lesen := func(r *quantenWAnteileAntwort, rang int) bool {
		if r == nil || r.K != e.K || r.Index != a.Index || r.X != rang || len(r.Anteile) != len(a.Stellen) {
			return false
		}
		ys := make([]*big.Int, len(r.Anteile))
		for i, s := range r.Anteile {
			if len(s) == 0 || len(s) > 32 || !kleinHex(s) {
				return false
			}
			y, ok := new(big.Int).SetString(s, 16)
			if !ok || y.Cmp(quantenWP) >= 0 {
				return false
			}
			ys[i] = y
		}
		von[rang] = ys
		return true
	}
	if !lesen(eigene, e.X) {
		return nil, errors.New("eigene Anteile nicht lesbar")
	}
	urls := v.urls()
	for i, adr := range e.Validatoren {
		rang := i + 1
		if rang == e.X || len(von) >= quantenWTMax {
			continue
		}
		basis := urls[adr]
		if basis == "" {
			continue
		}
		r, err := v.holeAnteile(basis, e.K, a.Index)
		if err != nil {
			continue // nicht erreichbar oder noch nicht final: der naechste Takt fragt wieder
		}
		lesen(r, rang)
	}
	if len(von) < e.T {
		return nil, fmt.Errorf("erst %d von %d Validatoren haben geliefert", len(von), e.T)
	}
	raenge := make([]int, 0, len(von))
	for r := range von {
		if r != e.X {
			raenge = append(raenge, r)
		}
	}
	sort.Ints(raenge)
	// Auswahlen von t Raengen, immer mit dem eigenen; hoechstens
	// quantenWValVersuche. Eine Auswahl gilt, wenn JEDE Stelle zum Blatt passt.
	versuche := 0
	var wahl []int
	var probieren func(ab int, bisher []int) bool
	probieren = func(ab int, bisher []int) bool {
		if len(bisher) == e.T-1 {
			versuche++
			kand := append([]int{e.X}, bisher...)
			for i, s := range a.Stellen {
				an := make([]quantenWAnteil, 0, e.T)
				for _, r := range kand {
					an = append(an, quantenWAnteil{X: r, Y: von[r][i]})
				}
				if quantenWPruefsumme(e.K, s, quantenWZusammensetzen(an)) != e.Blaetter[s] {
					return false
				}
			}
			wahl = kand
			return true
		}
		for i := ab; i < len(raenge) && versuche < quantenWValVersuche; i++ {
			if probieren(i+1, append(append([]int(nil), bisher...), raenge[i])) {
				return true
			}
		}
		return false
	}
	if !probieren(0, nil) {
		return nil, fmt.Errorf("keine Auswahl von %d Validatoren ergibt die Pruefsummen (%d Versuche)", e.T, versuche)
	}
	sort.Ints(wahl)
	q := &QuantenWAngaben{K: e.K, Index: a.Index}
	for i := range a.Stellen {
		o := QuantenWOffen{Pfad: eigene.Pfade[i]}
		for _, r := range wahl {
			o.Anteile = append(o.Anteile, QuantenWAnteilAngabe{X: r, Y: von[r][i].Text(16)})
		}
		q.Offen = append(q.Offen, o)
	}
	return q, nil
}

// ------------------------------------------------------------ Einbindung

// quantenWValFuer: der Dienst dieses Knotens (nil = keiner).
func (a *APIServer) quantenWValFuer() *quantenWValidator {
	if a == nil || a.state == nil {
		return nil
	}
	return a.state.quantenWVal.Load()
}

// quantenWValidatorEinrichten: der Dienst dieses Knotens, an Kette, Leitung
// und Annahmeweg angeschlossen (nil = kein Signierschluessel oder der Tresor
// ist nicht lesbar). Ohne Takt -- den startet StarteQuantenWValidator.
func (a *APIServer) quantenWValidatorEinrichten() *quantenWValidator {
	if a == nil || a.state == nil || a.blockchain == nil {
		return nil
	}
	key := a.blockchain.GetSigningKey()
	if key == nil {
		return nil
	}
	v, err := neuerQuantenWValidator(a.state, key)
	if err != nil {
		// Fail-closed: ohne lesbaren Tresor kein Dienst -- keine
		// Bestaetigung, kein Zufall, keine Anteile.
		fmt.Printf("[QUANTEN-W] ✗ Validator-Dienst nicht gestartet: %v\n", err)
		return nil
	}
	v.hoehe = a.blockchain.Height
	v.final = func() int64 {
		h, _ := a.state.GetFinalizedCheckpoint()
		return h
	}
	v.urls = func() map[string]string {
		if l := a.state.leitung.Load(); l != nil {
			return l.URLs()
		}
		return nil
	}
	v.einreichen = a.quantenWIntern
	a.state.quantenWVal.Store(v)
	return v
}

// StarteQuantenWValidator: richtet den Dienst ein, wenn dieser Knoten einen
// Signierschluessel hat, und startet den Takt. Vor quantenWAb tut der Takt
// nichts und die Endpunkte antworten 409. Einmal beim Start rufen.
func (a *APIServer) StarteQuantenWValidator() {
	v := a.quantenWValidatorEinrichten()
	if v == nil {
		return
	}
	SafeGoroutine("quantenWValidator", func() {
		tk := time.NewTicker(quantenWValTakt)
		defer tk.Stop()
		for range tk.C {
			SafeCall("quantenWValidatorTakt", func() { v.takt(time.Now().Unix()) })
		}
	})
}

// quantenWInternAntwort: der Empfaenger einer Einreichung im eigenen Prozess.
type quantenWInternAntwort struct {
	kopf   http.Header
	status int
	rumpf  bytes.Buffer
}

func (w *quantenWInternAntwort) Header() http.Header { return w.kopf }
func (w *quantenWInternAntwort) WriteHeader(s int) {
	if w.status == 0 {
		w.status = s
	}
}
func (w *quantenWInternAntwort) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.rumpf.Len()+len(b) > 4<<10 {
		return len(b), nil // die Antwort wird nur gemeldet; mehr braucht es nicht
	}
	return w.rumpf.Write(b)
}

// SetReadDeadline: fuer http.ResponseController in zumLeiter; der Rumpf
// liegt schon im Speicher.
func (w *quantenWInternAntwort) SetReadDeadline(time.Time) error { return nil }

// quantenWIntern: eine Einreichung des eigenen Dienstes -- durch dieselbe
// Kette wie jede von aussen (zumLeiter, handleQuantenW): ist dieser Knoten
// nicht der Leiter, geht sie an ihn weiter, und geprueft wird sie dort wie
// jede andere.
func (a *APIServer) quantenWIntern(art string, q *QuantenWAngaben) error {
	roh, err := json.Marshal(map[string]interface{}{"wallet": "", "zeit": 0, "quanten_w": q})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1/api/quanten-w/"+strings.TrimPrefix(art, "quanten_w_"), bytes.NewReader(roh))
	if err != nil {
		return err
	}
	req.RemoteAddr = "127.0.0.1:0"
	req.Header.Set("Content-Type", "application/json")
	w := &quantenWInternAntwort{kopf: http.Header{}}
	a.zumLeiter(a.handleQuantenW(art))(w, req)
	if w.status != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", w.status, strings.TrimSpace(w.rumpf.String()))
	}
	return nil
}

// handleQuantenWHinterlegen: POST /api/quanten-w/validator/hinterlegen.
func (a *APIServer) handleQuantenWHinterlegen(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	jetzt := time.Now().Unix()
	if !quantenWAktiv(jetzt) {
		jsonError(w, "quantum money variant W not active", http.StatusConflict)
		return
	}
	v := a.quantenWValFuer()
	if v == nil {
		jsonError(w, "this node is not a validator", http.StatusServiceUnavailable)
		return
	}
	if !burstErlaubt("quanten-w-val:"+erneuerungsAbsender(r), burstQuantenWValJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	if r.ContentLength > quantenWHinterlegenMax {
		w.Header().Set("Connection", "close")
		jsonError(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	var req QuantenWHinterlegung
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, quantenWHinterlegenMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	best, err := v.hinterlegen(&req, jetzt)
	if err != nil {
		var ab *quantenWValAbweisung
		if errors.As(err, &ab) {
			jsonError(w, "rejected: "+ab.text, ab.status)
			return
		}
		fmt.Printf("[API] quanten-w hinterlegen gescheitert: %v\n", err)
		jsonError(w, "internal error, please retry shortly", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(best)
}

// handleQuantenWAnteile: GET /api/quanten-w/validator/anteile?k=<K>&index=<j>.
func (a *APIServer) handleQuantenWAnteile(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodGet {
		jsonError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	jetzt := time.Now().Unix()
	if !quantenWAktiv(jetzt) {
		jsonError(w, "quantum money variant W not active", http.StatusConflict)
		return
	}
	v := a.quantenWValFuer()
	if v == nil {
		jsonError(w, "this node is not a validator", http.StatusServiceUnavailable)
		return
	}
	if !burstErlaubt("quanten-w-val:"+erneuerungsAbsender(r), burstQuantenWValJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	k := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("k")))
	index, err := strconv.Atoi(r.URL.Query().Get("index"))
	if !istHash64(k) || err != nil || index < 0 || index >= quantenWZufallL {
		jsonError(w, "k: 64 hex characters, index: number of the request", http.StatusBadRequest)
		return
	}
	out, err := v.anteileFuer(k, index, jetzt)
	if err != nil {
		var ab *quantenWValAbweisung
		if errors.As(err, &ab) {
			jsonError(w, ab.text, ab.status)
			return
		}
		jsonError(w, "internal error, please retry shortly", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(out)
}
