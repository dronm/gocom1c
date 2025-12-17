package gocom1c

import (
	"fmt"
	"runtime"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

func (c *COMConnection) comWorker(cfg *Config, ready chan<- error, logger Logger) {
	c.wg.Add(1) 
	defer c.wg.Done()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Initialize COM
	if err := ole.CoInitialize(0); err != nil {
		ready <- fmt.Errorf("CoInitialize failed: %w", err)
		return
	}
	defer ole.CoUninitialize()

	logger.Debugf("initializing COM: %s", cfg.COMObjectID)

	// Create COM connector
	unknown, err := oleutil.CreateObject(cfg.COMObjectID)
	if err != nil {
		ready <- fmt.Errorf("create COMConnector failed: %w", err)
		return
	}
	defer unknown.Release()

	dispatch, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		ready <- fmt.Errorf("QueryInterface failed: %w", err)
		return
	}
	defer dispatch.Release()

	logger.Debugf("trying to connect with: %s", cfg.ConnectionString)

	// Connect to 1C
	c.v8, err = oleutil.CallMethod(dispatch, "Connect", cfg.ConnectionString)
	if err != nil {
		ready <- fmt.Errorf("1C Connect failed: %w", err)
		return
	}
	// DO NOT defer c.v8.Clear() here - we need it for the lifetime of the connection

	// Get справочники
	spr, err := oleutil.GetProperty(c.v8.ToIDispatch(), "Справочники")
	if err != nil {
		c.cleanup()
		ready <- fmt.Errorf("object property 'Справочники' not found: %w", err)
		return
	}
	// DON'T defer spr.Clear() yet

	// Get ДополнительныеОтчетыИОбработки
	sprOtch, err := oleutil.GetProperty(spr.ToIDispatch(), "ДополнительныеОтчетыИОбработки")
	spr.Clear() // Clear spr now that we have sprOtch
	if err != nil {
		c.cleanup()
		ready <- fmt.Errorf("object property 'ДополнительныеОтчетыИОбработки' not found: %w", err)
		return
	}
	// DON'T defer sprOtch.Clear() yet

	// Find обработка by name
	extForm, err := oleutil.CallMethod(sprOtch.ToIDispatch(), "НайтиПоНаименованию", cfg.CommandExec, true)
	sprOtch.Clear() // Clear sprOtch now that we have extForm
	if err != nil {
		c.cleanup()
		ready <- fmt.Errorf("method 'НайтиПоНаименованию()' not found: %w", err)
		return
	}
	// DON'T defer extForm.Clear() yet - we need it for ХранилищеОбработки

	// Check if empty
	isEmpty, err := oleutil.CallMethod(extForm.ToIDispatch(), "Пустая")
	if err != nil {
		extForm.Clear()
		c.cleanup()
		ready <- fmt.Errorf("method 'Пустая()' not found: %w", err)
		return
	}
	// DON'T defer isEmpty.Clear() yet

	isEmptyRes, ok := isEmpty.Value().(bool)
	isEmpty.Clear() // Clear isEmpty immediately after getting value
	if !ok {
		extForm.Clear()
		c.cleanup()
		ready <- fmt.Errorf("invalid result type from Пустая()")
		return
	}
	if isEmptyRes {
		extForm.Clear()
		c.cleanup()
		ready <- fmt.Errorf("не найдена внешняя обработка \"%s\"", cfg.CommandExec)
		return
	}

	// Get temporary filename
	tempFileName, err := oleutil.CallMethod(c.v8.ToIDispatch(), "ПолучитьИмяВременногоФайла")
	if err != nil {
		extForm.Clear()
		c.cleanup()
		ready <- fmt.Errorf("method 'ПолучитьИмяВременногоФайла()' not found: %w", err)
		return
	}
	// DON'T defer tempFileName.Clear() yet - we need it for Создать()

	// Get ХранилищеОбработки from extForm
	obrStore, err := oleutil.GetProperty(extForm.ToIDispatch(), "ХранилищеОбработки")
	if err != nil {
		tempFileName.Clear()
		extForm.Clear()
		c.cleanup()
		ready <- fmt.Errorf("object property 'ХранилищеОбработки' not found: %w", err)
		return
	}
	// DON'T defer obrStore.Clear() yet

	// Get data from storage
	data, err := oleutil.CallMethod(obrStore.ToIDispatch(), "Получить")
	obrStore.Clear() // Clear obrStore now that we have data
	if err != nil {
		tempFileName.Clear()
		extForm.Clear()
		c.cleanup()
		ready <- fmt.Errorf("method 'Получить()' not found: %w", err)
		return
	}
	// DON'T defer data.Clear() yet

	// Write to temp file
	_, err = oleutil.CallMethod(data.ToIDispatch(), "Записать", tempFileName.Value())
	data.Clear() // Clear data after writing
	if err != nil {
		tempFileName.Clear()
		extForm.Clear()
		c.cleanup()
		ready <- fmt.Errorf("method 'Записать()' not found: %w", err)
		return
	}

	// Get ВнешниеОбработки
	c.commandExecParent, err = oleutil.GetProperty(c.v8.ToIDispatch(), "ВнешниеОбработки")
	if err != nil {
		tempFileName.Clear()
		extForm.Clear()
		c.cleanup()
		ready <- fmt.Errorf("object property 'ВнешниеОбработки' not found: %w", err)
		return
	}
	// Keep commandExecParent alive for the connection lifetime

	// DEBUG: Log before Создать()
	logger.Debugf("Creating обработка from temp file: %v", tempFileName.Value())

	// Call Создать on внешниеОбработки
	c.commandExec, err = oleutil.CallMethod(c.commandExecParent.ToIDispatch(), "Создать", tempFileName.Value(), false)
	
	// NOW we can clear tempFileName and extForm - after Создать() is done
	tempFileName.Clear()
	extForm.Clear()
	
	if err != nil {
		c.commandExecParent.Clear()
		c.cleanup()
		ready <- fmt.Errorf("method 'Создать()' not found: %w", err)
		return
	}

	logger.Infof("COM connection %d initialized successfully", c.id)
	ready <- nil

	// Process incoming commands
	for {
		select {
		case fn := <-c.commands:
			fn()
		case <-c.quit:
			logger.Debugf("COM connection %d worker shutting down", c.id)
			
			// Cleanup in reverse order
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
			
			return
		}
	}
}

