package keeper

// PFAND JE PRAEPARATORIN (P2, Plan Pfaender in forschung/quantengeld/DEZENTRAL.md;
// Spezifikation forschung/quantengeld/s_pfaender.py, Regeln R-P1 bis R-P3)
//
// Ab dem Stichtag der Pfaender (quantenSPfaenderAbUnix; P3 und P4 kommen
// hinter denselben Stichtag):
//
//   - R-P1 Melden kostet quantenSPraepPfand (10 AEQ) von der Wallet in den
//     Pfandtopf (quantenWPfandAddr). Ohne Deckung: abgewiesen. Wer schon vor
//     dem Stichtag gemeldet ist (ohne Pfand), legt es mit einem neuen Melden
//     nach; der Eintrag behaelt seine Meldezeit. Ins Los (Abbild der
//     Bestellung) kommt ab dem Stichtag nur, wer ein Pfand liegen hat.
//   - R-P2 Das Pfand gehoert dem MENSCHEN: der Eintrag merkt sich den
//     Nullifier N der Wallet beim Melden. Die Rueckgabe geht an die Wallet,
//     der N DANN gehoert (nach einem Umzug die neue, nie die alte). Ein Mensch
//     hat hoechstens ein Pfand im Register (gleiches N). Ohne Nullifier
//     (Konto ohne Gesicht, kann nicht umziehen) geht es an das Konto selbst.
//   - R-P3 Zurueck (neue Art quanten_w_s_pfand_zurueck, ausloesen darf jeder,
//     ohne Unterschrift): fruehestens quantenSPraepHaltezeit (30 Tage) nach
//     der Abmeldung UND erst, wenn kein Token mehr laeuft, in dessen Los das
//     Konto steht, und die Nachweis-Frist jedes beendeten verstrichen ist
//     (P2a: Lose je Token) -- und keine offene Bestellung es gelost hat.
//     Sonst meldete sich ab, wer gerade mitgefaelscht hat, und holte das
//     Pfand vor dem Nachweis.
//
// Ein Eintrag mit Pfand wird nie aufgeraeumt (quantenSPraepAufraeumen); nach
// der Rueckgabe gilt fuer ihn der Nachlauf wie bisher.
//
// ZEIT: alle Bedingungen der Rueckgabe sind "fruehestens"; eine
// zurueckdatierte Buchzeit (quantenWBuchSpiel) macht sie nur strenger.
//
// GELD BLEIBT ERHALTEN (R-P6): Melden bucht Wallet -> Pfandtopf, Rueckgabe
// Pfandtopf -> Wallet des Menschen, je genau quantenSPraepPfand.
//
// SCHLAFEND bis quantenSPfaenderAbUnix (MaxInt64). Setzt "Los behalten"
// (P2a) voraus: ohne Lose koennte kein Knoten R-P3 pruefen.

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
)

const (
	quantenSPfandZurueckTxArt = "quanten_w_s_pfand_zurueck"

	// 10 AEQ je Praeparatorin (Betreiber, 22.11.2026).
	quantenSPraepPfand int64 = 10_000_000
	// Zurueck fruehestens so lange nach der Abmeldung (Betreiber, 22.11.2026).
	quantenSPraepHaltezeit int64 = 30 * 86400
)

// STICHTAG DER PFAENDER (Kettenregel; P2, spaeter auch P3 und P4).
// 9223372036854775807 = math.MaxInt64 = schlafend. Der Betreiber nennt Datum
// und Uhrzeit (UTC); der PR, der ihn setzt, bekommt eine eigene
// Sicherheitspruefung, und der Deploy-Waechter
// .github/neustart/quanten-s-pfaender-nicht-verpasst.sh haelt jeden Wechsel
// an, der nicht mindestens 30 Minuten in der Zukunft liegt oder einen
// laufenden Stichtag aendern wuerde.
var quantenSPfaenderAbUnix int64 = 9223372036854775807

// quantenSPfaenderAbOverride: nur fuer Tests (0 = Stichtag oben).
var quantenSPfaenderAbOverride atomic.Int64

func quantenSPfaenderAb() int64 {
	if v := quantenSPfaenderAbOverride.Load(); v != 0 {
		return v
	}
	return quantenSPfaenderAbUnix
}

// quantenSPfaenderAktiv: setzt "Los behalten" voraus (und damit Weg B).
func quantenSPfaenderAktiv(t int64) bool {
	return quantenSLosBehaltenAktiv(t) && t >= quantenSPfaenderAb()
}

// quantenSPfaenderStatus: fuer /api/status (fehlt = schlafend).
func quantenSPfaenderStatus(m map[string]interface{}) {
	if ab := quantenSPfaenderAb(); ab != math.MaxInt64 {
		m["pfaender_ab"] = ab
	}
}

func istQuantenSPfandTxArt(t string) bool { return t == quantenSPfandZurueckTxArt }

// quantenSPfandNurEigeneFelder: die Rueckgabe traegt nur K (des Eintrags);
// Pfand und Empfaenger rechnet jeder Knoten selbst. Ohne Wallet, ohne Zeit.
func quantenSPfandNurEigeneFelder(tx *Transaction, q QuantenWAngaben) bool {
	q.Pfand, q.PfandAn = 0, ""
	return reflect.DeepEqual(q, QuantenWAngaben{}) && tx.QuantenW.Index == 0 && tx.Wallet == "" && tx.DistributionAt == 0
}

// quantenSPraepPfandGueltig: Pfand und Nullifier eines Eintrags (fail-closed).
func quantenSPraepPfandGueltig(pfand int64, n string) bool {
	if pfand < 0 || pfand > quantenSPraepPfand {
		return false
	}
	return n == "" || (pfand > 0 && umzugNullifierKanonisch(n))
}

// quantenSPraepEinPfandJeMensch: hoechstens ein Eintrag mit Pfand je
// Nullifier (Lesen, fail-closed).
func quantenSPraepEinPfandJeMensch(m map[string]quantenSPraep) error {
	gesehen := map[string]bool{}
	for _, e := range m {
		if e.N == "" {
			continue
		}
		if gesehen[e.N] {
			return errors.New("quanten_w: zwei Pfaender eines Menschen")
		}
		gesehen[e.N] = true
	}
	return nil
}

// quantenSNullifierVonLocked: der Nullifier der Wallet ("" = keiner). mehrere:
// die Wallet steht bei mehr als einem (darf nicht sein; der Aufrufer weist ab).
// Fehler = Zustand nicht lesbar.
func (cs *ChainState) quantenSNullifierVonLocked(ctx context.Context, wallet string) (n string, mehrere bool, err error) {
	wallet = strings.ToLower(wallet)
	if cs.db == nil {
		cs.nullifiersMu.Lock()
		defer cs.nullifiersMu.Unlock()
		for x, w := range cs.nullifiers {
			if strings.ToLower(w) != wallet {
				continue
			}
			if n != "" {
				return "", true, nil
			}
			n = x
		}
		return n, false, nil
	}
	rows, err := cs.dbExecCtx(ctx).Query(`SELECT nullifier FROM nullifiers WHERE lower(wallet_address) = $1 LIMIT 2`, wallet)
	if err != nil {
		return "", false, err
	}
	defer rows.Close()
	var alle []string
	for rows.Next() {
		var x string
		if err := rows.Scan(&x); err != nil {
			return "", false, err
		}
		alle = append(alle, x)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	switch len(alle) {
	case 0:
		return "", false, nil
	case 1:
		return alle[0], false, nil
	}
	return "", true, nil
}

// quantenSPfandMeldenLocked: R-P1/R-P2 beim Melden (ab dem Stichtag). alt:
// der Eintrag des Kontos, falls da (dann nur Nachlegen: gemeldet, ohne
// Pfand). Gibt den neuen Eintrag und den Weg ins Pfand -- oder einen Befund.
func (cs *ChainState) quantenSPfandMeldenLocked(ctx context.Context, reg *quantenWRegister, K, wallet string, e quantenSPraep) (quantenSPraep, quantenWWeg, quantenWBescheid, error) {
	n, mehrere, err := cs.quantenSNullifierVonLocked(ctx, wallet)
	if err != nil {
		return e, quantenWWeg{}, quantenWBescheid{}, err
	}
	if mehrere || (n != "" && !umzugNullifierKanonisch(n)) {
		return e, quantenWWeg{}, quantenWBefund("quanten_s_pfand_mensch", "%s: Nullifier nicht eindeutig", kurzAdresse(wallet)), nil
	}
	if n != "" {
		for k, x := range reg.Praep {
			if k != K && x.N == n && x.Pfand > 0 {
				return e, quantenWWeg{}, quantenWBefund("quanten_s_pfand_mensch", "dieser Mensch hat schon ein Pfand (Konto %s); erst zurueckholen", kurzAdresse(x.Konto)), nil
			}
		}
	}
	if cs.quantenWMikroLocked(ctx, wallet) < quantenSPraepPfand {
		return e, quantenWWeg{}, quantenWBefund("quanten_w_deckung", "%s hat keine %d Mikro-AEQ Pfand", kurzAdresse(wallet), quantenSPraepPfand), nil
	}
	e.Pfand, e.N = quantenSPraepPfand, n
	return e, quantenWWeg{wallet, quantenWPfandAddr, quantenSPraepPfand}, quantenWBescheid{}, nil
}

// quantenSPraepGebunden: steht das Konto im Los eines laufenden Tokens, eines
// beendeten mit offener Nachweis-Frist oder einer offenen Bestellung? Ein
// toter Eintrag ohne Zeit des Endes zaehlt als gebunden (fail-closed).
func quantenSPraepGebunden(reg *quantenWRegister, konto string, zeit int64) bool {
	for k, los := range reg.Lose {
		if !slices.Contains(los, konto) {
			continue
		}
		if reg.Token[k] != nil {
			return true
		}
		en, ok := reg.Enden[k]
		if !ok || zeit <= sattAdd(en.Ende, quantenSNachweisFrist) {
			return true
		}
	}
	for _, b := range reg.Best {
		if slices.Contains(b.Los, konto) {
			return true
		}
	}
	return false
}

// quantenSPfandZurueckBescheidLocked: R-P3. Caller haelt cs.mu.
func (cs *ChainState) quantenSPfandZurueckBescheidLocked(ctx context.Context, tx *Transaction, reg *quantenWRegister, zeit int64) (quantenWBescheid, error) {
	if !quantenSPfaenderAktiv(zeit) {
		return quantenWBefund("quanten_s_pfand_inaktiv", "Pfaender sind noch nicht aktiv"), nil
	}
	q := tx.QuantenW
	e, da := reg.Praep[q.K]
	if !da || e.Pfand <= 0 {
		return quantenWBefund("quanten_s_pfand_unbekannt", "kein Pfand unter %s", q.K[:12]), nil
	}
	if e.Abgemeldet == 0 {
		return quantenWBefund("quanten_s_pfand_gemeldet", "%s ist noch gemeldet", kurzAdresse(e.Konto)), nil
	}
	if ab := sattAdd(e.Abgemeldet, quantenSPraepHaltezeit); zeit < ab {
		return quantenWBefund("quanten_s_pfand_frist", "Pfand von %s zurueck ab %d", kurzAdresse(e.Konto), ab), nil
	}
	if quantenSPraepGebunden(reg, e.Konto, zeit) {
		return quantenWBefund("quanten_s_pfand_gebunden", "%s steht im Los eines Tokens, der laeuft oder dessen Frist offen ist", kurzAdresse(e.Konto)), nil
	}
	an := e.Konto
	if e.N != "" {
		inhaber, err := cs.umzugNullifierInhaberLocked(ctx, e.N)
		if err != nil {
			return quantenWBescheid{}, err
		}
		if inhaber == "" {
			return quantenWBefund("quanten_s_pfand_empfaenger", "der Mensch hinter %s hat keine Wallet", kurzAdresse(e.Konto)), nil
		}
		an = inhaber
	}
	if !kanonischeAdresse(an) || istSystemAdresse(an) {
		return quantenWBefund("quanten_s_pfand_empfaenger", "Empfaenger %s nicht moeglich", kurzAdresse(an)), nil
	}
	neu := reg.kopie()
	pfand := e.Pfand
	e.Pfand, e.N = 0, ""
	neu.Praep[q.K] = e
	return quantenWBescheid{reg: neu, wege: []quantenWWeg{{quantenWPfandAddr, an, pfand}}, felder: QuantenWAngaben{Pfand: pfand, PfandAn: an}}, nil
}
