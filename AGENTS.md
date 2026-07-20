# go-installer — agent notes

Install library of the
[go-apps](https://github.com/viniciusbuscacio/go-apps) family: classic DMG
on macOS (`cmd/mkdmg`), and on Windows the two-exe model — the release ships
the portable exe plus a `-setup.exe` that is the SAME binary
(`RunningAsSetup` keys the wizard off the file name).

Family rules:
[go-apps/AGENTS.md](https://github.com/viniciusbuscacio/go-apps/blob/main/AGENTS.md)
(`../go-apps/AGENTS.md` in a sibling checkout).

Library invariants:

- Windows installs are per-user, never admin: `%LOCALAPPDATA%\Programs\<app>`,
  Start Menu shortcut, Apps & Features entry; uninstall removes the
  REGISTERED folder (never the folder the process runs from) and warns first.
- Setup run with the app already installed opens Reinstall/Uninstall
  maintenance.
- Linux is Phase 3 (pending); today each app ships a `-install` CLI.
