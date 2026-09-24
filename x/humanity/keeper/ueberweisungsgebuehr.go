package keeper

import (
	"context"
	"math"
	"sync/atomic"
)

// Ueberweisungsgebuehr: dieselbe auf JEDER Ueberweisung, ganz ans
// Grundeinkommen (Entscheidung 24.09.2026, "das fairste Geld der Welt").
//
// # WARUM
//
// Bis dahin zahlte nur die Token-Ueberweisung ueber den V7-Vertrag eine
// Gebuehr. Die gewoehnliche Sendung -- die, die die App benutzt -- war frei.
// Die Gebuehr, die Website und Whitepaper beschreiben, zahlte also nur, wer den
// Umweg nicht kannte: Ungerechtigkeit durch Unwissen. Und das Grundeinkommen,
// das ausschliesslich aus Gebuehren und Umverteilung lebt (es wird kein Geld
// dafuer geschoepft), hing fast nur an den Swap-Gebuehren -- jeder Handel
// ausserhalb der Kette machte es kleiner. Mit einer Gebuehr auf jeder
// Ueberweisung traegt jede Bewegung von AEQ gleich bei, egal wo gehandelt wird.
//
// # WIE
//
//   - OBENDRAUF: der Empfaenger bekommt genau den gesendeten Betrag, der
//     Absender zahlt Betrag + Gebuehr. Ein Preis von 10 AEQ bringt 10 AEQ --
//     wichtig fuer jeden, der verkauft.
//   - 0,1 % fuer alle. Aufschlag nur fuer grosse Guthaben, gemessen am fairen
//     Anteil (1.000 AEQ, registrationGrant): ab dem 5-fachen +0,1 %, ab dem
//     10-fachen +0,5 %, ab dem 20-fachen +1 %. Frueher hing der Aufschlag am
//     Anteil an der GESAMTEN Geldmenge -- in einem kleinen Netz hielt damit
//     jeder "viel" und zahlte ihn (bei 18 Menschen 0,6 % statt 0,1 % auf jede
//     gewoehnliche Ueberweisung).
//   - Die Gutschrift ans Grundeinkommen kommt, wenn die Ueberweisung in einem
//     gespeicherten Block steht (gebuehrenInsGrundeinkommen) -- auf dem
//     erzeugenden Knoten genau wie auf jedem nachspielenden
//     (applyTransferDeltaLockedSammelnd): so sind beide Block fuer Block
//     gleich, und die schnellen Annahmewege beruehren keinen Topf.
//   - Die Transaktion traegt die Gebuehr (Transaction.Gebuehr).
//
// Die interne Transfer()-Funktion des Protokolls (keine Aufrufer von aussen)
// und die Ausschuettungen sind keine Ueberweisungen eines Menschen und zahlen
// nichts.

const (
	ueberweisungsGebuehrBps = 10 // 0,1 %
)

// ueberweisungsGebuehrFuer: die Gebuehr fuer betrag bei einem Guthaben des
// Absenders (vor der Ueberweisung). Auf Mikro-AEQ gerundet.
func ueberweisungsGebuehrFuer(betrag, guthaben float64) float64 {
	if betrag <= 0 || math.IsNaN(betrag) || math.IsInf(betrag, 0) {
		return 0
	}
	bps := float64(ueberweisungsGebuehrBps)
	switch fair := registrationGrant; {
	case guthaben >= 20*fair:
		bps += 100
	case guthaben >= 10*fair:
		bps += 50
	case guthaben >= 5*fair:
		bps += 10
	}
	return round6(betrag * bps / 10_000)
}

// gebuehrenSumme: Summe der Ueberweisungsgebuehren in txs.
func gebuehrenSumme(txs []Transaction) float64 {
	s := NewDecimal(0)
	for _, tx := range txs {
		if tx.Type == "transfer" && tx.Gebuehr > 0 {
			s = s.Add(NewDecimal(tx.Gebuehr))
		}
	}
	return s.Float()
}

// gebuehrenInsGrundeinkommen: die Gebuehren eines gerade gespeicherten,
// selbst erzeugten Blocks dem UBI-Topf gutschreiben -- und den Topf sofort
// speichern, damit ein Absturz sie nicht verliert (die Absender sind schon
// belastet, dauerhaft).
func (cs *ChainState) gebuehrenInsGrundeinkommen(summe float64) {
	if summe <= 0 {
		return
	}
	cs.mu.Lock()
	cs.ensureAccountLoadedCtx(context.Background(), ubiPoolAddr)
	ubiAcc, ok := cs.accounts.Get(ubiPoolAddr)
	if !ok {
		ubiAcc = &AccountState{Address: ubiPoolAddr}
		cs.accounts.Set(ubiPoolAddr, ubiAcc)
	}
	ubiAcc.Balance = ubiAcc.Balance.Add(NewDecimal(summe))
	cs.updateAccountLeafLocked(ubiAcc)
	if cs.useDB {
		cs.markPoolAccountsDirtyLocked()
	}
	cs.mu.Unlock()
	gebuehrenGutgeschriebenMicro.Add(NewDecimal(summe).Micro())
	if cs.useDB {
		cs.FlushPoolAccountsNow()
	}
}

// gebuehrenGutgeschriebenMicro: seit dem Start dieses Knotens aus eigenen
// Bloecken ans Grundeinkommen gutgeschrieben (Mikro-AEQ, fuer den Stand).
var gebuehrenGutgeschriebenMicro atomic.Int64
