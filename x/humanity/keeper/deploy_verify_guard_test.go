package keeper

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The post-deploy "alive and advancing" guard must be patient, must refuse a
// height it could not read, and must be the same on both boxes.
//
// THE INCIDENT THIS PINS (2026-08-19, the second one that day).
//
// The hotfix that got Contabo2 off height 0 deployed cleanly, the node rebuilt
// its DAG from Postgres correctly, and this step failed the deploy anyway:
//
//	height 4166335 -> 4166335 over 60s
//	FAILED: the node answers but its height did not move in 60 seconds.
//
// Half an hour later that same box was at 4168501, merging tips with Contabo1,
// checkpoints advancing — it had been healthy the whole time. The guard read
// its two samples during the window between "answers /api/health/combined" and
// "starts merging", which on a box that stores its payloads gzipped is minutes
// wide, because startup has to gunzip and replay millions of rows first.
//
// A red deploy on a healthy validator is not a harmless false alarm. It is
// indistinguishable from a real stall, so the next real one reads as noise; and
// it trips the non-success branch of Contabo2's wait step, which by design then
// refuses to deploy at all. The 60-second sample was measuring the deploy's
// timing, not the node's health.
//
// The same block had a quieter defect in the other direction. `[ "$h2" -le
// "$h1" ]` on a non-numeric height is a bash *error*, and an erroring `if`
// condition is merely false, so the step fell through to its success line. Fed
// `{"height":"unknown"}` the shipped guard printed "OK: node is alive and
// advancing" and exited 0, having never read a height at all.
//
// This is a source-level test on purpose: the guard only ever runs on a real
// deploy against real boxes, which is precisely where nobody can afford to
// discover it is wrong.
func TestDeployGuard_WaitsForAdvancementAndRejectsUnreadableHeight(t *testing.T) {
	// Bis zum 23.10.2026 stand der Waechter als Schritt in deploy-contabo.yml
	// und deploy-contabo2.yml. Beide Boxen sind abgeschaltet, die Workflows
	// geloescht; der Push-Deploy deploy-c1.yml ruft deploy/pruefe-knoten.sh
	// auf. Dort steht der Waechter heute -- und dort gelten dieselben Regeln.
	wired := deployStep(t, "deploy-c1.yml", "Hoehe steigt")
	if !strings.Contains(wired, "deploy/pruefe-knoten.sh") {
		t.Fatalf("deploy-c1.yml: the step after the rollout no longer runs " +
			"deploy/pruefe-knoten.sh -- the guard below would not run on a deploy at all")
	}
	src, err := os.ReadFile("../../../deploy/pruefe-knoten.sh")
	if err != nil {
		t.Fatalf("read deploy/pruefe-knoten.sh: %v", err)
	}
	// Assert on the shell logic only (comments and echo lines stripped).
	step := guardControlFlow(string(src))
	const f = "deploy/pruefe-knoten.sh"

	// The shape that caused the false failure: read height, sleep once,
	// read again, decide. Any fixed single window measures the deploy's
	// timing rather than the node's health.
	if regexp.MustCompile(`sleep 60\b`).MatchString(step) {
		t.Errorf("%s: the guard sleeps a single fixed 60s between two height "+
			"samples. That is the 2026-08-19 false failure: a node that is replaying "+
			"is not a node that is stuck. Sample until the height moves.", f)
	}
	if strings.Contains(step, "-le") {
		t.Errorf("%s: the guard decides on `-le` against the previous sample. "+
			"Compare each sample against the BASELINE and succeed as soon as one "+
			"exceeds it, so slow startup is not a failure.", f)
	}

	// It must keep looking rather than judging once: a polling loop, and
	// success only when a sample exceeds the baseline.
	if !strings.Contains(step, `-gt "$h0"`) || !regexp.MustCompile(`seq 1 \d+\)`).MatchString(step) {
		t.Errorf("%s: the guard must poll until the height exceeds its baseline "+
			"(`-gt \"$h0\"` inside a `seq` loop), not judge a single interval.", f)
	}

	// Every unreadable answer must land in the same bucket as no answer.
	// Without this the comparison errors and the step reports success.
	if !strings.Contains(step, `*[!0-9]*`) {
		t.Errorf("%s: the guard must reject a non-numeric .height explicitly "+
			"(a `case` guard on *[!0-9]*). Comparing it with `[ … -gt … ]` is a bash "+
			"error, and an erroring condition is false — which reports a PASS for a "+
			"node whose height was never read.", f)
	}
	// Falling out of the loop without advancing must be a failure.
	if !strings.HasSuffix(strings.TrimSpace(string(src)), "exit 1") {
		t.Errorf("%s: the guard must end with `exit 1` when the height never moved", f)
	}
}

// The guard must finish inside its job's timeout.
//
// deploy-c1.yml rolls back to the previous image when the guard fails
// (rueckrollen-c1.sh, `if: failure()`). If GitHub cancels the job on
// timeout-minutes first, that rollback never runs and C1 stays on an image
// nobody verified. So the build, one full guard and the rollback must fit. This replaces the C1/C2 staggering check from before
// 23.10.2026 (C2 waits longer than C1): with one box there is nothing to
// stagger, but the same arithmetic now has to fit the job's own deadline.
func TestDeployGuard_FitsInsideTheJobTimeout(t *testing.T) {
	src, err := os.ReadFile("../../../deploy/pruefe-knoten.sh")
	if err != nil {
		t.Fatalf("read deploy/pruefe-knoten.sh: %v", err)
	}
	guard := scriptBudget(t, string(src))
	wf, err := os.ReadFile("../../../.github/workflows/deploy-c1.yml")
	if err != nil {
		t.Fatalf("read deploy-c1.yml: %v", err)
	}
	m := regexp.MustCompile(`(?m)^  c1:\n(?:.*\n)*?    timeout-minutes: (\d+)`).FindStringSubmatch(string(wf))
	if m == nil {
		t.Fatalf("deploy-c1.yml: job c1 has no timeout-minutes -- without one the job " +
			"runs up to six hours and the arithmetic below means nothing")
	}
	var minutes int
	fmt.Sscanf(m[1], "%d", &minutes)
	// The image build (10 min, roughly double the observed worst case of
	// 7m36s on 2026-08-19), one full guard, then the rollback (5 min).
	const buildAllowance, rollbackAllowance = 600, 300
	need := buildAllowance + guard + rollbackAllowance
	if minutes*60 <= need {
		t.Errorf("deploy-c1.yml: job c1 times out after %ds, but build ~%ds + guard %ds "+
			"+ rollback ~%ds = %ds. GitHub would cancel the job before the rollback runs.",
			minutes*60, buildAllowance, guard, rollbackAllowance, need)
	}
}

// scriptBudget returns the worst-case seconds of every `for i in $(seq 1 N)`
// loop in a script: N times (its sleep plus every curl -m timeout in the
// loop body, including those of one-line shell functions the body calls).
// Unlike loopBudget it also reads `…; sleep S` at the end of a line, as
// deploy/pruefe-knoten.sh writes it.
func scriptBudget(t *testing.T, src string) int {
	t.Helper()
	parts := regexp.MustCompile(`for i in \$\(seq 1 (\d+)\)`).Split(src, -1)
	counts := regexp.MustCompile(`for i in \$\(seq 1 (\d+)\)`).FindAllStringSubmatch(src, -1)
	if len(counts) == 0 {
		t.Fatalf("no `for i in $(seq 1 N)` loop found -- update the arithmetic here rather than deleting it")
	}
	fnTimeout := map[string]int{}
	for _, f := range regexp.MustCompile(`(?m)^(\w+)\(\) \{(.*)\}`).FindAllStringSubmatch(src, -1) {
		for _, x := range regexp.MustCompile(`-m (\d+)`).FindAllStringSubmatch(f[2], -1) {
			var v int
			fmt.Sscanf(x[1], "%d", &v)
			fnTimeout[f[1]] += v
		}
	}
	total := 0
	for i, c := range counts {
		body := parts[i+1]
		if j := strings.Index(body, "\ndone"); j >= 0 {
			body = body[:j]
		}
		var n int
		fmt.Sscanf(c[1], "%d", &n)
		per := 0
		sl := regexp.MustCompile(`\bsleep (\d+)`).FindAllStringSubmatch(body, -1)
		if len(sl) != 1 {
			t.Fatalf("loop %d: expected exactly one `sleep S`, found %d", i+1, len(sl))
		}
		for _, x := range append(sl, regexp.MustCompile(`-m (\d+)`).FindAllStringSubmatch(body, -1)...) {
			var v int
			fmt.Sscanf(x[1], "%d", &v)
			per += v
		}
		for name, v := range fnTimeout {
			if regexp.MustCompile(`\b` + name + `\b`).MatchString(body) {
				per += v
			}
		}
		total += n * per
	}
	return total
}

// deployStep returns the `run:` body of the deploy job step whose name contains
// nameFragment. Text-based rather than YAML-parsed: these files are read and
// edited as text, and the assertions above are about their text.
func deployStep(t *testing.T, file, nameFragment string) string {
	t.Helper()
	path := "../../../.github/workflows/" + file
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(src), "\n")
	start := -1
	for i, ln := range lines {
		if strings.HasPrefix(ln, "      - name:") && strings.Contains(ln, nameFragment) {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("%s: no deploy step whose name contains %q — the step was renamed or "+
			"removed, and the guard it carries is what this test exists to protect", file, nameFragment)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "      - name:") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

// guardControlFlow strips comments, echo lines, blank lines and the box's own
// address, leaving the shell logic — what must be identical on both boxes.
func guardControlFlow(step string) string {
	var out []string
	for _, ln := range strings.Split(step, "\n") {
		s := strings.TrimSpace(ln)
		switch {
		case s == "", strings.HasPrefix(s, "#"), strings.HasPrefix(s, "echo "),
			strings.HasPrefix(s, "- name:"), strings.HasPrefix(s, "run:"),
			strings.HasPrefix(s, "BASE="):
			continue
		}
		out = append(out, s)
	}
	return strings.Join(out, "\n")
}

// loopBudget returns the worst-case seconds a step's `for i in $(seq 1 N)` …
// `sleep S` loops can consume, summed over every loop in the step.
func loopBudget(t *testing.T, step string) int {
	t.Helper()
	loops := regexp.MustCompile(`seq 1 (\d+)\)`).FindAllStringSubmatch(step, -1)
	sleeps := regexp.MustCompile(`(?m)^\s*sleep ([\d.]+)\s*$`).FindAllStringSubmatch(step, -1)
	if len(loops) == 0 || len(loops) != len(sleeps) {
		t.Fatalf("could not read the loop budget: %d `seq 1 N` loop(s) but %d `sleep S` "+
			"line(s). This test assumes one sleep per polling loop; if the step now "+
			"paces itself differently, update the arithmetic here rather than deleting it.",
			len(loops), len(sleeps))
	}
	total := 0
	for i := range loops {
		var n, s float64
		fmt.Sscanf(loops[i][1], "%f", &n)
		fmt.Sscanf(sleeps[i][1], "%f", &s)
		total += int(n * s)
	}
	return total
}
