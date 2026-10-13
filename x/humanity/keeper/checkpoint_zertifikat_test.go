package keeper

import (
	"crypto/ecdsa"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"

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

var testCheckpoint = KettenCheckpoint{Hoehe: 1234, Hash: strings.Repeat("ab", 32), Zeit: 1_800_000_000}

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
	return z
}

func TestCheckpointZertifikat_Quorum(t *testing.T) {
	keys, komitee := checkpointKomitee(t, 4)
	// 3 von 4 sind mehr als 2/3.
	if err := pruefeZertifikat(zertifikatVon(t, testCheckpoint, keys[:3]), komitee); err != nil {
		t.Fatalf("3 von 4: %v", err)
	}
	// 2 von 4 nicht.
	if pruefeZertifikat(zertifikatVon(t, testCheckpoint, keys[:2]), komitee) == nil {
		t.Fatal("2 von 4 bestaetigt")
	}
	// Genau 2/3 reicht nicht: 2 von 3.
	keys3, komitee3 := checkpointKomitee(t, 3)
	if pruefeZertifikat(zertifikatVon(t, testCheckpoint, keys3[:2]), komitee3) == nil {
		t.Fatal("genau 2/3 bestaetigt")
	}
	if err := pruefeZertifikat(zertifikatVon(t, testCheckpoint, keys3), komitee3); err != nil {
		t.Fatalf("3 von 3: %v", err)
	}
	// Ein Validator allein: er selbst ist das Komitee.
	keys1, komitee1 := checkpointKomitee(t, 1)
	if err := pruefeZertifikat(zertifikatVon(t, testCheckpoint, keys1), komitee1); err != nil {
		t.Fatalf("1 von 1: %v", err)
	}
	for n, q := range map[int]int{1: 1, 2: 2, 3: 3, 4: 3, 6: 5, 100: 67} {
		if got := checkpointQuorum(n); got != q {
			t.Fatalf("Quorum(%d) = %d, erwartet %d", n, got, q)
		}
	}
}

// Missbrauch: jede Faelschung, Wiederholung und fremde Unterschrift.
func TestCheckpointZertifikat_Faelschungen(t *testing.T) {
	keys, komitee := checkpointKomitee(t, 4)
	fremd, _ := checkpointKomitee(t, 1)
	ok := zertifikatVon(t, testCheckpoint, keys[:3])

	// Quorum erreicht, aber ein Eintrag doppelt: das ganze Zertifikat faellt.
	doppelt := zertifikatVon(t, testCheckpoint, keys[:3])
	doppelt.Unterschriften = append(doppelt.Unterschriften, doppelt.Unterschriften[0])

	fremder := zertifikatVon(t, testCheckpoint, append(keys[:2:2], fremd[0]))

	// Unterschrift von A, aber als B ausgegeben.
	vertauscht := zertifikatVon(t, testCheckpoint, keys[:3])
	vertauscht.Unterschriften[0].Sig, vertauscht.Unterschriften[1].Sig = vertauscht.Unterschriften[1].Sig, vertauscht.Unterschriften[0].Sig

	// Unterschriften fuer einen anderen Block wiederverwendet.
	anderer := testCheckpoint
	anderer.Hash = strings.Repeat("cd", 32)
	wiederverwendet := ok
	wiederverwendet.KettenCheckpoint = anderer

	andereHoehe := ok
	andereHoehe.Hoehe++
	andereZeit := ok
	andereZeit.Zeit++

	kaputt := zertifikatVon(t, testCheckpoint, keys[:3])
	kaputt.Unterschriften[2].Sig = "zz"

	gross := ok
	gross.Hash = strings.ToUpper(ok.Hash)

	zuViele := zertifikatVon(t, testCheckpoint, keys)
	for i := 0; i < targetCommitteeSize; i++ {
		zuViele.Unterschriften = append(zuViele.Unterschriften, CheckpointUnterschrift{Von: "0x" + strconv.Itoa(i)})
	}

	for name, z := range map[string]CheckpointZertifikat{
		"doppelt": doppelt, "fremder": fremder, "vertauscht": vertauscht,
		"anderer Block": wiederverwendet, "andere Hoehe": andereHoehe, "andere Zeit": andereZeit,
		"unlesbar": kaputt, "Hash gross": gross, "zu viele": zuViele,
	} {
		if pruefeZertifikat(z, komitee) == nil {
			t.Fatalf("%s: angenommen", name)
		}
	}
	if pruefeZertifikat(ok, map[string]bool{}) == nil {
		t.Fatal("leeres Komitee hat bestaetigt")
	}
	if pruefeZertifikat(CheckpointZertifikat{KettenCheckpoint: testCheckpoint}, map[string]bool{}) == nil {
		t.Fatal("leeres Komitee ohne Unterschriften hat bestaetigt")
	}
	if pruefeZertifikat(ok, komitee) != nil {
		t.Fatal("Gutfall abgewiesen")
	}
}

// Die Unterschrift gilt nur fuer diese Kette und diesen Zweck.
func TestCheckpointZertifikat_Domaene(t *testing.T) {
	if strconv.FormatInt(checkpointChainID, 16) != strings.TrimPrefix(chainIDHex, "0x") {
		t.Fatalf("Chain-ID %d passt nicht zu %s", checkpointChainID, chainIDHex)
	}
	key, _ := crypto.GenerateKey()
	// Eine Blocksignatur (Signatur ueber den Blockhash selbst) ist keine
	// Checkpoint-Unterschrift.
	blockSig, _ := crypto.Sign(testCheckpointHashBytes(), key)
	addr := strings.ToLower(crypto.PubkeyToAddress(key.PublicKey).Hex())
	z := CheckpointZertifikat{KettenCheckpoint: testCheckpoint,
		Unterschriften: []CheckpointUnterschrift{{Von: addr, Sig: hex.EncodeToString(blockSig)}}}
	if pruefeZertifikat(z, map[string]bool{addr: true}) == nil {
		t.Fatal("Blocksignatur als Checkpoint-Unterschrift angenommen")
	}
	if _, err := checkpointUnterschreiben(KettenCheckpoint{Hoehe: 1, Hash: "x", Zeit: 1}, key); err == nil {
		t.Fatal("ungueltigen Checkpoint unterschrieben")
	}
}

func testCheckpointHashBytes() []byte {
	b, _ := hex.DecodeString(testCheckpoint.Hash)
	return b
}
