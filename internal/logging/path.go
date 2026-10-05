package logging

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveLogFileName gives both Windows applications a predictable writable
// destination, independent of the service's working directory.
func ResolveLogFileName(logToFile bool, baseName string) (string, error) {
	if !logToFile {
		return "", nil
	}
	programData := os.Getenv("ProgramData")
	if programData == "" {
		return "", fmt.Errorf("ProgramData env variable is empty")
	}
	logDir, err := filepath.Abs(filepath.Join(programData, "GoCom1c", "logs"))
	if err != nil {
		return "", fmt.Errorf("resolve log directory: %w", err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create log directory %q: %w", logDir, err)
	}
	return filepath.Join(logDir, baseName), nil
}
