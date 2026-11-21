package main

import (
	"context"
	"errors"
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
//	aequitasd quanten-s-inhaberin neu --datei F --n N --v V --stellen adr=schl,adr=schl [--verschluesseln adr=api,...]
//	aequitasd quanten-s-inhaberin annehmen --datei F --api URL --validatoren 0xadr,... < paket.json
//	aequitasd quanten-s-inhaberin abholen --datei F --api URL --k K --validatoren adr=api,...
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
		fmt.Fprintln(log, "ABBRUCH: quanten-s-inhaberin neu | annehmen | abholen | einloesen")
		return 2
	}
	was := args[0]
	fs := flag.NewFlagSet("quanten-s-inhaberin "+was, flag.ContinueOnError)
	fs.SetOutput(log)
	datei := fs.String("datei", "", "Datei der Inhaberin (0600; enthaelt ihren Schluessel)")
	api := fs.String("api", "", "API eines Knotens (https://... oder http://127.0.0.1:port)")
	var n, v, stelle *int
	var stellen, validatoren, k *string
	var frist *time.Duration
	switch was {
	case "neu":
		n = fs.Int("n", 0, "Stellen des Tokens")
		v = fs.Int("v", 1, "Zahl der Validatoren (Abschnitte)")
		stellen = fs.String("stellen", "", "die zwei Annahmestellen: adresse=schluessel,adresse=schluessel")
		validatoren = fs.String("verschluesseln", "", "Basen je Abschnitt an diese Validatoren verschluesseln (der Ausgeber sieht sie dann nicht): adresse=api,adresse=api")
	case "annehmen":
		validatoren = fs.String("validatoren", "", "Adressen der Validatoren, denen die Inhaberin vertraut, in der Reihenfolge der Abschnitte: 0xadr,0xadr")
	case "abholen":
		k = fs.String("k", "", "Kennung K des Tokens (64 Hex-Zeichen)")
		validatoren = fs.String("validatoren", "", "Validatoren des Tokens mit ihrer API, in der Reihenfolge der Abschnitte: adresse=api,adresse=api")
	case "einloesen":
		stelle = fs.Int("stelle", -1, "Annahmestelle 0 oder 1")
		frist = fs.Duration("frist", 15*time.Minute, "Frist je Schritt")
	default:
		fmt.Fprintln(log, "ABBRUCH: quanten-s-inhaberin neu | annehmen | abholen | einloesen")
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
		var vs []keeper.QuantenSValidator
		if st, err = keeper.QuantenSStellenLesen(*stellen); err == nil {
			if vs, err = keeper.QuantenSValidatorenLesen(*validatoren); err == nil {
				err = keeper.QuantenSInhaberinNeu(ctx, *datei, *n, *v, st, vs, aus)
			}
		}
	case "annehmen":
		var vs []string
		if vs, err = keeper.QuantenSAdressenLesen(*validatoren); err == nil {
			err = keeper.QuantenSInhaberinAnnehmen(ctx, *api, *datei, vs, ein, log)
		}
	case "abholen":
		var vs []keeper.QuantenSValidator
		if vs, err = keeper.QuantenSValidatorenLesen(*validatoren); err == nil {
			err = keeper.QuantenSInhaberinAbholen(ctx, *api, *datei, *k, vs, log)
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
	nachweis := fs.Duration("nachweis-warten", 0, "nach dem Einloesen so lange auf den Nachweis aller Validatoren warten und ihn pruefen (0 = nicht)")
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
	if err := keeper.QuantenSAnnahme(ctx, *api, schluessel, *frist, *nachweis, vs, ein, aus, log); err != nil {
		fmt.Fprintln(log, "FEHLER:", err)
		if errors.Is(err, keeper.ErrQuantenSNachweis) {
			return 3 // eingeloest; nur der Nachweis fehlt oder passt nicht
		}
		return 1
	}
	return 0
}

// quantenSMessungOeffnenBefehl: `aequitasd quanten-s-messung-oeffnen --n N
// --name SHA256(y) --verzeichnis D` < versiegelte Messung (Hex). Legt den
// Datensatz als D/<name>.json ab (nie ueberschrieben, keinem Verweis gefolgt)
// und gibt nur den Dateinamen aus, nie den Inhalt. Der Schluessel ist der des
// Knotens (RELAYER_PRIVATE_KEY aus der Umgebung des Containers), nie auf der
// Befehlszeile. Nicht als root.
func quantenSMessungOeffnenBefehl(args []string) int {
	return quantenSMessungOeffnenMit(args, os.Getuid(), os.Getenv("RELAYER_PRIVATE_KEY"), os.Stdin, os.Stdout, os.Stderr)
}

func quantenSMessungOeffnenMit(args []string, uid int, schluessel string, ein io.Reader, aus, log io.Writer) int {
	fs := flag.NewFlagSet("quanten-s-messung-oeffnen", flag.ContinueOnError)
	fs.SetOutput(log)
	n := fs.Int("n", 0, "Stellen des Abschnitts")
	name := fs.String("name", "", "sha256 der Basen (64 Hex-Zeichen, Name im Messverzeichnis)")
	verzeichnis := fs.String("verzeichnis", "", "Messverzeichnis des Knotens (AEQUITAS_QS_MESSQUELLE, z. B. /data/quanten)")
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
	if *verzeichnis == "" {
		fmt.Fprintln(log, "ABBRUCH: --verzeichnis fehlt")
		return 2
	}
	if err := keeper.QuantenSMessungOeffnen(schluessel, ein, *n, *name, *verzeichnis, aus); err != nil {
		fmt.Fprintln(log, "FEHLER:", err)
		return 1
	}
	return 0
}

// quantenSNachweisBefehl: `aequitasd quanten-s-nachweis --k K --validatoren
// adr=api,... [--wurzel H --proben H0,H1]` -- einen beendeten Token
// nachpruefen (keeper/quanten_s_nachweis.go). Ohne Schluessel, nur lesend.
func quantenSNachweisBefehl(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return quantenSNachweisMit(ctx, args, os.Stdout, os.Stderr)
}

func quantenSNachweisMit(ctx context.Context, args []string, aus, log io.Writer) int {
	fs := flag.NewFlagSet("quanten-s-nachweis", flag.ContinueOnError)
	fs.SetOutput(log)
	k := fs.String("k", "", "Kennung K des Tokens")
	validatoren := fs.String("validatoren", "", "Validatoren des Tokens: adresse=api,adresse=api (in der Reihenfolge der Abschnitte)")
	wurzel := fs.String("wurzel", "", "Wurzel der Ausgabe (aus der Ansicht vor dem Ende oder dem Protokoll der Ausgabe)")
	proben := fs.String("proben", "", "Proben der Ausgabe: H0,H1")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return 2
	}
	vs, err := keeper.QuantenSValidatorenLesen(*validatoren)
	if err != nil || len(vs) == 0 {
		fmt.Fprintln(log, "ABBRUCH: --validatoren adresse=api,...")
		return 2
	}
	var pr []string
	if strings.TrimSpace(*proben) != "" {
		pr = strings.Split(strings.ToLower(strings.ReplaceAll(*proben, " ", "")), ",")
	}
	if err := keeper.QuantenSNachweisPruefenBefehl(ctx, *k, vs, *wurzel, pr, aus); err != nil {
		fmt.Fprintln(log, "FEHLER:", err)
		return 1
	}
	return 0
}
