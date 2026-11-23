package keeper

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Nachzug (quanten_s_rollen.go): ein Validator, der den Block mit der Ausgabe
// noch nicht hat, antwortet "erst, wenn der Token auf der Kette steht"; die
// Inhaberin fragt dann begrenzt erneut. Gesehen in db-tests
// (TestVierKnotenQuantenSRollen_RealDB, Validator 3: HTTP 409).

type qsNachzugNetz struct {
	quantenSAbschnittNetz
	fehlerBis int
	fehler    error
	aufrufe   int
}

func (n *qsNachzugNetz) fuerInhaberin(_ context.Context, x int, K string) (QuantenSPaketAbschnitt, error) {
	n.aufrufe++
	if n.aufrufe <= n.fehlerBis {
		return QuantenSPaketAbschnitt{}, n.fehler
	}
	return QuantenSPaketAbschnitt{Von: x}, nil
}

func TestQuantenSFuerInhaberinNachzug(t *testing.T) {
	alt := quantenSNachzugPause
	quantenSNachzugPause = time.Millisecond
	defer func() { quantenSNachzugPause = alt }()
	nochNicht := errors.New("HTTP 409: rejected: quanten_w: " + quantenSNachzugGrund)

	// Gutfall: der Knoten holt den Block nach.
	n := &qsNachzugNetz{fehlerBis: 3, fehler: nochNicht}
	ab, err := quantenSFuerInhaberinNachzug(context.Background(), n, 2, "k")
	if err != nil || ab.Von != 2 || n.aufrufe != 4 {
		t.Fatalf("Nachzug: %v, %d Aufrufe", err, n.aufrufe)
	}
	// Grenze: nie mehr als quantenSNachzugVersuche Anfragen.
	n = &qsNachzugNetz{fehlerBis: 1 << 30, fehler: nochNicht}
	if _, err := quantenSFuerInhaberinNachzug(context.Background(), n, 2, "k"); err == nil || n.aufrufe != quantenSNachzugVersuche {
		t.Fatalf("Grenze: %v, %d Aufrufe", err, n.aufrufe)
	}
	// Jeder andere Fehler sofort (kein Warten auf einen falschen Abschnitt).
	n = &qsNachzugNetz{fehlerBis: 1 << 30, fehler: errors.New("HTTP 409: rejected: quanten_w: kein Abschnitt zu diesem K")}
	if _, err := quantenSFuerInhaberinNachzug(context.Background(), n, 2, "k"); err == nil || n.aufrufe != 1 {
		t.Fatalf("anderer Fehler: %v, %d Aufrufe", err, n.aufrufe)
	}
	// Abbruch durch den Aufrufer beendet das Warten.
	quantenSNachzugPause = time.Hour
	ctx, stop := context.WithCancel(context.Background())
	stop()
	n = &qsNachzugNetz{fehlerBis: 1 << 30, fehler: nochNicht}
	if _, err := quantenSFuerInhaberinNachzug(ctx, n, 2, "k"); !errors.Is(err, context.Canceled) || n.aufrufe != 1 {
		t.Fatalf("Abbruch: %v, %d Aufrufe", err, n.aufrufe)
	}
}
