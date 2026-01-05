package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	com_pool "github.com/dronm/gocom1c"
)

type SimpleLogger struct{}

func (l *SimpleLogger) Infof(format string, args ...any) {
	log.Printf("INFO: "+format, args...)
}

func (l *SimpleLogger) Errorf(format string, args ...any) {
	log.Printf("ERROR: "+format, args...)
}

func (l *SimpleLogger) Debugf(format string, args ...any) {
	log.Printf("DEBUG: "+format, args...)
}

func (l *SimpleLogger) Warnf(format string, args ...any) {
	log.Printf("WARN: "+format, args...)
}

func main() {
	cfg := com_pool.Config{
		ConnectionString: `Srvr="vds484";Ref="21315_576_60751";Usr="Михалевич АА";Pwd="jU5gujas"`,
		CommandExec:      "WebAPI",
		MaxPoolSize:      1,
		MinPoolSize:      1,
		IdleTimeout:      10 * time.Minute,
		COMObjectID:      "V83.COMConnector",
	}

	logger := &SimpleLogger{}

	// Create pool
	pool, err := com_pool.NewCOMPool(&cfg, logger)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	// Execute multiple commands concurrently
	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			params := map[string]any{
				"client_ref": fmt.Sprintf("client_%d", id),
				"products": []map[string]any{
					{"ref": "22222", "name": "ProductA"},
					{"ref": "33333", "name": "ProductB"},
				},
			}
			paramsB , err := json.Marshal(params)
			if err != nil {
				log.Printf("json.Marshal():%v", err)
				return
			}
			result, err := pool.ExecuteCommand("TestMethod", string(paramsB))
			if err != nil {
				log.Printf("Request %d failed: %v", id, err)
			} else {
				log.Printf("Request %d succeeded: %s", id, result)
			}
		}(i)
	}

	wg.Wait()
}
