package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dronm/gocom1c/internal/logging"
	"github.com/dronm/gocom1c/redis/config"
	"github.com/dronm/gocom1c/redis/logger"
)

func main() {
	serviceCmd := flag.String("service", "", "install | uninstall | run")
	serviceName := flag.String("service-name", "GoCOM1CRedisService", "Windows service name")
	flag.Parse()

	switch *serviceCmd {
	case "install":
		if err := installService(*serviceName); err != nil {
			panic(err)
		}
		fmt.Println("Service installed")
		return

	case "uninstall":
		if err := uninstallService(*serviceName); err != nil {
			panic(err)
		}
		fmt.Println("Service uninstalled")
		return
	}

	app := &ServiceApp{}

	// service mode
	if *serviceCmd == "run" {
		if err := runAsService(*serviceName, app.Start, app.Stop); err != nil {
			panic(err)
		}
	} else {
		if err := runConsole(app.Start, app.Stop); err != nil {
			panic(err)
		}
	}
}

type ServiceApp struct {
	mu      sync.Mutex
	cfg     *config.Config
	srv     *RedisServer
	stopErr error
}

func (app *ServiceApp) Start() error {
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.srv != nil {
		return fmt.Errorf("server is already running or its previous shutdown failed")
	}
	exeDir, err := getExecutableDir()
	if err != nil {
		return fmt.Errorf("failed to get executable directory: %v", err)
	}
	configPath := filepath.Join(exeDir, "config.json")

	cfg := &config.Config{}

	if err := cfg.ReadConf(configPath); err != nil {
		return fmt.Errorf("failed to read config: %v", err)
	}

	// Initialize logger
	logFileName, err := logging.ResolveLogFileName(cfg.LogToFile, "redis1c.log")
	if err != nil {
		return fmt.Errorf("resolve log filename: %w", err)
	}
	if err := logger.Initialize(logger.LoggerLogLevel(cfg.LogLevel), logFileName, cfg.LogRotationPeriod); err != nil {
		return fmt.Errorf("failed to initialize logger: %v", err)
	}

	// Create server
	srv, err := NewRedisServer(cfg)
	if err != nil {
		return errors.Join(fmt.Errorf("failed to initialize server: %w", err), logger.Close())
	}
	if err := srv.Start(); err != nil {
		logger.Logger.Errorf("Server startup failed: %v", err)
		return errors.Join(fmt.Errorf("failed to start server: %w", err), srv.Stop(), logger.Close())
	}
	app.cfg = cfg
	app.srv = srv
	return nil
}

func (app *ServiceApp) Stop() error {
	app.mu.Lock()
	defer app.mu.Unlock()
	var err error
	if app.srv != nil {
		err = app.srv.Stop()
		app.stopErr = errors.Join(app.stopErr, err)
		if app.stopErr == nil {
			app.srv = nil
			app.cfg = nil
		}
	}
	return errors.Join(app.stopErr, logger.Close())
}

func getExecutableDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exePath), nil
}
