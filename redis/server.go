package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	com_pool "github.com/dronm/gocom1c"
	"github.com/dronm/gocom1c/redis/config"
	"github.com/dronm/gocom1c/redis/logger"
	"github.com/redis/go-redis/v9"
)

// RedisServer holds Redis server state
type RedisServer struct {
	pool      *com_pool.COMPool
	redis     *redis.Client
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.RWMutex
	cfg       *config.Config
	isRunning bool
}

// NewRedisServer creates a new Redis server
func NewRedisServer(cfg *config.Config) (*RedisServer, error) {
	ctx, cancel := context.WithCancel(context.Background())

	s := &RedisServer{
		ctx:    ctx,
		cancel: cancel,
		cfg:    cfg,
	}

	return s, nil
}

// Start starts the Redis server
func (s *RedisServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return fmt.Errorf("server is already running")
	}

	// Initialize Redis client
	s.redis = redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", s.cfg.Redis.Host, s.cfg.Redis.Port),
		Password:     s.cfg.Redis.Password,
		Username:     s.cfg.Redis.Username,
		DB:           s.cfg.Redis.DB,
		ReadTimeout:  s.cfg.Redis.ReadTimeout.Duration,
		WriteTimeout: s.cfg.Redis.WriteTimeout.Duration,
		MaxIdleConns: s.cfg.Redis.MaxIdle,
		PoolSize:     s.cfg.Redis.MaxActive,
	})

	// Test Redis connection
	if err := s.redis.Ping(s.ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// Initialize COM pool
	poolCfg := NewCOMPoolCfg(s.cfg)
	var err error
	s.pool, err = com_pool.NewCOMPool(poolCfg, logger.Logger)
	if err != nil {
		return fmt.Errorf("failed to create COM pool: %w", err)
	}

	// Start command processor
	s.wg.Add(1)
	go s.processCommands()

	s.isRunning = true
	logger.Logger.Info("Redis server started successfully")

	return nil
}

// Stop gracefully stops the server
func (s *RedisServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return nil
	}

	logger.Logger.Info("Shutting down Redis server...")

	// Cancel context to stop goroutines
	s.cancel()

	// Wait for goroutines to finish
	s.wg.Wait()

	// Close Redis connection
	if s.redis != nil {
		if err := s.redis.Close(); err != nil {
			logger.Logger.Errorf("Redis connection close error: %v", err)
		}
	}

	// Close COM pool
	if s.pool != nil {
		if err := s.pool.Close(); err != nil {
			logger.Logger.Errorf("COM pool close error: %v", err)
		}
	}

	s.isRunning = false
	logger.Logger.Info("Redis server stopped successfully")

	return nil
}

// processCommands listens for commands from Redis queue
func (s *RedisServer) processCommands() {
	defer s.wg.Done()

	queueName := s.cfg.Redis.CommandQueue
	if queueName == "" {
		queueName = "com1c:commands"
	}

	logger.Logger.Infof("Started processing commands from queue: %s", queueName)

	for {
		// Check if context is cancelled before attempting to pop
		select {
		case <-s.ctx.Done():
			logger.Logger.Info("Command processor stopping due to cancellation")
			return
		default:
			// Continue to BLPOP
		}

		result, err := s.redis.BLPop(s.ctx, s.cfg.Redis.BLPopTimeout.Duration, queueName).Result()
		if err != nil {
			if err == context.Canceled || err == redis.Nil {
				// Context cancelled or timeout, continue to check context
				continue
			}
			logger.Logger.Errorf("Redis BLPOP error: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if len(result) < 2 {
			continue
		}

		// Process command
		commandJSON := result[1]
		logger.Logger.Debugf("Received command: %s", commandJSON)
		go s.handleCommand(commandJSON)
	}
}

func NewCOMPoolCfg(cfg *config.Config) *com_pool.Config {
	return &com_pool.Config{
		ConnectionString: cfg.COM.ConnectionString,
		CommandExec:      cfg.COM.CommandExec,
		MaxPoolSize:      cfg.COM.MaxPoolSize,
		MinPoolSize:      cfg.COM.MinPoolSize,
		IdleTimeout:      cfg.COM.IdleTimeout.Duration,
		COMObjectID:      cfg.COM.COMObjectID,
		WaitConnTimeout:  cfg.COM.WaitConnTimeout.Duration,
		CleanupIdleConn:  cfg.COM.CleanupIdleConn.Duration,
		ConnCloseTimeout: cfg.COM.ConnCloseTimeout.Duration,
	}
}
