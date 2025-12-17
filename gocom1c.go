// Package gocom1c is a COM pool connector to 1c bases.
package gocom1c

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-ole/go-ole/oleutil"
)

// COMPool manages a pool of COM connections
type COMPool struct {
	cfg         *Config
	connections []*COMConnection
	freeConn    chan *COMConnection
	createMutex sync.Mutex
	closeOnce   sync.Once
	shutdown    chan struct{}
	logger      Logger
	nextID      int
	activeCount int
	poolMutex   sync.RWMutex
}

// Result represents the result of a COM operation
type Result struct {
	Value any
	Error error
}

// NewCOMPool creates a new COM connection pool
func NewCOMPool(cfg *Config, logger Logger) (*COMPool, error) {
	cfg.SetDefaults()

	pool := &COMPool{
		cfg:         cfg,
		connections: make([]*COMConnection, 0, cfg.MaxPoolSize),
		freeConn:    make(chan *COMConnection, cfg.MaxPoolSize),
		shutdown:    make(chan struct{}),
		logger:      logger,
	}

	// Initialize minimum connections
	if err := pool.InitConnections(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to create initial connection: %w", err)
	}

	// Start cleanup goroutine
	go pool.cleanupIdleConnections()

	return pool, nil
}

// Execute runs a function on a COM connection
func (p *COMPool) Execute(fn func(conn *COMConnection) (any, error)) (any, error) {
	conn, err := p.GetConnection()
	if err != nil {
		return nil, err
	}
	defer p.ReleaseConnection(conn)

	return fn(conn)
}

// ExecuteCommand executes a command on 1C COM object
func (p *COMPool) ExecuteCommand(command string, params string) ([]byte, error) {
	result, err := p.Execute(func(conn *COMConnection) (any, error) {
		return conn.ExecuteCommand(command, params)
	})
	if err != nil {
		return []byte{}, err
	}

	str, ok := result.(string)
	if !ok {
		return []byte{}, fmt.Errorf("result can not be converted to string")
	}
	return []byte(str), nil
}

func (p *COMPool) InitConnections() error {
	// Initialize minimum connections
	for i := 0; i < p.cfg.MinPoolSize; i++ {
		if err := p.createConnection(); err != nil {
			return err
		}
	}
	return nil
}

func (p *COMPool) ConnStatuses() map[int]map[string]any {
	p.poolMutex.Lock()
	defer p.poolMutex.Unlock()

	stat := make(map[int]map[string]any)
	for _, conn := range p.connections {
		stat[conn.id] = map[string]any{
			"useCount": conn.GetUseCount(),
			"lastUsed": conn.GetLastUsed(),
		}
	}
	return stat
}

func (p *COMPool) ActiveCount() int {
	p.poolMutex.Lock()
	defer p.poolMutex.Unlock()

	return p.activeCount
}

// CloseConnections closes all connections
func (p *COMPool) CloseConnections() {
	p.poolMutex.Lock()
	defer p.poolMutex.Unlock()

	for _, conn := range p.connections {
		p.closeConnection(conn)
	}
	p.connections = nil
	p.activeCount = 0
}

// Close shuts down the pool and all connections
func (p *COMPool) Close() error {
	p.closeOnce.Do(func() {
		close(p.shutdown)
		p.CloseConnections()
	})

	return nil
}

func (p *COMPool) cleanup() {
	p.poolMutex.Lock()
	defer p.poolMutex.Unlock()

	if p.activeCount <= p.cfg.MinPoolSize {
		return
	}

	now := time.Now()
	for _, conn := range p.connections {
		if p.activeCount <= p.cfg.MinPoolSize {
			break
		}

		conn.mutex.RLock()
		idle := !conn.busy && now.Sub(conn.lastUsed) > p.cfg.IdleTimeout
		conn.mutex.RUnlock()

		if idle {
			// Try to remove from freeConn channel
			select {
			case c := <-p.freeConn:
				if c.id == conn.id {
					p.closeConnection(conn)
				} else {
					// Put it back
					p.freeConn <- c
				}
			default:
				// No free connections in channel
			}
		}
	}
}

// Add cleanup method to COMConnection
func (c *COMConnection) cleanup() {
	if c.commandExec != nil {
		c.commandExec.Clear()
		c.commandExec = nil
	}
	if c.commandExecParent != nil {
		c.commandExecParent.Clear()
		c.commandExecParent = nil
	}
	if c.v8 != nil {
		c.v8.Clear()
		c.v8 = nil
	}
}

// ExecuteCommand executes a command on this COM connection
func (c *COMConnection) ExecuteCommand(command string, params string) (string, error) {
	resultChan := make(chan Result, 1)

	c.commands <- func() {
		res, err := oleutil.CallMethod(c.commandExec.ToIDispatch(), "ExecuteCommand", command, params)
		if err != nil {
			resultChan <- Result{Error: err}
			return
		}
		defer res.Clear()

		// Convert result to string
		val := res.Value()
		var strVal string
		switch v := val.(type) {
		case string:
			strVal = v
		case fmt.Stringer:
			strVal = v.String()
		default:
			strVal = fmt.Sprintf("%v", v)
		}

		resultChan <- Result{Value: strVal}
	}

	result := <-resultChan
	if result.Error != nil {
		return "", result.Error
	}

	return result.Value.(string), nil
}

// GetConnection acquires a COM connection from the pool
func (p *COMPool) GetConnection() (*COMConnection, error) {
	select {
	case conn := <-p.freeConn:
		conn.mutex.Lock()
		conn.busy = true
		conn.lastUsed = time.Now()
		conn.useCount++
		conn.mutex.Unlock()
		p.logger.Debugf("Reusing connection %d", conn.id)
		return conn, nil
	case <-time.After(p.cfg.WaitConnTimeout):
		// Try to create a new connection if under max pool size
		p.poolMutex.RLock()
		canCreate := p.activeCount < p.cfg.MaxPoolSize
		p.poolMutex.RUnlock()

		if canCreate {
			if err := p.createConnection(); err != nil {
				return nil, fmt.Errorf("failed to create new connection: %w", err)
			}
			return p.GetConnection()
		}
		return nil, fmt.Errorf("timeout waiting for COM connection")
	case <-p.shutdown:
		return nil, fmt.Errorf("pool is shutdown")
	}
}

// ReleaseConnection returns a connection to the pool
func (p *COMPool) ReleaseConnection(conn *COMConnection) {
	conn.mutex.Lock()
	conn.busy = false
	conn.lastUsed = time.Now()
	conn.mutex.Unlock()

	select {
	case p.freeConn <- conn:
		p.logger.Debugf("Released connection %d back to pool", conn.id)
	default:
		// Pool is full, close this connection
		p.logger.Debugf("Pool full, closing connection %d", conn.id)
		p.closeConnection(conn)
	}
}

// createConnection creates a new COM connection
func (p *COMPool) createConnection() error {
	p.createMutex.Lock()
	defer p.createMutex.Unlock()

	p.poolMutex.Lock()
	defer p.poolMutex.Unlock()

	if p.activeCount >= p.cfg.MaxPoolSize {
		return fmt.Errorf("maximum pool size reached")
	}

	conn := &COMConnection{
		id:       p.nextID,
		quit:     make(chan struct{}),
		commands: make(chan func(), 100),
		lastUsed: time.Now(),
		busy:     false,
	}
	p.nextID++

	// Start COM worker goroutine
	ready := make(chan error, 1)
	go conn.comWorker(p.cfg, ready, p.logger)

	// Wait for initialization
	if err := <-ready; err != nil {
		return fmt.Errorf("failed to initialize COM connection %d: %w", conn.id, err)
	}

	p.connections = append(p.connections, conn)
	p.activeCount++

	// Add to free connections pool
	select {
	case p.freeConn <- conn:
		p.logger.Infof("Created COM connection %d, total active: %d", conn.id, p.activeCount)
	default:
		// Should not happen since we just created it
	}

	return nil
}

// closeConnection closes a specific connection
func (p *COMPool) closeConnection(conn *COMConnection) {
	close(conn.quit)

	// Wait for worker to finish (with timeout)
	done := make(chan struct{})
	go func() {
		conn.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Worker shutdown complete
	case <-time.After(p.cfg.ConnCloseTimeout):
		p.logger.Warnf("COM connection %d worker shutdown timeout", conn.id)
	}

	// Remove from connections slice
	for i, c := range p.connections {
		if c.id == conn.id {
			p.connections = append(p.connections[:i], p.connections[i+1:]...)
			p.activeCount--
			p.logger.Infof("Closed COM connection %d, remaining: %d", conn.id, p.activeCount)
			break
		}
	}
}

// cleanupIdleConnections removes idle connections
func (p *COMPool) cleanupIdleConnections() {
	ticker := time.NewTicker(p.cfg.CleanupIdleConn)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.cleanup()
		case <-p.shutdown:
			return
		}
	}
}
