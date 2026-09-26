//go:build !linux

package main

import "os"

// Ausserhalb von Linux: voller fsync und Vorbelegen durch Nullen, wie
// x/humanity/wal/datasync_other.go. Die Zahlen sind dann obere Schranken.
func datensync(f *os.File) error { return f.Sync() }

func vorbelegen(f *os.File, n int64) error {
	nullen := make([]byte, 1<<20)
	for off := int64(0); off < n; off += int64(len(nullen)) {
		k := int64(len(nullen))
		if n-off < k {
			k = n - off
		}
		if _, err := f.WriteAt(nullen[:k], off); err != nil {
			return err
		}
	}
	return nil
}
