package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// Rechte ablegen (rechte.go). Missbrauch: ein Pfad, der aus /data
// herausfuehrt, eine symbolische Verknuepfung auf eine fremde Datei, ein
// Ablegen, das still scheitert.

func umgebung(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}

func TestRechte_NurVerzeichnisseUnterData(t *testing.T) {
	got := datenVerzeichnisse(umgebung(map[string]string{
		"AEQUITAS_WAL_PATH": "/data/wal/aequitas_transfers.wal",
		"MPC_TRIPLE_FILE":   "/data/mpc/tripel.bin",
	}))
	if strings.Join(got, ",") != "/data/wal,/data/mpc" {
		t.Fatalf("got %v", got)
	}
	for _, p := range []string{
		"/data/../etc/shadow", // fuehrt hinaus
		"/database/x.wal",     // nur gleicher Anfang
		"/data/x.wal",         // /data selbst
		"/x.wal",              // Wurzel
		"/etc/aequitas.wal",
		"aequitas_transfers.wal", // relativ: /app, im Image uebergeben
		"",
	} {
		if got := datenVerzeichnisse(umgebung(map[string]string{"AEQUITAS_WAL_PATH": p})); len(got) != 0 {
			t.Errorf("%q: %v uebergeben", p, got)
		}
	}
	doppelt := datenVerzeichnisse(umgebung(map[string]string{
		"AEQUITAS_WAL_PATH": "/data/wal/a.wal", "MPC_TRIPLE_FILE": "/data/wal/t.bin"}))
	if len(doppelt) != 1 {
		t.Fatalf("doppelt: %v", doppelt)
	}
}

type rechteProtokoll struct {
	euid              int
	chown, set, probe []string
}

func fakeUmgebung(p *rechteProtokoll, env map[string]string, chownErr, setErr, probeErr error, euidNach int) rechteUmgebung {
	return rechteUmgebung{
		euid:   func() int { return p.euid },
		getenv: umgebung(env),
		chownAll: func(dir string, uid, gid int) error {
			p.chown = append(p.chown, dir+":"+strconv.Itoa(uid))
			return chownErr
		},
		setIDs: func(uid, gid int) error {
			p.set = append(p.set, strconv.Itoa(uid)+":"+strconv.Itoa(gid))
			if setErr == nil {
				p.euid = euidNach
			}
			return setErr
		},
		pruefe: func(dir string) error { p.probe = append(p.probe, dir); return probeErr },
	}
}

func TestRechte_AblaufUndFailClosed(t *testing.T) {
	// Datenverzeichnis, das existiert -- unter /data geht im Test nicht,
	// also ohne Verzeichnisse: der Ablauf um setIDs herum.
	env := map[string]string{}
	fehler := errors.New("verweigert")

	p := &rechteProtokoll{euid: 1000}
	if err := rechteAblegen(fakeUmgebung(p, env, nil, nil, nil, 1000)); err != nil || len(p.set) != 0 {
		t.Fatalf("ohne root muss nichts geschehen: %v %v", err, p.set)
	}

	p = &rechteProtokoll{euid: 0}
	if err := rechteAblegen(fakeUmgebung(p, env, nil, nil, nil, knotenUID)); err != nil || len(p.set) != 1 || p.set[0] != "10001:10001" {
		t.Fatalf("root: %v %v", err, p.set)
	}

	p = &rechteProtokoll{euid: 0}
	if err := rechteAblegen(fakeUmgebung(p, env, nil, fehler, nil, knotenUID)); err == nil {
		t.Fatal("gescheitertes Ablegen nicht gemeldet")
	}

	// Setuid "gelingt", euid bleibt aber 0: nicht weiterlaufen.
	p = &rechteProtokoll{euid: 0}
	if err := rechteAblegen(fakeUmgebung(p, env, nil, nil, nil, 0)); err == nil {
		t.Fatal("euid 0 nach dem Ablegen nicht gemeldet")
	}

	// Notschalter: nur genau "ja".
	p = &rechteProtokoll{euid: 0}
	if err := rechteAblegen(fakeUmgebung(p, map[string]string{"AEQUITAS_ALS_ROOT": "ja"}, nil, fehler, nil, 0)); err != nil || len(p.set) != 0 {
		t.Fatalf("Notschalter: %v %v", err, p.set)
	}
	p = &rechteProtokoll{euid: 0}
	if err := rechteAblegen(fakeUmgebung(p, map[string]string{"AEQUITAS_ALS_ROOT": "true"}, nil, nil, nil, knotenUID)); err != nil || len(p.set) != 1 {
		t.Fatalf("\"true\" ist nicht \"ja\": %v %v", err, p.set)
	}
}

// Uebergabe: eine symbolische Verknuepfung wird selbst uebergeben, nie ihr
// Ziel; zu viele Eintraege brechen ab.
func TestRechte_UebergabeFolgtKeinemLink(t *testing.T) {
	daten := t.TempDir()
	fremd := filepath.Join(t.TempDir(), "fremd")
	if err := os.WriteFile(fremd, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(daten, "a.wal"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(fremd, filepath.Join(daten, "link")); err != nil {
		t.Fatal(err)
	}
	uid, gid := os.Getuid(), os.Getgid()
	if os.Geteuid() == 0 {
		uid, gid = 54321, 54321
	}
	if err := uebergeben(daten, uid, gid); err != nil {
		t.Fatalf("Uebergabe: %v", err)
	}
	besitzer := func(p string) int {
		var st syscall.Stat_t
		if err := syscall.Lstat(p, &st); err != nil {
			t.Fatal(err)
		}
		return int(st.Uid)
	}
	if besitzer(filepath.Join(daten, "a.wal")) != uid || besitzer(filepath.Join(daten, "link")) != uid {
		t.Fatal("Datenverzeichnis nicht uebergeben")
	}
	if os.Geteuid() == 0 && besitzer(fremd) == uid {
		t.Fatal("Ziel der Verknuepfung uebergeben")
	}

	alt := rechteHoechstensEintraege
	rechteHoechstensEintraege = 2
	t.Cleanup(func() { rechteHoechstensEintraege = alt })
	if err := uebergeben(daten, uid, gid); err == nil {
		t.Fatal("zu viele Eintraege nicht gemeldet")
	}
}

// Das echte Ablegen, in einem eigenen Prozess (es ist nicht umkehrbar). Nur
// als root zu zeigen; ohne root prueft der Kindprozess, dass nichts geschieht.
func TestRechte_EchtesAblegenImKindprozess(t *testing.T) {
	if os.Getenv("RECHTE_KIND") == "1" {
		err := rechteAblegen(echteRechteUmgebung())
		os.Stdout.WriteString("euid=" + strconv.Itoa(os.Geteuid()) + " uid=" + strconv.Itoa(os.Getuid()) +
			" gid=" + strconv.Itoa(os.Getgid()) + " err=" + strconv.FormatBool(err != nil) + "\n")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRechte_EchtesAblegenImKindprozess$")
	cmd.Env = append(os.Environ(), "RECHTE_KIND=1", "AEQUITAS_ALS_ROOT=", "AEQUITAS_WAL_PATH=", "MPC_TRIPLE_FILE=")
	aus, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Kindprozess: %v\n%s", err, aus)
	}
	soll := "euid=" + strconv.Itoa(os.Geteuid()) + " uid=" + strconv.Itoa(os.Getuid()) + " gid=" + strconv.Itoa(os.Getgid()) + " err=false"
	if os.Geteuid() == 0 {
		soll = "euid=10001 uid=10001 gid=10001 err=false"
	}
	if !strings.Contains(string(aus), soll) {
		t.Fatalf("Kindprozess meldet\n%s\nerwartet %q", aus, soll)
	}
}
