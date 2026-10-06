package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultRetentionDays = 30
	maxRetentionDays     = int((1<<63 - 1) / int64(24*time.Hour))
	cleanupInterval      = time.Hour
	periodTimestamp      = "2006-01-02T15-04-05.000000000Z"
)

// ValidateRetentionDays prevents negative retention and duration overflow.
// Zero disables automatic deletion.
func ValidateRetentionDays(days int) error {
	if days < 0 || days > maxRetentionDays {
		return fmt.Errorf("logRetentionDays must be an integer between 0 and %d", maxRetentionDays)
	}
	return nil
}

func retentionSetting(values []int) (int, error) {
	if len(values) > 1 {
		return 0, fmt.Errorf("expected at most one log retention setting")
	}
	days := DefaultRetentionDays
	if len(values) == 1 {
		days = values[0]
	}
	return days, ValidateRetentionDays(days)
}

// cleanup is called while the writer is exclusively owned or locked. Errors
// never stop logging; another cleanup will be attempted after one hour.
func (w *RotatingWriter) cleanup(now time.Time) {
	if w.retentionDays == 0 || (!w.nextCleanup.IsZero() && now.Before(w.nextCleanup)) {
		return
	}
	w.nextCleanup = now.Add(cleanupInterval)
	directory := filepath.Dir(w.filename)
	entries, err := os.ReadDir(directory)
	if err != nil {
		w.report("cannot inspect log directory %q for retention cleanup: %v", directory, err)
		return
	}
	cutoff := now.Add(-time.Duration(w.retentionDays) * 24 * time.Hour)
	activeName := filepath.Base(w.activeName)
	for _, entry := range entries {
		name := entry.Name()
		if strings.EqualFold(name, activeName) || entry.Type()&os.ModeSymlink != 0 || !periodLogName(name, filepath.Base(w.filename)) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			w.report("cannot inspect old log file %q: %v", filepath.Join(directory, name), err)
			continue
		}
		if !info.Mode().IsRegular() || !info.ModTime().Before(cutoff) {
			continue
		}
		name = filepath.Join(directory, name)
		if err := os.Remove(name); err != nil {
			w.report("cannot delete expired log file %q: %v", name, err)
		}
	}
}

// Recognize both supported rotation formats, even after the period is changed.
// Legacy, malformed and unrelated filenames are deliberately excluded.
func periodLogName(name, baseName string) bool {
	extension := filepath.Ext(baseName)
	prefix := strings.TrimSuffix(baseName, extension) + "-"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, extension) {
		return false
	}
	suffix := strings.TrimSuffix(strings.TrimPrefix(name, prefix), extension)
	if date, err := time.Parse("2006-01-02", suffix); err == nil && date.Format("2006-01-02") == suffix {
		return true
	}
	stamp, interval, found := strings.Cut(suffix, "-every-")
	if !found || !strings.HasSuffix(interval, "ns") {
		return false
	}
	date, err := time.Parse(periodTimestamp, stamp)
	if err != nil || date.Format(periodTimestamp) != stamp {
		return false
	}
	interval = strings.TrimSuffix(interval, "ns")
	nanoseconds, err := strconv.ParseInt(interval, 10, 64)
	return err == nil && nanoseconds > 0 && strconv.FormatInt(nanoseconds, 10) == interval
}
