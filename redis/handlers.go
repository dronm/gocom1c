package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"

	com_pool "github.com/dronm/gocom1c"
	"github.com/dronm/gocom1c/redis/logger"
)

const errPoolNotInitialized = "pool not initialized"

// RedisCommand structure for Redis commands
type RedisCommand struct {
	Command   string          `json:"command"`
	Params    json.RawMessage `json:"params"`
	RequestID string          `json:"request_id"`
	Channel   string          `json:"channel"` // Response channel override
}

// RedisResponse structure for Redis responses
type RedisResponse struct {
	RequestID string    `json:"request_id"`
	Success   bool      `json:"success"`
	Payload   any       `json:"payload,omitempty"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Channel   string    `json:"channel,omitempty"` // Response channel
}

// handleCommand processes a single Redis command
func (s *RedisServer) handleCommand(commandJSON string) {
	logger.Logger.Debugf("=== Received command: %s", commandJSON)

	var cmd RedisCommand
	if err := json.Unmarshal([]byte(commandJSON), &cmd); err != nil {
		logger.Logger.Errorf("Failed to unmarshal command: %v", err)
		return
	}

	if cmd.RequestID == "" {
		cmd.RequestID = generateRequestID()
	}

	logger.Logger.Debugf("Processing command: %s, RequestID: %s", cmd.Command, cmd.RequestID)

	response := s.executeCommand(&cmd)

	// Set response channel from command if provided
	if cmd.Channel != "" {
		response.Channel = cmd.Channel
	}

	logger.Logger.Debugf("Sending response for RequestID: %s, Success: %v",
		response.RequestID, response.Success)

	// Send response
	s.sendResponse(response)

	logger.Logger.Debugf("=== Command processing completed for: %s", cmd.RequestID)
}

// executeCommand executes the COM command
func (s *RedisServer) executeCommand(cmd *RedisCommand) *RedisResponse {
	response := &RedisResponse{
		RequestID: cmd.RequestID,
		Timestamp: time.Now(),
	}

	// Validate pool
	if s.pool == nil {
		response.Success = false
		response.Error = errPoolNotInitialized
		return response
	}

	// Handle special commands
	switch cmd.Command {
	case "health":
		response.Success = true
		response.Payload = "OK"
		return response

	case "status":
		status := s.getPoolStatus()
		response.Success = true
		response.Payload = status
		return response

	case "start":
		if err := s.startPool(); err != nil {
			response.Success = false
			response.Error = err.Error()
		} else {
			response.Success = true
		}
		return response

	case "stop":
		if err := s.stopPool(); err != nil {
			response.Success = false
			response.Error = err.Error()
		} else {
			response.Success = true
		}
		return response
	}

	// Execute COM command
	startTime := time.Now()
	result, err := s.executeCOMCommand(cmd.Command, cmd.Params)
	duration := time.Since(startTime)

	if err != nil {
		logger.Logger.Errorf("Command execution failed: %s, error: %v, duration: %v",
			cmd.Command, err, duration)
		response.Success = false
		response.Error = err.Error()
		return response
	}

	logger.Logger.Infof("Command executed successfully: %s, duration: %v",
		cmd.Command, duration)

	response.Success = true
	response.Payload = result
	return response
}

// executeCOMCommand executes a COM command with params
func (s *RedisServer) executeCOMCommand(command string, params json.RawMessage) (any, error) {
	paramsStr := s.prepareParams(params)

	result, err := s.pool.ExecuteCommand(command, paramsStr)
	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, nil
	}

	// Parse COM response
	var comResponse struct {
		Success bool   `json:"success"`
		Payload any    `json:"payload,omitempty"`
		Error   string `json:"error,omitempty"`
	}

	if err := json.Unmarshal(result, &comResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal COM response: %w", err)
	}

	if !comResponse.Success {
		errMsg := "unknown error"
		if comResponse.Error != "" {
			errMsg = comResponse.Error
		}
		return nil, errors.New(errMsg)
	}

	// Handle binary data
	if fileName, ok := comResponse.Payload.(string); ok {
		// Check if it's a file path
		if _, err := os.Stat(fileName); err == nil {
			// It's a file, read and return as base64
			return s.handleBinaryFile(fileName)
		}
	}

	return comResponse.Payload, nil
}

// handleBinaryFile reads a binary file and converts it
func (s *RedisServer) handleBinaryFile(fileName string) (any, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Determine content type
	contentType := s.getContentType(file, fileName)

	// Read file content
	content := make([]byte, fileInfo.Size())
	_, err = file.Read(content)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return map[string]any{
		"filename":     filepath.Base(fileName),
		"content_type": contentType,
		"size":         fileInfo.Size(),
		"data":         content, // This could be base64 encoded if needed
	}, nil
}

// getPoolStatus returns COM pool status
func (s *RedisServer) getPoolStatus() map[string]any {
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

	return status
}

// startPool starts the COM pool
func (s *RedisServer) startPool() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pool != nil {
		return fmt.Errorf("pool already started")
	}

	poolCfg := NewCOMPoolCfg(s.cfg)
	var err error
	s.pool, err = com_pool.NewCOMPool(poolCfg, logger.Logger)
	if err != nil {
		return fmt.Errorf("failed to create COM pool: %w", err)
	}

	return nil
}

// stopPool stops the COM pool
func (s *RedisServer) stopPool() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pool == nil {
		return fmt.Errorf("pool not initialized")
	}

	if err := s.pool.Close(); err != nil {
		return fmt.Errorf("failed to close pool: %w", err)
	}

	s.pool = nil
	return nil
}

// prepareParams converts request params to string format for COM pool
func (s *RedisServer) prepareParams(params json.RawMessage) string {
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

// getContentType determines the MIME type for a file
func (s *RedisServer) getContentType(file *os.File, fileName string) string {
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

// sendResponse sends response to Redis
func (s *RedisServer) sendResponse(response *RedisResponse) {
	responseJSON, err := json.Marshal(response)
	if err != nil {
		logger.Logger.Errorf("Failed to marshal response: %v", err)
		return
	}

	// Use provided channel or default queue
	channel := response.Channel
	if channel == "" {
		channel = s.cfg.Redis.ResponseQueue
		if channel == "" {
			channel = "com1c:responses"
		}
	}

	logger.Logger.Infof("Attempting to send response to queue: %s", channel)
	logger.Logger.Debugf("Response JSON size: %d bytes", len(responseJSON))

	// Try to publish first (Pub/Sub)
	if err := s.redis.Publish(s.ctx, channel, responseJSON).Err(); err != nil {
		logger.Logger.Warnf("Failed to publish to channel %s: %v, falling back to queue", channel, err)
		// Fallback to RPUSH to a queue
		s.redis.RPush(s.ctx, channel, responseJSON)
	}

	queueLen, _ := s.redis.LLen(s.ctx, channel).Result()
		logger.Logger.Infof("Response sent successfully to %s. Queue length: %d", 
			channel, queueLen)
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), os.Getpid())
}
