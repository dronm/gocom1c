package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dronm/gocom1c/redis/config"
	"github.com/dronm/gocom1c/redis/logger"
)

func TestShutdownWaitsForCommandsWithoutHoldingServerMutex(t *testing.T) {
	dir := t.TempDir()
	if err := logger.Initialize("debug", filepath.Join(dir, "redis1c.log"), "daily"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	server, err := NewRedisServer(&config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	server.isRunning = true
	server.wg.Add(1)
	workerDone := make(chan struct{})
	go func() {
		defer server.wg.Done()
		defer close(workerDone)
		<-server.ctx.Done()
		// Pool-control commands acquire this mutex while completing.
		server.mu.Lock()
		server.mu.Unlock()
		logger.Logger.Info("pending command completed")
	}()
	stopDone := make(chan error, 1)
	go func() { stopDone <- server.Stop() }()
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown blocked a command waiting for the server mutex")
	}
	select {
	case <-workerDone:
	default:
		t.Fatal("shutdown returned before the pending command completed")
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "redis1c-*.log"))
	if err != nil || len(files) != 1 {
		t.Fatalf("log files = %v, error = %v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	commandPos := strings.Index(text, "pending command completed")
	stopPos := strings.Index(text, "Redis server stopped successfully")
	if commandPos < 0 || stopPos <= commandPos {
		t.Fatalf("shutdown log omitted or preceded the command's final message: %s", text)
	}
	if err := server.Stop(); err != nil {
		t.Fatalf("repeated shutdown: %v", err)
	}
}

func TestShutdownTimeoutDoesNotWaitIndefinitelyForCOMCommands(t *testing.T) {
	if err := logger.Initialize("error", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	server, err := NewRedisServer(&config.Config{
		ShutdownTimeout: config.Duration{Duration: 20 * time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	server.isRunning = true
	server.wg.Add(1)
	releaseCommand := make(chan struct{})
	go func() {
		defer server.wg.Done()
		<-releaseCommand
	}()
	t.Cleanup(func() { close(releaseCommand) })
	stopDone := make(chan error, 1)
	go func() { stopDone <- server.Stop() }()
	select {
	case err := <-stopDone:
		if err == nil || !strings.Contains(err.Error(), "timed out waiting") {
			t.Fatalf("expected handler drain timeout, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown waited indefinitely for a stuck COM command")
	}
	if err := server.Start(); err == nil || !strings.Contains(err.Error(), "still finishing") {
		t.Fatalf("server reused resources while an old command was still running: %v", err)
	}
}
