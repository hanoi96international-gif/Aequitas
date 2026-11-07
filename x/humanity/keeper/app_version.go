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
	appVersionAbstand = 30 * time.Minute
)

var appVersionForm = regexp.MustCompile(`^v?([0-9]{1,4}\.[0-9]{1,4}\.[0-9]{1,4})$`)

var appVersionStand struct {
	mu      sync.Mutex
	version string
	geholt  time.Time
	laeuft  bool
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

func appVersionHolen() {
	k := &http.Client{Timeout: 10 * time.Second}
	resp, err := k.Get(appVersionQuelle)
	if err == nil {
		defer resp.Body.Close()
	}
	v := ""
	if err == nil && resp.StatusCode == http.StatusOK {
		roh, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		v = appVersionAusAntwort(roh)
	}
	appVersionStand.mu.Lock()
	defer appVersionStand.mu.Unlock()
	appVersionStand.laeuft = false
	appVersionStand.geholt = time.Now()
	if v != "" {
		appVersionStand.version = v // nur Neues ersetzt; ein Fehler laesst den letzten Stand
	}
}

func (a *APIServer) handleAppVersion(w http.ResponseWriter, r *http.Request) {
	writeJSONCORS(w)
	if r.Method != http.MethodGet {
		jsonError(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	appVersionStand.mu.Lock()
	v := appVersionStand.version
	if !appVersionStand.laeuft && time.Since(appVersionStand.geholt) > appVersionAbstand {
		appVersionStand.laeuft = true
		go appVersionHolen()
	}
	appVersionStand.mu.Unlock()
	w.Header().Set("Cache-Control", "public, max-age=300")
	json.NewEncoder(w).Encode(map[string]string{"version": v, "download": "/download/app.apk"})
}
