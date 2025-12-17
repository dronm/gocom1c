//go:build !windows
// +build !windows

package main

import "errors"

var winServiceName string

func runAsService(serviceName string, startServer func() error, stopServer func() error) error {
	return errors.New("windows service mode is only supported on Windows")
}

func installService(serviceName string) error {
	return errors.New("service install is only supported on Windows")
}

func uninstallService(serviceName string) error {
	return errors.New("service uninstall is only supported on Windows")
}

