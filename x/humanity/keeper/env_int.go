package keeper

import (
	"fmt"
	"os"
	"strconv"
)

// intFromEnv reads a positive integer from the environment, falling back to def.
//
// A malformed or non-positive value keeps the default and says so, rather than
// being silently treated as zero — a database pool of zero, or a limit of zero,
// would take a node down in a way that looks nothing like a typo in an env var.
func intFromEnv(key string, def int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		fmt.Printf("[CONFIG] ⚠ %s=%q is not a positive integer — keeping the default of %d\n", key, raw, def)
		return def
	}
	return n
}
