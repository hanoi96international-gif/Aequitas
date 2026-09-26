//go:build linux

package main

import (
	"os"
	"syscall"
)

// Wie der WAL (x/humanity/wal/datasync_linux.go): fdatasync auf eine
// vorbelegte Datei -- ohne Metadaten-Journal.
func datensync(f *os.File) error { return syscall.Fdatasync(int(f.Fd())) }

func vorbelegen(f *os.File, n int64) error {
	return syscall.Fallocate(int(f.Fd()), 0, 0, n)
}
