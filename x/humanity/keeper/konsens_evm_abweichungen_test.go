package keeper

// Bekannte, erklaerte Abweichungen von go-ethereum v1.17.8 gegenueber den
// Golden-Werten aus v1.13.0 (konsens_evm_golden_test.go). Jede steht hier
// mit der genauen Zeile, die jetzt herauskommen muss, und dem Grund. Alles,
// was nicht hier steht, muss byte-gleich bleiben.

type bekannteAbweichung struct {
	jetzt string // die Zeile, die v1.17.8 liefern muss ("" = entfaellt)
	grund string
}

var konsensBekannteAbweichungen = map[string]map[string]bekannteAbweichung{}
