package keeper

// STUFE 1.0, NACHTRAG -- JEDER AUFTRAG IST VON JEDEM VALIDATOR NACHPRUEFBAR.
//
// Stufe 1.0 (signierte_ueberweisung.go) hat Ueberweisungen nachpruefbar
// gemacht: die signierte Rohform reist im Block mit. Alle anderen Auftraege,
// die Geld oder Rechte bewegen, entstehen aus einer personal_sign-Unterschrift
// des Kontoinhabers (swap.go, wirtschaft_api.go, api.go) -- und die pruefte
// wieder nur der annehmende Knoten. Im Block stand nur, WAS geschehen soll,
// nicht, dass der Inhaber es wollte. Wer den annehmenden Knoten kontrolliert,
// haette fremde AEQ in tUSD tauschen, fremde Liquiditaet abziehen oder sich
// selbst zum Mitinhaber eines fremden Unternehmens machen koennen.
//
// Ab der Aktivierung (dieselbe wie fuer Ueberweisungen,
// signierteUeberweisungenPflicht) traegt jeder dieser Auftraege seinen
// Nachweis: die Unterschrift(en) und die Werte, die unterschrieben wurden.
// Jeder Validator baut beim Nachspielen die Nachricht aus dem Block nach --
// mit denselben Formaten wie die Annahme -- und prueft:
//
//   - die Unterschrift stammt vom Kontoinhaber (bei Unternehmen: von beiden
//     Unterzeichnern);
//   - die unterschriebenen Betraege sind die, die der Block ausfuehrt;
//   - der Zeitstempel passt zum Block (nicht aelter als eine Stunde, nicht
//     mehr als fuenf Minuten in dessen Zukunft);
//   - Tausch und Liquiditaet: die Nonce ist nicht verbraucht
//     (NaechsteAuftragsNonce im gemeinsamen Zustand, wie NaechsteNonce).
//
// Ein Verstoss macht den ganzen Block ungueltig -- wie bei Ueberweisungen.
//
// Nicht dabei: Auftraege, die das System selbst erzeugt (Verteilungsrunden,
// Treuhand-Verschiebung nach Inaktivitaet, Zuschuss-Staffel). Die entstehen
// aus Regeln, nicht aus einem Willen, und jeder Validator rechnet sie nach.
// Die Registrierung traegt ihren Beweis ohnehin (ProofA/B/C).

import (
	"context"
	"crypto/sha256"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// Auftragsnachweis: was der Auftraggeber unterschrieben hat.
type Auftragsnachweis struct {
	Sig string `json:"sig"`
	// Zweite Unterschrift (Unternehmen eroeffnen: die des Menschen;
	// Mitinhaber: die des bisher Verantwortlichen).
	Sig2 string `json:"sig2,omitempty"`
	// Von2: zweiter Unterzeichner, wenn er nicht im Block steht (Mitinhaber:
	// der bisher Verantwortliche).
	Von2    string  `json:"von2,omitempty"`
	Nonce   int64   `json:"nonce,omitempty"`
	Zeit    int64   `json:"zeit,omitempty"`
	Betrag  float64 `json:"betrag,omitempty"`
	Betrag2 float64 `json:"betrag2,omitempty"`
}

// Zeitfenster gegen die Blockzeit.
const (
	nachweisHoechstensAlt    int64 = 3600
	nachweisHoechstensVoraus int64 = 300
)

// braucheNachweis: welche Transaktionsarten ab der Aktivierung einen
// Nachweis tragen muessen.
func braucheNachweis(typ string) bool {
	switch typ {
	case "vorbehalt": // Stufe 2 (vorbehalt.go): der Nachweis des inneren Auftrags
		return true
	case "swap_aeq_tusd", "swap_tusd_aeq", "add_liquidity", "remove_liquidity",
		"faucet", "escrow_recover",
		"unternehmen_eroeffnen", "unternehmen_mitinhaber", "unternehmen_schliessen":
		return true
	}
	return false
}

// nutztAuftragsNonce: Auftraege mit fortlaufender Nonce (swap_nonces).
func nutztAuftragsNonce(typ string) bool {
	switch typ {
	case "swap_aeq_tusd", "swap_tusd_aeq", "add_liquidity", "remove_liquidity", "vorbehalt":
		return true
	}
	return false
}

// hatNachweisZeit: Auftraege, deren Nachricht einen Zeitstempel traegt.
func hatNachweisZeit(typ string) bool {
	return typ != "escrow_recover"
}

// unterschrift: eine erwartete Unterschrift -- wer, worueber.
type unterschrift struct {
	von, sig string
}

// auftragsNachricht baut die Nachricht nach, die der Auftraggeber
// unterschrieben hat, in genau den Formaten der Annahme (swap.go,
// wirtschaft_api.go, api.go), und prueft, dass der Block ausfuehrt, was
// unterschrieben wurde. Zustandslos.
func auftragsNachricht(tx *Transaction) (string, []unterschrift, error) {
	if tx.Type == "vorbehalt" {
		// Stufe 2: geprueft wird der innere Auftrag, wie ihn der Auftraggeber
		// unterschrieben hat (vorbehalt.go).
		if tx.Vorbehalt == nil || !vorbehaltsArt(tx.Vorbehalt.Art) {
			return "", nil, fmt.Errorf("Vorbehalt ohne gueltige Art")
		}
		inner := innererAuftrag(tx)
		return auftragsNachricht(&inner)
	}
	n := tx.Nachweis
	w := strings.ToLower(strings.TrimSpace(tx.Wallet))
	to := strings.ToLower(strings.TrimSpace(tx.To))
	switch tx.Type {
	case "swap_aeq_tusd", "swap_tusd_aeq":
		if tx.Amount != n.Betrag {
			return "", nil, fmt.Errorf("Tausch: unterschrieben %.8f, im Block %.8f", n.Betrag, tx.Amount)
		}
		richtung := "aeq_to_tusd"
		if tx.Type == "swap_tusd_aeq" {
			richtung = "tusd_to_aeq"
		}
		return fmt.Sprintf("Aequitas Swap: %s %.8f nonce:%d ts:%d", richtung, n.Betrag, n.Nonce, n.Zeit),
			[]unterschrift{{w, n.Sig}}, nil
	case "add_liquidity":
		if tx.Amount != n.Betrag || tx.AmountOut != n.Betrag2 {
			return "", nil, fmt.Errorf("Liquiditaet: unterschrieben %.8f/%.8f, im Block %.8f/%.8f", n.Betrag, n.Betrag2, tx.Amount, tx.AmountOut)
		}
		return fmt.Sprintf("Aequitas Add Liquidity: %.8f AEQ + %.8f tUSD nonce:%d ts:%d", n.Betrag, n.Betrag2, n.Nonce, n.Zeit),
			[]unterschrift{{w, n.Sig}}, nil
	case "remove_liquidity":
		if tx.Amount != n.Betrag {
			return "", nil, fmt.Errorf("Liquiditaet abziehen: unterschrieben %.8f Anteile, im Block %.8f", n.Betrag, tx.Amount)
		}
		return fmt.Sprintf("Aequitas Remove Liquidity: %.8f shares nonce:%d ts:%d", n.Betrag, n.Nonce, n.Zeit),
			[]unterschrift{{w, n.Sig}}, nil
	case "faucet":
		return fmt.Sprintf("Aequitas tUSD Faucet Claim: %s ts:%d", w, n.Zeit), []unterschrift{{w, n.Sig}}, nil
	case "escrow_recover":
		return "Aequitas: recover escrow " + w, []unterschrift{{w, n.Sig}}, nil
	case "unternehmen_eroeffnen":
		return unternehmenEroeffnenNachricht(w, to, tx.Name, tx.Kategorie, n.Zeit),
			[]unterschrift{{w, n.Sig}, {to, n.Sig2}}, nil
	case "unternehmen_mitinhaber":
		v := strings.ToLower(strings.TrimSpace(n.Von2))
		if v == "" {
			return "", nil, fmt.Errorf("Mitinhaber: kein bisher Verantwortlicher im Nachweis")
		}
		return unternehmenMitinhaberNachricht(w, to, n.Zeit),
			[]unterschrift{{to, n.Sig}, {v, n.Sig2}}, nil
	case "unternehmen_schliessen":
		return unternehmenSchliessenNachricht(w, n.Zeit), []unterschrift{{to, n.Sig}}, nil
	}
	return "", nil, fmt.Errorf("Art %q traegt keinen Nachweis", tx.Type)
}

// pruefeAuftragsNachweis: Nachweis einer Transaktion gegen den Block.
func pruefeAuftragsNachweis(tx *Transaction, blockZeit int64) error {
	if tx.Nachweis == nil {
		return fmt.Errorf("%w: %s ohne Nachweis", ErrUeberweisungNichtSigniert, tx.Type)
	}
	if hatNachweisZeit(tx.Type) {
		z := tx.Nachweis.Zeit
		if z < blockZeit-nachweisHoechstensAlt || z > blockZeit+nachweisHoechstensVoraus {
			return fmt.Errorf("%w: %s unterschrieben um %d, Block %d", ErrUeberweisungNichtSigniert, tx.Type, z, blockZeit)
		}
	}
	if nutztAuftragsNonce(tx.Type) && tx.Nachweis.Nonce < 0 {
		return fmt.Errorf("%w: negative Nonce", ErrUeberweisungNichtSigniert)
	}
	msg, sigs, err := auftragsNachricht(tx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUeberweisungNichtSigniert, err)
	}
	for _, u := range sigs {
		if err := pruefePersonalSignGemerkt(msg, u.sig, u.von); err != nil {
			return fmt.Errorf("%w: %s: Unterschrift von %s: %v", ErrUeberweisungNichtSigniert, tx.Type, u.von, err)
		}
	}
	return nil
}

// Stufe 1.2 auch hier: dieselbe Unterschrift nur einmal pruefen. Schluessel
// ist der Hash ueber Nachricht, Unterschrift und Unterzeichner -- ein Treffer
// heisst, genau diese drei wurden schon einmal als gueltig befunden.
var (
	personalSignGemerktMu sync.Mutex
	personalSignGemerkt   = map[[32]byte]struct{}{}
)

const personalSignGemerktGrenze = 1 << 16

func pruefePersonalSignGemerkt(msg, sig, von string) error {
	k := sha256.Sum256([]byte(msg + "\x00" + strings.ToLower(sig) + "\x00" + von))
	personalSignGemerktMu.Lock()
	_, ok := personalSignGemerkt[k]
	personalSignGemerktMu.Unlock()
	if ok {
		return nil
	}
	if err := verifyPersonalSign(msg, sig, von); err != nil {
		return err
	}
	personalSignGemerktMu.Lock()
	if len(personalSignGemerkt) >= personalSignGemerktGrenze {
		personalSignGemerkt = map[[32]byte]struct{}{} // selten; einfacher als ein Ring
	}
	personalSignGemerkt[k] = struct{}{}
	personalSignGemerktMu.Unlock()
	return nil
}

// auftragsNonce: Ergebnis der Vorpruefung fuer einen Auftrag mit Nonce.
type auftragsNonce struct {
	absender string
	nonce    int64
}

// pruefeAuftraegeImBlock prueft alle Nachweise eines Blocks parallel
// (Stufe 1.2) und gibt die Auftrags-Noncen in Blockreihenfolge zurueck. Bei
// mehreren Fehlern wird der mit dem kleinsten Index gemeldet.
func pruefeAuftraegeImBlock(txs []Transaction, blockZeit int64) ([]auftragsNonce, error) {
	var idx []int
	for i := range txs {
		if braucheNachweis(txs[i].Type) {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return nil, nil
	}
	fehler := make([]error, len(idx))
	workers := runtime.NumCPU()
	if workers > len(idx) {
		workers = len(idx)
	}
	var wg sync.WaitGroup
	var naechster atomic.Int64
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				k := int(naechster.Add(1) - 1)
				if k >= len(idx) {
					return
				}
				fehler[k] = pruefeAuftragsNachweis(&txs[idx[k]], blockZeit)
			}
		}()
	}
	wg.Wait()
	var out []auftragsNonce
	for k, err := range fehler {
		if err != nil {
			return nil, fmt.Errorf("Transaktion %d: %w", idx[k], err)
		}
		tx := &txs[idx[k]]
		if nutztAuftragsNonce(tx.Type) {
			out = append(out, auftragsNonce{absender: strings.ToLower(strings.TrimSpace(tx.Wallet)), nonce: tx.Nachweis.Nonce})
		}
	}
	return out, nil
}

// naechsteAuftragsNoncenFuerBlockLocked: wie naechsteNoncenFuerBlockLocked.
func (cs *ChainState) naechsteAuftragsNoncenFuerBlockLocked(ctx context.Context, liste []auftragsNonce) (map[string]int64, error) {
	stand := make(map[string]int64, len(liste))
	for _, u := range liste {
		naechste, bekannt := stand[u.absender]
		if !bekannt {
			cs.ensureAccountLoadedCtx(ctx, u.absender)
			if acc, ok := cs.accounts.Get(u.absender); ok {
				naechste = acc.NaechsteAuftragsNonce
			}
		}
		if u.nonce < naechste {
			return nil, fmt.Errorf("%w: Auftrags-Nonce %d von %s verbraucht (naechste erlaubte %d)",
				ErrUeberweisungNichtSigniert, u.nonce, u.absender, naechste)
		}
		stand[u.absender] = u.nonce + 1
	}
	return stand, nil
}

// setzeAuftragsNoncenLocked: die neuen Werte in den Zustand; gespeichert wird
// ueber den Kontensammler mit dem Block (oder sofort, wenn keiner da ist).
func (cs *ChainState) setzeAuftragsNoncenLocked(ctx context.Context, neu map[string]int64, sammler *kontenSammler) error {
	for a, n := range neu {
		acc, ok := cs.accounts.Get(a)
		if !ok || n <= acc.NaechsteAuftragsNonce {
			continue
		}
		acc.NaechsteAuftragsNonce = n
		cs.updateAccountLeafLocked(acc)
		if sammler != nil {
			sammler.hinzufuegen(acc)
			continue
		}
		if err := cs.saveAccountToDBCtx(ctx, acc); err != nil {
			return fmt.Errorf("NaechsteAuftragsNonce fuer %s speichern: %w", a, err)
		}
	}
	return nil
}

// merkeAuftragsNonceLocked: bei der ANNAHME, in derselben Transaktion wie der
// Auftrag (runAtomicWithOutbox). Lehnt eine verbrauchte Nonce ab -- der
// annehmende Knoten darf nie einen Block erzeugen, den die anderen verwerfen.
func (cs *ChainState) merkeAuftragsNonceLocked(ctx context.Context, tx Transaction) error {
	if tx.Nachweis == nil || !nutztAuftragsNonce(tx.Type) {
		return nil
	}
	w := strings.ToLower(strings.TrimSpace(tx.Wallet))
	acc, ok := cs.accounts.Get(w)
	if !ok {
		return fmt.Errorf("Auftrags-Nonce: Konto %s unbekannt", w)
	}
	if tx.Nachweis.Nonce < acc.NaechsteAuftragsNonce {
		return fmt.Errorf("nonce too low: %d (next allowed %d)", tx.Nachweis.Nonce, acc.NaechsteAuftragsNonce)
	}
	acc.NaechsteAuftragsNonce = tx.Nachweis.Nonce + 1
	cs.updateAccountLeafLocked(acc)
	if !cs.useDB {
		return nil
	}
	return cs.saveAccountToDBCtx(ctx, acc)
}

// nachweisFuerAnnahme: der Nachweis, den ein annehmender Knoten in die
// Transaktion schreibt -- erst ab dem Vorlauf vor der Aktivierung, damit
// aeltere Knoten bis dahin keine Bloecke mit unbekanntem Feld sehen.
func nachweisFuerAnnahme(n Auftragsnachweis) *Auftragsnachweis {
	if !signierteUeberweisungenAufnehmen(nowUnix()) {
		return nil
	}
	return &n
}

// pruefeVerantwortlichLocked: ist n.Von2 im Augenblick der Ausfuehrung
// verantwortlich fuer das offene Unternehmen u? Fuer Mitinhaber (der bisher
// Verantwortliche stimmt zu) und Schliessen (ein Verantwortlicher schliesst).
func (cs *ChainState) pruefeVerantwortlichLocked(u string, n *Auftragsnachweis) error {
	if n == nil || n.Von2 == "" {
		return fmt.Errorf("%w: kein Verantwortlicher im Nachweis", ErrUeberweisungNichtSigniert)
	}
	w := cs.wirt()
	w.mu.Lock()
	e := w.offenesUnternehmenLocked(strings.ToLower(u))
	ok := e != nil && e.istVerantwortlich(strings.ToLower(n.Von2))
	w.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w: %s ist nicht verantwortlich fuer %s", ErrUeberweisungNichtSigniert, n.Von2, u)
	}
	return nil
}

// pruefeAuftragsNonce: vor dem Annehmen, nur lesend -- eine verbrauchte
// Nonce wird abgelehnt, bevor irgendetwas geaendert ist. Die verbindliche
// Pruefung macht merkeAuftragsNonceLocked in der Transaktion des Auftrags.
func (cs *ChainState) pruefeAuftragsNonce(wallet string, n *Auftragsnachweis) error {
	if n == nil {
		return nil
	}
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	acc, ok := cs.accounts.Get(strings.ToLower(wallet))
	if ok && n.Nonce < acc.NaechsteAuftragsNonce {
		return fmt.Errorf("nonce too low: %d (next allowed %d)", n.Nonce, acc.NaechsteAuftragsNonce)
	}
	return nil
}
