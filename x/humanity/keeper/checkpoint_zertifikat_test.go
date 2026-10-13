package keeper

import (
	"crypto/ecdsa"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
)

func checkpointKomitee(t *testing.T, n int) ([]*ecdsa.PrivateKey, map[string]bool) {
	t.Helper()
	var keys []*ecdsa.PrivateKey
	komitee := map[string]bool{}
	for i := 0; i < n; i++ {
		k, err := crypto.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
		komitee[strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())] = true
	}
	return keys, komitee
}

func testCheckpointFuer(komitee map[string]bool) KettenCheckpoint {
	return neuerCheckpoint(1234, strings.Repeat("ab", 32), 1_800_000_000, komitee)
}

func zertifikatVon(t *testing.T, c KettenCheckpoint, keys []*ecdsa.PrivateKey) CheckpointZertifikat {
	t.Helper()
	z := CheckpointZertifikat{KettenCheckpoint: c}
	for _, k := range keys {
		u, err := checkpointUnterschreiben(c, k)
		if err != nil {
			t.Fatal(err)
		}
		z.Unterschriften = append(z.Unterschriften, u)
	}
	zertifikatOrdnen(&z)
	return z
}

func TestCheckpointZertifikat_Quorum(t *testing.T) {
	for _, f := range []struct{ n, ja, nein int }{{4, 3, 2}, {6, 5, 4}, {7, 5, 4}, {10, 7, 6}} {
		keys, komitee := checkpointKomitee(t, f.n)
		c := testCheckpointFuer(komitee)
		if err := pruefeZertifikat(zertifikatVon(t, c, keys[:f.ja]), komitee); err != nil {
			t.Fatalf("%d von %d: %v", f.ja, f.n, err)
		}
		if pruefeZertifikat(zertifikatVon(t, c, keys[:f.nein]), komitee) == nil {
			t.Fatalf("%d von %d bestaetigt", f.nein, f.n)
		}
	}
	for n, q := range map[int]int{0: 1, 1: 1, 2: 2, 3: 3, 4: 3, 6: 5, 100: 67} {
		if got := checkpointQuorum(n); got != q {
			t.Fatalf("Quorum(%d) = %d, erwartet %d", n, got, q)
		}
	}
}

// Kleine Komitees bestaetigen nichts -- sonst ist ein einzelner Schluessel
// (oder zwei) das Quorum, und wer die Zeit waehlt, waehlt eine Epoche, in
// der er (fast) allein war.
func TestCheckpointZertifikat_KleinesKomitee(t *testing.T) {
	for n := 0; n < checkpointMindestKomitee; n++ {
		keys, komitee := checkpointKomitee(t, n)
		if pruefeZertifikat(zertifikatVon(t, testCheckpointFuer(komitee), keys), komitee) == nil {
			t.Fatalf("Komitee mit %d Mitgliedern hat bestaetigt", n)
		}
	}
}

// Missbrauch: jede Faelschung, Wiederholung und fremde Unterschrift.
func TestCheckpointZertifikat_Faelschungen(t *testing.T) {
	keys, komitee := checkpointKomitee(t, 4)
	fremd, _ := checkpointKomitee(t, 1)
	c := testCheckpointFuer(komitee)
	ok := zertifikatVon(t, c, keys[:3])
	kopie := func() CheckpointZertifikat {
		z := ok
		z.Unterschriften = append([]CheckpointUnterschrift(nil), ok.Unterschriften...)
		return z
	}

	doppelt := kopie()
	doppelt.Unterschriften = append(doppelt.Unterschriften, doppelt.Unterschriften[2])
	unsortiert := kopie()
	unsortiert.Unterschriften[0], unsortiert.Unterschriften[1] = unsortiert.Unterschriften[1], unsortiert.Unterschriften[0]
	fremder := zertifikatVon(t, c, append(keys[:2:2], fremd[0]))
	vertauscht := kopie()
	vertauscht.Unterschriften[0].Sig, vertauscht.Unterschriften[1].Sig = vertauscht.Unterschriften[1].Sig, vertauscht.Unterschriften[0].Sig

	andererBlock := kopie()
	andererBlock.Hash = strings.Repeat("cd", 32)
	andereHoehe := kopie()
	andereHoehe.Hoehe++
	andereZeit := kopie()
	andereZeit.Zeit++ // gleiche Epoche, anderer Inhalt
	andereEpoche := kopie()
	andereEpoche.Zeit += epochLength
	andereEpoche.Epoche++
	falscheEpoche := kopie()
	falscheEpoche.Epoche++
	anderesKomitee := kopie()
	anderesKomitee.Komitee = strings.Repeat("00", 32)

	unlesbar := kopie()
	unlesbar.Unterschriften[2].Sig = "zz"
	ueberlang := kopie()
	ueberlang.Unterschriften[2].Sig += strings.Repeat("0", 1<<16)
	mit0x := kopie()
	mit0x.Unterschriften[2].Sig = "0x" + mit0x.Unterschriften[2].Sig
	gross := kopie()
	gross.Unterschriften[2].Sig = strings.ToUpper(gross.Unterschriften[2].Sig)
	vonGross := kopie()
	vonGross.Unterschriften[2].Von = "0x" + strings.ToUpper(vonGross.Unterschriften[2].Von[2:])
	hashGross := kopie()
	hashGross.Hash = strings.ToUpper(hashGross.Hash)

	// Zweite Schreibweise derselben Unterschrift: (r, n-s, v^1) und v+27.
	hochS := kopie()
	hochS.Unterschriften[2].Sig = hochSVariante(t, hochS.Unterschriften[2].Sig)
	v27 := kopie()
	v27.Unterschriften[2].Sig = vPlus27(t, v27.Unterschriften[2].Sig)

	zuViele := zertifikatVon(t, c, keys)
	zuViele.Unterschriften = append(zuViele.Unterschriften, CheckpointUnterschrift{Von: "0x" + strings.Repeat("f", 40)})

	for name, z := range map[string]CheckpointZertifikat{
		"doppelt": doppelt, "unsortiert": unsortiert, "fremder": fremder, "vertauscht": vertauscht,
		"anderer Block": andererBlock, "andere Hoehe": andereHoehe, "andere Zeit": andereZeit,
		"andere Epoche": andereEpoche, "Epoche passt nicht zur Zeit": falscheEpoche, "anderes Komitee": anderesKomitee,
		"unlesbar": unlesbar, "ueberlang": ueberlang, "mit 0x": mit0x, "Sig gross": gross, "Von gross": vonGross,
		"Hash gross": hashGross, "high-s": hochS, "v=27": v27, "zu viele": zuViele,
	} {
		if pruefeZertifikat(z, komitee) == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
	// Ein Knoten mit einer anderen Sicht auf das Komitee urteilt nicht
	// still anders, sondern lehnt ab (Komitee-Hash).
	_, anderes := checkpointKomitee(t, 4)
	if pruefeZertifikat(ok, anderes) == nil {
		t.Fatal("andere Komitee-Sicht hat bestaetigt")
	}
	if err := pruefeZertifikat(ok, komitee); err != nil {
		t.Fatalf("Gutfall abgewiesen: %v", err)
	}
}

func hochSVariante(t *testing.T, s string) string {
	t.Helper()
	sig, _ := hex.DecodeString(s)
	n := crypto.S256().Params().N
	sv := new(big.Int).SetBytes(sig[32:64])
	sv.Sub(n, sv)
	b := sv.FillBytes(make([]byte, 32))
	copy(sig[32:64], b)
	sig[64] ^= 1
	return hex.EncodeToString(sig)
}

func vPlus27(t *testing.T, s string) string {
	t.Helper()
	sig, _ := hex.DecodeString(s)
	sig[64] += 27
	return hex.EncodeToString(sig)
}

// Die Unterschrift gilt nur fuer diese Kette und diesen Zweck.
func TestCheckpointZertifikat_Domaene(t *testing.T) {
	if got := "0x" + big.NewInt(checkpointChainID).Text(16); got != chainIDHex {
		t.Fatalf("Chain-ID %s passt nicht zu %s", got, chainIDHex)
	}
	keys, komitee := checkpointKomitee(t, 4)
	c := testCheckpointFuer(komitee)
	mitSig := func(sign func(*ecdsa.PrivateKey) []byte) CheckpointZertifikat {
		z := CheckpointZertifikat{KettenCheckpoint: c}
		for _, k := range keys[:3] {
			z.Unterschriften = append(z.Unterschriften, CheckpointUnterschrift{
				Von: strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex()), Sig: hex.EncodeToString(sign(k))})
		}
		zertifikatOrdnen(&z)
		return z
	}
	for name, sign := range map[string]func(*ecdsa.PrivateKey) []byte{
		"Blocksignatur ueber den Hash": func(k *ecdsa.PrivateKey) []byte {
			h, _ := hex.DecodeString(c.Hash)
			s, _ := crypto.Sign(h, k)
			return s
		},
		"EIP-191 ueber denselben Text": func(k *ecdsa.PrivateKey) []byte {
			s, _ := crypto.Sign(accounts.TextHash(c.signierHash()), k)
			return s
		},
		"andere Chain-ID": func(k *ecdsa.PrivateKey) []byte {
			s, _ := crypto.Sign(c.signierHashFuer(checkpointChainID+1), k)
			return s
		},
		"Keccak statt sha256": func(k *ecdsa.PrivateKey) []byte {
			s, _ := crypto.Sign(crypto.Keccak256(c.signierHash()), k)
			return s
		},
	} {
		if pruefeZertifikat(mitSig(sign), komitee) == nil {
			t.Fatalf("%s als Checkpoint-Unterschrift angenommen", name)
		}
	}
	if pruefeZertifikat(mitSig(func(k *ecdsa.PrivateKey) []byte {
		s, _ := crypto.Sign(c.signierHash(), k)
		return s
	}), komitee) != nil {
		t.Fatal("Gegenprobe: echte Unterschriften abgewiesen")
	}
	if _, err := checkpointUnterschreiben(KettenCheckpoint{Hoehe: 1, Hash: "x", Zeit: 1}, keys[0]); err == nil {
		t.Fatal("ungueltigen Checkpoint unterschrieben")
	}
}

// Was vor der ersten Recovery abgewiesen wird, kostet keine Kryptographie:
// zu wenige Unterschriften, Nicht-Mitglieder, falsche Laenge.
func TestCheckpointZertifikat_BilligVorTeuer(t *testing.T) {
	keys, komitee := checkpointKomitee(t, 10)
	c := testCheckpointFuer(komitee)
	// 9 Mitglieder mit Muell-Unterschriften (richtige Form): die Form ist
	// kanonisch, also erst die Recovery entscheidet -- aber 6 Eintraege
	// (unter dem Quorum 7) werden vorher abgewiesen.
	z := CheckpointZertifikat{KettenCheckpoint: c}
	for _, k := range keys[:6] {
		z.Unterschriften = append(z.Unterschriften, CheckpointUnterschrift{
			Von: strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex()), Sig: strings.Repeat("0", 130)})
	}
	zertifikatOrdnen(&z)
	if err := pruefeZertifikat(z, komitee); err == nil || !strings.Contains(err.Error(), "noetig") {
		t.Fatalf("unter dem Quorum nicht vor der Kryptographie abgewiesen: %v", err)
	}
}

// Formfehler, die mit dazu passenden echten Unterschriften kommen: nur die
// Formpruefung haelt sie auf (nicht erst die Recovery).
func TestCheckpointZertifikat_FormfehlerMitEchtenUnterschriften(t *testing.T) {
	keys, komitee := checkpointKomitee(t, 4)
	roh := func(c KettenCheckpoint) CheckpointZertifikat {
		z := CheckpointZertifikat{KettenCheckpoint: c}
		for _, k := range keys[:3] {
			s, _ := crypto.Sign(c.signierHash(), k)
			z.Unterschriften = append(z.Unterschriften, CheckpointUnterschrift{
				Von: strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex()), Sig: hex.EncodeToString(s)})
		}
		zertifikatOrdnen(&z)
		return z
	}
	c := testCheckpointFuer(komitee)
	falscheEpoche := c
	falscheEpoche.Epoche++
	hashGross := c
	hashGross.Hash = strings.ToUpper(c.Hash)
	for name, z := range map[string]CheckpointZertifikat{
		"Epoche passt nicht zur Zeit": roh(falscheEpoche), "Hash gross": roh(hashGross),
	} {
		if pruefeZertifikat(z, komitee) == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
	if pruefeZertifikat(roh(c), komitee) != nil {
		t.Fatal("Gegenprobe abgewiesen")
	}
}

// Ein Knoten mit einer groesseren Sicht auf das Komitee (ein Mitglied mehr)
// lehnt ab, auch wenn alle Unterschreibenden auch bei ihm Mitglieder sind und
// das Quorum erreicht ist: das Zertifikat gilt fuer genau eine Menge.
func TestCheckpointZertifikat_AndereSichtLehntAb(t *testing.T) {
	keys, komitee := checkpointKomitee(t, 4)
	z := zertifikatVon(t, testCheckpointFuer(komitee), keys)
	_, extra := checkpointKomitee(t, 1)
	groesser := map[string]bool{}
	for a := range komitee {
		groesser[a] = true
	}
	for a := range extra {
		groesser[a] = true
	}
	if pruefeZertifikat(z, groesser) == nil {
		t.Fatal("andere Komitee-Sicht hat bestaetigt")
	}
	if pruefeZertifikat(z, komitee) != nil {
		t.Fatal("Gegenprobe abgewiesen")
	}
}
