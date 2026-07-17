// Package installer implements the Windows mechanics behind the family's
// embedded "next → next → finish" wizard: the downloaded exe IS the
// installer. The library has no UI — each app draws the wizard and calls
// into here, the same split go-updates uses for the updater.
//
// What it does, all per-user and without admin rights:
//
//   - Installed / InstallDir: detect whether the running exe is the
//     installed copy (default home: %LOCALAPPDATA%\Programs\<app>).
//   - Install: copy the running exe there and register the app in
//     Apps & Features (HKCU uninstall key).
//   - CreateShortcuts: Start Menu and/or Desktop .lnk files.
//   - Uninstall + MaybeCleanup: full removal — shortcuts, registry entry,
//     the app's data directories and the install directory itself, via a
//     self-deleting helper copy in %TEMP%.
//
// Every entry point but this doc is build-tagged windows; on other OSes the
// package compiles to nothing and apps guard their calls with their own
// per-OS files.
package installer
