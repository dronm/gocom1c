//go:build windows

package main

import (
	"fmt"

	"golang.org/x/sys/windows/svc"
)

const serviceDesc = "Go COM 1C HTTP Server"

func runAsService(serviceName string, startServer func() error, stopServer func() error) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}

	if !isService {
		// Console mode
		return runConsole(startServer, stopServer)
	}

	// Service mode
	return svc.Run(serviceName, &serviceHandler{
		start: startServer,
		stop:  stopServer,
	})
}

type serviceHandler struct {
	start func() error
	stop  func() error
}

func (s *serviceHandler) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown

	status <- svc.Status{State: svc.StartPending}
	status <- svc.Status{State: svc.Running, Accepts: accepts}

    initDone := make(chan bool, 1)
    initErr := make(chan error, 1)
    
    go func() {
		//COM init
        if err := s.start(); err != nil {
            initErr <- err
            return
        }
        initDone <- true
    }()

    for {
        select {
        case <-initDone:
            // Initialization completed successfully
            // Continue normal operation
            
        case err := <-initErr:
            // Initialization failed
			fmt.Printf("initialization failed: %v\n", err)
            
        case c := <-r:
            switch c.Cmd {
            case svc.Interrogate:
                status <- c.CurrentStatus
                
            case svc.Stop, svc.Shutdown:
                // Stop was requested even during initialization
                status <- svc.Status{State: svc.StopPending}
                if s.stop != nil {
                    _ = s.stop()
                }
                return false, 0
            }
        }
    }
}
