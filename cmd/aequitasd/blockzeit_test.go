package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"time"
)

// mainAnweisungen: die Anweisungen der obersten Ebene in func main, aus dem
// Syntaxbaum -- auskommentierte Zeilen gibt es dort nicht, und eine
// Anweisung in if, go oder einer Funktion ist keine der obersten Ebene
// (Pruefung von #322, LOW-2: ein Textvergleich fand auch "// x()" und
// "go func() { ...; x() }()").
func mainAnweisungen(t *testing.T) []ast.Stmt {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("main.go nicht lesbar: %v", err)
	}
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "main" && fn.Recv == nil {
			return fn.Body.List
		}
	}
	t.Fatal("func main nicht gefunden")
	return nil
}

// istAufruf: ist e ein Aufruf von x.sel (x leer: beliebiger Empfaenger)?
func istAufruf(e ast.Expr, x, sel string) bool {
	c, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	s, ok := c.Fun.(*ast.SelectorExpr)
	if !ok || s.Sel.Name != sel {
		return false
	}
	id, ok := s.X.(*ast.Ident)
	return x == "" || (ok && id.Name == x)
}

// direkterAufruf: Index der ersten Anweisung der obersten Ebene, die selbst
// der Aufruf x.sel(...) ist; -1, wenn es keine gibt.
func direkterAufruf(liste []ast.Stmt, x, sel string) int {
	for i, st := range liste {
		if es, ok := st.(*ast.ExprStmt); ok && istAufruf(es.X, x, sel) {
			return i
		}
	}
	return -1
}

// enthaeltAufruf: Index der ersten Anweisung der obersten Ebene, die den
// Aufruf x.sel(...) irgendwo enthaelt (auch in einer Funktion); -1 sonst.
func enthaeltAufruf(liste []ast.Stmt, x, sel string) int {
	for i, st := range liste {
		gefunden := false
		ast.Inspect(st, func(n ast.Node) bool {
			if e, ok := n.(ast.Expr); ok && istAufruf(e, x, sel) {
				gefunden = true
			}
			return !gefunden
		})
		if gefunden {
			return i
		}
	}
	return -1
}

// Der Takt war aus gutem Grund fest verdrahtet: am 04.07.2026 wurde er in
// einer Nacht 1s -> 2s -> 6s gedreht, waehrend die eigentlichen
// Konvergenzfehler noch offen waren, und jede Drehung sah kurz nach Besserung
// aus. Ihn wieder einstellbar zu machen holt genau diese Gefahr zurueck --
// darum die Grenzen, und darum diese Tests.

func TestBlockZeit_LeerBleibtBeiDerVorgabe(t *testing.T) {
	t.Setenv("BLOCK_TIME_MS", "")
	if got := blockZeit(); got != BLOCK_TIME_VORGABE {
		t.Fatalf("blockZeit() = %v ohne gesetzte Umgebung, erwartet %v", got, BLOCK_TIME_VORGABE)
	}
}

func TestBlockZeit_NimmtGueltigenWert(t *testing.T) {
	t.Setenv("BLOCK_TIME_MS", "500")
	if got := blockZeit(); got != 500*time.Millisecond {
		t.Fatalf("blockZeit() = %v bei BLOCK_TIME_MS=500, erwartet 500ms", got)
	}
}

// Die Grenzen sind der eigentliche Zweck. Ein Tippfehler darf nicht die Kette
// anhalten: unter 200 ms liegt der Takt in der Groessenordnung der Laufzeit
// zwischen den Boxen, und dann entstehen Bloecke schneller, als der andere sie
// sehen kann -- das Ergebnis waeren Waisen statt Transaktionen.
func TestBlockZeit_WeistUnsinnZurueckStattIhnZuUebernehmen(t *testing.T) {
	for _, fall := range []struct {
		roh   string
		warum string
	}{
		{"50", "unter der Untergrenze: schneller als die Laufzeit zwischen den Boxen"},
		{"0", "null waere ein Ticker ohne Pause"},
		{"-500", "negativ"},
		{"60000", "ueber der Obergrenze: der Totmannschalter liest so lange Luecken als Ausfall"},
		{"eine Sekunde", "keine Zahl"},
		{"1,5", "kein gueltiges Zahlenformat"},
	} {
		t.Run(fall.roh, func(t *testing.T) {
			t.Setenv("BLOCK_TIME_MS", fall.roh)
			if got := blockZeit(); got != BLOCK_TIME_VORGABE {
				t.Fatalf("blockZeit() = %v bei BLOCK_TIME_MS=%q (%s), erwartet die Vorgabe %v. "+
					"Ein unsinniger Wert muss auf die Vorgabe zurueckfallen, nicht uebernommen werden.",
					got, fall.roh, fall.warum, BLOCK_TIME_VORGABE)
			}
		})
	}
}

// Die Reihenfolge ist die halbe Aenderung: wird BLOCK_TIME erst nach
// TuneProposerBreakerForBlockTime gesetzt, skalieren Schutzschalter und
// Finalitaetsspielraum weiter auf den alten Takt -- die Kette liefe schneller,
// ihre Schwellen aber nicht mit. Das faellt in keinem Einzeltest auf, sondern
// erst live als wiederkehrendes Ausloesen des Schutzschalters.
func TestBlockZeit_WirdVorDenAbgeleitetenSchwellenGesetzt(t *testing.T) {
	liste := mainAnweisungen(t)
	setzen := -1
	for i, st := range liste {
		if as, ok := st.(*ast.AssignStmt); ok && len(as.Lhs) == 1 && len(as.Rhs) == 1 {
			if id, ok := as.Lhs[0].(*ast.Ident); ok && id.Name == "BLOCK_TIME" {
				if c, ok := as.Rhs[0].(*ast.CallExpr); ok {
					if f, ok := c.Fun.(*ast.Ident); ok && f.Name == "blockZeit" {
						setzen = i
						break
					}
				}
			}
		}
	}
	if setzen < 0 {
		t.Fatal("BLOCK_TIME wird in main nicht (direkt) aus blockZeit() gesetzt -- die Umgebungsvariable " +
			"BLOCK_TIME_MS waere damit wirkungslos, ohne dass es jemand merkt.")
	}
	for _, abgeleitet := range [][2]string{
		{"keeper", "TuneProposerBreakerForBlockTime"},
		{"keeper", "TuneFinalitySlackForBlockTime"},
	} {
		// Direkt, nicht in defer, go oder if (Pruefung von #322, LOW-5): in
		// main liefe ein defer nie, und der Schutzschalter bliebe auf dem
		// Vorgabetakt.
		if i := direkterAufruf(liste, abgeleitet[0], abgeleitet[1]); i < 0 || i < setzen {
			t.Errorf("%s.%s steht nicht als eigene Anweisung nach der Zuweisung von BLOCK_TIME (vorher, verschachtelt oder fehlt)", abgeleitet[0], abgeleitet[1])
		}
	}
	// Das Banner nennt die tatsaechliche Blockzeit.
	for i, st := range liste {
		ast.Inspect(st, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok && istAufruf(c, "fmt", "Printf") && len(c.Args) > 0 {
				if lit, ok := c.Args[0].(*ast.BasicLit); ok && strings.HasPrefix(lit.Value, "\"Block Time:") && i < setzen {
					t.Errorf("fmt.Printf(\"Block Time: ...\") steht vor der Zuweisung von BLOCK_TIME")
				}
			}
			return true
		})
	}
}

// Die Annahme-Pause misst ab dem Start, nicht ab dem ersten
// Erzeugungsversuch (Pruefung von #318): der Aufruf muss als eigene
// Anweisung in main vor dem API-Server stehen, sonst nehmen die HTTP-Wege
// waehrend Bootstrap und Resync ohne Pause an. Auskommentiert, bedingt oder
// verzoegert zaehlt nicht (Pruefung von #322, LOW-2).
func TestAnnahmeMessung_BeginntVorDemAPIServer(t *testing.T) {
	liste := mainAnweisungen(t)
	messen := direkterAufruf(liste, "chainState", "AnnahmeMessungBeginnen")
	if messen < 0 {
		t.Fatal("main beginnt die Messung der Annahme-Pause nicht als eigene Anweisung (chainState.AnnahmeMessungBeginnen())")
	}
	for _, spaeter := range [][2]string{{"keeper", "NewAPIServer"}, {"api", "Start"}, {"", "StartHTTPBlockSync"}} {
		if i := enthaeltAufruf(liste, spaeter[0], spaeter[1]); i < 0 || i < messen {
			t.Errorf("%s.%s steht vor AnnahmeMessungBeginnen (oder fehlt)", spaeter[0], spaeter[1])
		}
	}
}

// Die Pruefhilfen selbst: was sie nicht als Anweisung der obersten Ebene
// erkennen duerfen.
func TestMainAnweisungen_ErkenntNurEchteAufrufe(t *testing.T) {
	quelle := `package main
func main() {
	// chainState.AnnahmeMessungBeginnen()
	if x { chainState.AnnahmeMessungBeginnen() }
	go func() { chainState.AnnahmeMessungBeginnen() }()
	keeper.SafeGoroutine("a", func() { api.Start(1) })
}`
	f, err := parser.ParseFile(token.NewFileSet(), "probe.go", quelle, 0)
	if err != nil {
		t.Fatal(err)
	}
	liste := f.Decls[0].(*ast.FuncDecl).Body.List
	if i := direkterAufruf(liste, "chainState", "AnnahmeMessungBeginnen"); i >= 0 {
		t.Fatalf("auskommentierter, bedingter oder verzoegerter Aufruf als Anweisung %d erkannt", i)
	}
	// Der Kommentar ist keine Anweisung: if (0), go (1), SafeGoroutine (2).
	if i := enthaeltAufruf(liste, "api", "Start"); i != 2 {
		t.Fatalf("api.Start in der Funktion: Anweisung %d, erwartet 2", i)
	}
	if !strings.Contains(quelle, "AnnahmeMessungBeginnen") {
		t.Fatal("Probe kaputt")
	}
}

// Mit AEQUITAS_LEITUNG=an haelt die Annahme an, bis StarteLeitung gelaufen
// ist (annahme_pause.go). main muss es darum immer aufrufen -- als eigene
// Anweisung, nicht hinter einer Bedingung, die es auslassen koennte.
func TestMain_StartetLeitung(t *testing.T) {
	if direkterAufruf(mainAnweisungen(t), "keeper", "StarteLeitung") < 0 {
		t.Fatal("main ruft keeper.StarteLeitung nicht als eigene Anweisung auf -- mit AEQUITAS_LEITUNG=an bliebe die Annahme angehalten")
	}
}

// Ein Knoten mit nicht eingespieltem WAL-Rest darf nicht starten
// (wal_rest.go): main prueft das direkt nach NewChainState, vor Blockchain,
// P2P und Sync, und beendet sich bei einem Fehler.
func TestMain_WALRestVorDemStart(t *testing.T) {
	liste := mainAnweisungen(t)
	pruefen := enthaeltAufruf(liste, "chainState", "PruefeWALRest")
	if pruefen < 0 {
		t.Fatal("main ruft chainState.PruefeWALRest nicht auf")
	}
	wenn, ok := liste[pruefen].(*ast.IfStmt)
	if !ok || enthaeltAufruf(wenn.Body.List, "os", "Exit") < 0 {
		t.Fatal("PruefeWALRest steht nicht in einem if, dessen Rumpf os.Exit aufruft")
	}
	for _, spaeter := range [][2]string{{"keeper", "NewBlockchain"}, {"p2pNode", "SetDAG"}, {"keeper", "NewAPIServer"}, {"", "StartHTTPBlockSync"}} {
		if i := enthaeltAufruf(liste, spaeter[0], spaeter[1]); i >= 0 && i < pruefen {
			t.Errorf("%s.%s steht vor PruefeWALRest", spaeter[0], spaeter[1])
		}
	}
	if i := enthaeltAufruf(liste, "keeper", "NewChainState"); i < 0 || i > pruefen {
		t.Error("PruefeWALRest steht nicht nach NewChainState")
	}
}

// Beim geordneten Beenden schreibt main den WAL-Rest nach Postgres
// (Pruefung von #331, 2. Durchgang, Befund 3) -- als eigene Anweisung nach
// dem Warten auf das Signal.
func TestMain_FlushtWALBeimBeenden(t *testing.T) {
	liste := mainAnweisungen(t)
	warten := -1
	for i, st := range liste {
		if es, ok := st.(*ast.ExprStmt); ok {
			if u, ok := es.X.(*ast.UnaryExpr); ok && u.Op == token.ARROW {
				if id, ok := u.X.(*ast.Ident); ok && id.Name == "quit" {
					warten = i
				}
			}
		}
	}
	if warten < 0 {
		t.Fatal("<-quit nicht als Anweisung in main gefunden")
	}
	if i := direkterAufruf(liste, "chainState", "FlushWALNow"); i < 0 || i < warten {
		t.Fatalf("chainState.FlushWALNow() nicht als eigene Anweisung nach <-quit (Index %d, <-quit %d)", i, warten)
	}
}
