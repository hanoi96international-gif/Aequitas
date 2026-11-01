package keeper

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
)

// Staffel: Freigabe spaetestens an Tag 30 (grant_staffel.go) und Einspruch
// verschiedener Betreiber-Menschen (staffel_einspruch.go), ohne Datenbank.

const (
	seRegZeit int64 = 1_700_000_000 // vor nachrechnenStrengAbUnix: Befunde zaehlen, lehnen nicht ab
	seTag     int64 = 86400
	seWallet        = "0x00000000000000000000000000000000000000f1"
	seAndere        = "0x00000000000000000000000000000000000000f2"
	seGrund         = "Aufnahme aus einer Gruppe gleicher Kunstfiguren (Vorgang W-123)"
)

var seRegeln = []string{"einspruch_form", "einspruch_zeit", "einspruch_ohne_staffel", "einspruch_zu_spaet",
	"einspruch_ohne_bescheinigung", "einspruch_quorum",
	"freigabe_zu_hoch", "freigabe_ohne_lebenszeichen", "freigabe_angehalten", "freigabe_rundenzeit", "freigabe_doppelt"}

func seZaehler() map[string]int64 {
	m := map[string]int64{}
	for _, r := range seRegeln {
		m[r] = erhaltungZaehler(r)
	}
	return m
}

func seNeu(vorher map[string]int64) map[string]int64 {
	m := map[string]int64{}
	for _, r := range seRegeln {
		if d := erhaltungZaehler(r) - vorher[r]; d != 0 {
			m[r] = d
		}
	}
	return m
}

// seBetreiber: ein zugelassener Coordinator-Mensch mit Schluessel und Bindung.
type seBetreiber struct {
	priv    ed25519.PrivateKey
	pubHex  string
	bindung CoordinatorBindung
}

func (b seBetreiber) unterschreibe(grund, wallet string, issued int64) Lebendigkeitsbescheinigung {
	sig := ed25519.Sign(b.priv, []byte(staffelEinspruchNachricht(grund)(wallet, issued)))
	return Lebendigkeitsbescheinigung{PublicKey: b.pubHex, Signature: hex.EncodeToString(sig),
		Mensch: b.bindung.Mensch, MenschSig: b.bindung.MenschSig, SchluesselSig: b.bindung.SchluesselSig}
}

type seFall struct {
	t          *testing.T
	cs         *ChainState
	betreiber  []seBetreiber
	zugelassen map[string]bool
}

// neuerSeFall: Staffel aktiv, Coordinatoren im Konsens zugelassen, drei
// Betreiber-Menschen, zwei gestaffelte Registrierungen zu seRegZeit.
func neuerSeFall(t *testing.T) *seFall {
	t.Helper()
	vorherStaffel, vorherLeser := stagedGrantActivationOverride.Load(), registerLeserOverride.Load()
	stagedGrantActivationOverride.Store(1)
	registerLeserOverride.Store(1)
	t.Cleanup(func() {
		stagedGrantActivationOverride.Store(vorherStaffel)
		registerLeserOverride.Store(vorherLeser)
	})
	f := &seFall{t: t, cs: newTestState(), zugelassen: map[string]bool{}}
	for i := 0; i < 3; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		mk, mensch := neuerSchluessel(t)
		pubHex := hex.EncodeToString(pub)
		f.betreiber = append(f.betreiber, seBetreiber{priv: priv, pubHex: pubHex, bindung: CoordinatorBindung{Mensch: mensch,
			MenschSig:     personalSign(t, mk, coordinatorFreigabeNachricht(pubHex)),
			SchluesselSig: hex.EncodeToString(ed25519.Sign(priv, []byte(coordinatorBesitzNachricht(mensch))))}})
		addHuman(f.cs, mensch, 1000)
		f.zugelassen[mensch] = true
	}
	f.cs.zulassungOhneDB = func(m string, _ int64) (bool, error) { return f.zugelassen[m], nil }
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	for _, w := range []string{seWallet, seAndere} {
		if err := f.cs.registerHumanMitKlasseLocked(context.Background(), w, seRegZeit, "gestaffelt"); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *seFall) einspruch(wallet string, issued int64, grund string, von ...int) Transaction {
	e := &StaffelEinspruch{Grund: grund}
	for _, i := range von {
		e.Bescheinigungen = append(e.Bescheinigungen, f.betreiber[i].unterschreibe(grund, wallet, issued))
	}
	return Transaction{Type: staffelEinspruchTxArt, Wallet: wallet, DistributionAt: issued, Einspruch: e}
}

func (f *seFall) anwenden(tx Transaction, blockZeit int64) (map[string]int64, error) {
	f.t.Helper()
	vorher := seZaehler()
	f.cs.mu.Lock()
	err := f.cs.applyStaffelEinspruchLocked(context.Background(), &tx, blockZeit)
	f.cs.mu.Unlock()
	return seNeu(vorher), err
}

// runde: eine Tagesrunde der Staffel zur Zeit t; Summe der Freigaben je Wallet.
func (f *seFall) runde(t int64) map[string]float64 {
	f.t.Helper()
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	txs, err := f.cs.grantReleasesLocked(context.Background(), t)
	if err != nil {
		f.t.Fatal(err)
	}
	m := map[string]float64{}
	for _, tx := range txs {
		if tx.DistributionAt != t {
			f.t.Fatalf("Freigabe ohne Rundenzeit: %+v", tx)
		}
		m[tx.Wallet] += tx.Amount
	}
	return m
}

func (f *seFall) erneuern(w string, zeit int64) {
	f.t.Helper()
	f.cs.mu.Lock()
	defer f.cs.mu.Unlock()
	if err := f.cs.applyLivenessRenewalDeltaLocked(context.Background(), w, zeit, zeit); err != nil {
		f.t.Fatal(err)
	}
}

// Ohne zweite Pruefung: bis Tag 30 nichts, an Tag 30 der ganze Rest -- und
// die Geldmenge bleibt 1.000.
func TestStaffel_Tag30FreigabeOhneErneuerung(t *testing.T) {
	f := neuerSeFall(t)
	for tag := int64(1); tag < 30; tag++ {
		if m := f.runde(seRegZeit + tag*seTag); len(m) != 0 {
			t.Fatalf("Tag %d: Freigabe ohne Pruefung vor Tag 30: %v", tag, m)
		}
	}
	m := f.runde(seRegZeit + 30*seTag)
	if m[seWallet] != 800 || m[seAndere] != 800 {
		t.Fatalf("Tag 30: %v, erwartet je 800", m)
	}
	acc := acct(f.cs, seWallet)
	if acc.Balance.Float() != 1000 || acc.GrantStagedRest != 0 || acc.GrantStagedUntil != 0 {
		t.Fatalf("nach Tag 30: %+v", acc)
	}
	if m := f.runde(seRegZeit + 31*seTag); len(m) != 0 {
		t.Fatalf("nach dem Ende noch eine Freigabe: %v", m)
	}
}

// Die zweite Pruefung beschleunigt: wer sie macht, hat an jedem Tag
// mindestens so viel frei wie wer sie auslaesst, und an Tag 30 alles.
func TestStaffel_ErneuerungIstNieLangsamer(t *testing.T) {
	f := neuerSeFall(t)
	f.erneuern(seWallet, seRegZeit+7*seTag)
	var mit, ohne float64
	for tag := int64(8); tag <= 30; tag++ {
		m := f.runde(seRegZeit + tag*seTag)
		mit += m[seWallet]
		ohne += m[seAndere]
		if mit+1e-6 < ohne {
			t.Fatalf("Tag %d: mit Pruefung %.6f, ohne %.6f", tag, mit, ohne)
		}
	}
	if round6(mit) != 800 || round6(ohne) != 800 {
		t.Fatalf("an Tag 30: mit %.6f, ohne %.6f", mit, ohne)
	}
}

// Nachspielen: ein Erzeuger darf vor Tag 30 ohne Pruefung nichts und nie mehr
// als erlaubt freigeben; ab Tag 30 den Rest; die Rundenzeit nicht vordatieren.
func TestStaffel_FreigabeNachrechnen(t *testing.T) {
	block := seRegZeit + 10*seTag
	faelle := []struct {
		name  string
		vorab func(f *seFall)
		tx    Transaction
		block int64
		regel string
	}{
		{"vor Tag 30 ohne Pruefung", nil,
			Transaction{Type: "grant_release", Wallet: seWallet, Amount: 1, DistributionAt: block}, block, "freigabe_ohne_lebenszeichen"},
		{"vor Tag 30 alles mit Pruefung", func(f *seFall) { f.erneuern(seWallet, seRegZeit+7*seTag) },
			Transaction{Type: "grant_release", Wallet: seWallet, Amount: 800, DistributionAt: block}, block, "freigabe_zu_hoch"},
		{"Rundenzeit vordatiert auf Tag 30", nil,
			Transaction{Type: "grant_release", Wallet: seWallet, Amount: 800, DistributionAt: seRegZeit + 30*seTag}, block, "freigabe_rundenzeit"},
		{"Tag 30 trotz Einspruch", func(f *seFall) {
			if b, err := f.anwenden(f.einspruch(seWallet, seRegZeit+20*seTag, seGrund, 0, 1), seRegZeit+20*seTag); err != nil || len(b) != 0 {
				f.t.Fatalf("Einspruch: %v %v", b, err)
			}
		}, Transaction{Type: "grant_release", Wallet: seWallet, Amount: 800, DistributionAt: seRegZeit + 30*seTag}, seRegZeit + 30*seTag, "freigabe_angehalten"},
		{"ehrlich an Tag 30", nil,
			Transaction{Type: "grant_release", Wallet: seWallet, Amount: 800, DistributionAt: seRegZeit + 30*seTag}, seRegZeit + 30*seTag, ""},
		{"ehrlich mit Pruefung", func(f *seFall) { f.erneuern(seWallet, seRegZeit+7*seTag) },
			Transaction{Type: "grant_release", Wallet: seWallet, Amount: grantStaffelTagesrate(), DistributionAt: block}, block, ""},
	}
	for _, fall := range faelle {
		t.Run(fall.name, func(t *testing.T) {
			f := neuerSeFall(t)
			if fall.vorab != nil {
				fall.vorab(f)
			}
			vorher := seZaehler()
			f.cs.mu.Lock()
			err := f.cs.nachrechnenFreigabeLocked(&fall.tx, fall.block)
			f.cs.nachrechnenFreigabeRundeEndeLocked()
			f.cs.mu.Unlock()
			neu := seNeu(vorher)
			if err != nil {
				t.Fatalf("im Beobachtungsmodus kein Fehler: %v", err)
			}
			if fall.regel == "" && len(neu) != 0 {
				t.Fatalf("ehrlich meldet %v", neu)
			}
			if fall.regel != "" && neu[fall.regel] != 1 {
				t.Fatalf("Befund %v, erwartet %s", neu, fall.regel)
			}
		})
	}
}

// Ein gueltiger Einspruch haelt Tag 30 an -- befristet bis Tag 180, und eine
// neuere zweite Pruefung beendet die Pause sofort.
func TestStaffelEinspruch_HaeltAnBefristet(t *testing.T) {
	f := neuerSeFall(t)
	issued := seRegZeit + 20*seTag
	for i := 0; i < 2; i++ { // zweimal nachgespielt: idempotent
		if b, err := f.anwenden(f.einspruch(seWallet, issued, seGrund, 0, 1), issued+60); err != nil || len(b) != 0 {
			t.Fatalf("Lauf %d: %v %v", i, b, err)
		}
	}
	if acct(f.cs, seWallet).StaffelEinspruchAm != issued {
		t.Fatalf("Einspruch nicht gesetzt: %+v", acct(f.cs, seWallet))
	}
	if st := StaffelStand(acct(f.cs, seWallet)); st["frei_ab"] != seRegZeit+180*seTag || st["einspruch_am"] != issued {
		t.Fatalf("Anzeige: %v", st)
	}
	m := f.runde(seRegZeit + 30*seTag)
	if m[seWallet] != 0 || m[seAndere] != 800 {
		t.Fatalf("Tag 30: %v (angehalten nur der Betroffene)", m)
	}
	if m := f.runde(seRegZeit + 179*seTag); m[seWallet] != 0 {
		t.Fatalf("Tag 179: %v", m)
	}
	if m := f.runde(seRegZeit + 180*seTag); m[seWallet] != 800 {
		t.Fatalf("Tag 180 muss frei werden: %v", m)
	}
}

func TestStaffelEinspruch_NeuerePruefungBeendetPause(t *testing.T) {
	f := neuerSeFall(t)
	f.erneuern(seWallet, seRegZeit+8*seTag) // Pruefung VOR dem Einspruch ...
	issued := seRegZeit + 20*seTag
	if b, err := f.anwenden(f.einspruch(seWallet, issued, seGrund, 1, 2), issued); err != nil || len(b) != 0 {
		t.Fatalf("Einspruch: %v %v", b, err)
	}
	if m := f.runde(seRegZeit + 21*seTag); m[seWallet] != 0 {
		t.Fatalf("... beantwortet ihn nicht: %v", m)
	}
	f.erneuern(seWallet, seRegZeit+22*seTag) // ... eine danach schon.
	if !staffelErneuertGueltig(acct(f.cs, seWallet)) {
		t.Fatal("neuere Pruefung zaehlt nicht")
	}
	if m := f.runde(seRegZeit + 23*seTag); m[seWallet] != grantStaffelTagesrate() {
		t.Fatalf("Tagesrate nach neuerer Pruefung: %v", m)
	}
	rest := acct(f.cs, seWallet).GrantStagedRest.Float()
	if m := f.runde(seRegZeit + 30*seTag); round6(m[seWallet]) != round6(rest) {
		t.Fatalf("Tag 30 nach neuerer Pruefung: %v, Rest %.6f", m, rest)
	}
}

// Missbrauch: nichts davon wird angewandt (auch im Beobachtungsmodus), jeder
// Fall wird gemeldet.
func TestStaffelEinspruch_Missbrauch(t *testing.T) {
	issued := seRegZeit + 20*seTag
	block := issued + 60
	_, fremd, _ := ed25519.GenerateKey(rand.Reader)
	faelle := []struct {
		name  string
		tx    func(f *seFall) Transaction
		regel string
		vorab func(f *seFall)
	}{
		{"ein Betreiber allein", func(f *seFall) Transaction { return f.einspruch(seWallet, issued, seGrund, 0) }, "einspruch_form", nil},
		{"ein Betreiber zweimal", func(f *seFall) Transaction { return f.einspruch(seWallet, issued, seGrund, 0, 0) }, "einspruch_quorum", nil},
		{"zwei Schluessel desselben Menschen", func(f *seFall) Transaction {
			tx := f.einspruch(seWallet, issued, seGrund, 0, 1)
			// Bescheinigung 2 behauptet, zu Betreiber 0 zu gehoeren: die
			// Bindung passt nicht zum Schluessel.
			b := &tx.Einspruch.Bescheinigungen[1]
			b.Mensch, b.MenschSig = f.betreiber[0].bindung.Mensch, f.betreiber[0].bindung.MenschSig
			return tx
		}, "einspruch_ohne_bescheinigung", nil},
		{"gefaelschte Unterschrift", func(f *seFall) Transaction {
			tx := f.einspruch(seWallet, issued, seGrund, 0, 1)
			tx.Einspruch.Bescheinigungen[1].Signature = hex.EncodeToString(ed25519.Sign(fremd, []byte(staffelEinspruchNachricht(seGrund)(seWallet, issued))))
			return tx
		}, "einspruch_ohne_bescheinigung", nil},
		{"Grund nachtraeglich getauscht", func(f *seFall) Transaction {
			tx := f.einspruch(seWallet, issued, seGrund, 0, 1)
			tx.Einspruch.Grund = "Ein ganz anderer Grund, den niemand unterschrieben hat"
			return tx
		}, "einspruch_ohne_bescheinigung", nil},
		{"fremde Wallet", func(f *seFall) Transaction {
			tx := f.einspruch(seAndere, issued, seGrund, 0, 1)
			tx.Wallet = seWallet
			return tx
		}, "einspruch_ohne_bescheinigung", nil},
		{"Lebensnachweis-Unterschrift als Einspruch", func(f *seFall) Transaction {
			tx := f.einspruch(seWallet, issued, seGrund, 0, 1)
			for i := range tx.Einspruch.Bescheinigungen {
				tx.Einspruch.Bescheinigungen[i].Signature = hex.EncodeToString(ed25519.Sign(f.betreiber[i].priv, []byte(lebensnachweisNachricht(seWallet, issued))))
			}
			return tx
		}, "einspruch_ohne_bescheinigung", nil},
		{"Betreiber nicht zugelassen", func(f *seFall) Transaction { return f.einspruch(seWallet, issued, seGrund, 0, 1) },
			"einspruch_ohne_bescheinigung", func(f *seFall) { f.zugelassen[f.betreiber[1].bindung.Mensch] = false }},
		{"Betreiber mit offener Staffel", func(f *seFall) Transaction { return f.einspruch(seWallet, issued, seGrund, 0, 1) },
			"einspruch_ohne_bescheinigung", func(f *seFall) { acct(f.cs, f.betreiber[0].bindung.Mensch).GrantStagedRest = NewDecimal(800) }},
		{"nach Tag 30 bescheinigt", func(f *seFall) Transaction { return f.einspruch(seWallet, seRegZeit+30*seTag, seGrund, 0, 1) },
			"einspruch_zu_spaet", nil},
		{"aus der Zukunft des Blocks", func(f *seFall) Transaction {
			return f.einspruch(seWallet, block+nachweisHoechstensVoraus+1, seGrund, 0, 1)
		}, "einspruch_zeit", nil},
		{"aelter als 7 Tage", func(f *seFall) Transaction { return f.einspruch(seWallet, block-8*seTag, seGrund, 0, 1) },
			"einspruch_zeit", nil},
		{"ohne Staffel", func(f *seFall) Transaction {
			return f.einspruch(f.betreiber[2].bindung.Mensch, issued, seGrund, 0, 1)
		}, "einspruch_ohne_staffel", nil},
		{"Grund zu kurz", func(f *seFall) Transaction { return f.einspruch(seWallet, issued, "kurz", 0, 1) }, "einspruch_form", nil},
		{"Grund mit Steuerzeichen", func(f *seFall) Transaction {
			return f.einspruch(seWallet, issued, "Zeile eins\nZeile zwei ist versteckt", 0, 1)
		}, "einspruch_form", nil},
		{"Grund zu lang", func(f *seFall) Transaction {
			return f.einspruch(seWallet, issued, strings.Repeat("x", staffelEinspruchGrundMax+1), 0, 1)
		}, "einspruch_form", nil},
		{"zu viele Bescheinigungen", func(f *seFall) Transaction {
			return f.einspruch(seWallet, issued, seGrund, 0, 1, 2, 0, 1, 2, 0, 1, 2)
		}, "einspruch_form", nil},
		{"ohne Einspruch", func(f *seFall) Transaction {
			return Transaction{Type: staffelEinspruchTxArt, Wallet: seWallet, DistributionAt: issued}
		}, "einspruch_form", nil},
		{"Wallet gross geschrieben", func(f *seFall) Transaction {
			tx := f.einspruch(seWallet, issued, seGrund, 0, 1)
			tx.Wallet = "0x" + strings.ToUpper(seWallet[2:])
			return tx
		}, "einspruch_form", nil},
	}
	// Ein Einspruch von Tag 30 kommt nur in einen Block ab Tag 30.
	blockFuer := map[string]int64{"nach Tag 30 bescheinigt": seRegZeit + 30*seTag + 60}
	for _, fall := range faelle {
		t.Run(fall.name, func(t *testing.T) {
			f := neuerSeFall(t)
			if fall.vorab != nil {
				fall.vorab(f)
			}
			bz := block
			if b, ok := blockFuer[fall.name]; ok {
				bz = b
			}
			befund, err := f.anwenden(fall.tx(f), bz)
			if err != nil {
				t.Fatalf("im Beobachtungsmodus kein Fehler erwartet: %v", err)
			}
			if befund[fall.regel] != 1 {
				t.Fatalf("Befund %v, erwartet %s", befund, fall.regel)
			}
			for _, w := range []string{seWallet, seAndere} {
				if acct(f.cs, w).StaffelEinspruchAm != 0 {
					t.Fatalf("%s: angewandt", w)
				}
			}
			if m := f.runde(seRegZeit + 30*seTag); m[seWallet] != 800 {
				t.Fatalf("Tag 30 trotz ungueltigem Einspruch angehalten: %v", m)
			}
		})
	}
}

// Nur der erste Einspruch zaehlt: ein spaeterer verlaengert nichts.
func TestStaffelEinspruch_NurDerErste(t *testing.T) {
	f := neuerSeFall(t)
	erster := seRegZeit + 10*seTag
	if b, err := f.anwenden(f.einspruch(seWallet, erster, seGrund, 0, 1), erster); err != nil || len(b) != 0 {
		t.Fatalf("erster: %v %v", b, err)
	}
	f.erneuern(seWallet, seRegZeit+15*seTag)
	zweiter := seRegZeit + 20*seTag
	if b, err := f.anwenden(f.einspruch(seWallet, zweiter, "Zweiter Versuch, die Staffel weiter anzuhalten", 1, 2), zweiter); err != nil || len(b) != 0 {
		t.Fatalf("zweiter: %v %v", b, err)
	}
	acc := acct(f.cs, seWallet)
	if acc.StaffelEinspruchAm != erster || !staffelErneuertGueltig(acc) {
		t.Fatalf("zweiter Einspruch hat etwas geaendert: %+v", acc)
	}
}

// Vor der Staffel: Leerlauf, kein Befund, und die Art ist unbekannt.
func TestStaffelEinspruch_LeerlaufVorStaffel(t *testing.T) {
	f := neuerSeFall(t)
	tx := f.einspruch(seWallet, seRegZeit+20*seTag, seGrund, 0, 1)
	stagedGrantActivationOverride.Store(0)
	if b, err := f.anwenden(tx, seRegZeit+20*seTag); err != nil || len(b) != 0 || acct(f.cs, seWallet).StaffelEinspruchAm != 0 {
		t.Fatalf("vor der Staffel: %v %v", b, err)
	}
	if bekannteTxArt(staffelEinspruchTxArt, seRegZeit) {
		t.Fatal("staffel_einspruch vor der Staffel bekannt")
	}
	stagedGrantActivationOverride.Store(1)
	if !bekannteTxArt(staffelEinspruchTxArt, seRegZeit) {
		t.Fatal("staffel_einspruch nach dem Stichtag unbekannt")
	}
}

// Jedes Konto ohne Einspruch behaelt seinen Blattwert; mit Einspruch aendert
// er sich (sonst waere er kein Konsenszustand).
func TestStaffelEinspruch_BlattNurWennGesetzt(t *testing.T) {
	acc := &AccountState{Address: seWallet, Balance: NewDecimal(200), IsHuman: true,
		GrantStagedRest: NewDecimal(800), GrantStagedUntil: seRegZeit + 30*seTag}
	vorher := accountLeaf(acc)
	acc.StaffelEinspruchAm = 0
	if accountLeaf(acc) != vorher {
		t.Fatal("Blatt ohne Einspruch hat sich geaendert")
	}
	acc.StaffelEinspruchAm = seRegZeit + 20*seTag
	if accountLeaf(acc) == vorher {
		t.Fatal("Einspruch muss ins Blatt eingehen")
	}
}

// Die Rechnung der Freigabe kennt kein Zurueckhalten ohne Ende: fuer jeden
// Zustand ist spaetestens an Tag 180 alles frei.
func TestStaffel_KeinZurueckhaltenOhneEnde(t *testing.T) {
	until := seRegZeit + 30*seTag
	for _, einspruch := range []int64{0, seRegZeit + 1, until - 1} {
		for _, erneuert := range []int64{0, seRegZeit + 7*seTag, until - 2} {
			acc := &AccountState{IsHuman: true, GrantStagedRest: NewDecimal(800), GrantStagedUntil: until,
				StaffelEinspruchAm: einspruch, LivenessRenewedAt: erneuert}
			if b, _ := staffelFreigabeHoechstens(acc, seRegZeit+180*seTag); b != 800 {
				t.Fatalf("Einspruch %d, Pruefung %d: an Tag 180 frei %.6f", einspruch, erneuert, b)
			}
		}
	}
}

// Im strengen Modus lehnt ein ungueltiger Einspruch den Block ab (Fehler),
// ein gueltiger geht durch.
func TestStaffelEinspruch_StrengLehntAb(t *testing.T) {
	f := neuerSeFall(t)
	streng := nachrechnenStrengAbUnix + 100*seTag
	// Staffel neu anlegen, mit Tag 30 nach dem strengen Stichtag.
	w := "0x00000000000000000000000000000000000000f9"
	f.cs.mu.Lock()
	if err := f.cs.registerHumanMitKlasseLocked(context.Background(), w, streng-10*seTag, "gestaffelt"); err != nil {
		f.cs.mu.Unlock()
		t.Fatal(err)
	}
	f.cs.mu.Unlock()
	if _, err := f.anwenden(f.einspruch(w, streng, seGrund, 0), streng); err == nil {
		t.Fatal("ein Betreiber allein: im strengen Modus kein Fehler")
	}
	if acct(f.cs, w).StaffelEinspruchAm != 0 {
		t.Fatal("ungueltiger Einspruch angewandt")
	}
	if b, err := f.anwenden(f.einspruch(w, streng, seGrund, 0, 1), streng); err != nil || len(b) != 0 {
		t.Fatalf("gueltiger Einspruch im strengen Modus: %v %v", b, err)
	}
	if acct(f.cs, w).StaffelEinspruchAm != streng {
		t.Fatal("gueltiger Einspruch nicht angewandt")
	}
}

// Gegen eine echte Datenbank: die Spalte staffel_einspruch_am wird einzeln
// und im Stapel geschrieben, kalt geladen und beim Neuaufbau der StateRoot
// mitgezaehlt; und die Tagesrunde laesst genau die Wallets aus, deren
// Erneuerung oder Einspruch noch in keinem Block steht.
func TestStaffelEinspruch_SpalteUndAusgang_RealDB(t *testing.T) {
	truncateDistTestTables(t)
	cs := testKnoten(t, "unused-staffel-einspruch-test.json")
	if !cs.useDB {
		t.Fatal("erwartet eine echte PostgreSQL-Verbindung")
	}
	vorher := stagedGrantActivationOverride.Load()
	stagedGrantActivationOverride.Store(1)
	t.Cleanup(func() { stagedGrantActivationOverride.Store(vorher) })
	ctx := context.Background()
	until := seRegZeit + 30*seTag
	a, b, c := distTestAddr(2101), distTestAddr(2102), distTestAddr(2103)
	konto := func(w string, einspruch int64) *AccountState {
		return &AccountState{Address: w, IsHuman: true, Balance: NewDecimal(200), GrantStagedRest: NewDecimal(800),
			GrantStagedUntil: until, StaffelEinspruchAm: einspruch}
	}
	cs.mu.Lock()
	ka, kb := konto(a, 0), konto(b, 0)
	cs.accounts.Set(a, ka)
	cs.accounts.Set(b, kb)
	if err := cs.saveAccountToDB(ka); err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	if err := cs.saveAccountToDB(kb); err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	kc := konto(c, seRegZeit+20*seTag)
	cs.accounts.Set(c, kc)
	if err := cs.saveAccountsToDBBatchCtx(ctx, []*AccountState{kc}); err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	// Einzeln geschrieben, dann geaendert und noch einmal einzeln geschrieben.
	ka.StaffelEinspruchAm = seRegZeit + 21*seTag
	if err := cs.saveAccountToDB(ka); err != nil {
		cs.mu.Unlock()
		t.Fatal(err)
	}
	cs.mu.Unlock()
	for w, want := range map[string]int64{a: seRegZeit + 21*seTag, b: 0, c: seRegZeit + 20*seTag} {
		var got int64
		if err := cs.db.QueryRow(`SELECT staffel_einspruch_am FROM chain_accounts WHERE lower(address) = $1`, w).Scan(&got); err != nil || got != want {
			t.Fatalf("%s: Spalte %d (%v), erwartet %d", w, got, err, want)
		}
	}
	// Kalt laden, einzeln und im Stapel.
	cs.mu.Lock()
	cs.accounts.Delete(a)
	cs.accounts.Delete(c)
	cs.ensureAccountLoadedCtx(ctx, a)
	cs.ensureAccountsLoadedCtx(ctx, []string{c})
	ga, gc := acct(cs, a), acct(cs, c)
	cs.mu.Unlock()
	if ga == nil || ga.StaffelEinspruchAm != seRegZeit+21*seTag || gc == nil || gc.StaffelEinspruchAm != seRegZeit+20*seTag {
		t.Fatalf("kalt geladen: %+v / %+v", ga, gc)
	}
	// Neuaufbau aus der Datenbank == Speicher.
	vorherRoot := cs.StateRootComponentBreakdown()
	cs.mu.Lock()
	cs.rebuildStateAccumulators()
	cs.mu.Unlock()
	if nachher := cs.StateRootComponentBreakdown(); vorherRoot.AccountSetXOR != nachher.AccountSetXOR {
		t.Fatalf("account_set_xor vor/nach Neuaufbau verschieden: %s / %s", vorherRoot.AccountSetXOR, nachher.AccountSetXOR)
	}

	// Ausgang: b hat eine angewandte Erneuerung, die in keinem Block steht.
	// a und c sind angehalten (Einspruch, keine neuere Pruefung).
	if err := savePendingTxExec(cs.db, Transaction{Type: "liveness_renewal", Wallet: b, DistributionAt: until - seTag}); err != nil {
		t.Fatal(err)
	}
	cs.mu.Lock()
	txs, err := cs.grantReleasesLocked(ctx, until)
	cs.mu.Unlock()
	if err != nil || len(txs) != 0 {
		t.Fatalf("Runde an Tag 30 mit offener Erneuerung fuer b, a und c angehalten: %v %v", txs, err)
	}
	if _, err := cs.db.Exec(`UPDATE pending_txs SET included_at = 1 WHERE (tx_json::jsonb ->> 'type') = 'liveness_renewal'`); err != nil {
		t.Fatal(err)
	}
	cs.mu.Lock()
	txs, err = cs.grantReleasesLocked(ctx, until+seTag)
	cs.mu.Unlock()
	if err != nil || len(txs) != 1 || txs[0].Wallet != b || txs[0].Amount != 800 {
		t.Fatalf("Runde danach: %v %v", txs, err)
	}
}
