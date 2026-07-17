# go-installer

Install tooling for the [go-apps](https://github.com/viniciusbuscacio/go-apps)
family: give every app a native install experience on each OS.

**Status: work in progress.**

## macOS — styled DMG

A classic drag-to-Applications DMG, generated in the release CI.

- `cmd/mkdmg` — builds the DMG: stages the app, an `/Applications` symlink
  and the family background, mounts the image unbrowsed, writes the Finder
  layout (`.DS_Store`) via a short-lived Python helper, detaches and
  compresses. The helper exits before detach on purpose: long-lived
  processes holding files on the volume make `hdiutil detach` fail with
  "resource busy" on recent macOS (this is why dmgbuild alone wasn't used).
- `macos/background` — renders the family background (graphite, arrow) as
  `background.png` + `background@2x.png`; `tiffutil -cathidpicheck` merges
  them into the retina `background.tiff` that ships inside the DMG.

The family background is embedded in the tool, so a release CI needs just:

```sh
pip install ds-store mac-alias
go run github.com/viniciusbuscacio/go-installer/cmd/mkdmg@latest \
    -app path/to/go-calc.app \
    -volname go-calc -out go-calc-v0.1.3-macos-arm64.dmg
```

To change the background: edit `macos/background/gen.go`, then

```sh
go run ./macos/background -out macos/background
tiffutil -cathidpicheck macos/background/background.png \
    macos/background/background@2x.png -out macos/background/background.tiff
cp macos/background/background.tiff cmd/mkdmg/background.tiff  # re-embed
```

The volume name is the app name (no version) so the layout stays identical
across releases.

## Windows — embedded wizard

The downloaded exe IS the installer: on a non-installed launch the app
opens as a "next → next → finish" wizard and installs itself. This
package (`windows/`, import path
`github.com/viniciusbuscacio/go-installer/windows`) is the mechanics
behind it — library without UI; each app draws the wizard, like
[go-updates](https://github.com/viniciusbuscacio/go-updates).

Everything is per-user, no admin rights:

- `App.Installed` / `App.InstallDir` — detect whether the running exe is
  the installed copy (default `%LOCALAPPDATA%\Programs\<app>`, or the
  custom folder recorded in the registry).
- `App.Install` — copy the running exe there and register the app in
  Apps & Features (HKCU uninstall key: display name, version, publisher,
  estimated size, `"<exe>" --uninstall` as the uninstall command).
- `App.CreateShortcuts` — Start Menu and/or Desktop `.lnk`, resolved via
  known folders (a OneDrive-redirected Desktop still works), created
  through the WScript.Shell COM object.
- `App.Uninstall` + `MaybeCleanup` — full removal: shortcuts, registry
  entry, the app's data directories (the family decision: uninstalling
  removes everything) and the install folder. A running exe cannot
  delete itself, so Uninstall relaunches a helper copy from `%TEMP%`;
  `MaybeCleanup`, called first thing in the app's `main`, does the
  removal inside that copy and then removes the copy too.
- `UninstallRequested` / `Launch` — small helpers for the app's boot
  logic and the wizard's final "open the app".

Reference integration: go-calc (`install_windows.go` + the
`InstallerView.vue` wizard).

## License

[MIT](LICENSE)
