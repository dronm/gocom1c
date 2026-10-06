package logging

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func retentionTestFile(t *testing.T, directory, name string, modified time.Time) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("existing record\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	return path
}

func retentionTestExists(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Lstat(path)
	if want && err != nil {
		t.Fatalf("expected %q to remain: %v", path, err)
	}
	if !want && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %q to be removed, stat error = %v", path, err)
	}
}

func openRetentionTestFile(name string) (io.WriteCloser, error) {
	return os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

func newRetentionTestWriter(t *testing.T, directory, setting string, now *time.Time, diagnostics io.Writer, days ...int) *RotatingWriter {
	t.Helper()
	period, err := ParsePeriod(setting)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := newRotatingWriter(filepath.Join(directory, "log.txt"), period, func() time.Time { return *now }, openRetentionTestFile, diagnostics, days...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
	})
	return writer
}

func TestRetentionStartupDeletesOnlyExpiredOwnPeriodFiles(t *testing.T) {
	for _, setting := range []string{"daily", "1h"} {
		t.Run(setting, func(t *testing.T) {
			directory := t.TempDir()
			now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
			cutoff := now.Add(-30 * 24 * time.Hour)
			expired := cutoff.Add(-time.Second)
			activeName := "log-2026-10-06.txt"
			if setting == "1h" {
				activeName = "log-2026-10-06T12-00-00.000000000Z-every-3600000000000ns.txt"
			}
			files := []struct {
				name     string
				modified time.Time
				keep     bool
			}{
				// Both formats are eligible even when the configured period changes.
				{"log-2020-01-01.txt", expired, false},
				{"log-2020-01-01T00-00-00.000000000Z-every-250000000ns.txt", expired, false},
				// The timestamp in a filename is not the retention clock.
				{"log-2030-01-01.txt", expired, false},
				{"log-2030-01-01T00-00-00.000000000Z-every-3600000000000ns.txt", expired, false},
				{"log-1900-01-01.txt", now.Add(-24 * time.Hour), true},
				{"log-2020-01-02.txt", cutoff, true},
				{"log-2020-01-03.txt", now.Add(24 * time.Hour), true},
				{activeName, expired, true},
				{"log.txt", expired, true},
				{"redis1c-2020-01-01.txt", expired, true},
				{"log-2020-01-01.log", expired, true},
				{"log-2020-01-01.txt.bak", expired, true},
				{"log-2020-02-30.txt", expired, true},
				{"log-2020-1-01.txt", expired, true},
				{"log-backup.txt", expired, true},
				{"log-2020-01-01T00-00-00Z-every-3600000000000ns.txt", expired, true},
				{"log-2020-01-01T00-00-00.000000000Z-every-0ns.txt", expired, true},
				{"log-2020-01-01T00-00-00.000000000Z-every--1ns.txt", expired, true},
				{"log-2020-01-01T00-00-00.000000000Z-every-01ns.txt", expired, true},
				{"log-2020-01-01T00-00-00.000000000Z-every-9223372036854775808ns.txt", expired, true},
			}
			for _, file := range files {
				retentionTestFile(t, directory, file.name, file.modified)
			}
			directoryPath := filepath.Join(directory, "log-2020-01-04.txt")
			if err := os.Mkdir(directoryPath, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(directoryPath, expired, expired); err != nil {
				t.Fatal(err)
			}
			var diagnostics bytes.Buffer
			writer := newRetentionTestWriter(t, directory, setting, &now, &diagnostics)
			for _, file := range files {
				retentionTestExists(t, filepath.Join(directory, file.name), file.keep)
			}
			retentionTestExists(t, directoryPath, true)
			writeRotationRecord(t, writer, "active record\n")
			data, err := os.ReadFile(filepath.Join(directory, activeName))
			if err != nil || string(data) != "existing record\nactive record\n" {
				t.Fatalf("active file must remain writable and preserve earlier data: %q, %v", data, err)
			}
			if diagnostics.Len() != 0 {
				t.Fatalf("unexpected diagnostics: %s", diagnostics.String())
			}
		})
	}
}

func TestRetentionPreservesMatchingSymlinkAndItsTarget(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	target := retentionTestFile(t, t.TempDir(), "unrelated.txt", now.Add(-40*24*time.Hour))
	link := filepath.Join(directory, "log-2020-01-01.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	newRetentionTestWriter(t, directory, "daily", &now, io.Discard)
	retentionTestExists(t, link, true)
	retentionTestExists(t, target, true)
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("matching symlink was changed: %v, %v", info, err)
	}
}

func TestRetentionExplicitZeroDisablesStartupAndLaterCleanup(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	expired := retentionTestFile(t, directory, "log-2020-01-01.txt", now.Add(-40*24*time.Hour))
	writer := newRetentionTestWriter(t, directory, "daily", &now, io.Discard, 0)
	retentionTestExists(t, expired, true)
	now = now.Add(60 * 24 * time.Hour)
	writeRotationRecord(t, writer, "after idle\n")
	retentionTestExists(t, expired, true)
}

func TestRetentionRunsAtMostHourlyAndOnlyOnWrites(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	writer := newRetentionTestWriter(t, directory, "daily", &now, io.Discard, 1)
	expired := retentionTestFile(t, directory, "log-2020-01-01.txt", now.Add(-48*time.Hour))
	now = now.Add(time.Hour - time.Second)
	writeRotationRecord(t, writer, "before cleanup interval\n")
	retentionTestExists(t, expired, true)
	now = now.Add(time.Second)
	// Moving the clock alone must not trigger background deletion.
	retentionTestExists(t, expired, true)
	if n, err := writer.Write(nil); n != 0 || err != nil {
		t.Fatalf("empty Write = %d, %v", n, err)
	}
	retentionTestExists(t, expired, true)
	writeRotationRecord(t, writer, "at cleanup interval\n")
	retentionTestExists(t, expired, false)
	secondExpired := retentionTestFile(t, directory, "log-2020-01-02.txt", now.Add(-48*time.Hour))
	now = now.Add(30 * time.Minute)
	writeRotationRecord(t, writer, "between cleanups\n")
	retentionTestExists(t, secondExpired, true)
	now = now.Add(30 * time.Minute)
	writeRotationRecord(t, writer, "next cleanup\n")
	retentionTestExists(t, secondExpired, false)
}

func TestRetentionDeletesPreviousActiveFileAfterSuccessfulIdleRotation(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	oldName := "log-2026-10-06.txt"
	old := retentionTestFile(t, directory, oldName, now)
	writer := newRetentionTestWriter(t, directory, "daily", &now, io.Discard, 1)
	now = now.Add(3 * 24 * time.Hour)
	writeRotationRecord(t, writer, "after idle\n")
	retentionTestExists(t, old, false)
	newName := filepath.Join(directory, "log-2026-10-09.txt")
	data, err := os.ReadFile(newName)
	if err != nil || string(data) != "after idle\n" {
		t.Fatalf("new active log = %q, %v", data, err)
	}
}

func TestRetentionKeepsOldActiveFileWhenIdleRotationFails(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	old := retentionTestFile(t, directory, "log-2026-10-06.txt", now)
	period, err := ParsePeriod("daily")
	if err != nil {
		t.Fatal(err)
	}
	failRotation := false
	opener := func(name string) (io.WriteCloser, error) {
		if failRotation {
			return nil, errors.New("injected rotation failure")
		}
		return openRetentionTestFile(name)
	}
	var diagnostics bytes.Buffer
	writer, err := newRotatingWriter(filepath.Join(directory, "log.txt"), period, func() time.Time { return now }, opener, &diagnostics, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	now = now.Add(3 * 24 * time.Hour)
	failRotation = true
	writeRotationRecord(t, writer, "fallback record\n")
	retentionTestExists(t, old, true)
	data, err := os.ReadFile(old)
	if err != nil || string(data) != "existing record\nfallback record\n" {
		t.Fatalf("old active log = %q, %v", data, err)
	}
	if !strings.Contains(diagnostics.String(), "injected rotation failure") {
		t.Fatalf("missing rotation error: %s", diagnostics.String())
	}
}

func TestRetentionDirectoryReadFailureDoesNotInterruptLogging(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("renaming the directory of an open log file is not supported on Windows")
	}
	root := t.TempDir()
	directory := filepath.Join(root, "logs")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	var diagnostics bytes.Buffer
	writer := newRetentionTestWriter(t, directory, "daily", &now, &diagnostics, 1)
	moved := filepath.Join(root, "moved")
	if err := os.Rename(directory, moved); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	writeRotationRecord(t, writer, "record despite directory error\n")
	if !strings.Contains(diagnostics.String(), "cannot inspect log directory") {
		t.Fatalf("missing cleanup error diagnostic: %s", diagnostics.String())
	}
	firstDiagnostic := diagnostics.String()
	now = now.Add(30 * time.Minute)
	writeRotationRecord(t, writer, "between attempts\n")
	if diagnostics.String() != firstDiagnostic {
		t.Fatal("cleanup error was retried before its next hourly interval")
	}
	now = now.Add(30 * time.Minute)
	writeRotationRecord(t, writer, "next attempt\n")
	if strings.Count(diagnostics.String(), "cannot inspect log directory") != 2 {
		t.Fatalf("cleanup must retry after an hour: %s", diagnostics.String())
	}
	data, err := os.ReadFile(filepath.Join(moved, "log-2026-10-06.txt"))
	if err != nil || string(data) != "record despite directory error\nbetween attempts\nnext attempt\n" {
		t.Fatalf("records after cleanup failures = %q, %v", data, err)
	}
}

func TestRetentionInvalidSettingsCreateNoFiles(t *testing.T) {
	for _, test := range []struct {
		name string
		days []int
	}{
		{"negative", []int{-1}},
		{"duration overflow", []int{maxRetentionDays + 1}},
		{"multiple settings", []int{1, 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			filename := filepath.Join(directory, "log.txt")
			if writer, err := NewRotatingWriter(filename, "daily", test.days...); err == nil || writer != nil {
				if writer != nil {
					_ = writer.Close()
				}
				t.Fatalf("invalid retention accepted: writer=%v err=%v", writer, err)
			}
			if logger, closer, err := NewLogger("info", filename, "daily", test.days...); err == nil || logger != nil || closer != nil {
				if closer != nil {
					_ = closer.Close()
				}
				t.Fatalf("invalid logger retention accepted: logger=%v closer=%v err=%v", logger, closer, err)
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 0 {
				t.Fatalf("validation must precede opening files: entries=%v err=%v", entries, err)
			}
		})
	}
}

func TestNewLoggerRetentionDefaultAndExplicitDisable(t *testing.T) {
	for _, test := range []struct {
		name string
		days []int
		keep bool
	}{
		{"default", nil, false},
		{"disabled", []int{0}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			expired := retentionTestFile(t, directory, "log-2000-01-01.txt", time.Now().Add(-40*24*time.Hour))
			logger, closer, err := NewLogger("info", filepath.Join(directory, "log.txt"), "daily", test.days...)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = closer.Close() })
			retentionTestExists(t, expired, test.keep)
			logger.Info("retention wiring integration")
			retentionTestExists(t, expired, test.keep)
		})
	}
}

func TestValidateRetentionDaysDurationBoundaries(t *testing.T) {
	for _, days := range []int{0, 1, DefaultRetentionDays, maxRetentionDays} {
		if err := ValidateRetentionDays(days); err != nil {
			t.Errorf("valid retention %d rejected: %v", days, err)
		}
	}
	for _, days := range []int{-1, maxRetentionDays + 1} {
		if err := ValidateRetentionDays(days); err == nil {
			t.Errorf("invalid retention %d accepted", days)
		}
	}
}
