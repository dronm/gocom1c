package logging

import (
	"io"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
)

// NewLogger shares the output and formatting policy used by both executables.
// Closing the returned handle releases the log file and sends later messages to
// stderr, which keeps timed-out COM workers from writing to a closed file.
func NewLogger(level, filename, rotationPeriod string, retentionDays ...int) (*logrus.Logger, io.Closer, error) {
	days, err := retentionSetting(retentionDays)
	if err != nil {
		return nil, nil, err
	}
	if _, err := ParsePeriod(rotationPeriod); err != nil {
		return nil, nil, err
	}
	output := &loggerOutput{writer: os.Stderr}
	if filename != "" {
		writer, err := NewRotatingWriter(filename, rotationPeriod, days)
		if err != nil {
			return nil, nil, err
		}
		output.writer = writer
		output.closer = writer
	}
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	logger.SetLevel(logLevel(level))
	if filename != "" {
		logger.SetOutput(output)
	}
	return logger, output, nil
}

func logLevel(level string) logrus.Level {
	switch level {
	case "debug":
		return logrus.DebugLevel
	case "info":
		return logrus.InfoLevel
	case "warn":
		return logrus.WarnLevel
	case "error":
		return logrus.ErrorLevel
	default:
		return logrus.InfoLevel
	}
}

type loggerOutput struct {
	mu       sync.Mutex
	writer   io.Writer
	closer   io.Closer
	closed   bool
	closeErr error
}

func (o *loggerOutput) Write(data []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.writer.Write(data)
}

func (o *loggerOutput) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return o.closeErr
	}
	o.closed = true
	o.writer = os.Stderr
	if o.closer != nil {
		o.closeErr = o.closer.Close()
	}
	return o.closeErr
}
