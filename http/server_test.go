package main

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dronm/gocom1c/http/config"
	"github.com/dronm/gocom1c/http/logger"
)

func TestHTTPShutdownClosesListenerBeforeLogOutput(t *testing.T) {
	dir := t.TempDir()
	if err := logger.Initialize("debug", filepath.Join(dir, "log.txt"), "daily"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	cfg := &config.Config{
		HTTPAddr:        "127.0.0.1:0",
		ShutdownTimeout: config.Duration{Duration: time.Second},
	}
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	if err := server.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-server.serveDone:
	default:
		t.Fatal("HTTP serving goroutine is still running after shutdown")
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "log-*.txt"))
	if err != nil || len(files) != 1 {
		t.Fatalf("log files = %v, error = %v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil || !strings.Contains(string(data), "Server stopped successfully") {
		t.Fatalf("final shutdown record missing: %s (error: %v)", data, err)
	}
	if err := server.Stop(); err != nil {
		t.Fatalf("repeated stop: %v", err)
	}
}

func TestHTTPStartupReturnsListenerErrors(t *testing.T) {
	if err := logger.Initialize("info", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server, err := NewServer(&config.Config{HTTPAddr: listener.Addr().String()})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err == nil {
		t.Fatal("startup succeeded while the HTTP port was already occupied")
	}
	if server.pool != nil || server.server != nil {
		t.Fatal("failed startup retained server resources")
	}
}

func TestHTTPShutdownUnblocksHandlerAfterTimeout(t *testing.T) {
	if err := logger.Initialize("info", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	finished := make(chan struct{})
	server := &Server{
		cfg:       &config.Config{ShutdownTimeout: config.Duration{Duration: 20 * time.Millisecond}},
		serveDone: make(chan struct{}),
		server: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(entered)
			<-r.Context().Done()
			close(finished)
		})},
	}
	go func() {
		defer close(server.serveDone)
		_ = server.server.Serve(listener)
	}()
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		resp, err := http.Get("http://" + listener.Addr().String())
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("test request did not reach the handler")
	}
	app := &ServiceApp{srv: server}
	if err := app.Stop(); err == nil {
		t.Fatal("shutdown did not report its deadline")
	}
	if err := app.Start(); err == nil || !strings.Contains(err.Error(), "previous shutdown failed") {
		t.Fatalf("application restarted after an incomplete shutdown: %v", err)
	}
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("timed-out shutdown left the handler connection open")
	}
	<-clientDone
}
