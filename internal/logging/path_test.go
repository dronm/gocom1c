package logging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogPathsDoNotDependOnServiceWorkingDirectory(t *testing.T) {
	programData := t.TempDir()
	t.Setenv("ProgramData", programData)
	for _, baseName := range []string{"log.txt", "redis1c.log"} {
		name, err := ResolveLogFileName(true, baseName)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(programData, "GoCom1c", "logs", baseName)
		if name != want || !filepath.IsAbs(name) {
			t.Fatalf("log path = %q, want absolute %q", name, want)
		}
		if info, err := os.Stat(filepath.Dir(name)); err != nil || !info.IsDir() {
			t.Fatalf("log directory was not created: %v", err)
		}
	}
}

func TestConsoleLoggingDoesNotRequireProgramData(t *testing.T) {
	t.Setenv("ProgramData", "")
	name, err := ResolveLogFileName(false, "log.txt")
	if name != "" || err != nil {
		t.Fatalf("console path = %q, error = %v", name, err)
	}
	if _, err := ResolveLogFileName(true, "log.txt"); err == nil {
		t.Fatal("file logging without ProgramData should fail")
	}
}

func TestUnwritableLogDirectoryFailsBeforeStartup(t *testing.T) {
	programData := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(programData, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ProgramData", programData)
	if _, err := ResolveLogFileName(true, "redis1c.log"); err == nil {
		t.Fatal("invalid log directory should fail")
	}
}
