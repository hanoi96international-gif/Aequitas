//go:build linux

package wal

import (
	"os"
	"syscall"
)

// datasync flushes the file's DATA without forcing a metadata update.
//
// MEASURED on the production box, same filesystem, four shapes back to back:
//
//	fsync,     appending      352/s   p50 1970us  p99 14696us   <- what this was
//	fdatasync, appending      355/s   p50 1990us  p99 13321us
//	fsync,     preallocated   470/s   p50 1803us  p99  5571us
//	fdatasync, preallocated  1429/s   p50  559us  p99  2060us   <- what it is now
//
// Four times the sync rate and a seventh of the p99. Neither half works alone:
// fdatasync on a growing file still has to persist the new size, and fsync on a
// preallocated one still writes the inode. Only both together let the kernel
// skip the metadata journal transaction entirely.
//
// That matters here because the WAL writer's throughput ceiling is exactly
// records-per-batch divided by sync time, and sync time was the binding
// constraint on the whole chain: CPU sat at one core of six, GC at 0.15% of
// wall time, and four separate lock-side changes moved nothing.
func datasync(f *os.File) error {
	return syscall.Fdatasync(int(f.Fd()))
}

// preallocate reserves real blocks for [off, off+n) and grows the file to
// cover them.
//
// fallocate, not Truncate: growing with Truncate produces a SPARSE file, so
// the first write into each block still allocates it and still updates
// metadata -- which is exactly the cost preallocation exists to remove. Mode 0
// allocates the blocks and extends the size, so every later write lands on
// storage that is already there and the file size never changes again until
// the next extension.
func preallocate(f *os.File, off, n int64) error {
	return syscall.Fallocate(int(f.Fd()), 0, off, n)
}
