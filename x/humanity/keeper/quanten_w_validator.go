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
//     die der anderen Validatoren ueber QUANTEN_W_PEERS) und einreichen;
//     abgelaufene Fristen einreichen; tote Noten aus dem Tresor entfernen.
//
// Jeder Knoten prueft jede eingereichte Transaktion selbst (Pruefsummen,
// Hash-Ketten): der Dienst ist bequem, nicht vertraut. Ein falscher Anteil
// eines Nachbarn faellt an der Pruefsumme auf; dann wird eine andere Auswahl
// versucht (hoechstens quantenWSammelVersuche).

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
	"net/http"
	"net/url"
	"os"
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
	quantenWDomAuftrag       = "aequitas-quanten-w-auftrag-v1"
	// Bestaetigen: n bis 16384 Anteile (~34 Zeichen) und Blaetter (~66 Zeichen).
	quantenWTresorKoerperMax = 2 << 20
	quantenWDienstTakt       = 5 * time.Second
	quantenWPeerZeit         = 5 * time.Second
	quantenWPeerAntwortMax   = 256 << 10
	quantenWPeersHoechstens  = 32
	quantenWSammelVersuche   = 8
	burstQuantenWDienstJeIP  = 30
	quantenWDomTresor        = "aequitas-quanten-w-tresor-v1|"
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
	peers     []string
	client    *http.Client
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
		client: &http.Client{Timeout: quantenWPeerZeit}, einreichen: einreichen, jetzt: nowUnix,
		hoehe: func() int64 { return 0 }, final: func() int64 { return -1 }, sicht: map[string]int64{}}
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
}

// quantenWAuftragNachricht: was der Praeger dem Validator unterschreibt.
func quantenWAuftragNachricht(praeger, K string, n, t, tol, x int, wurzel string, validatoren []string, zeit int64) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s|%d|%d|%d|%d|%s|%s|%d", quantenWDomAuftrag, eingefroreneChainID,
		strings.ToLower(praeger), K, n, t, tol, x, wurzel, strings.Join(validatoren, ","), zeit)
}

// bestaetigen: pruefen, ablegen, unterschreiben. Gibt Anker und Bestaetigung.
func (d *quantenWDienst) bestaetigen(q QuantenWBestaetigenAnfrage) (anker string, sig string, err error) {
	q.K = strings.ToLower(strings.TrimSpace(q.K))
	if !istHash64(q.K) {
		return "", "", quantenWFehler("K")
	}
	reg := d.cs.quantenWJetzt()
	if _, da := reg.Noten[q.K]; da {
		return "", "", quantenWFehler("Note schon gepraegt")
	}
	if _, tot := reg.Tote[q.K]; tot {
		return "", "", quantenWFehler("Note war schon gepraegt")
	}
	for _, v := range q.Validatoren {
		if !kanonischeAdresse(v) {
			return "", "", quantenWFehler("Validator nicht kanonisch")
		}
	}
	if _, err := quantenWPraegenPruefen(false, q.K, q.N, q.T, q.Validatoren, len(q.Validatoren), len(q.Validatoren), q.Tol); err != nil {
		return "", "", err
	}
	if q.X < 1 || q.X > len(q.Validatoren) || q.Validatoren[q.X-1] != d.adresse {
		return "", "", quantenWFehler("Nummer %d gehoert nicht zu diesem Validator", q.X)
	}
	// Der Auftrag: unterschrieben von einem Menschen mit mindestens der
	// kleinsten Deckung (sonst belegt ein Fremder kostenlos den Tresor).
	q.Praeger = strings.ToLower(strings.TrimSpace(q.Praeger))
	if !kanonischeAdresse(q.Praeger) || !kanonischeSignatur(q.PraegerSig) ||
		verifyPersonalSign(quantenWAuftragNachricht(q.Praeger, q.K, q.N, q.T, q.Tol, q.X, q.Wurzel, q.Validatoren, q.Zeit), q.PraegerSig, q.Praeger) != nil {
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
	var geheim [32]byte
	if _, err := rand.Read(geheim[:]); err != nil {
		return "", "", err
	}
	a := quantenWKetteVor(geheim, quantenWZufallL)
	e := &quantenWTresorEintrag{K: q.K, N: q.N, T: q.T, Tol: q.Tol, X: q.X, Validatoren: append([]string(nil), q.Validatoren...),
		Wurzel: q.Wurzel, Zeit: q.Zeit, Geheim: hex.EncodeToString(geheim[:]), Anteile: append([]string(nil), q.Anteile...),
		Blaetter: append([]string(nil), q.Blaetter...), Praeger: q.Praeger}
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
		if _, gepraegt := reg.Noten[k]; !gepraegt {
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
	msg := quantenWBestaetigungNachricht(q.K, wurzel, q.N, q.T, q.Validatoren, q.X, a, q.Zeit, q.Tol)
	s, err := crypto.Sign(accounts.TextHash([]byte(msg)), d.key)
	if err != nil {
		return "", "", err
	}
	s[64] += 27
	return hex.EncodeToString(a[:]), "0x" + hex.EncodeToString(s), nil
}

// praegerTauglich: ein Mensch mit mindestens der kleinsten Deckung.
// var: Tests ohne Datenbank setzen ein.
var quantenWPraegerTauglich = func(cs *ChainState, wallet string) bool {
	cs.ensureAccountLoadedCtx(context.Background(), wallet)
	acc, ok := cs.accounts.Get(wallet)
	return ok && acc.IsHuman && acc.Balance.Micro() >= quantenMindestDeckungMik
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
	if ab < 0 || ab >= len(a.Stellen) {
		return QuantenWAnteilAusgabe{}, quantenWFehler("ab %d", ab)
	}
	bis := min(ab+quantenWOffenTeil, len(a.Stellen))
	blaetter := make([][32]byte, e.N)
	for i, h := range e.Blaetter {
		blaetter[i], _ = quantenWHex32(h)
	}
	// Den Baum EINMAL bauen (nicht je Stelle: bei n = 16384 waeren das
	// sonst ~2 Mio. Hashes je Abruf -- Sicherheitspruefung W2d).
	ebenen := quantenWMerkleEbenen(blaetter)
	out := QuantenWAnteilAusgabe{K: K, Index: index, Ab: ab, X: e.X}
	for _, i := range a.Stellen[ab:bis] {
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

// peerAnteile: die Ausgabe eines Nachbarn (Grenzen: Zeit, Groesse, Form).
func (d *quantenWDienst) peerAnteile(ctx context.Context, basis, K string, index, ab int) (QuantenWAnteilAusgabe, error) {
	u := strings.TrimRight(basis, "/") + "/api/quanten-w/validator/anteile?" + url.Values{
		"k": {K}, "index": {strconv.Itoa(index)}, "ab": {strconv.Itoa(ab)}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return QuantenWAnteilAusgabe{}, err
	}
	res, err := d.client.Do(req)
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
			d.offenlegenTeil(ctx, K, e, a)
		case (a.Antwort == nil && a.Basen != nil && jetzt > a.Gezogen+quantenWFristAntwort) ||
			(a.Antwort != nil && jetzt > a.AntwortZeit+quantenWFristOffen):
			_ = d.einreichen(quantenWFristTxArt, QuantenWAngaben{K: K, Index: a.Index})
		}
	}
}

func (d *quantenWDienst) offenlegenTeil(ctx context.Context, K string, e *quantenWTresorEintrag, a *quantenWAnfrage) {
	eigen, err := d.anteile(K, a.Index, a.Offen)
	if err != nil {
		return
	}
	aus := []QuantenWAnteilAusgabe{eigen}
	for _, p := range d.peers {
		if len(aus) >= len(e.Validatoren) {
			break
		}
		if pa, err := d.peerAnteile(ctx, p, K, a.Index, a.Offen); err == nil && pa.X != eigen.X {
			aus = append(aus, pa)
		}
	}
	for _, wahl := range quantenWAuswahlen(len(aus), e.T, quantenWSammelVersuche) {
		offen := quantenWSammeln(aus, wahl, e.T)
		if offen == nil {
			continue
		}
		if d.einreichen(quantenWOffenlegenTxArt, QuantenWAngaben{K: K, Index: a.Index, Ab: a.Offen, Offen: offen}) == nil {
			return
		}
	}
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
	json.NewEncoder(w).Encode(out)
}

// quantenWDienstJetzt: der Dienst dieses Knotens (nil = keiner).
func (cs *ChainState) quantenWDienstJetzt() *quantenWDienst {
	return cs.quantenWDienstZeiger.Load()
}

// StarteQuantenWDienst: Tresor oeffnen und Hintergrund starten -- nur mit
// Signierschluessel. Die Peers (Basis-URLs der anderen Validatoren) kommen
// aus QUANTEN_W_PEERS (vom Betreiber gesetzt, kommagetrennt, nur http(s));
// eingereicht wird ueber die eigene API (apiBasis), die zum Leiter leitet.
// hoehe: die DAG-Hoehe dieses Knotens (fuer reif); nil = nichts wird reif.
func (cs *ChainState) StarteQuantenWDienst(ctx context.Context, key *ecdsa.PrivateKey, apiBasis string, hoehe func() int64) error {
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
