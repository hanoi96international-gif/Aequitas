package keeper

// Gestaffelter Zuschuss (WP 2 aus docs/LEBENDIGKEIT_GEGEN_DEEPFAKES.md).
//
// WAS. Eine Registrierung, die der Coordinator als "gelb" einstuft
// (Lebendigkeit unsicher oder Herkunft auffaellig), bekommt die 1.000 AEQ
// nicht auf einmal: 200 sofort, 800 als Staffel ueber 30 Tage -- und die
// Staffel laeuft erst, wenn eine zweite Lebendigkeitspruefung (Transaktion
// liveness_renewal) bestanden ist. Bleibt sie aus, pausiert die Staffel; sie
// laeuft weiter, sobald sie kommt. Niemand verliert etwas ausser Zeit. Eine
// Deepfake-Farm dagegen muesste jede Kunstfigur zweimal durch eine zufaellige
// Pruefung bringen, um an den Grossteil des Zuschusses zu kommen.
//
// GELDMENGE. Die vollen 1.000 AEQ entstehen bei der Registrierung -- 200 im
// Guthaben, 800 in GrantStagedRest. Die Summe aus beiden ist der Zuschuss;
// TotalSupply = Menschen x 1.000 bleibt wahr (supply_measured.go zaehlt
// GrantStagedRest mit, siehe dort). Der Rest ist nicht ausgebbar, nicht
// uebertragbar und unterliegt keiner Demurrage: er ist ein Anspruch, kein
// Guthaben.
//
// SCHLAFEND. Nichts davon wirkt vor stagedGrantActivationUnix. Bis dahin wird
// grant_class ignoriert (voller Zuschuss wie immer), liveness_renewal und
// grant_release sind Leerlauf. Die Konstante steht auf 2100 -- WP 4 setzt das
// echte Datum, wenn die Schwellen aus echten Aufnahmen stehen (mindestens 20
// echte Registrierungen). Ein Datum im Code und nicht in einer
// Umgebungsvariablen, weil alle Knoten dieselbe Antwort geben muessen: ein
// Knoten mit anderer Einstellung liefe auseinander.
//
// DETERMINISMUS. Der Produzent entscheidet (Klasse aus der Herkunftsnotiz des
// eigenen /api/prove, Freigaben im Tagesdurchlauf nach Adressreihenfolge aus
// der Datenbank), die Bloecke tragen die Betraege, das Nachspielen wendet sie
// an -- dasselbe Modell wie ubi_distribution. Alle drei Felder stehen im
// accountLeaf (nur wenn sie ungleich null sind, damit bestehende Konten ihren
// Blattwert behalten) und im Snapshot (omitempty, aus demselben Grund).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// 2100-01-01T00:00:00Z, Platzhalter. WP 4 setzt das echte Datum -- aber
	// erst, wenn drei Dinge stehen (TestStaffel_SchlaeftBisZulassungUndStreng
	// wird sonst rot, und ihn zu aendern ist die bewusste Entscheidung):
	//   - Coordinatoren werden im Konsens zugelassen und entzogen -- heute
	//     kann jeder registrierte Mensch ohne offene Staffel bescheinigen
	//     (bescheinigungPruefen, "WER COORDINATOR SEIN KANN");
	//   - der strenge Modus beginnt spaetestens mit der Staffel;
	//   - Bindung und Bescheinigung tragen die Chain-ID.
	stagedGrantActivationUnix int64 = 4102444800

	grantKlasseSofort     = "sofort"
	grantKlasseGestaffelt = "gestaffelt"

	grantSofortAnteil  = 200.0 // AEQ sofort im Guthaben
	grantStaffelAnteil = 800.0 // AEQ als Staffel
	grantStaffelTage   = 30

	// Die Zweitpruefung zaehlt erst ab Tag 7 nach der Registrierung. Sonst
	// ist sie keine Pruefung an einem ANDEREN Tag, sondern dieselbe Sitzung
	// zweimal -- und genau die Wiederholung ueber Tage ist es, die eine
	// Deepfake-Farm teuer macht (docs/LEBENDIGKEIT_GEGEN_DEEPFAKES.md).
	erneuerungMindestTage = 7
)

// stagedGrantActivationOverride: nur fuer Tests (0 = Konstante gilt).
var stagedGrantActivationOverride atomic.Int64

func stagedGrantAktiv(blockUnix int64) bool {
	a := stagedGrantActivationUnix
	if o := stagedGrantActivationOverride.Load(); o > 0 {
		a = o
	}
	return blockUnix >= a
}

// grantStaffelTagesrate ist der Betrag, den ein Tagesdurchlauf freigibt:
// 800 / 30, auf 6 Stellen gerundet (round6 ist die Rundung der ganzen
// Kette). 30 Tage x 26,666667 = 800,00001; der letzte Tag gibt nur den Rest.
func grantStaffelTagesrate() float64 {
	return round6(grantStaffelAnteil / grantStaffelTage)
}

// grantKlasseNormalisiert bildet alles, was nicht ausdruecklich "gestaffelt"
// ist, auf "sofort" ab -- ein Tippfehler darf niemanden schlechter stellen.
func grantKlasseNormalisiert(k string) string {
	if strings.EqualFold(strings.TrimSpace(k), grantKlasseGestaffelt) {
		return grantKlasseGestaffelt
	}
	return grantKlasseSofort
}

// grantBeiRegistrierung teilt den Zuschuss auf. Vor der Aktivierung: alles
// sofort, egal was die Klasse sagt.
func grantBeiRegistrierung(klasse string, blockUnix int64) (sofort, staffel float64) {
	if !stagedGrantAktiv(blockUnix) || grantKlasseNormalisiert(klasse) != grantKlasseGestaffelt {
		return registrationGrant, 0
	}
	return grantSofortAnteil, grantStaffelAnteil
}

// hatStaffel sagt, ob ein Konto einen offenen Staffelrest traegt.
func hatStaffel(acc *AccountState) bool {
	return acc != nil && acc.GrantStagedRest > 0
}

// EIN ZEITPUNKT (05.10.2026).
//
// GrantStagedUntil und LivenessRenewedAt stehen im Blatt der StateRoot
// (accountLeaf, sobald eine Staffel existiert). Bis hierhin setzte der
// annehmende Knoten beide nach SEINER Uhr (Registrierung: time.Now(),
// Erneuerung: now) und jeder Nachspielende nach der Blockzeit -- zwei
// verschiedene Werte, also ab der Aktivierung fuer jedes gestaffelte Konto
// eine StateRoot-Abweichung zwischen Erzeuger und allen anderen. Jetzt nehmen
// beide denselben Wert aus der Transaktion:
//
//   - Registrierung: RegAt, den Annahmezeitpunkt, den der Erzeuger in die
//     Transaktion schreibt (V8 wie V7; V8 prueft ihn zusaetzlich gegen die
//     Unterschrift der Wallet, pruefeRegistrierungV8). staffelRegZeit
//     begrenzt ihn auf [Blockzeit - 1 Tag, Blockzeit + 5 min] -- auf beiden
//     Seiten, beim Erzeuger mit seiner Uhr. Ohne die untere Grenze verkuerzte
//     eine rueckdatierte Registrierung die Wartezeit bis zur Erneuerung oder
//     fiele vor die Aktivierung (voller Zuschuss sofort); ohne die obere
//     liesse ein vordatierter (bis MaxInt64) die Staffel ewig laufen oder
//     liefe ueber. Kommt eine Registrierung mehr als einen Tag nach der
//     Annahme in einen Block (Wiederanlauf), greift die untere Grenze beim
//     Erzeuger und beim Nachspielenden an verschiedenen Uhren -- dieses eine
//     Konto weicht dann um die Sekunden bis zum Block ab; die Kette haelt
//     nicht an.
//   - Erneuerung: issued_at aus der Bescheinigung (DistributionAt) als WERT.
//     Ob die Staffel gilt, entscheidet weiter die Blockzeit -- sonst haette
//     ein Erzeuger mit einem issued_at nach dem Stichtag die Erneuerung schon
//     heute setzen koennen (zweiter Sicherheitsdurchgang zu #295). issued_at
//     darf hoechstens 5 min nach dem Block liegen; fehlt er oder liegt er
//     spaeter, schaltet die Erneuerung nichts frei (nachrechnen_erneuerung.go
//     meldet sie).
//
// Vor der Aktivierung aendert sich nichts: Vor 2100 ist jede Staffel
// Leerlauf, gemessen an der Blockzeit wie bisher, und RegAt aendert an einer
// Registrierung ohne Staffel nichts.

// staffelRegZeit: der Zeitpunkt, nach dem eine Registrierung ihre Staffel
// bemisst -- beim Nachspielen mit der Blockzeit, beim Erzeuger mit seiner
// Uhr. Ohne RegAt oder mit einem RegAt nach (Bezug + 5 min) der Bezug selbst.
func staffelRegZeit(regAt, bezug int64) int64 {
	if regAt <= 0 || regAt > bezug+v8NachspielKarenz {
		return bezug
	}
	if fruehestens := bezug - 86400; regAt < fruehestens {
		return fruehestens
	}
	return regAt
}

// applyLivenessRenewalDeltaLocked wendet eine liveness_renewal an: der
// Zeitpunkt wird gesetzt, die Staffel darf laufen. Idempotent -- ein zweites
// Nachspielen derselben Erneuerung aendert nichts. Vor der Aktivierung
// Leerlauf, gemessen an der Blockzeit (bei der Annahme: jetzt). zeit ist
// der bescheinigte Zeitpunkt (issued_at) und wird der Wert -- bei der
// Annahme wie beim Nachspielen (siehe "EIN ZEITPUNKT"); fehlt er oder liegt
// er mehr als 5 min nach dem Block, schaltet die Erneuerung nichts frei.
// Caller haelt cs.mu.
func (cs *ChainState) applyLivenessRenewalDeltaLocked(ctx context.Context, address string, zeit, blockUnix int64) error {
	if !stagedGrantAktiv(blockUnix) {
		return nil
	}
	if zeit <= 0 || zeit > blockUnix+nachweisHoechstensVoraus {
		return nil
	}
	address = strings.ToLower(strings.TrimSpace(address))
	cs.ensureAccountLoadedCtx(ctx, address)
	acc, ok := cs.accounts.Get(address)
	if !ok || !acc.IsHuman {
		return fmt.Errorf("liveness_renewal fuer %s: kein registrierter Mensch", address)
	}
	if acc.LivenessRenewedAt >= zeit {
		return nil // schon erneuert (Replay derselben Transaktion)
	}
	acc.LivenessRenewedAt = zeit
	return cs.saveAccountToDBCtx(ctx, acc)
}

// applyGrantReleaseDeltaLocked bewegt einen Freigabebetrag vom Staffelrest
// ins Guthaben. Der Betrag stammt aus der Transaktion (vom Produzenten
// berechnet), wird aber auf den vorhandenen Rest gedeckelt -- ein Block kann
// nie mehr freigeben, als das Konto traegt. Caller haelt cs.mu.
func (cs *ChainState) applyGrantReleaseDeltaLocked(ctx context.Context, address string, amount float64, blockUnix int64) error {
	if !stagedGrantAktiv(blockUnix) {
		return nil
	}
	address = strings.ToLower(strings.TrimSpace(address))
	if amount <= 0 {
		return fmt.Errorf("grant_release fuer %s: Betrag %.6f", address, amount)
	}
	cs.ensureAccountLoadedCtx(ctx, address)
	acc, ok := cs.accounts.Get(address)
	if !ok || !acc.IsHuman {
		return fmt.Errorf("grant_release fuer %s: kein registrierter Mensch", address)
	}
	if acc.GrantStagedRest <= 0 {
		return nil // nichts mehr offen -- Replay einer schon angewandten Freigabe
	}
	frei := NewDecimal(amount)
	if frei > acc.GrantStagedRest {
		frei = acc.GrantStagedRest
	}
	acc.GrantStagedRest -= frei
	acc.Balance = acc.Balance.Add(frei)
	if acc.GrantStagedRest == 0 {
		acc.GrantStagedUntil = 0
	}
	touchActivityAt(acc, blockUnix)
	if err := cs.enforceWealthCapLockedCtx(ctx, acc); err != nil {
		return err
	}
	return cs.saveAccountToDBCtx(ctx, acc)
}

// grantReleasesLocked berechnet die Freigaben eines Tagesdurchlaufs: jedes
// Konto mit offenem Rest UND erfolgter Erneuerung bekommt die Tagesrate,
// gedeckelt auf den Rest. Reihenfolge nach Adresse aus der Datenbank, damit
// jeder Knoten dieselben Transaktionen in derselben Reihenfolge sieht.
// Ohne Datenbank (Tests) aus dem Speicher, sortiert. Vor der Aktivierung
// leer. Caller haelt cs.mu.
func (cs *ChainState) grantReleasesLocked(ctx context.Context, ubiAt int64) ([]Transaction, error) {
	if !stagedGrantAktiv(ubiAt) {
		return nil, nil
	}
	var adressen []string
	if cs.db != nil {
		rows, err := cs.dbExecCtx(ctx).Query(
			`SELECT lower(address) FROM chain_accounts
			 WHERE is_human = true AND COALESCE(grant_staged_rest, 0) > 0 AND COALESCE(liveness_renewed_at, 0) > 0
			 ORDER BY lower(address)`)
		if err != nil {
			return nil, fmt.Errorf("grant releases: %w", err)
		}
		for rows.Next() {
			var a string
			if rows.Scan(&a) == nil {
				adressen = append(adressen, a)
			}
		}
		rows.Close()
	} else {
		cs.accounts.Range(func(a string, acc *AccountState) bool {
			if acc.IsHuman && acc.GrantStagedRest > 0 && acc.LivenessRenewedAt > 0 {
				adressen = append(adressen, strings.ToLower(a))
			}
			return true
		})
		sort.Strings(adressen)
	}
	rate := grantStaffelTagesrate()
	var txs []Transaction
	for _, a := range adressen {
		cs.ensureAccountLoadedCtx(ctx, a)
		acc, ok := cs.accounts.Get(a)
		if !ok || acc.GrantStagedRest <= 0 {
			continue
		}
		betrag := rate
		if rest := acc.GrantStagedRest.Float(); rest < betrag {
			betrag = round6(rest)
		}
		if betrag <= 0 {
			continue
		}
		if err := cs.applyGrantReleaseDeltaLocked(ctx, a, betrag, ubiAt); err != nil {
			return nil, err
		}
		txs = append(txs, Transaction{Type: "grant_release", Wallet: a, Amount: betrag})
	}
	return txs, nil
}

// StaffelStand fuer /api/balance und die App: was offen ist, ob die Staffel
// laeuft, wann erneuert wurde.
func StaffelStand(acc *AccountState) map[string]interface{} {
	if acc == nil || (acc.GrantStagedRest == 0 && acc.LivenessRenewedAt == 0) {
		return nil
	}
	return map[string]interface{}{
		"rest_aeq":      acc.GrantStagedRest.Float(),
		"laeuft":        acc.GrantStagedRest > 0 && acc.LivenessRenewedAt > 0,
		"erneuert_am":   acc.LivenessRenewedAt,
		"bis":           acc.GrantStagedUntil,
		"tagesrate_aeq": grantStaffelTagesrate(),
		"bedeutung":     "Teil des Startzuschusses, der erst nach einer zweiten Lebendigkeitspruefung ueber 30 Tage freigegeben wird. Kein Verlust: pausiert, bis die Pruefung kommt.",
	}
}

// ------------------------------------------------------------ Herkunft

// proveHerkunftKlasse haelt je Nullifier die Klasse fest, die der
// Proof-Server aus der Coordinator-Bescheinigung gelesen hat (grantClass in
// der /prove-Antwort). Wie proveHerkunft: kurzlebig, nur im Arbeitsspeicher.
var proveHerkunftKlasse sync.Map

// grantKlasseAusHerkunft liefert die Klasse fuer einen Nullifier ("" = keine
// Notiz = sofort).
func grantKlasseAusHerkunft(nullifier string) string {
	v, ok := proveHerkunftKlasse.Load(nullifierSchluessel(nullifier))
	if !ok {
		return ""
	}
	k, _ := v.(string)
	return k
}

// merkeProveKlasse liest grantClass aus einer /prove-Antwort.
func merkeProveKlasse(respBody []byte) {
	var b struct {
		ZKNullifier string `json:"zkNullifier"`
		GrantClass  string `json:"grantClass"`
	}
	if err := json.Unmarshal(respBody, &b); err != nil || b.ZKNullifier == "" || b.GrantClass == "" {
		return
	}
	proveHerkunftKlasse.Store(nullifierSchluessel(b.ZKNullifier), grantKlasseNormalisiert(b.GrantClass))
}

// ------------------------------------------------------------ API

// handleLivenessRenewal nimmt eine Erneuerung entgegen: der Coordinator hat
// eine zweite Lebendigkeitspruefung bestanden gesehen und das mit seinem
// Ed25519-Schluessel bescheinigt (aequitas-liveness-renewal-v1|<wallet>|<issued_at>).
// Der Schluessel muss im Coordinator-Register dieses Knotens stehen (daher
// kommt die Bindung, die in die Transaktion geht); ueber die Gueltigkeit
// entscheidet bescheinigungPruefen, wie bei jedem Nachspielenden. Angenommen
// wird nur fuer Konten mit offener Staffel -- fuer alle anderen gibt es nichts
// zu erneuern. Vor der Aktivierung antwortet der Endpunkt 409.
func (a *APIServer) handleLivenessRenewal(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if !stagedGrantAktiv(time.Now().Unix()) {
		jsonError(w, "staged grant not active yet", http.StatusConflict)
		return
	}
	var req struct {
		Wallet    string `json:"wallet"`
		IssuedAt  int64  `json:"issued_at"`
		Signature string `json:"signature"`
		PublicKey string `json:"public_key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	wallet := strings.ToLower(strings.TrimSpace(req.Wallet))
	if len(wallet) != 42 || !strings.HasPrefix(wallet, "0x") {
		jsonError(w, "wallet must be a 0x address", http.StatusBadRequest)
		return
	}
	now := time.Now().Unix()
	if req.IssuedAt <= 0 || now-req.IssuedAt > 900 || req.IssuedAt-now > 60 {
		jsonError(w, "attestation expired or from the future", http.StatusBadRequest)
		return
	}
	// Die Bindung des Schluessels kommt aus dem Register dieses Knotens --
	// in die Transaktion, wo jeder andere sie selbst prueft
	// (bescheinigungPruefen). Fehlt der Eintrag oder hat er keine
	// Unterschriften (vor dem 06.10.2026 eingetragen), muss der Coordinator
	// sich einmal neu eintragen.
	bindung, ok := a.state.CoordinatorBindungLokal(req.PublicKey)
	if !ok {
		jsonError(w, "renewal attestation not signed by a registered coordinator (or its registration predates stored signatures -- register the coordinator key again)", http.StatusForbidden)
		return
	}
	tx := erneuerungsTransaktion(wallet, req.IssuedAt, req.PublicKey, req.Signature, bindung)
	if err := bescheinigungPruefen(wallet, req.IssuedAt, tx.Bescheinigung, a.state.coordinatorMenschStand); err != nil {
		jsonError(w, "invalid renewal attestation: "+err.Error(), http.StatusForbidden)
		return
	}
	a.state.mu.RLock()
	acc, ok := a.state.accounts.Get(wallet)
	offen := ok && acc.IsHuman && acc.GrantStagedRest > 0
	var ab int64
	if offen {
		ab = erneuerungFruehestens(acc)
	}
	a.state.mu.RUnlock()
	if !offen {
		jsonError(w, "no staged grant on this wallet", http.StatusConflict)
		return
	}
	// Nur hier, bei der Annahme -- nicht in applyLivenessRenewalDeltaLocked:
	// das Nachspielen bestehender Bloecke darf sich nicht aendern. Der
	// Coordinator prueft dasselbe schon vor der Aufnahme (erneuerung.py);
	// das hier haelt auch, wenn ein Coordinator es nicht tut.
	// Tag 7 gilt fuer die Bescheinigung selbst, nicht nur fuer ihre Annahme
	// -- so prueft es jeder Nachspielende (erneuerung_zu_frueh). Sonst nahme
	// dieser Knoten eine an Tag 6 ausgestellte Bescheinigung an Tag 7 an, und
	// im strengen Modus wiese jeder andere den Block ab.
	if now < ab || req.IssuedAt < ab {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "second liveness check counts from day 7 after registration", "frueh_ab": ab})
		return
	}
	// Durchs Annahme-Tor wie jeder andere Auftrag fuer ein Konto: nur der
	// Knoten, der fuer dieses Konto annimmt (Folger leiten weiter,
	// zumLeiter) -- sonst laegen Erneuerungen desselben Kontos in
	// Geschwisterbloecken.
	if err := a.state.annahmeBeginnen(wallet); err != nil {
		jsonError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer a.state.annahmeEnde()
	if err := a.state.runAtomicWithOutbox([]string{wallet}, false, func(ctx context.Context) (Transaction, error) {
		// Schon erneuert: nichts mehr in den Ausgang. Die Erneuerung schaltet
		// einmal frei (LivenessRenewedAt > 0); jede weitere waere nur eine
		// Transaktion mehr in den Bloecken -- dieselbe Anfrage binnen 15
		// Minuten noch einmal geschickt lag sonst mehrfach im Ausgang
		// (zweiter Sicherheitsdurchgang #300). Unter der Sperre geprueft, damit
		// zwei gleichzeitige Anfragen nicht beide durchkommen.
		a.state.ensureAccountLoadedCtx(ctx, wallet)
		if acc, ok := a.state.accounts.Get(wallet); ok && acc.LivenessRenewedAt > 0 {
			return Transaction{}, errSchonErneuert
		}
		if err := a.state.applyLivenessRenewalDeltaLocked(ctx, wallet, req.IssuedAt, now); err != nil {
			return Transaction{}, err
		}
		return tx, nil
	}); err != nil {
		if errors.Is(err, errSchonErneuert) {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "wallet": wallet, "schon_erneuert": true})
			return
		}
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "wallet": wallet, "renewed_at": req.IssuedAt})
}

// errSchonErneuert: das Konto ist schon erneuert -- nichts in den Ausgang.
var errSchonErneuert = errors.New("schon erneuert")

// erneuerungFruehestens: ab wann eine Erneuerung angenommen wird -- Tag 7
// nach der Registrierung. Die Registrierzeit steckt in GrantStagedUntil
// (Registrierung + 30 Tage, gesetzt in registerHumanMitKlasseLocked). Ohne
// offene Staffel 0: dann gibt es ohnehin nichts zu erneuern.
func erneuerungFruehestens(acc *AccountState) int64 {
	if acc == nil || acc.GrantStagedUntil == 0 {
		return 0
	}
	return acc.GrantStagedUntil - int64(grantStaffelTage-erneuerungMindestTage)*86400
}

const livenessRenewalDomain = "aequitas-liveness-renewal-v1"

// Lebendigkeitsbescheinigung: was der Coordinator unterschrieben hat, so wie
// es in der Transaktion steht. Der Zeitpunkt steht in DistributionAt.
type Lebendigkeitsbescheinigung struct {
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
	// DIE BINDUNG DES SCHLUESSELS, IN DER BESCHEINIGUNG SELBST (06.10.2026).
	//
	// Bis hierhin pruefte jeder Knoten den Schluessel gegen SEIN
	// Coordinator-Register (coordinator_keys) -- und das ist knotenlokal:
	// zwei Knoten mit verschiedenem Register haetten denselben Block
	// verschieden beurteilt, sobald die Staffel gilt. Jetzt traegt die
	// Bescheinigung, was eine Eintragung ausmacht: den Menschen, dem der
	// Schluessel gehoert, seine Freigabe ("Aequitas: authorize coordinator
	// <schluessel>", EIP-191) und den Besitznachweis des Schluessels
	// ("Aequitas: coordinator key for human <mensch>", Ed25519). Jeder Knoten
	// prueft sie selbst, dazu, dass der Mensch registriert ist -- dieselbe
	// Bedingung wie bei einer Eintragung, nur ohne lokale Liste.
	Mensch        string `json:"mensch,omitempty"`
	MenschSig     string `json:"mensch_sig,omitempty"`
	SchluesselSig string `json:"schluessel_sig,omitempty"`
}

// coordinatorFreigabeNachricht: was der Mensch fuer seinen Schluessel
// unterschreibt (wie bei der Eintragung, coordinator_registry.go).
func coordinatorFreigabeNachricht(schluessel string) string {
	return "Aequitas: authorize coordinator " + strings.ToLower(strings.TrimSpace(schluessel))
}

// erneuerungsTransaktion: die liveness_renewal mit der Bescheinigung --
// vorher trug sie nur Wallet und Zeitpunkt, und kein Nachspielender konnte
// pruefen, ob es die zweite Lebendigkeitspruefung gab. Jede Unterschrift in
// ihrer einen Schreibweise; bescheinigungPruefen nimmt keine andere.
func erneuerungsTransaktion(wallet string, issuedAt int64, publicHex, signatureHex string, bindung CoordinatorBindung) Transaction {
	return Transaction{Type: "liveness_renewal", Wallet: wallet, DistributionAt: issuedAt,
		Bescheinigung: &Lebendigkeitsbescheinigung{
			PublicKey:     strings.ToLower(strings.TrimSpace(publicHex)),
			Signature:     ed25519SigNormal(signatureHex),
			Mensch:        strings.ToLower(strings.TrimSpace(bindung.Mensch)),
			MenschSig:     kanonischeSignaturVersuch(bindung.MenschSig),
			SchluesselSig: ed25519SigNormal(bindung.SchluesselSig),
		}}
}

// CoordinatorMenschStand: was bescheinigungPruefen ueber den Menschen hinter
// einem Coordinator-Schluessel aus dem Kettenzustand braucht.
type CoordinatorMenschStand func(mensch string) (istMensch, staffelOffen bool)

// coordinatorMenschStand: dasselbe fuer die Annahme, ohne gehaltene Sperre.
func (cs *ChainState) coordinatorMenschStand(mensch string) (istMensch, staffelOffen bool) {
	cs.readAccount(mensch, func(acc *AccountState) {
		istMensch, staffelOffen = acc.IsHuman, acc.GrantStagedRest > 0
	})
	return
}

// bescheinigungPruefen: ist die Bescheinigung fuer wallet zu issuedAt
// gueltig? Jeder Knoten rechnet sie selbst nach, ohne Coordinator-Register:
//   - Bindung: Freigabe des Menschen (EIP-191, kanonische Schreibweise) und
//     Besitznachweis des Schluessels (Ed25519, streng -- kein Schluessel
//     kleiner Ordnung, ed25519_streng.go);
//   - der Mensch ist registriert und hat KEINE offene Staffel (stand, aus
//     dem Kettenzustand), und er ist nicht selbst der Erneuerte;
//   - die Ed25519-Unterschrift ueber Domaene|Wallet|Zeitpunkt.
//
// WER COORDINATOR SEIN KANN (Sicherheitspruefung #300, HIGH-1). Jeder
// registrierte Mensch kann einen Schluessel binden und damit Erneuerungen
// bescheinigen -- eine Zulassung im Konsens gibt es noch nicht. Ein Konto
// mit offener Staffel ist ausgeschlossen: sonst bescheinigte eine frisch
// registrierte Kunstfigur der naechsten die zweite Pruefung, und die Staffel
// koste eine Farm nichts. Das reicht NICHT gegen eine Farm, die ein altes
// oder fertig gestaffeltes Konto besitzt. Deshalb darf die Staffel erst
// aktiv werden, wenn die Zulassung (und der Entzug) der Coordinatoren im
// Konsens steht -- erzwungen in TestStaffel_SchlaeftBisZulassungUndStreng.
func bescheinigungPruefen(wallet string, issuedAt int64, b *Lebendigkeitsbescheinigung, stand CoordinatorMenschStand) error {
	if b == nil {
		return fmt.Errorf("keine Bescheinigung")
	}
	pub := b.PublicKey
	if len(pub) != 64 || !kleinHex(pub) {
		return fmt.Errorf("Schluessel ist kein Ed25519-Schluessel (64 Hex, klein)")
	}
	mensch := b.Mensch
	if !kanonischeAdresse(mensch) {
		return fmt.Errorf("Bindung fehlt: kein Mensch zum Schluessel")
	}
	if mensch == strings.ToLower(strings.TrimSpace(wallet)) {
		return fmt.Errorf("ein Coordinator bescheinigt sich nicht selbst")
	}
	if !verifyCoordinatorPossession(pub, b.SchluesselSig, mensch) {
		return fmt.Errorf("Besitznachweis des Schluessels fehlt oder ist falsch")
	}
	if !kanonischeSignatur(b.MenschSig) {
		return fmt.Errorf("Freigabe des Menschen nicht in kanonischer Schreibweise")
	}
	if err := pruefePersonalSignGemerkt(coordinatorFreigabeNachricht(pub), b.MenschSig, mensch); err != nil {
		return fmt.Errorf("Freigabe des Menschen: %v", err)
	}
	istMensch, staffelOffen := stand(mensch)
	if !istMensch {
		return fmt.Errorf("%s ist kein registrierter Mensch", kurzAdresse(mensch))
	}
	if staffelOffen {
		return fmt.Errorf("%s hat selbst eine offene Staffel -- bescheinigt keine Erneuerung", kurzAdresse(mensch))
	}
	msg := fmt.Sprintf("%s|%s|%d", livenessRenewalDomain, wallet, issuedAt)
	if !ed25519PruefenStreng(pub, b.Signature, []byte(msg)) {
		return fmt.Errorf("Bescheinigung passt nicht zu Wallet und Zeitpunkt (oder ist keine Ed25519-Unterschrift in kanonischer Schreibweise)")
	}
	return nil
}

// StaffelStandVon liest den Staffelstand eines Kontos unter der Lesesperre.
func (cs *ChainState) StaffelStandVon(address string) map[string]interface{} {
	address = strings.ToLower(strings.TrimSpace(address))
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	acc, ok := cs.accounts.Get(address)
	if !ok {
		return nil
	}
	return StaffelStand(acc)
}
