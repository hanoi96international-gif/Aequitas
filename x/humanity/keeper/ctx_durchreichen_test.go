package keeper

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"sort"
	"strings"
	"testing"
)

// TestWerEinenCtxHatDarfKeinenMantelRufen prueft die Regel, an der am
// 29.08.2026 vier Fehler haengen geblieben sind.
//
// # DIE FEHLERFORM
//
// Zu jeder DB-Hilfsfunktion gibt es ein Paar: eine *Ctx-Fassung, die den ctx
// nimmt, und einen gleichnamigen Mantel, der context.Background() einsetzt.
// Der Mantel ist fuer Aufrufer da, die keinen ctx haben -- Startcode,
// Hintergrundlaeufe, Statusabfragen.
//
// Ruft ihn dagegen eine Funktion, die selbst einen ctx fuehrt, wirft sie die
// Transaktion weg. Heute lief das trotzdem: dbExecCtx faellt auf cs.activeTx
// zurueck, also landete der Schreibvorgang doch in der richtigen Transaktion.
// Genau deshalb faellt es nicht auf. In dem Moment, in dem cs.activeTx
// verschwindet -- und das ist das Ziel der ganzen Migration -- schreibt so ein
// Pfad still ausserhalb seiner Transaktion und ueberlebt jeden Ruecklauf.
//
// Gemessen wurden vier solche Pfade, alle im Nachspielen:
// applyTransferDeltaLocked (zweimal), applyTransferBatchParallel und
// ResetFinalizedCheckpoint. Drei Lastlaeufe haben sie gefunden. Pfade, die
// seltener laufen -- die taegliche Verteilung, eine Registrierung, ein Swap,
// der Guardian -- laufen bei leeren Toepfen gar nicht und sind auf diesem Weg
// NICHT pruefbar. Dieser Test prueft sie trotzdem, weil er den Quelltext liest
// und nicht den Betrieb.
//
// # WAS ER NICHT KANN
//
// Er sieht nur den direkten Aufruf. Reicht eine Funktion ihren ctx an eine
// andere ctx-fuehrende Funktion weiter, die ihn dann wegwirft, faellt das hier
// auf -- aber bei DER Funktion, nicht beim Aufrufer. Das genuegt: es wird
// gemeldet, nur eine Ebene tiefer.
func TestWerEinenCtxHatDarfKeinenMantelRufen(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Alle Funktionsnamen sammeln, um die Paare zu finden.
	namen := map[string]bool{}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, d := range f.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok {
					namen[fd.Name.Name] = true
				}
			}
		}
	}
	// Ein Mantel ist ein X, zu dem es ein XCtx gibt.
	maentel := map[string]bool{}
	for n := range namen {
		if !strings.HasSuffix(n, "Ctx") && namen[n+"Ctx"] {
			maentel[n] = true
		}
	}
	if len(maentel) < 10 {
		t.Fatalf("nur %d Maentel erkannt -- die Paarerkennung greift nicht mehr, "+
			"der Test wuerde stillschweigend nichts pruefen", len(maentel))
	}

	// Bewusste Ausnahmen: der Mantel ist hier die richtige Wahl, mit Grund.
	ausnahmen := map[string]string{
		// Der Ruecknahmepfad DARF die Transaktion nicht mitfuehren: sie ist zu
		// dem Zeitpunkt schon zurueckgerollt. Siehe restoreFromRollbackLockedCtx.
		"restoreFromRollbackLockedCtx": "nimmt bewusst einen ctx ohne Transaktion",
	}

	var funde []string
	for _, pkg := range pkgs {
		for pfad, f := range pkg.Files {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Body == nil {
					continue
				}
				if !fuehrtCtx(fd) {
					continue
				}
				if _, frei := ausnahmen[fd.Name.Name]; frei {
					continue
				}
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					ruf, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := ruf.Fun.(*ast.SelectorExpr)
					if !ok || !maentel[sel.Sel.Name] {
						return true
					}
					// Der Mantel selbst ruft seine Ctx-Fassung -- das ist sein Zweck.
					if fd.Name.Name == sel.Sel.Name {
						return true
					}
					pos := fset.Position(ruf.Pos())
					funde = append(funde, fmt.Sprintf("%s:%d  %s ruft %s",
						kurz(pfad), pos.Line, fd.Name.Name, sel.Sel.Name))
					return true
				})
			}
		}
	}

	if len(funde) > 0 {
		sort.Strings(funde)
		t.Fatalf("%d Stelle(n), an denen eine ctx-fuehrende Funktion einen Mantel ruft "+
			"und damit die Transaktion wegwirft:\n  %s\n\n"+
			"Heute faellt das nicht auf, weil dbExecCtx auf cs.activeTx zurueckfaellt und der\n"+
			"Schreibvorgang doch in der richtigen Transaktion landet. Sobald cs.activeTx weg ist --\n"+
			"das Ziel dieser Migration -- schreibt so ein Pfad still ausserhalb seiner Transaktion\n"+
			"und ueberlebt jeden Ruecklauf.\n"+
			"Nimm die *Ctx-Fassung und reiche ctx durch. Soll dort wirklich KEINE Transaktion\n"+
			"gelten, trag es oben als Ausnahme mit Grund ein -- so wie restoreFromRollbackLockedCtx,\n"+
			"wo das Mitfuehren sogar ein Fehler waere.",
			len(funde), strings.Join(funde, "\n  "))
	}
}

// fuehrtCtx sagt, ob die Funktion einen context.Context als Parameter nimmt.
func fuehrtCtx(fd *ast.FuncDecl) bool {
	if fd.Type.Params == nil {
		return false
	}
	for _, p := range fd.Type.Params.List {
		sel, ok := p.Type.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		x, ok := sel.X.(*ast.Ident)
		if ok && x.Name == "context" && sel.Sel.Name == "Context" {
			return true
		}
	}
	return false
}

func kurz(pfad string) string {
	if i := strings.LastIndexAny(pfad, `/\`); i >= 0 {
		return pfad[i+1:]
	}
	return pfad
}
