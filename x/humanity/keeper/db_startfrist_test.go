package keeper

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// B4: mit gesetzter DATABASE_URL und unerreichbarer Datenbank startet der
// Knoten NICHT (kein Datei-Modus mit leerem oder fremdem Zustand).
func TestDBStart_UnerreichbarBrichtAb(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x:y@127.0.0.1:1/nirgends?sslmode=disable")
	t.Setenv("AEQUITAS_DB_STARTFRIST", "5")
	// Eine "fremde" Zustandsdatei wie im Audit -- sie darf nie geladen werden.
	pfad := t.TempDir() + "/zustand.json"
	if err := os.WriteFile(pfad, []byte(`{"0x00000000000000000000000000000000000000a1":{"address":"0x00000000000000000000000000000000000000a1","balance":47}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Knoten lief mit unerreichbarer Datenbank weiter")
		}
		if !strings.Contains(r.(string), "nicht erreichbar") {
			t.Fatalf("unerwartete Meldung: %v", r)
		}
	}()
	_ = NewChainState(pfad)
}

func TestDBStart_FristUndPausen(t *testing.T) {
	var geschlafen time.Duration
	n := 0
	err := dbPingMitFrist(func() error { n++; return errors.New("weg") }, 20*time.Second, func(d time.Duration) { geschlafen += d })
	if err == nil || geschlafen > 20*time.Second || n < 5 {
		t.Fatalf("err %v, geschlafen %s, Versuche %d", err, geschlafen, n)
	}
	// Gelingt der dritte Versuch: kein Fehler.
	n = 0
	if err := dbPingMitFrist(func() error {
		n++
		if n < 3 {
			return errors.New("noch nicht")
		}
		return nil
	}, 20*time.Second, func(time.Duration) {}); err != nil || n != 3 {
		t.Fatalf("err %v nach %d Versuchen", err, n)
	}
	for _, c := range []struct {
		env  string
		soll time.Duration
	}{{"", dbStartFristVorgabe}, {"x", dbStartFristVorgabe}, {"1", dbStartFristMin}, {"99999", dbStartFristMax}, {"30", 30 * time.Second}} {
		t.Setenv("AEQUITAS_DB_STARTFRIST", c.env)
		if d := dbStartFrist(); d != c.soll {
			t.Errorf("%q -> %s, soll %s", c.env, d, c.soll)
		}
	}
}

// B6: Datei-Modus schreibt in die eigene Datei, nur fuer den Eigentuemer.
func TestDBStart_DateiModusEigeneDatei(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	pfad := t.TempDir() + "/zustand.json"
	cs := NewChainState(pfad)
	cs.save()
	fi, err := os.Stat(pfad)
	if err != nil {
		t.Fatalf("nicht in die eigene Datei geschrieben: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("Rechte %v, soll 0600", fi.Mode().Perm())
	}
}
