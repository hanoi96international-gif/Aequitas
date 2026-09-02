package keeper

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"sort"
	"strings"
	"testing"
)

// Kein Range ueber alle Shards auf dem Ueberweisungspfad.
//
// # DER SCHADEN, DEN DIESE REGEL VERHINDERT
//
// shardedAccounts.Range sperrt und entsperrt JEDEN der numAccountShards
// Shards -- heute 16.384 -- unabhaengig davon, wie viele davon ueberhaupt
// belegt sind. Auf einem Pfad, der je Ueberweisung laeuft, ist das kein
// Schoenheitsfehler:
//
//	"confirmed live via CPU profiling to be 57% of total CPU time once
//	 numAccountShards was raised to 16384"
//	                       -- state.go, bootstrapMultiplierLocked
//
// 57 % der gesamten Rechenzeit fuer das Zaehlen von Menschen. Behoben wurde
// das damals, indem der Scan durch den gepflegten Zaehler humanCountLocked
// ersetzt wurde -- NICHT indem die Shard-Zahl wieder sank.
//
// # WARUM ALS TEST UND NICHT ALS KOMMENTAR
//
// Die Lehre steht seit dem 23.07.2026 als Prosa in state.go. Prosa haelt
// niemanden auf, der eine Zeile spaeter eine Durchschnittsberechnung, eine
// Gini-Zahl oder eine Summe "eben schnell" ueber Range holt. Der Fehler faellt
// auch nicht auf: er erzeugt kein falsches Ergebnis, nur ein langsames -- und
// zwar erst unter Last, auf einer Box, gegen die gerade niemand misst.
//
// Zusaetzlich haengt daran die Frage, ob numAccountShards je erhoeht werden
// darf. Rein rechnerisch waere das der wirksamste Hebel gegen
// Shard-Kollisionen (bei 618 gleichzeitigen Sendern 14,5 % Rueckfall bei
// 16.384 Shards gegen 0,94 % bei 262.144). Solange aber irgendein
// Range-Aufruf auf dem heissen Pfad sitzt, macht jede Erhoehung den Knoten
// linear langsamer statt schneller. Dieser Test ist die Bedingung, unter der
// diese Frage ueberhaupt gestellt werden darf.
//
// # WAS ER PRUEFT
//
// Von den Einstiegspunkten des Ueberweisungspfads aus wird der Aufrufgraph
// innerhalb des Pakets verfolgt. Erreicht einer davon accounts.Range, faellt
// der Test -- ausser die Funktion steht mit Begruendung in bekannteGrenzen.
//
// # WAS ER NICHT KANN
//
// Er liest Namen, keine Typen: zwei Methoden gleichen Namens auf
// verschiedenen Empfaengern verschmelzen. Das macht ihn eher zu streng als zu
// lasch, was hier die richtige Richtung ist. Aufrufe ueber
// Funktionsvariablen oder Schnittstellen sieht er nicht.
func TestKeinRangeAufDemUeberweisungspfad(t *testing.T) {
	// Einstiegspunkte: was je Ueberweisung laeuft.
	wurzeln := []string{
		"TransferAtomic",
		"transferConcurrent",
		"transferConcurrentWAL",
		"applyTransferDeltaLocked",
		"enforceWealthCapLockedCtx",
		"wealthCapAmountLocked",
		"bootstrapMultiplierLocked",
		"getAverageBalanceLocked",
	}

	// Bekannte, begruendete Grenzen: hier hoert die Verfolgung auf.
	bekannteGrenzen := map[string]string{
		// In Produktion (cs.useDB) ein gepflegter Zaehler, O(1). Der
		// Range-Zweig gilt nur der dateigestuetzten Notfassung, unter der
		// auch die Tests laufen. Genau diese Ersetzung war der Fix, der die
		// 57 % beseitigt hat.
		"humanCountLocked": "O(1) in cs.useDB; der Range-Zweig ist die Notfassung ohne Postgres",
		// Scannt NUR bei full == true. Der Ueberweisungspfad uebergibt
		// literal false -- das prueft TestUeberweisungNimmtNieDenVollenSnapshot
		// weiter unten nach, damit diese Ausnahme nicht zum Freibrief wird.
		"snapshotForRollbackLocked": "Range nur im full-Zweig; Ueberweisungen uebergeben false",
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return strings.HasSuffix(fi.Name(), ".go") && !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("Paket nicht lesbar: %v", err)
	}

	rufe := map[string]map[string]bool{} // Funktion -> gerufene Funktionen
	machtRange := map[string]bool{}      // Funktion ruft accounts.Range direkt
	stelle := map[string]string{}        // Funktion -> Datei:Zeile

	for _, pkg := range pkgs {
		for _, datei := range pkg.Files {
			ast.Inspect(datei, func(n ast.Node) bool {
				fd, ok := n.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					return true
				}
				name := fd.Name.Name
				if rufe[name] == nil {
					rufe[name] = map[string]bool{}
				}
				stelle[name] = fset.Position(fd.Pos()).String()
				ast.Inspect(fd.Body, func(m ast.Node) bool {
					ce, ok := m.(*ast.CallExpr)
					if !ok {
						return true
					}
					switch f := ce.Fun.(type) {
					case *ast.Ident:
						rufe[name][f.Name] = true
					case *ast.SelectorExpr:
						rufe[name][f.Sel.Name] = true
						// accounts.Range(...) -- der teure Aufruf.
						if f.Sel.Name == "Range" {
							if empf, ok := f.X.(*ast.SelectorExpr); ok && empf.Sel.Name == "accounts" {
								machtRange[name] = true
							}
						}
					}
					return true
				})
				return true
			})
		}
	}

	if len(machtRange) == 0 {
		t.Fatal("kein einziger accounts.Range-Aufruf gefunden -- der Test prueft dann nichts. " +
			"Wurde Range umbenannt oder das Feld cs.accounts anders genannt?")
	}

	type fund struct{ wurzel, ziel, pfad string }
	var funde []fund

	for _, wurzel := range wurzeln {
		if _, da := rufe[wurzel]; !da {
			t.Fatalf("Einstiegspunkt %q gibt es nicht mehr -- dieser Test prueft dann einen "+
				"Pfad, den es nicht gibt. Liste anpassen, nicht den Eintrag streichen", wurzel)
		}
		gesehen := map[string]bool{}
		var lauf func(fn string, pfad []string)
		lauf = func(fn string, pfad []string) {
			if gesehen[fn] {
				return
			}
			gesehen[fn] = true
			if _, grenze := bekannteGrenzen[fn]; grenze && fn != wurzel {
				return
			}
			if machtRange[fn] {
				funde = append(funde, fund{wurzel, fn, strings.Join(append(pfad, fn), " -> ")})
				return
			}
			ziele := make([]string, 0, len(rufe[fn]))
			for z := range rufe[fn] {
				ziele = append(ziele, z)
			}
			sort.Strings(ziele)
			for _, z := range ziele {
				if _, da := rufe[z]; da {
					lauf(z, append(pfad, fn))
				}
			}
		}
		lauf(wurzel, nil)
	}

	if len(funde) > 0 {
		var b strings.Builder
		b.WriteString("accounts.Range ist vom Ueberweisungspfad aus erreichbar. " +
			"Range sperrt JEDEN der numAccountShards Shards einzeln -- je Ueberweisung. " +
			"Genau das war 2026-07-23 einmal 57 % der gesamten CPU-Zeit " +
			"(siehe bootstrapMultiplierLocked in state.go).\n\n")
		for _, f := range funde {
			b.WriteString("  " + f.pfad + "\n      " + stelle[f.ziel] + "\n")
		}
		b.WriteString("\nRichtiger Weg: einen gepflegten Zaehler benutzen (wie humanCountLocked " +
			"in cs.useDB-Betrieb), nicht scannen. Ist der Scan wirklich noetig und billig, " +
			"gehoert die Funktion mit Begruendung in bekannteGrenzen -- aber dann bitte mit " +
			"einer Zahl dahinter, nicht mit einer Vermutung.")
		t.Fatal(b.String())
	}
}

// Die Ausnahme fuer snapshotForRollbackLocked haelt nur, solange der
// Ueberweisungspfad wirklich full=false uebergibt.
//
// Ohne diesen Test waere der Eintrag in bekannteGrenzen ein Freibrief: ein
// einziges gekipptes Argument macht aus jeder Ueberweisung einen vollen Scan
// ueber alle 16.384 Shards, und der Wachtest darueber wuerde weiter gruen
// melden, weil er den Namen kennt und nicht das Argument.
//
// Geprueft wird das ZWEITE Argument von runAtomicWithOutbox in den
// Ueberweisungsfunktionen: es muss das Literal false sein. Eine Variable
// genuegt nicht -- dann steht der Wert erst zur Laufzeit fest und die Ausnahme
// ist nicht mehr belegbar.
func TestUeberweisungNimmtNieDenVollenSnapshot(t *testing.T) {
	pflicht := map[string]bool{
		"transferAtomicDirect":    false,
		"TransferWithV7FeeAtomic": false,
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return strings.HasSuffix(fi.Name(), ".go") && !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("Paket nicht lesbar: %v", err)
	}

	gefunden := map[string]bool{}
	for _, pkg := range pkgs {
		for _, datei := range pkg.Files {
			ast.Inspect(datei, func(n ast.Node) bool {
				fd, ok := n.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					return true
				}
				if _, noetig := pflicht[fd.Name.Name]; !noetig {
					return true
				}
				ast.Inspect(fd.Body, func(m ast.Node) bool {
					ce, ok := m.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := ce.Fun.(*ast.SelectorExpr)
					if !ok || sel.Sel.Name != "runAtomicWithOutbox" || len(ce.Args) < 2 {
						return true
					}
					gefunden[fd.Name.Name] = true
					id, ok := ce.Args[1].(*ast.Ident)
					if !ok || id.Name != "false" {
						t.Errorf("%s uebergibt fullSnapshot als %s (%s) -- erwartet das Literal false.\n"+
							"Ein voller Snapshot laesst snapshotForRollbackLocked ueber ALLE "+
							"numAccountShards Shards laufen, und zwar je Ueberweisung. Genau diese "+
							"Kostenform war 2026-07-23 einmal 57 %% der gesamten CPU-Zeit.\n"+
							"Wird das absichtlich geaendert, gehoert snapshotForRollbackLocked aus "+
							"bekannteGrenzen in range_auf_heissem_pfad_test.go entfernt -- sonst "+
							"deckt eine Ausnahme etwas, das nicht mehr gilt.",
							fd.Name.Name, ast.Print(nil, ce.Args[1]), fset.Position(ce.Args[1].Pos()))
					}
					return true
				})
				return true
			})
		}
	}

	for name := range pflicht {
		if !gefunden[name] {
			t.Errorf("%q ruft runAtomicWithOutbox nicht mehr -- dieser Test prueft dann nichts. "+
				"Liste anpassen, statt den Eintrag stillschweigend zu verlieren", name)
		}
	}
}
