package keeper

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Ablage fuer bereits gepruefte Validator-Bindungen (Laien-Einrichtung).
//
// WARUM: Ein neuer Knoten meldet sich beim Primary nur mit
// NODE_OPERATOR_BINDING_SIGNATURE an (handlePeerRegister). Die Signatur macht
// die Wallet des Menschen, und die liegt in der App, nicht auf dem Server.
// Bisher hiess das: MetaMask im Browser, /node-binding, Signatur kopieren,
// in .env einfuegen -- fuer jemanden ohne Krypto-Erfahrung kaum machbar.
//
// Jetzt zeigt deploy/validator/einrichten.sh einen QR-Code, die App
// unterschreibt und reicht die Bindung ueber /api/register-validator-key ein.
// Dieser Knoten prueft sie dort vollstaendig (Menschen-Signatur UND Beweis des
// Signierschluessels) und legt sie erst DANN hier ab. Das Skript holt sie mit
// GET /api/validator-binding ab und traegt sie in .env ein.
//
// Nichts hier ist geheim: die Signatur sagt nur, dass diese Wallet genau
// diese Signieradresse ermaechtigt -- dieselbe Aussage, die jeder Knoten beim
// Anmelden ohnehin vorzeigt. Begrenzt ist die Ablage trotzdem, weil sie von
// aussen gefuellt werden kann:
//   - nur gepruefte Bindungen (siehe oben), also nur registrierte Menschen,
//   - hoechstens EIN Eintrag je Wallet (eine neue Bindung ersetzt die alte),
//   - hoechstens bindungsAblageMax Eintraege, der aelteste weicht,
//   - jeder Eintrag verfaellt nach bindungsAblageDauer.
// Nichts davon wird gespeichert oder an andere Knoten weitergegeben; ein
// Neustart leert die Ablage, das Skript wartet dann einfach auf die naechste
// Einreichung.

const (
	bindungsAblageMax   = 256
	bindungsAblageDauer = 24 * time.Hour
)

type bindungsEintrag struct {
	Wallet   string
	Signatur string
	Zeit     time.Time
}

type bindungsAblageTyp struct {
	mu         sync.Mutex
	nachAdr    map[string]bindungsEintrag // Signieradresse -> Eintrag
	nachWallet map[string]string          // Wallet -> Signieradresse
}

func neueBindungsAblage() *bindungsAblageTyp {
	return &bindungsAblageTyp{nachAdr: map[string]bindungsEintrag{}, nachWallet: map[string]string{}}
}

var bindungsAblage = neueBindungsAblage()

// entferneLocked nimmt einen Eintrag samt Wallet-Verweis heraus.
func (b *bindungsAblageTyp) entferneLocked(adr string) {
	if e, ok := b.nachAdr[adr]; ok {
		if b.nachWallet[e.Wallet] == adr {
			delete(b.nachWallet, e.Wallet)
		}
		delete(b.nachAdr, adr)
	}
}

// merke legt eine GEPRUEFTE Bindung ab. Aufrufer muessen die Signatur vorher
// verifiziert haben (handleRegisterValidatorKey).
func (b *bindungsAblageTyp) merke(adr, wallet, signatur string, jetzt time.Time) {
	adr = strings.ToLower(strings.TrimSpace(adr))
	wallet = strings.ToLower(strings.TrimSpace(wallet))
	if adr == "" || wallet == "" || signatur == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for a, e := range b.nachAdr {
		if jetzt.Sub(e.Zeit) > bindungsAblageDauer {
			b.entferneLocked(a)
		}
	}
	// Eine Wallet, ein Eintrag: die neue Bindung ersetzt die alte.
	if alt, ok := b.nachWallet[wallet]; ok {
		b.entferneLocked(alt)
	}
	b.entferneLocked(adr)
	for len(b.nachAdr) >= bindungsAblageMax {
		aeltester, t := "", jetzt
		for a, e := range b.nachAdr {
			if aeltester == "" || e.Zeit.Before(t) {
				aeltester, t = a, e.Zeit
			}
		}
		b.entferneLocked(aeltester)
	}
	b.nachAdr[adr] = bindungsEintrag{Wallet: wallet, Signatur: signatur, Zeit: jetzt}
	b.nachWallet[wallet] = adr
}

func (b *bindungsAblageTyp) hole(adr string, jetzt time.Time) (bindungsEintrag, bool) {
	adr = strings.ToLower(strings.TrimSpace(adr))
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.nachAdr[adr]
	if !ok {
		return bindungsEintrag{}, false
	}
	if jetzt.Sub(e.Zeit) > bindungsAblageDauer {
		b.entferneLocked(adr)
		return bindungsEintrag{}, false
	}
	return e, true
}

// handleValidatorBinding: GET /api/validator-binding?signing_address=0x...
// Liefert die hier gepruefte Bindung, damit einrichten.sh sie in .env
// eintragen kann. 404, solange noch keine eingereicht wurde.
func (a *APIServer) handleValidatorBinding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		jsonError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	adr := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("signing_address")))
	if !istHexAdresse(adr) {
		jsonError(w, "signing_address must be a 0x address", http.StatusBadRequest)
		return
	}
	e, ok := bindungsAblage.hole(adr, time.Now())
	if !ok {
		jsonError(w, "no binding for this signing address yet", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{
		"signing_address": adr,
		"human_wallet":    e.Wallet,
		"human_signature": e.Signatur,
	})
}

func istHexAdresse(s string) bool {
	if len(s) != 42 || !strings.HasPrefix(s, "0x") {
		return false
	}
	return strings.Trim(s[2:], "0123456789abcdef") == ""
}
