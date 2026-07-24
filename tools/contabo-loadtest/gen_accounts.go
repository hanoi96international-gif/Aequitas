//go:build ignore

// gen_accounts.go generates N fresh EVM keypairs for a loadtest run. Writes
// a CSV (index,address,privkey_hex) to stdout -- NEVER commit that output,
// it contains private keys. Run with: go run gen_accounts.go [N] > accounts.csv
//
// Keep this file itself out of the normal build (see the "ignore" build tag
// above) since main.go already defines its own main function.
package main

import (
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
)

func main() {
	n := 150
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &n)
	}
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()
	w.Write([]string{"index", "address", "privkey_hex"})
	for i := 0; i < n; i++ {
		priv, err := crypto.GenerateKey()
		if err != nil {
			panic(err)
		}
		addr := crypto.PubkeyToAddress(priv.PublicKey).Hex()
		privHex := hex.EncodeToString(crypto.FromECDSA(priv))
		w.Write([]string{fmt.Sprintf("%d", i), addr, privHex})
	}
}
