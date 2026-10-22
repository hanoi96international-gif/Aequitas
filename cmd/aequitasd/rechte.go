package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// RECHTE ABLEGEN (Audit 2026-10-21, niedrig: "Container laeuft als root").
//
// Der Knoten nimmt Bloecke von Peers und Anfragen aus dem offenen Netz an.
// Eine Luecke darin haette als root im Container alles: jede Datei des Images,
// jedes eingehaengte Verzeichnis, die Faehigkeiten, die Docker einem
// root-Prozess laesst. Als eigener Benutzer nur das, was er braucht.
//
// WARUM NICHT EINFACH `USER` IM DOCKERFILE
//
// Die Datenverzeichnisse bestehender Knoten gehoeren root: auf C1/C2 der
// Bind-Mount /root/aequitas-wal-data -> /data/wal, bei Validatoren das
// benannte Volume waldata, dazu /data/mpc mit der Tripel-Datei (vom Dealer mit
// 0600 angelegt). Mit `USER` allein koennte der Knoten nach dem naechsten
// Ausrollen sein WAL nicht mehr oeffnen -- ein Ausfall auf jedem Knoten, und
// ein Umzug von Hand auf jeder Box.
//
// WIE ES STATTDESSEN GEHT
//
// Der Prozess startet weiter als root, uebergibt GENAU die Datenverzeichnisse
// unter /data, die er laut Konfiguration benutzt (AEQUITAS_WAL_PATH,
// MPC_TRIPLE_FILE), an den Benutzer des Images (knotenUID) und legt die
// root-Rechte dann ab -- bevor er irgendetwas oeffnet oder lauscht. Der Umzug
// geschieht so beim ersten Start von selbst und ist danach ein Nichts.
// Ausserhalb von /data wird nichts umgeschrieben: ein falsch gesetzter Pfad
// darf nicht /etc oder / dem Knoten uebergeben.
//
// FEHLERFALL: fail-closed. Scheitert die Uebergabe, das Ablegen oder danach
// ein Schreibversuch ins WAL-Verzeichnis, startet der Knoten nicht -- mit
// einer Meldung, die sagt, was zu tun ist. Ein Knoten, der als root
// weiterliefe, weil das Ablegen still scheiterte, waere genau der Zustand,
// der hier beendet wird.
//
// NOTSCHALTER: AEQUITAS_ALS_ROOT=ja laesst ihn als root laufen, laut im Log.
// Nur fuer den Fall, dass eine Box die Uebergabe nicht erlaubt (etwa ein
// Volume, das chown verweigert) und der Knoten trotzdem laufen muss.

const (
	knotenUID = 10001
	knotenGID = 10001
)

// Hoechstens so viele Eintraege werden je Verzeichnis uebergeben: ein
// WAL-Verzeichnis haelt eine Handvoll Dateien, ein Tripel-Verzeichnis eine.
// Mehr hiesse, der Pfad zeigt woanders hin. var: Tests senken sie.
var rechteHoechstensEintraege = 10_000

// datenVerzeichnisse: die Verzeichnisse unter /data, die der Knoten laut
// Umgebung benutzt. Das Image setzt AEQUITAS_WAL_PATH auf /data/wal; /app
// bleibt root (Dockerfile) -- ein relativer WAL-Pfad scheitert dort beim
// Oeffnen, statt dem Knoten das Verzeichnis seines Binarys zu geben.
func datenVerzeichnisse(getenv func(string) string) []string {
	var aus []string
	for _, name := range []string{"AEQUITAS_WAL_PATH", "MPC_TRIPLE_FILE"} {
		p := strings.TrimSpace(getenv(name))
		if p == "" || !filepath.IsAbs(p) {
			continue
		}
		d := filepath.Dir(filepath.Clean(p))
		if !strings.HasPrefix(d+"/", "/data/") || d == "/data" {
			continue
		}
		neu := true
		for _, a := range aus {
			if a == d {
				neu = false
			}
		}
		if neu {
			aus = append(aus, d)
		}
	}
	return aus
}

// uebergeben: Verzeichnis und alles darin an uid/gid. Symbolische
// Verknuepfungen werden selbst uebergeben (Lchown), nie ihr Ziel -- ein Link
// im Datenverzeichnis darf nicht /etc/shadow dem Knoten zuspielen.
func uebergeben(dir string, uid, gid int) error {
	n := 0
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		n++
		if n > rechteHoechstensEintraege {
			return fmt.Errorf("%s: mehr als %d Eintraege -- kein Datenverzeichnis des Knotens", dir, rechteHoechstensEintraege)
		}
		// Gehoert es schon uid/gid, nichts schreiben: so startet auch ein
		// schon uebergebenes, schreibgeschuetzt eingehaengtes Verzeichnis.
		var st syscall.Stat_t
		if syscall.Lstat(p, &st) == nil && int(st.Uid) == uid && int(st.Gid) == gid {
			return nil
		}
		return os.Lchown(p, uid, gid)
	})
}

// schreibbar: kann dieser Prozess in dir eine Datei anlegen und loeschen?
func schreibbar(dir string) error {
	f, err := os.CreateTemp(dir, ".rechte-probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

// rechteAblegen: siehe oben. euid, Systemaufrufe und Umgebung kommen von
// aussen, damit der Ablauf ohne root testbar ist.
type rechteUmgebung struct {
	euid     func() int
	getenv   func(string) string
	chownAll func(dir string, uid, gid int) error
	setIDs   func(uid, gid int) error
	pruefe   func(dir string) error
}

func echteRechteUmgebung() rechteUmgebung {
	return rechteUmgebung{
		euid:     os.Geteuid,
		getenv:   os.Getenv,
		chownAll: uebergeben,
		setIDs: func(uid, gid int) error {
			// Reihenfolge: erst Gruppen, dann gid, dann uid -- nach Setuid
			// waere das Aendern der Gruppen nicht mehr erlaubt. Seit Go 1.16
			// gelten die Aufrufe fuer alle Threads des Prozesses.
			if err := syscall.Setgroups([]int{}); err != nil {
				return fmt.Errorf("setgroups: %w", err)
			}
			if err := syscall.Setgid(gid); err != nil {
				return fmt.Errorf("setgid: %w", err)
			}
			if err := syscall.Setuid(uid); err != nil {
				return fmt.Errorf("setuid: %w", err)
			}
			return nil
		},
		pruefe: schreibbar,
	}
}

func rechteAblegen(u rechteUmgebung) error {
	if u.euid() != 0 {
		return nil // schon ohne root (etwa `USER` gesetzt oder ausserhalb von Docker)
	}
	if strings.TrimSpace(u.getenv("AEQUITAS_ALS_ROOT")) == "ja" {
		fmt.Println("⚠⚠⚠ AEQUITAS_ALS_ROOT=ja: der Knoten laeuft als root. Nur als Notbehelf -- die Datenverzeichnisse dem Benutzer 10001 uebergeben und den Schalter entfernen.")
		return nil
	}
	dirs := datenVerzeichnisse(u.getenv)
	for _, d := range dirs {
		if _, err := os.Stat(d); err != nil {
			continue // nicht eingehaengt: der Knoten meldet das spaeter selbst
		}
		if err := u.chownAll(d, knotenUID, knotenGID); err != nil {
			return fmt.Errorf("%s nicht an den Knotenbenutzer %d uebergeben: %w", d, knotenUID, err)
		}
	}
	if err := u.setIDs(knotenUID, knotenGID); err != nil {
		return fmt.Errorf("root-Rechte nicht abgelegt: %w", err)
	}
	if u.euid() == 0 {
		return fmt.Errorf("root-Rechte nicht abgelegt: euid ist weiter 0")
	}
	for _, d := range dirs {
		if _, err := os.Stat(d); err != nil {
			continue
		}
		if err := u.pruefe(d); err != nil {
			return fmt.Errorf("%s ist fuer den Knotenbenutzer nicht schreibbar: %w", d, err)
		}
	}
	return nil
}
