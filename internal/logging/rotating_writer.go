package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type fileOpener func(string) (io.WriteCloser, error)

// RotatingWriter appends to one file per period. It creates a new period's file
// only when a record is written, and never deletes existing log files.
type RotatingWriter struct {
	mu          sync.Mutex
	filename    string
	period      Period
	now         func() time.Time
	openFile    fileOpener
	diagnostics io.Writer
	file        io.WriteCloser
	activeName  string
	start       time.Time
	end         time.Time
	closed      bool
}

var _ io.WriteCloser = (*RotatingWriter)(nil)

// NewRotatingWriter opens the current period's file immediately. The parent
// directory must already exist. Existing files are always opened in append mode.
func NewRotatingWriter(filename string, rotationPeriod string) (*RotatingWriter, error) {
	period, err := ParsePeriod(rotationPeriod)
	if err != nil {
		return nil, err
	}

	return newRotatingWriter(filename, period, time.Now, func(name string) (io.WriteCloser, error) {
		return os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	}, os.Stderr)
}

func newRotatingWriter(filename string, period Period, now func() time.Time, opener fileOpener, diagnostics io.Writer) (*RotatingWriter, error) {
	if strings.TrimSpace(filename) == "" {
		return nil, fmt.Errorf("log filename must not be empty")
	}

	currentTime := now()
	start, end := period.window(currentTime)
	writer := &RotatingWriter{
		filename:    filename,
		period:      period,
		now:         now,
		openFile:    opener,
		diagnostics: diagnostics,
		start:       start,
		end:         end,
	}
	name := writer.periodFilename(start)
	if period.daily {
		// The actual calendar date determines the name. Midnight can normalize
		// into another date in locations that change UTC offset at midnight.
		name = writer.periodFilename(currentTime)
	}
	file, err := opener(name)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", name, err)
	}
	writer.file = file
	writer.activeName = name
	return writer, nil
}

func (w *RotatingWriter) periodFilename(start time.Time) string {
	extension := filepath.Ext(w.filename)
	stem := strings.TrimSuffix(w.filename, extension)
	return stem + "-" + w.period.suffix(start) + extension
}

// Write serializes records with rotation and Close. If a new file cannot be
// opened, it reports the failure to stderr and keeps the record in the old file.
// The following write retries the rotation.
func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, os.ErrClosed
	}
	if len(p) == 0 {
		return 0, nil
	}

	now := w.now()
	rotate := now.Before(w.start) || !now.Before(w.end)
	name := ""
	if w.period.daily {
		name = w.periodFilename(now)
		rotate = name != w.activeName
	}
	if rotate {
		start, end := w.period.window(now)
		if !w.period.daily {
			name = w.periodFilename(start)
		}
		next, err := w.openFile(name)
		if err != nil {
			w.report("cannot rotate log to %q: %v; continuing in the previous log file", name, err)
		} else {
			previous := w.file
			w.file = next
			w.activeName = name
			w.start, w.end = start, end
			if err := previous.Close(); err != nil {
				w.report("cannot close the previous log file after rotating to %q: %v", name, err)
			}
		}
	}

	n, err := w.file.Write(p)
	if n < len(p) && err == nil {
		err = io.ErrShortWrite
	}
	return n, err
}

func (w *RotatingWriter) report(format string, args ...any) {
	if w.diagnostics != nil {
		_, _ = fmt.Fprintf(w.diagnostics, "gocom1c logging: "+format+"\n", args...)
	}
}

// Close closes the active file once. Subsequent writes return os.ErrClosed.
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true
	return w.file.Close()
}
