package keeper

// QUANTENGELD S MIT GETRENNTEN ROLLEN (forschung/quantengeld/DEZENTRAL.md 17,
// Schritte R1 und R2)
//
// `aequitasd quanten-s-ausgabe` ohne Rollentrennung spielt alle Rollen ausser
// den Validatoren: es erzeugt den Schluessel der Inhaberin, liest ihr x,
// bindet, waehlt und legt an beiden Stellen vor. Das belegt, dass die Kette
// richtig urteilt -- aber wer das Werkzeug laufen laesst, haelt alles in der
// Hand. Hier hat jede Rolle ihren eigenen Befehl und ihren eigenen Schluessel:
//
//	Inhaberin      quanten-s-inhaberin neu: Schluessel und Basen y; heraus
//	               kommt die Bestellung (oeffentlich).
//	Ausgeber       quanten-s-ausgabe --fuer-inhaberin < Bestellung: Abschnitte
//	               bei den Validatoren, Bestaetigungen, Ausgabe auf der Kette;
//	               heraus kommt das Paket. Er hat keinen Schluessel der
//	               Inhaberin und sieht x nie (nur fuer sie verschluesselt).
//	Inhaberin      quanten-s-inhaberin annehmen < Paket: jede Unterschrift
//	               der Validatoren, die Blaetter gegen die Wurzel und alles
//	               gegen die Kette; dann liest sie x.
//	               Oder ohne Paket: quanten-s-inhaberin abholen --k K -- den
//	               Stand von der Kette, jeden Abschnitt direkt vom Validator
//	               (GET /api/quanten-w/validator/fuer-inhaberin), dieselbe
//	               Pruefung.
//	Inhaberin      quanten-s-inhaberin einloesen --stelle b: bindet
//	               d = y XOR z, waehlt c = b XOR z und unterschreibt die
//	               Vorlage fuer GENAU diese Stelle.
//	Annahmestelle  quanten-s-annahme < Vorlage: Vorlage gegen die Kette, die
//	               Blaetter (mitgegeben oder bei den Validatoren geholt) gegen
//	               die Wurzel, nach der Finalitaet die eigene Stichprobe (mit
//	               dem eigenen Schluessel), das Urteil der Kette vorab
//	               nachgerechnet, anmelden, auf das Urteil warten. Die
//	               Vorlage ist kompakt (K, Stelle, x, Unterschrift).
//
// Nur mit Stichprobe v2 (DEZENTRAL.md 16): im bisherigen Weg muessten beide
// Stellen ihre Stichprobe schon bei der Ausgabe laden (Befund 5i).
//
// KEIN NEUER KONSENS. Die Kette prueft schon, dass nur die Inhaberin bindet
// und waehlt (quantenSToken.binden/waehlen), dass die Vorlage von ihr
// unterschrieben ist und dass an Stelle b in d XOR b XOR c geprueft wird
// (quantenSToken.offenlegenTeil). Neu ist nur die Ansicht des Tokens: Wurzel,
// Proben, d und c (oeffentlich, stehen in den Transaktionen).
//
// NICHTS WIRD GEGLAUBT:
//   - Die Inhaberin glaubt dem Ausgeber kein x: jeder Abschnitt traegt die
//     Unterschrift seines Validators (ueber den Hash von fuer_inhaberin, den
//     Blaettern und den Buendeln), die Validatoren nennt sie selbst
//     (--validatoren), Wurzel und Proben vergleicht sie mit der Kette.
//   - Die Annahmestelle glaubt der Inhaberin nichts: Unterschrift der
//     Vorlage gegen den Inhaber auf der Kette, Blaetter gegen die Wurzel auf
//     der Kette, Stichprobe und Salze gegen die Proben auf der Kette, Urteil
//     von der Kette. Ihre Vorab-Rechnung muss zum Urteil passen (sonst BEFUND).
//   - Die Validatoren-APIs kommen aus der eigenen Angabe, nie aus Paket oder
//     Vorlage (keine fremde Adresse wird angefragt).
//   - Den Stand der Kette liest jede Rolle bei dem Knoten, den SIE nennt
//     (--api), und glaubt ihm -- wie jede Wallet ohne eigenen Knoten. Die
//     Annahmestelle nimmt darum ihren eigenen Knoten (Vorgabe 127.0.0.1).
//
// WAS DER AUSGEBER LERNT: mit Basen im Klartext die Basen y (er reicht sie an
// die Validatoren weiter). Mit d auf der Kette kennt er z und aus c die
// gewaehlte Stelle, bevor die Inhaberin vorlegt -- also WO sie einloesen
// will, nicht x; einloesen oder binden kann er nicht. Darum kann die
// Inhaberin y je Validator verschluesseln (quanten-s-inhaberin neu
// --verschluesseln, quanten_s_basen_chiffrat.go): Dann sieht der Ausgeber nur
// Chiffrate, und jeder Validator unterschreibt den Hash des Chiffrats, das er
// geoeffnet hat -- einen Tausch erkennt die Inhaberin beim Annehmen.
//
// GRENZEN: Bestellung, Paket und Vorlage hoechstens quantenSRollenDateiMax,
// jede Anfrage mit Frist (quantenSHTTP), die Datei der Inhaberin nur 0600 und
// kein Verweis. Schluessel stehen in keiner Ausgabe und keiner Fehlermeldung.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// quantenSRollenDateiMax: Bestellung, Paket, Vorlage und Datei der Inhaberin
// (Blaetter: n <= 16.384 Hashes zu 64 Zeichen, etwa 1,1 MB).
const quantenSRollenDateiMax = 4 << 20

// QuantenSStelle: eine Annahmestelle -- Adresse und oeffentlicher Schluessel
// (unkomprimiert, hex). An ihn verschluesseln die Validatoren ihre Oeffnung.
type QuantenSStelle struct {
	Adresse    string `json:"adresse"`
	Schluessel string `json:"schluessel"`
}

// QuantenSBestellung: was die Inhaberin dem Ausgeber gibt. Nichts Geheimes.
type QuantenSBestellung struct {
	N     int      `json:"n"`
	Basen []string `json:"basen,omitempty"`
	// BasenVerschluesselt: statt Basen -- y je Abschnitt, an den Validator
	// dieses Abschnitts verschluesselt (quanten_s_basen_chiffrat.go).
	// Genau eines von beiden.
	BasenVerschluesselt []string          `json:"basen_verschluesselt,omitempty"`
	Inhaberin           string            `json:"inhaberin"`
	InhaberinSchluessel string            `json:"inhaberin_schluessel"`
	Stellen             [2]QuantenSStelle `json:"stellen"`
}

// QuantenSPaket: was der Ausgeber der Inhaberin zurueckgibt. Nichts Geheimes:
// x steht nur fuer die Inhaberin verschluesselt darin.
type QuantenSPaket struct {
	K           string                   `json:"k"`
	N           int                      `json:"n"`
	T           int                      `json:"t"`
	Tol         int                      `json:"tol"`
	ProbeArt    int                      `json:"probe_art"`
	Ausgeber    string                   `json:"ausgeber"`
	Inhaberin   string                   `json:"inhaberin"`
	Stellen     [2]string                `json:"stellen"`
	Validatoren []string                 `json:"validatoren"`
	Wurzel      string                   `json:"wurzel"`
	Proben      [2]string                `json:"proben"`
	Deckung     int64                    `json:"deckung_mikro"`
	Abschnitte  []QuantenSPaketAbschnitt `json:"abschnitte"`
}

// QuantenSPaketAbschnitt: die Antwort von Validator X ohne die Buendel fuer
// die anderen Validatoren (nur ihre Hashes) -- genug, um seine Unterschrift
// zu pruefen (quantenSAbschnittAntwortNachrichtAus).
type QuantenSPaketAbschnitt struct {
	X              int      `json:"x"`
	Von            int      `json:"von"`
	Bis            int      `json:"bis"`
	Blaetter       []string `json:"blaetter"`
	FuerInhaberin  string   `json:"fuer_inhaberin"`
	BuendelHash    string   `json:"buendel_hash"`
	BuendelSigHash string   `json:"buendel_sig_hash"`
	Teile          []string `json:"teile"`
	Zaehlung       [][2]int `json:"zaehlung"`
	Sig            string   `json:"sig"`
	// BasenHash: nur bei verschluesselten Basen -- Hash des Chiffrats, das
	// der Validator geoeffnet hat (mit unterschrieben).
	BasenHash string `json:"basen_hash,omitempty"`
}

// QuantenSInhaberinDatei: was die Inhaberin aufbewahrt (0600). GEHEIM: der
// Schluessel; x und z sagen, wie der Token vorzulegen ist.
type QuantenSInhaberinDatei struct {
	Schluessel string             `json:"schluessel"`
	Bestellung QuantenSBestellung `json:"bestellung"`
	V          int                `json:"v"`
	// Nur bei verschluesselten Basen: y je Abschnitt im Klartext (GEHEIM --
	// in der Bestellung stehen nur die Chiffrate) und die Validatoren, an die
	// verschluesselt ist (in der Reihenfolge der Abschnitte).
	Basen       []string `json:"basen,omitempty"`
	Validatoren []string `json:"validatoren,omitempty"`
	K           string   `json:"k,omitempty"`
	X           string   `json:"x,omitempty"`
	Blaetter    []string `json:"blaetter,omitempty"`
	// Z: die Maske der Bindung, gespeichert BEVOR die Bindung eingereicht
	// wird -- ein Abbruch danach verliert sie nicht.
	Z *int `json:"z,omitempty"`
}

// QuantenSVorlage: was die Inhaberin der Annahmestelle gibt. Blaetter sind
// freiwillig (dann ohne Validatoren pruefbar gegen die Wurzel); fehlen sie,
// holt die Stelle sie selbst bei den Validatoren.
type QuantenSVorlage struct {
	K        string   `json:"k"`
	B        int      `json:"b"`
	X        string   `json:"x"`
	Vorlage  string   `json:"vorlage"`
	Blaetter []string `json:"blaetter,omitempty"`
}

// QuantenSAnnahmeErgebnis: was die Annahmestelle am Ende ausgibt.
type QuantenSAnnahmeErgebnis struct {
	K          string `json:"k"`
	B          int    `json:"stelle_nummer"`
	Stelle     string `json:"stelle"`
	Inhaberin  string `json:"inhaberin"`
	Stichprobe int    `json:"stichprobe"`
	Treffer    int    `json:"treffer"`
	Fehler     int    `json:"fehler"`
	Vorab      string `json:"vorab"`  // "besteht" oder "abgewiesen" (eigene Rechnung)
	Urteil     string `json:"urteil"` // "eingeloest", "abgewiesen" oder "frist" (Kette)
	// Wurzel und Proben der Ausgabe, vor dem Anmelden von der Kette gelesen
	// (oeffentlich) -- dagegen prueft der Nachweis nach dem Ende.
	Wurzel string   `json:"wurzel"`
	Proben []string `json:"proben"`
	// Nachweis nach dem Ende (quanten_s_nachweis.go), wenn verlangt; sonst,
	// warum nicht (das Einloesen gilt trotzdem).
	Nachweis       *QuantenSNachweisBericht `json:"nachweis,omitempty"`
	NachweisFehler string                   `json:"nachweis_fehler,omitempty"`
}

// ErrQuantenSNachweis: eingeloest, aber der Nachweis nach dem Ende fehlt oder
// passt nicht (BEFUND steht dann im Text).
var ErrQuantenSNachweis = errors.New("eingeloest, aber Nachweis nach dem Ende nicht bestaetigt")

// ------------------------------------------------------------ Inhaberin: neu

func quantenSSchluesselText(s string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x")
}

// pruefen: Form der Bestellung fuer v Validatoren; Adressen klein, Schluessel
// ohne 0x (so unterschreibt der Ausgeber sie im Auftrag).
func (b *QuantenSBestellung) pruefen(v int) error {
	verschl := b.BasenVerschluesselt != nil
	abschnitte := len(b.Basen)
	if verschl {
		abschnitte = len(b.BasenVerschluesselt)
	}
	if b.N < 2 || b.N > quantenWNMax || v < 1 || v > quantenWVMax || abschnitte != v || (verschl && b.Basen != nil) {
		return fmt.Errorf("Bestellung: n = %d, %d Abschnitte fuer %d Validatoren (Basen genau einmal: Klartext oder verschluesselt)", b.N, abschnitte, v)
	}
	for x := 1; x <= v; x++ {
		von, bis, ok := quantenSAbschnitt(b.N, v, x)
		if !ok {
			return fmt.Errorf("Bestellung: Abschnitte n = %d, v = %d", b.N, v)
		}
		if verschl {
			// Oeffnen kann nur der Validator; hier nur die Form.
			if !quantenSBasenChiffratForm(b.BasenVerschluesselt[x-1]) {
				return fmt.Errorf("Bestellung: verschluesselte Basen, Abschnitt %d: Form oder Groesse", x)
			}
			continue
		}
		if _, err := quantenSBitsText(b.Basen[x-1], bis-von); err != nil {
			return fmt.Errorf("Bestellung: Basen, Abschnitt %d: %v", x, err)
		}
	}
	b.Inhaberin = strings.ToLower(strings.TrimSpace(b.Inhaberin))
	b.InhaberinSchluessel = quantenSSchluesselText(b.InhaberinSchluessel)
	if !kanonischeAdresse(b.Inhaberin) {
		return errors.New("Bestellung: Inhaberin")
	}
	if _, err := quantenSSchluesselPruefen(b.InhaberinSchluessel, b.Inhaberin); err != nil {
		return fmt.Errorf("Bestellung: Inhaberin: %v", err)
	}
	for s := 0; s < 2; s++ {
		st := &b.Stellen[s]
		st.Adresse = strings.ToLower(strings.TrimSpace(st.Adresse))
		st.Schluessel = quantenSSchluesselText(st.Schluessel)
		if !kanonischeAdresse(st.Adresse) || st.Adresse == b.Inhaberin || (s == 1 && st.Adresse == b.Stellen[0].Adresse) {
			return fmt.Errorf("Bestellung: Annahmestelle %d (kanonisch, verschieden, nicht die Inhaberin)", s)
		}
		if _, err := quantenSSchluesselPruefen(st.Schluessel, st.Adresse); err != nil {
			return fmt.Errorf("Bestellung: Annahmestelle %d: %v", s, err)
		}
	}
	return nil
}

func (b QuantenSBestellung) stellenAdressen() [2]string {
	return [2]string{b.Stellen[0].Adresse, b.Stellen[1].Adresse}
}

// quantenSInhaberinNeu: neuer Schluessel und Basen y je Abschnitt (aus
// crypto/rand) fuer einen Token mit n Stellen bei v Validatoren. Mit
// validatoren (Adresse und oeffentlicher Schluessel je Abschnitt, Schluessel
// gegen die Adresse geprueft) stehen in der Bestellung nur Chiffrate; y
// bleibt in der Datei der Inhaberin.
func quantenSInhaberinNeu(n, v int, stellen [2]QuantenSStelle, validatoren []QuantenSStelle) (*QuantenSInhaberinDatei, error) {
	basen, err := quantenSBasenZiehen(n, v)
	if err != nil {
		return nil, err
	}
	key, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}
	inhaberin := quantenSAdresse(key)
	b := QuantenSBestellung{N: n, Inhaberin: inhaberin, InhaberinSchluessel: quantenSPubHex(key), Stellen: stellen}
	d := &QuantenSInhaberinDatei{V: v}
	if validatoren == nil {
		b.Basen = basen
	} else {
		if len(validatoren) != v {
			return nil, fmt.Errorf("%d Validatoren mit Schluessel fuer %d Abschnitte", len(validatoren), v)
		}
		for x := 1; x <= v; x++ {
			a := strings.ToLower(strings.TrimSpace(validatoren[x-1].Adresse))
			if !kanonischeAdresse(a) || slices.Contains(d.Validatoren, a) {
				return nil, fmt.Errorf("Validator %d: Adresse nicht kanonisch oder doppelt", x)
			}
			d.Validatoren = append(d.Validatoren, a)
		}
		for x := 1; x <= v; x++ {
			c, err := quantenSBasenVerschluesseln(validatoren[x-1].Schluessel, d.Validatoren, n, x, inhaberin, basen[x-1])
			if err != nil {
				return nil, fmt.Errorf("Validator %d: %v", x, err)
			}
			b.BasenVerschluesselt = append(b.BasenVerschluesselt, c)
		}
		d.Basen = basen
	}
	if err := b.pruefen(v); err != nil {
		return nil, err
	}
	d.Schluessel, d.Bestellung = hex.EncodeToString(crypto.FromECDSA(key)), b
	if _, err := d.y(); err != nil {
		return nil, err
	}
	return d, nil
}

// schluessel: der private Schluessel der Datei; er muss zur Inhaberin der
// Bestellung gehoeren.
func (d *QuantenSInhaberinDatei) schluessel() (*ecdsa.PrivateKey, error) {
	key, err := crypto.HexToECDSA(quantenSSchluesselText(d.Schluessel))
	if err != nil {
		return nil, errors.New("Datei der Inhaberin: Schluessel unlesbar") // ohne den Schluessel im Text
	}
	if quantenSAdresse(key) != d.Bestellung.Inhaberin {
		return nil, errors.New("Datei der Inhaberin: Schluessel gehoert nicht zur Bestellung")
	}
	return key, nil
}

// y: die Basen der Inhaberin ueber alle Abschnitte -- aus der Bestellung
// oder, bei verschluesselten Basen, aus der Datei (dann muss auch die Liste
// der Validatoren passen).
func (d *QuantenSInhaberinDatei) y() ([]int, error) {
	if err := d.Bestellung.pruefen(d.V); err != nil {
		return nil, err
	}
	basen := d.Bestellung.Basen
	if d.Bestellung.BasenVerschluesselt != nil {
		if len(d.Basen) != d.V || len(d.Validatoren) != d.V {
			return nil, errors.New("Datei der Inhaberin: verschluesselte Bestellung ohne eigene Basen oder Validatoren")
		}
		basen = d.Basen
	} else if d.Basen != nil || d.Validatoren != nil {
		return nil, errors.New("Datei der Inhaberin: Basen doppelt (Bestellung im Klartext)")
	}
	var y []int
	for x := 1; x <= d.V; x++ {
		von, bis, _ := quantenSAbschnitt(d.Bestellung.N, d.V, x)
		b, err := quantenSBitsText(basen[x-1], bis-von)
		if err != nil {
			return nil, fmt.Errorf("Datei der Inhaberin: Basen, Abschnitt %d: %v", x, err)
		}
		y = append(y, b...)
	}
	return y, nil
}

// basenHash: was Validator x in seiner Antwort unterschrieben haben muss --
// der Hash des eigenen Chiffrats, oder leer bei Basen im Klartext.
func (b QuantenSBestellung) basenHash(x int) string {
	if b.BasenVerschluesselt == nil || x < 1 || x > len(b.BasenVerschluesselt) {
		return ""
	}
	return quantenSBasenHash(b.BasenVerschluesselt[x-1])
}

// basenFuerRunde: was der Ausgeber den Validatoren weiterreicht.
func (b QuantenSBestellung) basenFuerRunde() QuantenSBasen {
	return QuantenSBasen{N: b.N, Basen: b.Basen, Verschluesselt: b.BasenVerschluesselt, Inhaberin: b.Inhaberin}
}

// ------------------------------------------------------------ Ausgeber

// quantenSAusgabeFuerInhaberin: der Ausgeber gibt einen Token fuer die
// Inhaberin der Bestellung aus (nur Stichprobe v2). Er sieht weder x noch
// einen Schluessel der Inhaberin oder der Stellen.
func quantenSAusgabeFuerInhaberin(ctx context.Context, a QuantenSAusgabeAuftrag, best QuantenSBestellung, validatoren []string, t int,
	netz quantenSAbschnittNetz, log io.Writer) (*QuantenSPaket, error) {
	a.log, a.beginn = log, time.Now()
	if !a.ProbeV2 {
		return nil, errors.New("getrennte Rollen nur mit Stichprobe v2 (sonst laedt jede Stelle ihre Stichprobe schon bei der Ausgabe)")
	}
	if err := best.pruefen(len(validatoren)); err != nil {
		return nil, err
	}
	ausgeber := quantenSAdresse(a.Schluessel)
	if best.Inhaberin == ausgeber {
		return nil, errors.New("Bestellung: die Inhaberin ist der Ausgeber selbst -- das ist keine Trennung der Rollen")
	}
	basen := best.basenFuerRunde()
	r, err := quantenSRundeBeginnen(a, basen, validatoren, t)
	if err != nil {
		return nil, err
	}
	if err := r.schluesselHolen(ctx, netz); err != nil {
		return nil, err
	}
	stellen := best.stellenAdressen()
	if err := r.abschnitteHolen(ctx, a, basen, netz, best.Inhaberin, best.InhaberinSchluessel, stellen,
		[2]string{best.Stellen[0].Schluessel, best.Stellen[1].Schluessel}); err != nil {
		return nil, err
	}
	proben, probenHex, treffer, fehler, err := r.probenV2()
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(log, "Token %s fuer %s: n=%d in %d Abschnitten, Stichprobe v2, Fehlerrate bei passender Basis %d/%d (aus den Zaehlungen der Validatoren)\n",
		r.K[:12], best.Inhaberin, r.n, r.v, fehler[0]+fehler[1], treffer[0]+treffer[1])
	if err := quantenSEhrlichBesteht(treffer, fehler, a.Tol); err != nil {
		return nil, err
	}
	bestaetigungen, err := r.bestaetigungenHolen(ctx, a, netz, proben, probenHex)
	if err != nil {
		return nil, err
	}
	wurzelHex := hex.EncodeToString(r.wurzel[:])
	q := QuantenWAngaben{K: r.K, N: r.n, T: t, Wurzel: wurzelHex, Validatoren: validatoren, Bestaetigungen: bestaetigungen,
		Deckung: a.Deckung, Tol: a.Tol, Inhaber: best.Inhaberin, Annahme: stellen[:], Proben: probenHex, ProbeArt: 2}
	if err := quantenSAusgabeEinreichen(ctx, a, netz, q, ausgeber, r.zeit); err != nil {
		return nil, err
	}
	fmt.Fprintf(log, "ausgegeben (nach %s); das Paket geht an die Inhaberin\n", a.quantenSNach())
	return r.paket(a, best.Inhaberin, stellen, probenHex), nil
}

// paket: was die Inhaberin bekommt -- je Abschnitt die unterschriebene
// Antwort des Validators, die Buendel nur als Hashes.
func (r *quantenSRunde) paket(a QuantenSAusgabeAuftrag, inhaberin string, stellen [2]string, probenHex []string) *QuantenSPaket {
	p := &QuantenSPaket{K: r.K, N: r.n, T: r.t, Tol: a.Tol, ProbeArt: 2, Ausgeber: r.ausgeber, Inhaberin: inhaberin, Stellen: stellen,
		Validatoren: append([]string(nil), r.validatoren...), Wurzel: hex.EncodeToString(r.wurzel[:]), Proben: [2]string{probenHex[0], probenHex[1]}, Deckung: a.Deckung}
	for _, ans := range r.antw {
		p.Abschnitte = append(p.Abschnitte, quantenSPaketAbschnittAus(ans))
	}
	return p
}

// quantenSPaketAbschnittAus: die Antwort eines Validators ohne die Buendel
// fuer die anderen (nur ihre Hashes); die Unterschrift bleibt pruefbar.
func quantenSPaketAbschnittAus(ans QuantenSAbschnittAntwort) QuantenSPaketAbschnitt {
	return QuantenSPaketAbschnitt{X: ans.X, Von: ans.Von, Bis: ans.Bis, Blaetter: ans.Blaetter,
		FuerInhaberin: ans.FuerInhaberin, BuendelHash: quantenSHashListe(ans.Buendel), BuendelSigHash: quantenSHashListe(ans.BuendelSig),
		Teile: ans.Teile, Zaehlung: ans.Zaehlung, Sig: ans.Sig, BasenHash: ans.BasenHash}
}

// ------------------------------------------------------------ Inhaberin: annehmen

func qsText(st map[string]interface{}, k string) string {
	s, _ := st[k].(string)
	return s
}

func qsTexte(st map[string]interface{}, k string) []string {
	l, _ := st[k].([]interface{})
	out := make([]string, 0, len(l))
	for _, e := range l {
		s, ok := e.(string)
		if !ok {
			return nil
		}
		out = append(out, s)
	}
	return out
}

// quantenSStandWie: der Stand auf der Kette muss genau zum Paket passen.
func quantenSStandWie(st map[string]interface{}, p QuantenSPaket, v int) error {
	stellen := qsTexte(st, "stellen")
	proben := qsTexte(st, "proben")
	switch {
	case st == nil:
		return errors.New("der Token steht nicht auf der Kette")
	case st["art"] != "s" || quantenSZahl(st["probe_art"]) != 2:
		return errors.New("auf der Kette kein Token mit Stichprobe v2")
	case qsText(st, "inhaber") != p.Inhaberin:
		return errors.New("auf der Kette gehoert der Token jemand anderem")
	case quantenSZahl(st["n"]) != p.N || quantenSZahl(st["t"]) != p.T || quantenSZahl(st["v"]) != v || quantenSZahl(st["tol"]) != p.Tol:
		return errors.New("n, t, v oder Toleranz auf der Kette weichen vom Paket ab")
	case len(stellen) != 2 || stellen[0] != p.Stellen[0] || stellen[1] != p.Stellen[1]:
		return errors.New("die Annahmestellen auf der Kette weichen vom Paket ab")
	case qsText(st, "wurzel") != p.Wurzel || len(proben) != 2 || proben[0] != p.Proben[0] || proben[1] != p.Proben[1]:
		return errors.New("Wurzel oder Proben auf der Kette weichen vom Paket ab")
	case st["tot"] == true:
		return errors.New("der Token ist tot")
	}
	return nil
}

// quantenSInhaberinAnnehmen: die Inhaberin prueft das Paket und liest x. Die
// Validatoren (Adressen, in der Reihenfolge der Abschnitte) nennt sie selbst.
// Schreibt K, x und die Blaetter in die Datei (der Aufrufer speichert).
func quantenSInhaberinAnnehmen(ctx context.Context, netz quantenSNetz, dat *QuantenSInhaberinDatei, validatoren []string, p QuantenSPaket, log io.Writer) error {
	key, err := dat.schluessel()
	if err != nil {
		return err
	}
	best := dat.Bestellung
	if err := best.pruefen(dat.V); err != nil {
		return err
	}
	v, n := len(validatoren), best.N
	if v != dat.V {
		return fmt.Errorf("%d Validatoren angegeben, bestellt fuer %d", v, dat.V)
	}
	p.K = strings.ToLower(strings.TrimSpace(p.K))
	if !istHash64(p.K) {
		return errors.New("Paket: K")
	}
	if dat.K != "" && dat.K != p.K {
		return fmt.Errorf("diese Datei hat schon den Token %s angenommen", dat.K[:12])
	}
	if p.Inhaberin != best.Inhaberin || p.N != n || p.Stellen != best.stellenAdressen() {
		return errors.New("das Paket ist nicht fuer diese Bestellung (Inhaberin, n oder Annahmestellen)")
	}
	if p.ProbeArt != 2 {
		return errors.New("Paket: nur Stichprobe v2")
	}
	if len(p.Validatoren) != v {
		return fmt.Errorf("das Paket nennt %d Validatoren, angegeben %d", len(p.Validatoren), v)
	}
	for i := range validatoren {
		if p.Validatoren[i] != strings.ToLower(strings.TrimSpace(validatoren[i])) {
			return fmt.Errorf("das Paket nennt als Validator %d einen anderen als angegeben", i+1)
		}
	}
	if _, err := dat.y(); err != nil {
		return err
	}
	if dat.Validatoren != nil && !slices.Equal(dat.Validatoren, p.Validatoren) {
		return errors.New("die Basen sind an andere Validatoren verschluesselt als angegeben")
	}
	if p.T < 1 || p.T > v || quantenWKNote(p.Tol, p.T, v) == 0 {
		return fmt.Errorf("Paket: Schwelle %d oder Toleranz %d", p.T, p.Tol)
	}
	wurzel, ok := quantenWHex32(p.Wurzel)
	if !ok {
		return errors.New("Paket: Wurzel")
	}
	var proben [2][32]byte
	for s := 0; s < 2; s++ {
		if proben[s], ok = quantenWHex32(p.Proben[s]); !ok {
			return errors.New("Paket: Proben")
		}
	}
	if len(p.Abschnitte) != v {
		return fmt.Errorf("Paket: %d Abschnitte fuer %d Validatoren", len(p.Abschnitte), v)
	}
	blaetter := make([][32]byte, 0, n)
	x := make([]int, n)
	var treffer, fehler [2]int
	var teile [2][][32]byte
	for i, ab := range p.Abschnitte {
		xi := i + 1
		if err := quantenSPaketAbschnittPruefen(ab, p.K, n, v, xi, p.Validatoren[i]); err != nil {
			return fmt.Errorf("Paket: %v", err)
		}
		// Hat der Validator genau das eigene Chiffrat geoeffnet? (Leer bei
		// Basen im Klartext.) Sonst hat jemand die Basen getauscht.
		if ab.BasenHash != best.basenHash(xi) {
			return fmt.Errorf("BEFUND: Validator %d hat nicht in den eigenen Basen praepariert (Chiffrat getauscht)", xi)
		}
		von, bis, _ := quantenSAbschnitt(n, v, xi)
		if err := quantenSAntwortV2Pruefen(QuantenSAbschnittAntwort{Teile: ab.Teile, Zaehlung: ab.Zaehlung}, bis-von); err != nil {
			return fmt.Errorf("Paket: Abschnitt %d: %v", xi, err)
		}
		for j, h := range ab.Blaetter {
			bl, ok := quantenWHex32(h)
			if !ok {
				return fmt.Errorf("Paket: Abschnitt %d, Blatt %d", xi, j)
			}
			blaetter = append(blaetter, bl)
		}
		xs, err := quantenSFuerInhaberinOeffnen(key, ab.FuerInhaberin, p.K, von, bis)
		if err != nil {
			return fmt.Errorf("Paket: Abschnitt %d: %v", xi, err)
		}
		copy(x[von:], xs)
		for s := 0; s < 2; s++ {
			h, _ := quantenWHex32(ab.Teile[s])
			teile[s] = append(teile[s], h)
			treffer[s] += ab.Zaehlung[s][0]
			fehler[s] += ab.Zaehlung[s][1]
		}
	}
	if quantenWMerkleWurzel(blaetter) != wurzel {
		return errors.New("Paket: die Blaetter ergeben nicht die Wurzel")
	}
	for s := 0; s < 2; s++ {
		if h, err := quantenSProbeHashV2(p.K, s, teile[s]); err != nil || h != proben[s] {
			return fmt.Errorf("Paket: Probe %d passt nicht zu den Teil-Hashes der Validatoren", s)
		}
	}
	for s := 0; s < 2; s++ {
		if quantenWUrteil(treffer[s], fehler[s], p.Tol) != quantenWEcht {
			return fmt.Errorf("BEFUND: nach den Zaehlungen der Validatoren bestuende die ehrliche Vorlage an Stelle %d nicht (%d Fehler bei %d Treffern)", s, fehler[s], treffer[s])
		}
	}
	st, err := netz.stand(ctx, p.K)
	if err != nil {
		return err
	}
	if err := quantenSStandWie(st, p, v); err != nil {
		return err
	}
	if dat.K == "" && (st["gebunden"] == true || st["anmeldung"] != nil || quantenSZahl(st["fehl"]) != 0) {
		return errors.New("BEFUND: der Token ist schon gebunden oder angemeldet, bevor die Inhaberin ihn angenommen hat")
	}
	dat.K, dat.X = p.K, qsBitsText(x)
	dat.Blaetter = nil
	for _, ab := range p.Abschnitte {
		dat.Blaetter = append(dat.Blaetter, ab.Blaetter...)
	}
	fmt.Fprintf(log, "Token %s angenommen: %d Abschnitte von %d Validatoren unterschrieben, Blaetter ergeben die Wurzel, Kette passt; ehrlich bestuende er an beiden Stellen (%d/%d und %d/%d Fehler)\n",
		p.K[:12], v, v, fehler[0], treffer[0], fehler[1], treffer[1])
	return nil
}

// quantenSPaketAbschnittPruefen: Form des Abschnitts x und die Unterschrift
// seines Validators (Adresse aus der eigenen Liste) -- Grundlage fuer
// Inhaberin und Annahmestelle.
func quantenSPaketAbschnittPruefen(ab QuantenSPaketAbschnitt, K string, n, v, x int, validator string) error {
	von, bis, ok := quantenSAbschnitt(n, v, x)
	if !ok || ab.X != x || ab.Von != von || ab.Bis != bis || len(ab.Blaetter) != bis-von || !istHash64(ab.BuendelHash) || !istHash64(ab.BuendelSigHash) ||
		(ab.BasenHash != "" && !istHash64(ab.BasenHash)) {
		return fmt.Errorf("Abschnitt %d hat nicht die Form", x)
	}
	m := quantenSAbschnittAntwortNachrichtAus(K, x, von, bis, quantenSHashListe(ab.Blaetter), ab.FuerInhaberin, ab.BuendelHash, ab.BuendelSigHash, ab.Teile, ab.Zaehlung, ab.BasenHash)
	if !kanonischeSignatur(ab.Sig) || verifyPersonalSign(m, ab.Sig, validator) != nil {
		return fmt.Errorf("Abschnitt %d ist nicht von Validator %d unterschrieben", x, x)
	}
	return nil
}

// quantenSBlaetterHolen: die Blaetter aller Abschnitte, jeder direkt von
// seinem Validator und von ihm unterschrieben. Ob sie die Wurzel ergeben,
// prueft der Aufrufer gegen die Kette.
func quantenSBlaetterHolen(ctx context.Context, netz quantenSAbschnittNetz, K string, n, v int, validatoren []string) ([][32]byte, error) {
	if v != len(validatoren) {
		return nil, fmt.Errorf("%d Validatoren angegeben, %d verlangt", len(validatoren), v)
	}
	if n < 1 || n > quantenWNMax {
		return nil, fmt.Errorf("n = %d", n)
	}
	out := make([][32]byte, n)
	var fehlend []int
	var fehler []string
	for x := 1; x <= v; x++ {
		ab, err := netz.fuerInhaberin(ctx, x, K)
		if err != nil {
			// R9: nicht erreichbar -- seine Blaetter kann jeder andere
			// Validator nennen; ob sie stimmen, prueft der Aufrufer an der
			// Wurzel auf der Kette.
			fehlend = append(fehlend, x)
			fehler = append(fehler, fmt.Sprintf("Validator %d: Abschnitt: %v", x, err))
			continue
		}
		if err := quantenSPaketAbschnittPruefen(ab, K, n, v, x, validatoren[x-1]); err != nil {
			return nil, err
		}
		von, _, _ := quantenSAbschnitt(n, v, x)
		for j, h := range ab.Blaetter {
			bl, ok := quantenWHex32(h)
			if !ok {
				return nil, fmt.Errorf("Abschnitt %d, Blatt %d", x, j)
			}
			out[von+j] = bl
		}
	}
	if len(fehlend) == 0 {
		return out, nil
	}
	for j := 1; j <= v; j++ {
		if slices.Contains(fehlend, j) {
			continue
		}
		alle, err := netz.alleBlaetter(ctx, j, K)
		if err != nil || len(alle) != n {
			continue
		}
		voll := slices.Clone(out)
		ok := true
		for _, x := range fehlend {
			von, bis, _ := quantenSAbschnitt(n, v, x)
			for i := von; i < bis && ok; i++ {
				voll[i], ok = quantenWHex32(alle[i])
			}
		}
		if ok {
			return voll, nil
		}
	}
	return nil, errors.New(strings.Join(fehler, "; ") + "; kein anderer Validator nannte die Blaetter")
}

// quantenSPaketVonKette: das Paket ohne den Ausgeber -- der Stand von der
// Kette, jeder Abschnitt direkt von seinem Validator. Geglaubt wird nichts
// davon: quantenSInhaberinAnnehmen prueft es danach wie ein Paket des
// Ausgebers (Unterschriften, Wurzel, Proben, Zaehlungen, Kette).
func quantenSPaketVonKette(ctx context.Context, netz quantenSAbschnittNetz, K string, validatoren []string) (QuantenSPaket, error) {
	K = strings.ToLower(strings.TrimSpace(K))
	if !istHash64(K) {
		return QuantenSPaket{}, errors.New("K: 64 Hex-Zeichen")
	}
	st, err := netz.stand(ctx, K)
	if err != nil {
		return QuantenSPaket{}, err
	}
	stellen, proben := qsTexte(st, "stellen"), qsTexte(st, "proben")
	if st == nil || len(stellen) != 2 || len(proben) != 2 {
		return QuantenSPaket{}, errors.New("der Token steht nicht auf der Kette")
	}
	v := quantenSZahl(st["v"])
	if v != len(validatoren) {
		return QuantenSPaket{}, fmt.Errorf("die Kette nennt %d Validatoren, angegeben %d", v, len(validatoren))
	}
	p := QuantenSPaket{K: K, N: quantenSZahl(st["n"]), T: quantenSZahl(st["t"]), Tol: quantenSZahl(st["tol"]), ProbeArt: quantenSZahl(st["probe_art"]),
		Inhaberin: qsText(st, "inhaber"), Stellen: [2]string{stellen[0], stellen[1]}, Wurzel: qsText(st, "wurzel"), Proben: [2]string{proben[0], proben[1]}}
	for _, a := range validatoren {
		p.Validatoren = append(p.Validatoren, strings.ToLower(strings.TrimSpace(a)))
	}
	for x := 1; x <= v; x++ {
		ab, err := netz.fuerInhaberin(ctx, x, K)
		if err != nil {
			return QuantenSPaket{}, fmt.Errorf("Validator %d: Abschnitt: %w", x, err)
		}
		p.Abschnitte = append(p.Abschnitte, ab)
	}
	return p, nil
}

// ------------------------------------------------------------ Inhaberin: einloesen

// quantenSInhaberinEinloesen: bindet und waehlt (je nur, wenn es noch nicht
// auf der Kette steht) und unterschreibt die Vorlage fuer Stelle s. Ist
// schon die andere Stelle gewaehlt, unterschreibt sie NICHT (das waere eine
// Doppelvorlage, die die Kette als Fehlversuch zaehlt). speichern sichert die
// Datei, bevor die Bindung eingereicht wird.
func quantenSInhaberinEinloesen(ctx context.Context, netz quantenSNetz, frist time.Duration, dat *QuantenSInhaberinDatei, speichern func() error, s int, log io.Writer) (*QuantenSVorlage, error) {
	if s != 0 && s != 1 {
		return nil, fmt.Errorf("Stelle %d (0 oder 1)", s)
	}
	if dat.K == "" {
		return nil, errors.New("noch kein Token angenommen (quanten-s-inhaberin annehmen)")
	}
	key, err := dat.schluessel()
	if err != nil {
		return nil, err
	}
	y, err := dat.y()
	if err != nil {
		return nil, err
	}
	n, K, inhaber := dat.Bestellung.N, dat.K, dat.Bestellung.Inhaberin
	x, err := quantenSBitsText(dat.X, n)
	if err != nil {
		return nil, errors.New("Datei der Inhaberin: x")
	}
	st, err := netz.stand(ctx, K)
	if err != nil {
		return nil, err
	}
	stellen := qsTexte(st, "stellen")
	switch {
	case st == nil:
		return nil, errors.New("der Token steht nicht (mehr) auf der Kette -- eingeloest oder tot")
	case qsText(st, "inhaber") != inhaber || len(stellen) != 2 || stellen[s] != dat.Bestellung.Stellen[s].Adresse:
		return nil, errors.New("der Token auf der Kette passt nicht zur Datei")
	case st["tot"] == true:
		return nil, errors.New("der Token ist tot")
	case st["anmeldung"] != nil:
		return nil, errors.New("eine Annahmestelle hat schon angemeldet -- erst ihr Urteil abwarten")
	}
	if st["gebunden"] != true {
		if dat.Z == nil {
			z, err := quantenSZufallInt(2)
			if err != nil {
				return nil, err
			}
			dat.Z = &z
			if err := speichern(); err != nil {
				return nil, fmt.Errorf("Maske nicht gespeichert, nichts eingereicht: %v", err)
			}
		}
		d := make([]int, n)
		for i := range y {
			d[i] = y[i] ^ *dat.Z
		}
		if err := quantenSBinden(ctx, netz, frist, K, inhaber, key, d); err != nil {
			return nil, err
		}
		if st, err = netz.stand(ctx, K); err != nil || st == nil {
			return nil, fmt.Errorf("Stand nach der Bindung: %v", err)
		}
		fmt.Fprintf(log, "Token %s gebunden\n", K[:12])
	}
	if dat.Z == nil {
		return nil, errors.New("BEFUND: der Token ist gebunden, aber die Datei hat keine Maske")
	}
	z := *dat.Z
	d := make([]int, n)
	for i := range y {
		d[i] = y[i] ^ z
	}
	if qsText(st, "d") != qsBitsText(d) {
		return nil, errors.New("BEFUND: die Bindung auf der Kette passt nicht zur Datei")
	}
	if st["gewaehlt"] != true {
		if err := quantenSWaehlen(ctx, netz, frist, K, inhaber, key, s^z); err != nil {
			return nil, err
		}
		fmt.Fprintf(log, "Token %s: Stelle %d gewaehlt\n", K[:12], s)
	} else if c := quantenSZahl(st["c"]); c != s^z {
		return nil, fmt.Errorf("gewaehlt ist die andere Stelle; eine Vorlage fuer Stelle %d waere eine Doppelvorlage -- nicht unterschrieben", s)
	}
	vorlage, err := quantenSUnterschreiben(key, quantenSVorlageText(K, s, x))
	if err != nil {
		return nil, err
	}
	// Ohne Blaetter (kompakt, bei n = 1.104 rund 1,3 KB): die Annahmestelle
	// holt sie bei den Validatoren und prueft sie gegen die Wurzel.
	return &QuantenSVorlage{K: K, B: s, X: dat.X, Vorlage: vorlage}, nil
}

// ------------------------------------------------------------ Annahmestelle

// quantenSAnnahmeLauf: die Annahmestelle (Schluessel a.Schluessel) nimmt die
// Vorlage an. Die Validatoren (Adressen in der Reihenfolge der Abschnitte,
// APIs in netz) nennt sie selbst.
func quantenSAnnahmeLauf(ctx context.Context, a QuantenSAusgabeAuftrag, vl QuantenSVorlage, validatoren []string, netz quantenSAbschnittNetz, log io.Writer) (*QuantenSAnnahmeErgebnis, error) {
	a.log, a.beginn = log, time.Now()
	stelle := quantenSAdresse(a.Schluessel)
	K := strings.ToLower(strings.TrimSpace(vl.K))
	b := vl.B
	if !istHash64(K) || (b != 0 && b != 1) {
		return nil, errors.New("Vorlage: K oder Stelle")
	}
	st, err := netz.stand(ctx, K)
	if err != nil {
		return nil, err
	}
	stellen := qsTexte(st, "stellen")
	switch {
	case st == nil:
		return nil, errors.New("der Token steht nicht (mehr) auf der Kette -- eingeloest, tot oder nie ausgegeben")
	case st["art"] != "s" || quantenSZahl(st["probe_art"]) != 2:
		return nil, errors.New("auf der Kette kein Token mit Stichprobe v2")
	case st["gebunden"] != true || st["gewaehlt"] != true:
		return nil, errors.New("erst nach Bindung und Wahl der Inhaberin")
	case st["tot"] == true:
		return nil, errors.New("der Token ist tot")
	case st["anmeldung"] != nil:
		return nil, errors.New("es gibt schon eine offene Anmeldung")
	case len(stellen) != 2 || stellen[b] != stelle:
		return nil, fmt.Errorf("die Vorlage ist fuer Stelle %d, das ist nicht diese Stelle", b)
	}
	n, t, v, tol, fehl := quantenSZahl(st["n"]), quantenSZahl(st["t"]), quantenSZahl(st["v"]), quantenSZahl(st["tol"]), quantenSZahl(st["fehl"])
	kn := quantenWKNote(tol, t, v)
	if n < 2 || n > quantenWNMax || v != len(validatoren) || kn == 0 || fehl < 0 {
		return nil, fmt.Errorf("die Kette nennt n = %d, t = %d, v = %d, Toleranz %d; angegeben %d Validatoren", n, t, v, tol, len(validatoren))
	}
	inhaber := qsText(st, "inhaber")
	x, err := quantenSBitsText(vl.X, n)
	if err != nil {
		return nil, fmt.Errorf("Vorlage: x: %v", err)
	}
	if !kanonischeAdresse(inhaber) || !kanonischeSignatur(vl.Vorlage) || verifyPersonalSign(quantenSVorlageText(K, b, x), vl.Vorlage, inhaber) != nil {
		return nil, errors.New("die Vorlage ist nicht von der Inhaberin auf der Kette unterschrieben")
	}
	wurzel, ok := quantenWHex32(qsText(st, "wurzel"))
	proben := qsTexte(st, "proben")
	if !ok || len(proben) != 2 {
		return nil, errors.New("Stand: Wurzel oder Proben")
	}
	probeBindung, ok := quantenWHex32(proben[b])
	if !ok {
		return nil, errors.New("Stand: Proben")
	}
	var blaetter [][32]byte
	if len(vl.Blaetter) == 0 {
		// Kompakte Vorlage: die Blaetter von den Validatoren selbst.
		if blaetter, err = quantenSBlaetterHolen(ctx, netz, K, n, v, validatoren); err != nil {
			return nil, err
		}
	} else {
		if len(vl.Blaetter) != n {
			return nil, fmt.Errorf("Vorlage: %d Blaetter, n = %d", len(vl.Blaetter), n)
		}
		blaetter = make([][32]byte, n)
		for i, h := range vl.Blaetter {
			if blaetter[i], ok = quantenWHex32(h); !ok {
				return nil, fmt.Errorf("Vorlage: Blatt %d", i)
			}
		}
	}
	if quantenWMerkleWurzel(blaetter) != wurzel {
		return nil, errors.New("die Blaetter ergeben nicht die Wurzel auf der Kette")
	}
	d, err := quantenSBitsText(qsText(st, "d"), n)
	c := quantenSZahl(st["c"])
	if err != nil || (c != 0 && c != 1) {
		return nil, errors.New("Stand: Bindung oder Wahl")
	}
	fmt.Fprintf(log, "Token %s: Vorlage fuer Stelle %d von %s geprueft; Stichprobe von %d Validatoren nach der Finalitaet\n", K[:12], b, inhaber, v)
	probe, salze, geladen, err := quantenSStichprobeV2Laden(ctx, netz, time.Now().Add(a.Frist), K, b, n, t, v, wurzel, blaetter, kn, a.Schluessel, probeBindung)
	if err != nil {
		return nil, err
	}
	// Vorab dieselbe Rechnung wie die Kette (quantenSToken.offenlegenTeil):
	// Basis an Stelle b ist d XOR b XOR c.
	erg := &QuantenSAnnahmeErgebnis{K: K, B: b, Stelle: stelle, Inhaberin: inhaber, Stichprobe: len(probe), Vorab: "abgewiesen",
		Wurzel: hex.EncodeToString(wurzel[:]), Proben: append([]string(nil), proben...)}
	for i, ut := range geladen {
		if d[i]^b^c == ut[0] {
			erg.Treffer++
			if x[i] != ut[1] {
				erg.Fehler++
			}
		}
	}
	if quantenWUrteil(erg.Treffer, erg.Fehler, tol) == quantenWEcht {
		erg.Vorab = "besteht"
	}
	fmt.Fprintf(log, "Stichprobe geladen (%d Stellen, nach %s); vorab: %s (%d Fehler bei %d Treffern)\n", len(probe), a.quantenSNach(), erg.Vorab, erg.Fehler, erg.Treffer)
	urteil, err := quantenSAnmeldenUrteil(ctx, netz, a, K, b, stelle, a.Schluessel, x, vl.Vorlage, probe, salze, fehl)
	if err != nil {
		return erg, err
	}
	erg.Urteil = urteil
	fmt.Fprintf(log, "Urteil der Kette: %s (nach %s)\n", urteil, a.quantenSNach())
	if urteil != "frist" && (erg.Vorab == "besteht") != (urteil == "eingeloest") {
		return erg, fmt.Errorf("BEFUND: vorab %s, die Kette urteilt %s", erg.Vorab, urteil)
	}
	if urteil != "eingeloest" {
		return erg, fmt.Errorf("nicht eingeloest (%s)", urteil)
	}
	return erg, nil
}

// ------------------------------------------------------------ Dateien

// quantenSJSONLesen: hoechstens quantenSRollenDateiMax, keine unbekannten
// Felder, nichts danach.
func quantenSJSONLesen(r io.Reader, was string, ziel interface{}) error {
	roh, err := io.ReadAll(io.LimitReader(r, quantenSRollenDateiMax+1))
	if err != nil || len(roh) > quantenSRollenDateiMax {
		return fmt.Errorf("%s: unlesbar oder zu gross", was)
	}
	dec := json.NewDecoder(bytes.NewReader(roh))
	dec.DisallowUnknownFields()
	if err := dec.Decode(ziel); err != nil {
		return fmt.Errorf("%s: kein passendes JSON", was) // ohne Inhalt im Text
	}
	if dec.More() {
		return fmt.Errorf("%s: mehr als ein JSON-Wert", was)
	}
	return nil
}

// quantenSInhaberinLesen: nur eine gewoehnliche Datei (kein Verweis), nur fuer
// die Eigentuemerin lesbar (0600).
func quantenSInhaberinLesen(pfad string) (*QuantenSInhaberinDatei, error) {
	fi, err := os.Lstat(pfad)
	if err != nil {
		return nil, fmt.Errorf("Datei der Inhaberin: %v", err)
	}
	if !fi.Mode().IsRegular() { // auch kein FIFO, an dem Open haengen bliebe
		return nil, errors.New("Datei der Inhaberin: keine gewoehnliche Datei (Verweis?)")
	}
	f, err := os.Open(pfad)
	if err != nil {
		return nil, fmt.Errorf("Datei der Inhaberin: %v", err)
	}
	defer f.Close()
	if err := quantenSInhaberinDateiPasst(f, pfad); err != nil {
		return nil, err
	}
	var d QuantenSInhaberinDatei
	if err := quantenSJSONLesen(f, "Datei der Inhaberin", &d); err != nil {
		return nil, err
	}
	if _, err := d.schluessel(); err != nil {
		return nil, err
	}
	if err := d.Bestellung.pruefen(d.V); err != nil {
		return nil, err
	}
	return &d, nil
}

// quantenSInhaberinDateiPasst: die GEOEFFNETE Datei ist genau die, die jetzt
// unter pfad liegt -- eine gewoehnliche Datei, kein Verweis, auch nicht
// zwischen Pruefen und Oeffnen vertauscht -- und nur fuer die Eigentuemerin
// lesbar.
func quantenSInhaberinDateiPasst(f *os.File, pfad string) error {
	fi, err := f.Stat()
	li, lerr := os.Lstat(pfad)
	if err != nil || lerr != nil || !li.Mode().IsRegular() || !fi.Mode().IsRegular() || !os.SameFile(fi, li) {
		return errors.New("Datei der Inhaberin: keine gewoehnliche Datei (Verweis?)")
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return errors.New("Datei der Inhaberin: fuer andere lesbar -- erst chmod 600")
	}
	return nil
}

// quantenSInhaberinSchreiben: neu = die Datei darf es noch nicht geben;
// sonst atomar ersetzen (neue Datei 0600 daneben, umbenennen).
func quantenSInhaberinSchreiben(pfad string, d *QuantenSInhaberinDatei, neu bool) error {
	roh, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if neu {
		f, err := os.OpenFile(pfad, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("Datei der Inhaberin: %v", err)
		}
		if _, err := f.Write(roh); err != nil {
			f.Close()
			return err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}
	if fi, err := os.Lstat(pfad); err != nil || !fi.Mode().IsRegular() {
		return errors.New("Datei der Inhaberin: keine gewoehnliche Datei (Verweis?)")
	}
	f, err := os.CreateTemp(filepath.Dir(pfad), ".inhaberin-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // nach dem Umbenennen gibt es tmp nicht mehr
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(roh); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, pfad)
}

// ------------------------------------------------------------ Befehle

// QuantenSStellenLesen: "0xadr=04schluessel,0xadr2=04schluessel2" (genau zwei).
func QuantenSStellenLesen(roh string) ([2]QuantenSStelle, error) {
	var out [2]QuantenSStelle
	teile := strings.Split(strings.TrimSpace(roh), ",")
	if len(teile) != 2 {
		return out, errors.New("Annahmestellen: genau zwei, Form adresse=schluessel,adresse=schluessel")
	}
	for i, t := range teile {
		a, s, ok := strings.Cut(strings.TrimSpace(t), "=")
		if !ok {
			return out, errors.New("Annahmestellen: Form adresse=schluessel")
		}
		out[i] = QuantenSStelle{Adresse: a, Schluessel: s}
	}
	return out, nil
}

// QuantenSAdressenLesen: "0xadr,0xadr2" -- die Validatoren, denen die
// Inhaberin vertraut, in der Reihenfolge der Abschnitte.
func QuantenSAdressenLesen(roh string) ([]string, error) {
	roh = strings.TrimSpace(roh)
	if roh == "" {
		return nil, errors.New("Validatoren: mindestens einer (0xadresse,0xadresse)")
	}
	teile := strings.Split(roh, ",")
	if len(teile) > quantenWVMax {
		return nil, fmt.Errorf("Validatoren: hoechstens %d", quantenWVMax)
	}
	gesehen := map[string]bool{}
	var out []string
	for i, t := range teile {
		a := strings.ToLower(strings.TrimSpace(t))
		if !kanonischeAdresse(a) || gesehen[a] {
			return nil, fmt.Errorf("Validator %d: Adresse nicht kanonisch oder doppelt", i+1)
		}
		gesehen[a] = true
		out = append(out, a)
	}
	return out, nil
}

// QuantenSInhaberinNeu: `aequitasd quanten-s-inhaberin neu`. Legt die Datei
// an (0600, es darf sie noch nicht geben) und schreibt die Bestellung auf aus.
// Mit validatoren (adresse=api, in der Reihenfolge der Abschnitte;
// --verschluesseln) holt sie bei jedem den oeffentlichen Schluessel, prueft
// ihn gegen die Adresse und verschluesselt y seines Abschnitts an ihn; v ist
// dann die Zahl der Validatoren.
func QuantenSInhaberinNeu(ctx context.Context, pfad string, n, v int, stellen [2]QuantenSStelle, validatoren []QuantenSValidator, aus io.Writer) error {
	var schl []QuantenSStelle
	if len(validatoren) > 0 {
		if v != len(validatoren) {
			return fmt.Errorf("v = %d, aber %d Validatoren angegeben", v, len(validatoren))
		}
		adressen, netze, err := quantenSValidatorenPruefen(validatoren, 1)
		if err != nil {
			return err
		}
		netz := &quantenSMehrNetz{validatoren: netze}
		for x := 1; x <= v; x++ {
			pub, err := netz.schluessel(ctx, x)
			if err != nil {
				return fmt.Errorf("Validator %d: Schluessel: %w", x, err)
			}
			// Gegen die Adresse geprueft in quantenSBasenVerschluesseln.
			schl = append(schl, QuantenSStelle{Adresse: adressen[x-1], Schluessel: pub})
		}
	}
	d, err := quantenSInhaberinNeu(n, v, stellen, schl)
	if err != nil {
		return err
	}
	if err := quantenSInhaberinSchreiben(pfad, d, true); err != nil {
		return err
	}
	enc := json.NewEncoder(aus)
	enc.SetIndent("", "  ")
	return enc.Encode(d.Bestellung)
}

// QuantenSInhaberinAnnehmen: `aequitasd quanten-s-inhaberin annehmen` < Paket.
func QuantenSInhaberinAnnehmen(ctx context.Context, apiBasis, pfad string, validatoren []string, paket io.Reader, log io.Writer) error {
	h, err := neuerQuantenSHTTP(apiBasis)
	if err != nil {
		return err
	}
	d, err := quantenSInhaberinLesen(pfad)
	if err != nil {
		return err
	}
	var p QuantenSPaket
	if err := quantenSJSONLesen(paket, "Paket", &p); err != nil {
		return err
	}
	if err := quantenSInhaberinAnnehmen(ctx, h, d, validatoren, p, log); err != nil {
		return err
	}
	return quantenSInhaberinSchreiben(pfad, d, false)
}

// QuantenSInhaberinAbholen: `aequitasd quanten-s-inhaberin abholen --k K
// --validatoren adr=api,...` -- ohne Paket des Ausgebers: Stand von der
// Kette, jeder Abschnitt direkt vom Validator, dann dieselbe Pruefung.
func QuantenSInhaberinAbholen(ctx context.Context, apiBasis, pfad, K string, validatoren []QuantenSValidator, log io.Writer) error {
	eigen, err := neuerQuantenSHTTP(apiBasis)
	if err != nil {
		return err
	}
	if len(validatoren) == 0 {
		return errors.New("Validatoren: adresse=api,... (bei wem die Abschnitte liegen)")
	}
	adressen, netze, err := quantenSValidatorenPruefen(validatoren, 1)
	if err != nil {
		return err
	}
	d, err := quantenSInhaberinLesen(pfad)
	if err != nil {
		return err
	}
	netz := &quantenSMehrNetz{quantenSHTTP: eigen, validatoren: netze}
	p, err := quantenSPaketVonKette(ctx, netz, K, adressen)
	if err != nil {
		return err
	}
	if err := quantenSInhaberinAnnehmen(ctx, netz, d, adressen, p, log); err != nil {
		return err
	}
	return quantenSInhaberinSchreiben(pfad, d, false)
}

// QuantenSInhaberinEinloesen: `aequitasd quanten-s-inhaberin einloesen
// --stelle b`; die Vorlage fuer die Annahmestelle auf aus.
func QuantenSInhaberinEinloesen(ctx context.Context, apiBasis, pfad string, s int, frist time.Duration, aus, log io.Writer) error {
	if frist <= 0 || frist > time.Hour {
		return errors.New("Frist: 1 s bis 1 h")
	}
	h, err := neuerQuantenSHTTP(apiBasis)
	if err != nil {
		return err
	}
	d, err := quantenSInhaberinLesen(pfad)
	if err != nil {
		return err
	}
	vl, err := quantenSInhaberinEinloesen(ctx, h, frist, d, func() error { return quantenSInhaberinSchreiben(pfad, d, false) }, s, log)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(aus)
	enc.SetIndent("", "  ")
	return enc.Encode(vl)
}

// QuantenSAnnahme: `aequitasd quanten-s-annahme` < Vorlage. Der Schluessel
// ist der der Annahmestelle; ohne Validatorenliste ist der eigene Knoten der
// einzige Validator.
func QuantenSAnnahme(ctx context.Context, apiBasis, schluesselHex string, frist, nachweisWarten time.Duration, validatoren []QuantenSValidator, vorlage io.Reader, aus, log io.Writer) error {
	if nachweisWarten < 0 || nachweisWarten > time.Hour {
		return errors.New("Nachweis abwarten: 0 bis 1 h")
	}
	if frist <= 0 || frist > time.Hour {
		return errors.New("Frist: 1 s bis 1 h")
	}
	eigen, err := neuerQuantenSHTTP(apiBasis)
	if err != nil {
		return err
	}
	key, err := crypto.HexToECDSA(quantenSSchluesselText(schluesselHex))
	if err != nil {
		return errors.New("Schluessel der Annahmestelle unlesbar") // ohne den Schluessel im Text
	}
	if len(validatoren) == 0 {
		return errors.New("Validatoren: adresse=api,... (welchen Validatoren die Stelle die Oeffnung abfragt)")
	}
	adressen, netze, err := quantenSValidatorenPruefen(validatoren, 1)
	if err != nil {
		return err
	}
	var vl QuantenSVorlage
	if err := quantenSJSONLesen(vorlage, "Vorlage", &vl); err != nil {
		return err
	}
	a := QuantenSAusgabeAuftrag{Schluessel: key, Frist: frist}
	netz := &quantenSMehrNetz{quantenSHTTP: eigen, validatoren: netze}
	erg, err := quantenSAnnahmeLauf(ctx, a, vl, adressen, netz, log)
	if err == nil && erg != nil && nachweisWarten > 0 {
		// Nach dem Ende: die Nachweise aller Validatoren, gegen Wurzel und
		// Proben, die vor dem Anmelden auf der Kette standen. Ein Fehler hier
		// macht das Einloesen nicht ungeschehen -- er steht im Ergebnis und
		// kommt als eigener Fehler (ErrQuantenSNachweis) zurueck.
		var nerr error
		if erg.Nachweis, nerr = quantenSNachweisAbwarten(ctx, netz, erg.K, adressen, erg.Wurzel, erg.Proben, time.Now().Add(nachweisWarten), log); nerr != nil {
			erg.NachweisFehler = nerr.Error()
			err = fmt.Errorf("%w: %v", ErrQuantenSNachweis, nerr)
		}
	}
	if erg != nil {
		enc := json.NewEncoder(aus)
		enc.SetIndent("", "  ")
		enc.Encode(erg)
	}
	return err
}

// QuantenSAusgabeFuerInhaberin: `aequitasd quanten-s-ausgabe --fuer-inhaberin`
// < Bestellung; das Paket auf aus. Nur mit Stichprobe v2 (auf der Kette
// aktiv, vor jedem Auftrag gefragt).
func QuantenSAusgabeFuerInhaberin(ctx context.Context, apiBasis, schluesselHex string, deckung int64, frist time.Duration, validatoren []QuantenSValidator, t, tol int,
	messenWarten time.Duration, bestellung io.Reader, aus, log io.Writer) error {
	if quantenWKNote(tol, 1, 1) == 0 {
		return fmt.Errorf("Toleranz %d %%: die Kette kennt nur %v", tol, quantenSToleranzen())
	}
	if messenWarten < 0 || messenWarten > quantenSMessenWartenMax {
		return fmt.Errorf("Messen abwarten: 0 bis %s", quantenSMessenWartenMax)
	}
	if frist <= 0 || frist > time.Hour {
		return errors.New("Frist: 1 s bis 1 h")
	}
	eigen, err := neuerQuantenSHTTP(apiBasis)
	if err != nil {
		return err
	}
	key, err := crypto.HexToECDSA(quantenSSchluesselText(schluesselHex))
	if err != nil {
		return errors.New("Knotenschluessel unlesbar") // ohne den Schluessel im Text
	}
	if len(validatoren) == 0 {
		validatoren, t = []QuantenSValidator{{Adresse: quantenSAdresse(key), API: apiBasis}}, 1
	}
	adressen, netze, err := quantenSValidatorenPruefen(validatoren, t)
	if err != nil {
		return err
	}
	var best QuantenSBestellung
	if err := quantenSJSONLesen(bestellung, "Bestellung", &best); err != nil {
		return err
	}
	if err := best.pruefen(len(adressen)); err != nil {
		return err
	}
	if err := quantenSProbeV2AufKette(ctx, eigen, time.Now().Unix()); err != nil {
		return err
	}
	auftrag := QuantenSAusgabeAuftrag{Schluessel: key, Deckung: deckung, Tol: tol, Frist: frist, ProbeV2: true}
	netz := &quantenSMehrNetz{quantenSHTTP: eigen, validatoren: netze}
	if messenWarten > 0 {
		if err := quantenSMessungenAbwarten(ctx, key, best.basenFuerRunde(), adressen, netz, messenWarten, quantenSVormerkTakt,
			func() int64 { return time.Now().Unix() }, log); err != nil {
			return err
		}
	}
	p, err := quantenSAusgabeFuerInhaberin(ctx, auftrag, best, adressen, t, netz, log)
	if p != nil {
		enc := json.NewEncoder(aus)
		enc.SetIndent("", "  ")
		enc.Encode(p)
	}
	return err
}
