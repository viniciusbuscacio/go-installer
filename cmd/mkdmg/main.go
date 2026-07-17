// Command mkdmg builds a styled DMG for a go-apps family app.
//
// It assembles a staging folder (app + Applications symlink + background),
// creates a writable image, mounts it unbrowsed, styles it with a
// short-lived Python helper (ds-store + mac-alias) and converts the result
// to a compressed read-only DMG.
//
// The styling helper runs as its own process and exits before the volume
// is detached — dmgbuild keeps files open across the detach, which on
// recent macOS fails with "resource busy"; this tool exists to avoid that.
//
// Usage:
//
//	go run ./cmd/mkdmg -app path/to/go-calc.app \
//	    -bg macos/background/background.tiff \
//	    -volname go-calc -out go-calc-v0.1.3-macos-arm64.dmg
//
// Requires: macOS, python3 with `pip install ds-store mac-alias`
// (override the interpreter with -python).
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed write_dsstore.py
var writeDSStore []byte

// the family background ships inside the binary so release CIs only need
// the one `go run` invocation; -bg overrides it.
//
//go:embed background.tiff
var familyBackground []byte

func main() {
	app := flag.String("app", "", "path to the .app bundle (required)")
	bg := flag.String("bg", "", "background image (tiff); default: embedded family background")
	volname := flag.String("volname", "", "volume name (required)")
	out := flag.String("out", "", "output .dmg path (required)")
	python := flag.String("python", "python3", "python interpreter with ds-store + mac-alias")
	flag.Parse()
	if *app == "" || *volname == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}

	if *bg == "" {
		f, err := os.CreateTemp("", "mkdmg-bg-*.tiff")
		if err != nil {
			fmt.Fprintln(os.Stderr, "mkdmg:", err)
			os.Exit(1)
		}
		defer os.Remove(f.Name())
		if _, err := f.Write(familyBackground); err != nil {
			fmt.Fprintln(os.Stderr, "mkdmg:", err)
			os.Exit(1)
		}
		f.Close()
		*bg = f.Name()
	}

	if err := build(*app, *bg, *volname, *out, *python); err != nil {
		fmt.Fprintln(os.Stderr, "mkdmg:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", *out)
}

func build(app, bg, volname, out, python string) error {
	appName := filepath.Base(app)
	bgName := ".background" + filepath.Ext(bg) // hidden file at volume root

	// 1. staging folder
	staging, err := os.MkdirTemp("", "mkdmg-staging-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	if err := run("/usr/bin/ditto", app, filepath.Join(staging, appName)); err != nil {
		return fmt.Errorf("copying app: %w", err)
	}
	if err := os.Symlink("/Applications", filepath.Join(staging, "Applications")); err != nil {
		return err
	}
	if err := run("/bin/cp", bg, filepath.Join(staging, bgName)); err != nil {
		return fmt.Errorf("copying background: %w", err)
	}
	// Gatekeeper blocks unsigned apps on first launch; ship the fix with
	// the DMG so users aren't left guessing.
	if err := os.WriteFile(filepath.Join(staging, "README.txt"),
		[]byte(readmeText(appName)), 0o644); err != nil {
		return err
	}
	// keep Spotlight off the mounted volume
	if err := os.WriteFile(filepath.Join(staging, ".metadata_never_index"), nil, 0o644); err != nil {
		return err
	}

	// 2. writable image from the staging folder
	tmpDMG := out + ".tmp.dmg"
	defer os.Remove(tmpDMG)
	if err := run("/usr/bin/hdiutil", "create", "-quiet", "-ov",
		"-srcfolder", staging, "-volname", volname, "-fs", "HFS+",
		"-format", "UDRW", tmpDMG); err != nil {
		return fmt.Errorf("hdiutil create: %w", err)
	}

	// 3. mount unbrowsed (Finder and friends stay away)
	attachOut, err := runOut("/usr/bin/hdiutil", "attach", "-nobrowse", "-noautoopen", tmpDMG)
	if err != nil {
		return fmt.Errorf("hdiutil attach: %w", err)
	}
	device, mount := parseAttach(attachOut)
	if device == "" || mount == "" {
		return fmt.Errorf("could not parse hdiutil attach output:\n%s", attachOut)
	}
	detached := false
	defer func() {
		if !detached {
			_ = run("/usr/bin/hdiutil", "detach", device, "-force")
		}
	}()

	// 4. style it: short-lived helper writes .DS_Store and exits
	helper, err := os.CreateTemp("", "write_dsstore-*.py")
	if err != nil {
		return err
	}
	defer os.Remove(helper.Name())
	if _, err := helper.Write(writeDSStore); err != nil {
		return err
	}
	helper.Close()
	if err := run(python, helper.Name(), mount, appName, bgName, "README.txt"); err != nil {
		return fmt.Errorf("writing .DS_Store (is `pip install ds-store mac-alias` done for %s?): %w", python, err)
	}

	// 5. detach; everything that touched the volume is gone, so retries are
	// just insurance against slow system scanners
	_ = run("/bin/sync")
	for attempt := 1; ; attempt++ {
		if err = run("/usr/bin/hdiutil", "detach", device); err == nil {
			break
		}
		if attempt == 8 {
			return fmt.Errorf("hdiutil detach %s: %w", device, err)
		}
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	detached = true

	// 6. compress to the final read-only image
	if err := run("/usr/bin/hdiutil", "convert", "-quiet", "-ov", tmpDMG,
		"-format", "UDZO", "-imagekey", "zlib-level=9", "-o", out); err != nil {
		return fmt.Errorf("hdiutil convert: %w", err)
	}
	return nil
}

// readmeText is the install note shipped inside every DMG. The apps are not
// notarized, so Gatekeeper blocks the first launch; the xattr line is the
// reliable way around it on current macOS.
func readmeText(appName string) string {
	name := strings.TrimSuffix(appName, ".app")
	return fmt.Sprintf(`%s — macOS install notes
=====================================

1. Drag %s onto the Applications folder in this window.

2. macOS blocks the first launch because this app is not notarized by
   Apple. To allow it, run this once in Terminal:

   xattr -dr com.apple.quarantine /Applications/%s

   Then open the app normally.

More: https://github.com/viniciusbuscacio/%s
`, name, name, appName, name)
}

// parseAttach pulls the device node and mount point out of hdiutil attach
// output, e.g. "/dev/disk4s1 ... Apple_HFS ... /Volumes/go-calc".
func parseAttach(out string) (device, mount string) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "/dev/disk") {
			continue
		}
		if device == "" {
			device = fields[0]
		}
		if i := strings.Index(line, "/Volumes/"); i >= 0 {
			mount = strings.TrimSpace(line[i:])
		}
	}
	return device, mount
}

func run(name string, args ...string) error {
	_, err := runOut(name, args...)
	return err
}

func runOut(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return string(b), fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, b)
	}
	return string(b), nil
}
