package keeper

import (
	"context"
	"strings"
)

// Grundeinkommens-Runde nachrechnen (Audit 2026-09-29, K-2, Schritt 3).
//
// # WARUM
//
// erhaltung.go stellt sicher, dass eine Runde nicht mehr auszahlt, als im Topf
// liegt. WER wieviel bekommt, glaubte jeder Knoten dem Produzenten: Er konnte
// den ganzen Topf einer einzigen Wallet geben, einer Wallet, die kein Mensch
// ist, einen Menschen zweimal bedenken und einen anderen auslassen, oder
// allen weniger zahlen als ihnen zusteht. Erhaltung bemerkt keinen dieser
// Faelle -- die Summe stimmt jedes Mal.
//
// # WAS
//
// Die Regel des Erzeugers (distributeUBIPoolLocked) ist einfach genug, um sie
// beim Nachspielen vollstaendig zu pruefen: jeder registrierte Mensch genau
// einmal, alle denselben Betrag, und der Betrag ist floor6(Topf / Menschen),
// wobei Topf der Stand nach der Demurrage aller Menschen ist.
//
//   - ubi_kein_mensch: Empfaenger ist im eigenen Zustand kein Mensch.
//   - ubi_doppelt:     derselbe Empfaenger zweimal in einer Runde.
//   - ubi_ungleich:    ein Betrag weicht vom ersten der Runde ab.
//   - ubi_empfaenger:  Zahl der Empfaenger ungleich Zahl der Menschen
//     (beim Abschluss; zusammen mit den beiden ersten Regeln heisst das:
//     genau die Menschen, jeder genau einmal).
//   - ubi_anteil:      der Betrag ist nicht floor6(Topf / Menschen).
//   - demurrage_nach_umstellung: seit der Umlaufsicherung (wirtschaft.go)
//     gibt es keine Demurrage mehr -- effectiveBalance gibt das Guthaben
//     unveraendert zurueck, settleDemurrageLocked liefert 0. Ein
//     FromDemurrageLost/ToDemurrageLost > 0 ist dann erfunden: es nimmt einem
//     Konto Geld und gibt es dem Topf.
//
// Topf und Menschen werden beim ERSTEN Grundeinkommen der Runde festgehalten,
// vor dessen Gutschrift -- genau der Augenblick, in dem der Erzeuger teilt.
// Die Demurrage, die der Erzeuger vorab in den Topf rechnet, kommt beim
// Nachspielen erst mit den Gutschriften; sie wird mitgezaehlt und beim
// Abschluss dazugerechnet. Was waehrend der Gutschriften ueber die
// Vermoegensgrenze in den Topf fliesst, aendert den Anteil nicht und zaehlt
// deshalb nicht.
//
// # NEUSTART
//
// Der Stand liegt im Speicher, wie bei erhaltung.go. Ein Knoten, der mitten in
// einer Runde neu startet, sieht ihren Anfang nicht; er prueft dann bis zur
// naechsten Rundengrenze nur, was ohne Anfang geht (Mensch, doppelt, gleich)
// und laesst Zahl und Anteil aus. Er zaehlt zu wenig, nie zu viel -- auch
// im strengen Modus haelt ein Neustart die Kette nicht an.
//
// # STUFE
//
// Wie Schritt 1 und 2: bis nachrechnenStrengAbUnix nur BEOBACHTEN.

// ubiRundePruefung: Stand der laufenden Grundeinkommens-Runde beim
// Nachspielen. Unter cs.mu; im Rueckroll-Snapshot (als Kopie, siehe kopie).
type ubiRundePruefung struct {
	// unsicher: der Anfang der laufenden Runde ist nicht beobachtet
	// (Neustart). Bis zur naechsten Rundengrenze keine Zahl-/Anteilspruefung.
	unsicher bool
	aktiv    bool
	// Beim ersten Grundeinkommen der Runde festgehalten.
	topfStart int64 // Mikro-AEQ
	menschen  int64
	anteil    int64 // Mikro-AEQ, Betrag der ersten Gutschrift
	// Laufend.
	demurrage int64 // Mikro-AEQ, Summe FromDemurrageLost
	n         int64
	// Nur registrierte Menschen, also hoechstens so viele Eintraege wie
	// Menschen -- egal wieviele Bloecke eine offene Runde fortsetzen.
	empfaenger map[string]int64 // Wallet -> laufende Nummer in der Runde
}

// kopie fuer den Rueckroll-Snapshot, in O(1): die Empfaengermenge wird
// geteilt, nicht kopiert -- eine Kopie je Block kostete waehrend einer Runde
// ueber viele Bloecke O(Menschen) je Block. Jeder Eintrag traegt seine
// laufende Nummer n; zurueckgerollt wird mit zurueck, das alles entfernt, was
// nach dem Snapshot hinzukam.
func (r ubiRundePruefung) kopie() ubiRundePruefung { return r }

// zurueck: Stand eines Snapshots wiederherstellen. Die geteilte Menge kann
// Empfaenger enthalten, die erst der zurueckgewiesene Block eingetragen hat
// (Nummer > r.n); sie werden entfernt. Begann im zurueckgewiesenen Block eine
// neue Runde, ist deren Menge ein anderes Objekt und faellt mit weg.
// Nur beim Zurueckrollen, also selten; dann O(Groesse der Menge).
func (r ubiRundePruefung) zurueck() ubiRundePruefung {
	for k, nr := range r.empfaenger {
		if nr > r.n {
			delete(r.empfaenger, k)
		}
	}
	return r
}

// demurrageUmstellungPuffer: die Umstellung gilt beim Erzeuger nach SEINER
// Uhr (nowUnix in effectiveBalance), der Block traegt seine Blockzeit. Eine
// Stunde Abstand, damit kein ehrlicher Block an der Grenze angeschlagen wird.
const demurrageUmstellungPuffer = 3600

// nachrechnenDemurrage: nach der Umstellung keine Demurrage mehr.
func nachrechnenDemurrage(tx *Transaction, blockZeit int64) error {
	if tx.FromDemurrageLost <= 0 && tx.ToDemurrageLost <= 0 {
		return nil
	}
	if !wirtschaftAktiv(blockZeit - demurrageUmstellungPuffer) {
		return nil
	}
	return nachrechnenAbweichung("demurrage_nach_umstellung", blockZeit,
		"%s %s: Demurrage %.6f/%.6f AEQ, seit der Umlaufsicherung gibt es keine",
		tx.Type, kurzAdresse(tx.Wallet), tx.FromDemurrageLost, tx.ToDemurrageLost)
}

// nachrechnenUBILocked: ein Grundeinkommen der neuen Form (je Mensch eine
// Transaktion), VOR dem Anwenden.
func (cs *ChainState) nachrechnenUBILocked(tx *Transaction, blockZeit int64) error {
	wallet := strings.ToLower(strings.TrimSpace(tx.Wallet))
	if tx.AmountPerHuman > 0 || wallet == "" {
		return nil // alte Form: nachrechnenTxLocked; ohne Wallet: Nachspielen lehnt ab
	}
	r := &cs.ubiRunde
	betrag := NewDecimal(tx.Amount).Micro()
	if !r.aktiv {
		menschen, err := cs.countHumanAccountsLocked(context.Background())
		if err != nil {
			// Ohne eigene Zahl keine Pruefung von Zahl und Anteil -- aber
			// auch kein Glaube: die Runde gilt als unsicher und wird
			// gemeldet, nicht stillschweigend als geprueft gezaehlt.
			r.unsicher = true
			if aerr := nachrechnenAbweichung("ubi_menschen_unlesbar", blockZeit, "%v", err); aerr != nil {
				return aerr
			}
		}
		*r = ubiRundePruefung{
			unsicher:   r.unsicher,
			aktiv:      true,
			topfStart:  cs.topfMikroLocked(ubiPoolAddr),
			menschen:   menschen,
			anteil:     betrag,
			empfaenger: make(map[string]int64),
		}
	}
	r.n++
	r.demurrage = plusGesaettigt(r.demurrage, NewDecimal(tx.FromDemurrageLost).Micro())

	cs.ensureAccountLoadedCtx(context.Background(), wallet)
	if acc, ok := cs.accounts.Get(wallet); !ok || !acc.IsHuman {
		// Nicht in die Empfaengermenge: sie darf nur Menschen enthalten,
		// damit sie durch den Zustand begrenzt ist und nicht durch das,
		// was ein Block hineinschreibt.
		return nachrechnenAbweichung("ubi_kein_mensch", blockZeit,
			"%s ist kein registrierter Mensch (%.6f AEQ)", kurzAdresse(wallet), tx.Amount)
	}
	if _, schon := r.empfaenger[wallet]; schon {
		if err := nachrechnenAbweichung("ubi_doppelt", blockZeit,
			"%s bekommt in dieser Runde zum zweiten Mal", kurzAdresse(wallet)); err != nil {
			return err
		}
	} else {
		r.empfaenger[wallet] = r.n
	}
	if d := betrag - r.anteil; d > 1 || d < -1 {
		return nachrechnenAbweichung("ubi_ungleich", blockZeit,
			"%s: %.6f AEQ, die Runde zahlt %.6f", kurzAdresse(wallet), tx.Amount,
			NewDecimalFromMicro(r.anteil).Float())
	}
	return nil
}

// nachrechnenUBIAbschlussLocked: beim Abschluss (finalize) oder spaetestens
// bei der Rundenmarke -- jetzt ist die Runde vollstaendig.
func (cs *ChainState) nachrechnenUBIAbschlussLocked(blockZeit int64) error {
	r := cs.ubiRunde
	cs.ubiRunde = ubiRundePruefung{} // Rundengrenze: der naechste Anfang ist beobachtet
	if !r.aktiv || r.unsicher {
		return nil
	}
	if r.n != r.menschen {
		if err := nachrechnenAbweichung("ubi_empfaenger", blockZeit,
			"%d Empfaenger, registriert sind %d Menschen", r.n, r.menschen); err != nil {
			return err
		}
	}
	if r.menschen <= 0 {
		return nil
	}
	// Dieselbe Rechnung wie der Erzeuger: floor6 auf den float-Quotienten.
	gesamt := NewDecimalFromMicro(plusGesaettigt(r.topfStart, r.demurrage)).Float()
	erwartet := NewDecimal(floor6(gesamt / float64(r.menschen))).Micro()
	if d := r.anteil - erwartet; d > 1 || d < -1 {
		return nachrechnenAbweichung("ubi_anteil", blockZeit,
			"je Mensch %.6f AEQ, nachgerechnet %.6f (Topf %.6f, %d Menschen)",
			NewDecimalFromMicro(r.anteil).Float(), NewDecimalFromMicro(erwartet).Float(), gesamt, r.menschen)
	}
	return nil
}
