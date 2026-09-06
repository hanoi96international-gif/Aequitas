//go:build windows

package keeper

import "golang.org/x/sys/windows"

// freierPlattenplatz auf Windows -- nur damit die Testsuite auf dem
// Entwicklungsrechner dieselbe Funktion sieht. Produktiv laeuft der Knoten
// unter Linux.
func freierPlattenplatz(pfad string) (frei, gesamt int64, err error) {
	p, err := windows.UTF16PtrFromString(pfad)
	if err != nil {
		return 0, 0, err
	}
	var freiFuerAufrufer, gesamtBytes, freiGesamt uint64
	if err := windows.GetDiskFreeSpaceEx(p, &freiFuerAufrufer, &gesamtBytes, &freiGesamt); err != nil {
		return 0, 0, err
	}
	return int64(freiFuerAufrufer), int64(gesamtBytes), nil
}
