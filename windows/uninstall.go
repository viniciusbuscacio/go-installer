//go:build windows

package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// cleanupFlag is the hidden argv marker the %TEMP% helper copy runs under.
const cleanupFlag = "--go-installer-cleanup"

// manifest is everything the helper needs to finish the uninstall after the
// installed exe has exited.
type manifest struct {
	InstallDir  string   `json:"installDir"`
	DataDirs    []string `json:"dataDirs"`
	Shortcuts   []string `json:"shortcuts"`
	RegistryKey string   `json:"registryKey"` // relative to HKCU
	ParentPID   int      `json:"parentPid"`
}

// Uninstall removes the app completely: shortcuts, the Apps & Features
// entry, the data directories and the install directory. A running exe
// cannot delete itself, so this copies the exe to %TEMP%, relaunches that
// copy with a hidden flag and returns — the caller must quit the app right
// after. MaybeCleanup, called first thing in main, picks the flag up inside
// the helper copy and does the actual removal.
func (a App) Uninstall() error {
	self, err := selfPath()
	if err != nil {
		return err
	}
	// The directory to remove is the REGISTERED install, not necessarily
	// where this process runs from: the setup exe offers Uninstall from the
	// Downloads folder too, and Dir(self) there would delete the wrong tree.
	dir := ""
	if loc, _, ok := a.InstalledInfo(); ok {
		dir = loc
	} else if a.Installed() {
		dir = filepath.Dir(self)
	}
	m := manifest{
		InstallDir:  dir,
		DataDirs:    a.DataDirs,
		Shortcuts:   a.shortcutPaths(),
		RegistryKey: uninstallRoot + a.ID,
		ParentPID:   os.Getpid(),
	}

	tmpDir, err := os.MkdirTemp("", a.ID+"-uninstall-")
	if err != nil {
		return err
	}
	helper := filepath.Join(tmpDir, a.ID+"-uninstall.exe")
	if err := copyFile(self, helper); err != nil {
		return err
	}
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return err
	}

	cmd := exec.Command(helper, cleanupFlag+"="+manifestPath)
	cmd.Dir = tmpDir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start uninstall helper: %w", err)
	}
	return nil
}

// MaybeCleanup runs the removal when the process is the %TEMP% helper copy.
// Apps call it first thing in main, before any UI starts; it returns true
// when it handled the flag and the caller must exit immediately.
func MaybeCleanup() bool {
	for _, arg := range os.Args[1:] {
		if rest, ok := strings.CutPrefix(arg, cleanupFlag+"="); ok {
			runCleanup(rest)
			return true
		}
	}
	return false
}

func runCleanup(manifestPath string) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return
	}
	var m manifest
	if json.Unmarshal(data, &m) != nil {
		return
	}

	// Wait for the installed exe to exit so its file unlocks.
	if h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(m.ParentPID)); err == nil {
		windows.WaitForSingleObject(h, 30_000)
		windows.CloseHandle(h)
	}

	for _, lnk := range m.Shortcuts {
		os.Remove(lnk)
	}
	registry.DeleteKey(registry.CURRENT_USER, m.RegistryKey)
	for _, dir := range m.DataDirs {
		os.RemoveAll(dir)
	}
	// The freshly-exited exe can stay locked a beat longer (antivirus,
	// pending handles); retry rather than leave a half-removed folder.
	if m.InstallDir != "" {
		for i := 0; i < 40; i++ {
			if err := os.RemoveAll(m.InstallDir); err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	selfDestruct(filepath.Dir(manifestPath))
}

// selfDestruct removes the helper's own %TEMP% folder: cmd waits out the
// ping delay (~2s, by which time this process has exited) then deletes the
// folder, helper exe included. Hidden window, detached.
func selfDestruct(tmpDir string) {
	cmd := exec.Command("cmd.exe", "/C",
		"ping -n 3 127.0.0.1 > nul & rd /s /q \""+tmpDir+"\"")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Start()
}
