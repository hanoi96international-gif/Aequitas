package keeper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// EINLOESEN: der Rueckweg einer Quanten-Note in AEQ (15.11.2026).
//
// Bis hierher war die Bruecke einbahnig: wer AEQ in eine Note gab, bekam sie
// nie zurueck -- die Deckung lag fuer immer im Topf. Eine Deckung, die nie
// ausgezahlt wird, deckt nichts.
//
// WARUM NICHT EINFACH "WER DIE NOTE HAT, BEKOMMT DAS GELD"
//
// Die Kette ist klassisch: sie kann keinen Quantentest ausfuehren und also
// nicht sehen, wer eine Note gerade in Haenden haelt. Die Seriennummer ist
// oeffentlich -- wer sie nennt, beweist nichts. Was die Kette pruefen kann,
// ist eine EINMAL-UNTERSCHRIFT (one-shot signature; Amos, Georgiou, Kiayias,
// Zhandry 2020; Konstruktion im Standardmodell: Shmueli, Zhandry 2025): der
// Halter des Quantenzustands kann damit genau EINE Nachricht klassisch
// unterschreiben -- hier "zahle die Note K an Adresse X aus" --, und der
// Zustand ist danach verbraucht. Wer die Note weitergegeben hat, kann sie
// nicht mehr einloesen; wer sie hat, nur einmal.
//
// SCHLAFEND, FAIL-CLOSED
//
// Die Pruefung einer Einmal-Unterschrift haengt am Parametersatz der Note
// (quantenEinmalPruefer). Heute kennt der Knoten fuer KEINEN Satz eine --
// jede Einloesung wird abgewiesen, bei der Annahme wie beim Nachspielen. Ein
// Pruefer kommt nur mit einer Aenderung im Code hinzu (Konsensregel, eigene
// Sicherheitspruefung, externe Begutachtung: docs/QUANTENGELD_PRUEFAUFTRAG.md).
// Bis dahin ist alles andere fertig und getestet: Urteil, Auszahlung,
// Register, Nachspielen.
//
// WAS JEDER KNOTEN PRUEFT
//
//   - die Note ist angemeldet, ihr Satz hat einen Pruefer;
//   - die Einmal-Unterschrift gilt fuer quantenEinloesenNachricht(K, X);
//   - der Betrag im Block ist GENAU der Wert der Note jetzt (Anteile * Topf /
//     alle Anteile, quanten_geld.go) -- die Umlaufabgabe, die der Topf seit
//     der Anmeldung gezahlt hat, ist darin schon abgezogen;
//   - danach ist die Note aus dem Register; ein zweites Einloesen findet
//     sie nicht mehr.
//
// Ausgezahlt wird ohne Gebuehr an X (wie das Grundeinkommen aus seinem
// Topf); Vermoegensgrenze und Umlaufabgabe gelten fuer X danach wie fuer
// jedes Konto.

const (
	quantenEinloesenTxArt = "quanten_auszahlung"
	// v2: anders als der Entwurf in quanten_note.go (v1, mit Betrag) deckt die
	// Unterschrift KEINEN Betrag -- der Wert einer Note sinkt jeden Tag mit
	// der Umlaufabgabe; eine Unterschrift ueber einen festen Betrag waere nach
	// der naechsten Runde falsch und verbrauchte die Note umsonst. Die Kette
	// rechnet den Wert bei der Auszahlung.
	quantenDomaeneEinloesenV2 = "aequitas-quanten-einloesen-v2"
	// quantenEinmalSigMax: groesste Einmal-Unterschrift (hex-dekodiert).
	quantenEinmalSigMax = 64 << 10
)

// quantenEinmalPruefer: je Parametersatz (Name) die Pruefung einer
// Einmal-Unterschrift: serie ist die Seriennummer der Note (ihr oeffentlicher
// Schluessel; die Kette prueft vorher, dass sie zu K gehoert), nachricht der
// unterschriebene Text, sig die Unterschrift. LEER: kein Satz kann eingeloest
// werden. Nur im Code erweiterbar (siehe oben); Tests setzen einen Pruefer
// und raeumen ihn ab.
var quantenEinmalPruefer = map[string]func(serie, nachricht, sig []byte) bool{}

// quantenEinloesenNachricht: was die Einmal-Unterschrift deckt.
func quantenEinloesenNachricht(k, empfaenger string) string {
	return fmt.Sprintf("%s|chain:%d|%s|%s", quantenDomaeneEinloesenV2, eingefroreneChainID, strings.ToLower(k), strings.ToLower(empfaenger))
}

// quantenEinloesenTxHash: der TxHash haengt an Note und Empfaenger -- eine
// Note kann nur einmal eingeloest werden, ein zweiter Versuch hat denselben.
func quantenEinloesenTxHash(k, empfaenger string) string {
	h := sha256.Sum256([]byte(quantenEinloesenNachricht(k, empfaenger)))
	return "0x" + hex.EncodeToString(h[:])
}

// quantenEinloesung: das Urteil ueber eine Einloesung.
type quantenEinloesung struct {
	regel, text string
	note        quantenNoteEintrag
	wertMikro   int64
}

// quantenEinloesenUrteilLocked: darf tx die Note einloesen, und was ist sie
// wert? Dieselbe Pruefung fuer Annahme und Nachspielen. Caller haelt cs.mu.
func (cs *ChainState) quantenEinloesenUrteilLocked(tx *Transaction, blockZeit int64) quantenEinloesung {
	nein := func(regel, format string, args ...interface{}) quantenEinloesung {
		return quantenEinloesung{regel: regel, text: fmt.Sprintf(format, args...)}
	}
	q := tx.Quanten
	if !quantenGeldAktiv(blockZeit) || q == nil || tx.Type != quantenEinloesenTxArt {
		return nein("quanten_form", "keine Einloesung")
	}
	empf := strings.ToLower(strings.TrimSpace(tx.Wallet))
	if !kanonischeAdresse(empf) || istSystemAdresse(empf) || tx.To != "" ||
		q.Sig != "" || q.Ordnung != "" || q.Ab != 0 || len(q.Stimmen) != 0 ||
		!istHash64(q.K) || !quantenSatzGueltig(q.Satz) || q.Anteile < 0 {
		return nein("quanten_form", "Einloesung nicht in gueltiger Form")
	}
	serie, ok := quantenSerieAusHex(q.Serie)
	if !ok {
		return nein("quanten_form", "Seriennummer nicht in gueltiger Form")
	}
	if len(q.Einmal) == 0 || len(q.Einmal) > 2*quantenEinmalSigMax || len(q.Einmal)%2 != 0 || !kleinHex(q.Einmal) {
		return nein("quanten_form", "Einmal-Unterschrift fehlt oder nicht in gueltiger Form")
	}
	reg := cs.quantenGeldJetzt()
	e, _, da := reg.note(q.K)
	if !da {
		return nein("quanten_unbekannt", "Note %s ist nicht angemeldet (oder schon eingeloest)", q.K[:12])
	}
	if e.Satz != q.Satz || quantenSerieSchluessel(q.Satz, e.Von, serie) != q.K {
		return nein("quanten_form", "Seriennummer und Satz gehoeren nicht zu Note %s", q.K[:12])
	}
	pruefer := quantenEinmalPruefer[e.Satz]
	if pruefer == nil {
		return nein("quanten_einloesen_unmoeglich", "fuer Satz %q gibt es noch keine Pruefung einer Einmal-Unterschrift -- Einloesen ist noch nicht moeglich", e.Satz)
	}
	sig, _ := hex.DecodeString(q.Einmal)
	if !pruefer(serie, []byte(quantenEinloesenNachricht(q.K, empf)), sig) {
		return nein("quanten_unterschrift", "Einmal-Unterschrift der Note gilt nicht fuer diese Auszahlung")
	}
	wert := quantenWert(e.Anteile, cs.quantenTopfMikroLocked(), reg.summeAnteile())
	if wert <= 0 {
		return nein("quanten_deckung", "Note ist nichts mehr wert")
	}
	return quantenEinloesung{note: e, wertMikro: wert}
}

// applyQuantenEinloesenLocked: Auszahlung aus dem Topf an den Empfaenger und
// die Note aus dem Register. erwartet: beim Nachspielen der Betrag im Block
// (muss genau stimmen), bei der Annahme 0. Fehler: nichts angewandt.
func (cs *ChainState) applyQuantenEinloesenLocked(ctx context.Context, tx *Transaction, blockZeit int64, erwartet int64) (quantenEinloesung, error) {
	u := cs.quantenEinloesenUrteilLocked(tx, blockZeit)
	if u.regel != "" {
		return u, fmt.Errorf("quanten_einloesen %s: %s", u.regel, u.text)
	}
	if erwartet != 0 && erwartet != u.wertMikro {
		return u, fmt.Errorf("quanten_einloesen: Block zahlt %d Mikro-AEQ aus, die Note ist %d wert", erwartet, u.wertMikro)
	}
	if tx.Quanten.Anteile != 0 && tx.Quanten.Anteile != u.note.Anteile {
		return u, fmt.Errorf("quanten_einloesen: Block nennt %d Anteile, die Note hat %d", tx.Quanten.Anteile, u.note.Anteile)
	}
	empf := strings.ToLower(strings.TrimSpace(tx.Wallet))
	topf, ok := cs.accounts.Get(quantenTopfAddr)
	betrag := Decimal(u.wertMikro)
	if !ok || topf.Balance < betrag {
		return u, fmt.Errorf("quanten_einloesen: Topf deckt %d Mikro-AEQ nicht: %w", u.wertMikro, ErrZustandLehntAb)
	}
	cs.ensureAccountLoadedCtx(ctx, empf)
	acc, da := cs.accounts.Get(empf)
	if !da {
		acc = &AccountState{Address: empf}
		cs.accounts.Set(empf, acc)
	}
	topf.Balance = topf.Balance.Sub(betrag)
	acc.Balance = acc.Balance.Add(betrag)
	for _, a := range []*AccountState{topf, acc} {
		cs.updateAccountLeafLocked(a)
		if err := cs.saveAccountToDBCtx(ctx, a); err != nil {
			return u, fmt.Errorf("quanten_einloesen: %w", err)
		}
	}
	if err := cs.quantenGeldSetzenLocked(ctx, cs.quantenGeldJetzt().ohneNote(u.note.K)); err != nil {
		return u, err
	}
	return u, nil
}

// quantenEinloesenAnnehmen: die Einloesung in EINER Datenbank-Transaktion.
func (cs *ChainState) quantenEinloesenAnnehmen(tx Transaction) (Transaction, error) {
	empf := strings.ToLower(strings.TrimSpace(tx.Wallet))
	var fertig Transaction
	err := cs.runAtomicWithOutbox([]string{quantenTopfAddr, empf}, false, func(ctx context.Context) (Transaction, error) {
		at := nowUnix()
		u, err := cs.applyQuantenEinloesenLocked(ctx, &tx, at, 0)
		if err != nil {
			return Transaction{}, err
		}
		q := *tx.Quanten
		q.Anteile = u.note.Anteile
		tx.Quanten = &q
		tx.Amount = Decimal(u.wertMikro).Float()
		tx.BuchAt = buchStempel(at)
		fertig = tx
		return tx, nil
	})
	return fertig, err
}

// handleQuantenEinloesen: POST {"k", "satz", "serie", "empfaenger", "einmal"} -- loest
// eine Note ein. Jeder Knoten prueft beim Nachspielen dasselbe. Heute fuer
// jeden Satz abgewiesen (quantenEinmalPruefer leer).
func (a *APIServer) handleQuantenEinloesen(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodPost {
		jsonError(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	jetzt := time.Now().Unix()
	if !quantenGeldAktiv(jetzt) {
		jsonError(w, "quantum money not active", http.StatusConflict)
		return
	}
	if !burstErlaubt("quanten-einloesen:"+erneuerungsAbsender(r), burstQuantenJeIP, burstFenster) {
		jsonError(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	var req struct {
		K          string `json:"k"`
		Satz       string `json:"satz"`
		Serie      string `json:"serie"`
		Empfaenger string `json:"empfaenger"`
		Einmal     string `json:"einmal"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*quantenEinmalSigMax+4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	k := strings.ToLower(strings.TrimSpace(req.K))
	empf := strings.ToLower(strings.TrimSpace(req.Empfaenger))
	tx := Transaction{Type: quantenEinloesenTxArt, Wallet: empf, TxHash: quantenEinloesenTxHash(k, empf),
		Quanten: &QuantenAngaben{Satz: req.Satz, Serie: req.Serie, K: k, Einmal: strings.ToLower(strings.TrimSpace(req.Einmal))}}
	fertig, err := a.state.quantenEinloesenAnnehmen(tx)
	if err != nil {
		if istWiederholbareAnnahmeAblehnung(err) {
			jsonError(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		jsonError(w, "redemption failed: "+err.Error(), http.StatusConflict)
		return
	}
	a.state.SyncBalancesToEVM(V7_CONTRACT_ADDR, empf, quantenTopfAddr)
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "tx_hash": fertig.TxHash, "ausgezahlt_aeq": fertig.Amount})
}
