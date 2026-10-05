//go:build windows

package main

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

const serviceDesc = "Go COM 1C HTTP Server"

func runAsService(serviceName string, startServer func() error, stopServer func() error) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}

	if !isService {
		return runConsole(startServer, stopServer)
	}

	return svc.Run(serviceName, &serviceHandler{
		name:  serviceName,
		start: startServer,
		stop:  stopServer,
	})
}

type serviceHandler struct {
	name        string
	start       func() error
	stop        func() error
	reportError func(string, error)
}

func (s *serviceHandler) report(operation string, err error) {
	if s.reportError != nil {
		s.reportError(operation, err)
		return
	}

	message := fmt.Sprintf("%s: %v", operation, err)
	// Keep diagnostics available even when file logger initialization failed or
	// its output has already been closed by the shutdown callback.
	fmt.Fprintln(os.Stderr, message)
	log, openErr := eventlog.Open(s.name)
	if openErr != nil {
		return
	}
	defer log.Close()
	_ = log.Error(1, message)
}

func (s *serviceHandler) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown
	const waitHint = 30000

	current := svc.Status{State: svc.StartPending, CheckPoint: 1, WaitHint: waitHint}
	status <- current
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	startResult := make(chan error, 1)
	go func() {
		startResult <- s.start()
	}()
	var startDone <-chan error = startResult
	var stopDone <-chan error
	stopRequested := false
	var exitCode uint32

	beginStop := func() {
		if current.State != svc.StopPending {
			current = svc.Status{State: svc.StopPending, CheckPoint: 1, WaitHint: waitHint}
			status <- current
		}
		result := make(chan error, 1)
		stopDone = result
		go func() {
			var err error
			if s.stop != nil {
				err = s.stop()
			}
			result <- err
		}()
	}

	requestStop := func() {
		if stopRequested {
			return
		}
		stopRequested = true
		current = svc.Status{State: svc.StopPending, CheckPoint: 1, WaitHint: waitHint}
		status <- current
		if startDone == nil {
			beginStop()
		}
	}

	for {
		select {
		case err := <-startDone:
			startDone = nil
			if err != nil {
				s.report("initialization failed", err)
				exitCode = 1
				if !stopRequested {
					return true, exitCode
				}
			}
			if stopRequested {
				beginStop()
				continue
			}
			current = svc.Status{State: svc.Running, Accepts: accepts}
			status <- current

		case err := <-stopDone:
			if err != nil {
				s.report("shutdown failed", err)
				if exitCode == 0 {
					exitCode = 2
				}
			}
			return exitCode != 0, exitCode

		case c, ok := <-r:
			if !ok {
				r = nil
				requestStop()
				continue
			}
			switch c.Cmd {
			case svc.Interrogate:
				status <- current
			case svc.Stop, svc.Shutdown:
				requestStop()
			}

		case <-ticker.C:
			if current.State == svc.StartPending || current.State == svc.StopPending {
				// Start and Stop callbacks must eventually return. In particular,
				// COM startup cannot be cancelled by this interface, so a stop
				// during initialization waits for it before closing resources.
				current.CheckPoint++
				status <- current
			}
		}
	}
}
