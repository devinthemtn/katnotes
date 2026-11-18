#!/usr/bin/env bash
# Runs the Windows katnotes-gui.exe from WSL. `task install-windows` fills in
# the exe path and installs this as katnotes-win.
exe='@EXE@'
for a in "$@"; do
  case "$a" in
    # A flag (--version, a typo, ...) prints something and exits.
    -*) exec "$exe" "$@" ;;
  esac
done
# Detached, so the shell isn't held while the window is open. Started from
# the install folder, since a Windows program can't use a WSL folder as its
# working directory.
cd "$(dirname "$exe")" 2>/dev/null || true
nohup "$exe" "$@" >/dev/null 2>&1 &
disown
