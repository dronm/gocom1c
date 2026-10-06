// Package logger
package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Logger is the global logger instance
var Logger *logrus.Logger

type LoggerLogLevel string

const (
	logLevelDebug LoggerLogLevel = "debug"
	logLevelInfo  LoggerLogLevel = "info"
	logLevelWarn  LoggerLogLevel = "warn"
	logLevelError LoggerLogLevel = "error"
)

type LogWriter struct {
	logger *logrus.Logger
}

func NewLogWriter() *LogWriter {
	return &LogWriter{logger: Logger}
}

func (lw *LogWriter) Write(p []byte) (n int, err error) {
	lw.logger.Info(string(p))
	return len(p), nil
}

func Initialize(logLevel LoggerLogLevel, toFile string) error {
	Logger = logrus.New()

	// Set log format (can be JSON or text)
	Logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true, // Show full timestamp
	})

	// Set log level (you can change to logrus.DebugLevel or others)
	Logger.SetLevel(logrusLogLevel(logLevel))

	// Optionally, set output to a file
	if toFile != "" {
		logFile, err := os.OpenFile(toFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
		if err != nil {
			return err
		}
		Logger.SetOutput(logFile)
	}

	return nil
}

func logrusLogLevel(logLevel LoggerLogLevel) logrus.Level {
	var lvl logrus.Level

	switch logLevel {
	case logLevelDebug:
		lvl = logrus.DebugLevel
	case logLevelInfo:
		lvl = logrus.InfoLevel
	case logLevelWarn:
		lvl = logrus.WarnLevel
	case logLevelError:
		lvl = logrus.ErrorLevel
	default:
		lvl = logrus.InfoLevel
	}
	return lvl
}


