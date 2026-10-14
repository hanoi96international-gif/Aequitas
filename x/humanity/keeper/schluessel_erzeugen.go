package keeper

// Schluessel eines neuen Knotens (Beta-Pruefung 14.10.2026, HOCH): bis dahin
// erzeugte der Knoten beim ersten Start RELAYER_PRIVATE_KEY und NODE_KEY und
// druckte BEIDE ins Log -- dorthin, wo Log-Workflows, docker logs und
// Hosting-Dashboards sie lesen. Jetzt erzeugt sie ein eigener Aufruf
// (`aequitasd schluessel`) und schreibt sie nur auf seine Standardausgabe,
// die das Einrichtungsskript direkt in die .env leitet. Der laufende Knoten
// druckt nie einen privaten Schluessel.

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
)

// SchluesselAusgeben schreibt zwei .env-Zeilen (RELAYER_PRIVATE_KEY,
// NODE_KEY) mit frisch erzeugten Schluesseln nach w -- nichts sonst.
func SchluesselAusgeben(w io.Writer) error {
	rk, err := ethcrypto.GenerateKey()
	if err != nil {
		return fmt.Errorf("Signierschluessel: %w", err)
	}
	nk, _, err := p2pcrypto.GenerateKeyPairWithReader(p2pcrypto.Ed25519, -1, rand.Reader)
	if err != nil {
		return fmt.Errorf("P2P-Schluessel: %w", err)
	}
	roh, err := p2pcrypto.MarshalPrivateKey(nk)
	if err != nil {
		return fmt.Errorf("P2P-Schluessel: %w", err)
	}
	_, err = fmt.Fprintf(w, "RELAYER_PRIVATE_KEY=0x%s\nNODE_KEY=%s\n",
		hex.EncodeToString(ethcrypto.FromECDSA(rk)), base64.StdEncoding.EncodeToString(roh))
	return err
}
