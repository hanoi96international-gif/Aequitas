package keeper

// STUFE 2: DIE VERMOEGENSGRENZE IST EINE BELASTUNG -- ALSO SACHE DES
// ZUSTAENDIGEN.
//
// Bis Stufe 2 kappt jede Gutschrift sofort (enforceWealthCapLockedCtx): wer
// ueber avg x Faktor landet, gibt den Ueberschuss ans Grundeinkommen ab. Das
// ist eine BELASTUNG des Empfaengers, und sie haengt an seinem Stand im
// Augenblick der Gutschrift.
//
// Mit vielen Annehmenden (zustaendigkeit.go) nimmt jeder Gutschriften an --
// Addition kennt keine Reihenfolge. Die Kappung aber schon: nimmt Knoten B
// eine Gutschrift an Y an, waehrend Y's Zustaendiger A gerade eine Ausgabe
// von Y annimmt, sieht B einen hoeheren Stand, kappt mehr, und jeder
// nachspielende Knoten rechnet in seiner Reihenfolge etwas anderes aus.
//
// Deshalb ab der Aktivierung:
//
//   - Gutschriften kappen nicht mehr. Das Konto wird vorgemerkt.
//   - Der Zustaendige des Kontos -- der einzige, der es belastet -- kappt mit
//     einer eigenen Transaktion "kappung" und festem Betrag. Nachgespielt wird
//     genau dieser Betrag, nichts wird neu gerechnet.
//   - Der Betrag ist hoechstens der Kontostand (LP-Anteile werden hier nicht
//     aufgeloest: das beruehrte den Pool, und der gehoert dem Leiter). Der
//     Rest faellt bei der naechsten Kappung.
//
// Ein Konto kann damit fuer die Dauer eines Blocks ueber der Grenze liegen.
// Das ist der Preis dafuer, dass jeder annehmen kann; gerecht bleibt es, weil
// der Ueberschuss vollstaendig ans Grundeinkommen geht wie bisher.

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

// kappungVerschobenLocked: gilt fuer die laufende Operation Stufe 2?
// Nachspielen: Blockzeit; Annahme: jetzt. cs.mu gehalten.
func (cs *ChainState) kappungVerschobenLocked() bool {
	zeit := cs.nachspielZeit
	if zeit == 0 {
		zeit = nowUnix()
	}
	return verteilteAnnahmeAktiv(zeit)
}

func (cs *ChainState) kappungVormerken(addr string) {
	cs.kappungsMu.Lock()
	if cs.kappungsKandidaten == nil {
		cs.kappungsKandidaten = map[string]bool{}
	}
	cs.kappungsKandidaten[strings.ToLower(addr)] = true
	cs.kappungsMu.Unlock()
}

// kappungsBetragLocked: wie viel Y abgeben muss -- dieselbe Rechnung wie
// enforceWealthCapLockedCtx, begrenzt auf den Kontostand.
func (cs *ChainState) kappungsBetragLocked(acc *AccountState) float64 {
	if acc == nil || isTokenomicsPoolAddress(acc.Address) {
		return 0
	}
	if !acc.IsHuman && cs.wirt().istUnternehmen(acc.Address) {
		return 0
	}
	grenze, ok := cs.wealthCapAmountLocked()
	if !ok {
		return 0
	}
	ueber := acc.Balance.Float() + cs.lpValueLockedAEQ(acc) - grenze
	if ueber <= 0 {
		return 0
	}
	return round6(math.Min(ueber, acc.Balance.Float()))
}

// KappungenAbarbeiten: fuer jedes vorgemerkte Konto, fuer das dieser Knoten
// zustaendig ist, die Kappung annehmen. Gibt die Zahl der Kappungen zurueck.
// Konten anderer Zustaendiger bleiben vorgemerkt -- deren Zustaendiger merkt
// sie sich selbst, sobald er die Gutschrift nachspielt.
func (cs *ChainState) KappungenAbarbeiten() int {
	cs.kappungsMu.Lock()
	var konten []string
	for a := range cs.kappungsKandidaten {
		konten = append(konten, a)
	}
	cs.kappungsMu.Unlock()
	sort.Strings(konten)
	n := 0
	for _, a := range konten {
		if !cs.nimmtAnFuer(a) {
			continue
		}
		gekappt, err := cs.kappungAnnehmen(a)
		if err != nil {
			fmt.Printf("[KAPPUNG] ✗ %s: %v\n", a, err)
			continue
		}
		cs.kappungsMu.Lock()
		delete(cs.kappungsKandidaten, a)
		cs.kappungsMu.Unlock()
		if gekappt {
			n++
		}
	}
	return n
}

// kappungAnnehmen: die Kappung eines Kontos als eigene Transaktion.
func (cs *ChainState) kappungAnnehmen(addr string) (bool, error) {
	if err := cs.annahmeBeginnen(addr); err != nil {
		return false, err
	}
	defer cs.annahmeEnde()
	gekappt := false
	err := cs.runAtomicWithOutbox([]string{addr, validatorsPoolAddr, lpPoolAddr, ubiPoolAddr, treasuryPoolAddr}, false,
		func(ctx context.Context) (Transaction, error) {
			cs.ensureAccountLoadedCtx(ctx, addr)
			acc, ok := cs.accounts.Get(addr)
			if !ok {
				return Transaction{}, fmt.Errorf("Konto unbekannt")
			}
			betrag := cs.kappungsBetragLocked(acc)
			if betrag <= 0 {
				return Transaction{}, errNichtsZuKappen
			}
			if err := cs.kappungAnwendenLocked(ctx, acc, betrag, nil); err != nil {
				return Transaction{}, err
			}
			gekappt = true
			return Transaction{Type: "kappung", Wallet: addr, Amount: betrag}, nil
		})
	if err == errNichtsZuKappen {
		return false, nil
	}
	return gekappt, err
}

var errNichtsZuKappen = fmt.Errorf("nichts zu kappen")

// kappungAnwendenLocked: Annahme und Nachspielen -- genau der Betrag aus der
// Transaktion, vom Konto ans Grundeinkommen.
func (cs *ChainState) kappungAnwendenLocked(ctx context.Context, acc *AccountState, betrag float64, sammler *kontenSammler) error {
	if betrag <= 0 || math.IsNaN(betrag) || math.IsInf(betrag, 0) {
		return fmt.Errorf("kappung: ungueltiger Betrag %v: %w", betrag, ErrZustandLehntAb)
	}
	if acc.Balance.Float() < betrag {
		return fmt.Errorf("kappung: %s hat %.6f, soll %.6f abgeben: %w", acc.Address, acc.Balance.Float(), betrag, ErrZustandLehntAb)
	}
	acc.Balance = acc.Balance.Sub(NewDecimal(betrag))
	cs.updateAccountLeafLocked(acc)
	if err := cs.umverteilenAnAlleCtx(ctx, betrag); err != nil {
		return fmt.Errorf("kappung: Toepfe: %w", err)
	}
	fmt.Printf("[WEALTH CAP] %s: %.6f AEQ ueber der Grenze ganz ins Grundeinkommen (Stufe 2)\n", acc.Address, betrag)
	if sammler != nil {
		sammler.hinzufuegen(acc)
		return nil
	}
	return cs.saveAccountToDBCtx(ctx, acc)
}

// applyKappungDeltaLocked: Nachspielen einer "kappung".
func (cs *ChainState) applyKappungDeltaLocked(ctx context.Context, wallet string, betrag float64, sammler *kontenSammler) error {
	wallet = strings.ToLower(wallet)
	cs.ensureAccountLoadedCtx(ctx, wallet)
	acc, ok := cs.accounts.Get(wallet)
	if !ok {
		return fmt.Errorf("kappung: Konto %s unbekannt: %w", wallet, ErrZustandLehntAb)
	}
	return cs.kappungAnwendenLocked(ctx, acc, betrag, sammler)
}

// KappungStand fuer /health.
func (cs *ChainState) KappungStand() map[string]interface{} {
	cs.kappungsMu.Lock()
	n := len(cs.kappungsKandidaten)
	cs.kappungsMu.Unlock()
	return map[string]interface{}{
		"vorgemerkt": n,
		"bedeutung":  "Stufe 2: Konten ueber der Vermoegensgrenze, deren Kappung der Zustaendige noch annehmen muss.",
	}
}
