package keeper

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestAbsenderCache_EinmalWiederherstellenNieVerwechseln(t *testing.T) {
	a, b := neuerTestSchluessel(t), neuerTestSchluessel(t)
	empf := "0x00000000000000000000000000000000000000e1"
	txA := signiere(t, a, empf, aeqWei(5), 7, 1926)
	txB := signiere(t, b, empf, aeqWei(5), 7, 1926) // gleicher Inhalt, andere Signatur

	vorT, vorV := absenderTreffer.Load(), absenderVerfehlt.Load()
	_, s1, _, err := decodeAndRecoverSender(txA.Roh)
	if err != nil || s1 != a.addr {
		t.Fatalf("erste Wiederherstellung: %s, %v", s1, err)
	}
	_, s2, _, err := decodeAndRecoverSender(txA.Roh)
	if err != nil || s2 != a.addr {
		t.Fatalf("zweite Wiederherstellung: %s, %v", s2, err)
	}
	if absenderTreffer.Load()-vorT != 1 || absenderVerfehlt.Load()-vorV != 1 {
		t.Fatalf("erwartet 1 Treffer und 1 Fehlgriff, bekam %d/%d",
			absenderTreffer.Load()-vorT, absenderVerfehlt.Load()-vorV)
	}
	// Gleicher Inhalt, andere Signatur: anderer Hash, kein Treffer, anderer Absender.
	_, s3, _, err := decodeAndRecoverSender(txB.Roh)
	if err != nil || s3 != b.addr {
		t.Fatalf("Signatur von B ergab %s (%v) -- der Cache darf nie A liefern", s3, err)
	}

	// Mit warmem Cache bleibt eine Faelschung eine Faelschung: B's Rohform
	// unter A's Namen.
	gefaelscht := txB
	gefaelscht.Wallet = a.addr
	if _, err := pruefeUeberweisungsRoh(&gefaelscht); !errors.Is(err, ErrUeberweisungNichtSigniert) {
		t.Fatalf("gefaelschter Absender mit warmem Cache angenommen: %v", err)
	}
	// Und ein gefaelschtes TxHash-Feld trifft keinen Cache-Eintrag: der
	// Schluessel ist der Hash der Bytes.
	gefaelscht = txB
	gefaelscht.TxHash = txA.TxHash
	if _, err := pruefeUeberweisungsRoh(&gefaelscht); !errors.Is(err, ErrUeberweisungNichtSigniert) {
		t.Fatalf("fremdes TxHash-Feld angenommen: %v", err)
	}
}

func TestAbsenderCache_BegrenzteGroesse(t *testing.T) {
	c := &absenderCache{eintrag: map[common.Hash]string{}}
	for i := 0; i < absenderCacheGroesse+10; i++ {
		var h common.Hash
		h[0], h[1], h[2], h[3] = byte(i), byte(i>>8), byte(i>>16), byte(i>>24)
		c.merken(h, "x")
	}
	if len(c.eintrag) != absenderCacheGroesse || len(c.ring) != absenderCacheGroesse {
		t.Fatalf("Cache waechst ueber die Grenze: %d Eintraege, Ring %d", len(c.eintrag), len(c.ring))
	}
	var erster common.Hash // i = 0: muss als Erster herausgefallen sein
	if _, ok := c.eintrag[erster]; ok {
		t.Fatal("aeltester Eintrag blieb erhalten")
	}
}
