package main

import "net/http"

// setupRoutes configures HTTP routes
func (s *Server) setupRoutes() {
	// Health check
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")

	// Protected routes
	protected := s.router.PathPrefix("/").Subrouter()
	if s.cfg.Auth.RequireAuth {
		protected.Use(s.basicAuthMiddleware)
	}

	// Execute command
	protected.HandleFunc("/execute", s.handleExecute).Methods("POST")
	protected.HandleFunc("/bin-data", s.handleGetBinData).Methods("POST")

	protected.HandleFunc("/stop", s.handleStop).Methods("POST")
	protected.HandleFunc("/start", s.handleStart).Methods("POST")

	// Pool status
	protected.HandleFunc("/status", s.handlePoolStatus).Methods("GET")

	// 404 handler
	protected.NotFoundHandler = http.HandlerFunc(s.handleNotFound)

	// Add middleware
	s.router.Use(s.loggingMiddleware)
	s.router.Use(s.recoveryMiddleware)
}
