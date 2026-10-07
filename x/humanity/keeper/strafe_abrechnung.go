package keeper

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DIE GELDSTRAFE WIRD SPAETER UND FUER ALLE GLEICH ABGERECHNET (ab
// registerLeserAb; zweiter Sicherheitsdurchgang zu #303, M1/M2/L3).
//
// M1. Bisher zog der Erkennende die 50 AEQ beim Erkennen ab, jeder andere
// beim Nachspielen seines Blocks. Beide fragten das Register, wer den
// Schluessel haelt -- aber zu verschiedenen Zeiten. Spielte der Erkennende
// dazwischen eine Uebergabe nach, die die anderen schon vor seinem Block
// kannten, belasteten sie verschiedene Konten.
//
// Jetzt bucht slash_equivocation keine Geldstrafe mehr. Es vermerkt Beweis,
// Zaehler und Sperre wie bisher und markiert ein zweites Vergehen als
// "strafe_offen". Die Strafe bucht eine eigene Transaktion slash_abrechnung,
// fruehestens bei Blockzeit DetectedAt + strafBeweisFrisch + erzeugerFrist.
// Sie liest nur Bindungen bis DetectedAt + strafBeweisFrisch. Eine solche
// Bindung steht in einem Block mit hoechstens einer Stunde Abstand
// (nachweisHoechstensAlt); bis zur Abrechnung hatte also jeder Knoten
// mindestens eine Stunde, sie nachzuspielen -- dieselbe Annahme wie bei der
// Erzeugerpruefung (erzeugerFrist). Wer erkennt und wer nachspielt, rechnet
// damit dasselbe.
//
// M2. "Nach der Tat hat ein anderer gebunden, also zahlt keiner" machte die
// Strafe fuer zwei, die zusammenarbeiten, freiwillig: A doppelsigniert, B
// bindet den Schluessel, niemand zahlt. Jetzt zahlt, wer den Schluessel
// innerhalb von strafBeweisFrisch nach der Tat als ERSTER ANDERER Betreiber
// gebunden hat; sonst der Halter zur Tat. Wer einen Schluessel uebernimmt,
// haftet also fuer Beweise, die hoechstens strafBeweisFrisch vor seiner
// Bindung datiert sind.
//
// FRISCHE. Das reicht nur, wenn ein spaeterer Halter keinen alten Beweis
// erfinden kann: er kennt den Schluessel und koennte zwei Koepfe mit einem
// Zeitpunkt unterschreiben, zu dem der fruehere ihn hielt. Darum muss der
// Beweis ab registerLeserAb in einem Block stehen, dessen Zeit hoechstens
// strafBeweisFrisch nach DetectedAt liegt (beweisFrischPruefen, beim
// Nachspielen und in block_tauglich.go). Erfindet B nach seiner Bindung X
// einen Beweis, gilt DetectedAt >= Blockzeit - W >= X - W, also liegt X in
// (DetectedAt, DetectedAt + W] -- B zahlt selbst (oder ein noch frueherer
// anderer Binder in diesem Fenster, siehe Grenzen).
//
// L3. Geschaltet wird jetzt nur an DetectedAt (strafeSpaeter), beim
// Erkennen wie beim Nachspielen -- vorher an max(DetectedAt, Blockzeit),
// beim Erkennen an der Uhr, und um den Stichtag herum rechneten beide
// verschieden. Rueckdatieren vor den Stichtag (LOW 2 aus #303) begrenzt
// jetzt die Frische: ab dem Stichtag hoechstens strafBeweisFrisch.
//
// GRENZEN.
//   - Kennt ein Knoten den Beweis nicht (er ist nicht in der StateRoot und
//     nicht im Snapshot), weist er die Abrechnung ab -- wie bisher beim
//     zweiten Vergehen, das er nicht zaehlen konnte.
//   - Abgerechnet wird nur vom Leiter bzw. dem einen annehmenden Knoten
//     (annahmeBeginnenLeiter). Ohne ihn bleibt die Strafe offen, Sperre und
//     Zaehler gelten trotzdem.
//   - Haben innerhalb von W nach der Tat mehrere andere gebunden, zahlt der
//     erste -- auch wenn ein spaeterer den Beweis erfunden hat. Wer einen
//     Schluessel uebernimmt, den kurz zuvor ein anderer uebernommen hat,
//     traegt dieses Risiko.
//   - Spaet eingehaengte Bloecke mit einer Bindung (L1) wirken wie bei der
//     Erzeugerpruefung rueckwirkend; das bleibt eine Bedingung vor dem
//     Stichtag (docs/VALIDATOR_REGISTER_KONSENS.md).

const (
	// strafBeweisFrisch (W): so lange nach der Tat zaehlen Bindungen fuer das
	// Strafkonto, und so alt darf ein Beweis bei seinem Block hoechstens sein.
	strafBeweisFrisch int64 = nachweisHoechstensAlt
	// strafBeweisMarge: so viel frueher hoert der Erkennende auf, einen
	// Beweis in den Ausgang zu legen -- sein Block entsteht etwas spaeter.
	strafBeweisMarge int64 = 10 * 60
	// strafAbrechnungMarge: so viel nach der Faelligkeit legt der Leiter die
	// Abrechnung in den Ausgang (Rueck-Toleranz der Blockzeit und mehr).
	strafAbrechnungMarge int64 = 5 * 60
	// strafAbrechnungJeLauf: hoechstens so viele Abrechnungen je Lauf.
	strafAbrechnungJeLauf = 10
)

// strafeSpaeter: wird die Geldstrafe dieses Beweises spaeter abgerechnet
// (slash_abrechnung)? Nur an DetectedAt -- beim Erkennen wie beim
// Nachspielen gleich.
func strafeSpaeter(detectedAt int64) bool { return registerLeserAktiv(detectedAt) }

// strafeFaelligAb: die frueheste Blockzeit der Abrechnung.
func strafeFaelligAb(detectedAt int64) int64 {
	return sattAdd(sattAdd(detectedAt, strafBeweisFrisch), erzeugerFrist)
}

// beweisFrischPruefen: ab registerLeserAb darf ein slash_equivocation nur in
// einem Block stehen, der hoechstens strafBeweisFrisch nach DetectedAt liegt.
// Davor wie bisher ohne Grenze.
func beweisFrischPruefen(detectedAt, blockZeit int64) error {
	if !registerLeserAktiv(blockZeit) {
		return nil
	}
	if blockZeit > sattAdd(detectedAt, strafBeweisFrisch) {
		return fmt.Errorf("slash_equivocation: Beweis von %d zu alt fuer Block %d (hoechstens %ds)", detectedAt, blockZeit, strafBeweisFrisch)
	}
	return nil
}

// beweisFrischFenster: die Blockzeiten, zu denen beweisFrischPruefen
// besteht -- vor dem Stichtag alle, danach bis DetectedAt + W.
func beweisFrischFenster(detectedAt int64) (bis int64) {
	return max(sattAdd(detectedAt, strafBeweisFrisch), sattAdd(registerLeserAb(), -1))
}

// strafKontoZurAbrechnung: wer die Strafe fuer die Tat zahlt. Gelesen werden
// nur Bindungen des Schluessels bis tat + strafBeweisFrisch.
//
//   - Hat innerhalb dieser Zeit ein anderer Betreiber als der Halter zur Tat
//     den Schluessel gebunden, zahlt der erste davon (bei gleichem Zeitpunkt
//     die kleinere Adresse).
//   - Sonst der Halter zur Tat: wer ihn zuletzt vor oder bei der Tat
//     gebunden hat -- auch wenn er danach selbst neu gebunden hat. Zwei mit
//     demselben letzten Zeitpunkt (umstritten) oder keiner: keine Strafe.
//   - Nur ein registrierter Mensch zahlt.
//
// Zwei Abfragen mit fester Antwortgroesse (hoechstens zwei bzw. eine Zeile).
// Ein Lesefehler ist ein Fehler, nie "keine Strafe".
func strafKontoZurAbrechnung(q sqlExecutor, signer string, tat int64) (string, error) {
	signer = strings.ToLower(signer)
	bis := sattAdd(tat, strafBeweisFrisch)
	rows, err := q.Query(`SELECT operator_wallet FROM validator_verlauf
		WHERE signing_address = $1 AND bindung_ts = (
			SELECT max(bindung_ts) FROM validator_verlauf WHERE signing_address = $1 AND bindung_ts <= $2)
		ORDER BY operator_wallet LIMIT 2`, signer, tat)
	if err != nil {
		return "", fmt.Errorf("Strafkonto aus dem Verlauf: %w", err)
	}
	var halter []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			rows.Close()
			return "", fmt.Errorf("Strafkonto aus dem Verlauf: %w", err)
		}
		halter = append(halter, strings.ToLower(b))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("Strafkonto aus dem Verlauf: %w", err)
	}
	h1, h2 := "", ""
	if len(halter) > 0 {
		h1 = halter[0]
	}
	if len(halter) > 1 {
		h2 = halter[1]
	}
	var wer string
	switch err := q.QueryRow(`SELECT operator_wallet FROM validator_verlauf
		WHERE signing_address = $1 AND bindung_ts > $2 AND bindung_ts <= $3
		  AND operator_wallet <> $4 AND operator_wallet <> $5
		ORDER BY bindung_ts, operator_wallet LIMIT 1`, signer, tat, bis, h1, h2).Scan(&wer); {
	case errors.Is(err, sql.ErrNoRows):
		if len(halter) != 1 {
			fmt.Printf("[SLASHING] ⚠ kein eindeutiger Halter fuer %s zur Tat (%d) -- keine Geldstrafe\n", signer, tat)
			return "", nil
		}
		wer = h1
	case err != nil:
		return "", fmt.Errorf("Strafkonto aus dem Verlauf: %w", err)
	default:
		wer = strings.ToLower(wer)
		fmt.Printf("[SLASHING] %s wurde innerhalb von %ds nach der Tat von %s gebunden -- er zahlt\n", signer, strafBeweisFrisch, kurzAdresse(wer))
	}
	mensch, err := istMenschIn(q, wer)
	if err != nil {
		return "", fmt.Errorf("Strafkonto aus dem Verlauf: %w", err)
	}
	if !mensch {
		fmt.Printf("[SLASHING] ⚠ %s (Strafkonto fuer %s) ist kein registrierter Mensch -- keine Geldstrafe\n", kurzAdresse(wer), signer)
		return "", nil
	}
	return wer, nil
}

// errAbrechnungSchonErledigt: die Strafe dieses Paars ist auf diesem Knoten
// schon abgerechnet.
var errAbrechnungSchonErledigt = errors.New("Strafe schon abgerechnet")

// strafeAbrechnenLocked: slash_abrechnung anwenden -- beim Nachspielen und
// beim Leiter, der sie in den Ausgang legt. Unter cs.mu, in der Transaktion
// von ctx.
//
// Ungueltig (Fehler, der Block wird abgewiesen): unvollstaendig, Beweis vor
// dem Stichtag, Blockzeit vor der Faelligkeit, Beweis unbekannt oder ohne
// offene Strafe, Lesefehler. Schon abgerechnet: errAbrechnungSchonErledigt
// (beim Nachspielen ein Duplikat, nichts zu tun).
//
// nachtragen nimmt das Strafkonto vor dem Abzug in die Ruecknahme auf
// (beim Nachspielen kontoNachtragenLocked); gibt das Strafkonto zurueck.
func (cs *ChainState) strafeAbrechnenLocked(ctx context.Context, tx *Transaction, blockZeit int64, nachtragen func(string) error, activityAt int64) (wer string, betrag float64, err error) {
	signer := strings.ToLower(strings.TrimSpace(tx.Wallet))
	a, b := tx.BlockAHash, tx.BlockBHash
	if signer == "" || a == "" || b == "" || a == b {
		return "", 0, fmt.Errorf("slash_abrechnung unvollstaendig")
	}
	if a > b {
		a, b = b, a
	}
	if !strafeSpaeter(tx.DetectedAt) {
		return "", 0, fmt.Errorf("slash_abrechnung fuer einen Beweis vor dem Stichtag (%d)", tx.DetectedAt)
	}
	if ab := strafeFaelligAb(tx.DetectedAt); blockZeit < ab {
		return "", 0, fmt.Errorf("slash_abrechnung zu frueh: Block %d, faellig ab %d", blockZeit, ab)
	}
	q := cs.dbExecCtx(ctx)
	var von string
	var erkannt int64
	var offen, erledigt bool
	switch err := q.QueryRow(`SELECT signing_address, detected_at, strafe_offen, slash_applied FROM equivocation_evidence
		WHERE block_a_hash = $1 AND block_b_hash = $2 FOR UPDATE`, a, b).Scan(&von, &erkannt, &offen, &erledigt); {
	case errors.Is(err, sql.ErrNoRows):
		return "", 0, fmt.Errorf("slash_abrechnung: Beweis %s/%s unbekannt", kurzHash(a), kurzHash(b))
	case err != nil:
		return "", 0, fmt.Errorf("slash_abrechnung: %w", err)
	}
	if strings.ToLower(von) != signer || erkannt != tx.DetectedAt {
		return "", 0, fmt.Errorf("slash_abrechnung passt nicht zum Beweis (%s um %d)", kurzAdresse(von), erkannt)
	}
	if !offen {
		return "", 0, fmt.Errorf("slash_abrechnung: fuer %s/%s ist keine Geldstrafe offen", kurzHash(a), kurzHash(b))
	}
	if erledigt {
		return "", 0, errAbrechnungSchonErledigt
	}
	wer, err = strafKontoZurAbrechnung(q, signer, tx.DetectedAt)
	if err != nil {
		return "", 0, err
	}
	if wer == "" {
		// Niemand zahlt: als abgerechnet vermerken, damit sie nicht wieder
		// in den Ausgang kommt.
		if _, err := q.Exec(`UPDATE equivocation_evidence SET slash_applied = TRUE WHERE block_a_hash = $1 AND block_b_hash = $2`, a, b); err != nil {
			return "", 0, fmt.Errorf("slash_abrechnung: %w", err)
		}
		return "", 0, nil
	}
	if nachtragen != nil {
		if err := nachtragen(wer); err != nil {
			return "", 0, err
		}
	}
	betrag, abgezogen, err := cs.strafeAbziehenLocked(ctx, signer, a, b, tx.DetectedAt, wer, activityAt)
	if err != nil {
		return "", 0, err
	}
	if !abgezogen {
		return "", 0, errAbrechnungSchonErledigt
	}
	return wer, betrag, nil
}

// ------------------------------------------------------------ Leiter

// StarteStrafAbrechnung: einmal je Minute die faelligen offenen Strafen in
// den Ausgang legen. Nur ab registerLeserAb, nur beim Leiter bzw. dem einen
// annehmenden Knoten.
func (cs *ChainState) StarteStrafAbrechnung() {
	if cs.db == nil {
		return
	}
	SafeGoroutine("strafAbrechnung", func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for range t.C {
			SafeCall("strafAbrechnung", cs.strafAbrechnungLauf)
		}
	})
}

type offeneStrafe struct {
	signer, a, b string
	erkannt      int64
}

// strafAbrechnungLauf: ein Durchlauf, hoechstens strafAbrechnungJeLauf.
func (cs *ChainState) strafAbrechnungLauf() {
	jetzt := nowUnix()
	if !registerLeserAktiv(jetzt) {
		return
	}
	// Faellig: DetectedAt + W + Frist + Marge <= jetzt.
	grenze := jetzt - strafBeweisFrisch - erzeugerFrist - strafAbrechnungMarge
	rows, err := cs.db.Query(`SELECT signing_address, block_a_hash, block_b_hash, detected_at FROM equivocation_evidence
		WHERE strafe_offen AND NOT slash_applied AND detected_at <= $1 AND detected_at >= $2
		ORDER BY detected_at LIMIT $3`, grenze, registerLeserAb(), strafAbrechnungJeLauf)
	if err != nil {
		fmt.Printf("[SLASHING] offene Strafen nicht lesbar: %v\n", err)
		return
	}
	var offen []offeneStrafe
	for rows.Next() {
		var o offeneStrafe
		if err := rows.Scan(&o.signer, &o.a, &o.b, &o.erkannt); err != nil {
			rows.Close()
			fmt.Printf("[SLASHING] offene Strafen nicht lesbar: %v\n", err)
			return
		}
		offen = append(offen, o)
	}
	rows.Close()
	for _, o := range offen {
		if err := cs.strafeAbrechnen(o); err != nil {
			fmt.Printf("[SLASHING] Abrechnung %s (%s/%s) nicht gelegt: %v\n", kurzAdresse(o.signer), kurzHash(o.a), kurzHash(o.b), err)
			if errors.Is(err, errKeinAlleinigerAnnehmer) {
				return
			}
		}
	}
}

// strafeAbrechnen: eine faellige Strafe anwenden und slash_abrechnung in den
// Ausgang legen, in einer Transaktion.
func (cs *ChainState) strafeAbrechnen(o offeneStrafe) error {
	if err := cs.annahmeBeginnenLeiter(); err != nil {
		return err
	}
	defer cs.annahmeEnde()
	vorab, err := strafKontoZurAbrechnung(cs.db, o.signer, o.erkannt)
	if err != nil {
		return err
	}
	konten := []string{ubiPoolAddr}
	if vorab != "" {
		konten = append(konten, vorab)
	}
	tx := Transaction{Type: "slash_abrechnung", Wallet: strings.ToLower(o.signer), BlockAHash: o.a, BlockBHash: o.b, DetectedAt: o.erkannt}
	var betrag float64
	err = cs.runAtomicWithOutbox(konten, false, func(ctx context.Context) (Transaction, error) {
		wer, b, err := cs.strafeAbrechnenLocked(ctx, &tx, nowUnix(), nil, 0)
		if err != nil {
			return Transaction{}, err
		}
		if wer != vorab {
			return Transaction{}, fmt.Errorf("%w: %s statt %s", errStrafkontoGeaendert, wer, vorab)
		}
		betrag = b
		return tx, nil
	})
	if errors.Is(err, errAbrechnungSchonErledigt) {
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Printf("[SLASHING] ✓ Strafe fuer %s abgerechnet: %.4f AEQ von %s (Beweis %d)\n", kurzAdresse(o.signer), betrag, kurzAdresse(vorab), o.erkannt)
	return nil
}
