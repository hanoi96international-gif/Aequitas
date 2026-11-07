package keeper

import (
	"context"
	"crypto/ecdsa"
	"strings"
	"testing"
)

// Umzug (umzug.go): Gutfall mit und ohne Code, Widerruf, und jeder Angriff,
// den der Entwurf nennt (docs/UMZUG_ENTWURF.md 6.5).

const (
	uzZeit int64 = 1_700_000_000 // vor nachrechnenStrengAbUnix: Befunde zaehlen, lehnen nicht ab
	uzN          = "2258162185744682068870376411042672036729508593423772695192684261112116008409"
	uzN2         = "9904028930859697121695025471312564917337032846528014134060777877259199866166"
	uzCode       = "ABCD-EFGH-JKMN-PQRS-TVWX-YZ01"
)

var uzRegeln = []string{"umzug_form", "umzug_zeit", "umzug_unterschrift", "umzug_bescheinigung", "umzug_nullifier",
	"umzug_ziel", "umzug_offen", "umzug_jahresgrenze", "umzug_code", "umzug_voll", "umzug_nicht_offen",
	"umzug_frist", "umzug_code_alt", "umzug_code_kein_mensch", "umzug_code_voll"}

type uzFall struct {
	t      *testing.T
	cs     *ChainState
	coord  bTestSchluessel
	zeuge  bTestSchluessel
	altKey *ecdsa.PrivateKey
	alt    string
	neuKey *ecdsa.PrivateKey
	neu    string
}

func neuerUzFall(t *testing.T) *uzFall {
	t.Helper()
	vorher := umzugAbOverride.Load()
	umzugAbOverride.Store(1)
	t.Cleanup(func() { umzugAbOverride.Store(vorher) })
	f := &uzFall{t: t, cs: newTestState(), coord: neuerEd25519(t), zeuge: neuerEd25519(t)}
	setBescheinigungFuerTest(t, &bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: 2,
		Coordinator: []string{f.coord.pub}, Bezeuger: []string{f.zeuge.pub}})
	f.altKey, f.alt = neuerSchluessel(t)
	f.neuKey, f.neu = neuerSchluessel(t)
	addHuman(f.cs, f.alt, 1000)
	f.cs.nullifiersMu.Lock()
	if f.cs.nullifiers == nil {
		f.cs.nullifiers = map[string]string{}
	}
	f.cs.nullifiers[uzN] = f.alt
	f.cs.nullifiersMu.Unlock()
	return f
}

func uzZaehler() map[string]int64 {
	m := map[string]int64{}
	for _, r := range uzRegeln {
		m[r] = erhaltungZaehler(r)
	}
	return m
}

func uzNeu(vorher map[string]int64) map[string]int64 {
	m := map[string]int64{}
	for _, r := range uzRegeln {
		if d := erhaltungZaehler(r) - vorher[r]; d != 0 {
			m[r] = d
		}
	}
	return m
}

// antrag: ein vollstaendig gueltiger Antrag von neu fuer den Menschen n.
func (f *uzFall) antrag(neuKey *ecdsa.PrivateKey, neu, alt, n string, zeit int64, code string) Transaction {
	b := &BioBescheinigung{Zeit: zeit, Modus: "test", Signaturen: []string{
		f.coord.unterschreibe(string(umzugBindungNachricht("test", n, neu, zeit))),
		f.zeuge.unterschreibe(string(umzugAussageNachricht("test", n, zeit))),
	}}
	return Transaction{Type: umzugAnkuendigenTxArt, Wallet: neu, To: alt, Nullifier: n, DistributionAt: zeit,
		BioBescheinigung: b, Umzug: &UmzugAngaben{Code: umzugCodeNormal(code), Sig: personalSign(f.t, neuKey, umzugAntragNachricht(n, alt, neu, zeit))}}
}

func (f *uzFall) codeTx(key *ecdsa.PrivateKey, wallet, code string, zeit int64) Transaction {
	h := umzugCodeHash(wallet, umzugCodeNormal(code))
	return Transaction{Type: umzugCodeTxArt, Wallet: wallet, DistributionAt: zeit,
		Umzug: &UmzugAngaben{CodeHash: h, Sig: personalSign(f.t, key, umzugCodeSetzenNachricht(wallet, h, zeit))}}
}

func (f *uzFall) widerruf(key *ecdsa.PrivateKey, alt, neu, n string, angekuendigt int64) Transaction {
	return Transaction{Type: umzugWiderrufTxArt, Wallet: alt, To: neu, Nullifier: n,
		Umzug: &UmzugAngaben{Angekuendigt: angekuendigt, Sig: personalSign(f.t, key, umzugWiderrufNachricht(n, neu, angekuendigt))}}
}

func (f *uzFall) anwenden(tx Transaction, blockZeit int64) map[string]int64 {
	f.t.Helper()
	vorher := uzZaehler()
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	if err := f.cs.applyUmzugLocked(context.Background(), &tx, blockZeit); err != nil {
		f.t.Fatalf("applyUmzugLocked: %v", err)
	}
	return uzNeu(vorher)
}

func (f *uzFall) mensch(w string) bool {
	acc, ok := f.cs.accounts.Get(w)
	return ok && acc.IsHuman
}

func (f *uzFall) guthaben(w string) float64 {
	if acc, ok := f.cs.accounts.Get(w); ok {
		return acc.Balance.Float()
	}
	return 0
}

func (f *uzFall) inhaber(n string) string {
	f.cs.nullifiersMu.Lock()
	defer f.cs.nullifiersMu.Unlock()
	return f.cs.nullifiers[n]
}

func (f *uzFall) umgezogen() bool {
	return !f.mensch(f.alt) && f.mensch(f.neu) && f.inhaber(uzN) == f.neu
}

func TestUmzug_MitCodeSofortOhneNeuesGeld(t *testing.T) {
	f := neuerUzFall(t)
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit); len(b) != 0 {
		t.Fatalf("Code hinterlegen: Befunde %v", b)
	}
	if _, ok := f.cs.umzugRegisterJetzt().code(f.alt); !ok {
		t.Fatal("Code-Hash steht nicht im Register")
	}
	if b := f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, uzZeit+100, uzCode), uzZeit+120); len(b) != 0 {
		t.Fatalf("Antrag mit Code: Befunde %v", b)
	}
	if !f.umgezogen() {
		t.Fatal("mit Gesicht und Code muss der Umzug sofort geschehen")
	}
	if f.guthaben(f.alt) != 1000 || f.guthaben(f.neu) != 0 {
		t.Fatalf("Guthaben bewegt: alt %v, neu %v -- es darf nichts umziehen und nichts entstehen", f.guthaben(f.alt), f.guthaben(f.neu))
	}
	if acc, _ := f.cs.accounts.Get(f.neu); !acc.FaucetClaimed {
		t.Fatal("neue Wallet koennte ein zweites Startgeld abholen")
	}
	reg := f.cs.umzugRegisterJetzt()
	if _, ok := reg.code(f.alt); ok || len(reg.Erledigt) != 1 || len(reg.Offen) != 0 {
		t.Fatalf("Register nach dem Umzug: %+v", reg)
	}
	// Der Code ist verbraucht: derselbe Antrag noch einmal tut nichts.
	if b := f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, uzZeit+200, uzCode), uzZeit+220); b["umzug_nullifier"] != 1 {
		t.Fatalf("Wiederholung: %v", b)
	}
}

func TestUmzug_OhneCodeErstNachFrist(t *testing.T) {
	f := neuerUzFall(t)
	z := uzZeit + 100
	if b := f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, z, ""), z+10); len(b) != 0 {
		t.Fatalf("Antrag: Befunde %v", b)
	}
	if f.umgezogen() {
		t.Fatal("ohne Code darf der Umzug nicht sofort geschehen")
	}
	o, ok := f.cs.umzugRegisterJetzt().offen(uzN)
	if !ok || o.FreiAb != z+umzugFrist {
		t.Fatalf("offener Umzug: %+v %v", o, ok)
	}
	// Vollzug vor der Frist: Befund, nichts geschieht.
	frueh := Transaction{Type: umzugVollzugTxArt, Wallet: f.neu, To: f.alt, Nullifier: uzN, DistributionAt: z + umzugFrist - 1}
	if b := f.anwenden(frueh, z+umzugFrist-1); b["umzug_frist"] != 1 || f.umgezogen() {
		t.Fatalf("Vollzug vor der Frist: %v", b)
	}
	// Die Tagesrunde nach der Frist vollzieht.
	f.cs.mu.Lock()
	txs, err := f.cs.umzugVollzuegeLocked(context.Background(), z+umzugFrist)
	f.cs.mu.Unlock()
	if err != nil || len(txs) != 1 || !f.umgezogen() {
		t.Fatalf("Tagesrunde: %v %v, umgezogen %v", txs, err, f.umgezogen())
	}
	if f.guthaben(f.alt) != 1000 {
		t.Fatal("das Guthaben der alten Wallet muss bleiben")
	}
}

func TestUmzug_NachspielenGleichErzeuger(t *testing.T) {
	// Erzeuger (Tagesrunde) und Nachspielender (Vollzugs-Transaktion) kommen
	// zum selben Register und denselben Konten.
	erzeuger, nachspiel := neuerUzFall(t), neuerUzFall(t)
	nachspiel.altKey, nachspiel.alt, nachspiel.neuKey, nachspiel.neu = erzeuger.altKey, erzeuger.alt, erzeuger.neuKey, erzeuger.neu
	nachspiel.coord, nachspiel.zeuge = erzeuger.coord, erzeuger.zeuge
	// Beide Knoten mit denselben Schluesseln der Genesis.
	setBescheinigungFuerTest(t, &bescheinigungKonfig{AbUnix: 1, Modus: "test", Quorum: 2,
		Coordinator: []string{erzeuger.coord.pub}, Bezeuger: []string{erzeuger.zeuge.pub}})
	nachspiel.cs = newTestState()
	addHuman(nachspiel.cs, nachspiel.alt, 1000)
	nachspiel.cs.nullifiers = map[string]string{uzN: nachspiel.alt}
	z := uzZeit + 100
	antrag := erzeuger.antrag(erzeuger.neuKey, erzeuger.neu, erzeuger.alt, uzN, z, "")
	if b := erzeuger.anwenden(antrag, z+10); len(b) != 0 {
		t.Fatalf("Erzeuger: %v", b)
	}
	if b := nachspiel.anwenden(antrag, z+10); len(b) != 0 {
		t.Fatalf("Nachspielender: %v", b)
	}
	erzeuger.cs.mu.Lock()
	txs, err := erzeuger.cs.umzugVollzuegeLocked(context.Background(), z+umzugFrist)
	erzeuger.cs.mu.Unlock()
	if err != nil || len(txs) != 1 {
		t.Fatal(txs, err)
	}
	if b := nachspiel.anwenden(txs[0], z+umzugFrist+5); len(b) != 0 {
		t.Fatalf("Nachspielen: Befunde %v", b)
	}
	if erzeuger.cs.umzugRegisterJetzt().hash != nachspiel.cs.umzugRegisterJetzt().hash || !nachspiel.umgezogen() {
		t.Fatal("Erzeuger und Nachspielender weichen ab")
	}
}

func TestUmzug_WiderrufNurDurchAlteWallet(t *testing.T) {
	f := neuerUzFall(t)
	z := uzZeit + 100
	f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, z, ""), z+10)
	fremdKey, _ := neuerSchluessel(t)
	if b := f.anwenden(f.widerruf(fremdKey, f.alt, f.neu, uzN, z), z+20); b["umzug_unterschrift"] != 1 {
		t.Fatalf("Widerruf mit fremdem Schluessel: %v", b)
	}
	if b := f.anwenden(f.widerruf(f.neuKey, f.alt, f.neu, uzN, z), z+20); b["umzug_unterschrift"] != 1 {
		t.Fatalf("Widerruf durch die neue Wallet: %v", b)
	}
	if b := f.anwenden(f.widerruf(f.altKey, f.alt, f.neu, uzN, z-1), z+20); b["umzug_nicht_offen"] != 1 {
		t.Fatalf("Widerruf einer anderen Ankuendigung: %v", b)
	}
	if b := f.anwenden(f.widerruf(f.altKey, f.alt, f.neu, uzN, z), z+20); len(b) != 0 {
		t.Fatalf("Widerruf: %v", b)
	}
	f.cs.mu.Lock()
	txs, _ := f.cs.umzugVollzuegeLocked(context.Background(), z+umzugFrist)
	f.cs.mu.Unlock()
	if len(txs) != 0 || f.umgezogen() || !f.mensch(f.alt) {
		t.Fatal("nach dem Widerruf darf nichts vollzogen werden")
	}
}

func TestUmzug_MitCodeKeinWiderrufMoeglich(t *testing.T) {
	// Entscheidung 07.11.: Gesicht + Code -- die alte Wallet (vielleicht ein
	// Dieb mit dem Seed) kann nichts mehr aufhalten.
	f := neuerUzFall(t)
	f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit)
	f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, uzZeit+100, uzCode), uzZeit+110)
	if b := f.anwenden(f.widerruf(f.altKey, f.alt, f.neu, uzN, uzZeit+100), uzZeit+120); b["umzug_nicht_offen"] != 1 || !f.umgezogen() {
		t.Fatalf("Widerruf nach Umzug mit Code: %v", b)
	}
}

func TestUmzug_Missbrauch(t *testing.T) {
	z := uzZeit + 100
	fall := func(t *testing.T, mit func(f *uzFall) (Transaction, int64)) (*uzFall, map[string]int64) {
		f := neuerUzFall(t)
		tx, block := mit(f)
		return f, f.anwenden(tx, block)
	}
	tests := map[string]struct {
		regel string
		mit   func(f *uzFall) (Transaction, int64)
	}{
		"Unterschrift einer Registrierung statt eines Umzugs": {"umzug_bescheinigung", func(f *uzFall) (Transaction, int64) {
			tx := f.antrag(f.neuKey, f.neu, f.alt, uzN, z, "")
			tx.BioBescheinigung.Signaturen = []string{bindung(f.coord, "test", uzN, f.neu, z), aussage(f.zeuge, "test", uzN, z)}
			return tx, z
		}},
		"nur der Coordinator, kein Vergleichsdienst": {"umzug_bescheinigung", func(f *uzFall) (Transaction, int64) {
			tx := f.antrag(f.neuKey, f.neu, f.alt, uzN, z, "")
			tx.BioBescheinigung.Signaturen = tx.BioBescheinigung.Signaturen[:1]
			return tx, z
		}},
		"Bescheinigung fuer eine andere neue Wallet": {"umzug_bescheinigung", func(f *uzFall) (Transaction, int64) {
			fremdKey, fremd := neuerSchluessel(t)
			tx := f.antrag(f.neuKey, f.neu, f.alt, uzN, z, "")
			// Die Bescheinigung gilt f.neu; ein Dritter setzt seine Wallet ein.
			tx.Wallet = fremd
			tx.Umzug.Sig = personalSign(t, fremdKey, umzugAntragNachricht(uzN, f.alt, fremd, z))
			return tx, z
		}},
		"Antrag nicht von der neuen Wallet unterschrieben": {"umzug_unterschrift", func(f *uzFall) (Transaction, int64) {
			fremdKey, _ := neuerSchluessel(t)
			tx := f.antrag(f.neuKey, f.neu, f.alt, uzN, z, "")
			tx.Umzug.Sig = personalSign(t, fremdKey, umzugAntragNachricht(uzN, f.alt, f.neu, z))
			return tx, z
		}},
		"Nullifier gehoert einem anderen": {"umzug_nullifier", func(f *uzFall) (Transaction, int64) {
			return f.antrag(f.neuKey, f.neu, f.alt, uzN2, z, ""), z
		}},
		"neue Wallet ist schon ein Mensch": {"umzug_ziel", func(f *uzFall) (Transaction, int64) {
			addHuman(f.cs, f.neu, 5)
			return f.antrag(f.neuKey, f.neu, f.alt, uzN, z, ""), z
		}},
		"neue Wallet hat schon einen Nullifier": {"umzug_ziel", func(f *uzFall) (Transaction, int64) {
			f.cs.nullifiers[uzN2] = f.neu
			return f.antrag(f.neuKey, f.neu, f.alt, uzN, z, ""), z
		}},
		"Bescheinigung zu alt (Wiederholung)": {"umzug_zeit", func(f *uzFall) (Transaction, int64) {
			return f.antrag(f.neuKey, f.neu, f.alt, uzN, z, ""), z + umzugHoechstensAlt + 1
		}},
		"Bescheinigung aus der Zukunft": {"umzug_zeit", func(f *uzFall) (Transaction, int64) {
			return f.antrag(f.neuKey, f.neu, f.alt, uzN, z+umzugHoechstensVoraus+1, ""), z
		}},
		"falscher Code faellt nicht still auf 14 Tage zurueck": {"umzug_code", func(f *uzFall) (Transaction, int64) {
			f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit)
			return f.antrag(f.neuKey, f.neu, f.alt, uzN, z, "ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ"), z
		}},
		"Code ohne hinterlegten Hash": {"umzug_code", func(f *uzFall) (Transaction, int64) {
			return f.antrag(f.neuKey, f.neu, f.alt, uzN, z, uzCode), z
		}},
		"Code von fremder Hand hinterlegt": {"umzug_unterschrift", func(f *uzFall) (Transaction, int64) {
			fremdKey, _ := neuerSchluessel(t)
			return f.codeTx(fremdKey, f.alt, uzCode, uzZeit), uzZeit
		}},
		"aelterer Code wiederholt": {"umzug_code_alt", func(f *uzFall) (Transaction, int64) {
			alt := f.codeTx(f.altKey, f.alt, uzCode, uzZeit)
			f.anwenden(f.codeTx(f.altKey, f.alt, "ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ", uzZeit+10), uzZeit+10)
			return alt, uzZeit + 20
		}},
		"Code fuer jemanden, der kein Mensch ist": {"umzug_code_kein_mensch", func(f *uzFall) (Transaction, int64) {
			return f.codeTx(f.neuKey, f.neu, uzCode, uzZeit), uzZeit
		}},
		"zweiter offener Umzug fuer denselben Menschen": {"umzug_offen", func(f *uzFall) (Transaction, int64) {
			f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, z, ""), z)
			k3, w3 := neuerSchluessel(t)
			return f.antrag(k3, w3, f.alt, uzN, z+5, ""), z + 5
		}},
		"neue Wallet nicht kanonisch geschrieben": {"umzug_form", func(f *uzFall) (Transaction, int64) {
			tx := f.antrag(f.neuKey, f.neu, f.alt, uzN, z, "")
			tx.Wallet = strings.ToUpper(f.neu) // nicht kanonisch
			return tx, z
		}},
		"Vollzug ohne offenen Umzug": {"umzug_nicht_offen", func(f *uzFall) (Transaction, int64) {
			return Transaction{Type: umzugVollzugTxArt, Wallet: f.neu, To: f.alt, Nullifier: uzN, DistributionAt: z}, z
		}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f, befunde := fall(t, tc.mit)
			if befunde[tc.regel] != 1 {
				t.Fatalf("erwartet Befund %s, bekommen %v", tc.regel, befunde)
			}
			if f.umgezogen() || !f.mensch(f.alt) {
				t.Fatal("der Angriff hat das Menschsein bewegt")
			}
		})
	}
}

func TestUmzug_JahresgrenzeZwei(t *testing.T) {
	f := neuerUzFall(t)
	z := uzZeit + 100
	// Erster Umzug alt -> neu, zweiter neu -> drei, dritter drei -> vier.
	f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, z), z)
	f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, z+10, uzCode), z+10)
	k3, w3 := neuerSchluessel(t)
	f.anwenden(f.codeTx(f.neuKey, f.neu, uzCode, z+20), z+20)
	if b := f.anwenden(f.antrag(k3, w3, f.neu, uzN, z+30, uzCode), z+30); len(b) != 0 {
		t.Fatalf("zweiter Umzug: %v", b)
	}
	k4, w4 := neuerSchluessel(t)
	f.anwenden(f.codeTx(k3, w3, uzCode, z+40), z+40)
	if b := f.anwenden(f.antrag(k4, w4, w3, uzN, z+50, uzCode), z+50); b["umzug_jahresgrenze"] != 1 {
		t.Fatalf("dritter Umzug im Jahr: %v", b)
	}
	// Nach einem Jahr wieder moeglich.
	spaet := z + umzugJahr + 100
	f.anwenden(f.codeTx(k3, w3, uzCode, spaet), spaet)
	if b := f.anwenden(f.antrag(k4, w4, w3, uzN, spaet+10, uzCode), spaet+10); len(b) != 0 {
		t.Fatalf("Umzug nach einem Jahr: %v", b)
	}
}

func TestUmzug_VorDemStichtagUnbekannt(t *testing.T) {
	vorher := umzugAbOverride.Load()
	umzugAbOverride.Store(0)
	defer umzugAbOverride.Store(vorher)
	for _, a := range []string{umzugCodeTxArt, umzugAnkuendigenTxArt, umzugWiderrufTxArt, umzugVollzugTxArt} {
		if bekannteTxArt(a, umzugAbUnix-1) {
			t.Fatalf("%s vor dem Stichtag bekannt", a)
		}
		if !bekannteTxArt(a, umzugAbUnix) {
			t.Fatalf("%s ab dem Stichtag unbekannt", a)
		}
	}
}

func TestUmzug_RegisterKanonischUndStateRoot(t *testing.T) {
	f := neuerUzFall(t)
	leer := f.cs.StateRoot()
	f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit)
	if f.cs.StateRoot() == leer {
		t.Fatal("das Register muss in die StateRoot")
	}
	reg := f.cs.umzugRegisterJetzt()
	wieder, err := umzugRegisterLesen(reg.kodiert())
	if err != nil || wieder.hash != reg.hash {
		t.Fatalf("Register nicht wieder lesbar: %v", err)
	}
	for name, roh := range map[string]string{
		"Leerzeichen":       " " + reg.kodiert(),
		"unbekanntes Feld":  `{"codes":[],"x":1}`,
		"Frist manipuliert": `{"offen":[{"n":"` + uzN + `","alt":"` + f.alt + `","neu":"` + f.neu + `","z":10,"frei_ab":11}]}`,
	} {
		if _, err := umzugRegisterLesen(roh); err == nil {
			t.Fatalf("%s: unlesbares Register angenommen", name)
		}
	}
	// Ohne Eintraege bleibt die Wurzel byte-gleich mit der ohne Umzug.
	f2 := neuerUzFall(t)
	f2.cs.umzugReg.Store((&umzugRegister{}).fertig())
	if f2.cs.StateRoot() != leer {
		t.Log("Hinweis: zwei Faelle haben verschiedene Konten; nur Leer-Register geprueft")
	}
}

func TestUmzug_CodeForm(t *testing.T) {
	for roh, gut := range map[string]bool{
		uzCode: true, strings.ToLower(uzCode): true, "ABCDEFGHJKMNPQRSTVWXYZ01": true,
		"ABCD-EFGH-JKMN-PQRS-TVWX-YZ0": false, "ABCD-EFGH-JKMN-PQRS-TVWX-YZOI": false, "": false,
	} {
		if (umzugCodeNormal(roh) != "") != gut {
			t.Fatalf("%q: gueltig=%v erwartet", roh, gut)
		}
	}
	if umzugCodeHash("0x01", "A") == umzugCodeHash("0x02", "A") {
		t.Fatal("der Hash muss an die Wallet gebunden sein")
	}
}

// Pruefvektoren, byte-gleich in der App (lib/__tests__/umzug.test.ts). Weicht
// ein Zeichen ab, passt kein Code und keine Unterschrift.
func TestUmzug_PruefvektorenFuerDieApp(t *testing.T) {
	alt := "0x00000000000000000000000000000000000000a1"
	if h := umzugCodeHash(alt, "ABCDEFGHJKMNPQRSTVWXYZ01"); h != "abfd9a9f06ddeadaf8b4a8ab9eb4f30363ceabbdb34e44b57cb9503d68529777" {
		t.Fatalf("umzugCodeHash = %s", h)
	}
	for got, want := range map[string]string{
		umzugCodeSetzenNachricht(alt, "ab", 7):                "aequitas-umzug-code-setzen-v1|chain:1926|" + alt + "|ab|7",
		umzugAntragNachricht("42", alt, "0xb", 9):             "aequitas-umzug-v1|chain:1926|42|" + alt + "|0xb|9",
		umzugWiderrufNachricht("42", "0xb", 9):                "aequitas-umzug-widerruf-v1|chain:1926|42|0xb|9",
		string(umzugBindungNachricht("real", "42", "0xb", 9)): "aequitas-umzug-bindung-v1|real|42|0xb|9",
		string(umzugAussageNachricht("real", "42", 9)):        "aequitas-umzug-aussage-v1|real|42|9",
	} {
		if got != want {
			t.Fatalf("%q != %q", got, want)
		}
	}
}

// Sicherheitsdurchgang des Diffs: ein Dieb mit dem alten Schluessel hinterlegt
// einen eigenen Code. Der Code des Menschen bleibt gueltig -- mit Gesicht und
// SEINEM Code zieht der Mensch sofort um, und der Dieb kann nichts widerrufen.
func TestUmzug_DiebKannDenCodeNichtErsetzen(t *testing.T) {
	f := neuerUzFall(t)
	f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit), uzZeit)
	// Der Dieb hat den Seed: er unterschreibt als f.alt einen neuen Code.
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, "ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ", uzZeit+10), uzZeit+10); len(b) != 0 {
		t.Fatalf("zweiter Code: %v", b)
	}
	if c, _ := f.cs.umzugRegisterJetzt().code(f.alt); len(c.Hashes) != 2 {
		t.Fatalf("Codes: %+v", c)
	}
	if b := f.anwenden(f.antrag(f.neuKey, f.neu, f.alt, uzN, uzZeit+100, uzCode), uzZeit+110); len(b) != 0 || !f.umgezogen() {
		t.Fatalf("der Mensch muss mit seinem Code sofort umziehen koennen: %v", b)
	}
}

func TestUmzug_HoechstensDreiCodes(t *testing.T) {
	f := neuerUzFall(t)
	for i, c := range []string{uzCode, "ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZ", "1111-2222-3333-4444-5555-6666"} {
		z := uzZeit + int64(i)*10
		if b := f.anwenden(f.codeTx(f.altKey, f.alt, c, z), z); len(b) != 0 {
			t.Fatalf("Code %d: %v", i+1, b)
		}
	}
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, "7777-8888-9999-AAAA-BBBB-CCCC", uzZeit+40), uzZeit+40); b["umzug_code_voll"] != 1 {
		t.Fatalf("vierter Code: %v", b)
	}
	if b := f.anwenden(f.codeTx(f.altKey, f.alt, uzCode, uzZeit+50), uzZeit+50); b["umzug_code_voll"] != 1 {
		t.Fatalf("Wiederholung eines Codes: %v", b)
	}
}
