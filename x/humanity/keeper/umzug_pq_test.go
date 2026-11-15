package keeper

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Stufe 3.6 (umzug_pq.go): das Menschsein -- und mit ihm das Grundeinkommen
// -- auf eine PQ-Adresse. Gutfall mit und ohne Code, Code und Widerruf von
// der PQ-Adresse, und jede Faelschung: fremder Schluessel, andere Nachricht,
// andere Domaene, beide Unterschriften, Fehlform, vor dem Stichtag, an der
// falschen Art, ohne Gesicht.

func umzugPQAktivFuerTest(t *testing.T) {
	t.Helper()
	vorher := umzugPQAbOverride.Load()
	umzugPQAbOverride.Store(1)
	t.Cleanup(func() { umzugPQAbOverride.Store(vorher) })
}

// pqKonto: die ML-DSA-Unterschrift von k ueber msg als UmzugAngaben.PQ.
func pqKonto(t *testing.T, k pqKontoSchluessel, msg string) *PQKonto {
	t.Helper()
	return &PQKonto{Pub: k.pub, Sig: pqUnterschreibe(t, k, msg)}
}

// antragPQ: ein vollstaendig gueltiger Antrag (Gesicht wie bei antragMit) fuer
// die PQ-Adresse p, von p mit ML-DSA unterschrieben.
func (f *uzFall) antragPQ(p pqKontoSchluessel, alt, n string, zeit int64, code, pruefung string) Transaction {
	tx := f.antragMit(f.neuKey, p.addr, alt, n, zeit, code, pruefung)
	tx.Umzug.Sig = ""
	tx.Umzug.PQ = pqKonto(f.t, p, umzugAntragNachricht(n, alt, p.addr, zeit))
	return tx
}

func codeTxPQ(t *testing.T, p pqKontoSchluessel, code string, zeit int64) Transaction {
	h := umzugCodeHash(p.addr, umzugCodeNormal(code))
	return Transaction{Type: umzugCodeTxArt, Wallet: p.addr, DistributionAt: zeit,
		Umzug: &UmzugAngaben{CodeHash: h, PQ: pqKonto(t, p, umzugCodeSetzenNachricht(p.addr, h, zeit))}}
}

func widerrufPQ(t *testing.T, p pqKontoSchluessel, neu, n string, angekuendigt int64) Transaction {
	return Transaction{Type: umzugWiderrufTxArt, Wallet: p.addr, To: neu, Nullifier: n,
		Umzug: &UmzugAngaben{Angekuendigt: angekuendigt, PQ: pqKonto(t, p, umzugWiderrufNachricht(n, neu, angekuendigt))}}
}

// Gesicht + Code: das Menschsein ist sofort auf der PQ-Adresse; das
// Guthaben der alten Wallet bleibt, kein zweites Startgeld. Danach bekommt
// die PQ-Adresse mit ML-DSA ihren eigenen Wiederherstellungscode.
func TestUmzugPQ_MitCodeSofortAufDiePQAdresse(t *testing.T) {
	umzugPQAktivFuerTest(t)
	f := neuerUzFall(t)
	p := neuerPQKontoSchluessel(t, 0x61)
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit); len(b) != 0 {
		t.Fatalf("Code hinterlegen: %v", b)
	}
	if b := f.anwenden(f.antragPQ(p, f.alt, uzN, uzZeit+100, uzCode, umzugPruefungEinfach), uzZeit+120); len(b) != 0 {
		t.Fatalf("Antrag der PQ-Adresse: %v", b)
	}
	if f.mensch(f.alt) || !f.mensch(p.addr) || f.inhaber(uzN) != p.addr {
		t.Fatal("das Menschsein ist nicht auf der PQ-Adresse")
	}
	if f.guthaben(f.alt) != 1000 || f.guthaben(p.addr) != 0 {
		t.Fatalf("Guthaben bewegt: alt %v, PQ %v", f.guthaben(f.alt), f.guthaben(p.addr))
	}
	if acc, _ := f.cs.accounts.Get(p.addr); !acc.FaucetClaimed {
		t.Fatal("die PQ-Adresse koennte ein zweites Startgeld abholen")
	}
	// Neuer Code fuer das Menschsein auf der PQ-Adresse, mit ML-DSA.
	if b := f.anwenden(codeTxPQ(t, p, "ZZZZ-YYYY-XXXX-WWWW-VVVV-TTTT", uzZeit+200), uzZeit+210); len(b) != 0 {
		t.Fatalf("Code der PQ-Adresse: %v", b)
	}
	if _, ok := f.cs.umzugRegisterJetzt().code(p.addr); !ok {
		t.Fatal("Code der PQ-Adresse fehlt im Register")
	}
	// Und er ist nicht aenderbar -- auch nicht mit ML-DSA (Regel "ein Code
	// je Wallet", hier fuer die Testzeit gesetzt).
	vorher := umzugCodeRegelnAbOverride.Load()
	umzugCodeRegelnAbOverride.Store(1)
	t.Cleanup(func() { umzugCodeRegelnAbOverride.Store(vorher) })
	if b := f.anwenden(codeTxPQ(t, p, "QQQQ-RRRR-SSSS-TTTT-VVVV-WWWW", uzZeit+300), uzZeit+310); b["umzug_code_fest"] != 1 {
		t.Fatalf("zweiter Code: %v", b)
	}
}

// Das Grundeinkommen folgt dem Menschsein: nach dem Umzug zahlt die Runde an
// die PQ-Adresse, nicht mehr an die alte Wallet.
func TestUmzugPQ_GrundeinkommenGehtAnDiePQAdresse(t *testing.T) {
	umzugPQAktivFuerTest(t)
	f := neuerUzFall(t)
	p := neuerPQKontoSchluessel(t, 0x62)
	f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit)
	if b := f.anwenden(f.antragPQ(p, f.alt, uzN, uzZeit+100, uzCode, umzugPruefungEinfach), uzZeit+120); len(b) != 0 {
		t.Fatalf("Antrag: %v", b)
	}
	lnTopf(f.cs, 100)
	an := map[string]float64{}
	for _, tx := range lnRunde(t, f.cs, uzZeit+86400) {
		if tx.Type == "ubi_distribution" {
			an[tx.Wallet] += tx.Amount
		}
	}
	if an[p.addr] <= 0 || an[f.alt] != 0 {
		t.Fatalf("Grundeinkommen: %v (PQ-Adresse %s, alte Wallet %s)", an, kurzAdresse(p.addr), kurzAdresse(f.alt))
	}
}

// Ohne Code: 14 Tage Frist wie jeder Umzug. Und von der PQ-Adresse weg kann
// sie einen Umzug ohne Code mit ML-DSA widerrufen -- wie jede Wallet.
func TestUmzugPQ_WiderrufVonDerPQAdresse(t *testing.T) {
	umzugPQAktivFuerTest(t)
	f := neuerUzFall(t)
	p := neuerPQKontoSchluessel(t, 0x63)
	f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit)
	f.anwenden(f.antragPQ(p, f.alt, uzN, uzZeit+100, uzCode, umzugPruefungEinfach), uzZeit+120)
	if !f.mensch(p.addr) {
		t.Fatal("Vorbedingung: Mensch auf der PQ-Adresse")
	}
	// Ein Angreifer mit Deepfake beantragt den Umzug von der PQ-Adresse weg.
	dieb, diebAdr := neuerSchluessel(t)
	z := uzZeit + 1000
	if b := f.anwenden(f.antrag(dieb, diebAdr, p.addr, uzN, z, ""), z+10); len(b) != 0 {
		t.Fatalf("Antrag des Angreifers: %v", b)
	}
	// Ein Widerruf mit einem fremden ML-DSA-Schluessel zaehlt nicht.
	fremd := neuerPQKontoSchluessel(t, 0x64)
	falsch := widerrufPQ(t, fremd, diebAdr, uzN, z)
	falsch.Wallet = p.addr
	if b := f.anwenden(falsch, z+20); b["umzug_unterschrift"] != 1 {
		t.Fatalf("Widerruf mit fremdem Schluessel: %v", b)
	}
	// Ein Widerruf ueber einen anderen Zeitpunkt zaehlt nicht.
	if b := f.anwenden(widerrufPQ(t, p, diebAdr, uzN, z+1), z+20); b["umzug_nicht_offen"] != 1 {
		t.Fatalf("Widerruf fuer eine andere Ankuendigung: %v", b)
	}
	if b := f.anwenden(widerrufPQ(t, p, diebAdr, uzN, z), z+30); len(b) != 0 {
		t.Fatalf("Widerruf der PQ-Adresse: %v", b)
	}
	if _, offen := f.cs.umzugRegisterJetzt().offen(uzN); offen {
		t.Fatal("der Umzug des Angreifers ist noch offen")
	}
	f.cs.mu.Lock()
	txs, err := f.cs.umzugVollzuegeLocked(t.Context(), z+umzugFrist)
	f.cs.mu.Unlock()
	if err != nil || len(txs) != 0 || !f.mensch(p.addr) {
		t.Fatalf("nach dem Widerruf vollzogen: %v %v", txs, err)
	}
}

// Ohne Code zieht das Menschsein erst nach der Frist auf die PQ-Adresse.
func TestUmzugPQ_OhneCodeErstNachFrist(t *testing.T) {
	umzugPQAktivFuerTest(t)
	f := neuerUzFall(t)
	p := neuerPQKontoSchluessel(t, 0x65)
	z := uzZeit + 100
	if b := f.anwenden(f.antragPQ(p, f.alt, uzN, z, "", umzugPruefungStreng), z+10); len(b) != 0 {
		t.Fatalf("Antrag: %v", b)
	}
	if f.mensch(p.addr) {
		t.Fatal("ohne Code sofort umgezogen")
	}
	f.cs.mu.Lock()
	txs, err := f.cs.umzugVollzuegeLocked(t.Context(), z+umzugFrist)
	f.cs.mu.Unlock()
	if err != nil || len(txs) != 1 || !f.mensch(p.addr) || f.mensch(f.alt) {
		t.Fatalf("Vollzug: %v %v", txs, err)
	}
}

// Jede Faelschung am Antrag ist ein Befund; das Menschsein bleibt, wo es ist.
func TestUmzugPQ_Missbrauch(t *testing.T) {
	umzugPQAktivFuerTest(t)
	f := neuerUzFall(t)
	p := neuerPQKontoSchluessel(t, 0x66)
	fremd := neuerPQKontoSchluessel(t, 0x67)
	f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit)
	z := uzZeit + 100
	gut := func() Transaction { return f.antragPQ(p, f.alt, uzN, z, uzCode, umzugPruefungEinfach) }

	faelle := []struct {
		name  string
		tx    func() Transaction
		regel string
	}{
		{"fremder Schluessel fuer die PQ-Adresse", func() Transaction {
			tx := gut()
			tx.Umzug.PQ = pqKonto(t, fremd, umzugAntragNachricht(uzN, f.alt, p.addr, z))
			return tx
		}, "umzug_unterschrift"},
		{"andere Zeit unterschrieben", func() Transaction {
			tx := gut()
			tx.Umzug.PQ = pqKonto(t, p, umzugAntragNachricht(uzN, f.alt, p.addr, z+1))
			return tx
		}, "umzug_unterschrift"},
		{"anderer Mensch unterschrieben", func() Transaction {
			tx := gut()
			tx.Umzug.PQ = pqKonto(t, p, umzugAntragNachricht(uzN2, f.alt, p.addr, z))
			return tx
		}, "umzug_unterschrift"},
		{"andere alte Wallet unterschrieben", func() Transaction {
			tx := gut()
			tx.Umzug.PQ = pqKonto(t, p, umzugAntragNachricht(uzN, p.addr, p.addr, z))
			return tx
		}, "umzug_unterschrift"},
		{"Unterschrift einer PQ-Ueberweisung", func() Transaction {
			tx := gut()
			tx.Umzug.PQ = pqKonto(t, p, pqUeberweisungNachricht(p.addr, f.alt, "1", 0))
			return tx
		}, "umzug_unterschrift"},
		{"Widerruf-Satz statt Antrag", func() Transaction {
			tx := gut()
			tx.Umzug.PQ = pqKonto(t, p, umzugWiderrufNachricht(uzN, p.addr, z))
			return tx
		}, "umzug_unterschrift"},
		{"ohne Domaene", func() Transaction {
			tx := gut()
			tx.Umzug.PQ = pqKonto(t, p, strings.TrimPrefix(umzugAntragNachricht(uzN, f.alt, p.addr, z), umzugDomaeneAntrag+"|"))
			return tx
		}, "umzug_unterschrift"},
		{"Bit in der Unterschrift gekippt", func() Transaction {
			tx := gut()
			b := []byte(tx.Umzug.PQ.Sig)
			if b[10] == '0' {
				b[10] = '1'
			} else {
				b[10] = '0'
			}
			tx.Umzug.PQ.Sig = string(b)
			return tx
		}, "umzug_unterschrift"},
		{"beide Unterschriften", func() Transaction {
			tx := gut()
			tx.Umzug.Sig = personalSign(t, f.neuKey, umzugAntragNachricht(uzN, f.alt, p.addr, z))
			return tx
		}, "umzug_form"},
		{"Schluessel abgeschnitten", func() Transaction {
			tx := gut()
			tx.Umzug.PQ.Pub = tx.Umzug.PQ.Pub[:len(tx.Umzug.PQ.Pub)-2]
			return tx
		}, "umzug_form"},
		{"Unterschrift zu lang", func() Transaction {
			tx := gut()
			tx.Umzug.PQ.Sig += "00"
			return tx
		}, "umzug_form"},
		{"Grossschrift", func() Transaction {
			tx := gut()
			tx.Umzug.PQ.Pub = strings.ToUpper(tx.Umzug.PQ.Pub)
			return tx
		}, "umzug_form"},
		{"leere PQ-Angabe", func() Transaction {
			tx := gut()
			tx.Umzug.PQ = &PQKonto{}
			return tx
		}, "umzug_form"},
		// Der Kern: ohne das Gesicht zieht das Menschsein nie um -- auch ein
		// Dieb mit dem secp256k1-Schluessel der alten Wallet holt es nicht
		// auf seine PQ-Adresse.
		{"ohne Gesicht (Bindung fuer eine andere Wallet)", func() Transaction {
			tx := f.antragPQ(fremd, f.alt, uzN, z, uzCode, umzugPruefungEinfach)
			b := gut().BioBescheinigung
			tx.BioBescheinigung = b
			return tx
		}, "umzug_bescheinigung"},
	}
	for _, c := range faelle {
		if b := f.anwenden(c.tx(), z+10); b[c.regel] != 1 {
			t.Fatalf("%s: Befunde %v, erwartet %s", c.name, b, c.regel)
		}
		if !f.mensch(f.alt) || f.mensch(p.addr) || f.mensch(fremd.addr) {
			t.Fatalf("%s: das Menschsein ist umgezogen", c.name)
		}
	}
	// Der Gutfall danach geht durch -- die Faelle oben waren nur die Faelschung.
	if b := f.anwenden(gut(), z+10); len(b) != 0 || !f.mensch(p.addr) {
		t.Fatalf("Gutfall nach den Faelschungen: %v", b)
	}
}

// Vor dem Stichtag: jede PQ-Unterschrift an einem Umzug ist ein Befund, der
// Endpunkt antwortet 409, kein Block nimmt sie auf. Ein Umzug mit EIP-191
// bleibt wie bisher.
func TestUmzugPQ_VorDemStichtag(t *testing.T) {
	vorher := umzugPQAbOverride.Load()
	umzugPQAbOverride.Store(math.MaxInt64)
	t.Cleanup(func() { umzugPQAbOverride.Store(vorher) })
	f := neuerUzFall(t)
	p := neuerPQKontoSchluessel(t, 0x68)
	f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit)
	tx := f.antragPQ(p, f.alt, uzN, uzZeit+100, uzCode, umzugPruefungEinfach)
	if b := f.anwenden(tx, uzZeit+110); b["umzug_form"] != 1 || f.mensch(p.addr) {
		t.Fatalf("Antrag mit PQ vor dem Stichtag: %v", b)
	}
	if b := f.anwenden(codeTxPQ(t, p, uzCode, uzZeit+100), uzZeit+110); b["umzug_form"] != 1 {
		t.Fatalf("Code mit PQ vor dem Stichtag: %v", b)
	}
	if err := blockTauglich(&tx, uzZeit+110); err == nil {
		t.Fatal("Block vor dem Stichtag nimmt einen Umzug mit PQ auf")
	}
	if s := umzugPQStatus(); len(s) != 0 {
		t.Fatalf("Status nennt einen Stichtag, solange die Regel schlaeft: %v", s)
	}
	// Der gewoehnliche Weg bleibt.
	if b := f.anwenden(f.antragMit(f.neuKey, f.neu, f.alt, uzN, uzZeit+100, uzCode, umzugPruefungEinfach), uzZeit+110); len(b) != 0 || !f.umgezogen() {
		t.Fatalf("gewoehnlicher Umzug: %v", b)
	}
}

// Stichtag im Block: erst ab dann tauglich, das Fenster beginnt dort; an
// einer anderen Art ist die PQ-Unterschrift nie tauglich; pq_zusage nimmt sie
// nicht.
func TestUmzugPQ_BlockUndArten(t *testing.T) {
	const ab int64 = 2_000_000_000
	vorher := umzugPQAbOverride.Load()
	umzugPQAbOverride.Store(ab)
	t.Cleanup(func() { umzugPQAbOverride.Store(vorher) })
	p := neuerPQKontoSchluessel(t, 0x69)
	tx := codeTxPQ(t, p, uzCode, ab+10)
	if err := blockTauglich(&tx, ab-1); err == nil {
		t.Fatal("vor dem Stichtag tauglich")
	}
	if err := blockTauglich(&tx, ab+20); err != nil {
		t.Fatalf("nach dem Stichtag nicht tauglich: %v", err)
	}
	if fe := auftragsFenster(&tx, ab+100); fe.von < ab {
		t.Fatalf("Fenster beginnt %d vor dem Stichtag", fe.von)
	}
	for _, art := range []string{umzugVollzugTxArt, pqZusageTxArt, "transfer", "register_human"} {
		x := tx
		x.Type = art
		if err := blockTauglich(&x, ab+20); err == nil {
			t.Fatalf("PQ-Unterschrift an %s tauglich", art)
		}
		if fe := auftragsFenster(&x, ab+100); fe.von <= fe.bis {
			t.Fatalf("PQ-Unterschrift an %s hat ein Fenster", art)
		}
	}
	// pq_zusage traegt nie eine PQ-Unterschrift (auch wenn sonst alles passt).
	umzugPQAbOverride.Store(1)
	cs := newTestState()
	z := Transaction{Type: pqZusageTxArt, Wallet: p.addr, DistributionAt: pqZusageAb() + 10,
		Umzug: &UmzugAngaben{PQHash: strings.Repeat("ab", 32), Sig: "0x" + strings.Repeat("11", 65), PQ: pqKonto(t, p, "x")}}
	cs.mu.Lock()
	u := cs.pqZusageUrteilLocked(t.Context(), &z, pqZusageAb()+20)
	cs.mu.Unlock()
	if u.regel != "pq_form" {
		t.Fatalf("pq_zusage mit PQ: %+v", u)
	}
}

// Ein Umzugs-Auftrag mit ML-DSA zaehlt zu den 500 schweren PQ-Nachweisen je
// Block -- Erzeuger und Pruefende schneiden gleich ab.
func TestUmzugPQ_ZaehltZurBlockgrenze(t *testing.T) {
	umzugPQAktivFuerTest(t)
	pqAktivFuerTest(t)
	a := neuerPQKontoSchluessel(t, 0x6a)
	p := neuerPQKontoSchluessel(t, 0x6b)
	b := neuerTestSchluessel(t)
	var txs []Transaction
	for i := 0; i < pqUeberweisungenJeBlock; i++ {
		txs = append(txs, pqTransfer(t, a, b.addr, aeqWei(1), uint64(i)))
	}
	if n := pqUeberweisungGrenze(txs); n != len(txs) {
		t.Fatalf("500 PQ-Ueberweisungen: Grenze %d", n)
	}
	txs = append(txs, codeTxPQ(t, p, uzCode, uzZeit))
	if n := pqUeberweisungGrenze(txs); n != pqUeberweisungenJeBlock {
		t.Fatalf("500 + ein Umzug mit PQ: Grenze %d, erwartet %d", n, pqUeberweisungenJeBlock)
	}
	if _, err := pruefeUeberweisungenImBlock(txs, uzZeit); err == nil {
		t.Fatal("Block mit 501 schweren PQ-Nachweisen angenommen")
	}
	if !pqNachweisSchwer(&txs[len(txs)-1]) || pqNachweisSchwer(&Transaction{Type: umzugCodeTxArt, Umzug: &UmzugAngaben{}}) {
		t.Fatal("pqNachweisSchwer zaehlt falsch")
	}
}

// Ohne PQ bleibt jeder Auftrag byte-gleich (omitempty) -- alte Bloecke
// behalten ihren Hash.
func TestUmzugPQ_OhnePQByteGleich(t *testing.T) {
	roh, err := json.Marshal(UmzugAngaben{CodeHash: "ab", Sig: "0x11"})
	if err != nil || strings.Contains(string(roh), `"pq"`) {
		t.Fatalf("ohne PQ: %s", roh)
	}
}

// Endpunkte: vor dem Stichtag 409; nach dem Stichtag sig ODER pq_pub/pq_sig,
// nie beides; ein fremder Schluessel 403; der Gutfall 200 und die
// Transaktion traegt die ML-DSA-Unterschrift. Annahme prueft wie das
// Nachspielen.
func TestUmzugPQ_Endpunkte(t *testing.T) {
	f := neuerUzFall(t)
	f.cs.annehmendAusdruecklich.Store(true)
	a := &APIServer{state: f.cs}
	jetzt := nowUnix()
	p := neuerPQKontoSchluessel(t, 0x6c)
	fremd := neuerPQKontoSchluessel(t, 0x6d)
	// Das Menschsein steht schon auf der PQ-Adresse (wie nach einem Umzug).
	addHuman(f.cs, p.addr, 0)
	h := umzugCodeHash(p.addr, umzugCodeNormal(uzCode))
	msg := umzugCodeSetzenNachricht(p.addr, h, jetzt)

	ip := 0
	post := func(body map[string]interface{}) (int, string) {
		t.Helper()
		ip++
		roh, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, "/api/umzug/code", bytes.NewReader(roh))
		r.RemoteAddr = "198.51.100." + string(rune('0'+ip)) + ":1"
		w := httptest.NewRecorder()
		a.handleUmzugCode(w, r)
		return w.Code, w.Body.String()
	}
	koerper := func(pub, sig, eip string) map[string]interface{} {
		m := map[string]interface{}{"wallet": p.addr, "code_hash": h, "zeit": jetzt, "pq_pub": pub, "pq_sig": sig}
		if eip != "" {
			m["sig"] = eip
		}
		return m
	}
	gutSig := pqUnterschreibe(t, p, msg)

	vorher := umzugPQAbOverride.Load()
	t.Cleanup(func() { umzugPQAbOverride.Store(vorher) })
	umzugPQAbOverride.Store(math.MaxInt64)
	if c, b := post(koerper(p.pub, gutSig, "")); c != http.StatusConflict {
		t.Fatalf("vor dem Stichtag: %d %s", c, b)
	}
	umzugPQAbOverride.Store(1)
	if c, b := post(koerper(p.pub, gutSig, "0x"+strings.Repeat("11", 65))); c != http.StatusBadRequest {
		t.Fatalf("sig und pq zugleich: %d %s", c, b)
	}
	if c, b := post(koerper(p.pub[:100], gutSig, "")); c != http.StatusBadRequest {
		t.Fatalf("abgeschnittener Schluessel: %d %s", c, b)
	}
	if c, b := post(koerper(fremd.pub, pqUnterschreibe(t, fremd, msg), "")); c != http.StatusForbidden {
		t.Fatalf("fremder Schluessel: %d %s", c, b)
	}
	if c, b := post(koerper(p.pub, pqUnterschreibe(t, p, msg+"x"), "")); c != http.StatusForbidden {
		t.Fatalf("andere Nachricht: %d %s", c, b)
	}
	if c, b := post(koerper(p.pub, gutSig, "")); c != http.StatusOK {
		t.Fatalf("Gutfall: %d %s", c, b)
	}
	if c, ok := f.cs.umzugRegisterJetzt().code(p.addr); !ok || c.Hashes[0] != h {
		t.Fatal("Code der PQ-Adresse nicht im Register")
	}
}

// Der Antrag mit PQ-Unterschrift passt mit Bindung und fuenf Zeugen (je mit
// pq:-Eintrag) in die Grenze des Antrags -- heute laeuft ein Zeuge.
func TestUmzugPQ_AntragKoerperGrenze(t *testing.T) {
	eintrag := len(`"pq:"`) + pqEintragLaenge + 1
	v3 := 2*(len(`"v3:"`)+128) + 4096
	pq := len(`"pq_pub":"","pq_sig":""`) + pqPubHex + pqSigHex
	if 6*eintrag+v3+pq > umzugAntragKoerperMax {
		t.Fatalf("Antrag mit PQ und sechs pq:-Eintraegen: %d > %d", 6*eintrag+v3+pq, umzugAntragKoerperMax)
	}
	if pq+4096 > umzugKoerperMax {
		t.Fatal("Code oder Widerruf mit PQ passt nicht in umzugKoerperMax")
	}
}

// Die App (lib/tresor.ts + lib/umzug.ts, @noble/post-quantum, Seed 0x11*32)
// unterschreibt Antrag, Code und Widerruf so, dass JEDER Knoten sie annimmt
// (Vektor aus lib/__tests__/umzug_tresor.test.ts). Saetze byte-gleich, die
// Adresse ist die des Schluessels, jede Unterschrift nur fuer ihren Satz.
func TestUmzugPQ_VektorAusDerApp(t *testing.T) {
	roh, err := os.ReadFile("testdata/umzug_pq_app_vektor.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Adresse, Alt, Nullifier, Pub string
		Zeit                         int64
		CodeHash                     string `json:"code_hash"`
		Antrag, Code, Widerruf       string
		AntragSig                    string `json:"antrag_sig"`
		CodeSig                      string `json:"code_sig"`
		WiderrufSig                  string `json:"widerruf_sig"`
	}
	if err := json.Unmarshal(roh, &v); err != nil {
		t.Fatal(err)
	}
	if got := umzugAntragNachricht(v.Nullifier, v.Alt, v.Adresse, v.Zeit); got != v.Antrag {
		t.Fatalf("Antrag:\n got %s\nwant %s", got, v.Antrag)
	}
	if got := umzugCodeSetzenNachricht(v.Adresse, v.CodeHash, v.Zeit); got != v.Code {
		t.Fatalf("Code:\n got %s\nwant %s", got, v.Code)
	}
	if got := umzugWiderrufNachricht(v.Nullifier, v.Alt, v.Zeit); got != v.Widerruf {
		t.Fatalf("Widerruf:\n got %s\nwant %s", got, v.Widerruf)
	}
	if v.CodeHash != umzugCodeHash(v.Adresse, "ABCDEFGHJKMNPQRSTVWXYZ01") {
		t.Fatal("Code-Hash der App weicht ab")
	}
	for _, f := range []struct{ satz, sig string }{{v.Antrag, v.AntragSig}, {v.Code, v.CodeSig}, {v.Widerruf, v.WiderrufSig}} {
		if err := pqKontoPruefen(v.Adresse, f.satz, &PQKonto{Pub: v.Pub, Sig: f.sig}); err != nil {
			t.Fatalf("Unterschrift der App abgewiesen (%s): %v", f.satz[:30], err)
		}
		if pqKontoPruefen(v.Alt, f.satz, &PQKonto{Pub: v.Pub, Sig: f.sig}) == nil {
			t.Fatal("Unterschrift gilt fuer eine fremde Adresse")
		}
	}
	if pqKontoPruefen(v.Adresse, v.Code, &PQKonto{Pub: v.Pub, Sig: v.AntragSig}) == nil {
		t.Fatal("die Unterschrift des Antrags gilt fuer den Code")
	}
}

// Der gesetzte Stichtag (15.11.2026 19:00 UTC) gilt ohne jede Test-
// Ueberschreibung: eine Sekunde davor ist jede PQ-Unterschrift am Umzug ein
// Befund, ab dem Stichtag die Form erlaubt; /api/status nennt ihn (der
// Waechter liest ihn dort).
func TestUmzugPQ_StichtagGesetzt(t *testing.T) {
	vorher := umzugPQAbOverride.Load()
	umzugPQAbOverride.Store(0)
	t.Cleanup(func() { umzugPQAbOverride.Store(vorher) })
	const ab int64 = 1794769200
	if umzugPQAb() != ab {
		t.Fatalf("Stichtag %d, erwartet %d", umzugPQAb(), ab)
	}
	p := neuerPQKontoSchluessel(t, 0x70)
	u := &UmzugAngaben{PQ: pqKonto(t, p, "x")}
	if umzugSigForm(u, ab-1) {
		t.Fatal("eine Sekunde vor dem Stichtag erlaubt")
	}
	if !umzugSigForm(u, ab) {
		t.Fatal("zum Stichtag nicht erlaubt")
	}
	tx := codeTxPQ(t, p, uzCode, ab-10)
	if err := blockTauglich(&tx, ab-1); err == nil {
		t.Fatal("Block vor dem Stichtag nimmt einen Umzug mit PQ auf")
	}
	if s := umzugPQStatus(); s["ab"] != ab {
		t.Fatalf("Status: %v", s)
	}
}
