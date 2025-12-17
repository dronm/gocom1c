//go:build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

func installService(serviceName string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service already exists")
	}

	s, err = m.CreateService(
		serviceName,
		exe,
		mgr.Config{
			DisplayName: serviceDesc,
			StartType:   mgr.StartAutomatic,
		},
		"--service", "run",
	)
	if err != nil {
		return err
	}
	defer s.Close()

	eventlog.InstallAsEventCreate(serviceName, eventlog.Info|eventlog.Error)
	return nil
}

func uninstallService(serviceName string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return err
	}
	defer s.Close()

	eventlog.Remove(serviceName)
	return s.Delete()
}

