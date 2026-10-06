// Package config is redis broker configuration.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/dronm/gocom1c/internal/logging"
)

type Config struct {
	// Redis configuration
	Redis RedisConfig `json:"redis"`
	// COM configuration
	COM COMConfig `json:"com"`
	// Common configuration
	LogLevel          string   `json:"logLevel"`
	LogToFile         bool     `json:"logToFile"`
	LogRotationPeriod string   `json:"logRotationPeriod"`
	LogRetentionDays  int      `json:"logRetentionDays"`
	ShutdownTimeout   Duration `json:"shutdownTimeout"`
}

type RedisConfig struct {
	// Connection settings
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
	Username string `json:"username"`
	DB       int    `json:"db"`

	// Queue settings
	CommandQueue  string `json:"commandQueue"`
	ResponseQueue string `json:"responseQueue"`

	// Timeouts
	ReadTimeout  Duration `json:"readTimeout"`
	WriteTimeout Duration `json:"writeTimeout"`
	BLPopTimeout Duration `json:"blpopTimeout"`

	// Pool settings
	MaxIdle   int `json:"maxIdle"`
	MaxActive int `json:"maxActive"`
}

type COMConfig struct {
	ConnectionString string `json:"connectionString"`
	CommandExec      string `json:"commandExec"`
	MaxPoolSize      int    `json:"maxPoolSize"`
	MinPoolSize      int    `json:"minPoolSize"`
	COMObjectID      string `json:"comObjectId"`

	IdleTimeout      Duration `json:"idleTimeout"`
	WaitConnTimeout  Duration `json:"waitConnTimeout"`
	CleanupIdleConn  Duration `json:"cleanupIdleConn"`
	ConnCloseTimeout Duration `json:"connCloseTimeout"`
}

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
		d.Duration = time.Duration(value)
	case string:
		var err error
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}
	}
	return nil
}

// ReadConf reads configuration from JSON file
func (c *Config) ReadConf(filename string) error {
	file, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	file = bytes.TrimPrefix(file, []byte("\xef\xbb\xbf"))
	if err := json.Unmarshal([]byte(file), c); err != nil {
		return fmt.Errorf("json.Unmarshal():%v", err)
	}

	if c.LogLevel == "" {
		c.LogLevel = defLogLevel
	}
	if err := c.readLogRotationPeriod(file); err != nil {
		return err
	}
	if err := c.readLogRetentionDays(file); err != nil {
		return err
	}

	if c.ShutdownTimeout.Duration == 0 {
		c.ShutdownTimeout.Duration = defShutdownTimeout
	}

	if c.Redis.Host == "" {
		c.Redis.Host = defRedisHost
	}
	if c.Redis.Port == 0 {
		c.Redis.Port = defRedisPort
	}
	if c.Redis.CommandQueue == "" {
		c.Redis.CommandQueue = defCommandQueue
	}

	if c.Redis.ResponseQueue == "" {
		c.Redis.ResponseQueue = defResponseQueue
	}
	if c.Redis.ReadTimeout.Duration == 0 {
		c.Redis.ReadTimeout.Duration = defReadTimeout
	}
	if c.Redis.WriteTimeout.Duration == 0 {
		c.Redis.WriteTimeout.Duration = defWriteTimeout
	}
	if c.Redis.BLPopTimeout.Duration == 0 {
		c.Redis.BLPopTimeout.Duration = defBLPopTimeout
	}

	return nil
}

func (c *Config) readLogRotationPeriod(data []byte) error {
	var fields struct {
		LogRotationPeriod json.RawMessage `json:"logRotationPeriod"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	value := fields.LogRotationPeriod
	if len(value) == 0 {
		c.LogRotationPeriod = logging.DefaultRotationPeriod
	} else if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || len(bytes.TrimSpace([]byte(c.LogRotationPeriod))) == 0 {
		return fmt.Errorf("logRotationPeriod must be a string: daily or a positive duration")
	}
	if _, err := logging.ParsePeriod(c.LogRotationPeriod); err != nil {
		return fmt.Errorf("logRotationPeriod: %w", err)
	}
	return nil
}

func (c *Config) readLogRetentionDays(data []byte) error {
	var fields struct {
		LogRetentionDays json.RawMessage `json:"logRetentionDays"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	value := fields.LogRetentionDays
	if len(value) == 0 {
		c.LogRetentionDays = logging.DefaultRetentionDays
	} else {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("logRetentionDays must be a nonnegative integer")
		}
		if err := json.Unmarshal(value, &c.LogRetentionDays); err != nil {
			return fmt.Errorf("logRetentionDays must be a nonnegative integer: %w", err)
		}
	}
	if err := logging.ValidateRetentionDays(c.LogRetentionDays); err != nil {
		return fmt.Errorf("logRetentionDays: %w", err)
	}
	return nil
}

// Default Redis configuration values
const (
	defLogLevel        = "debug"
	defShutdownTimeout = 10 * time.Second

	defRedisHost     = "localhost"
	defRedisPort     = 6379
	defCommandQueue  = "com1c:commands"
	defResponseQueue = "com1c:responses"
	defReadTimeout   = 5 * time.Second
	defWriteTimeout  = 5 * time.Second
	defBLPopTimeout  = 1 * time.Second
)
