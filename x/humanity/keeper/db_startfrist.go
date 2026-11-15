package keeper

import (
	"os"
	"strconv"
	"time"
)

// Audit 14.11.2026, B4: Datenbank beim Start nicht erreichbar.
//
// Mit gesetzter DATABASE_URL wartet der Knoten hoechstens dbStartFrist() auf
// die Datenbank und bricht sonst ab (NewChainState). Vorher lief er mit dem
// Zustand aus einer Datei weiter -- leer oder fremd -- und erzeugte Bloecke.
//
// AEQUITAS_DB_STARTFRIST: Sekunden, 5..600, Vorgabe 120. Unlesbar: Vorgabe.
const (
	dbStartFristVorgabe = 120 * time.Second
	dbStartFristMin     = 5 * time.Second
	dbStartFristMax     = 600 * time.Second
	dbPingPauseMax      = 5 * time.Second
)

func dbStartFrist() time.Duration {
	v, err := strconv.Atoi(os.Getenv("AEQUITAS_DB_STARTFRIST"))
	if err != nil {
		return dbStartFristVorgabe
	}
	d := time.Duration(v) * time.Second
	if d < dbStartFristMin {
		return dbStartFristMin
	}
	if d > dbStartFristMax {
		return dbStartFristMax
	}
	return d
}

// dbPingMitFrist: ping, bis er gelingt oder die Frist um ist (Pausen 0,5 s,
// verdoppelt bis 5 s). Gibt den letzten Fehler zurueck.
func dbPingMitFrist(ping func() error, frist time.Duration, schlaf func(time.Duration)) error {
	pause := 500 * time.Millisecond
	var gewartet time.Duration
	for {
		err := ping()
		if err == nil {
			return nil
		}
		if gewartet+pause > frist {
			return err
		}
		schlaf(pause)
		gewartet += pause
		if pause *= 2; pause > dbPingPauseMax {
			pause = dbPingPauseMax
		}
	}
}
