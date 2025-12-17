package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	com_pool "github.com/dronm/gocom1c"
	"github.com/dronm/gocom1c/http/logger"
)

const errPoolNotInitialized = "pool not initialized"

// APIRequest structure for API calls
type APIRequest struct {
	Command string          `json:"command"`
	Params  json.RawMessage `json:"params"`
}

// APIResponse structure for API calls
type APIResponse struct {
	Success bool   `json:"success"`
	Payload any    `json:"payload,omitempty"`
	Error   string `json:"error,omitempty"`
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	response := APIResponse{
		Success: true,
		Payload: "OK",
	}
	s.respondJSON(w, http.StatusOK, response)
}

// handlePoolStatus returns COM pool status
func (s *Server) handlePoolStatus(w http.ResponseWriter, r *http.Request) {
	status := make(map[string]any)

	var statusDescr string
	if s.pool != nil {
		statusDescr = "running"
		status["connStatuses"] = s.pool.ConnStatuses()
		status["connCount"] = s.pool.ActiveCount()
	} else {
		statusDescr = "stopped"
	}
	status["status"] = statusDescr

	response := APIResponse{
		Success: true,
		Payload: status,
	}
	s.respondJSON(w, http.StatusOK, response)
}

// handleNotFound handles 404 errors
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	s.respondError(w, http.StatusNotFound, "endpoint not found")
}

// loggingMiddleware logs all requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create response writer wrapper to capture status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		logger.Logger.Debugf("%s %s %d %v", r.Method, r.URL.Path, rw.statusCode, duration)
	})
}

// recoveryMiddleware recovers from panics
func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Logger.Warnf("panic recovered: %v", err)
				s.respondError(w, http.StatusInternalServerError, "internal server error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// respondJSON sends JSON response
func (s *Server) respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Logger.Errorf("json.NewEncoder(): %v", err)
	}
}

// respondError sends error response
func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	response := APIResponse{
		Success: false,
		Error:   message,
	}
	s.respondJSON(w, status, response)
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// stop stops all com connections
func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if s.pool == nil {
		s.respondError(w, http.StatusBadGateway, errPoolNotInitialized)
		return
	}
	if err := s.pool.Close(); err != nil {
		logger.Logger.Errorf("pool.Close(): %v", err)
	}
	s.pool = nil
	s.respondJSON(w, http.StatusOK, nil)
}

// start starts min number of connections
func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	poolCfg := NewCOMPoolCfg(s.cfg)
	var err error
	s.pool, err = com_pool.NewCOMPool(poolCfg, logger.Logger)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Errorf("NewCOMPool(): %v", err).Error())
		return
	}
	s.respondJSON(w, http.StatusOK, nil)
}

// handleExecute handles command execution with JSON response
func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	s.handleCommand(w, r, false)
}

// handleGetBinData handles command execution and returns binary file
func (s *Server) handleGetBinData(w http.ResponseWriter, r *http.Request) {
	s.handleCommand(w, r, true)
}

// handleCommand is the common handler for both JSON and binary responses
func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request, returnBinary bool) {
	// Common validation
	if s.pool == nil {
		s.respondError(w, http.StatusBadGateway, errPoolNotInitialized)
		return
	}

	// Parse request
	req, err := s.parseRequest(r)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Execute command with common logic
	paramsStr := s.prepareParams(req.Params)

	logger.Logger.Debugf("Executing command: %s, params: %s", req.Command, req.Params)

	startTime := time.Now()
	result, err := s.pool.ExecuteCommand(req.Command, paramsStr)
	duration := time.Since(startTime)

	// Handle execution error
	if err != nil {
		logger.Logger.Errorf("Command execution failed: %s, error: %v, duration: %v",
			req.Command, err, duration)

		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resultAPI := APIResponse{Success: true}
	if len(result) > 0 {
		if err := json.Unmarshal(result, &resultAPI); err != nil {
			s.respondError(w, http.StatusInternalServerError, fmt.Errorf("com response Unmarshal(): %v", err).Error())
			return
		}
	}
	if !resultAPI.Success {
		errT := "unknown error"
		if resultAPI.Error != "" {
			errT = resultAPI.Error
		}
		s.respondError(w, http.StatusBadRequest, errT)
		return
	}

	logger.Logger.Infof("Command executed successfully: %s, duration: %v",
		req.Command, duration)

	// Handle response based on type
	if returnBinary {
		s.handleBinaryResponse(w, &resultAPI)
	} else {
		s.handleJSONResponse(w, &resultAPI)
	}
}

// parseRequest parses JSON request body
func (s *Server) parseRequest(r *http.Request) (*APIRequest, error) {
	var req APIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid JSON request")
	}

	if req.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	return &req, nil
}

// prepareParams converts request params to string format for COM pool
func (s *Server) prepareParams(params json.RawMessage) string {
	if params == nil {
		return "null"
	}

	paramsStr := string(params)
	if len(paramsStr) == 0 {
		return "null"
	}

	// If it's a JSON object/array, keep it as JSON string
	// If it's a simple value, 1C might expect a string
	if paramsStr[0] != '{' && paramsStr[0] != '[' {
		// Simple value, quote it as string for 1C
		return fmt.Sprintf(`"%s"`, paramsStr)
	}

	return paramsStr
}

// handleJSONResponse processes successful command execution for JSON responses
func (s *Server) handleJSONResponse(w http.ResponseWriter, response *APIResponse) {
	s.respondJSON(w, http.StatusOK, response)
}

// handleBinaryResponse processes successful command execution for binary responses
func (s *Server) handleBinaryResponse(w http.ResponseWriter, response *APIResponse) {
	fileName, ok := response.Payload.(string)
	if !ok {
		s.respondError(w, http.StatusNotFound, "payload can not be cast to string")
		return
	}
	logger.Logger.Debugf("handleBinaryResponse fileName:%s", fileName)
	file, err := os.Open(fileName)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "file not found")
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Determine content type
	contentType := s.getContentType(file, fileName)

	// Set headers
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(fileName)+"\"")
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))

	// Stream the file with buffer
	s.streamFile(w, file)
}

// getContentType determines the MIME type for a file
func (s *Server) getContentType(file *os.File, fileName string) string {
	// First try to get from file extension
	contentType := mime.TypeByExtension(filepath.Ext(fileName))
	if contentType != "" {
		return contentType
	}

	// Fallback to content detection from first 512 bytes
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		// If we can't read, default to octet-stream
		return "application/octet-stream"
	}

	// Reset file pointer to beginning
	file.Seek(0, 0)

	if n == 0 {
		return "application/octet-stream"
	}

	return http.DetectContentType(buffer[:n])
}

// streamFile efficiently streams a file to the HTTP response
func (s *Server) streamFile(w http.ResponseWriter, file *os.File) {
	const bufferSize = 32 * 1024 // 32KB buffer

	bufWriter := bufio.NewWriterSize(w, bufferSize)
	_, err := io.Copy(bufWriter, file)
	if err != nil {
		// Log error but headers already sent
		logger.Logger.Errorf("Streaming error: %v", err)
	}
	bufWriter.Flush()
}
