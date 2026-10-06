package logging

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type rotationTestFile struct {
	bytes.Buffer
	closes   int
	writeErr error
	closeErr error
	short    bool
}

func (f *rotationTestFile) Write(p []byte) (int, error) {
	if f.closes > 0 {
		return 0, os.ErrClosed
	}
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	if f.short && len(p) > 0 {
		return f.Buffer.Write(p[:len(p)-1])
	}
	return f.Buffer.Write(p)
}

func (f *rotationTestFile) Close() error {
	f.closes++
	return f.closeErr
}

type rotationTestOpener struct {
	files      map[string]*rotationTestFile
	opened     []string
	failName   string
	failCount  int
	openErr    error
	onNextOpen func()
}

func (o *rotationTestOpener) open(name string) (io.WriteCloser, error) {
	o.opened = append(o.opened, name)
	if name == o.failName && o.failCount > 0 {
		o.failCount--
		return nil, o.openErr
	}
	if o.onNextOpen != nil {
		o.onNextOpen()
	}
	if o.files == nil {
		o.files = make(map[string]*rotationTestFile)
	}
	file := &rotationTestFile{}
	o.files[name] = file
	return file, nil
}

func newRotationTestWriter(t *testing.T, filename, setting string, now *time.Time, opener *rotationTestOpener, diagnostics io.Writer) *RotatingWriter {
	t.Helper()
	period, err := ParsePeriod(setting)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := newRotatingWriter(filename, period, func() time.Time { return *now }, opener.open, diagnostics, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	return writer
}

func writeRotationRecord(t *testing.T, writer io.Writer, record string) {
	t.Helper()
	n, err := writer.Write([]byte(record))
	if err != nil || n != len(record) {
		t.Fatalf("Write(%q) = %d, %v", record, n, err)
	}
}

func TestRotatingWriterDailyBoundaryAndIdleGap(t *testing.T) {
	now := time.Date(2026, 10, 5, 23, 59, 59, 0, time.FixedZone("UTC+3", 3*60*60))
	opener := &rotationTestOpener{}
	writer := newRotationTestWriter(t, "log.txt", "", &now, opener, io.Discard)
	if len(opener.opened) != 1 || opener.opened[0] != "log-2026-10-05.txt" {
		t.Fatalf("initial open = %v", opener.opened)
	}
	writeRotationRecord(t, writer, "before\n")
	previous := opener.files["log-2026-10-05.txt"]
	opener.onNextOpen = func() {
		if previous.closes != 0 {
			t.Fatal("previous file was closed before opening its replacement")
		}
	}
	now = now.Add(time.Second)
	writeRotationRecord(t, writer, "midnight\n")
	opener.onNextOpen = nil
	if previous.String() != "before\n" || previous.closes != 1 {
		t.Fatalf("previous file contents %q, closes %d", previous.String(), previous.closes)
	}
	if opener.files["log-2026-10-06.txt"].String() != "midnight\n" {
		t.Fatal("midnight record did not go to the new day")
	}

	now = now.AddDate(0, 0, 4)
	writeRotationRecord(t, writer, "after idle\n")
	if got := strings.Join(opener.opened, ","); got != "log-2026-10-05.txt,log-2026-10-06.txt,log-2026-10-10.txt" {
		t.Fatalf("opened %s; idle days must not create files", got)
	}
}

func TestRotatingWriterFixedPeriodPreservesSubsecondPrecision(t *testing.T) {
	now := time.Date(2026, 10, 5, 8, 0, 0, 499999999, time.UTC)
	opener := &rotationTestOpener{}
	writer := newRotationTestWriter(t, "redis1c.log", "250ms", &now, opener, io.Discard)
	writeRotationRecord(t, writer, "first")
	now = now.Add(time.Nanosecond)
	writeRotationRecord(t, writer, "second")
	want := []string{
		"redis1c-2026-10-05T08-00-00.250000000Z-every-250000000ns.log",
		"redis1c-2026-10-05T08-00-00.500000000Z-every-250000000ns.log",
	}
	if strings.Join(opener.opened, ",") != strings.Join(want, ",") {
		t.Fatalf("opened %v; want %v", opener.opened, want)
	}

	other, err := ParsePeriod("500ms")
	if err != nil {
		t.Fatal(err)
	}
	start, _ := other.window(now)
	if other.suffix(start) == writer.period.suffix(start) {
		t.Fatal("different configured periods must not share a filename")
	}
}

func TestRotatingWriterDailyNamesFollowDatesWhenMidnightIsSkipped(t *testing.T) {
	for _, test := range []struct {
		zone  string
		year  int
		month time.Month
		day   int
	}{
		{"Africa/Cairo", 2026, time.April, 24},
		{"America/Sao_Paulo", 2018, time.November, 4},
	} {
		t.Run(test.zone, func(t *testing.T) {
			location, err := time.LoadLocation(test.zone)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Date(test.year, test.month, test.day, 1, 0, 0, 0, location)
			opener := &rotationTestOpener{}
			writer := newRotationTestWriter(t, "log.txt", "daily", &now, opener, io.Discard)
			firstName := "log-" + now.Format("2006-01-02") + ".txt"
			if len(opener.opened) != 1 || opener.opened[0] != firstName {
				t.Fatalf("initial open = %v; want %s", opener.opened, firstName)
			}
			writeRotationRecord(t, writer, "after skipped midnight\n")
			now = time.Date(test.year, test.month, test.day+1, 0, 0, 0, 0, location).Add(-time.Nanosecond)
			writeRotationRecord(t, writer, "before next midnight\n")
			if len(opener.opened) != 1 {
				t.Fatalf("opened %v before calendar date changed", opener.opened)
			}
			now = now.Add(time.Nanosecond)
			writeRotationRecord(t, writer, "at next midnight\n")
			secondName := "log-" + now.Format("2006-01-02") + ".txt"
			if len(opener.opened) != 2 || opener.opened[1] != secondName {
				t.Fatalf("midnight opens = %v; want %s", opener.opened, secondName)
			}
			if opener.files[firstName].String() != "after skipped midnight\nbefore next midnight\n" || opener.files[secondName].String() != "at next midnight\n" {
				t.Fatal("records crossed local calendar-date files")
			}
		})
	}
}

func TestRotatingWriterRestartAppendsAndPreservesHistoricalFile(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "log.txt")
	if err := os.WriteFile(filename, []byte("historical\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	period, err := ParsePeriod("daily")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	open := func(name string) (io.WriteCloser, error) {
		return os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	}
	for _, record := range []string{"first\n", "after restart\n"} {
		writer, err := newRotatingWriter(filename, period, func() time.Time { return now }, open, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		writeRotationRecord(t, writer, record)
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	content, err := os.ReadFile(filepath.Join(directory, "log-2026-10-05.txt"))
	if err != nil || string(content) != "first\nafter restart\n" {
		t.Fatalf("period file = %q, %v", content, err)
	}
	content, err = os.ReadFile(filename)
	if err != nil || string(content) != "historical\n" {
		t.Fatalf("historical file = %q, %v", content, err)
	}
}

func TestRotatingWriterFailedRotationRetainsRecordAndRetries(t *testing.T) {
	now := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	openErr := errors.New("injected open failure")
	opener := &rotationTestOpener{failName: "log-2026-10-06.txt", failCount: 1, openErr: openErr}
	var diagnostics bytes.Buffer
	writer := newRotationTestWriter(t, "log.txt", "daily", &now, opener, &diagnostics)
	writeRotationRecord(t, writer, "before\n")
	now = now.AddDate(0, 0, 1)
	writeRotationRecord(t, writer, "fallback\n")
	previous := opener.files["log-2026-10-05.txt"]
	if previous.closes != 0 || previous.String() != "before\nfallback\n" {
		t.Fatalf("old file must remain writable after failure: closes %d, contents %q", previous.closes, previous.String())
	}
	if !strings.Contains(diagnostics.String(), openErr.Error()) || !strings.Contains(diagnostics.String(), "continuing") {
		t.Fatalf("independent diagnostics = %q", diagnostics.String())
	}
	writeRotationRecord(t, writer, "retry\n")
	if previous.closes != 1 || opener.files["log-2026-10-06.txt"].String() != "retry\n" {
		t.Fatal("next write did not retry and complete rotation")
	}
	if len(opener.opened) != 3 {
		t.Fatalf("open attempts = %v; want initial, failure, retry", opener.opened)
	}
}

func TestRotatingWriterConcurrentBoundaryWritesAreComplete(t *testing.T) {
	now := time.Date(2026, 10, 5, 23, 59, 59, 0, time.UTC)
	opener := &rotationTestOpener{}
	writer := newRotationTestWriter(t, "log.txt", "daily", &now, opener, io.Discard)
	now = now.Add(time.Second)
	const count = 100
	var workers sync.WaitGroup
	for index := range count {
		workers.Add(1)
		go func() {
			defer workers.Done()
			record := fmt.Sprintf("record-%03d\n", index)
			n, err := writer.Write([]byte(record))
			if err != nil || n != len(record) {
				t.Errorf("concurrent Write = %d, %v", n, err)
			}
		}()
	}
	workers.Wait()
	if len(opener.opened) != 2 {
		t.Fatalf("open attempts = %v; want one rotation", opener.opened)
	}
	lines := strings.Split(strings.TrimSuffix(opener.files["log-2026-10-06.txt"].String(), "\n"), "\n")
	seen := make(map[string]bool)
	for _, line := range lines {
		if seen[line] {
			t.Fatalf("duplicate record %q", line)
		}
		seen[line] = true
	}
	for index := range count {
		if !seen[fmt.Sprintf("record-%03d", index)] {
			t.Fatalf("missing record %d", index)
		}
	}
	if opener.files["log-2026-10-05.txt"].closes != 1 {
		t.Fatal("old file must be closed once")
	}
}

func TestRotatingWriterCloseAndWriteFailures(t *testing.T) {
	now := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	opener := &rotationTestOpener{}
	writer := newRotationTestWriter(t, "log.txt", "daily", &now, opener, io.Discard)
	file := opener.files["log-2026-10-05.txt"]
	file.short = true
	if n, err := writer.Write([]byte("record")); n != 5 || !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short Write = %d, %v", n, err)
	}
	file.short = false
	writeErr := errors.New("injected write failure")
	file.writeErr = writeErr
	if _, err := writer.Write([]byte("record")); !errors.Is(err, writeErr) {
		t.Fatalf("Write error = %v; want %v", err, writeErr)
	}
	closeErr := errors.New("injected close failure")
	file.closeErr = closeErr
	if err := writer.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("Close error = %v; want %v", err, closeErr)
	}
	if err := writer.Close(); err != nil || file.closes != 1 {
		t.Fatalf("second Close = %v, close count %d", err, file.closes)
	}
	if n, err := writer.Write([]byte("late")); n != 0 || !errors.Is(err, os.ErrClosed) {
		t.Fatalf("late Write = %d, %v; want os.ErrClosed", n, err)
	}
	if n, err := writer.Write(nil); n != 0 || !errors.Is(err, os.ErrClosed) {
		t.Fatalf("late empty Write = %d, %v; want os.ErrClosed", n, err)
	}
}

func TestRotatingWriterInitializationErrors(t *testing.T) {
	for _, period := range []string{"0s", "wrong"} {
		if _, err := NewRotatingWriter("log.txt", period); err == nil {
			t.Fatalf("NewRotatingWriter with period %q should fail", period)
		}
	}
	if _, err := NewRotatingWriter("", "daily"); err == nil {
		t.Fatal("empty filename should fail")
	}
	openErr := errors.New("injected initial open failure")
	opener := &rotationTestOpener{failName: "log-2026-10-05.txt", failCount: 1, openErr: openErr}
	period, _ := ParsePeriod("daily")
	now := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	if _, err := newRotatingWriter("log.txt", period, func() time.Time { return now }, opener.open, io.Discard); !errors.Is(err, openErr) {
		t.Fatalf("initial open error = %v; want %v", err, openErr)
	}
}

func TestRotatingWriterReportsOldFileCloseFailureAndContinues(t *testing.T) {
	now := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	opener := &rotationTestOpener{}
	var diagnostics bytes.Buffer
	writer := newRotationTestWriter(t, "log.txt", "daily", &now, opener, &diagnostics)
	old := opener.files["log-2026-10-05.txt"]
	old.closeErr = errors.New("injected close failure")
	now = now.AddDate(0, 0, 1)
	writeRotationRecord(t, writer, "new file\n")
	if opener.files["log-2026-10-06.txt"].String() != "new file\n" || !strings.Contains(diagnostics.String(), old.closeErr.Error()) {
		t.Fatalf("new file or diagnostic missing: %q", diagnostics.String())
	}
}
