package keeper

// VORMUND UND LEBENSZEICHEN AUF DER KETTE (04.10.2026).
//
// # WARUM
//
// /api/set-guardian und /api/confirm-alive schrieben nur in den Zustand des
// Knotens, der die Anfrage bekam: die Tabelle guardians und den
// Aktivitaetszeitpunkt des Kontos. Keine Transaktion, kein Block. Zwei Folgen:
//
//   - Ein Lebenszeichen auf einem Knoten, der die Treuhand-Verschiebung nicht
//     erzeugt, wirkte nirgends. Der Erzeuger sah das Konto weiter als
//     untaetig und schob es nach 2,5 Jahren in die Treuhand -- obwohl der
//     Vormund die ganze Zeit bestaetigt hatte.
//   - Die Unterschriften trugen keinen Zeitpunkt: "Aequitas: confirm alive
//     0x...". Wer eine einmal gesehene Unterschrift besass, konnte sie beliebig
//     oft wieder einreichen und ein Konto auf ewig "lebendig" halten -- auch
//     das eines Verstorbenen, dessen Guthaben sonst nach der Frist in den
//     Grundeinkommenstopf zurueckfliesst.
//
// # WIE
//
// Beide Handlungen sind jetzt Transaktionen mit Auftragsnachweis
// (auftrag_nachweis.go): die Unterschrift reist im Block mit, die Nachricht
// traegt den Zeitpunkt ("... ts:<unix>"), und JEDER Knoten prueft beim
// Nachspielen Unterschrift und Zeitfenster (hoechstens eine Stunde alt,
// hoechstens fuenf Minuten voraus) selbst. Eine alte Unterschrift ist nach
// einer Stunde wertlos; innerhalb der Stunde bewirkt eine Wiederholung nichts
// Neues (der Zeitpunkt wird ohnehin auf den Block gesetzt, und den Vormund
// sperrt die Sieben-Tage-Frist).
//
//   vormund_setzen: Wallet = der Schutzbefohlene (unterschreibt),
//                   To     = der Vormund
//   lebenszeichen:  Wallet = der Schutzbefohlene,
//                   To     = der Vormund (unterschreibt)
//
// Die Regeln (beide Menschen, kein Kreis, hoechstens drei Schutzbefohlene,
// Sieben-Tage-Frist, Lebenszeichen nur vom eingetragenen Vormund) prueft der
// Annehmende vor dem Schreiben und jeder Nachspielende noch einmal
// (nachrechnenVormundLocked) -- gegen seine EIGENE Tabelle, die er aus
// denselben Bloecken aufgebaut hat. Als Zeitpunkt der Eintragung gilt der
// unterschriebene (Nachweis.Zeit), nicht die Uhr eines Knotens: so rechnen
// alle die Frist gleich.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func vormundSetzenNachricht(vormund string, zeit int64) string {
	return fmt.Sprintf("Aequitas: set guardian %s ts:%d", strings.ToLower(vormund), zeit)
}

func lebenszeichenNachricht(schuetzling string, zeit int64) string {
	return fmt.Sprintf("Aequitas: confirm alive %s ts:%d", strings.ToLower(schuetzling), zeit)
}

// vormundRegelnCtx: darf vormund jetzt (zeit) Vormund von wallet werden?
// Liest ueber ctx -- im Annehmen die Outbox-Transaktion, im Nachspielen die
// des Blocks. Erwartet cs.mu gehalten.
func (cs *ChainState) vormundRegelnCtx(ctx context.Context, wallet, vormund string, zeit int64) error {
	if wallet == vormund {
		return fmt.Errorf("wallet cannot be its own guardian")
	}
	for _, k := range []struct{ rolle, addr string }{{"wallet", wallet}, {"guardian", vormund}} {
		cs.ensureAccountLoadedCtx(ctx, k.addr)
		if acc, ok := cs.accounts.Get(k.addr); !ok || !acc.IsHuman {
			return fmt.Errorf("%s %s is not a registered human", k.rolle, k.addr)
		}
	}
	db := cs.dbExecCtx(ctx)
	var bisher int64
	switch err := db.QueryRow(`SELECT COALESCE(set_at, 0) FROM guardians WHERE lower(wallet_address) = $1`, wallet).Scan(&bisher); {
	case err == sql.ErrNoRows:
	case err != nil:
		return fmt.Errorf("guardian lookup failed: %w", err)
	}
	if bisher > 0 && zeit-bisher < guardianTimelockSeconds {
		rest := guardianTimelockSeconds - (zeit - bisher)
		return fmt.Errorf("guardian was set %d days ago — must wait 7 days before changing (%d days remaining)",
			(zeit-bisher)/86400, (rest+86399)/86400)
	}
	var schuetzlinge int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM guardians WHERE lower(guardian_address) = $1 AND lower(wallet_address) != $2`,
		vormund, wallet,
	).Scan(&schuetzlinge); err != nil {
		return fmt.Errorf("ward-count check failed: %w", err) // fail-closed
	}
	if schuetzlinge >= maxWardsPerGuardian {
		return fmt.Errorf("guardian %s already has %d wards (maximum %d)", vormund, schuetzlinge, maxWardsPerGuardian)
	}
	// Kein Kreis: die Kette der Vormuende ab vormund darf nicht zu wallet
	// zurueckfuehren. Begrenzt auf fuenf Glieder wie bisher.
	aktuell := vormund
	for tiefe := 0; tiefe < 5; tiefe++ {
		var naechster string
		err := db.QueryRow(`SELECT lower(guardian_address) FROM guardians WHERE lower(wallet_address) = $1`, aktuell).Scan(&naechster)
		if err == sql.ErrNoRows {
			break
		}
		if err != nil {
			return fmt.Errorf("guardian chain lookup failed: %w", err)
		}
		if naechster == wallet {
			return fmt.Errorf("circular guardian relationship detected (cycle depth %d)", tiefe+2)
		}
		aktuell = naechster
	}
	return nil
}

func (cs *ChainState) vormundEintragenCtx(ctx context.Context, wallet, vormund string, zeit int64) error {
	if _, err := cs.dbExecCtx(ctx).Exec(
		`INSERT INTO guardians (wallet_address, guardian_address, set_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (wallet_address) DO UPDATE SET guardian_address = $2, set_at = $3`,
		wallet, vormund, zeit,
	); err != nil {
		return fmt.Errorf("db error: %w", err)
	}
	cs.syncGuardianEscrowSlotsLockedCtx(ctx, V7_CONTRACT_ADDR, wallet)
	return nil
}

// eingetragenerVormundCtx: der Vormund von wallet in der eigenen Tabelle.
func (cs *ChainState) eingetragenerVormundCtx(ctx context.Context, wallet string) (string, error) {
	var v string
	err := cs.dbExecCtx(ctx).QueryRow(`SELECT lower(guardian_address) FROM guardians WHERE lower(wallet_address) = $1`, wallet).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// pruefeNachweisJetzt: Zeitfenster und Unterschrift schon beim Annehmen --
// dieselbe Pruefung, die jeder Nachspielende gegen die Blockzeit macht.
func pruefeNachweisJetzt(tx *Transaction) error {
	return pruefeAuftragsNachweis(tx, nowUnix())
}

// VormundSetzen: wallet setzt vormund, mit Unterschrift und Zeitpunkt.
func (cs *ChainState) VormundSetzen(wallet, vormund string, n *Auftragsnachweis) error {
	wallet, vormund = strings.ToLower(strings.TrimSpace(wallet)), strings.ToLower(strings.TrimSpace(vormund))
	if n == nil {
		return fmt.Errorf("signature required")
	}
	tx := Transaction{Type: "vormund_setzen", Wallet: wallet, To: vormund, Nachweis: n}
	if err := pruefeNachweisJetzt(&tx); err != nil {
		return err
	}
	if cs.db == nil {
		return fmt.Errorf("guardian operations require a database — run with DATABASE_URL set")
	}
	if err := cs.annahmeBeginnen(wallet); err != nil {
		return err
	}
	defer cs.annahmeEnde()
	return cs.runAtomicWithOutbox([]string{wallet}, false, func(ctx context.Context) (Transaction, error) {
		// Zwei gleichzeitige Eintragungen fuer denselben Vormund koennten
		// sonst beide unter der Grenze von drei bleiben (Pruefung vor dem
		// Schreiben). Gilt fuer die Transaktion, endet mit ihr.
		if _, err := cs.dbExecCtx(ctx).Exec(`SELECT pg_advisory_xact_lock(abs(hashtext($1)))`, vormund); err != nil {
			return Transaction{}, fmt.Errorf("could not acquire guardian advisory lock: %w", err)
		}
		if err := cs.vormundRegelnCtx(ctx, wallet, vormund, n.Zeit); err != nil {
			return Transaction{}, err
		}
		if err := cs.vormundEintragenCtx(ctx, wallet, vormund, n.Zeit); err != nil {
			return Transaction{}, err
		}
		fmt.Printf("[GUARDIAN] ✓ %s set guardian to %s\n", wallet, vormund)
		return tx, nil
	})
}

// Lebenszeichen: der eingetragene Vormund bestaetigt, dass wallet lebt.
func (cs *ChainState) Lebenszeichen(wallet, vormund string, n *Auftragsnachweis) error {
	wallet, vormund = strings.ToLower(strings.TrimSpace(wallet)), strings.ToLower(strings.TrimSpace(vormund))
	if n == nil {
		return fmt.Errorf("signature required")
	}
	tx := Transaction{Type: "lebenszeichen", Wallet: wallet, To: vormund, Nachweis: n}
	if err := pruefeNachweisJetzt(&tx); err != nil {
		return err
	}
	if cs.db == nil {
		return fmt.Errorf("guardian operations require a database — run with DATABASE_URL set")
	}
	if err := cs.annahmeBeginnen(wallet); err != nil {
		return err
	}
	defer cs.annahmeEnde()
	return cs.runAtomicWithOutbox([]string{wallet}, false, func(ctx context.Context) (Transaction, error) {
		if err := cs.lebenszeichenRegelnCtx(ctx, wallet, vormund); err != nil {
			return Transaction{}, err
		}
		acc, _ := cs.accounts.Get(wallet)
		touchActivity(acc)
		if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
			return Transaction{}, fmt.Errorf("could not persist activity timer reset for %s: %w", wallet, err)
		}
		fmt.Printf("[GUARDIAN] ✓ Guardian confirmed %s is alive — activity timer reset\n", wallet)
		return tx, nil
	})
}

func (cs *ChainState) lebenszeichenRegelnCtx(ctx context.Context, wallet, vormund string) error {
	cs.ensureAccountLoadedCtx(ctx, wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		return fmt.Errorf("account %s not found", wallet)
	}
	if !acc.IsHuman {
		return fmt.Errorf("account %s is not a registered human — guardian confirm-alive not applicable", wallet)
	}
	eingetragen, err := cs.eingetragenerVormundCtx(ctx, wallet)
	if err != nil {
		return fmt.Errorf("guardian lookup failed: %w", err)
	}
	if eingetragen == "" {
		return fmt.Errorf("no guardian set for this wallet")
	}
	if eingetragen != vormund {
		return fmt.Errorf("guardian mismatch: %s is not the guardian of %s", kurzAdresse(vormund), kurzAdresse(wallet))
	}
	return nil
}

// nachrechnenVormundLocked: die Regeln beim Nachspielen, gegen die eigene
// Tabelle. Die Unterschrift prueft pruefeAuftraegeImBlock schon vorher.
func (cs *ChainState) nachrechnenVormundLocked(tx *Transaction, blockZeit int64) error {
	wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
	vormund := strings.ToLower(strings.TrimSpace(tx.To))
	if tx.Nachweis == nil {
		return nachrechnenAbweichung("vormund_ohne_nachweis", blockZeit, "%s %s ohne Nachweis", tx.Type, kurzAdresse(wallet))
	}
	ctx := context.Background()
	switch tx.Type {
	case "vormund_setzen":
		if err := cs.vormundRegelnCtx(ctx, wallet, vormund, tx.Nachweis.Zeit); err != nil {
			return nachrechnenAbweichung("vormund_regel", blockZeit, "%s -> %s: %v", kurzAdresse(wallet), kurzAdresse(vormund), err)
		}
	case "lebenszeichen":
		if err := cs.lebenszeichenRegelnCtx(ctx, wallet, vormund); err != nil {
			return nachrechnenAbweichung("lebenszeichen_fremd", blockZeit, "%s: %v", kurzAdresse(wallet), err)
		}
	}
	return nil
}

// Nachspielen: dasselbe wie beim Annehmen, mit der Blockzeit.
func (cs *ChainState) applyVormundSetzenLocked(ctx context.Context, tx *Transaction) error {
	if tx.Nachweis == nil {
		return fmt.Errorf("vormund_setzen ohne Nachweis")
	}
	return cs.vormundEintragenCtx(ctx, strings.ToLower(strings.TrimSpace(tx.Wallet)),
		strings.ToLower(strings.TrimSpace(tx.To)), tx.Nachweis.Zeit)
}

func (cs *ChainState) applyLebenszeichenLocked(ctx context.Context, tx *Transaction, blockZeit int64) error {
	wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
	cs.ensureAccountLoadedCtx(ctx, wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		return fmt.Errorf("lebenszeichen: Konto %s unbekannt", kurzAdresse(wallet))
	}
	touchActivityAt(acc, blockZeit)
	return cs.saveAccountToDBCtx(ctx, acc)
}
