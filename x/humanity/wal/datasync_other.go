//go:build !linux

package wal

import "os"

// datasync falls back to a full fsync everywhere except Linux.
//
// Correct, just slower: fsync is strictly stronger than fdatasync, so a
// platform without the cheaper call still gets the durability guarantee the
// WAL depends on. Production runs Linux; this exists so the package builds and
// its tests run on a developer machine.
func datasync(f *os.File) error {
	return f.Sync()
}

// preallocate reserves space by writing zeros, since fallocate is Linux-only.
//
// Slower than fallocate and identical in effect: real blocks, real size, no
// sparseness. Production is Linux; this keeps the package building and its
// tests meaningful on a developer machine.
func preallocate(f *os.File, off, n int64) error {
	const chunk = 1 << 20
	zeros := make([]byte, chunk)
	for written := int64(0); written < n; {
		size := int64(chunk)
		if remaining := n - written; remaining < size {
			size = remaining
		}
		if _, err := f.WriteAt(zeros[:size], off+written); err != nil {
			return err
		}
		written += size
	}
	return nil
}
