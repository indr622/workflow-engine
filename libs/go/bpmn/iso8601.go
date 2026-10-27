package bpmn

import (
	"regexp"
	"strings"
	"time"
)

var (
	durationPattern = regexp.MustCompile(`^P(\d+Y)?(\d+M)?(\d+W)?(\d+D)?(T(\d+H)?(\d+M)?(\d+(\.\d+)?S)?)?$`)
	zoneIDSuffix    = regexp.MustCompile(`\[[A-Za-z0-9_+\-/]+\]$`)
)

// isISODuration memeriksa durasi ISO 8601, mis. "PT15M" atau "P1DT2H".
func isISODuration(s string) bool {
	s = strings.TrimSpace(s)
	if !durationPattern.MatchString(s) || s == "P" || strings.HasSuffix(s, "T") {
		return false
	}
	return true
}

// isISODateTime memeriksa tanggal-waktu ISO 8601 dengan offset, opsional diikuti zone id
// seperti "2026-01-01T09:00:00+07:00[Asia/Jakarta]".
func isISODateTime(s string) bool {
	s = zoneIDSuffix.ReplaceAllString(strings.TrimSpace(s), "")
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}

// isTimerCycle memeriksa interval berulang ISO 8601 ("R5/PT10S", "R/2026-01-01T00:00:00Z/P1D")
// atau ekspresi cron enam field yang juga diterima Zeebe.
func isTimerCycle(s string) bool {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "R") {
		parts := strings.Split(s, "/")
		if len(parts) < 2 || len(parts) > 3 {
			return false
		}
		if rep := parts[0][1:]; rep != "" && strings.Trim(rep, "0123456789") != "" {
			return false
		}
		if len(parts) == 2 {
			return isISODuration(parts[1])
		}
		// start/duration atau duration/end
		return (isISODateTime(parts[1]) && isISODuration(parts[2])) ||
			(isISODuration(parts[1]) && isISODateTime(parts[2]))
	}
	return len(strings.Fields(s)) == 6
}
