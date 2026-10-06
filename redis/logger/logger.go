// Package logger supplies the application's shared logging policy.
package logger

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/dronm/gocom1c/internal/logging"
	"github.com/sirupsen/logrus"
)

// Logger is the global logger instance. Rotation keeps this instance stable.
var Logger *logrus.Logger

var (
	lifecycleMu sync.Mutex
	output      io.Closer
)

type LoggerLogLevel string

type LogWriter struct {
	logger *logrus.Logger
}

func NewLogWriter() *LogWriter {
	return &LogWriter{logger: Logger}
}

func (lw *LogWriter) Write(p []byte) (int, error) {
	lw.logger.Info(string(p))
	return len(p), nil
}

// Initialize accepts an optional period so existing two-argument callers keep
// the daily default. Log retention defaults to 30 days.
func Initialize(logLevel LoggerLogLevel, filename string, rotationPeriod ...string) error {
	period := logging.DefaultRotationPeriod
	if len(rotationPeriod) > 1 {
		return fmt.Errorf("expected at most one log rotation period")
	}
	if len(rotationPeriod) == 1 {
		period = rotationPeriod[0]
	}
	return InitializeWithRetention(logLevel, filename, period, logging.DefaultRetentionDays)
}

// InitializeWithRetention applies explicit rotation and retention settings.
func InitializeWithRetention(logLevel LoggerLogLevel, filename, period string, days int) error {
	logger, closer, err := logging.NewLogger(string(logLevel), filename, period, days)
	if err != nil {
		return err
	}
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()
	if output != nil {
		if err := output.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close previous log output: %v\n", err)
		}
	}
	Logger = logger
	output = closer
	return nil
}

// Close is idempotent and never closes stderr. Later messages use stderr.
func Close() error {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()
	if output == nil {
		return nil
	}
	return output.Close()
}
