// Package logger menyiapkan logger slog berformat JSON.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New membuat logger JSON dengan level dari string ("debug", "info", "warn", "error").
func New(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
