package keeper

import (
	"log/slog"
	"os"
)

// Log is the shared structured logger for the keeper package.
// P3-5: uses log/slog (Go 1.21+) with JSON output so hosted
// log aggregators can filter and search by field, not just grep text.
//
// Usage:
//
//	Log.Info("transfer completed", "from", from, "to", to, "amount", amount)
//	Log.Warn("optimistic lock conflict", "address", addr, "version", v)
//	Log.Error("DB write failed", "error", err)
var Log *slog.Logger

func init() {
	// JSON handler on stderr so log aggregators capture structured events
	// without mixing with the fmt.Printf debug lines on stdout.
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel(),
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Rename "time" to "ts" and "msg" to "event" for compactness.
			if a.Key == slog.TimeKey {
				a.Key = "ts"
			}
			if a.Key == slog.MessageKey {
				a.Key = "event"
			}
			return a
		},
	})
	Log = slog.New(handler).With("service", "aequitas-chain")
	slog.SetDefault(Log)
}

// logLevel reads LOG_LEVEL env var; defaults to Info.
func logLevel() slog.Level {
	switch os.Getenv("LOG_LEVEL") {
	case "debug", "DEBUG":
		return slog.LevelDebug
	case "warn", "WARN":
		return slog.LevelWarn
	case "error", "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
