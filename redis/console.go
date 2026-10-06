package main

import (
	"os"
	"os/signal"
	"syscall"
)

func runConsole(startServer func() error, stopServer func() error) error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(quit)

	if err := startServer(); err != nil {
		return err
	}

	<-quit
	return stopServer()
}
