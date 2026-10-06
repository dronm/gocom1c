// Package config is an application configuration.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const (
	DefLogFileName     = "log.txt"
	defLogLevel        = "debug"
	defShutdownTimeout = 10 * time.Second
)

const (
	// HTTP defaults
	defHTTPAddr         = ":8080"
	defHTTPReadTimeout  = 120 * time.Second
	defHTTPWriteTimeout = 30 * time.Second
	defHTTPIdleTimeout  = 60 * time.Second
)

type COMConfig struct {
	ConnectionString string   `json:"connectionString"`
	CommandExec      string   `json:"commandExec"` // WebAPI
	MaxPoolSize      int      `json:"maxPoolSize"`
	MinPoolSize      int      `json:"minPoolSize"`
	IdleTimeout      Duration `json:"idleTimeout"`
	COMObjectID      string   `json:"comObjectID"` // V83.COMConnector
	WaitConnTimeout  Duration `json:"waitConnTimeout"`
	CleanupIdleConn  Duration `json:"cleanupIdleConn"`
	ConnCloseTimeout Duration `json:"connCloseTimeout"`
}

type Auth struct {
	RequireAuth bool   `json:"requireAuth"`
	Username    string `json:"username"`
	Password    string `json:"password"`
}

type Config struct {
	LogLevel        string   `json:"logLevel"`
	LogToFile       bool     `json:"logToFile"`
	ShutdownTimeout Duration `json:"shutdownTimeout"`

	Auth Auth `json:"auth"`

	HTTPAddr     string   `json:"httpAddr"`
	ReadTimeout  Duration `json:"readTimeout"`
	WriteTimeout Duration `json:"writeTimeout"`
	IdleTimeout  Duration `json:"idleTimeout"`

	COM COMConfig `json:"com"`
}

// ReadConf reads configuration from json file
func (c *Config) ReadConf(fileName string) error {
	file, err := os.ReadFile(fileName)
	if err != nil {
		return fmt.Errorf("os.ReadFile(): %v", err)
	}

	file = bytes.TrimPrefix(file, []byte("\xef\xbb\xbf"))
	if err := json.Unmarshal([]byte(file), c); err != nil {
		return fmt.Errorf("json.Unmarshal():%v", err)
	}

	if c.LogLevel == "" {
		c.LogLevel = defLogLevel
	}

	if c.ShutdownTimeout.Duration == 0 {
		c.ShutdownTimeout.Duration = defShutdownTimeout
	}

	if c.HTTPAddr == "" {
		c.HTTPAddr = defHTTPAddr
	}

	if c.ReadTimeout.Duration == 0 {
		c.ReadTimeout.Duration = defHTTPReadTimeout
	}
	if c.WriteTimeout.Duration == 0 {
		c.WriteTimeout.Duration = defHTTPWriteTimeout
	}
	if c.IdleTimeout.Duration == 0 {
		c.IdleTimeout.Duration = defHTTPIdleTimeout
	}

	return nil
}

// Duration is a wrapper for time.Duration with custom JSON unmarshaling
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case float64:
		// If it's a number, assume it's seconds
		d.Duration = time.Duration(value * float64(time.Second))
		return nil
	case string:
		// Parse duration string (e.g., "30s", "2m", "1h")
		var err error
		d.Duration, err = time.ParseDuration(value)
		return err
	default:
		return fmt.Errorf("invalid duration: %v", v)
	}
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}
