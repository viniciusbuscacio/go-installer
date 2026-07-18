//go:build windows

package installer

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// App describes one app of the family to the installer.
type App struct {
	ID          string // machine name: install folder, exe name, registry key ("go-calc")
	DisplayName string // human name: shortcuts, Apps & Features ("go-Calc")
	Version     string // running version without the "v" prefix ("0.1.3")
	Publisher   string // Apps & Features "Publisher" column
	URL         string // project home, Apps & Features help link
	// Dir overrides the install directory (the wizard's "Change…" button).
	// Empty means the default, %LOCALAPPDATA%\Programs\<ID>.
	Dir string
	// DataDirs are the app's per-user data directories (settings, sessions).
	// Uninstall removes them: the family decision is that uninstalling
	// removes everything, and the wizard warns the user about exactly that.
	DataDirs []string
}

// uninstallRoot is where per-user apps register for Apps & Features.
const uninstallRoot = `Software\Microsoft\Windows\CurrentVersion\Uninstall\`

// InstallDir returns the directory the app installs into.
func (a App) InstallDir() (string, error) {
	if a.Dir != "" {
		return a.Dir, nil
	}
	return a.defaultDir()
}

func (a App) defaultDir() (string, error) {
	base, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return "", fmt.Errorf("resolve LocalAppData: %w", err)
	}
	return filepath.Join(base, "Programs", a.ID), nil
}

// Installed reports whether the running executable is the installed copy —
// either in the default directory or wherever the registry says a previous
// wizard run put it (the user may have picked a custom folder).
func (a App) Installed() bool {
	self, err := selfPath()
	if err != nil {
		return false
	}
	here := filepath.Dir(self)
	if def, err := a.defaultDir(); err == nil && strings.EqualFold(here, def) {
		return true
	}
	if loc, err := a.registryInstallLocation(); err == nil && strings.EqualFold(here, loc) {
		return true
	}
	return false
}

// Install copies the running executable into the install directory and
// registers the app in Apps & Features. Shortcuts are separate
// (CreateShortcuts) so the wizard can offer them on its final screen.
// Returns the path of the installed executable.
func (a App) Install() (string, error) {
	self, err := selfPath()
	if err != nil {
		return "", err
	}
	dir, err := a.InstallDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	dst := filepath.Join(dir, a.ID+".exe")
	// Windows lets a running exe be renamed but not overwritten, so a
	// reinstall over a possibly-running copy moves it aside first.
	if _, err := os.Stat(dst); err == nil && !strings.EqualFold(dst, self) {
		old := dst + ".old"
		os.Remove(old)
		if err := os.Rename(dst, old); err != nil {
			return "", fmt.Errorf("replace previous install: %w", err)
		}
	}
	if !strings.EqualFold(dst, self) {
		if err := copyFile(self, dst); err != nil {
			return "", err
		}
	}
	os.Remove(dst + ".old") // best effort; a locked leftover dies with the old process
	if err := a.writeRegistry(dst); err != nil {
		return "", err
	}
	return dst, nil
}

// Shortcuts selects which shortcuts CreateShortcuts writes.
type Shortcuts struct {
	StartMenu bool
	Desktop   bool
}

// CreateShortcuts writes the selected .lnk files pointing at exe. Known
// folders (not %USERPROFILE% guesses) so a OneDrive-redirected Desktop still
// gets its shortcut in the right place.
func (a App) CreateShortcuts(exe string, sc Shortcuts) error {
	var folders []*windows.KNOWNFOLDERID
	if sc.StartMenu {
		folders = append(folders, windows.FOLDERID_Programs)
	}
	if sc.Desktop {
		folders = append(folders, windows.FOLDERID_Desktop)
	}
	for _, id := range folders {
		base, err := windows.KnownFolderPath(id, 0)
		if err != nil {
			return fmt.Errorf("resolve shortcut folder: %w", err)
		}
		lnk := filepath.Join(base, a.DisplayName+".lnk")
		if err := makeShortcut(lnk, exe, filepath.Dir(exe), a.DisplayName); err != nil {
			return fmt.Errorf("create %s: %w", lnk, err)
		}
	}
	return nil
}

// shortcutPaths lists every .lnk the app may have created, for uninstall.
func (a App) shortcutPaths() []string {
	var paths []string
	for _, id := range []*windows.KNOWNFOLDERID{windows.FOLDERID_Programs, windows.FOLDERID_Desktop} {
		if base, err := windows.KnownFolderPath(id, 0); err == nil {
			paths = append(paths, filepath.Join(base, a.DisplayName+".lnk"))
		}
	}
	return paths
}

// writeRegistry creates the HKCU uninstall key that puts the app in
// Apps & Features with a working Uninstall button.
func (a App) writeRegistry(exe string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallRoot+a.ID, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("registry: %w", err)
	}
	defer k.Close()

	strs := []struct{ name, value string }{
		{"DisplayName", a.DisplayName},
		{"DisplayVersion", a.Version},
		{"Publisher", a.Publisher},
		{"URLInfoAbout", a.URL},
		{"InstallLocation", filepath.Dir(exe)},
		{"DisplayIcon", exe},
		{"UninstallString", `"` + exe + `" --uninstall`},
		{"InstallDate", time.Now().Format("20060102")},
	}
	for _, s := range strs {
		if s.value == "" {
			continue
		}
		if err := k.SetStringValue(s.name, s.value); err != nil {
			return fmt.Errorf("registry %s: %w", s.name, err)
		}
	}
	dwords := map[string]uint32{"NoModify": 1, "NoRepair": 1}
	if fi, err := os.Stat(exe); err == nil {
		dwords["EstimatedSize"] = uint32(fi.Size() / 1024) // KB
	}
	for name, v := range dwords {
		if err := k.SetDWordValue(name, v); err != nil {
			return fmt.Errorf("registry %s: %w", name, err)
		}
	}
	return nil
}

func (a App) registryInstallLocation() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallRoot+a.ID, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()
	loc, _, err := k.GetStringValue("InstallLocation")
	return loc, err
}

// RunningAsSetup reports whether the running executable is named as an
// installer: the release ships the same binary twice, and the "-setup.exe"
// name is what turns it into the install wizard — a plain "<app>.exe" just
// runs the app (portable). The check is on the base name only, so a
// browser's "go-calc-setup (1).exe" rename still counts.
func RunningAsSetup() bool {
	self, err := selfPath()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(filepath.Base(self)), "setup")
}

// InstalledInfo returns the location and version of an existing install
// recorded in Apps & Features, if any — what the setup exe uses to offer
// Reinstall / Uninstall (maintenance) instead of a fresh install. A registry
// entry whose folder no longer exists is treated as not installed.
func (a App) InstalledInfo() (location, version string, ok bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, uninstallRoot+a.ID, registry.QUERY_VALUE)
	if err != nil {
		return "", "", false
	}
	defer k.Close()
	loc, _, err := k.GetStringValue("InstallLocation")
	if err != nil || loc == "" {
		return "", "", false
	}
	if _, err := os.Stat(loc); err != nil {
		return "", "", false
	}
	ver, _, _ := k.GetStringValue("DisplayVersion")
	return loc, ver, true
}

// UninstallRequested reports whether the process was launched with
// --uninstall — the UninstallString Apps & Features invokes. The app should
// then show its uninstall confirmation and call Uninstall.
func UninstallRequested() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--uninstall" {
			return true
		}
	}
	return false
}

// Launch starts exe detached from the current process (the wizard's final
// "open the app": the installed copy outlives the downloaded one).
func Launch(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	return cmd.Start()
}

func selfPath() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(self)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copy to %s: %w", dst, err)
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
