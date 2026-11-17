package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/keeper"
)

// quantenSAusgabeBefehl: `aequitasd quanten-s-ausgabe [--api URL]
// [--deckung MIKRO] [--frist DAUER] < messung.json`. Der Schluessel ist der
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := keeper.QuantenSAusgabe(ctx, *api, schluessel, *deckung, *frist, ein, aus, log); err != nil {
		fmt.Fprintln(log, "FEHLER:", err)
		return 1
	}
	return 0
}
