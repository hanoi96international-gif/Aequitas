package keeper

// QUANTENGELD VARIANTE W -- STUFE W2d: DER DIENST DER VALIDATOREN (schlafend)
//
// Was ein Validator einer W-Note tun muss, damit die Kette sie pruefen kann
// (Regeln: quanten_w*.go, Spezifikation wiesner_kette.py):
//
//   - TRESOR: seine Anteile (je Stelle ein Shamir-Anteil an x = seine Nummer),
//     die Merkle-Blaetter (fuer die Pfade) und das Geheimnis seiner Hash-Kette.
//     Verschluesselt abgelegt (AES-256-GCM, Schluessel aus dem Signierschluessel
//     des Knotens, K als Zusatzdaten), hoechstens quantenWTresorHoechstens Noten.
//   - BESTAETIGEN (POST /api/quanten-w/validator/bestaetigen, vom Praeger):
//     prueft Form, Groessen, Toleranz/k_note, seine Nummer, das Zeitfenster
//     und dass die Blaetter genau die Wurzel ergeben; erzeugt das Geheimnis,
//     legt alles ab und unterschreibt die Bestaetigung (EIP-191 seiner
//     Signieradresse) mit seinem Anker.
//   - ANTEILE HERAUSGEBEN (GET /api/quanten-w/validator/anteile): seine Anteile
//     und Pfade fuer einen Teil der gezogenen Stellen -- NUR wenn auf der Kette
//     die Anfrage mit dieser Nummer offen ist UND ihre Antwort schon steht.
//     Vorher wuerde der Halter die Geheimnisse der Aufforderung erfahren und
//     koennte faelschen (fail-closed).
//   - NICHTS VOR DER FINALITAET (reif): den Wert der Hash-Kette gibt der
//     Dienst erst heraus, wenn die ANFRAGE final ist, die Anteile erst, wenn
//     die ANTWORT final ist. "Die Antwort steht" im eigenen Zustand reicht
//     nicht: faellt ihr Block noch aus der Kette, haette der Anfragende die
//     Anteile gesehen und reichte danach die richtige Antwort ein -- jede
//     Faelschung ginge durch. Ebenso koennte er nach dem Zufall seine Anfrage
//     in anderer Groesse neu unterbringen und sich die Aufforderung
//     aussuchen. Final heisst: der Stand wurde bei DAG-Hoehe h gesehen,
//     steht unveraendert da, und der finalisierte Checkpoint hat h erreicht
//     (darunter ordnet kein Knoten mehr um, finality.go). Ohne Quelle fuer
//     Hoehe und Finalitaet wird nichts reif (fail-closed).
//   - HINTERGRUND (quantenWDienst.takt): eigenen Zufall einreichen, sobald eine
//     Anfrage auf ihn wartet; Teile der Offenlegung sammeln (eigene Anteile +
//     die der anderen Validatoren von den Nachbarn des Knotens und aus
//     QUANTEN_W_PEERS) und einreichen; abgelaufene Fristen einreichen; tote
//     Noten aus dem Tresor entfernen.
//
// Jeder Knoten prueft jede eingereichte Transaktion selbst (Pruefsummen,
// Hash-Ketten): der Dienst ist bequem, nicht vertraut. Ein falscher Anteil
// eines Nachbarn faellt an der Pruefsumme auf; dann wird eine andere Auswahl
// versucht (hoechstens quantenWSammelVersuche).
//
// NACHBARN SIND FREMDE. Die Liste kommt aus den Sync-Peers und Seeds des
// Knotens (QuantenWNachbarn) -- dorthin kommt jeder oeffentliche Knoten.
// Deshalb: nur oeffentliche Adressen, DNS einmal aufgeloest, keine
// Weiterleitung (fremdKlient, wie jeder Abruf an eine Adresse, die nicht der
// Betreiber gesetzt hat); alle gleichzeitig unter EINER Frist (ein Nachbar,
// der nicht antwortet, haelt den Takt nicht auf); und jede Ausgabe ist vom
// Validator unterschrieben, dessen Anteile sie traegt (quantenWAnteilEcht).
// Ohne die Unterschrift belegte ein Fremder, der schneller antwortet, mit
// erfundenen Anteilen jeden Platz, und keine Auswahl passte je.

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	quantenWTresorHoechstens = 64
	// Noch nicht gepraegte Bestaetigungen: hoechstens so viele insgesamt und
	// je Praeger (sonst belegt ein Fremder den Tresor -- Sicherheitspruefung W2d).
	quantenWOffeneHoechstens = 32
	quantenWOffeneJePraeger  = 4
	quantenWDomAuftrag       = "aequitas-quanten-w-auftrag-v2"
	// Bestaetigen: n bis 16384 Anteile (~34 Zeichen) und Blaetter (~66 Zeichen).
	quantenWTresorKoerperMax = 2 << 20
	quantenWDienstTakt       = 5 * time.Second
	quantenWPeerZeit         = 5 * time.Second
	// Sind schon t Validatoren beisammen, wird auf weitere (fuer andere
	// Auswahlen, falls einer falsche Anteile liefert) nur so lange gewartet.
	quantenWPeerNachlauf    = time.Second
	quantenWPeerAntwortMax  = 256 << 10
	quantenWPeersHoechstens = 32
	quantenWSammelVersuche  = 8
	// Staffel beim Offenlegen (Beobachtung aus #549): je Versuch EIN
	// zustaendiger Validator (quantenWZustaendig) reicht die Teile ohne Pause
	// nacheinander ein, hoechstens quantenWTeileJeTakt je Takt; die anderen
	// springen fuer einen Teil ein, der quantenWStaffelWarten lang offen
	// ist. Vorher reichten alle jeden Teil gleichzeitig ein (die meisten
	// abgewiesen, gegen die Burst-Grenze), und bei k_note = 5200 (82 Teile)
	// dauerte die Offenlegung mit einem Teil je Takt ~7 Minuten.
	quantenWTeileJeTakt     = 8
	quantenWStaffelWarten   = 2 * int64(quantenWDienstTakt/time.Second)
	burstQuantenWDienstJeIP = 30
	quantenWDomTresor       = "aequitas-quanten-w-tresor-v1|"
	quantenWDomAnteil       = "aequitas-quanten-w-anteil-v1|"
)

// quantenWTresorEintrag: was ein Validator zu einer Note haelt (Klartext nur
// im Speicher; abgelegt verschluesselt).
type quantenWTresorEintrag struct {
	K           string   `json:"k"`
	N           int      `json:"n"`
	T           int      `json:"t"`
	Tol         int      `json:"tol"`
	X           int      `json:"x"`
	Validatoren []string `json:"validatoren"`
	Wurzel      string   `json:"wurzel"`
	Zeit        int64    `json:"zeit"`
	Geheim      string   `json:"geheim"`
	Anteile     []string `json:"anteile"`  // hex, je Stelle der eigene Anteil
	Blaetter    []string `json:"blaetter"` // hex, Merkle-Blaetter
	Praeger     string   `json:"praeger"`
	// Variante S (quanten_w_s_kette.go): "s" und die zwei Stichproben-
	// Bindungen; ohne Hash-Kette (Geheim leer).
	Art    string   `json:"art,omitempty"`
	Proben []string `json:"proben,omitempty"`
}

// quantenWAblage: wohin der Tresor schreibt (Datenbank oder Speicher).
type quantenWAblage interface {
	alle() (map[string][]byte, error)
	setzen(k string, b []byte) error
	loeschen(k string) error
}

type quantenWSpeicherAblage struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (s *quantenWSpeicherAblage) alle() (map[string][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string][]byte, len(s.m))
	for k, v := range s.m {
		out[k] = append([]byte(nil), v...)
	}
	return out, nil
}
func (s *quantenWSpeicherAblage) setzen(k string, b []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string][]byte{}
	}
	s.m[k] = append([]byte(nil), b...)
	return nil
}
func (s *quantenWSpeicherAblage) loeschen(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, k)
	return nil
}

type quantenWDBAblage struct{ db *sql.DB }

func (d quantenWDBAblage) alle() (map[string][]byte, error) {
	rows, err := d.db.Query(`SELECT k, daten FROM quanten_w_tresor`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]byte{}
	for rows.Next() {
		var k string
		var b []byte
		if err := rows.Scan(&k, &b); err != nil {
			return nil, err
		}
		out[k] = b
	}
	return out, rows.Err()
}
func (d quantenWDBAblage) setzen(k string, b []byte) error {
	_, err := d.db.Exec(`INSERT INTO quanten_w_tresor (k, daten) VALUES ($1, $2) ON CONFLICT (k) DO UPDATE SET daten = EXCLUDED.daten`, k, b)
	return err
}
func (d quantenWDBAblage) loeschen(k string) error {
	_, err := d.db.Exec(`DELETE FROM quanten_w_tresor WHERE k = $1`, k)
	return err
}

// quantenWDienst: Tresor und Hintergrundarbeit eines Validators.
type quantenWDienst struct {
	mu        sync.Mutex
	cs        *ChainState
	key       *ecdsa.PrivateKey
	adresse   string
	aead      cipher.AEAD
	ablage    quantenWAblage
	eintraege map[string]*quantenWTresorEintrag
	// peers: vom Betreiber gesetzt (QUANTEN_W_PEERS, auch private Netze),
	// abgerufen ueber client. nachbarn: die Nachbarn des Knotens -- nur
	// Adressen, die erlaubt zulaesst, abgerufen ueber fremd.
	// peerZeit/nachlauf: Fristen eines Abrufs (Tests verkuerzen sie).
	peers    []string
	client   *http.Client
	nachbarn func() []string
	erlaubt  func(string) bool
	fremd    *http.Client
	peerZeit time.Duration
	nachlauf time.Duration
	// einreichen: eine W-Transaktion an die Kette (im Betrieb ueber die
	// eigene API zum Leiter; in Tests direkt).
	einreichen func(art string, q QuantenWAngaben) error
	jetzt      func() int64
	// hoehe: DAG-Hoehe dieses Knotens; final: Hoehe des finalisierten
	// Checkpoints (StarteQuantenWDienst setzt beide). sicht: bei welcher
	// Hoehe ein Stand zum ersten Mal gesehen wurde (reif); durch den Tresor
	// begrenzt, der Takt vergisst, was es nicht mehr gibt.
	hoehe func() int64
	final func() int64
	sicht map[string]int64
	// faellig: seit wann ein Teil offen ist, den ein anderer zuerst
	// einreichen soll (Staffel); wie sicht begrenzt und aufgeraeumt.
	faellig map[string]int64
}

// quantenWZustaendig: welcher Validator (1-basiert) die Teile eines Versuchs
// zuerst einreicht -- verteilt ueber K und den Versuch.
func quantenWZustaendig(K string, index, v int) int {
	if v <= 0 {
		return 0
	}
	b, _ := hex.DecodeString(K[:2])
	start := 0
	if len(b) == 1 {
		start = int(b[0])
	}
	return (start+index)%v + 1
}

// darfEinreichen: der Zustaendige sofort; jeder andere erst, wenn der Teil
// seit quantenWStaffelWarten offen ist (der Zustaendige faellt aus).
func (d *quantenWDienst) darfEinreichen(K string, e *quantenWTresorEintrag, index, ab int, jetzt int64, gesehen map[string]bool) bool {
	if quantenWZustaendig(K, index, len(e.Validatoren)) == e.X {
		return true
	}
	key := fmt.Sprintf("f|%s|%d|%d", K, index, ab)
	gesehen[key] = true
	d.mu.Lock()
	defer d.mu.Unlock()
	t0, da := d.faellig[key]
	if !da {
		if len(d.faellig) < quantenWSichtHoechstens {
			d.faellig[key] = jetzt
		}
		return false
	}
	return jetzt-t0 >= quantenWStaffelWarten
}

// quantenWSichtHoechstens: gemerkte Sichtungen -- je Note hoechstens eine
// Anfrage und eine Antwort. Voll = nichts Neues wird reif (fail-closed).
const quantenWSichtHoechstens = 4 * quantenWTresorHoechstens

// reif: ist der Stand unter diesem Schluessel final? Die erste Frage merkt
// die DAG-Hoehe; reif ist er, sobald der finalisierte Checkpoint sie
// erreicht hat.
func (d *quantenWDienst) reif(schluessel string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	h, bekannt := d.sicht[schluessel]
	if !bekannt {
		if len(d.sicht) >= quantenWSichtHoechstens {
			return false
		}
		h = d.hoehe()
		d.sicht[schluessel] = h
	}
	return d.final() >= h
}

// sichtRaeumen: Sichtungen, deren Stand es nicht mehr gibt, vergessen.
func (d *quantenWDienst) sichtRaeumen(gesehen map[string]bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for s := range d.sicht {
		if !gesehen[s] {
			delete(d.sicht, s)
		}
	}
	for s := range d.faellig {
		if !gesehen[s] {
			delete(d.faellig, s)
		}
	}
}

// quantenWAnfrageSicht, quantenWAntwortSicht: alles, was die Aufforderung
// bzw. das Urteil bestimmt -- eine ANDERE Anfrage oder Antwort an derselben
// Stelle ist ein anderer Stand und wartet neu.
func quantenWAnfrageSicht(K string, a *quantenWAnfrage) string {
	return fmt.Sprintf("z|%s|%d|%s|%d|%d|%d|%s", K, a.Index, a.Von, a.K, a.Zeit, a.Pfand, a.EinloesenAn)
}

func quantenWAntwortSicht(K string, a *quantenWAnfrage) string {
	return fmt.Sprintf("o|%s|%d|%s|%d|%v|%v|%v|%d", K, a.Index, a.Von, a.Gezogen, a.Stellen, a.Basen, a.Antwort, a.AntwortZeit)
}

func neuerQuantenWDienst(cs *ChainState, key *ecdsa.PrivateKey, ablage quantenWAblage, peers []string,
	einreichen func(string, QuantenWAngaben) error) (*quantenWDienst, error) {
	if key == nil {
		return nil, errors.New("quanten_w: kein Signierschluessel")
	}
	// Schluessel des Tresors aus dem Signierschluessel (nie ausgegeben).
	sk := sha256.Sum256(append([]byte(quantenWDomTresor), crypto.FromECDSA(key)...))
	blk, err := aes.NewCipher(sk[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(blk)
	if err != nil {
		return nil, err
	}
	if len(peers) > quantenWPeersHoechstens {
		peers = peers[:quantenWPeersHoechstens]
	}
	d := &quantenWDienst{cs: cs, key: key, adresse: strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex()),
		aead: aead, ablage: ablage, eintraege: map[string]*quantenWTresorEintrag{}, peers: peers,
		client: quantenWPeerKlient(nil), erlaubt: isAllowedPeerURL, fremd: quantenWPeerKlient(pinningDialer),
		peerZeit: quantenWPeerZeit, nachlauf: quantenWPeerNachlauf, einreichen: einreichen, jetzt: nowUnix,
		hoehe: func() int64 { return 0 }, final: func() int64 { return -1 }, sicht: map[string]int64{}, faellig: map[string]int64{}}
	roh, err := ablage.alle()
	if err != nil {
		return nil, fmt.Errorf("quanten_w: Tresor nicht lesbar: %w", err)
	}
	for k, b := range roh {
		e, err := d.oeffnen(k, b)
		if err != nil {
			return nil, fmt.Errorf("quanten_w: Tresor-Eintrag %s: %w", k[:min(12, len(k))], err) // fail-closed: kein Start mit kaputtem Tresor
		}
		d.eintraege[k] = e
	}
	return d, nil
}

func (d *quantenWDienst) versiegeln(e *quantenWTresorEintrag) ([]byte, error) {
	klar, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, d.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return d.aead.Seal(nonce, nonce, klar, []byte(e.K)), nil
}

func (d *quantenWDienst) oeffnen(k string, b []byte) (*quantenWTresorEintrag, error) {
	ns := d.aead.NonceSize()
	if len(b) < ns {
		return nil, errors.New("zu kurz")
	}
	klar, err := d.aead.Open(nil, b[:ns], b[ns:], []byte(k))
	if err != nil {
		return nil, errors.New("Siegel passt nicht")
	}
	var e quantenWTresorEintrag
	if err := json.Unmarshal(klar, &e); err != nil || e.K != k {
		return nil, errors.New("Inhalt passt nicht")
	}
	return &e, nil
}

// QuantenWBestaetigenAnfrage: was der Praeger einem Validator schickt.
type QuantenWBestaetigenAnfrage struct {
	K           string   `json:"k"`
	N           int      `json:"n"`
	T           int      `json:"t"`
	Tol         int      `json:"tol"`
	X           int      `json:"x"`
	Wurzel      string   `json:"wurzel"`
	Validatoren []string `json:"validatoren"`
	Zeit        int64    `json:"zeit"`
	Anteile     []string `json:"anteile"`
	Blaetter    []string `json:"blaetter"`
	// Der Praeger unterschreibt den Auftrag (EIP-191, quantenWAuftragNachricht):
	// nur ein Mensch mit Deckung belegt Platz im Tresor.
	Praeger    string `json:"praeger"`
	PraegerSig string `json:"praeger_sig"`
	// Variante S: Art "s" und die zwei Stichproben-Bindungen (hex); der
	// Auftrag ist dann quantenWSAuftragNachricht, die Bestaetigung die der
	// S-Ausgabe (quantenWSBestaetigungNachricht), ohne Anker.
	Art    string   `json:"art,omitempty"`
	Proben []string `json:"proben,omitempty"`
}

const quantenWSDomAuftrag = "aequitas-quanten-s-auftrag-v1"

// quantenWSAuftragNachricht: der Auftrag des Ausgebers eines S-Tokens an
// einen Validator -- wie bei W mit dem Hash seiner Anteile, dazu die
// Stichproben-Bindungen.
func quantenWSAuftragNachricht(praeger, K string, n, t, tol, x int, wurzel string, validatoren, proben []string, zeit int64, anteileHash string) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%d|%d|%d|%s|%s|%s|%d|%s", quantenWSDomAuftrag, eingefroreneChainID,
		strings.ToLower(praeger), K, n, t, tol, x, wurzel, strings.Join(validatoren, ","), strings.Join(proben, ","), zeit, anteileHash)
}

// quantenWAuftragNachricht: was der Praeger dem Validator unterschreibt --
// seit v2 auch den Hash SEINER Anteile (quantenWAnteileHash). Sonst koennte
// wer den Auftrag auf dem Weg abfaengt, ihn mit anderen Anteilen zuerst
// einreichen: der Validator bestaetigte die Note, haette aber Anteile, die
// nicht zur Wurzel passen -- die Note waere unpruefbar und K belegt.
func quantenWAuftragNachricht(praeger, K string, n, t, tol, x int, wurzel string, validatoren []string, zeit int64, anteileHash string) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%d|%d|%d|%s|%s|%d|%s", quantenWDomAuftrag, eingefroreneChainID,
		strings.ToLower(praeger), K, n, t, tol, x, wurzel, strings.Join(validatoren, ","), zeit, anteileHash)
}

// quantenWAnteileHash: SHA-256 ueber die Anteile eines Validators (hex, in
// Reihenfolge der Stellen, durch Komma getrennt).
func quantenWAnteileHash(anteile []string) string {
	h := sha256.Sum256([]byte(strings.Join(anteile, ",")))
	return hex.EncodeToString(h[:])
}

// bestaetigen: pruefen, ablegen, unterschreiben. Gibt Anker und Bestaetigung.
func (d *quantenWDienst) bestaetigen(q QuantenWBestaetigenAnfrage) (anker string, sig string, err error) {
	q.K = strings.ToLower(strings.TrimSpace(q.K))
	if !istHash64(q.K) {
		return "", "", quantenWFehler("K")
	}
	reg := d.cs.quantenWJetzt()
	if reg.Noten[q.K] != nil || reg.Token[q.K] != nil {
		return "", "", quantenWFehler("Note schon gepraegt")
	}
	if _, tot := reg.Tote[q.K]; tot {
		return "", "", quantenWFehler("Note war schon gepraegt")
	}
	gesehenV := map[string]bool{}
	for _, v := range q.Validatoren {
		if !kanonischeAdresse(v) || gesehenV[v] {
			return "", "", quantenWFehler("Validator nicht kanonisch oder doppelt")
		}
		gesehenV[v] = true
	}
	var proben [2][32]byte
	switch q.Art {
	case "":
		if q.Proben != nil {
			return "", "", quantenWFehler("Stichproben nur bei Variante S")
		}
		if _, err := quantenWPraegenPruefen(false, q.K, q.N, q.T, q.Validatoren, len(q.Validatoren), len(q.Validatoren), q.Tol); err != nil {
			return "", "", err
		}
	case "s":
		ok := len(q.Proben) == 2
		for i := 0; ok && i < 2; i++ {
			proben[i], ok = quantenWHex32(q.Proben[i])
		}
		if !ok {
			return "", "", quantenWFehler("S: zwei Stichproben-Bindungen")
		}
		if _, err := neuerQuantenSToken(q.N, d.adresse, [32]byte{}, 1, proben, q.T, len(q.Validatoren), q.Tol); err != nil {
			return "", "", err
		}
	default:
		return "", "", quantenWFehler("Art %q", q.Art)
	}
	if q.X < 1 || q.X > len(q.Validatoren) || q.Validatoren[q.X-1] != d.adresse {
		return "", "", quantenWFehler("Nummer %d gehoert nicht zu diesem Validator", q.X)
	}
	// Der Auftrag: unterschrieben von einem Menschen mit mindestens der
	// kleinsten Deckung (sonst belegt ein Fremder kostenlos den Tresor).
	q.Praeger = strings.ToLower(strings.TrimSpace(q.Praeger))
	if !kanonischeAdresse(q.Praeger) || !kanonischeSignatur(q.PraegerSig) ||
		verifyPersonalSign(quantenWDienstAuftrag(q), q.PraegerSig, q.Praeger) != nil {
		return "", "", quantenWFehler("Auftrag nicht vom Praeger unterschrieben")
	}
	if !d.praegerTauglich(q.Praeger) {
		return "", "", quantenWFehler("Praeger ist kein Mensch mit Deckung")
	}
	jetzt := d.jetzt()
	if q.Zeit < jetzt-quantenWAlt || q.Zeit > jetzt+quantenWVoraus {
		return "", "", quantenWFehler("Praegezeit ausserhalb des Fensters")
	}
	if len(q.Anteile) != q.N || len(q.Blaetter) != q.N {
		return "", "", quantenWFehler("Zahl der Anteile oder Blaetter")
	}
	wurzel, ok := quantenWHex32(q.Wurzel)
	if !ok {
		return "", "", quantenWFehler("Wurzel")
	}
	blaetter := make([][32]byte, q.N)
	for i, h := range q.Blaetter {
		b, ok := quantenWHex32(h)
		if !ok {
			return "", "", quantenWFehler("Blatt %d", i)
		}
		blaetter[i] = b
	}
	for i, y := range q.Anteile {
		if len(y) == 0 || len(y) > 32 || !kleinHex(y) {
			return "", "", quantenWFehler("Anteil %d", i)
		}
		v, ok := new(big.Int).SetString(y, 16)
		if !ok || v.Cmp(quantenWP) >= 0 {
			return "", "", quantenWFehler("Anteil %d", i)
		}
	}
	if quantenWMerkleWurzel(blaetter) != wurzel {
		return "", "", quantenWFehler("Blaetter ergeben nicht die Wurzel")
	}
	// Derselbe Auftrag noch einmal (die Antwort ging verloren): dieselbe
	// Bestaetigung mit demselben Anker -- sonst braeuchte der Praeger ein
	// neues K. Ein ANDERER Auftrag fuer dasselbe K bleibt abgewiesen.
	if vorher, da := d.eintragVon(q.K); da {
		if !quantenWGleicherAuftrag(vorher, q) {
			return "", "", quantenWFehler("schon bestaetigt (anderer Auftrag)")
		}
		if q.Art == "s" {
			return d.bestaetigungSUnterschreiben(q, wurzel, proben)
		}
		g, ok := quantenWHex32(vorher.Geheim)
		if !ok {
			return "", "", quantenWFehler("Tresor: Geheimnis unlesbar")
		}
		return d.bestaetigungUnterschreiben(q, wurzel, quantenWKetteVor(g, quantenWZufallL))
	}
	var geheim [32]byte
	var a [32]byte
	e := &quantenWTresorEintrag{K: q.K, N: q.N, T: q.T, Tol: q.Tol, X: q.X, Validatoren: append([]string(nil), q.Validatoren...),
		Wurzel: q.Wurzel, Zeit: q.Zeit, Anteile: append([]string(nil), q.Anteile...),
		Blaetter: append([]string(nil), q.Blaetter...), Praeger: q.Praeger, Art: q.Art, Proben: append([]string(nil), q.Proben...)}
	if q.Art == "" {
		if _, err := rand.Read(geheim[:]); err != nil {
			return "", "", err
		}
		a = quantenWKetteVor(geheim, quantenWZufallL)
		e.Geheim, e.Proben = hex.EncodeToString(geheim[:]), nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, da := d.eintraege[q.K]; da {
		return "", "", quantenWFehler("schon bestaetigt")
	}
	if len(d.eintraege) >= quantenWTresorHoechstens {
		return "", "", quantenWFehler("Tresor voll")
	}
	offen, jePraeger := 0, 0
	for k, x := range d.eintraege {
		if reg.Noten[k] == nil && reg.Token[k] == nil {
			offen++
			if x.Praeger == q.Praeger {
				jePraeger++
			}
		}
	}
	if offen >= quantenWOffeneHoechstens || jePraeger >= quantenWOffeneJePraeger {
		return "", "", quantenWFehler("zu viele noch nicht gepraegte Bestaetigungen")
	}
	roh, err := d.versiegeln(e)
	if err != nil {
		return "", "", err
	}
	if err := d.ablage.setzen(q.K, roh); err != nil {
		return "", "", err
	}
	d.eintraege[q.K] = e
	if q.Art == "s" {
		return d.bestaetigungSUnterschreiben(q, wurzel, proben)
	}
	return d.bestaetigungUnterschreiben(q, wurzel, a)
}

// quantenWDienstAuftrag: was der Praeger bzw. Ausgeber unterschrieben hat.
func quantenWDienstAuftrag(q QuantenWBestaetigenAnfrage) string {
	if q.Art == "s" {
		return quantenWSAuftragNachricht(q.Praeger, q.K, q.N, q.T, q.Tol, q.X, q.Wurzel, q.Validatoren, q.Proben, q.Zeit, quantenWAnteileHash(q.Anteile))
	}
	return quantenWAuftragNachricht(q.Praeger, q.K, q.N, q.T, q.Tol, q.X, q.Wurzel, q.Validatoren, q.Zeit, quantenWAnteileHash(q.Anteile))
}

// bestaetigungSUnterschreiben: die Bestaetigung einer S-Ausgabe (kein Anker).
func (d *quantenWDienst) bestaetigungSUnterschreiben(q QuantenWBestaetigenAnfrage, wurzel [32]byte, proben [2][32]byte) (string, string, error) {
	msg := quantenWSBestaetigungNachricht(q.K, wurzel, q.N, q.T, q.Validatoren, q.X, proben, q.Zeit, q.Tol)
	s, err := crypto.Sign(accounts.TextHash([]byte(msg)), d.key)
	if err != nil {
		return "", "", err
	}
	s[64] += 27
	return "", "0x" + hex.EncodeToString(s), nil
}

func (d *quantenWDienst) bestaetigungUnterschreiben(q QuantenWBestaetigenAnfrage, wurzel, a [32]byte) (string, string, error) {
	msg := quantenWBestaetigungNachricht(q.K, wurzel, q.N, q.T, q.Validatoren, q.X, a, q.Zeit, q.Tol)
	s, err := crypto.Sign(accounts.TextHash([]byte(msg)), d.key)
	if err != nil {
		return "", "", err
	}
	s[64] += 27
	return hex.EncodeToString(a[:]), "0x" + hex.EncodeToString(s), nil
}

// eintragVon: der Tresor-Eintrag zu K (unter der Sperre, mit defer).
func (d *quantenWDienst) eintragVon(k string) (*quantenWTresorEintrag, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	e, da := d.eintraege[k]
	return e, da
}

// quantenWGleicherAuftrag: dieselbe Note, derselbe Praeger, dieselben
// Anteile und Blaetter -- dann darf die Bestaetigung wiederholt werden.
func quantenWGleicherAuftrag(e *quantenWTresorEintrag, q QuantenWBestaetigenAnfrage) bool {
	return e.K == q.K && e.N == q.N && e.T == q.T && e.Tol == q.Tol && e.X == q.X && e.Wurzel == q.Wurzel && e.Zeit == q.Zeit &&
		e.Praeger == q.Praeger && slices.Equal(e.Validatoren, q.Validatoren) && slices.Equal(e.Anteile, q.Anteile) && slices.Equal(e.Blaetter, q.Blaetter) &&
		e.Art == q.Art && slices.Equal(e.Proben, q.Proben)
}

// praegerTauglich: mindestens die kleinste Deckung, und entweder selbst ein
// Mensch oder die Signieradresse eines Validators, die im Validator-Register
// an einen Menschen gebunden ist (so gibt C1 mit dem eigenen Schluessel aus,
// quanten_s_ausgabe.go). Das Register erlaubt je Mensch EINE Signieradresse:
// ein Mensch belegt damit hoechstens doppelt so viele offene Plaetze
// (quantenWOffeneJePraeger), die Gesamtgrenze quantenWOffeneHoechstens bleibt.
// Register nicht lesbar = nicht tauglich (fail-closed).
// var: Tests ohne Datenbank setzen ein.
var quantenWPraegerTauglich = func(cs *ChainState, wallet string) bool {
	ctx := context.Background()
	cs.ensureAccountLoadedCtx(ctx, wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok || acc.Balance.Micro() < quantenMindestDeckungMik {
		return false
	}
	if acc.IsHuman {
		return true
	}
	b, err := quantenWBetreiberVon(cs, ctx, wallet)
	b = strings.ToLower(b)
	if err != nil || b == "" {
		return false
	}
	cs.ensureAccountLoadedCtx(ctx, b)
	mensch, ok := cs.accounts.Get(b)
	return ok && mensch.IsHuman
}

func (d *quantenWDienst) praegerTauglich(wallet string) bool {
	return quantenWPraegerTauglich(d.cs, wallet)
}

// QuantenWAnteilAusgabe: die Anteile EINES Validators fuer einen Teil.
type QuantenWAnteilAusgabe struct {
	K       string     `json:"k"`
	Index   int        `json:"index"`
	Ab      int        `json:"ab"`
	X       int        `json:"x"`
	Stellen []int      `json:"stellen"`
	Y       []string   `json:"y"`
	Pfade   [][]string `json:"pfade"`
	// Sig: Unterschrift des Validators X ueber alles andere
	// (quantenWAnteilNachricht) -- ein Nachbar, der nicht Validator X ist,
	// kann keine Anteile in seinem Namen liefern.
	Sig string `json:"sig,omitempty"`
}

// quantenWAnteilNachricht: was der Validator zu seiner Ausgabe unterschreibt
// (JSON ohne Sig: eindeutig, jede Laenge und Grenze steht darin).
func quantenWAnteilNachricht(a QuantenWAnteilAusgabe) string {
	a.Sig = ""
	b, _ := json.Marshal(a)
	h := sha256.Sum256(b)
	return quantenWDomAnteil + hex.EncodeToString(h[:])
}

// anteilUnterschreiben: die eigene Ausgabe unterschreiben.
func (d *quantenWDienst) anteilUnterschreiben(a *QuantenWAnteilAusgabe) error {
	s, err := crypto.Sign(accounts.TextHash([]byte(quantenWAnteilNachricht(*a))), d.key)
	if err != nil {
		return err
	}
	s[64] += 27
	a.Sig = "0x" + hex.EncodeToString(s)
	return nil
}

// quantenWAnteilEcht: die Ausgabe stammt vom Validator X der Note.
func quantenWAnteilEcht(e *quantenWTresorEintrag, a QuantenWAnteilAusgabe) bool {
	if a.X < 1 || a.X > len(e.Validatoren) || a.Sig == "" {
		return false
	}
	return verifyPersonalSign(quantenWAnteilNachricht(a), a.Sig, e.Validatoren[a.X-1]) == nil
}

// anteile: eigene Anteile fuer a.Stellen[ab : ab+quantenWOffenTeil] -- nur
// wenn die Anfrage index offen ist und ihre Antwort steht (fail-closed).
// eintrag: der Eintrag zu K (nil = keiner); Eintraege werden nach dem
// Anlegen nicht veraendert, nur ersetzt oder entfernt.
func (d *quantenWDienst) eintrag(K string) *quantenWTresorEintrag {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.eintraege[K]
}

// schluessel: die K im Tresor, sortiert.
func (d *quantenWDienst) schluessel() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	ks := make([]string, 0, len(d.eintraege))
	for k := range d.eintraege {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func (d *quantenWDienst) anteile(K string, index, ab int) (QuantenWAnteilAusgabe, error) {
	e := d.eintrag(K)
	if e == nil {
		return QuantenWAnteilAusgabe{}, quantenWFehler("Note nicht im Tresor")
	}
	if tok := d.cs.quantenWJetzt().Token[K]; tok != nil {
		return d.anteileS(K, e, tok.token, index, ab)
	}
	ein := d.cs.quantenWJetzt().Noten[K]
	if ein == nil || ein.note.Anfrage == nil || ein.note.Anfrage.Index != index {
		return QuantenWAnteilAusgabe{}, quantenWFehler("keine offene Anfrage %d", index)
	}
	a := ein.note.Anfrage
	if a.Antwort == nil || a.Basen == nil {
		return QuantenWAnteilAusgabe{}, quantenWFehler("die Antwort steht noch aus -- vorher kein Anteil")
	}
	if !d.reif(quantenWAntwortSicht(K, a)) {
		return QuantenWAnteilAusgabe{}, quantenWFehler("die Antwort ist noch nicht final -- vorher kein Anteil")
	}
	return d.anteilAusgabe(K, e, index, ab, a.Stellen)
}

// anteileS: eigene Anteile fuer die Stichprobe einer S-Anmeldung, Teil ab --
// nur wenn die Anmeldung final ist (sonst laege die Stichprobe nach einem
// Rueckroll offen, ohne dass eingeloest wurde). index = Zahl der bisherigen
// Fehlversuche des Tokens (benennt den Versuch).
func (d *quantenWDienst) anteileS(K string, e *quantenWTresorEintrag, t *quantenSToken, index, ab int) (QuantenWAnteilAusgabe, error) {
	if e.Art != "s" {
		return QuantenWAnteilAusgabe{}, quantenWFehler("kein S-Token im Tresor")
	}
	if t.Anmeldung == nil || index != t.Fehl {
		return QuantenWAnteilAusgabe{}, quantenWFehler("keine offene Anmeldung %d", index)
	}
	if !d.reif(quantenWSAnmeldungSicht(K, t)) {
		return QuantenWAnteilAusgabe{}, quantenWFehler("die Anmeldung ist noch nicht final -- vorher kein Anteil")
	}
	return d.anteilAusgabe(K, e, index, ab, t.Anmeldung.Probe)
}

// quantenWSAnmeldungSicht: alles, was die Offenlegung einer S-Anmeldung
// bestimmt (als Hash: die Stichprobe kann lang sein).
func quantenWSAnmeldungSicht(K string, t *quantenSToken) string {
	a := t.Anmeldung
	h := sha256.Sum256([]byte(fmt.Sprintf("%d|%d|%s|%d|%v|%s", t.Fehl, a.B, a.Stelle, a.Zeit, a.Probe, quantenWBitsHex(a.X))))
	return "s|" + K + "|" + hex.EncodeToString(h[:])
}

// anteilAusgabe: eigene Anteile und Merkle-Pfade fuer stellen[ab : ab+Teil].
func (d *quantenWDienst) anteilAusgabe(K string, e *quantenWTresorEintrag, index, ab int, stellen []int) (QuantenWAnteilAusgabe, error) {
	if ab < 0 || ab >= len(stellen) {
		return QuantenWAnteilAusgabe{}, quantenWFehler("ab %d", ab)
	}
	bis := min(ab+quantenWOffenTeil, len(stellen))
	blaetter := make([][32]byte, e.N)
	for i, h := range e.Blaetter {
		blaetter[i], _ = quantenWHex32(h)
	}
	// Den Baum EINMAL bauen (nicht je Stelle: bei n = 16384 waeren das
	// sonst ~2 Mio. Hashes je Abruf -- Sicherheitspruefung W2d).
	ebenen := quantenWMerkleEbenen(blaetter)
	out := QuantenWAnteilAusgabe{K: K, Index: index, Ab: ab, X: e.X}
	for _, i := range stellen[ab:bis] {
		if i < 0 || i >= e.N {
			return QuantenWAnteilAusgabe{}, quantenWFehler("Stelle %d", i)
		}
		out.Stellen = append(out.Stellen, i)
		out.Y = append(out.Y, e.Anteile[i])
		var p []string
		for _, h := range quantenWPfadAusEbenen(ebenen, i) {
			p = append(p, hex.EncodeToString(h[:]))
		}
		out.Pfade = append(out.Pfade, p)
	}
	return out, nil
}

// quantenWMerkleEbenen: alle Ebenen des Baums (ungerade Ebenen verdoppeln
// das letzte Blatt, wie quantenWMerkleWurzel).
func quantenWMerkleEbenen(blaetter [][32]byte) [][][32]byte {
	ebene := append([][32]byte(nil), blaetter...)
	var out [][][32]byte
	for len(ebene) > 1 {
		if len(ebene)%2 == 1 {
			ebene = append(ebene, ebene[len(ebene)-1])
		}
		out = append(out, ebene)
		naechste := make([][32]byte, 0, len(ebene)/2)
		for j := 0; j < len(ebene); j += 2 {
			naechste = append(naechste, quantenWKnoten(ebene[j], ebene[j+1]))
		}
		ebene = naechste
	}
	return out
}

func quantenWPfadAusEbenen(ebenen [][][32]byte, i int) [][32]byte {
	var pfad [][32]byte
	for _, e := range ebenen {
		pfad = append(pfad, e[i^1])
		i /= 2
	}
	return pfad
}

// quantenWSammeln: aus den Ausgaben mehrerer Validatoren (gleiche Stellen)
// je Stelle t Anteile verschiedener x -- die Auswahl wahl (Nummern der
// Ausgaben). nil, wenn die Auswahl nicht passt.
func quantenWSammeln(aus []QuantenWAnteilAusgabe, wahl []int, t int) []QuantenWOffen {
	if len(wahl) != t || t == 0 {
		return nil
	}
	erste := aus[wahl[0]]
	gesehen := map[int]bool{}
	for _, w := range wahl {
		a := aus[w]
		if gesehen[a.X] || len(a.Stellen) != len(erste.Stellen) || len(a.Y) != len(a.Stellen) || len(a.Pfade) != len(a.Stellen) {
			return nil
		}
		for j := range a.Stellen {
			if a.Stellen[j] != erste.Stellen[j] {
				return nil
			}
		}
		gesehen[a.X] = true
	}
	var out []QuantenWOffen
	for j := range erste.Stellen {
		o := QuantenWOffen{Pfad: erste.Pfade[j]}
		for _, w := range wahl {
			o.Anteile = append(o.Anteile, QuantenWAnteilAngabe{X: aus[w].X, Y: aus[w].Y[j]})
		}
		out = append(out, o)
	}
	return out
}

// quantenWAuswahlen: alle t-Auswahlen aus m, die die eigene (Nummer 0)
// enthalten, in fester Reihenfolge -- hoechstens grenze.
func quantenWAuswahlen(m, t, grenze int) [][]int {
	var out [][]int
	var rek func(start int, cur []int)
	rek = func(start int, cur []int) {
		if len(out) >= grenze {
			return
		}
		if len(cur) == t {
			out = append(out, append([]int(nil), cur...))
			return
		}
		for i := start; i < m; i++ {
			rek(i+1, append(cur, i))
		}
	}
	if t >= 1 && m >= t {
		rek(1, []int{0})
	}
	return out
}

// quantenWPeerKlient: ein Klient fuer Nachbarn -- folgt keiner Weiterleitung
// (eine erlaubte Adresse duerfte sonst ins interne Netz umleiten). dial nil:
// der gewoehnliche Dialer (nur fuer die Adressen des Betreibers).
func quantenWPeerKlient(dial func(context.Context, string, string) (net.Conn, error)) *http.Client {
	c := &http.Client{Timeout: quantenWPeerZeit, CheckRedirect: ohneUmleitung}
	if dial != nil {
		c.Transport = &http.Transport{DialContext: dial}
	}
	return c
}

// quantenWPeer: eine Basis-URL; fremd = ein Nachbar des Knotens (nicht vom
// Betreiber gesetzt).
type quantenWPeer struct {
	url   string
	fremd bool
}

// peerListe: erst die Adressen des Betreibers, dann die Nachbarn des Knotens,
// soweit erlaubt -- ohne Doppelte, hoechstens quantenWPeersHoechstens.
func (d *quantenWDienst) peerListe() []quantenWPeer {
	var out []quantenWPeer
	schon := map[string]bool{}
	dazu := func(u string, fremd bool) {
		u = strings.TrimRight(strings.TrimSpace(u), "/")
		if u == "" || schon[u] || (fremd && !d.erlaubt(u)) {
			return
		}
		schon[u] = true
		out = append(out, quantenWPeer{url: u, fremd: fremd})
	}
	for _, p := range d.peers {
		if len(out) >= quantenWPeersHoechstens {
			return out
		}
		dazu(p, false)
	}
	if d.nachbarn != nil {
		for _, p := range d.nachbarn() {
			if len(out) >= quantenWPeersHoechstens {
				return out
			}
			dazu(p, true)
		}
	}
	return out
}

// nachbarAnteile: die Ausgaben der anderen Validatoren -- alle Peers
// gleichzeitig unter einer gemeinsamen Frist (peerZeit). Es zaehlt nur eine
// vom Validator X der Note unterschriebene Ausgabe fuer genau die Stellen
// dieses Teils (wie die eigene: eine alte, echt unterschriebene Ausgabe mit
// anderer Stichprobe -- S nach einer Frist, gleiche Nummer -- belegte sonst
// den Platz von X), je Validator die erste.
// Schluss, sobald alle anderen Validatoren da sind, alle Peers geantwortet
// haben oder die Frist ablaeuft; sind schon t beisammen, wird nur noch
// nachlauf gewartet. Hoechstens quantenWPeersHoechstens Abrufe; keiner
// ueberlebt die Rueckkehr lange (die Frist wird abgebrochen, der Kanal ist
// gepuffert, kein Sender bleibt haengen).
func (d *quantenWDienst) nachbarAnteile(ctx context.Context, K string, e *quantenWTresorEintrag, eigen QuantenWAnteilAusgabe) []QuantenWAnteilAusgabe {
	peers := d.peerListe()
	if len(peers) == 0 {
		return nil
	}
	ctx, abbrechen := context.WithTimeout(ctx, d.peerZeit)
	defer abbrechen()
	antworten := make(chan QuantenWAnteilAusgabe, len(peers))
	for _, p := range peers {
		c := d.client
		if p.fremd {
			c = d.fremd
		}
		go func(c *http.Client, basis string) {
			a, err := d.peerAnteile(ctx, c, basis, K, eigen.Index, eigen.Ab)
			if err != nil || !slices.Equal(a.Stellen, eigen.Stellen) || !quantenWAnteilEcht(e, a) {
				a = QuantenWAnteilAusgabe{} // X = 0: nichts Brauchbares
			}
			antworten <- a
		}(c, p.url)
	}
	var out []QuantenWAnteilAusgabe
	da := map[int]bool{eigen.X: true}
	var nachlauf <-chan time.Time
	var uhr *time.Timer
	defer func() {
		if uhr != nil {
			uhr.Stop()
		}
	}()
	for offen := len(peers); offen > 0 && len(da) < len(e.Validatoren); {
		select {
		case <-ctx.Done():
			return out
		case <-nachlauf:
			return out
		case a := <-antworten:
			offen--
			if a.X == 0 || da[a.X] {
				continue
			}
			da[a.X] = true
			out = append(out, a)
			if uhr == nil && len(da) >= e.T {
				uhr = time.NewTimer(d.nachlauf)
				nachlauf = uhr.C
			}
		}
	}
	return out
}

// peerAnteile: die Ausgabe eines Nachbarn (Grenzen: Zeit, Groesse, Form).
func (d *quantenWDienst) peerAnteile(ctx context.Context, c *http.Client, basis, K string, index, ab int) (QuantenWAnteilAusgabe, error) {
	u := strings.TrimRight(basis, "/") + "/api/quanten-w/validator/anteile?" + url.Values{
		"k": {K}, "index": {strconv.Itoa(index)}, "ab": {strconv.Itoa(ab)}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return QuantenWAnteilAusgabe{}, err
	}
	res, err := c.Do(req)
	if err != nil {
		return QuantenWAnteilAusgabe{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return QuantenWAnteilAusgabe{}, fmt.Errorf("Status %d", res.StatusCode)
	}
	var a QuantenWAnteilAusgabe
	dec := json.NewDecoder(io.LimitReader(res.Body, quantenWPeerAntwortMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return QuantenWAnteilAusgabe{}, err
	}
	if a.K != K || a.Index != index || a.Ab != ab || len(a.Stellen) > quantenWOffenTeil {
		return QuantenWAnteilAusgabe{}, errors.New("Ausgabe passt nicht zur Anfrage")
	}
	return a, nil
}

// takt: eine Runde Hintergrundarbeit (begrenzt: je Note hoechstens ein
// Zufall, ein Teil der Offenlegung, eine Frist).
func (d *quantenWDienst) takt(ctx context.Context) {
	jetzt := d.jetzt()
	if !quantenWAktiv(jetzt) {
		return
	}
	reg := d.cs.quantenWJetzt()
	gesehen := map[string]bool{}
	defer d.sichtRaeumen(gesehen)
	for _, K := range d.schluessel() {
		e := d.eintrag(K)
		if e == nil {
			continue
		}
		if tok := reg.Token[K]; tok != nil {
			d.taktS(ctx, K, e, tok.token, jetzt, gesehen)
			continue
		}
		ein := reg.Noten[K]
		if ein == nil {
			// Tot oder nie gepraegt: nach dem Praegefenster vergessen.
			if _, tot := reg.Tote[K]; tot || e.Zeit < jetzt-quantenWToteBehalten {
				d.vergessen(K)
			}
			continue
		}
		a := ein.note.Anfrage
		if a == nil {
			continue
		}
		x := e.X - 1
		if a.Basen == nil {
			gesehen[quantenWAnfrageSicht(K, a)] = true
		} else if a.Antwort != nil {
			gesehen[quantenWAntwortSicht(K, a)] = true
		}
		switch {
		case a.Basen == nil && x < len(ein.note.ZIndex) && ein.note.ZIndex[x] < a.Index:
			if !d.reif(quantenWAnfrageSicht(K, a)) {
				continue // die Anfrage ist noch nicht final
			}
			g, _ := hex.DecodeString(e.Geheim)
			var gh [32]byte
			copy(gh[:], g)
			z := quantenWKetteVor(gh, quantenWZufallL-1-a.Index)
			_ = d.einreichen(quantenWZufallTxArt, QuantenWAngaben{K: K, Index: a.Index,
				Zufall: []QuantenWZufall{{V: e.X, Z: hex.EncodeToString(z[:])}}})
		case a.Antwort != nil && a.Offen < len(a.Stellen) && jetzt <= a.AntwortZeit+quantenWFristOffen:
			for i := 0; i < quantenWTeileJeTakt; i++ {
				ein := d.cs.quantenWJetzt().Noten[K]
				if ein == nil || ein.note.Anfrage == nil || ein.note.Anfrage.Index != a.Index {
					break
				}
				na := ein.note.Anfrage
				if na.Antwort == nil || na.Offen >= len(na.Stellen) || !d.darfEinreichen(K, e, na.Index, na.Offen, jetzt, gesehen) ||
					!d.offenlegenTeil(ctx, K, e, na) {
					break
				}
			}
		case (a.Antwort == nil && a.Basen != nil && jetzt > a.Gezogen+quantenWFristAntwort) ||
			(a.Antwort != nil && jetzt > a.AntwortZeit+quantenWFristOffen):
			_ = d.einreichen(quantenWFristTxArt, QuantenWAngaben{K: K, Index: a.Index})
		}
	}
}

// taktS: ein S-Token mit offener Anmeldung -- nach der Finalitaet in Teilen
// offenlegen; nach der Frist die Frist einreichen.
func (d *quantenWDienst) taktS(ctx context.Context, K string, e *quantenWTresorEintrag, t *quantenSToken, jetzt int64, gesehen map[string]bool) {
	a := t.Anmeldung
	if a == nil || e.Art != "s" {
		return
	}
	sicht := quantenWSAnmeldungSicht(K, t)
	gesehen[sicht] = true
	if jetzt > a.Zeit+quantenWFristOffen {
		_ = d.einreichen(quantenWSFristTxArt, QuantenWAngaben{K: K})
		return
	}
	if !d.reif(sicht) {
		return // die Anmeldung ist noch nicht final
	}
	for i := 0; i < quantenWTeileJeTakt; i++ {
		tok := d.cs.quantenWJetzt().Token[K]
		if tok == nil || tok.token.Anmeldung == nil || tok.token.Fehl != t.Fehl || quantenWSAnmeldungSicht(K, tok.token) != sicht {
			return
		}
		ab := tok.token.Anmeldung.Offen
		if !d.darfEinreichen(K, e, t.Fehl, ab, jetzt, gesehen) {
			return
		}
		if !d.teilOffenlegen(ctx, K, e, t.Fehl, ab, func(offen []QuantenWOffen) error {
			return d.einreichen(quantenWSOffenlegenTxArt, QuantenWAngaben{K: K, Ab: ab, Offen: offen})
		}) {
			return
		}
	}
}

func (d *quantenWDienst) offenlegenTeil(ctx context.Context, K string, e *quantenWTresorEintrag, a *quantenWAnfrage) bool {
	return d.teilOffenlegen(ctx, K, e, a.Index, a.Offen, func(offen []QuantenWOffen) error {
		return d.einreichen(quantenWOffenlegenTxArt, QuantenWAngaben{K: K, Index: a.Index, Ab: a.Offen, Offen: offen})
	})
}

// teilOffenlegen: eigene Anteile und die von Nachbarn sammeln, t passende
// zusammenstellen und einreichen (fuer W-Anfragen und S-Anmeldungen).
func (d *quantenWDienst) teilOffenlegen(ctx context.Context, K string, e *quantenWTresorEintrag, index, ab int, einreichen func([]QuantenWOffen) error) bool {
	eigen, err := d.anteile(K, index, ab)
	if err != nil {
		return false
	}
	aus := append([]QuantenWAnteilAusgabe{eigen}, d.nachbarAnteile(ctx, K, e, eigen)...)
	for _, wahl := range quantenWAuswahlen(len(aus), e.T, quantenWSammelVersuche) {
		offen := quantenWSammeln(aus, wahl, e.T)
		if offen == nil {
			continue
		}
		if einreichen(offen) == nil {
			return true
		}
	}
	return false
}

func (d *quantenWDienst) vergessen(K string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ablage.loeschen(K) == nil {
		delete(d.eintraege, K)
	}
}

// laufen: der Hintergrund, bis ctx endet.
func (d *quantenWDienst) laufen(ctx context.Context) {
	t := time.NewTicker(quantenWDienstTakt)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// Eine Panik in einem Takt beendet den Arbeiter nicht.
			SafeCall("quanten-w-dienst", func() { d.takt(ctx) })
		}
	}
}

// ------------------------------------------------------------ HTTP

func (a *APIServer) handleQuantenWBestaetigen(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	d := a.state.quantenWDienstJetzt()
	if d == nil || !quantenWAktiv(time.Now().Unix()) {
		jsonError(w, "quantum money variant W validator service not active", http.StatusConflict)
		return
	}
	if !burstErlaubt("quanten-w-dienst:"+erneuerungsAbsender(r), burstQuantenWDienstJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	var q QuantenWBestaetigenAnfrage
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, quantenWTresorKoerperMax))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&q); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	anker, sig, err := d.bestaetigen(q)
	if err != nil {
		if errors.Is(err, errQuantenW) {
			jsonError(w, "rejected: "+err.Error(), http.StatusConflict)
			return
		}
		fmt.Printf("[quanten_w] Bestaetigen gescheitert: %v\n", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"validator": d.adresse, "anker": anker, "bestaetigung": sig})
}

func (a *APIServer) handleQuantenWAnteile(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	d := a.state.quantenWDienstJetzt()
	if d == nil || !quantenWAktiv(time.Now().Unix()) {
		jsonError(w, "quantum money variant W validator service not active", http.StatusConflict)
		return
	}
	if !burstErlaubt("quanten-w-dienst:"+erneuerungsAbsender(r), burstQuantenWDienstJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	K := strings.ToLower(r.URL.Query().Get("k"))
	index, e1 := strconv.Atoi(r.URL.Query().Get("index"))
	ab, e2 := strconv.Atoi(r.URL.Query().Get("ab"))
	if !istHash64(K) || e1 != nil || e2 != nil {
		jsonError(w, "k, index, ab", http.StatusBadRequest)
		return
	}
	out, err := d.anteile(K, index, ab)
	if err != nil {
		jsonError(w, "rejected: "+err.Error(), http.StatusConflict)
		return
	}
	if err := d.anteilUnterschreiben(&out); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(out)
}

// quantenWDienstJetzt: der Dienst dieses Knotens (nil = keiner).
func (cs *ChainState) quantenWDienstJetzt() *quantenWDienst {
	return cs.quantenWDienstZeiger.Load()
}

// StarteQuantenWDienst: Tresor oeffnen und Hintergrund starten -- nur mit
// Signierschluessel. Die Peers (Basis-URLs der anderen Validatoren): die
// Nachbarn des Knotens (nachbarn, bei jedem Abruf neu; fremd, also nur
// oeffentliche Adressen) und dazu QUANTEN_W_PEERS (vom Betreiber gesetzt,
// kommagetrennt, nur http(s), auch private Netze); eingereicht wird ueber
// die eigene API (apiBasis), die zum Leiter leitet. hoehe: die DAG-Hoehe
// dieses Knotens (fuer reif); nil = nichts wird reif.
func (cs *ChainState) StarteQuantenWDienst(ctx context.Context, key *ecdsa.PrivateKey, apiBasis string, hoehe func() int64, nachbarn func() []string) error {
	if key == nil {
		return nil
	}
	var ablage quantenWAblage = &quantenWSpeicherAblage{}
	if cs.db != nil {
		if _, err := cs.db.Exec(`CREATE TABLE IF NOT EXISTS quanten_w_tresor (k TEXT PRIMARY KEY, daten BYTEA NOT NULL)`); err != nil {
			return err
		}
		ablage = quantenWDBAblage{db: cs.db}
	}
	var peers []string
	for _, p := range strings.Split(os.Getenv("QUANTEN_W_PEERS"), ",") {
		p = strings.TrimSpace(p)
		if u, err := url.Parse(p); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
			peers = append(peers, p)
		}
	}
	client := &http.Client{Timeout: quantenWPeerZeit}
	einreichen := func(art string, q QuantenWAngaben) error {
		body, err := json.Marshal(map[string]interface{}{"wallet": "", "zeit": 0, "quanten_w": q})
		if err != nil {
			return err
		}
		res, err := client.Post(strings.TrimRight(apiBasis, "/")+"/api/quanten-w/"+strings.TrimPrefix(art, "quanten_w_"), "application/json", strings.NewReader(string(body)))
		if err != nil {
			return err
		}
		defer res.Body.Close()
		io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("Status %d", res.StatusCode)
		}
		return nil
	}
	d, err := neuerQuantenWDienst(cs, key, ablage, peers, einreichen)
	if err != nil {
		return err
	}
	d.nachbarn = nachbarn
	if hoehe != nil {
		d.hoehe = hoehe
		d.final = func() int64 {
			h, _ := cs.GetFinalizedCheckpoint()
			return h
		}
	}
	cs.quantenWDienstZeiger.Store(d)
	go d.laufen(ctx)
	return nil
}
