// Package installer implements the Windows mechanics behind the family's
// embedded "next → next → finish" wizard. The release ships the SAME binary
// twice: "<app>.exe" (inside the zip) just runs the app, portable; the
// "<app>-...-setup.exe" asset opens as the install wizard — the classic
// two-exe model Windows users expect. The library has no UI — each app
// draws the wizard and calls into here, the same split go-updates uses for
// the updater.
//
// What it does, all per-user and without admin rights:
//
//   - RunningAsSetup: is this process the "-setup.exe" copy? (base-name
//     check, so a browser's "app-setup (1).exe" rename still counts).
//   - Installed / InstallDir: detect whether the running exe is the
//     installed copy (default home: %LOCALAPPDATA%\Programs\<app>).
//   - InstalledInfo: is the app registered in Apps & Features, where and
//     which version — what the setup exe uses to offer Reinstall/Uninstall
//     (maintenance) instead of a fresh install.
//   - Install: copy the running exe there and register the app in
//     Apps & Features (HKCU uninstall key).
//   - CreateShortcuts: Start Menu and/or Desktop .lnk files.
//   - Uninstall + MaybeCleanup: full removal — shortcuts, registry entry,
//     the app's data directories and the registered install directory, via
//     a self-deleting helper copy in %TEMP%.
//
// Every entry point but this doc is build-tagged windows; on other OSes the
// package compiles to nothing and apps guard their calls with their own
// per-OS files.
package installer
