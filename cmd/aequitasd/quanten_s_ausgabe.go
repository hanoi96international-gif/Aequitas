package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/keeper"
)

// quantenSAusgabeBefehl: `aequitasd quanten-s-ausgabe [--api URL]
// [--deckung MIKRO] [--frist DAUER] [--toleranz PROZENT] [--messen-warten DAUER]
// [--stichprobe-v2] [--fuer-inhaberin] < messung.json`. Der Schluessel ist der
// des Knotens (RELAYER_PRIVATE_KEY aus der Umgebung des Containers); er wird
// nie ausgegeben. Laeuft NICHT als root: das Werkzeug braucht keine Rechte,
// und als root liefe ein Fehler mit allen Rechten im Container.
func quantenSAusgabeBefehl(args []string) int {
	return quantenSAusgabeMit(args, os.Getuid(), os.Getenv("RELAYER_PRIVATE_KEY"), os.Stdin, os.Stdout, os.Stderr)
}

func quantenSAusgabeMit(args []string, uid int, schluessel string, ein io.Reader, aus, log io.Writer) int {
	fs := flag.NewFlagSet("quanten-s-ausgabe", flag.ContinueOnError)
	fs.SetOutput(log)
	api := fs.String("api", "http://127.0.0.1:8080", "API des eigenen Knotens")
	deckung := fs.Int64("deckung", 1_000_000, "Deckung des Tokens in Mikro-AEQ")
	frist := fs.Duration("frist", 15*time.Minute, "Frist je Schritt")
	validatoren := fs.String("validatoren", "", "mehrere Validatoren: adresse=api,adresse=api (leer = nur dieser Knoten)")
	schwelle := fs.Int("t", 1, "Schwelle: so viele Validatoren muessen ihre Anteile offenlegen")
	toleranz := fs.Int("toleranz", keeper.QuantenSToleranzStandard, "Toleranz in Prozent; darunter nur mit kleinem Messfehler (hardware_ibm.py --wiederholen)")
	stichprobeV2 := fs.Bool("stichprobe-v2", false, "Abschnitts-Modus: Stichprobe erst nach der Bindung (nur, wenn die Kette sie kennt)")
	messenWarten := fs.Duration("messen-warten", 0, "Abschnitts-Modus: Basen bei den Validatoren vormerken und so lange warten, bis jeder gemessen hat (0 = Messungen liegen schon vor)")
	nachweis := fs.Duration("nachweis-warten", 0, "Stichprobe v2: nach dem Einloesen so lange auf den Nachweis aller Validatoren warten und ihn pruefen (0 = nicht)")
	fuerInhaberin := fs.Bool("fuer-inhaberin", false, "getrennte Rollen: Eingabe ist die Bestellung einer Inhaberin, Ausgabe das Paket fuer sie (immer Stichprobe v2)")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	if uid == 0 {
		fmt.Fprintln(log, "ABBRUCH: nicht als root starten (docker exec -u 10001:10001)")
		return 2
	}
	if schluessel == "" {
		fmt.Fprintln(log, "ABBRUCH: RELAYER_PRIVATE_KEY fehlt")
		return 2
	}
	vs, err := keeper.QuantenSValidatorenLesen(*validatoren)
	if err != nil {
		fmt.Fprintln(log, "ABBRUCH:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *fuerInhaberin {
		if err := keeper.QuantenSAusgabeFuerInhaberin(ctx, *api, schluessel, *deckung, *frist, vs, *schwelle, *toleranz, *messenWarten, ein, aus, log); err != nil {
			fmt.Fprintln(log, "FEHLER:", err)
			return 1
		}
		return 0
	}
	if err := keeper.QuantenSAusgabe(ctx, *api, schluessel, *deckung, *frist, vs, *schwelle, *toleranz, *messenWarten, *stichprobeV2, *nachweis, ein, aus, log); err != nil {
		fmt.Fprintln(log, "FEHLER:", err)
		if errors.Is(err, keeper.ErrQuantenSNachweis) {
			return 3 // eingeloest; nur der Nachweis fehlt oder passt nicht
		}
		return 1
	}
	return 0
}
