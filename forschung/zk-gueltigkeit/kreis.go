// Package zkgueltigkeit ist der Forschungsprototyp zu Stufe 4 aus
// docs/SKALIERUNG_DEZENTRAL.md: ein Gueltigkeitsbeweis (zk-SNARK, Groth16 auf
// BN254), dass ein Buendel von Ueberweisungen den Zustand korrekt von einer
// alten zu einer neuen Wurzel fuehrt.
//
// Wer den Beweis prueft, braucht weder den Zustand noch die Ueberweisungen --
// nur zwei Wurzeln und einige hundert Bytes Beweis. Pruefen kostet
// Millisekunden, auch auf einem Handy; Uebernehmen ohne Nachrechnen wird
// moeglich, ohne jemandem zu vertrauen.
//
// Was der Schaltkreis je Ueberweisung erzwingt:
//   - das Konto des Absenders steht mit (Schluessel, Guthaben, Nonce) unter
//     der aktuellen Wurzel (Merkle-Pfad, MiMC);
//   - die Ueberweisung ist vom Absender signiert (EdDSA auf der
//     Twisted-Edwards-Kurve von BN254);
//   - die Nonce passt, das Guthaben deckt den Betrag (64-Bit-Bereich);
//   - Absender und Empfaenger sind verschieden;
//   - die neue Wurzel ergibt sich aus Belastung und Gutschrift.
//
// Eigenes Go-Modul: gnark hebt gnark-crypto auf eine Version, von der
// go-ethereum im Hauptmodul abhaengt. Der Prototyp beeinflusst den Knoten
// nicht.
package zkgueltigkeit

import (
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/algebra/native/twistededwards"
	"github.com/consensys/gnark/std/hash/mimc"
	"github.com/consensys/gnark/std/signature/eddsa"

	tedwards "github.com/consensys/gnark-crypto/ecc/twistededwards"
)

// KontoKreis: ein Konto im Schaltkreis.
type KontoKreis struct {
	Schluessel eddsa.PublicKey
	Guthaben   frontend.Variable
	Nonce      frontend.Variable
}

// UeberweisungKreis: eine Ueberweisung mit allem, was der Beweis braucht.
type UeberweisungKreis struct {
	VonIndex, AnIndex frontend.Variable
	Von, An           KontoKreis // Stand VOR dieser Ueberweisung
	VonPfad, AnPfad   []frontend.Variable
	Betrag            frontend.Variable
	Sig               eddsa.Signature
}

// Kreis: ein Buendel. Oeffentlich sind nur die beiden Wurzeln.
type Kreis struct {
	AltWurzel frontend.Variable `gnark:",public"`
	NeuWurzel frontend.Variable `gnark:",public"`
	Txs       []UeberweisungKreis
}

// NeuerKreis: leerer Kreis fuer tiefe und buendel (fuer Compile und Zeugen).
func NeuerKreis(tiefe, buendel int) *Kreis {
	k := &Kreis{Txs: make([]UeberweisungKreis, buendel)}
	for i := range k.Txs {
		k.Txs[i].VonPfad = make([]frontend.Variable, tiefe)
		k.Txs[i].AnPfad = make([]frontend.Variable, tiefe)
	}
	return k
}

func blatt(api frontend.API, k KontoKreis) (frontend.Variable, error) {
	h, err := mimc.NewMiMC(api)
	if err != nil {
		return nil, err
	}
	h.Write(k.Schluessel.A.X, k.Schluessel.A.Y, k.Guthaben, k.Nonce)
	return h.Sum(), nil
}

// wurzelAus: Wurzel aus Blatt, Index (Bits von unten) und Geschwistern.
func wurzelAus(api frontend.API, b, index frontend.Variable, pfad []frontend.Variable) (frontend.Variable, error) {
	bits := api.ToBinary(index, len(pfad))
	cur := b
	for i, g := range pfad {
		h, err := mimc.NewMiMC(api)
		if err != nil {
			return nil, err
		}
		l := api.Select(bits[i], g, cur)
		r := api.Select(bits[i], cur, g)
		h.Write(l, r)
		cur = h.Sum()
	}
	return cur, nil
}

// Define: die Regeln.
func (c *Kreis) Define(api frontend.API) error {
	kurve, err := twistededwards.NewEdCurve(api, tedwards.BN254)
	if err != nil {
		return err
	}
	wurzel := c.AltWurzel
	for _, tx := range c.Txs {
		// Absender unter der aktuellen Wurzel.
		b, err := blatt(api, tx.Von)
		if err != nil {
			return err
		}
		w, err := wurzelAus(api, b, tx.VonIndex, tx.VonPfad)
		if err != nil {
			return err
		}
		api.AssertIsEqual(w, wurzel)

		// Signatur ueber (VonIndex, AnIndex, Betrag, Nonce).
		mh, err := mimc.NewMiMC(api)
		if err != nil {
			return err
		}
		mh.Write(tx.VonIndex, tx.AnIndex, tx.Betrag, tx.Von.Nonce)
		nachricht := mh.Sum()
		sh, err := mimc.NewMiMC(api)
		if err != nil {
			return err
		}
		if err := eddsa.Verify(kurve, tx.Sig, nachricht, tx.Von.Schluessel, &sh); err != nil {
			return err
		}

		// Betrag und Rest im 64-Bit-Bereich: keine Ueberziehung, kein
		// Ueberlauf durch den Koerper.
		api.ToBinary(tx.Betrag, 64)
		rest := api.Sub(tx.Von.Guthaben, tx.Betrag)
		api.ToBinary(rest, 64)
		api.AssertIsDifferent(tx.VonIndex, tx.AnIndex)

		// Belastung.
		neuVon := KontoKreis{Schluessel: tx.Von.Schluessel, Guthaben: rest, Nonce: api.Add(tx.Von.Nonce, 1)}
		b, err = blatt(api, neuVon)
		if err != nil {
			return err
		}
		zwischen, err := wurzelAus(api, b, tx.VonIndex, tx.VonPfad)
		if err != nil {
			return err
		}

		// Empfaenger unter der Zwischenwurzel, dann Gutschrift.
		b, err = blatt(api, tx.An)
		if err != nil {
			return err
		}
		w, err = wurzelAus(api, b, tx.AnIndex, tx.AnPfad)
		if err != nil {
			return err
		}
		api.AssertIsEqual(w, zwischen)
		neuAn := KontoKreis{Schluessel: tx.An.Schluessel, Guthaben: api.Add(tx.An.Guthaben, tx.Betrag), Nonce: tx.An.Nonce}
		api.ToBinary(neuAn.Guthaben, 64)
		b, err = blatt(api, neuAn)
		if err != nil {
			return err
		}
		wurzel, err = wurzelAus(api, b, tx.AnIndex, tx.AnPfad)
		if err != nil {
			return err
		}
	}
	api.AssertIsEqual(wurzel, c.NeuWurzel)
	return nil
}
