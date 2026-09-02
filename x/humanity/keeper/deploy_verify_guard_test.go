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
	for _, f := range []string{"deploy-contabo.yml", "deploy-contabo2.yml"} {
		// Assert on the shell logic only. The comments quote the old code as
		// the record of what went wrong, and must not read as the old code
		// still being there.
		step := guardControlFlow(deployStep(t, f, "alive and advancing"))

		// The shape that caused the false failure: read height, sleep once,
		// read again, decide. Any fixed single window measures the deploy's
		// timing rather than the node's health.
		if regexp.MustCompile(`sleep 60\b`).MatchString(step) {
			t.Errorf("%s: the guard still sleeps a single fixed 60s between two height "+
				"samples. That is the 2026-08-19 false failure: a node that is replaying "+
				"is not a node that is stuck. Sample until the height moves.", f)
		}
		if strings.Contains(step, "-le") {
			t.Errorf("%s: the guard still decides on `-le` against the previous sample. "+
				"Compare each sample against the BASELINE and succeed as soon as one "+
				"exceeds it, so slow startup is not a failure.", f)
		}

		// It must keep looking rather than judging once.
		if !strings.Contains(step, "-gt") || !strings.Contains(step, "advanced=yes") {
			t.Errorf("%s: the guard must poll until the height exceeds its baseline "+
				"(`-gt` … advanced=yes), not judge a single interval.", f)
		}

		// Every unreadable answer must land in the same bucket as no answer.
		// Without this the comparison errors and the step reports success.
		if !strings.Contains(step, `*[!0-9]*`) {
			t.Errorf("%s: the guard must reject a non-numeric .height explicitly "+
				"(a `case` guard on *[!0-9]*). Comparing it with `[ … -gt … ]` is a bash "+
				"error, and an erroring condition is false — which reports a PASS for a "+
				"node whose height was never read.", f)
		}
	}

	// Contabo2 lacked this check entirely until 2026-08-15, and Contabo1's copy
	// is where it was written first. Two hand-maintained copies of one guard
	// drift; assert the control flow is identical and let the prose differ.
	c1 := guardControlFlow(deployStep(t, "deploy-contabo.yml", "alive and advancing"))
	c2 := guardControlFlow(deployStep(t, "deploy-contabo2.yml", "alive and advancing"))
	if c1 != c2 {
		t.Errorf("the two deploy guards have drifted apart.\ncontabo1:\n%s\ncontabo2:\n%s", c1, c2)
	}
}

// Contabo2 must not fall open while Contabo1 is still deploying.
//
// Contabo2's deploy waits for Contabo1's run of the same commit and, on
// timeout, proceeds anyway — deliberately, so a wedged Contabo1 can never
// permanently block shipping a fix. That fail-open is only safe while the wait
// actually outlasts Contabo1's job. Making the guard above patient added up to
// ten minutes to that job, and nothing in either file states the coupling, so
// the next person to raise a budget would silently turn the staggered rollout
// back into the simultaneous restart that forked the network twice on
// 2026-07-25.
func TestDeployGuard_Contabo2OutwaitsContabo1(t *testing.T) {
	c2Wait := loopBudget(t, deployStep(t, "deploy-contabo2.yml", "deploy of this same commit"))
	c1Wait := loopBudget(t, deployStep(t, "deploy-contabo.yml", "never restart both validators"))
	c1Verify := loopBudget(t, deployStep(t, "deploy-contabo.yml", "alive and advancing"))

	// Contabo1's job is its wait for this box, plus the image build, plus the
	// verify. The build is not expressed in the workflow; 10 minutes is roughly
	// double the observed worst case (7m36s on 2026-08-19).
	const buildAllowance = 600
	need := c1Wait + c1Verify + buildAllowance
	if c2Wait <= need {
		t.Errorf("Contabo2 waits %ds for Contabo1, but Contabo1's job can take %ds "+
			"(wait %ds + build ~%ds + verify %ds). Contabo2 would fail open and restart "+
			"while Contabo1 is still mid-rollout — the simultaneous restart the "+
			"staggering exists to prevent.", c2Wait, need, c1Wait, buildAllowance, c1Verify)
	}
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
