//go:build windows

package installer

import (
	"fmt"
	"runtime"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// COM HRESULTs CoInitializeEx can return that still mean "COM is usable on
// this thread": S_FALSE (already initialized) and RPC_E_CHANGED_MODE
// (initialized earlier in the other apartment model).
const (
	hrSFalse         = 1
	hrRPCChangedMode = 0x80010106
)

// makeShortcut writes a .lnk at path pointing at target, via the
// WScript.Shell COM object — the same API installers have used for decades,
// which keeps this free of the hand-written .lnk binary format.
func makeShortcut(path, target, workDir, desc string) error {
	// The goroutine calling this is scheduled by Wails, not by us; pin it so
	// COM initialization and use stay on one OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		oleErr, ok := err.(*ole.OleError)
		if !ok || (oleErr.Code() != hrSFalse && oleErr.Code() != hrRPCChangedMode) {
			return fmt.Errorf("initialize COM: %w", err)
		}
	} else {
		defer ole.CoUninitialize()
	}

	unknown, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return fmt.Errorf("WScript.Shell: %w", err)
	}
	defer unknown.Release()
	shell, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer shell.Release()

	cs, err := oleutil.CallMethod(shell, "CreateShortcut", path)
	if err != nil {
		return err
	}
	lnk := cs.ToIDispatch()
	defer lnk.Release()

	props := []struct{ name, value string }{
		{"TargetPath", target},
		{"WorkingDirectory", workDir},
		{"IconLocation", target + ",0"},
		{"Description", desc},
	}
	for _, p := range props {
		if _, err := oleutil.PutProperty(lnk, p.name, p.value); err != nil {
			return fmt.Errorf("shortcut %s: %w", p.name, err)
		}
	}
	if _, err := oleutil.CallMethod(lnk, "Save"); err != nil {
		return fmt.Errorf("save shortcut: %w", err)
	}
	return nil
}
