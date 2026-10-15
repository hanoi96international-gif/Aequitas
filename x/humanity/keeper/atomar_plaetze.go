package keeper

// Plaetze fuer atomare Auftraege (Beta-Pruefung 15.10.2026, Tempo, HOCH).
//
// runAtomicWithOutbox und runAtomicDistributionWithOutbox holen ERST eine
// Verbindung (cs.db.Begin) und warten DANN auf cs.mu. Das Nachspielen und der
// WAL-Flush halten cs.mu und holen danach eine Verbindung. Liefen so viele
// Auftraege gleichzeitig, wie der Pool Verbindungen hat (Vorgabe 20; ueber
// eth_sendRawTransaction, Tausch, Registrierung oeffentlich erreichbar),
// hielten sie alle Verbindungen und warteten auf cs.mu -- wer cs.mu hielt,
// bekam keine Verbindung mehr. Der Knoten stand.
//
// Die Sperre selbst laesst ohnehin nur einen Auftrag zugleich arbeiten.
// Darum warten Auftraege VOR Begin auf einen von atomarPlatzZahl Plaetzen,
// und dem Pool bleiben atomarReserve Verbindungen fuer die, die cs.mu halten.

import (
	"fmt"
	"sync"
	"time"
)

const atomarReserve = 4

var (
	atomarPlaetzeEinmal sync.Once
	atomarPlaetze       chan struct{}
)

// atomarPlatzZahl: Plaetze bei einem Pool mit maxOpen Verbindungen
// (0 = unbegrenzt: keine Grenze noetig).
func atomarPlatzZahl(maxOpen int) int {
	if maxOpen <= 0 {
		return 0
	}
	if n := maxOpen - atomarReserve; n >= 1 {
		return n
	}
	return 1
}

// atomarWarteFrist: so lange wartet ein Auftrag auf einen Platz, dann
// ErrAnnahmePausiert (503) statt eines Staus ohne Grenze (Nachpruefung der
// Eingaenge, NIEDRIG). Der Aufrufer versucht es spaeter erneut.
var atomarWarteFrist = 10 * time.Second

// atomarPlatzMitFrist: wie atomarPlatz, aber hoechstens atomarWarteFrist.
func (cs *ChainState) atomarPlatzMitFrist() (func(), error) {
	if cs.db == nil {
		return func() {}, nil
	}
	cs.atomarPlaetzeAnlegen()
	if atomarPlaetze == nil {
		return func() {}, nil
	}
	t := time.NewTimer(atomarWarteFrist)
	defer t.Stop()
	select {
	case atomarPlaetze <- struct{}{}:
		return func() { <-atomarPlaetze }, nil
	case <-t.C:
		return nil, fmt.Errorf("%w: zu viele gleichzeitige Auftraege -- bitte gleich erneut versuchen", ErrAnnahmePausiert)
	}
}

func (cs *ChainState) atomarPlaetzeAnlegen() {
	atomarPlaetzeEinmal.Do(func() {
		if n := atomarPlatzZahl(cs.db.Stats().MaxOpenConnections); n > 0 {
			atomarPlaetze = make(chan struct{}, n)
		}
	})
}

// atomarPlatz: vor cs.db.Begin() eines atomaren Auftrags aufrufen; die
// zurueckgegebene Funktion nach Commit/Rollback. Ohne Frist -- fuer die
// taegliche Verteilung, die nicht ausfallen soll.
func (cs *ChainState) atomarPlatz() func() {
	if cs.db == nil {
		return func() {}
	}
	cs.atomarPlaetzeAnlegen()
	if atomarPlaetze == nil {
		return func() {}
	}
	atomarPlaetze <- struct{}{}
	return func() { <-atomarPlaetze }
}
