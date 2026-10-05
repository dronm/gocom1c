package logging

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestNewLoggerRetainsFormattingAndLevels(t *testing.T) {
	for _, tc := range []struct {
		name string
		want logrus.Level
	}{
		{name: "debug", want: logrus.DebugLevel},
		{name: "info", want: logrus.InfoLevel},
		{name: "warn", want: logrus.WarnLevel},
		{name: "error", want: logrus.ErrorLevel},
		{name: "unknown", want: logrus.InfoLevel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger, closer, err := NewLogger(tc.name, "", DefaultRotationPeriod)
			if err != nil {
				t.Fatal(err)
			}
			if logger.GetLevel() != tc.want {
				t.Fatalf("level = %v, want %v", logger.GetLevel(), tc.want)
			}
			formatter, ok := logger.Formatter.(*logrus.TextFormatter)
			if !ok || !formatter.FullTimestamp {
				t.Fatal("expected text formatter with full timestamps")
			}
			if err := closer.Close(); err != nil {
				t.Fatal(err)
			}
			if closer.(*loggerOutput).writer != os.Stderr {
				t.Fatal("console output changed or was closed")
			}
		})
	}
}

func TestNewLoggerWritesDatedFile(t *testing.T) {
	directory := t.TempDir()
	logger, closer, err := NewLogger("info", filepath.Join(directory, "log.txt"), "daily")
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("filtered debug message")
	logger.Info("saved application message")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(directory, "log-*.txt"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected one dated log file, got %v, %v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "saved application message") || strings.Contains(string(data), "filtered debug message") {
		t.Fatalf("unexpected file content: %s", data)
	}
	output := closer.(*loggerOutput)
	if output.writer != os.Stderr {
		t.Fatal("late log records must use stderr")
	}
	if _, err := output.closer.(*RotatingWriter).Write([]byte("closed\n")); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("file handle remains open after logger close: %v", err)
	}
}

func TestLoggerInitializationRejectsInvalidPeriod(t *testing.T) {
	logger, closer, err := NewLogger("info", "", "0s")
	if err == nil || logger != nil || closer != nil {
		t.Fatalf("invalid period accepted: logger=%v closer=%v error=%v", logger, closer, err)
	}
}

// A COM worker can still be writing when shutdown closes the logger. Closing
// waits for that write and then redirects the existing logger to stderr.
func TestLoggerOutputCloseWaitsForCurrentWrite(t *testing.T) {
	writer := &blockingLogWriter{entered: make(chan struct{}), release: make(chan struct{})}
	output := &loggerOutput{writer: writer, closer: writer}
	writeDone := make(chan error, 1)
	go func() {
		_, err := output.Write([]byte("in-flight log record"))
		writeDone <- err
	}()
	<-writer.entered
	closeDone := make(chan error, 1)
	go func() { closeDone <- output.Close() }()
	close(writer.release)
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	if writer.closedBeforeWriteFinished {
		t.Fatal("underlying file closed before the current write finished")
	}
	if output.writer != os.Stderr {
		t.Fatal("closed output was not redirected to stderr")
	}
	if err := output.Close(); err != nil || writer.closeCount != 1 {
		t.Fatalf("close is not idempotent: count=%d error=%v", writer.closeCount, err)
	}
}

type blockingLogWriter struct {
	mu                        sync.Mutex
	entered                   chan struct{}
	release                   chan struct{}
	writeFinished             bool
	closedBeforeWriteFinished bool
	closeCount                int
}

func (w *blockingLogWriter) Write(data []byte) (int, error) {
	close(w.entered)
	<-w.release
	w.mu.Lock()
	w.writeFinished = true
	w.mu.Unlock()
	return len(data), nil
}

func (w *blockingLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closeCount++
	w.closedBeforeWriteFinished = !w.writeFinished
	return nil
}
