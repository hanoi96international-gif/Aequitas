package keeper

// STUFE 2: TAUSCH UND LIQUIDITAET IN ZWEI SCHRITTEN.
//
// Ein Tausch belastet ZWEI Dinge: das Konto des Auftraggebers X (sein AEQ,
// tUSD oder seine Anteile) und den Liquiditaetspool. Im verteilten Term
// (zustaendigkeit.go) gehoeren sie verschiedenen: X seinem Zustaendigen, der
// Pool dem Leiter. Keiner darf beides belasten -- sonst belastete ein Knoten
// ein Konto, fuer das ein anderer zustaendig ist, und die Kontenstaende
// liefen auseinander wie am 15.09.2026.
//
// Deshalb zwei Schritte, jeder vom Zustaendigen seines Teils:
//
//  1. VORBEHALT (Zustaendiger von X): der Einsatz geht von X auf ein eigenes
//     Vorbehaltskonto "vorbehalt:<Hash>". Die Transaktion traegt den
//     unterschriebenen Auftrag (Nachweis) -- jeder Validator prueft ihn wie
//     einen direkten Tausch, samt Auftrags-Nonce.
//  2. AUSFUEHRUNG (Leiter): der Inhalt des Vorbehaltskontos geht an X zurueck,
//     und im selben Schritt laeuft der Tausch wie immer (swapLockedMitAbgabe
//     usw.) -- der eben gutgeschriebene Einsatz deckt ihn. Scheitert er (etwa
//     am Mindestbetrag), bleibt es bei der Rueckbuchung: Erstattung.
//
// Doppelt ausfuehren geht nicht: nach der Ausfuehrung ist das Vorbehaltskonto
// leer, eine zweite scheitert an der Deckung -- beim Leiter wie bei jedem, der
// nachspielt. Ein neuer Leiter findet offene Vorbehalte ueber offeneVorbehalte
// und fuehrt sie aus.
//
// Der Auftraggeber bekommt nach Schritt 1 die Antwort "vorgemerkt"; das
// Ergebnis steht mit dem naechsten Block des Leiters in seinem Konto.

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// VorbehaltAngaben: was ein Vorbehalt und seine Ausfuehrung tragen.
type VorbehaltAngaben struct {
	// Art: der eigentliche Auftrag (swap_aeq_tusd, swap_tusd_aeq,
	// add_liquidity, remove_liquidity).
	Art string `json:"art"`
	// Ref: bei der Ausfuehrung der TxHash des Vorbehalts.
	Ref string `json:"ref,omitempty"`
	// Was auf dem Vorbehaltskonto liegt (Vorbehalt) bzw. von dort an X
	// zurueckgeht (Ausfuehrung).
	AEQ  float64 `json:"aeq,omitempty"`
	TUSD float64 `json:"tusd,omitempty"`
	LP   float64 `json:"lp,omitempty"`
	// MinOut: Mindestbetrag des Tauschs (Schutz des Auftraggebers).
	MinOut float64 `json:"min_out,omitempty"`
	// Erstattet: die Ausfuehrung ist gescheitert, es bleibt bei der
	// Rueckbuchung.
	Erstattet bool `json:"erstattet,omitempty"`
}

func vorbehaltsKonto(txHash string) string {
	return "vorbehalt:" + strings.ToLower(strings.TrimSpace(txHash))
}

func vorbehaltsArt(art string) bool {
	switch art {
	case "swap_aeq_tusd", "swap_tusd_aeq", "add_liquidity", "remove_liquidity":
		return true
	}
	return false
}

// innererAuftrag: der Vorbehalt so, wie der direkte Auftrag aussaehe -- fuer
// die Pruefung des Nachweises (auftrag_nachweis.go).
func innererAuftrag(tx *Transaction) Transaction {
	inner := *tx
	if tx.Vorbehalt != nil {
		inner.Type = tx.Vorbehalt.Art
	}
	return inner
}

// Einsatz eines Auftrags: was vom Konto auf das Vorbehaltskonto geht.
// Tausch AEQ -> tUSD: der Betrag plus die hoechstmoegliche Ausstiegsabgabe
// (2 %) -- der Leiter rechnet die tatsaechliche, der Rest geht zurueck.
func vorbehaltsEinsatz(art string, betrag, betrag2 float64) (aeq, tusd, lp float64) {
	switch art {
	case "swap_aeq_tusd":
		return round6(betrag * (1 + float64(ausstiegsAbgabeBps)/10_000)), 0, 0
	case "swap_tusd_aeq":
		return 0, betrag, 0
	case "add_liquidity":
		return betrag, betrag2, 0
	case "remove_liquidity":
		return 0, 0, betrag
	}
	return 0, 0, 0
}

// --- offene Vorbehalte ---------------------------------------------------------

var offeneVorbehalteMu sync.Mutex

// offenerVorbehalt: was der Leiter zum Ausfuehren braucht -- aus dem
// signierten Vorbehalt, nicht aus dem Inhalt des Vorbehaltskontos.
type offenerVorbehalt struct {
	wallet, art     string
	betrag, betrag2 float64
	minOut          float64
}

// vorbehaltOffen: Vorbehaltskonto -> offener Vorbehalt, auf diesem Knoten
// bekannt (Annahme und Nachspielen). Nur der Leiter arbeitet sie ab.
//
// Mit Datenbank ist die Tabelle vorbehalte_offen die EINZIGE Quelle (seit
// 05.10.2026). Vorher fuehrte der Knoten zusaetzlich cs.vorbehalte im
// Speicher -- und die Karte wurde beim Zurueckrollen eines Blocks nicht
// zurueckgesetzt, die Tabelle schon. Folgen: ein zurueckgerollter Vorbehalt
// stand weiter in der Karte, und der Leiter fuehrte ihn aus (Rueckbuchung aus
// einem Vorbehaltskonto, das es so nicht gab); eine zurueckgerollte Loeschung
// fehlte in der Karte, und der Vorbehalt blieb bis zum Neustart liegen. Die
// Karte gilt nur noch ohne Datenbank (Tests, Entwicklungsknoten).
func (cs *ChainState) vorbehaltOffen(ctx context.Context, konto string, o *offenerVorbehalt) error {
	if cs.db == nil {
		offeneVorbehalteMu.Lock()
		if cs.vorbehalte == nil {
			cs.vorbehalte = map[string]offenerVorbehalt{}
		}
		if o != nil {
			cs.vorbehalte[konto] = *o
		} else {
			delete(cs.vorbehalte, konto)
		}
		offeneVorbehalteMu.Unlock()
		return nil
	}
	var err error
	if o != nil {
		_, err = cs.dbExecCtx(ctx).Exec(`INSERT INTO vorbehalte_offen (konto, wallet, art, betrag, betrag2, min_out) VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (konto) DO UPDATE SET wallet=$2, art=$3, betrag=$4, betrag2=$5, min_out=$6`,
			konto, o.wallet, o.art, o.betrag, o.betrag2, o.minOut)
	} else {
		_, err = cs.dbExecCtx(ctx).Exec(`DELETE FROM vorbehalte_offen WHERE konto = $1`, konto)
	}
	if err != nil {
		return fmt.Errorf("vorbehalte_offen: %w", err)
	}
	return nil
}

// vorbehaltSicherung: Stand von cs.vorbehalte fuer den Rueckroll-Snapshot.
//
// Die Karte im Speicher steht neben der Tabelle vorbehalte_offen, die mit
// der Datenbanktransaktion zurueckgeht -- die Karte ging nicht mit. Mit
// Datenbank arbeitet der Leiter seit #284 nur noch die Tabelle ab; ohne
// Datenbank (Tests, Entwicklungsknoten) ist die Karte der einzige Stand, und
// ein zurueckgewiesener Block hinterliess dort einen Vorbehalt, den es nicht
// gibt, oder loeschte einen offenen. Die Sicherung haelt beide gleich.
//
// Gesichert werden nur die Vorbehaltskonten unter den Adressen des
// Snapshots: blockTouchedAddresses nennt sie fuer jeden Vorbehalt und jede
// Ausfuehrung im Block, VorbehaltAtomic und VorbehaltAusfuehren geben sie
// runAtomicWithOutbox mit. Eine Ueberweisung beruehrt keines und zahlt
// nichts. Der volle Snapshot sichert die ganze Karte.
type vorbehaltSicherung struct {
	voll      bool
	eintraege map[string]vorbehaltEintrag
}

type vorbehaltEintrag struct {
	o     offenerVorbehalt
	offen bool
}

// vorbehaltSichern: unter cs.mu (wie snapshotForRollbackLocked).
func (cs *ChainState) vorbehaltSichern(addrs []string, voll bool) vorbehaltSicherung {
	offeneVorbehalteMu.Lock()
	defer offeneVorbehalteMu.Unlock()
	if voll {
		s := vorbehaltSicherung{voll: true, eintraege: make(map[string]vorbehaltEintrag, len(cs.vorbehalte))}
		for k, o := range cs.vorbehalte {
			s.eintraege[k] = vorbehaltEintrag{o: o, offen: true}
		}
		return s
	}
	var s vorbehaltSicherung
	for _, a := range addrs {
		if !strings.HasPrefix(a, "vorbehalt:") {
			continue
		}
		if s.eintraege == nil {
			s.eintraege = make(map[string]vorbehaltEintrag)
		}
		o, ok := cs.vorbehalte[a]
		s.eintraege[a] = vorbehaltEintrag{o: o, offen: ok}
	}
	return s
}

// vorbehaltZurueck: nur der Speicher -- die Zeilen in vorbehalte_offen lagen
// in der zurueckgerollten Transaktion.
func (cs *ChainState) vorbehaltZurueck(s vorbehaltSicherung) {
	offeneVorbehalteMu.Lock()
	defer offeneVorbehalteMu.Unlock()
	if s.voll {
		cs.vorbehalte = make(map[string]offenerVorbehalt, len(s.eintraege))
		for k, e := range s.eintraege {
			cs.vorbehalte[k] = e.o
		}
		return
	}
	for k, e := range s.eintraege {
		if !e.offen {
			delete(cs.vorbehalte, k)
			continue
		}
		if cs.vorbehalte == nil {
			cs.vorbehalte = map[string]offenerVorbehalt{}
		}
		cs.vorbehalte[k] = e.o
	}
}

// --- Schritt 1: Vorbehalt (Zustaendiger des Kontos) ---------------------------

// VorbehaltAtomic nimmt den ersten Schritt an. tmpl traegt Art (Type),
// Betraege und Nachweis wie der direkte Auftrag.
func (cs *ChainState) VorbehaltAtomic(tmpl Transaction, minOut float64) (string, error) {
	x := strings.ToLower(strings.TrimSpace(tmpl.Wallet))
	if !vorbehaltsArt(tmpl.Type) {
		return "", fmt.Errorf("Vorbehalt fuer %q nicht moeglich", tmpl.Type)
	}
	if err := cs.annahmeBeginnen(x); err != nil {
		return "", err
	}
	defer cs.annahmeEnde()
	if tmpl.TxHash == "" {
		tmpl.TxHash = fmt.Sprintf("0xvb%x", time.Now().UnixNano())
	}
	aeq, tusd, lp := vorbehaltsEinsatz(tmpl.Type, tmpl.Amount, tmpl.AmountOut)
	tx := tmpl
	tx.Type = "vorbehalt"
	tx.Vorbehalt = &VorbehaltAngaben{Art: tmpl.Type, AEQ: aeq, TUSD: tusd, LP: lp, MinOut: minOut}
	v := vorbehaltsKonto(tx.TxHash)
	err := cs.runAtomicWithOutbox([]string{x, v}, false, func(ctx context.Context) (Transaction, error) {
		if tmpl.Type == "add_liquidity" && wirtschaftAktiv(nowUnix()) {
			cs.ensureAccountLoadedCtx(ctx, x)
			if acc, ok := cs.accounts.Get(x); !ok || !acc.IsHuman {
				return Transaction{}, fmt.Errorf("only registered humans can provide liquidity")
			}
		}
		if err := cs.vorbehaltAnwendenLocked(ctx, &tx, nil); err != nil {
			return Transaction{}, err
		}
		return tx, nil
	})
	if err != nil {
		return "", err
	}
	return tx.TxHash, nil
}

// vorbehaltAnwendenLocked: X -> Vorbehaltskonto (Annahme und Nachspielen).
func (cs *ChainState) vorbehaltAnwendenLocked(ctx context.Context, tx *Transaction, sammler *kontenSammler) error {
	vb := tx.Vorbehalt
	if vb == nil || !vorbehaltsArt(vb.Art) {
		return fmt.Errorf("vorbehalt: Angaben fehlen: %w", ErrZustandLehntAb)
	}
	if vb.AEQ < 0 || vb.TUSD < 0 || vb.LP < 0 || vb.AEQ+vb.TUSD+vb.LP <= 0 {
		return fmt.Errorf("vorbehalt: ungueltiger Einsatz: %w", ErrZustandLehntAb)
	}
	x := strings.ToLower(strings.TrimSpace(tx.Wallet))
	cs.ensureAccountLoadedCtx(ctx, x)
	acc, ok := cs.accounts.Get(x)
	if !ok {
		return fmt.Errorf("vorbehalt: Konto %s unbekannt: %w", x, ErrZustandLehntAb)
	}
	if acc.Balance.Float() < vb.AEQ || acc.TUsdBalance.Float() < vb.TUSD || acc.LPShares.Float() < vb.LP {
		return fmt.Errorf("vorbehalt: %s deckt den Einsatz nicht: %w", x, ErrZustandLehntAb)
	}
	v := vorbehaltsKonto(tx.TxHash)
	cs.ensureAccountLoadedCtx(ctx, v)
	vAcc, ok := cs.accounts.Get(v)
	if !ok {
		vAcc = &AccountState{Address: v, LastActivityAt: nowUnix()}
		cs.accounts.Set(v, vAcc)
	}
	if !vAcc.Balance.IsZero() || !vAcc.TUsdBalance.IsZero() || !vAcc.LPShares.IsZero() {
		return fmt.Errorf("vorbehalt: %s existiert schon: %w", v, ErrZustandLehntAb)
	}
	acc.Balance = acc.Balance.Sub(NewDecimal(vb.AEQ))
	acc.TUsdBalance = acc.TUsdBalance.Sub(NewDecimal(vb.TUSD))
	acc.LPShares = acc.LPShares.Sub(NewDecimal(vb.LP))
	vAcc.Balance = NewDecimal(vb.AEQ)
	vAcc.TUsdBalance = NewDecimal(vb.TUSD)
	vAcc.LPShares = NewDecimal(vb.LP)
	cs.updateAccountLeafLocked(acc)
	cs.updateAccountLeafLocked(vAcc)
	if err := cs.vorbehaltOffen(ctx, v, &offenerVorbehalt{wallet: x, art: vb.Art, betrag: tx.Amount, betrag2: tx.AmountOut, minOut: vb.MinOut}); err != nil {
		return err
	}
	if sammler != nil {
		sammler.hinzufuegen(acc, vAcc)
		return nil
	}
	if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
		return err
	}
	return cs.saveAccountToDBCtx(ctx, vAcc)
}

// --- Schritt 2: Ausfuehrung (Leiter) ------------------------------------------

// rueckbuchenLocked: Vorbehaltskonto -> X, genau die angegebenen Betraege.
func (cs *ChainState) rueckbuchenLocked(ctx context.Context, x, v string, aeq, tusd, lp float64) (*AccountState, *AccountState, error) {
	cs.ensureAccountLoadedCtx(ctx, v)
	vAcc, ok := cs.accounts.Get(v)
	if !ok || vAcc.Balance.Float() < aeq || vAcc.TUsdBalance.Float() < tusd || vAcc.LPShares.Float() < lp {
		return nil, nil, fmt.Errorf("ausfuehrung: %s deckt die Rueckbuchung nicht (schon ausgefuehrt?): %w", v, ErrZustandLehntAb)
	}
	cs.ensureAccountLoadedCtx(ctx, x)
	acc, ok := cs.accounts.Get(x)
	if !ok {
		acc = &AccountState{Address: x}
		cs.accounts.Set(x, acc)
	}
	vAcc.Balance = vAcc.Balance.Sub(NewDecimal(aeq))
	vAcc.TUsdBalance = vAcc.TUsdBalance.Sub(NewDecimal(tusd))
	vAcc.LPShares = vAcc.LPShares.Sub(NewDecimal(lp))
	acc.Balance = acc.Balance.Add(NewDecimal(aeq))
	acc.TUsdBalance = acc.TUsdBalance.Add(NewDecimal(tusd))
	acc.LPShares = acc.LPShares.Add(NewDecimal(lp))
	cs.updateAccountLeafLocked(vAcc)
	cs.updateAccountLeafLocked(acc)
	if vAcc.Balance.IsZero() && vAcc.TUsdBalance.IsZero() && vAcc.LPShares.IsZero() {
		if err := cs.vorbehaltOffen(ctx, v, nil); err != nil {
			return nil, nil, err
		}
	}
	return acc, vAcc, nil
}

// VorbehaltAusfuehren: der Leiter fuehrt einen offenen Vorbehalt aus.
func (cs *ChainState) VorbehaltAusfuehren(v string) error {
	eintrag, ok, err := cs.offenerVorbehaltCtx(context.Background(), v)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("kein offener Vorbehalt %s", v)
	}
	x, art := eintrag.wallet, eintrag.art
	if err := cs.annahmeBeginnen(x, kontoLiquiditaetspool); err != nil {
		return err
	}
	defer cs.annahmeEnde()
	return cs.runAtomicWithOutbox([]string{x, v, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}, false,
		func(ctx context.Context) (Transaction, error) {
			// Unter der Sperre und in der Transaktion noch einmal: steht er
			// noch so da? Ein Block dazwischen kann ihn erledigt oder
			// zurueckgerollt haben.
			jetztEintrag, noch, lesFehler := cs.offenerVorbehaltCtx(ctx, v)
			if lesFehler != nil {
				return Transaction{}, lesFehler
			}
			if !noch || jetztEintrag != eintrag {
				return Transaction{}, fmt.Errorf("Vorbehalt %s nicht mehr offen oder veraendert", v)
			}
			cs.ensureAccountLoadedCtx(ctx, v)
			vAcc, ok := cs.accounts.Get(v)
			if !ok {
				return Transaction{}, fmt.Errorf("Vorbehaltskonto %s fehlt", v)
			}
			aeq, tusd, lp := vAcc.Balance.Float(), vAcc.TUsdBalance.Float(), vAcc.LPShares.Float()
			minOut := eintrag.minOut
			if _, _, err := cs.rueckbuchenLocked(ctx, x, v, aeq, tusd, lp); err != nil {
				return Transaction{}, err
			}
			ref := strings.TrimPrefix(v, "vorbehalt:")
			tx := Transaction{Type: "vorbehalt_ausfuehrung", Wallet: x, TxHash: "0xvx" + strings.TrimPrefix(ref, "0x"),
				Vorbehalt: &VorbehaltAngaben{Art: art, Ref: ref, AEQ: aeq, TUSD: tusd, LP: lp}}
			// Der eigentliche Auftrag, gedeckt durch die Rueckbuchung. Er
			// darf scheitern -- dann bleibt es bei ihr (Erstattung). Dafuer
			// wird er auf einer Kopie des Zustands versucht: runAtomicWithOutbox
			// setzt nur bei Fehler des GANZEN Schritts zurueck.
			at := nowUnix()
			bctx := mitBuchZeit(ctx, at)
			snap := cs.snapshotForRollbackLocked([]string{x, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}, false, nil)
			var err error
			switch art {
			case "swap_aeq_tusd", "swap_tusd_aeq":
				betrag := eintrag.betrag // unterschrieben; der Einsatz deckt ihn samt Abgabe
				var out, lost, abgabe float64
				out, lost, abgabe, err = cs.swapLockedMitAbgabe(bctx, x, betrag, art == "swap_aeq_tusd", minOut)
				tx.Amount, tx.AmountOut, tx.FromDemurrageLost, tx.Gebuehr = betrag, out, lost, abgabe
			case "add_liquidity":
				vorher := cs.lpAnteile(x)
				var lost float64
				lost, err = cs.addLiquidityLocked(ctx, x, eintrag.betrag, eintrag.betrag2)
				tx.Amount, tx.AmountOut, tx.FromDemurrageLost, tx.LPShares = eintrag.betrag, eintrag.betrag2, lost, cs.lpAnteile(x)-vorher
			case "remove_liquidity":
				var lost float64
				_, _, lost, err = cs.removeLiquidityLocked(ctx, x, eintrag.betrag)
				tx.Amount, tx.FromDemurrageLost = eintrag.betrag, lost
			}
			tx.BuchAt = buchStempel(at)
			if err != nil {
				if rbErr := cs.restoreFromRollbackLocked(snap); rbErr != nil {
					return Transaction{}, fmt.Errorf("Erstattung: Ruecksetzen: %w", rbErr)
				}
				tx.Amount, tx.AmountOut, tx.FromDemurrageLost, tx.Gebuehr, tx.LPShares = 0, 0, 0, 0, 0
				tx.Vorbehalt.Erstattet = true
				fmt.Printf("[VORBEHALT] %s: %s gescheitert (%v) -- erstattet\n", x, art, err)
			}
			return tx, nil
		})
}

func (cs *ChainState) lpAnteile(x string) float64 {
	if acc, ok := cs.accounts.Get(x); ok {
		return acc.LPShares.Float()
	}
	return 0
}

// applyVorbehaltAusfuehrungLocked: Nachspielen des zweiten Schritts -- genau
// die getragenen Betraege, nie neu gerechnet.
func (cs *ChainState) applyVorbehaltAusfuehrungLocked(ctx context.Context, tx *Transaction, blockZeit int64) error {
	vb := tx.Vorbehalt
	if vb == nil || !vorbehaltsArt(vb.Art) || vb.Ref == "" {
		return fmt.Errorf("ausfuehrung: Angaben fehlen: %w", ErrZustandLehntAb)
	}
	x := strings.ToLower(strings.TrimSpace(tx.Wallet))
	if _, _, err := cs.rueckbuchenLocked(ctx, x, vorbehaltsKonto(vb.Ref), vb.AEQ, vb.TUSD, vb.LP); err != nil {
		return err
	}
	if vb.Erstattet {
		return nil
	}
	bctx := mitBuchZeit(ctx, buchZeitBeimNachspielen(tx.BuchAt, blockZeit))
	switch vb.Art {
	case "swap_aeq_tusd":
		return cs.applySwapDeltaLockedMitAbgabe(bctx, x, tx.Amount, tx.AmountOut, true, tx.FromDemurrageLost, blockZeit, tx.Gebuehr)
	case "swap_tusd_aeq":
		return cs.applySwapDeltaLockedMitAbgabe(bctx, x, tx.Amount, tx.AmountOut, false, tx.FromDemurrageLost, blockZeit, tx.Gebuehr)
	case "add_liquidity":
		return cs.addLiquidityDeltaLocked(ctx, x, tx.Amount, tx.AmountOut, tx.LPShares, tx.FromDemurrageLost, blockZeit)
	case "remove_liquidity":
		return cs.removeLiquidityDeltaLocked(ctx, x, tx.Amount, tx.FromDemurrageLost, blockZeit)
	}
	return nil
}

// VorbehalteAbarbeiten: der Leiter fuehrt alle offenen Vorbehalte aus, die er
// kennt. Gibt die Zahl der ausgefuehrten zurueck.
func (cs *ChainState) VorbehalteAbarbeiten() int {
	if !cs.nimmtAnFuer(kontoLiquiditaetspool) {
		return 0
	}
	konten, err := cs.offeneVorbehaltKonten()
	if err != nil {
		fmt.Printf("[VORBEHALT] ✗ offene Vorbehalte nicht lesbar: %v\n", err)
		return 0
	}
	n := 0
	for _, v := range konten {
		if err := cs.VorbehaltAusfuehren(v); err != nil {
			fmt.Printf("[VORBEHALT] ✗ %s: %v\n", v, err)
			continue
		}
		n++
	}
	return n
}

// VorbehalteEinlesen: wie viele Vorbehalte nach einem Neustart offen sind.
// Mit Datenbank wird nichts in den Speicher geholt -- die Tabelle ist die
// Quelle (vorbehaltOffen); gezaehlt wird nur fuer das Protokoll.
func (cs *ChainState) VorbehalteEinlesen() int {
	if cs.db == nil {
		offeneVorbehalteMu.Lock()
		defer offeneVorbehalteMu.Unlock()
		return len(cs.vorbehalte)
	}
	var n int
	if err := cs.db.QueryRow(`SELECT COUNT(*) FROM vorbehalte_offen`).Scan(&n); err != nil {
		return 0
	}
	return n
}

// vorbehaltAbarbeitenGrenze: so viele Vorbehalte arbeitet der Leiter je
// Durchgang hoechstens ab; der Rest folgt im naechsten.
const vorbehaltAbarbeitenGrenze = 1000

// offenerVorbehaltCtx: ein offener Vorbehalt -- aus der Tabelle (in der
// Transaktion von ctx, wenn es eine gibt), ohne Datenbank aus der Karte.
func (cs *ChainState) offenerVorbehaltCtx(ctx context.Context, konto string) (offenerVorbehalt, bool, error) {
	if cs.db == nil {
		offeneVorbehalteMu.Lock()
		o, ok := cs.vorbehalte[konto]
		offeneVorbehalteMu.Unlock()
		return o, ok, nil
	}
	var o offenerVorbehalt
	err := cs.dbExecCtx(ctx).QueryRow(
		`SELECT wallet, art, betrag, betrag2, min_out FROM vorbehalte_offen WHERE konto = $1`, konto,
	).Scan(&o.wallet, &o.art, &o.betrag, &o.betrag2, &o.minOut)
	if err == sql.ErrNoRows {
		return o, false, nil
	}
	if err != nil {
		return o, false, fmt.Errorf("vorbehalte_offen: %w", err)
	}
	return o, true, nil
}

// offeneVorbehaltKonten: die offenen Vorbehaltskonten, sortiert, begrenzt.
func (cs *ChainState) offeneVorbehaltKonten() ([]string, error) {
	if cs.db == nil {
		offeneVorbehalteMu.Lock()
		var konten []string
		for v := range cs.vorbehalte {
			konten = append(konten, v)
		}
		offeneVorbehalteMu.Unlock()
		sort.Strings(konten)
		if len(konten) > vorbehaltAbarbeitenGrenze {
			konten = konten[:vorbehaltAbarbeitenGrenze]
		}
		return konten, nil
	}
	rows, err := cs.db.Query(`SELECT konto FROM vorbehalte_offen ORDER BY konto LIMIT $1`, vorbehaltAbarbeitenGrenze)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var konten []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		konten = append(konten, k)
	}
	return konten, rows.Err()
}

// VorbehaltStand fuer /health.
func (cs *ChainState) VorbehaltStand() map[string]interface{} {
	n := -1
	if cs.db == nil {
		offeneVorbehalteMu.Lock()
		n = len(cs.vorbehalte)
		offeneVorbehalteMu.Unlock()
	} else if err := cs.db.QueryRow(`SELECT COUNT(*) FROM vorbehalte_offen`).Scan(&n); err != nil {
		n = -1 // unlesbar
	}
	return map[string]interface{}{
		"offen":     n,
		"bedeutung": "Stufe 2: Tausch/Liquiditaet, deren Einsatz vorgemerkt ist und die der Leiter noch ausfuehren muss.",
	}
}

// vorbehaltStattDirekt: soll ein Tausch/Liquiditaetsauftrag fuer x ueber den
// Vorbehalt laufen? Ja im verteilten Term, wenn dieser Knoten x annimmt, aber
// nicht den Pool (dann ist er nicht der Leiter). Sonst direkt wie bisher --
// das Tor entscheidet dort.
func (cs *ChainState) vorbehaltStattDirekt(x string) bool {
	l := cs.leitung.Load()
	if l == nil {
		return false
	}
	l.mu.Lock()
	verteilt := l.verteiltImTerm()
	l.mu.Unlock()
	return verteilt && cs.nimmtAnFuer(x) && !cs.nimmtAnFuer(kontoLiquiditaetspool)
}
