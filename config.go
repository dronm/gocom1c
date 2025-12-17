package gocom1c

import "time"

const (
	defMinPoopSize        = 1
	defMaxPoopSize        = 1
	defIdleTimeoutSec     = 5 * 60
	defComObject          = "V83.COMConnector"
	defWaitConnTimeoutSec = 10
	defCleanupIdleConnSec = 60
	defConnCloseTimeout   = 30
)

// Config holds configuration for COM pool
type Config struct {
	ConnectionString string
	CommandExec      string // WebAPI
	MaxPoolSize      int
	MinPoolSize      int
	IdleTimeout      time.Duration
	COMObjectID      string // V83.COMConnector
	WaitConnTimeout  time.Duration
	CleanupIdleConn  time.Duration
	ConnCloseTimeout time.Duration
}

func (cfg *Config) SetDefaults() {
	if cfg.MaxPoolSize <= 0 {
		cfg.MaxPoolSize = defMaxPoopSize
	}
	if cfg.MinPoolSize < 0 {
		cfg.MinPoolSize = defMinPoopSize
	}
	if cfg.MinPoolSize > cfg.MaxPoolSize {
		cfg.MinPoolSize = cfg.MaxPoolSize
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = defIdleTimeoutSec * time.Second
	}
	if cfg.WaitConnTimeout <= 0 {
		cfg.WaitConnTimeout = defWaitConnTimeoutSec * time.Second
	}
	if cfg.CleanupIdleConn <= 0 {
		cfg.CleanupIdleConn = defCleanupIdleConnSec * time.Second
	}
	if cfg.ConnCloseTimeout <= 0 {
		cfg.ConnCloseTimeout = defConnCloseTimeout * time.Second
	}
	if cfg.COMObjectID == "" {
		cfg.COMObjectID = defComObject
	}
}
