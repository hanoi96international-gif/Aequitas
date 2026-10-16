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
//     SelfFetched geprueft (selbstGeholtOhneSperre). Alte Bloecke bleiben
//     durch: IsValidatorSuspended laesst Bloecke vor dem Vergehen ohnehin
//     durch (slashing.go, blockTimestamp < lastOffenseAt).
//  2. Finalitaet -- finality.go, isFinalityViolation. FromSync entbindet ab
//     dem Stichtag nicht mehr. (Die enge SelfFetched-Ausnahme dort bleibt:
//     sie greift nur, solange eine Waise auf genau diesen Block wartet, und
//     gilt fuer jeden Peer gleich, nicht nur fuer Seeds.)
//  3. Strafe bei Doppelsignatur -- block.go, checkAndIndexEquivocation: ein
//     FromSync-Block loeste keine Strafe aus. Ab dem Stichtag doch.
//  4. Spaet eingehaengte Bindung/Strafe -- spaet_eingehaengt.go. Ab dem
//     Stichtag auch fuer FromSync.
//  5. Erzeugerpruefung -- block.go, erzeugerErlaubt: hat schon ihren eigenen,
//     abgelaufenen Stichtag (syncGeschichte, zeitstempelPruefungAbUnix,
//     30.09.2026). Unveraendert.
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
// Block mit Timestamp >= fromSyncStrengAb() wird beim Sync genau so geprueft
// wie beim Push. Aeltere Bloecke spielen unveraendert nach. Zurueckdatieren
// hilft nicht ueber Tage: zeitstempelRueckdatiert begrenzt den Abstand zum
// juengsten Elternblock (zeitstempel_pruefung.go); die Rueck-Toleranz ist
// deshalb wie bei syncGeschichte eingerechnet.
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
const fromSyncStrengAbUnix int64 = math.MaxInt64

// fromSyncStrengOverride: nur fuer Tests (0 = Konstante gilt).
var fromSyncStrengOverride atomic.Int64

func fromSyncStrengAb() int64 {
	if o := fromSyncStrengOverride.Load(); o != 0 {
		return o
	}
	return fromSyncStrengAbUnix
}

// fromSyncStreng: gilt fuer einen Block mit dieser Zeit jede Pruefung auch
// beim Sync? Mit der Rueck-Toleranz: ein um bis zu zwei Minuten
// zurueckdatierter Block entgeht ihr dadurch nicht (wie erzeugerSchnittAktiv).
// Schlafend immer falsch, auch fuer MaxInt64 selbst.
func fromSyncStreng(blockZeit int64) bool {
	ab := fromSyncStrengAb()
	if ab == math.MaxInt64 {
		return false
	}
	return sattAdd(blockZeit, zeitstempelRueckToleranz) >= ab
}

// fromSyncAusnahme: darf dieser Block die FromSync-Ausnahmen (Stellen 2-4
// oben) noch nutzen? Schlafend genau block.FromSync.
func fromSyncAusnahme(b *Block) bool {
	return b.FromSync && !fromSyncStreng(b.Timestamp)
}

// selbstGeholtOhneSperre: entbindet FromSync/SelfFetched diesen Block von der
// Sperrpruefung (Stelle 1)? Schlafend genau block.FromSync || block.SelfFetched.
func selbstGeholtOhneSperre(b *Block) bool {
	return (b.FromSync || b.SelfFetched) && !fromSyncStreng(b.Timestamp)
}
