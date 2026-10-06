package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

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
	cfg *config.Config
	srv *RedisServer
}

func (app *ServiceApp) Start() error {
	// Lazy initialization
	if app.cfg == nil {
		exeDir, err := getExecutableDir()
		if err != nil {
			return fmt.Errorf("failed to get executable directory: %v", err)
		}
		configPath := filepath.Join(exeDir, "config.json")

		cfg := &config.Config{}

		if err := cfg.ReadConf(configPath); err != nil {
			return fmt.Errorf("failed to read config: %v", err)
		}
		app.cfg = cfg

		// Initialize logger
		var logFileName string
		if cfg.LogToFile {
			logFileName = "redis1c.log"
		}
		if err := logger.Initialize(logger.LoggerLogLevel(cfg.LogLevel), logFileName); err != nil {
			return fmt.Errorf("failed to initialize logger: %v", err)
		}

		// Create server
		srv, err := NewRedisServer(cfg)
		if err != nil {
			return fmt.Errorf("failed to initialize server: %v", err)
		}
		app.srv = srv
	}

	return app.srv.Start()
}

func (app *ServiceApp) Stop() error {
	if app.srv != nil {
		return app.srv.Stop()
	}
	return nil
}

func getExecutableDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exePath), nil
}
