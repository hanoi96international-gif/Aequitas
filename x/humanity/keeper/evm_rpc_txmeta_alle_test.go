package keeper

import (
	"reflect"
	"testing"
)

// Der vorhandene Begrenzungstest zaehlt die Karten NAMENTLICH auf und kannte
// sechs. Seitdem sind fuenf dazugekommen (nonces, values, gasLimits, types,
// inputs) -- keine davon wurde geprueft. Eine Karte ohne Verdraengung waechst
// fuer die ganze Prozesslaufzeit und macht die Sperre nebenbei immer teurer,
// weil Go eine wachsende Map umhaengt, waehrend der Aufrufer sie haelt.
//
// Dieser Test zaehlt die Karten NICHT auf, sondern findet sie. Wer eine
// zwoelfte hinzufuegt und die Verdraengung vergisst, faellt hier auf; wer sie
// hinzufuegt und nur den Test vergisst, ebenfalls -- deshalb die zweite
// Behauptung, dass jede gefundene Karte auch wirklich befuellt wurde.
func TestTxMetaShard_JedeKarteBleibtBegrenzt(t *testing.T) {
	s := &EVMRPCServer{}
	s.initTxMetaShards()

	// Von Hand befuellt. Wird hier eine vergessen, meldet der Abgleich weiter
	// unten sie als "gefunden, aber nie befuellt".
	befuellt := map[string]bool{
		"status": true, "errMsg": true, "senders": true, "tos": true,
		"deployed": true, "nonces": true, "values": true, "gasLimits": true,
		"types": true, "inputs": true,
	}

	const n = txMetaMax + 20000
	for i := 0; i < n; i++ {
		h := "0x" + itoa(i)
		sh := s.txMetaShardFor(h)
		sh.mu.Lock()
		sh.status[h] = true
		sh.errMsg[h] = "e"
		sh.senders[h] = "0xfrom"
		sh.tos[h] = "0xto"
		sh.deployed[h] = "0xcontract"
		sh.nonces[h] = uint64(i)
		sh.values[h] = "0x1"
		sh.gasLimits[h] = 21000
		sh.types[h] = 2
		sh.inputs[h] = "0xdeadbeef"
		sh.note(h)
		sh.mu.Unlock()
	}

	gefunden := map[string]bool{}
	for i := range s.txMeta {
		sh := &s.txMeta[i]
		v := reflect.ValueOf(sh).Elem()
		typ := v.Type()
		for f := 0; f < v.NumField(); f++ {
			if v.Field(f).Kind() != reflect.Map {
				continue
			}
			name := typ.Field(f).Name
			gefunden[name] = true
			// Len() ist auf einem unexportierten Feld erlaubt, Set/Interface
			// waeren es nicht -- deshalb wird oben von Hand befuellt.
			if got := v.Field(f).Len(); got > txMetaMaxPerShard {
				t.Fatalf("Karte %s in Splitter %d haelt %d Eintraege, ihr Anteil ist %d -- sie wird nicht verdraengt und waechst unbegrenzt", name, i, got, txMetaMaxPerShard)
			}
		}
	}

	if len(gefunden) == 0 {
		t.Fatal("keine einzige Karte gefunden -- der Test prueft nichts mehr")
	}
	for name := range gefunden {
		if !befuellt[name] {
			t.Fatalf("txMetaShard hat die Karte %s, dieser Test befuellt sie aber nicht. Sie wird damit nur scheinbar geprueft: eine leere Karte ist immer innerhalb der Grenze. Bitte oben mit befuellen (und in note()/der Verdraengung beruecksichtigen)", name)
		}
	}
	for name := range befuellt {
		if !gefunden[name] {
			t.Fatalf("dieser Test befuellt %s, das Feld gibt es aber nicht mehr -- bitte hier entfernen", name)
		}
	}
}
