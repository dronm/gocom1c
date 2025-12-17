package gocom1c

import (
	"sync"
	"time"

	"github.com/go-ole/go-ole"
)

// COMConnection represents a single COM connection
type COMConnection struct {
	id                int
	v8                *ole.VARIANT
	commandExecParent *ole.VARIANT
	commandExec       *ole.VARIANT
	wg                sync.WaitGroup // all
	quit              chan struct{}
	commands          chan func()
	lastUsed          time.Time
	useCount          int64
	busy              bool
	mutex             sync.RWMutex
}

// GetID returns the connection ID
func (c *COMConnection) GetID() int {
	return c.id
}

// IsBusy returns whether the connection is currently busy
func (c *COMConnection) IsBusy() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.busy
}

// GetLastUsed returns when the connection was last used
func (c *COMConnection) GetLastUsed() time.Time {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.lastUsed
}

func (c *COMConnection) GetUseCount() int64 {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.useCount
}
