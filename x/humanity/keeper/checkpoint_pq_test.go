package keeper

import (
	"crypto/ecdsa"
	"crypto/mldsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// Stufe 5 (checkpoint_pq.go): Gutfall, Faelschungen, schlafend.

const testPQAb int64 = 1_700_000_000 // vor testCheckpointFuer (1_800_000_000, ein Anker-Slot)

func testPQSchluessel(t *testing.T) *mldsa.PrivateKey {
	t.Helper()
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	sk, err := mldsa.NewPrivateKey(mldsa.MLDSA65(), seed)
	if err != nil {
		t.Fatal(err)
	}
	return sk
}

func pqHex(sk *mldsa.PrivateKey) string { return hex.EncodeToString(sk.PublicKey().Bytes()) }

func cpAdresse(k *ecdsa.PrivateKey) string {
	return strings.ToLower(crypto.PubkeyToAddress(k.PublicKey).Hex())
}

// hybridesZertifikat: Zertifikat der ersten n Schluessel samt PQ-Eintraegen
// in der Reihenfolge der (sortierten) Unterschriften.
func hybridesZertifikat(t *testing.T, c KettenCheckpoint, keys []*ecdsa.PrivateKey, pq map[string]*mldsa.PrivateKey) (CheckpointZertifikat, []string) {
	t.Helper()
	z := zertifikatVon(t, c, keys)
	var eintraege []string
	for _, u := range z.Unterschriften {
		s, err := checkpointPQUnterschreiben(c, pq[u.Von])
		if err != nil {
			t.Fatal(err)
		}
		eintraege = append(eintraege, s)
	}
	return z, eintraege
}

func TestCheckpointPQ_GutfallUndFaelschungen(t *testing.T) {
	keys, komitee := testKomiteeSchluessel(t, 4)
	pq := map[string]*mldsa.PrivateKey{}
	register := map[string]string{}
	for _, k := range keys {
		a := cpAdresse(k)
		pq[a] = testPQSchluessel(t)
		register[a] = pqHex(pq[a])
	}
	c := testCheckpointFuer(komitee)
	if !checkpointPQAnkerAb(c, testPQAb) {
		t.Fatal("Testzeit ist kein Anker-Slot")
	}
	z, eintraege := hybridesZertifikat(t, c, keys[:3], pq)
	if err := pruefeZertifikat(z, komitee); err != nil {
		t.Fatal(err)
	}
	if err := pruefeZertifikatPQAb(z, eintraege, register, testPQAb); err != nil {
		t.Fatalf("Gutfall abgelehnt: %v", err)
	}

	kopie := func() []string { return append([]string(nil), eintraege...) }
	angriff := func(name string, e []string, reg map[string]string) {
		t.Helper()
		if pruefeZertifikatPQAb(z, e, reg, testPQAb) == nil {
			t.Errorf("Angriff ging durch: %s", name)
		}
	}

	ohne := kopie()
	ohne[1] = ""
	angriff("Anker ohne PQ-Unterschrift eines Mitglieds", ohne, register)

	fremd := kopie()
	fremd[0], _ = checkpointPQUnterschreiben(c, testPQSchluessel(t))
	angriff("PQ-Unterschrift eines fremden Schluessels", fremd, register)

	andererCP := c
	andererCP.Hoehe++
	wiederholt := kopie()
	wiederholt[0], _ = checkpointPQUnterschreiben(andererCP, pq[z.Unterschriften[0].Von])
	angriff("PQ-Unterschrift eines anderen Checkpoints (Wiederholung)", wiederholt, register)

	gekippt := kopie()
	b := []byte(gekippt[2])
	if b[100] == 'a' {
		b[100] = 'b'
	} else {
		b[100] = 'a'
	}
	gekippt[2] = string(b)
	angriff("ein Zeichen der PQ-Unterschrift geaendert", gekippt, register)

	angriff("abgeschnitten", append(kopie()[:2], eintraege[2][:len(eintraege[2])-2]), register)
	angriff("PQ-Eintraege fehlen ganz", kopie()[:2], register)
	angriff("ein PQ-Eintrag zu viel", append(kopie(), eintraege[0]), register)

	ohneSchluessel := map[string]string{}
	for a, h := range register {
		ohneSchluessel[a] = h
	}
	delete(ohneSchluessel, z.Unterschriften[1].Von)
	angriff("Mitglied ohne eingetragenen PQ-Schluessel (fail-closed)", eintraege, ohneSchluessel)

	vertauscht := map[string]string{}
	for a, h := range register {
		vertauscht[a] = h
	}
	vertauscht[z.Unterschriften[0].Von], vertauscht[z.Unterschriften[1].Von] =
		vertauscht[z.Unterschriften[1].Von], vertauscht[z.Unterschriften[0].Von]
	angriff("PQ-Schluessel zweier Mitglieder vertauscht", eintraege, vertauscht)

	kaputt := map[string]string{}
	for a, h := range register {
		kaputt[a] = h
	}
	kaputt[z.Unterschriften[0].Von] = "00"
	angriff("eingetragener PQ-Schluessel in falscher Form", eintraege, kaputt)
}

// Zwischen den Ankern darf PQ fehlen; ist sie da, muss die Form stimmen.
func TestCheckpointPQ_ZwischenAnkern(t *testing.T) {
	keys, komitee := testKomiteeSchluessel(t, 4)
	c := testCheckpointFuer(komitee)
	c.Zeit += checkpointSlotLaenge
	c.Epoche = c.Zeit / epochLength
	if checkpointPQAnkerAb(c, testPQAb) {
		t.Fatal("naechster Slot ist ein Anker")
	}
	z := zertifikatVon(t, c, keys[:3])
	if err := pruefeZertifikatPQAb(z, []string{"", "", ""}, nil, testPQAb); err != nil {
		t.Fatalf("Minuten-Checkpoint ohne PQ abgelehnt: %v", err)
	}
	if pruefeZertifikatPQAb(z, []string{"", strings.Repeat("zz", pqSignaturHex/2), ""}, nil, testPQAb) == nil {
		t.Fatal("PQ-Eintrag in falscher Form angenommen")
	}
	// Jeder checkpointPQJederSlot-te Slot ist ein Anker, die anderen nicht.
	anker := 0
	for i := int64(0); i < 10*checkpointPQJederSlot; i++ {
		d := testCheckpointFuer(komitee)
		d.Zeit += i * checkpointSlotLaenge
		if checkpointPQAnkerAb(d, testPQAb) {
			anker++
		}
	}
	if anker != 10 {
		t.Fatalf("%d Anker in %d Slots", anker, 10*checkpointPQJederSlot)
	}
}

// Vor dem Stichtag: kein Anker, die PQ-Pruefung haelt nichts an.
func TestCheckpointPQ_Schlafend(t *testing.T) {
	if checkpointPQAbUnix != math.MaxInt64 {
		t.Fatal("Stufe 5 ist eingeschaltet -- nur in einem eigenen, geprueften PR")
	}
	keys, komitee := testKomiteeSchluessel(t, 4)
	c := testCheckpointFuer(komitee)
	if checkpointPQAnker(c) {
		t.Fatal("Anker vor dem Stichtag")
	}
	z := zertifikatVon(t, c, keys[:3])
	if err := pruefeZertifikatPQ(z, []string{"", "", ""}, nil); err != nil {
		t.Fatalf("schlafend, aber abgelehnt: %v", err)
	}
}

func bindungUnterschreiben(t *testing.T, k *ecdsa.PrivateKey, adresse string, pqPub string, pqSk *mldsa.PrivateKey) (string, string) {
	t.Helper()
	n := pqErzeugerBindungNachricht(adresse, pqPub)
	h := sha256.Sum256(n)
	s, err := crypto.Sign(h[:], k)
	if err != nil {
		t.Fatal(err)
	}
	p, err := pqSk.SignDeterministic(n, nil)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(s), hex.EncodeToString(p)
}

func TestCheckpointPQ_ErzeugerBindung(t *testing.T) {
	k, _ := crypto.GenerateKey()
	a := cpAdresse(k)
	sk := testPQSchluessel(t)
	pub := pqHex(sk)
	s, p := bindungUnterschreiben(t, k, a, pub, sk)
	if err := pruefePQErzeugerBindung(a, pub, s, p); err != nil {
		t.Fatalf("Gutfall abgelehnt: %v", err)
	}

	angriff := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Errorf("Angriff ging durch: %s", name)
		}
	}
	// Fremder secp256k1-Schluessel traegt einen PQ-Schluessel fuer a ein.
	dieb, _ := crypto.GenerateKey()
	s2, p2 := bindungUnterschreiben(t, dieb, a, pub, sk)
	angriff("secp256k1-Unterschrift eines Fremden", pruefePQErzeugerBindung(a, pub, s2, p2))
	// Besitz des PQ-Schluessels nicht bewiesen: PQ-Unterschrift eines anderen.
	anderer := testPQSchluessel(t)
	s3, p3 := bindungUnterschreiben(t, k, a, pub, anderer)
	angriff("PQ-Unterschrift eines anderen Schluessels", pruefePQErzeugerBindung(a, pub, s3, p3))
	// Unterschriften fuer Schluessel A, eingereicht mit Schluessel B.
	angriff("Schluessel nach dem Unterschreiben ausgetauscht", pruefePQErzeugerBindung(a, pqHex(anderer), s, p))
	// Unterschriften fuer Adresse a, eingereicht fuer eine andere Adresse.
	angriff("andere Adresse", pruefePQErzeugerBindung(cpAdresse(dieb), pub, s, p))
	angriff("Adresse nicht kanonisch", pruefePQErzeugerBindung(strings.ToUpper(a[:4])+a[4:], pub, s, p))
	angriff("secp256k1-Unterschrift abgeschnitten", pruefePQErzeugerBindung(a, pub, s[:128], p))
	angriff("PQ-Unterschrift abgeschnitten", pruefePQErzeugerBindung(a, pub, s, p[:len(p)-2]))
	angriff("PQ-Schluessel in falscher Form", pruefePQErzeugerBindung(a, pub[:100], s, p))
}
