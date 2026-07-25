package keeper

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// activetx_static_test.go is the completeness half of the cs.activeTx
// migration gate (Roadmap step 5). activetx_trace.go measures which
// fallbacks actually FIRE; that is sound but only as complete as the tests
// that happen to run against a real Postgres — the vast majority of this
// package's tests run with cs.db == nil, where no transaction exists to
// fall back to and the trace is silent by construction.
//
// This test closes that gap without depending on coverage at all: it reads
// the package's own source, builds the intra-package call graph, and walks
// it from the two places where a transaction is opened and parked in
// cs.activeTx — the closures handed to runAtomicWithOutbox /
// runAtomicDistributionWithOutbox, and BlockDAG.replayTransactions. Any
// function reachable from there that still reaches the database WITHOUT
// threading the operation's ctx is a path that only works because of the
// implicit cs.activeTx field, and therefore a path that must be migrated
// before two atomic operations can ever run concurrently.
//
// Three things count as "reaches the database without threading ctx":
//
//  1. calling cs.dbExec() — the ctx-less executor, whose entire behavior is
//     "find the transaction in the ChainState field";
//  2. calling the ctx-less wrapper F of a function that already has a FCtx
//     sibling in this package — the codebase's own marker for "this call
//     site has not been migrated yet" (see dbExecCtx's comment);
//  3. handing context.Background() (or context.TODO()) to ANY function in
//     this package that takes a context.Context — the same thing spelled
//     explicitly: the operation's ctx exists at that point and is being
//     dropped on the floor. This deliberately covers more than the *Ctx
//     naming convention, because the biggest single unmigrated caller,
//     BlockDAG.replayTransactions, dropped ctx into two dozen
//     apply*DeltaLocked functions that carry no Ctx suffix at all.
//
// Goroutine boundaries are NOT followed: work started with `go` or
// SafeGoroutine runs outside the operation's transaction by definition, and
// is precisely what dbExecCtx's [DB-GUARD] branch already refuses to hand
// the transaction to.
//
// Some call sites genuinely run outside the transaction even though they sit
// inside an atomic function's body — the rollback snapshot taken BEFORE
// Begin(), and the restore/nullifier-release that run AFTER Commit() or
// Rollback() has already resolved it. Those carry an explicit
//
//	// activetx:outside-tx — <why>
//
// marker on the call line or in the comment block directly above it. The
// marker is a claim about ordering that a reader can check against the
// surrounding code in seconds, and the test prints every one of them, so
// they stay visible rather than disappearing into an opaque allowlist.
//
// The gate is a fixed expected-set rather than "must be zero" so it can be
// tightened one migration at a time, with every intermediate state pinned:
// a newly added unmigrated write fails this test, and a completed migration
// fails it too until the entry is removed. Both directions are what make it
// a gate instead of a comment.

// knownUnmigratedPaths is the set of (reachable function → unmigrated call)
// pairs that still exist. EMPTY is the goal state; every entry here is a
// concrete, still-open piece of Roadmap step 5.
//
// Format: "caller -> callee".
var knownUnmigratedPaths = map[string]string{}

func TestActiveTx_NoUnmigratedWritesReachableFromAtomicRoots(t *testing.T) {
	pkg := parseKeeperPackage(t)

	found := pkg.unmigratedFromAtomicRoots()

	// Surface the deliberate exemptions on every run — an outside-tx claim
	// that stops being true is a real bug, and it can only be spotted if
	// the claims are printed rather than silently skipped.
	if ex := pkg.outsideTxExemptions(); len(ex) > 0 {
		t.Logf("%d call site(s) marked `activetx:outside-tx` (run outside the transaction by\n"+
			"construction — snapshot before Begin, restore after Commit/Rollback):\n  %s",
			len(ex), strings.Join(ex, "\n  "))
	}

	var unexpected []string
	for _, f := range found {
		if _, ok := knownUnmigratedPaths[f.key()]; !ok {
			unexpected = append(unexpected, f.String())
		}
	}
	seen := map[string]bool{}
	for _, f := range found {
		seen[f.key()] = true
	}
	var stale []string
	for k := range knownUnmigratedPaths {
		if !seen[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(unexpected)
	sort.Strings(stale)

	if len(unexpected) > 0 {
		t.Errorf("%d NEW unmigrated DB write path(s) reachable from an atomic root.\n"+
			"Each of these only works because cs.activeTx is a ChainState-wide field, which is\n"+
			"exactly what blocks two atomic operations from running at the same time.\n"+
			"Thread the operation's ctx through instead (see dbExecCtx's comment), or — if this\n"+
			"genuinely runs outside the transaction — start it via SafeGoroutine so the boundary\n"+
			"is explicit.\n\n  %s", len(unexpected), strings.Join(unexpected, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("%d entr(y/ies) in knownUnmigratedPaths no longer exist — the migration moved\n"+
			"forward; delete them so the gate keeps its teeth:\n\n  %s",
			len(stale), strings.Join(stale, "\n  "))
	}
}

// ---------------------------------------------------------------------------
// call graph
// ---------------------------------------------------------------------------

type callSite struct {
	caller string
	callee string
	reason string
	pos    string
}

func (c callSite) key() string    { return c.caller + " -> " + c.callee }
func (c callSite) String() string { return fmt.Sprintf("%s  [%s] at %s", c.key(), c.reason, c.pos) }

type keeperPkg struct {
	fset *token.FileSet
	// bodies maps a function key to every body belonging to it: the
	// declaration itself plus any func literal defined inside it that is
	// NOT a goroutine boundary (a literal passed to SafeGoroutine or used
	// in a `go` statement runs outside the caller's transaction).
	bodies map[string][]*ast.BlockStmt
	// hasCtxSibling[F] is true when a function FCtx also exists in this
	// package — i.e. F is a ctx-less wrapper and calling it is a migration
	// marker.
	hasCtxSibling map[string]bool
	// ctxTakers[F] is true when F's first parameter is a context.Context.
	// Passing context.Background() to one of these from inside an atomic
	// operation drops the transaction just as surely as calling a ctx-less
	// wrapper does.
	ctxTakers map[string]bool
	// atomicRootLits are the func literals passed to runAtomic*WithOutbox.
	atomicRoots []string
	// srcLines holds each parsed file's lines, for reading the
	// activetx:outside-tx markers back out at a given call position.
	srcLines map[string][]string
	// exempt collects the marked call sites, for reporting.
	exempt []string
}

func parseKeeperPackage(t *testing.T) *keeperPkg {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	p := &keeperPkg{
		fset:          fset,
		bodies:        map[string][]*ast.BlockStmt{},
		hasCtxSibling: map[string]bool{},
		ctxTakers:     map[string]bool{},
		srcLines:      map[string][]string{},
	}
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		f, err := parser.ParseFile(fset, filepath.Join(".", name), src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		p.srcLines[name] = strings.Split(string(src), "\n")
		files = append(files, f)
	}
	if len(files) == 0 {
		t.Fatal("no non-test sources parsed — is this test running outside the package dir?")
	}

	declared := map[string]bool{}
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			name := funcDeclKey(fd)
			declared[name] = true
			if takesCtxFirst(fd) {
				p.ctxTakers[name] = true
			}
			p.bodies[name] = append(p.bodies[name], fd.Body)
			// Func literals inside this declaration belong to the same
			// synchronous flow unless they cross a goroutine boundary.
			for _, lit := range syncFuncLits(fd.Body) {
				p.bodies[name] = append(p.bodies[name], lit.Body)
			}
		}
	}
	for name := range declared {
		if strings.HasSuffix(name, "Ctx") {
			p.hasCtxSibling[strings.TrimSuffix(name, "Ctx")] = true
		}
	}
	// A ctx-less name only counts as a migration marker if the ctx-less
	// wrapper genuinely exists as its own declaration.
	for name := range p.hasCtxSibling {
		if !declared[name] {
			delete(p.hasCtxSibling, name)
		}
	}

	p.atomicRoots = []string{"replayTransactions"}
	return p
}

// funcDeclKey names a declaration the way call sites refer to it: bare
// function name, or plain method name for methods. Method names are not
// qualified by receiver type on purpose — call expressions in the AST
// (`cs.foo()`) carry no type information without full type checking, so the
// graph keys on the selector name alone. That over-approximates (two
// same-named methods on different types merge into one node), which for a
// safety gate errs in the right direction: it can report a path that does
// not exist, never miss one that does.
func funcDeclKey(fd *ast.FuncDecl) string { return fd.Name.Name }

// syncFuncLits returns every func literal in body that runs synchronously
// within the enclosing function — skipping the ones started as goroutines
// (`go func(){}()`) or handed to SafeGoroutine, which by definition execute
// outside the caller's transaction.
func syncFuncLits(body *ast.BlockStmt) []*ast.FuncLit {
	async := map[*ast.FuncLit]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.GoStmt:
			if lit, ok := s.Call.Fun.(*ast.FuncLit); ok {
				async[lit] = true
			}
		case *ast.CallExpr:
			if calleeName(s.Fun) == "SafeGoroutine" {
				for _, a := range s.Args {
					if lit, ok := a.(*ast.FuncLit); ok {
						async[lit] = true
					}
				}
			}
		}
		return true
	})
	var out []*ast.FuncLit
	ast.Inspect(body, func(n ast.Node) bool {
		lit, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}
		if async[lit] {
			return false // and don't descend: nested literals are async too
		}
		out = append(out, lit)
		return true
	})
	return out
}

func calleeName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

// callsIn returns the callee names invoked directly in body, plus any
// unmigrated-write call sites found there.
func (p *keeperPkg) callsIn(caller string, body *ast.BlockStmt) (callees []string, bad []callSite) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := calleeName(call.Fun)
		if name == "" {
			return true
		}
		callees = append(callees, name)

		// A ctx-less wrapper delegating to its own *Ctx implementation is
		// the wrapper's DEFINITION, not a call site to migrate — the thing
		// that needs migrating is whoever still calls the wrapper, which
		// this walk reports separately.
		if name == caller+"Ctx" {
			return true
		}
		var reason string
		switch {
		case name == "dbExec":
			reason = "ctx-less executor"
		case p.hasCtxSibling[name]:
			reason = "ctx-less wrapper of " + name + "Ctx"
		case p.ctxTakers[name] && hasBackgroundArg(call):
			reason = "context.Background() passed to a ctx-taking function"
		}
		if reason != "" {
			if p.markedOutsideTx(call) {
				p.exempt = append(p.exempt, caller+" -> "+name+" at "+p.pos(call))
			} else {
				bad = append(bad, callSite{caller, name, reason, p.pos(call)})
			}
		}
		return true
	})
	return callees, bad
}

// takesCtxFirst reports whether fd's first parameter is a context.Context.
func takesCtxFirst(fd *ast.FuncDecl) bool {
	if fd.Type.Params == nil || len(fd.Type.Params.List) == 0 {
		return false
	}
	sel, ok := fd.Type.Params.List[0].Type.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "context" && sel.Sel.Name == "Context"
}

func hasBackgroundArg(call *ast.CallExpr) bool {
	for _, a := range call.Args {
		inner, ok := a.(*ast.CallExpr)
		if !ok {
			continue
		}
		sel, ok := inner.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		pkg, ok := sel.X.(*ast.Ident)
		if ok && pkg.Name == "context" && (sel.Sel.Name == "Background" || sel.Sel.Name == "TODO") {
			return true
		}
	}
	return false
}

// markedOutsideTx reports whether the call carries an
// `activetx:outside-tx` marker — on its own line, or in the contiguous
// comment block immediately above it.
func (p *keeperPkg) markedOutsideTx(call *ast.CallExpr) bool {
	pos := p.fset.Position(call.Pos())
	lines, ok := p.srcLines[filepath.Base(pos.Filename)]
	if !ok || pos.Line < 1 || pos.Line > len(lines) {
		return false
	}
	const marker = "activetx:outside-tx"
	if strings.Contains(lines[pos.Line-1], marker) {
		return true
	}
	for i := pos.Line - 2; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(t, "//") {
			return false
		}
		if strings.Contains(t, marker) {
			return true
		}
	}
	return false
}

// outsideTxExemptions returns the marked call sites found during the last
// walk, sorted and deduplicated.
func (p *keeperPkg) outsideTxExemptions() []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range p.exempt {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	sort.Strings(out)
	return out
}

func (p *keeperPkg) pos(n ast.Node) string {
	pos := p.fset.Position(n.Pos())
	return fmt.Sprintf("%s:%d", filepath.Base(pos.Filename), pos.Line)
}

// unmigratedFromAtomicRoots walks the call graph from every atomic root and
// returns each unmigrated DB write reachable from one.
func (p *keeperPkg) unmigratedFromAtomicRoots() []callSite {
	roots := append([]string{}, p.atomicRoots...)
	roots = append(roots, p.atomicClosureCallees()...)

	visited := map[string]bool{}
	var out []callSite
	var walk func(fn string)
	walk = func(fn string) {
		if visited[fn] {
			return
		}
		visited[fn] = true
		for _, body := range p.bodies[fn] {
			callees, bad := p.callsIn(fn, body)
			out = append(out, bad...)
			for _, c := range callees {
				if _, known := p.bodies[c]; known {
					walk(c)
				}
			}
		}
	}
	for _, r := range roots {
		walk(r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// atomicClosureCallees finds the func literals handed to
// runAtomicWithOutbox / runAtomicDistributionWithOutbox and returns the
// functions they call — the closures themselves have no name, so their
// bodies are analysed here directly and their callees become extra roots.
func (p *keeperPkg) atomicClosureCallees() []string {
	var roots []string
	for fn, bodies := range p.bodies {
		for _, body := range bodies {
			ast.Inspect(body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch calleeName(call.Fun) {
				case "runAtomicWithOutbox", "runAtomicDistributionWithOutbox":
				default:
					return true
				}
				for _, a := range call.Args {
					lit, ok := a.(*ast.FuncLit)
					if !ok {
						continue
					}
					callees, _ := p.callsIn(fn, lit.Body)
					roots = append(roots, callees...)
				}
				return true
			})
		}
	}
	return roots
}
