package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/hanoi96international-gif/aequitas-chain/x/humanity/keeper"
)

// Quantengeld S mit getrennten Rollen (keeper/quanten_s_rollen.go,
// DEZENTRAL.md 17):
//
//	aequitasd quanten-s-inhaberin neu --datei F --n N --v V --stellen adr=schl,adr=schl
//	aequitasd quanten-s-inhaberin annehmen --datei F --api URL --validatoren 0xadr,... < paket.json
//	aequitasd quanten-s-inhaberin einloesen --datei F --api URL --stelle 0|1 > vorlage.json
//	aequitasd quanten-s-annahme --api URL --validatoren adr=api,... < vorlage.json
//
// Die Datei der Inhaberin haelt ihren Schluessel (0600, nie ausgegeben). Die
// Annahmestelle liest ihren Schluessel aus einer Umgebungsvariablen (nie von
// der Befehlszeile, die jeder Prozess sehen kann).

func quantenSInhaberinBefehl(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return quantenSInhaberinMit(ctx, args, os.Stdin, os.Stdout, os.Stderr)
}

func quantenSInhaberinMit(ctx context.Context, args []string, ein io.Reader, aus, log io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(log, "ABBRUCH: quanten-s-inhaberin neu | annehmen | einloesen")
		return 2
	}
	was := args[0]
	fs := flag.NewFlagSet("quanten-s-inhaberin "+was, flag.ContinueOnError)
	fs.SetOutput(log)
	datei := fs.String("datei", "", "Datei der Inhaberin (0600; enthaelt ihren Schluessel)")
	api := fs.String("api", "", "API eines Knotens (https://... oder http://127.0.0.1:port)")
	var n, v, stelle *int
	var stellen, validatoren *string
	var frist *time.Duration
	switch was {
	case "neu":
		n = fs.Int("n", 0, "Stellen des Tokens")
		v = fs.Int("v", 1, "Zahl der Validatoren (Abschnitte)")
		stellen = fs.String("stellen", "", "die zwei Annahmestellen: adresse=schluessel,adresse=schluessel")
	case "annehmen":
		validatoren = fs.String("validatoren", "", "Adressen der Validatoren, denen die Inhaberin vertraut, in der Reihenfolge der Abschnitte: 0xadr,0xadr")
	case "einloesen":
		stelle = fs.Int("stelle", -1, "Annahmestelle 0 oder 1")
		frist = fs.Duration("frist", 15*time.Minute, "Frist je Schritt")
	default:
		fmt.Fprintln(log, "ABBRUCH: quanten-s-inhaberin neu | annehmen | einloesen")
		return 2
	}
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return 2
	}
	if *datei == "" {
		fmt.Fprintln(log, "ABBRUCH: --datei fehlt")
		return 2
	}
	var err error
	switch was {
	case "neu":
		var st [2]keeper.QuantenSStelle
		if st, err = keeper.QuantenSStellenLesen(*stellen); err == nil {
			err = keeper.QuantenSInhaberinNeu(*datei, *n, *v, st, aus)
		}
	case "annehmen":
		var vs []string
		if vs, err = keeper.QuantenSAdressenLesen(*validatoren); err == nil {
			err = keeper.QuantenSInhaberinAnnehmen(ctx, *api, *datei, vs, ein, log)
		}
	case "einloesen":
		err = keeper.QuantenSInhaberinEinloesen(ctx, *api, *datei, *stelle, *frist, aus, log)
	}
	if err != nil {
		fmt.Fprintln(log, "FEHLER:", err)
		return 1
	}
	return 0
}

func quantenSAnnahmeBefehl(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return quantenSAnnahmeMit(ctx, args, os.Getuid(), os.Getenv, os.Stdin, os.Stdout, os.Stderr)
}

func quantenSAnnahmeMit(ctx context.Context, args []string, uid int, umgebung func(string) string, ein io.Reader, aus, log io.Writer) int {
	fs := flag.NewFlagSet("quanten-s-annahme", flag.ContinueOnError)
	fs.SetOutput(log)
	api := fs.String("api", "http://127.0.0.1:8080", "API des eigenen Knotens")
	validatoren := fs.String("validatoren", "", "Validatoren des Tokens: adresse=api,adresse=api (in der Reihenfolge der Abschnitte)")
	variable := fs.String("schluessel-variable", "RELAYER_PRIVATE_KEY", "Umgebungsvariable mit dem Schluessel der Annahmestelle")
	frist := fs.Duration("frist", 15*time.Minute, "Frist je Schritt")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	if uid == 0 {
		fmt.Fprintln(log, "ABBRUCH: nicht als root starten")
		return 2
	}
	name := strings.TrimSpace(*variable)
	schluessel := ""
	if name != "" {
		schluessel = umgebung(name)
	}
	if schluessel == "" {
		fmt.Fprintf(log, "ABBRUCH: Schluessel der Annahmestelle fehlt (Umgebungsvariable %q)\n", name)
		return 2
	}
	vs, err := keeper.QuantenSValidatorenLesen(*validatoren)
	if err != nil {
		fmt.Fprintln(log, "ABBRUCH:", err)
		return 2
	}
	if err := keeper.QuantenSAnnahme(ctx, *api, schluessel, *frist, vs, ein, aus, log); err != nil {
		fmt.Fprintln(log, "FEHLER:", err)
		return 1
	}
	return 0
}
