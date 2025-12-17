// Package config is redis broker configuration.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Config struct {
	// Redis configuration
	Redis RedisConfig `json:"redis"`
	// COM configuration
	COM COMConfig `json:"com"`
	// Common configuration
	LogLevel        string   `json:"log_level"`
	LogToFile       bool     `json:"log_to_file"`
	ShutdownTimeout Duration `json:"shutdownTimeout"`
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
	PLPopTimeout Duration `json:"blpopTimeout"`

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
	if c.Redis.PLPopTimeout.Duration == 0 {
		c.Redis.PLPopTimeout.Duration = defPLPopTimeout
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
	defPLPopTimeout  = 1 * time.Second
)
