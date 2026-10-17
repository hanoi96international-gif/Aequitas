package keeper

// FROMSYNC-STICHTAG (Beta-Bericht, Abschnitt 2: "FromSync-Ausnahmen (Sperre,
// Finalitaet) ueber Seed-Verbindungen ohne TLS", MITTEL) -- SCHLAFEND, bis
// ein eigener PR den Stichtag setzt.
//
// # WORUM ES GEHT
//
// Block.FromSync ist wahr fuer Bloecke, die per HTTP-Sync von einem vom
// Betreiber eingetragenen Seed (PRIMARY_NODE_URL/PRIMARY_NODE_URLS/PEER_NODES,
// isTrustedSyncSource) geholt wurden. Fuer sie fielen bisher Pruefungen weg,
// die jeder andere Block bestehen muss. Ein Seed darf "http://IP:8080" sein
// (isAllowedPeerURL) -- wer sich in diese Verbindung haengt, liefert Bloecke,
// die als FromSync gelten.
//
// # DIE STELLEN (Stand dieses PRs)
//
//  1. Sperre wegen Doppelsignatur -- block.go, AddPeerBlock,
//     IsValidatorSuspended. Wird ab dem Stichtag auch fuer FromSync und
//     SelfFetched geprueft (dag.sperrpruefungEntfaellt) -- AUSSER eine Waise
//     wartet auf genau diesen Block (hasAwaitingOrphan, dieselbe enge Regel
//     wie bei der Finalitaet; Befund F1 der Sicherheitspruefung von #373).
//     Grund: lastOffenseAt ist die Blockzeit des Vergehens. Die Folgebloecke
//     von V zwischen Tat und Erkennung stehen in der kanonischen Geschichte,
//     ehrliche Nachfahren bauen auf ihnen auf; sie abzuweisen, liesse jeden
//     Nachholenden bis zu 14 Tage (suspendedUntil > now) an diesen Waisen
//     haengen. Die Waise ist der Beleg, dass ein ANDERER, zugelassener und
//     ungesperrter Erzeuger auf dem Block aufgebaut hat: eine Waise entsteht
//     erst hinter Signatur-, Erzeuger-, Finalitaets- und Sperrpruefung (Waise
//     von V selbst wird schon an der Sperre abgewiesen). Alte Bloecke bleiben
//     ohnehin durch: IsValidatorSuspended laesst Bloecke vor dem Vergehen
//     durch (slashing.go, blockTimestamp < lastOffenseAt).
//  2. Finalitaet -- finality.go, isFinalityViolation. FromSync entbindet nicht
//     mehr, wenn der Block ab dem Stichtag datiert ist ODER der Knoten ihn ab
//     dem Stichtag empfaengt (fromSyncAusnahmeFinalitaet, Befund F2: ein Ast
//     unter der Finalitaet hat zwangslaeufig alte Eltern und darf seine
//     Blockzeit alt waehlen). Die Finalitaet ist knotenlokal (eigener
//     Checkpoint, eigene Annahme), keine Regel des Nachspielens -- die
//     Empfangszeit entscheidet dort also nichts, was ein anderer Knoten
//     gleich entscheiden muesste. (Die enge SelfFetched-Ausnahme dort bleibt:
//     sie greift nur, solange eine Waise auf genau diesen Block wartet, und
//     gilt fuer jeden Peer gleich, nicht nur fuer Seeds.)
//  3. Strafe bei Doppelsignatur -- block.go, checkAndIndexEquivocation: ein
//     FromSync-Block loeste keine Strafe aus. Ab dem Stichtag doch.
//  4. Spaet eingehaengte Bindung/Strafe -- spaet_eingehaengt.go. Ab dem
//     Stichtag auch fuer FromSync.
//  5. Erzeugerpruefung -- block.go, erzeugerErlaubt: hat schon ihren eigenen,
//     abgelaufenen Stichtag (syncGeschichte, zeitstempelPruefungAbUnix,
//     30.09.2026). Unveraendert. Ab dem Empfangs-Stichtag (Stelle 2) faellt
//     ein solcher Block aber an der Finalitaet, sobald er unter ihr liegt.
//     RESTRISIKO (Befund F2, nicht im Code loesbar): an der Sync-Front eines
//     Knotens, der die Geschichte vor dem 30.09. noch nachspielt, kann ein
//     Block mit gefaelschtem Erzeuger und alter Blockzeit weiter durch. Eine
//     Bindung an einen signierten Checkpoint geht nicht: der Checkpoint in
//     der Kette schlaeft (checkpointTxAb), und ein vorwaerts nachspielender
//     Knoten kennt den Checkpoint-Block erst NACH den alten Bloecken -- ob
//     ein alter Block dessen Vorfahr ist, laesst sich beim Empfang nicht
//     pruefen. Siehe docs/FROMSYNC_STICHTAG.md, "Restrisiken".
//  6. Proposer-Sperre (Circuit Breaker) -- block.go, proposerBlockBlocked und
//     recordProposerOutcome: BLEIBT ausgenommen. Sie ist keine
//     Gueltigkeitspruefung, sondern eine Lastbremse vor allen Pruefungen; ein
//     Block, der an ihr vorbeikommt, durchlaeuft danach jede Pruefung. Sie
//     auch fuer Seeds scharf zu stellen, braechte die dort beschriebene
//     Verklemmung zwischen Seed und Nachholendem zurueck, ohne einen
//     ungueltigen Block mehr abzuweisen. SelfFetched ebenso.
//
// Signaturen (Block und Ueberweisungen), Noncen, Deckung, Hash und Hoehe hat
// FromSync nie entbunden.
//
// # REGEL
//
// Massgeblich ist die BLOCKZEIT, wie bei jedem Stichtag dieser Kette: ein
// Block mit Ab <= Timestamp < Bis wird beim Sync genau so geprueft wie beim
// Push. Aeltere Bloecke spielen unveraendert nach. Einzige Ausnahme ist die
// knotenlokale Finalitaet (Stelle 2), die zusaetzlich an der Empfangszeit
// haengt. Zurueckdatieren hilft nicht ueber Tage: zeitstempelRueckdatiert
// begrenzt den Abstand zum juengsten Elternblock (zeitstempel_pruefung.go);
// die Rueck-Toleranz ist deshalb wie bei syncGeschichte eingerechnet.
//
// RUECKNAHME (Befund F3): nie Ab zurueck auf MaxInt64 -- das aenderte die
// Regel fuer schon geschriebene Bloecke rueckwirkend, ein frisch
// synchronisierender Knoten naehme an, was laufende abgewiesen haben. Statt
// dessen fromSyncStrengBisUnix setzen: Bloecke mit Blockzeit in [Ab, Bis)
// bleiben fuer immer streng geprueft, ab Bis gilt wieder die alte Ausnahme.
// Die Ruecknahme ist selbst ein Konsenswechsel (48 h Vorlauf, alle
// Validatoren); Strafen aus der Zwischenzeit bleiben bestehen.
//
// SCHLAFEND: fromSyncStrengAbUnix = math.MaxInt64. Dann ist alles byte-gleich
// zu vorher (TestFromSyncStichtag_SchlafendByteGleich). Ablauf fuer den
// Betreiber: docs/FROMSYNC_STICHTAG.md.

import (
	"math"
	"sync/atomic"
)

// fromSyncStrengAbUnix: Blockzeit, ab der FromSync keine Pruefung mehr
// entbindet. math.MaxInt64 = SCHLAFEND.
//
// Gesetzt am 17.10.2026 (zunaechst 20.10.), auf Anweisung des Betreibers
// vorgezogen auf 1792281600 = 18.10.2026 00:00:00 UTC (rund 2,5 h
// Vorlauf, volle Stunde UTC). Aktiver Validator ist nur C1, der beim Merge
// ausgerollt wird -- also vor Ab. Nie zurueck auf MaxInt64 (siehe oben).
const fromSyncStrengAbUnix int64 = 1792281600

// fromSyncStrengBisUnix: Blockzeit, ab der die alte Ausnahme wieder gilt
// (Ruecknahme, siehe oben). math.MaxInt64 = keine Ruecknahme. Gilt nur, wenn
// er nach Ab liegt; ein Bis <= Ab wird nicht beachtet (fail-closed: eine
// verdrehte Konstante darf den Stichtag nicht still abschalten).
const fromSyncStrengBisUnix int64 = math.MaxInt64

// fromSyncStrengOverride, fromSyncStrengBisOverride: nur fuer Tests
// (0 = Konstante gilt).
var (
	fromSyncStrengOverride    atomic.Int64
	fromSyncStrengBisOverride atomic.Int64
)

func fromSyncStrengAb() int64 {
	if o := fromSyncStrengOverride.Load(); o != 0 {
		return o
	}
	return fromSyncStrengAbUnix
}

func fromSyncStrengBis() int64 {
	if o := fromSyncStrengBisOverride.Load(); o != 0 {
		return o
	}
	return fromSyncStrengBisUnix
}

// fromSyncStreng: gilt fuer einen Block mit dieser Zeit jede Pruefung auch
// beim Sync? Mit Toleranzen zugunsten der Pruefung: ein um bis zu zwei
// Minuten zurueckdatierter Block entgeht ihr am Anfang nicht (wie
// erzeugerSchnittAktiv), ein um bis zu zwei Minuten vorausdatierter am Ende
// nicht (zeitstempelZukunftToleranz). Schlafend immer falsch, auch fuer
// MaxInt64 selbst.
func fromSyncStreng(blockZeit int64) bool {
	ab := fromSyncStrengAb()
	if ab == math.MaxInt64 {
		return false
	}
	if sattAdd(blockZeit, zeitstempelRueckToleranz) < ab {
		return false
	}
	if bis := fromSyncStrengBis(); bis != math.MaxInt64 && bis > ab &&
		blockZeit >= sattAdd(bis, zeitstempelZukunftToleranz) {
		return false
	}
	return true
}

// fromSyncStrengJetzt: liegt die Empfangszeit (eigene Uhr) im Fenster
// [Ab, Bis)? Nur fuer die knotenlokale Finalitaet. Schlafend immer falsch.
func fromSyncStrengJetzt() bool {
	ab := fromSyncStrengAb()
	if ab == math.MaxInt64 {
		return false
	}
	jetzt := nowUnix()
	if jetzt < ab {
		return false
	}
	if bis := fromSyncStrengBis(); bis != math.MaxInt64 && bis > ab && jetzt >= bis {
		return false
	}
	return true
}

// fromSyncAusnahme: darf dieser Block die FromSync-Ausnahmen (Stellen 3-4
// oben) noch nutzen? Schlafend genau block.FromSync.
func fromSyncAusnahme(b *Block) bool {
	return b.FromSync && !fromSyncStreng(b.Timestamp)
}

// fromSyncAusnahmeFinalitaet: Stelle 2 -- wie fromSyncAusnahme, zusaetzlich
// nicht, solange die Empfangszeit im Fenster liegt (Befund F2). Schlafend
// genau block.FromSync.
func fromSyncAusnahmeFinalitaet(b *Block) bool {
	return fromSyncAusnahme(b) && !fromSyncStrengJetzt()
}

// selbstGeholtOhneSperre: entbindet FromSync/SelfFetched diesen Block allein
// nach der Blockzeit von der Sperrpruefung (Stelle 1)? Schlafend genau
// block.FromSync || block.SelfFetched.
func selbstGeholtOhneSperre(b *Block) bool {
	return (b.FromSync || b.SelfFetched) && !fromSyncStreng(b.Timestamp)
}

// sperrpruefungEntfaellt: Stelle 1 vollstaendig. Ab dem Stichtag entbindet
// FromSync/SelfFetched nur noch, wenn eine Waise auf genau diesen Block
// wartet (Befund F1, siehe oben). Schlafend genau selbstGeholtOhneSperre --
// der zweite Teil wird dann nie ausgewertet. Unter dag.mu.
func (dag *BlockDAG) sperrpruefungEntfaellt(b *Block) bool {
	if selbstGeholtOhneSperre(b) {
		return true
	}
	return (b.FromSync || b.SelfFetched) && dag.hasAwaitingOrphan(b.Hash)
}
