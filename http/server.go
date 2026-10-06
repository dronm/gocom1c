package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	com_pool "github.com/dronm/gocom1c"
	"github.com/dronm/gocom1c/http/config"
	"github.com/dronm/gocom1c/http/logger"
	"github.com/gorilla/mux"
)

// Server holds HTTP server state
type Server struct {
	pool      *com_pool.COMPool
	router    *mux.Router
	server    *http.Server
	mu        sync.RWMutex
	cfg       *config.Config
	serveDone chan struct{}
}

// NewServer creates a new HTTP server
func NewServer(cfg *config.Config) (*Server, error) {
	s := &Server{
		router: mux.NewRouter(),
		cfg:    cfg,
	}

	s.setupRoutes()

	return s, nil
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		return fmt.Errorf("server is already running")
	}
	listener, err := net.Listen("tcp", s.cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen for HTTP requests: %w", err)
	}

	// Initialize COM pool
	poolCfg := NewCOMPoolCfg(s.cfg)
	s.pool, err = com_pool.NewCOMPool(poolCfg, logger.Logger)
	if err != nil {
		_ = listener.Close()
		return fmt.Errorf("failed to create COM pool: %w", err)
	}

	s.server = &http.Server{
		Addr:         s.cfg.HTTPAddr,
		Handler:      s.router,
		ReadTimeout:  s.cfg.ReadTimeout.Duration,
		WriteTimeout: s.cfg.WriteTimeout.Duration,
		IdleTimeout:  s.cfg.IdleTimeout.Duration,
	}
	s.serveDone = make(chan struct{})
	server, serveDone := s.server, s.serveDone

	// Start server in goroutine
	go func() {
		defer close(serveDone)
		logger.Logger.Infof("Starting HTTP server on %s", listener.Addr())
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Logger.Errorf("HTTP server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully stops the server
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server == nil {
		return nil
	}

	logger.Logger.Info("Shutting down server...")

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout.Duration)
	defer cancel()
	var stopErr error

	if err := s.server.Shutdown(ctx); err != nil {
		logger.Logger.Errorf("HTTP server shutdown error: %v", err)
		stopErr = errors.Join(stopErr, err, s.server.Close())
	}
	if s.serveDone != nil {
		<-s.serveDone
	}

	// Close COM pool
	if s.pool != nil {
		if err := s.pool.Close(); err != nil {
			logger.Logger.Errorf("COM pool close error: %v", err)
			stopErr = errors.Join(stopErr, err)
		}
		s.pool = nil
	}
	s.server = nil

	logger.Logger.Info("Server stopped successfully")

	return stopErr
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
