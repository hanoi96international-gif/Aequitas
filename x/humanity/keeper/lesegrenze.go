package keeper

import (
	"database/sql"
	"net/http"
	"strings"
	"unicode/utf8"
)

// Audit 14.11.2026, B7: oeffentliche Lesepfade.
//
//  1. lesenBegrenzt: eine Grenze je Absender und Endpunkt (lesenJeAbsender
//     in burstFenster) fuer die Lese-Endpunkte, die bisher keine hatten. Die
//     Karte dahinter (ipBurst) ist selbst begrenzt. Grosszuegig genug fuer
//     Website und App (Explorer: /api/status und /api/blocks alle 6 s).
//  2. kontoNurLesen: ein Konto, das nicht im Speicher steht, wird OHNE Sperre
//     direkt aus der Datenbank gelesen (cs.db, nie die laufende Transaktion)
//     und NICHT in den Speicher gelegt. Vorher nahm jede Anfrage fuer eine
//     unbekannte Adresse bis zu viermal die exklusive Sperre cs.mu, die sich
//     Blockerzeugung und Nachspielen teilen.
//  3. lesbareAdresse: nur 0x + 40 Hex (Gross/Klein egal).
const lesenJeAbsender = 600

func (a *APIServer) lesenBegrenzt(name string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !burstErlaubt("lesen:"+name+":"+erneuerungsAbsender(r), lesenJeAbsender, burstFenster) {
			writeJSONCORS(w)
			jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

func lesbareAdresse(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	return s, kanonischeAdresse(s)
}

// kontoNurLesen: der Stand eines Kontos, das nicht im Speicher steht, ohne
// Sperre und ohne es anzulegen. ok=false: nicht in der Datenbank oder nicht
// lesbar (dann wie "unbekannt" -- reine Anzeige, nie Grundlage einer Buchung).
func (cs *ChainState) kontoNurLesen(addr string) (*AccountState, bool) {
	if cs.db == nil {
		return nil, false
	}
	acc := &AccountState{Address: addr}
	var bal, tusd, lp, staffelRest float64
	var version int64
	err := cs.db.QueryRow(
		`SELECT balance, is_human, tusd_balance, lp_shares,
		        COALESCE(last_activity_at, 0), COALESCE(version, 1),
		        COALESCE(faucet_claimed, false),
		        COALESCE(demurrage_14_day_warning_shown, false),
		        COALESCE(grant_staged_rest, 0), COALESCE(grant_staged_until, 0), COALESCE(liveness_renewed_at, 0),
		        COALESCE(naechste_nonce, 0), COALESCE(naechste_auftrag_nonce, 0),
		        COALESCE(lebensnachweis_am, 0), COALESCE(staffel_einspruch_am, 0)
		 FROM chain_accounts WHERE lower(address) = $1`, addr,
	).Scan(&bal, &acc.IsHuman, &tusd, &lp, &acc.LastActivityAt, &version,
		&acc.FaucetClaimed, &acc.Demurrage14DayWarningShown, &staffelRest, &acc.GrantStagedUntil, &acc.LivenessRenewedAt,
		&acc.NaechsteNonce, &acc.NaechsteAuftragsNonce, &acc.LebensnachweisAm, &acc.StaffelEinspruchAm)
	if err != nil {
		if err != sql.ErrNoRows {
			cs.kontoUnlesbarMerken(addr) // B5: auch hier
		}
		return nil, false
	}
	acc.Balance = NewDecimal(bal)
	acc.TUsdBalance = NewDecimal(tusd)
	acc.LPShares = NewDecimal(lp)
	acc.GrantStagedRest = NewDecimal(staffelRest)
	acc.Version = version
	return acc, true
}

// demurrageStatusVon: wie GetDemurrageStatus, nur lesend (ohne den
// 14-Tage-Hinweis als gezeigt zu vermerken) -- fuer kalte Konten.
func demurrageStatusVon(acc *AccountState, jetzt int64) DemurrageStatus {
	secondsUntilStart := demurrageGracePeriodSeconds - (jetzt - acc.LastActivityAt)
	if secondsUntilStart <= 0 {
		return DemurrageStatus{Active: true}
	}
	days := float64(secondsUntilStart) / 86400
	st := DemurrageStatus{Active: false, DaysUntilStart: days}
	if days <= 7 {
		st.ShowSevenDayNotice = true
	} else if days <= 14 && !acc.Demurrage14DayWarningShown {
		st.ShowFourteenDayNotice = true
	}
	return st
}

// logKurz: Text vom Aufrufer fuer ein Log -- hoechstens n Bytes (an einer
// gueltigen UTF-8-Grenze gekuerzt); zusammen mit %q landen Zeilenumbrueche
// und Steuerzeichen nur maskiert im Log (Audit B13).
func logKurz(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + "…"
}
