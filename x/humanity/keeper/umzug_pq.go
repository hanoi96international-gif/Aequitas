package keeper

// Quantensicherheit Stufe 3.6 (docs/QUANTENSICHERHEIT.md): das Menschsein --
// und mit ihm das Grundeinkommen -- auf eine PQ-Adresse umziehen.
//
// # WARUM
//
// Eine PQ-Adresse (pq_ueberweisung.go) gibt nur mit einer ML-DSA-65-
// Unterschrift aus. Das Menschsein (is_human, Nullifier, Lebensnachweis,
// Staffel) stand aber bisher immer an einer gewoehnlichen Wallet: alles, was
// der Mensch fuer sein Menschsein unterschreibt -- den Umzugsantrag der neuen
// Wallet, den Widerruf der alten, den Wiederherstellungscode -- war eine
// EIP-191-Unterschrift mit secp256k1. Eine PQ-Adresse hat keinen solchen
// Schluessel; sie konnte das Menschsein weder aufnehmen noch schuetzen.
//
// # WAS DAZUKOMMT
//
// Dieselben drei Auftraege (umzug_ankuendigen, umzug_widerruf, umzug_code)
// duerfen statt der EIP-191-Unterschrift (UmzugAngaben.Sig) eine ML-DSA-65-
// Unterschrift (UmzugAngaben.PQ) ueber GENAU DIESELBE Nachricht tragen --
// gueltig nur, wenn der Schluessel genau die unterschreibende Adresse ergibt
// (pqAdresseAusSchluessel). Genau eins von beiden, nie beide.
//
//   - Antrag: die NEUE Wallet unterschreibt -- so kann sie eine PQ-Adresse
//     sein. Das Gesicht bleibt Pflicht (v3-Bescheinigung des Coordinators,
//     Zeugen, Code oder 14 Tage Frist): das Menschsein zieht nie ohne das
//     Gesicht des Menschen um. Ein Dieb, der nur den secp256k1-Schluessel der
//     alten Wallet hat, holt es also auch nicht auf "seine" PQ-Adresse.
//   - Widerruf: die ALTE Wallet unterschreibt -- so kann eine PQ-Adresse
//     einen Umzug ohne Code von sich weg aufhalten, wie jede Wallet.
//   - Code: die Wallet des Menschen unterschreibt -- so bekommt das
//     Menschsein auf der PQ-Adresse wieder einen Wiederherstellungscode.
//
// Das Grundeinkommen folgt dem Menschsein: die Tagesrunde zahlt an jedes
// Konto mit is_human (distributeUBIPoolLocked); ausgeben geht von der
// PQ-Adresse mit einer PQ-Ueberweisung. Das Guthaben der alten Wallet zieht
// NICHT mit -- Geld bewegt nur eine Unterschrift (eine gewoehnliche
// Ueberweisung in den Tresor).
//
// Die Nachrichten haben eigene Domaenen (aequitas-umzug-*-v1, mit Kette);
// keine andere ML-DSA-Nachricht der Kette beginnt so. Eine Unterschrift fuer
// eine Ueberweisung, eine Pruefer-Stimme oder eine Dienst-Freigabe passt nie
// auf einen Umzug und umgekehrt.
//
// # REGELN (jeder Knoten prueft selbst, beim Nachspielen und bei der Annahme)
//
//   - Ab umzugPQAbUnix (Blockzeit; bei der Annahme die Uhrzeit). Schlafend
//     (MaxInt64), bis alle Knoten diesen Code haben -- ein aelterer Knoten
//     saehe nur die fehlende EIP-191-Unterschrift und wiese den Block ab.
//     Waechter: .github/neustart/umzug-pq-nicht-verpasst.sh.
//   - Feste Groessen: Schluessel genau 1.952, Unterschrift genau 3.309 Byte
//     (als Hex, klein), sonst ab vor jeder Rechnung. Billig vor teuer:
//     Form, Adresse aus dem Schluessel, dann ML-DSA.
//   - pq_zusage traegt nie eine PQ-Unterschrift: die Zusage schuetzt eine
//     secp256k1-Wallet, eine PQ-Adresse braucht keine.
//
// # GRENZEN
//
// Ein Auftrag wird rund 10,5 KB groesser. Er zaehlt im Block zu den
// pqUeberweisungenJeBlock (500) schweren PQ-Nachweisen (pqNachweisSchwer),
// beim Erzeuger wie bei jedem Pruefenden. Koerper: Code und Widerruf
// umzugKoerperMax (32 KiB), Antrag umzugAntragKoerperMax (64 KiB, mit
// Bindung und den pq:-Eintraegen der Zeugen).
//
// # NOCH NICHT (eigene Schritte)
//
// Auf der PQ-Adresse fehlen weiter alle Wege, die eine EIP-191-Unterschrift
// des Menschen verlangen: Pruefer werden (B1b), Waechter setzen, Treuhand
// zurueckholen, PQ-Zusage. Fuer das Grundeinkommen braucht es keinen davon.

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sync/atomic"
	"time"
)

const (
	// umzugPQAbUnix: 2026-11-15 19:00 UTC (Entscheidung des Betreibers vom
	// 15.11.2026, "Stichtag heute"). C1 laeuft seit #519 mit diesem Code,
	// Coordinator ec33002 nimmt die Tresor-Unterschrift, App 1.21.0 bietet
	// den Weg an, sobald die Kette den Stichtag nennt. Die Regel erlaubt nur
	// (ein neuer Weg), sie verlangt nichts: wer nichts tut, merkt nichts.
	// Waechter: .github/neustart/umzug-pq-nicht-verpasst.sh.
	umzugPQAbUnix int64 = 1794769200
)

// umzugPQAbOverride: nur fuer Tests (0 = Konstante gilt).
var umzugPQAbOverride atomic.Int64

func umzugPQAb() int64 {
	if v := umzugPQAbOverride.Load(); v != 0 {
		return v
	}
	return umzugPQAbUnix
}

func umzugPQAktiv(t int64) bool { return t >= umzugPQAb() }

// PQKonto: die ML-DSA-65-Unterschrift einer PQ-Adresse an Stelle von EIP-191
// (Hex, klein, ohne 0x).
type PQKonto struct {
	Pub string `json:"pub"`
	Sig string `json:"sig"`
}

var errPQKonto = errors.New("PQ-Unterschrift des Kontos")

// pqKontoForm: feste Groessen, klein -- vor jeder Rechnung.
func pqKontoForm(p *PQKonto) bool {
	return p != nil && len(p.Pub) == pqPubHex && kleinHex(p.Pub) && len(p.Sig) == pqSigHex && kleinHex(p.Sig)
}

// pqKontoPruefen: unterschreibt der ML-DSA-65-Schluessel, der genau die
// Adresse wallet ergibt, die Nachricht?
func pqKontoPruefen(wallet, nachricht string, p *PQKonto) error {
	if !pqKontoForm(p) {
		return fmt.Errorf("%w: Schluessel oder Unterschrift in falscher Form", errPQKonto)
	}
	// Billig vor teuer: die Adresse aus den Bytes des Schluessels, dann ML-DSA.
	roh, err := hex.DecodeString(p.Pub)
	if err != nil {
		return fmt.Errorf("%w: Schluessel unlesbar", errPQKonto)
	}
	if pqAdresseAusSchluessel(roh) != wallet {
		return fmt.Errorf("%w: %s gehoert nicht zu diesem Schluessel", errPQKonto, kurzAdresse(wallet))
	}
	if !mldsaPruefen(p.Pub, p.Sig, nachricht) {
		return fmt.Errorf("%w: Unterschrift passt nicht", errPQKonto)
	}
	return nil
}

// umzugSigForm: traegt der Auftrag genau eine Unterschrift in fester Form --
// EIP-191 (Sig) oder, ab dem Stichtag, ML-DSA (PQ)? Zur Blockzeit t.
func umzugSigForm(u *UmzugAngaben, t int64) bool {
	if u.PQ != nil {
		return u.Sig == "" && umzugPQAktiv(t) && pqKontoForm(u.PQ)
	}
	return kanonischeSignatur(u.Sig)
}

// umzugUnterschriftPruefen: hat wallet die Nachricht unterschrieben -- mit
// EIP-191 oder ML-DSA, je nachdem, was der Auftrag traegt? Die Form prueft
// umzugSigForm vorher.
func umzugUnterschriftPruefen(u *UmzugAngaben, nachricht, wallet string) error {
	if u.PQ != nil {
		return pqKontoPruefen(wallet, nachricht, u.PQ)
	}
	return pruefePersonalSignGemerkt(nachricht, u.Sig, wallet)
}

// umzugPQErlaubt: eine PQ-Unterschrift gehoert nur an Antrag, Widerruf und
// Code -- an jeder anderen Art waere sie ungeprueft.
func umzugPQErlaubt(tx *Transaction) bool {
	if tx.Umzug == nil || tx.Umzug.PQ == nil {
		return true
	}
	switch tx.Type {
	case umzugAnkuendigenTxArt, umzugWiderrufTxArt, umzugCodeTxArt:
		return true
	}
	return false
}

// pqNachweisSchwer: traegt die Transaktion einen ML-DSA-Nachweis von rund
// 10,5 KB, der zu pqUeberweisungenJeBlock zaehlt?
func pqNachweisSchwer(tx *Transaction) bool {
	return tx.PQ != nil || (tx.Umzug != nil && tx.Umzug.PQ != nil)
}

// umzugPQAnfrage: was die Endpunkte an Stelle von "sig" annehmen.
type umzugPQAnfrage struct {
	PQPub string `json:"pq_pub"`
	PQSig string `json:"pq_sig"`
}

// angaben: die Unterschrift fuer UmzugAngaben aus "sig" oder "pq_pub"/"pq_sig"
// -- genau eins. ok=false: keine, beide oder falsche Form (400).
func (q umzugPQAnfrage) angaben(sigRoh string, jetzt int64) (sig string, pq *PQKonto, ok bool) {
	if q.PQPub == "" && q.PQSig == "" {
		sig = kanonischeSignaturVersuch(sigRoh)
		return sig, nil, kanonischeSignatur(sig)
	}
	if sigRoh != "" || !umzugPQAktiv(jetzt) {
		return "", nil, false
	}
	pq = &PQKonto{Pub: q.PQPub, Sig: q.PQSig}
	return "", pq, pqKontoForm(pq)
}

// umzugPQAnnehmbar: vor dem Stichtag beantwortet ein Endpunkt eine Anfrage
// mit PQ-Unterschrift mit 409 (wie andere schlafende Wege). false = Antwort
// geschrieben.
func umzugPQAnnehmbar(w http.ResponseWriter, q umzugPQAnfrage) bool {
	if (q.PQPub != "" || q.PQSig != "") && !umzugPQAktiv(time.Now().Unix()) {
		jsonError(w, "PQ signatures for wallet moves not active yet", http.StatusConflict)
		return false
	}
	return true
}

// umzugUnterschriftVorab: die Pruefung vor der Sperre (ohne den Merkspeicher
// der Unterschriften, wie bisher verifyPersonalSign) -- beim Nachspielen
// prueft jeder Knoten sie mit umzugUnterschriftPruefen noch einmal.
func umzugUnterschriftVorab(u *UmzugAngaben, nachricht, wallet string) error {
	if u.PQ != nil {
		return pqKontoPruefen(wallet, nachricht, u.PQ)
	}
	return verifyPersonalSign(nachricht, u.Sig, wallet)
}

// umzugPQStatus: fuer /api/status ("umzug_pq"). ab fehlt, solange die Regel
// schlaeft -- der Waechter liest das so.
func umzugPQStatus() map[string]interface{} {
	m := map[string]interface{}{}
	if ab := umzugPQAb(); ab != math.MaxInt64 {
		m["ab"] = ab
	}
	return m
}
