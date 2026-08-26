package keeper

import (
	"fmt"
	"strings"
	"time"
)

// Sybil-Report: rueckwirkende Erkennung statt praeventiver Verhinderung.
//
// Hintergrund (siehe BIOMETRIE_ANALYSE*.md): auf Standardhardware laesst sich
// Einmaligkeit nicht beweisen. Ein Fuzzy Extractor authentifiziert 1:1, er
// dedupliziert nicht global, und keine Fernverifikation kann verhindern, dass
// ein echter Mensch die Pruefung fuer einen Betreiber durchfuehrt. Damit bleibt
// als wirksame Verteidigungslinie die nachtraegliche Erkennung: Farmen sehen
// statistisch anders aus als echte Nutzer, und die Chain hat die Daten dafuer
// bereits.
//
// Dieses Modul ist bewusst AUSSCHLIESSLICH lesend. Es faellt keine
// Entscheidung, sperrt niemanden und schreibt keinen Zustand — es liefert
// Signale. Der Grund ist nicht Vorsicht um ihrer selbst willen: jede
// automatische Sanktion waere ein konsensrelevanter Zustandsuebergang, und ein
// Knoten, der anders sanktioniert als seine Nachbarn, forkt das Netz (vgl.
// ANALYSE_STATEROOT_DIVERGENZ.md). Bewertung und Konsequenz gehoeren deshalb
// aus dem Konsenspfad heraus.
//
// Wichtige Einschraenkung, die bei der Interpretation mitgelesen werden muss:
// Kein einzelnes Signal hier beweist einen Angriff. Eine Registrierungsspitze
// kann eine Kampagne sein, eine unberuehrte Praemie kann schlicht ein Nutzer
// sein, der wartet. Die Signale sind Anhaltspunkte fuer eine Untersuchung,
// keine Urteile — und sie sollten nie ohne menschliche Pruefung zu einer
// Massnahme fuehren.

// SybilSeverity stuft ein Signal ein. Bewusst grob gehalten: eine feinere
// Skala wuerde eine Praezision suggerieren, die die Heuristiken nicht haben.
type SybilSeverity string

const (
	SybilInfo  SybilSeverity = "info"
	SybilWarn  SybilSeverity = "warn"
	SybilAlert SybilSeverity = "alert"
)

// registrationGrantAEQ ist die Registrierungspraemie. Bewusst hier dupliziert
// statt aus AequitasV7.sol importiert (Solidity-Konstante, in Go nicht
// erreichbar) — bei einer Aenderung der Praemie muss dieser Wert mitgezogen
// werden, sonst meldet untouchedGrants nichts mehr.
const registrationGrantAEQ = 1000.0

// grantTouchGraceSeconds ist die Kulanz zwischen Registrierungszeitpunkt und
// letzter Aktivitaet, innerhalb derer ein Konto noch als "hat die Praemie nie
// angefasst" gilt. Die Registrierung selbst stempelt LastActivityAt, deshalb
// darf der Vergleich nicht exakt sein.
const grantTouchGraceSeconds = 300

// maxListedWallets begrenzt, wie viele Adressen ein einzelnes Signal
// namentlich auffuehrt. Ohne Deckel wuerde ein Report bei einer grossen Farm
// selbst zum Speicherproblem.
const maxListedWallets = 50

// SybilSignal ist ein einzelner Befund.
type SybilSignal struct {
	Kind        string        `json:"kind"`
	Severity    SybilSeverity `json:"severity"`
	Description string        `json:"description"`
	Count       int           `json:"count"`
	// Wallets ist auf maxListedWallets gekuerzt; Count nennt die volle Zahl.
	Wallets []string `json:"wallets,omitempty"`
}

// SybilReport buendelt alle Signale eines Laufs.
type SybilReport struct {
	GeneratedAt time.Time     `json:"generated_at"`
	WindowHours int           `json:"window_hours"`
	TotalHumans int           `json:"total_humans"`
	Signals     []SybilSignal `json:"signals"`
	// Errors sammelt Teilfehler. Ein Report mit einem fehlgeschlagenen
	// Einzelcheck ist mehr wert als gar keiner, deshalb bricht BuildSybilReport
	// nicht ab, sondern liefert, was es hat, und benennt den Rest hier.
	Errors []string `json:"errors,omitempty"`
}

// BuildSybilReport erhebt alle Signale ueber das angegebene Zeitfenster.
// windowHours <= 0 wird auf 24 gesetzt.
//
// Der Report ist rein lesend und haelt keine Sperren auf cs.mu laenger als
// noetig — die Einzelchecks arbeiten direkt gegen die DB, nicht gegen den
// In-Memory-Zustand, damit ein laufender Report den Blockpfad nicht bremst.
func (cs *ChainState) BuildSybilReport(windowHours int) SybilReport {
	if windowHours <= 0 {
		windowHours = 24
	}
	rep := SybilReport{
		GeneratedAt: time.Now().UTC(),
		WindowHours: windowHours,
		TotalHumans: cs.TotalHumans(),
	}
	if cs.db == nil {
		rep.Errors = append(rep.Errors, "no database configured — report unavailable")
		return rep
	}

	for _, check := range []struct {
		name string
		run  func(int) (*SybilSignal, error)
	}{
		{"registration_burst", cs.checkRegistrationBurst},
		{"untouched_grant", cs.checkUntouchedGrants},
		{"missing_biohash_link", cs.checkMissingBioHashLink},
		{"dormant_since_registration", cs.checkDormantSinceRegistration},
	} {
		sig, err := check.run(windowHours)
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %v", check.name, err))
			continue
		}
		if sig != nil {
			rep.Signals = append(rep.Signals, *sig)
		}
	}
	return rep
}

// burstThreshold liefert die Mindestzahl an Registrierungen pro Stunde, ab der
// ein Stundenfenster als Spitze gilt: 10 % der bestehenden Population, aber nie
// unter 5.
//
// Relativ statt absolut, damit die Schwelle mit dem Netz mitwaechst und nicht
// bei jedem Wachstumsschub falsch anschlaegt. Der Boden von 5 verhindert das
// Gegenteil — bei kleiner Population waeren 10 % sonst 0 oder 1 und jede
// normale Registrierung ein Alarm.
//
// Als eigene Funktion herausgezogen, weil das die Groesse ist, die im Betrieb
// nachjustiert werden muss, und weil sie sonst nur ueber eine Datenbank
// pruefbar waere.
func burstThreshold(humans int) int {
	t := humans / 10
	if t < 5 {
		return 5
	}
	return t
}

// burstSeverity stuft eine gemessene Spitze ein. Ab dem Dreifachen der
// Schwelle gilt sie als Alarm statt als Warnung — ein Vielfaches der
// erwarteten Rate ist mit organischem Zustrom kaum noch erklaerbar.
func burstSeverity(total, threshold int) SybilSeverity {
	if threshold > 0 && total >= threshold*3 {
		return SybilAlert
	}
	return SybilWarn
}

// dormantSeverity stuft den Anteil dauerhaft inaktiver Menschen ein.
//
// Unterhalb von 20 Konten bleibt es bewusst bei "info": bei kleiner Population
// ist die Quote statistisch bedeutungslos, und ein Alarm bei 3 von 4 Konten
// waere reines Rauschen. Die Prozentschwellen sind Erfahrungswerte, keine
// gemessenen Groessen — sie gehoeren nachjustiert, sobald echte Nutzungsdaten
// vorliegen.
func dormantSeverity(dormant, humans int) SybilSeverity {
	if humans < 20 {
		return SybilInfo
	}
	share := float64(dormant) / float64(humans) * 100
	switch {
	case share >= 80:
		return SybilAlert
	case share >= 50:
		return SybilWarn
	default:
		return SybilInfo
	}
}

// checkRegistrationBurst sucht Stunden, in denen auffaellig viele
// Registrierungen stattfanden. Echte Nutzerzustroeme sind ueber den Tag
// verteilt; automatisierte Registrierung erzeugt Spitzen.
func (cs *ChainState) checkRegistrationBurst(windowHours int) (*SybilSignal, error) {
	threshold := burstThreshold(cs.TotalHumans())

	rows, err := cs.db.Query(`
		SELECT date_trunc('hour', registered_at) AS bucket, COUNT(*) AS n
		FROM nullifiers
		WHERE registered_at >= NOW() - make_interval(hours => $1)
		GROUP BY bucket
		HAVING COUNT(*) >= $2
		ORDER BY n DESC`, windowHours, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var buckets []string
	total := 0
	for rows.Next() {
		var bucket time.Time
		var n int
		if err := rows.Scan(&bucket, &n); err != nil {
			return nil, err
		}
		total += n
		if len(buckets) < maxListedWallets {
			buckets = append(buckets, fmt.Sprintf("%s: %d", bucket.UTC().Format(time.RFC3339), n))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}

	return &SybilSignal{
		Kind:     "registration_burst",
		Severity: burstSeverity(total, threshold),
		Description: fmt.Sprintf(
			"%d Registrierungen in Stundenfenstern mit je >= %d — automatisierte Registrierung erzeugt solche Spitzen, echter Zustrom verteilt sich",
			total, threshold),
		Count:   total,
		Wallets: buckets,
	}, nil
}

// checkUntouchedGrants findet registrierte Menschen, die ihre Praemie nie
// angefasst haben. Fuer sich genommen harmlos — in Verbindung mit einer
// Registrierungsspitze ist es das Muster einer Farm, die auf Auszahlung
// wartet.
func (cs *ChainState) checkUntouchedGrants(windowHours int) (*SybilSignal, error) {
	rows, err := cs.db.Query(`
		SELECT a.address
		FROM chain_accounts a
		JOIN nullifiers n ON lower(n.wallet_address) = lower(a.address)
		WHERE a.is_human = true
		  AND a.balance >= $1
		  AND a.last_activity_at <= EXTRACT(EPOCH FROM n.registered_at)::bigint + $2
		  AND n.registered_at >= NOW() - make_interval(hours => $3)`,
		registrationGrantAEQ, grantTouchGraceSeconds, windowHours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wallets []string
	total := 0
	for rows.Next() {
		var addr string
		if err := rows.Scan(&addr); err != nil {
			return nil, err
		}
		total++
		if len(wallets) < maxListedWallets {
			wallets = append(wallets, addr)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}

	return &SybilSignal{
		Kind:     "untouched_grant",
		Severity: SybilInfo,
		Description: fmt.Sprintf(
			"%d im Fenster registrierte Konten haben ihre Praemie nie bewegt — allein unauffaellig, zusammen mit registration_burst ein Farm-Muster",
			total),
		Count:   total,
		Wallets: wallets,
	}, nil
}

// checkMissingBioHashLink findet Nullifier ohne zugehoerigen bio_hash-Eintrag.
//
// Das ist kein Farm-Signal, sondern ein Integritaetssignal: die bio_hashes-
// Absicherung ist laut register.go eine echte zweite Verteidigungslinie gegen
// Doppelregistrierung. AUDIT_2026-07-12 hat sie in Produktion mit
// chain_bio_hashes = 0 bei 6 registrierten Menschen vorgefunden — also fuer
// niemanden wirksam. Dieser Check macht daraus eine laufende Beobachtung statt
// eines Einmalbefunds, damit ein erneutes Wegbrechen sofort auffaellt.
func (cs *ChainState) checkMissingBioHashLink(windowHours int) (*SybilSignal, error) {
	rows, err := cs.db.Query(`
		SELECT n.wallet_address
		FROM nullifiers n
		WHERE n.registered_at >= NOW() - make_interval(hours => $1)
		  AND NOT EXISTS (
			SELECT 1 FROM bio_registrations b
			WHERE lower(b.wallet_address) = lower(n.wallet_address)
			  AND b.bio_hash IS NOT NULL
			  AND b.bio_hash <> ''
		  )`, windowHours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wallets []string
	total := 0
	for rows.Next() {
		var addr string
		if err := rows.Scan(&addr); err != nil {
			return nil, err
		}
		total++
		if len(wallets) < maxListedWallets {
			wallets = append(wallets, addr)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}

	return &SybilSignal{
		Kind:     "missing_biohash_link",
		Severity: SybilAlert,
		Description: fmt.Sprintf(
			"%d Registrierungen ohne bio_hash-Eintrag — die zweite Verteidigungslinie gegen Doppelregistrierung greift fuer diese Konten nicht (vgl. AUDIT_2026-07-12)",
			total),
		Count:   total,
		Wallets: wallets,
	}, nil
}

// checkDormantSinceRegistration zaehlt Menschen, die seit ihrer Registrierung
// nie aktiv waren, unabhaengig vom Zeitfenster. Liefert den Anteil an der
// Gesamtpopulation — eine hohe Quote ist der deutlichste Hinweis darauf, dass
// registriert wird, um zu kassieren, nicht um zu benutzen.
func (cs *ChainState) checkDormantSinceRegistration(_ int) (*SybilSignal, error) {
	var dormant int
	err := cs.db.QueryRow(`
		SELECT COUNT(*)
		FROM chain_accounts a
		JOIN nullifiers n ON lower(n.wallet_address) = lower(a.address)
		WHERE a.is_human = true
		  AND a.last_activity_at <= EXTRACT(EPOCH FROM n.registered_at)::bigint + $1`,
		grantTouchGraceSeconds).Scan(&dormant)
	if err != nil {
		return nil, err
	}
	if dormant == 0 {
		return nil, nil
	}

	humans := cs.TotalHumans()
	share := 0.0
	if humans > 0 {
		share = float64(dormant) / float64(humans) * 100
	}

	return &SybilSignal{
		Kind:     "dormant_since_registration",
		Severity: dormantSeverity(dormant, humans),
		Description: fmt.Sprintf(
			"%d von %d Menschen (%.1f %%) waren seit ihrer Registrierung nie aktiv",
			dormant, humans, share),
		Count: dormant,
	}, nil
}

// String rendert den Report als menschenlesbaren Mehrzeiler fuer Logs und CLI.
func (r SybilReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Sybil-Report %s (Fenster %dh, %d Menschen)\n",
		r.GeneratedAt.Format(time.RFC3339), r.WindowHours, r.TotalHumans)
	if len(r.Signals) == 0 {
		b.WriteString("  keine Signale\n")
	}
	for _, s := range r.Signals {
		fmt.Fprintf(&b, "  [%s] %s: %s\n", strings.ToUpper(string(s.Severity)), s.Kind, s.Description)
	}
	for _, e := range r.Errors {
		fmt.Fprintf(&b, "  [FEHLER] %s\n", e)
	}
	return b.String()
}
