package keeper

// DIE NEUESTE APP-VERSION, FUER DIE "JETZT AKTUALISIEREN"-MELDUNG IN DER APP.
//
// Die App fragt ihren eigenen Knoten, nicht GitHub: so erfaehrt kein Dritter,
// wer die App wann oeffnet. Der Knoten fragt GitHub hoechstens alle
// appVersionAbstand (eine feste Adresse, Zeitgrenze, Antwort hoechstens
// 64 KiB) und gibt nur eine Versionsnummer in strenger Form weiter. Herunter-
// geladen wird ueber /download/app.apk wie bisher -- die App oeffnet nur diese
// Seite, sie installiert nichts von selbst.
//
// GET /api/app-version -> {"version": "1.16.0", "download": "/download/app.apk"}
// Ohne bekannte Version: {"version": ""} -- die App zeigt dann nichts.

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	appVersionQuelle  = "https://api.github.com/repos/hanoi96international-gif/Aequitas-App/releases/latest"
	// Zweiter Weg, wenn die API nicht antwortet (ohne Anmeldung 60 Anfragen je
	// Stunde und Adresse): github.com leitet .../releases/latest auf
	// .../releases/tag/<tag> weiter. Gelesen wird nur das Ziel der
	// Weiterleitung, ihr gefolgt wird nicht.
	appVersionSeite     = "https://github.com/hanoi96international-gif/Aequitas-App/releases/latest"
	appVersionSeiteZiel = "https://github.com/hanoi96international-gif/Aequitas-App/releases/tag/"
	appVersionAbstand = 30 * time.Minute
	// Nach einem Fehlschlag (GitHub antwortet nicht, Ratenlimit) frueher neu
	// fragen: sonst meldet ein frisch gestarteter Knoten 30 Minuten lang gar
	// keine Version, und niemand sieht die Update-Meldung. Weiterhin hoechstens
	// eine Anfrage zur Zeit.
	appVersionWiederholung = 2 * time.Minute
)

var appVersionForm = regexp.MustCompile(`^v?([0-9]{1,4}\.[0-9]{1,4}\.[0-9]{1,4})$`)

var appVersionStand struct {
	mu        sync.Mutex
	version   string
	naechster time.Time // frueheste naechste Anfrage bei GitHub
	laeuft    bool
}

// appVersionAusAntwort: die Versionsnummer aus der Antwort von GitHub, streng.
func appVersionAusAntwort(roh []byte) string {
	var j struct {
		TagName string `json:"tag_name"`
	}
	if json.Unmarshal(roh, &j) != nil {
		return ""
	}
	m := appVersionForm.FindStringSubmatch(strings.TrimSpace(j.TagName))
	if m == nil {
		return ""
	}
	return m[1]
}

// appVersionAusWeiterleitung: die Versionsnummer aus dem Ziel der
// Weiterleitung von appVersionSeite -- nur genau appVersionSeiteZiel plus ein
// Tag in strenger Form, sonst "".
func appVersionAusWeiterleitung(ziel string) string {
	if !strings.HasPrefix(ziel, appVersionSeiteZiel) {
		return ""
	}
	m := appVersionForm.FindStringSubmatch(strings.TrimPrefix(ziel, appVersionSeiteZiel))
	if m == nil {
		return ""
	}
	return m[1]
}

func appVersionAnfrage(url string) (*http.Response, error) {
	k := &http.Client{
		Timeout: 10 * time.Second,
		// Keiner Weiterleitung folgen: die API leitet nicht weiter, und bei
		// der Seite wird nur das Ziel gelesen.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "aequitas-knoten")
	return k.Do(req)
}

func appVersionHolen() {
	v := ""
	if resp, err := appVersionAnfrage(appVersionQuelle); err == nil {
		if resp.StatusCode == http.StatusOK {
			roh, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
			v = appVersionAusAntwort(roh)
		}
		resp.Body.Close()
	}
	if v == "" {
		if resp, err := appVersionAnfrage(appVersionSeite); err == nil {
			if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
				v = appVersionAusWeiterleitung(resp.Header.Get("Location"))
			}
			resp.Body.Close()
		}
	}
	appVersionErgebnis(v, time.Now())
}

// appVersionErgebnis traegt das Ergebnis einer Anfrage ein: eine gueltige
// Version ersetzt den Stand, naechste Anfrage nach appVersionAbstand; ein
// Fehlschlag laesst den letzten Stand, naechste Anfrage nach
// appVersionWiederholung.
func appVersionErgebnis(v string, jetzt time.Time) {
	appVersionStand.mu.Lock()
	defer appVersionStand.mu.Unlock()
	appVersionStand.laeuft = false
	if v != "" {
		appVersionStand.version = v
		appVersionStand.naechster = jetzt.Add(appVersionAbstand)
		return
	}
	appVersionStand.naechster = jetzt.Add(appVersionWiederholung)
}

// appVersionFaellig: ob jetzt bei GitHub gefragt wird; markiert die Anfrage
// als laufend (hoechstens eine zur Zeit). Gibt auch den aktuellen Stand.
func appVersionFaellig(jetzt time.Time) (version string, fragen bool) {
	appVersionStand.mu.Lock()
	defer appVersionStand.mu.Unlock()
	if !appVersionStand.laeuft && !jetzt.Before(appVersionStand.naechster) {
		appVersionStand.laeuft = true
		fragen = true
	}
	return appVersionStand.version, fragen
}

func (a *APIServer) handleAppVersion(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodGet {
		jsonError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	v, fragen := appVersionFaellig(time.Now())
	if fragen {
		go appVersionHolen()
	}
	if v == "" {
		// Noch keine Version: nicht 5 Minuten lang "nichts" zwischenspeichern.
		w.Header().Set("Cache-Control", "public, max-age=30")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	json.NewEncoder(w).Encode(map[string]string{"version": v, "download": "/download/app.apk"})
}
