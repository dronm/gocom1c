//go:build windows

package main

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

type serviceExit struct {
	specific bool
	code     uint32
}

func executeService(t *testing.T, handler *serviceHandler) (chan svc.ChangeRequest, <-chan svc.Status, <-chan serviceExit) {
	t.Helper()
	requests := make(chan svc.ChangeRequest, 4)
	statuses := make(chan svc.Status, 32)
	done := make(chan serviceExit, 1)
	go func() {
		specific, code := handler.Execute(nil, requests, statuses)
		done <- serviceExit{specific: specific, code: code}
	}()
	return requests, statuses, done
}

func nextServiceStatus(t *testing.T, statuses <-chan svc.Status) svc.Status {
	t.Helper()
	select {
	case status := <-statuses:
		return status
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for service status")
		return svc.Status{}
	}
}

func nextServiceExit(t *testing.T, done <-chan serviceExit) serviceExit {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for service exit")
		return serviceExit{}
	}
}

func TestServiceInitializationFailure(t *testing.T) {
	failure := errors.New("configuration unavailable")
	diagnostics := make(chan error, 1)
	_, statuses, done := executeService(t, &serviceHandler{
		start: func() error { return failure },
		reportError: func(operation string, err error) {
			if operation != "initialization failed" {
				t.Errorf("unexpected operation %q", operation)
			}
			diagnostics <- err
		},
	})
	if status := nextServiceStatus(t, statuses); status.State != svc.StartPending {
		t.Fatalf("initial status = %v, want StartPending", status.State)
	}
	if exit := nextServiceExit(t, done); !exit.specific || exit.code != 1 {
		t.Fatalf("startup failure result = %+v, want service-specific code 1", exit)
	}
	if err := <-diagnostics; !errors.Is(err, failure) {
		t.Fatalf("diagnostic error = %v, want %v", err, failure)
	}
	select {
	case status := <-statuses:
		if status.State == svc.Running {
			t.Fatal("failed initialization reported Running")
		}
	default:
	}
}

func TestServiceStopWaitsForInitialization(t *testing.T) {
	allowStart := make(chan struct{})
	allowStop := make(chan struct{})
	stopStarted := make(chan struct{})
	var stopCalls atomic.Int32
	requests, statuses, done := executeService(t, &serviceHandler{
		start: func() error {
			<-allowStart
			return nil
		},
		stop: func() error {
			stopCalls.Add(1)
			close(stopStarted)
			<-allowStop
			return nil
		},
	})
	if status := nextServiceStatus(t, statuses); status.State != svc.StartPending || status.CheckPoint == 0 || status.WaitHint == 0 {
		t.Fatalf("initial status = %+v, want StartPending with progress values", status)
	}
	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	firstPending := nextServiceStatus(t, statuses)
	if firstPending.State != svc.StopPending {
		t.Fatalf("stop status = %v, want StopPending", firstPending.State)
	}
	requests <- svc.ChangeRequest{Cmd: svc.Interrogate}
	if status := nextServiceStatus(t, statuses); status.State != svc.StopPending {
		t.Fatalf("interrogate status = %v, want StopPending", status.State)
	}
	// Initialization may be slow; SCM still receives checkpoint updates while
	// the stop callback waits for startup to release its resources.
	if status := nextServiceStatus(t, statuses); status.State != svc.StopPending || status.CheckPoint <= firstPending.CheckPoint {
		t.Fatalf("pending status = %+v, want an increased StopPending checkpoint", status)
	}
	select {
	case <-stopStarted:
		t.Fatal("shutdown started before initialization returned")
	case <-done:
		t.Fatal("service exited while initialization was still running")
	default:
	}
	close(allowStart)
	select {
	case <-stopStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown was not called after initialization completed")
	}
	requests <- svc.ChangeRequest{Cmd: svc.Shutdown}
	close(allowStop)
	if exit := nextServiceExit(t, done); exit.specific || exit.code != 0 {
		t.Fatalf("successful shutdown result = %+v", exit)
	}
	if got := stopCalls.Load(); got != 1 {
		t.Fatalf("shutdown callback calls = %d, want 1", got)
	}
	for len(statuses) > 0 {
		if status := <-statuses; status.State == svc.Running {
			t.Fatal("service reported Running after stop was requested")
		}
	}
}

func TestServiceReportsRunningAfterInitialization(t *testing.T) {
	allowStart := make(chan struct{})
	failure := errors.New("log close failed")
	diagnostics := make(chan error, 1)
	requests, statuses, done := executeService(t, &serviceHandler{
		start: func() error {
			<-allowStart
			return nil
		},
		stop: func() error { return failure },
		reportError: func(operation string, err error) {
			if operation != "shutdown failed" {
				t.Errorf("unexpected operation %q", operation)
			}
			diagnostics <- err
		},
	})
	if status := nextServiceStatus(t, statuses); status.State != svc.StartPending {
		t.Fatalf("initial status = %v, want StartPending", status.State)
	}
	requests <- svc.ChangeRequest{Cmd: svc.Interrogate}
	if status := nextServiceStatus(t, statuses); status.State != svc.StartPending {
		t.Fatalf("startup interrogate status = %v, want StartPending", status.State)
	}
	close(allowStart)
	if status := nextServiceStatus(t, statuses); status.State != svc.Running || status.Accepts != svc.AcceptStop|svc.AcceptShutdown {
		t.Fatalf("initialized status = %+v, want Running accepting stop/shutdown", status)
	}
	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	if status := nextServiceStatus(t, statuses); status.State != svc.StopPending {
		t.Fatalf("stop status = %v, want StopPending", status.State)
	}
	if exit := nextServiceExit(t, done); !exit.specific || exit.code != 2 {
		t.Fatalf("shutdown failure result = %+v, want service-specific code 2", exit)
	}
	if err := <-diagnostics; !errors.Is(err, failure) {
		t.Fatalf("diagnostic error = %v, want %v", err, failure)
	}
}
