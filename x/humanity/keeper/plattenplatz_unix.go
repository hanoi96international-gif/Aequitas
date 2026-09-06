//go:build !windows

package keeper

import "golang.org/x/sys/unix"

// freierPlattenplatz liefert freie und gesamte Bytes des Dateisystems, in dem
// pfad liegt. Genommen wird Bavail (was einem unprivilegierten Prozess
// wirklich zur Verfuegung steht), nicht Bfree -- ext4 haelt per Vorgabe 5 %
// fuer root zurueck, und ein Knoten, der diese Reserve mitzaehlt, meldet
// Platz, den er nicht bekommt.
func freierPlattenplatz(pfad string) (frei, gesamt int64, err error) {
	var st unix.Statfs_t
	if err := unix.Statfs(pfad, &st); err != nil {
		return 0, 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), int64(st.Blocks) * int64(st.Bsize), nil
}
