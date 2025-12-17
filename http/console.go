package main

import (
	"os"
	"os/signal"
	"syscall"
)

func runConsole(startServer func() error, stopServer func() error) error {
	if err := startServer(); err != nil {
		return err
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	<-quit
	return stopServer()
}

