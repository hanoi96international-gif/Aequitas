package keeper

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/crypto"
)

// LEITER-SEITE DES CHECKPOINT-VORSCHLAGS (OFFEN 10, Stufe C, Baustein 5,
// Rest; docs/CHECKPOINT_FAHRPLAN.md).
//
// Offene Punkte aus der Pruefung der Mitglieds-Seite (#347), hier geloest:
//   - VORSCHLAG VOM LEITER UNTERSCHRIEBEN: sonst verbraucht jeder, der die
//     Mitglieds-Funktion erreicht, den Slot zuerst. Ein Mitglied nimmt nur
//     einen Vorschlag an, den der Leiter seiner eigenen Sicht (Adresse und
//     Amtszeit aus der Leitung) kanonisch unterschrieben hat, und nur, wenn
//     dieser Leiter zur Blockzeit selbst Komiteemitglied ist.
//   - CHECKPOINTS BILDEN EINE KETTE: kennt das Mitglied einen letzten
//     zertifizierten Checkpoint, muss der vorgeschlagene Block juenger sein
//     und ihn in seiner Vergangenheit haben (vorfahrSuchen). Nicht pruefbar
//     (Luecke, Grenze, Lesefehler) heisst: keine Unterschrift.
//   - SLOT-VERBRAUCH: ein boeswilliger Leiter kann jedem Mitglied einen
//     anderen Block schicken; dann bekommt keiner das Quorum. Zwei
//     Vorschlaege desselben Leiters fuer denselben Slot mit verschiedenem
//     Block sind ein Beweis (checkpointVorschlagDoppelt); unabhaengig davon
//     meldet checkpointLeiterStockt nach checkpointLeerSlots Slots ohne
//     Zertifikat, dass die Leitung wechseln soll.
//   - SAMMELN: der Leiter nimmt je Mitglied hoechstens eine Unterschrift an,
//     prueft jede beim Eingang (Recovery), speichert nur Mitglieder -- die
//     Menge ist durch das Komitee begrenzt. Bei Quorum: Zertifikat, das
//     pruefeZertifikat selbst noch einmal besteht.
//
// Schlafend: nirgends angeschlossen. Der Aufrufer haelt dag.mu nicht.

const (
	checkpointVorschlagDomaene = "Aequitas Checkpoint-Vorschlag v1"
	// checkpointLeerSlots: so viele Slots ohne Zertifikat (ueber das
	// hoechste erlaubte Alter hinaus), dann soll die Leitung wechseln.
	checkpointLeerSlots int64 = 5
)

// CheckpointVorschlag: der vom Leiter unterschriebene Vorschlag.
type CheckpointVorschlag struct {
	KettenCheckpoint
	Leiter string `json:"leiter"`
	Term   uint64 `json:"term"`
	Sig    string `json:"sig"`
}

// vorschlagHash: was der Leiter unterschreibt -- eigene Domaene, damit eine
// Vorschlags-Unterschrift nie als Checkpoint-Unterschrift gilt und umgekehrt.
func (v CheckpointVorschlag) vorschlagHash() []byte {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%s|%s",
		checkpointVorschlagDomaene, checkpointChainID, v.Term, v.Leiter,
		hex.EncodeToString(v.KettenCheckpoint.signierHash()))))
	return h[:]
}

// checkpointVorschlagen: der Vorschlag dieses Leiters fuer c in Amtszeit term.
func checkpointVorschlagen(c KettenCheckpoint, term uint64, key *ecdsa.PrivateKey) (CheckpointVorschlag, error) {
	if err := c.gueltig(); err != nil {
		return CheckpointVorschlag{}, err
	}
	v := CheckpointVorschlag{
		KettenCheckpoint: c,
		Leiter:           strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex()),
		Term:             term,
	}
	sig, err := crypto.Sign(v.vorschlagHash(), key)
	if err != nil {
		return CheckpointVorschlag{}, err
	}
	v.Sig = hex.EncodeToString(sig)
	return v, nil
}

// checkpointVorschlagErstellen: Leiter-Seite. Der Vorschlag fuer den hier
// nachgespielten Block hash in Amtszeit term, und der Sammler fuer die
// Unterschriften. Wie bei den Mitgliedern: je Slot hoechstens ein Block,
// dauerhaft gemerkt (checkpoint_vorschlaege), BEVOR der Vorschlag
// herausgeht, und nur Bloecke nach dem Start dieses Prozesses -- ein
// ehrlicher Leiter liefert so nie einen Beweis gegen sich
// (checkpointVorschlagDoppelt), auch nicht nach einem Neustart.
func (dag *BlockDAG) checkpointVorschlagErstellen(hash string, term uint64, jetzt int64) (CheckpointVorschlag, *checkpointSammler, error) {
	nein := func(f string, a ...interface{}) (CheckpointVorschlag, *checkpointSammler, error) {
		return CheckpointVorschlag{}, nil, fmt.Errorf(f, a...)
	}
	if dag.signingKey == nil || dag.selfProposer == "" {
		return nein("kein Signierschluessel")
	}
	hoehe, zeit, da := dag.nachgespielterBlockKopf(hash)
	if !da {
		return nein("Block %s hier nicht nachgespielt", kurzAdresse(hash))
	}
	if !erzeugerSchnittAktiv(zeit) {
		return nein("vor erzeugerSchnittAb")
	}
	if zeit <= checkpointProzessStart.Load() {
		return nein("Block vor dem Start dieses Prozesses")
	}
	if alter := jetzt - zeit; alter < zeitstempelRueckToleranz || alter > checkpointHoechstensAlt {
		return nein("Block %d s alt -- vorgeschlagen wird zwischen %d und %d s", alter, zeitstempelRueckToleranz, checkpointHoechstensAlt)
	}
	if dag.state == nil {
		return nein("kein Zustand")
	}
	komitee, err := checkpointKomitee(dag.state.erzeugerRegister.Load(), zeit, dag.festeErzeugerListe())
	if err != nil {
		return nein("Komitee nicht bestimmbar: %v", err)
	}
	if !komitee[dag.selfProposer] {
		return nein("Leiter nicht im Komitee zur Blockzeit")
	}
	c := neuerCheckpoint(hoehe, hash, zeit, komitee)
	s, err := neuerCheckpointSammler(c, komitee)
	if err != nil {
		return nein("%v", err)
	}
	if dag.state.db == nil {
		return nein("keine Datenbank -- ohne dauerhaften Merker kein Vorschlag")
	}
	if err := dag.checkpointSlotMerken(checkpointMerkerVorschlag, c); err != nil {
		return CheckpointVorschlag{}, nil, err
	}
	v, err := checkpointVorschlagen(c, term, dag.signingKey)
	if err != nil {
		return CheckpointVorschlag{}, nil, err
	}
	return v, s, nil
}

// vorschlagUnterschriftGueltig: kanonisch und vom genannten Leiter.
func vorschlagUnterschriftGueltig(v CheckpointVorschlag) error {
	if !istKanonischeAdresse(v.Leiter) {
		return fmt.Errorf("Leiter %q nicht kanonisch", kurzAdresse(v.Leiter))
	}
	sig, ok := kanonischeSig(v.Sig)
	if !ok {
		return fmt.Errorf("Vorschlags-Unterschrift nicht kanonisch")
	}
	pub, err := crypto.SigToPub(v.vorschlagHash(), sig)
	if err != nil {
		return fmt.Errorf("Vorschlags-Unterschrift nicht pruefbar")
	}
	if strings.ToLower(crypto.PubkeyToAddress(*pub).Hex()) != v.Leiter {
		return fmt.Errorf("Vorschlags-Unterschrift passt nicht zu %s", kurzAdresse(v.Leiter))
	}
	return nil
}

// pruefeVorschlag: billig zuerst (Format, Leiter und Amtszeit der eigenen
// Sicht, Mitgliedschaft), dann eine Recovery.
func pruefeVorschlag(v CheckpointVorschlag, leiter string, term uint64, komitee map[string]bool) error {
	if err := v.gueltig(); err != nil {
		return fmt.Errorf("Vorschlag ungueltig: %v", err)
	}
	if leiter == "" || v.Leiter != leiter {
		return fmt.Errorf("Vorschlag von %s, Leiter ist %s", kurzAdresse(v.Leiter), kurzAdresse(leiter))
	}
	if v.Term != term {
		return fmt.Errorf("Vorschlag aus Amtszeit %d, aktuell %d", v.Term, term)
	}
	if !komitee[v.Leiter] {
		return fmt.Errorf("Leiter %s ist zur Blockzeit nicht im Komitee", kurzAdresse(v.Leiter))
	}
	return vorschlagUnterschriftGueltig(v)
}

// checkpointVorschlagAnnehmen: Mitglieds-Seite fuer einen Vorschlag des
// Leiters. leiter/term: die eigene Sicht der Leitung. letzter: der juengste
// zertifizierte Checkpoint, den dieses Mitglied kennt (nil: keiner). lies:
// Koepfe fuer die Vorfahren-Suche.
//
// Reihenfolge (Pruefung des Bausteins, MITTEL 1/2): erst alles ohne
// Rechenaufwand (Format, Leiter und Amtszeit der eigenen Sicht, Stichtag,
// Startsperre, Alter, im Speicher nachgespielt), dann das Komitee, dann die
// Recovery, zuletzt die Vorfahren-Suche -- deren Urteil wird je (Block,
// letzter Checkpoint) begrenzt gemerkt, ein wiederholter Vorschlag sucht
// nicht noch einmal.
func (dag *BlockDAG) checkpointVorschlagAnnehmen(v CheckpointVorschlag, leiter string, term uint64, letzter *KettenCheckpoint, lies kopfLeser, jetzt int64) (CheckpointUnterschrift, error) {
	nein := func(f string, a ...interface{}) (CheckpointUnterschrift, error) {
		return CheckpointUnterschrift{}, fmt.Errorf(f, a...)
	}
	if err := v.gueltig(); err != nil {
		return nein("Vorschlag ungueltig: %v", err)
	}
	if leiter == "" || v.Leiter != leiter {
		return nein("Vorschlag von %s, Leiter ist %s", kurzAdresse(v.Leiter), kurzAdresse(leiter))
	}
	if v.Term != term {
		return nein("Vorschlag aus Amtszeit %d, aktuell %d", v.Term, term)
	}
	if !erzeugerSchnittAktiv(v.Zeit) {
		return nein("vor erzeugerSchnittAb")
	}
	if v.Zeit <= checkpointProzessStart.Load() {
		return nein("Block vor dem Start dieses Prozesses")
	}
	if alter := jetzt - v.Zeit; alter < zeitstempelRueckToleranz || alter > checkpointHoechstensAlt {
		return nein("Block %d s alt -- unterschrieben wird zwischen %d und %d s", alter, zeitstempelRueckToleranz, checkpointHoechstensAlt)
	}
	if letzter != nil && (v.Zeit <= letzter.Zeit || v.Hoehe <= letzter.Hoehe) {
		return nein("Vorschlag (Zeit %d) nicht juenger als der letzte Checkpoint (Zeit %d)", v.Zeit, letzter.Zeit)
	}
	if _, _, da := dag.nachgespielterBlockKopf(v.Hash); !da {
		return nein("Block %s hier nicht nachgespielt", kurzAdresse(v.Hash))
	}
	if dag.state == nil {
		return nein("kein Zustand")
	}
	komitee, err := checkpointKomitee(dag.state.erzeugerRegister.Load(), v.Zeit, dag.festeErzeugerListe())
	if err != nil {
		return nein("Komitee nicht bestimmbar: %v", err)
	}
	if err := pruefeVorschlag(v, leiter, term, komitee); err != nil {
		return CheckpointUnterschrift{}, err
	}
	if letzter != nil {
		if lies == nil {
			return nein("Vorfahren nicht pruefbar")
		}
		gefunden, err := checkpointNachfolger(lies, v.Hash, *letzter)
		switch {
		case err != nil:
			return nein("Vorfahren nicht pruefbar: %v", err)
		case !gefunden:
			return nein("letzter Checkpoint %s nicht in der Vergangenheit des Vorschlags", kurzAdresse(letzter.Hash))
		}
	}
	return dag.checkpointMitunterschreiben(v.KettenCheckpoint, jetzt)
}

// checkpointNachfolgerUrteile: gemerkte Urteile der Vorfahren-Suche. Das
// Urteil haengt nur an Hashes (die Eltern stehen im Blockhash) -- gemerkt
// wird nur, was bei vollstaendigen Daten feststand. Begrenzt: bei
// checkpointNachfolgerGrenze Eintraegen wird geleert.
var (
	checkpointNachfolgerMu      sync.Mutex
	checkpointNachfolgerUrteile = map[[2]string]bool{}
)

const checkpointNachfolgerGrenze = 256

var errCheckpointLueckeVorfahren = errors.New("Vorfahren unvollstaendig")

// checkpointNachfolger: liegt letzter in der Vergangenheit von hash?
// Fehler: nicht pruefbar (Lesefehler, Luecke, Grenze) -- heisst fuer den
// Aufrufer keine Unterschrift und wird nicht gemerkt.
func checkpointNachfolger(lies kopfLeser, hash string, letzter KettenCheckpoint) (bool, error) {
	schluessel := [2]string{hash, letzter.Hash}
	checkpointNachfolgerMu.Lock()
	urteil, da := checkpointNachfolgerUrteile[schluessel]
	checkpointNachfolgerMu.Unlock()
	if da {
		return urteil, nil
	}
	_, gefunden, vollstaendig, err := vorfahrSuchen(lies, []string{hash}, letzter.Hash, letzter.Zeit)
	switch {
	case err != nil: // auch die Grenze: nicht gemerkt, keine Unterschrift
		return false, err
	case !vollstaendig && !gefunden:
		return false, errCheckpointLueckeVorfahren
	}
	checkpointNachfolgerMu.Lock()
	if len(checkpointNachfolgerUrteile) >= checkpointNachfolgerGrenze {
		checkpointNachfolgerUrteile = map[[2]string]bool{}
	}
	checkpointNachfolgerUrteile[schluessel] = gefunden
	checkpointNachfolgerMu.Unlock()
	return gefunden, nil
}

// checkpointVorschlagDoppelt: zwei gueltig unterschriebene Vorschlaege
// desselben Leiters in derselben Amtszeit fuer denselben Slot mit
// verschiedenem Block -- Beweis fuer Slot-Verbrauch. Verschiedene Amtszeiten
// zaehlen nicht (Wiederwahl). Ein ehrlicher Leiter erzeugt keinen solchen
// Beweis: checkpointVorschlagErstellen merkt den Slot dauerhaft vor dem
// Versand. Strafbar erst, wenn die Leiter-Seite angeschlossen ist.
func checkpointVorschlagDoppelt(a, b CheckpointVorschlag) bool {
	if a.Leiter != b.Leiter || a.Term != b.Term || a.Hash == b.Hash {
		return false
	}
	if checkpointSlot(a.Zeit) != checkpointSlot(b.Zeit) {
		return false
	}
	if a.gueltig() != nil || b.gueltig() != nil {
		return false
	}
	return vorschlagUnterschriftGueltig(a) == nil && vorschlagUnterschriftGueltig(b) == nil
}

// checkpointLeiterStockt: seit dem letzten Zertifikat (Blockzeit) sind mehr
// als checkpointLeerSlots Slots ueber das hoechste erlaubte Alter hinaus
// vergangen -- die Leitung soll wechseln. Ohne Zertifikat (letzteZeit <= 0)
// zaehlt ab start (z. B. erzeugerSchnittAb oder Amtsantritt).
func checkpointLeiterStockt(letzteZeit, start, jetzt int64) bool {
	ab := letzteZeit
	if ab <= 0 || start > ab {
		ab = start
	}
	return jetzt-ab > checkpointHoechstensAlt+checkpointLeerSlots*checkpointSlotLaenge
}

// checkpointSammler: der Leiter sammelt fuer einen Vorschlag die
// Unterschriften. Sicher fuer gleichzeitige Aufrufe.
type checkpointSammler struct {
	mu      sync.Mutex
	c       KettenCheckpoint
	komitee map[string]bool
	sigs    map[string]CheckpointUnterschrift
	fertig  *CheckpointZertifikat
}

var errSammlerFremd = errors.New("Unterschrift nicht vom Komitee")

func neuerCheckpointSammler(c KettenCheckpoint, komitee map[string]bool) (*checkpointSammler, error) {
	if err := c.gueltig(); err != nil {
		return nil, err
	}
	if len(komitee) < checkpointMindestKomitee {
		return nil, fmt.Errorf("Komitee mit %d Mitgliedern bestaetigt nichts", len(komitee))
	}
	if c.Komitee != komiteeHash(komitee) {
		return nil, fmt.Errorf("Komitee-Hash passt nicht")
	}
	k := make(map[string]bool, len(komitee))
	for a := range komitee {
		k[a] = true
	}
	return &checkpointSammler{c: c, komitee: k, sigs: map[string]CheckpointUnterschrift{}}, nil
}

// hinzu: eine Unterschrift annehmen. Liefert das Zertifikat (eine Kopie),
// sobald das Quorum erreicht ist (danach jedes Mal).
//
// Reihenfolge (Pruefung, NIEDRIG 4): Form und Mitgliedschaft, dann unter der
// Sperre "schon fertig" / "schon gezaehlt" -- erst danach die Recovery. Eine
// Wiederholung oder eine Faelschung im Namen eines gezaehlten Mitglieds
// kostet keine Recovery.
func (s *checkpointSammler) hinzu(u CheckpointUnterschrift) (*CheckpointZertifikat, error) {
	if !istKanonischeAdresse(u.Von) || !s.komitee[u.Von] {
		return nil, fmt.Errorf("%w: %s", errSammlerFremd, kurzAdresse(u.Von))
	}
	sig, ok := kanonischeSig(u.Sig)
	if !ok {
		return nil, fmt.Errorf("Unterschrift von %s nicht kanonisch", kurzAdresse(u.Von))
	}
	s.mu.Lock()
	fertig, gezaehlt := s.fertig, false
	if fertig == nil {
		_, gezaehlt = s.sigs[u.Von]
	}
	s.mu.Unlock()
	if fertig != nil {
		return zertifikatKopie(fertig), nil
	}
	if gezaehlt {
		return nil, nil
	}
	pub, err := crypto.SigToPub(s.c.signierHash(), sig)
	if err != nil || strings.ToLower(crypto.PubkeyToAddress(*pub).Hex()) != u.Von {
		return nil, fmt.Errorf("Unterschrift passt nicht zu %s", kurzAdresse(u.Von))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fertig != nil {
		return zertifikatKopie(s.fertig), nil
	}
	if _, da := s.sigs[u.Von]; !da {
		s.sigs[u.Von] = u
	}
	if len(s.sigs) < checkpointQuorum(len(s.komitee)) {
		return nil, nil
	}
	z := CheckpointZertifikat{KettenCheckpoint: s.c}
	for _, x := range s.sigs {
		z.Unterschriften = append(z.Unterschriften, x)
	}
	zertifikatOrdnen(&z)
	if err := pruefeZertifikat(z, s.komitee); err != nil {
		return nil, fmt.Errorf("gesammeltes Zertifikat ungueltig: %v", err)
	}
	s.fertig = &z
	return zertifikatKopie(s.fertig), nil
}

func zertifikatKopie(z *CheckpointZertifikat) *CheckpointZertifikat {
	k := *z
	k.Unterschriften = append([]CheckpointUnterschrift(nil), z.Unterschriften...)
	return &k
}
