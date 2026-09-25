@echo off
setlocal
cd /d "%~dp0.."

if not defined USERPROFILE set "USERPROFILE=%HOMEDRIVE%%HOMEPATH%"
if not defined LOCALAPPDATA set "LOCALAPPDATA=%USERPROFILE%\AppData\Local"
if not defined GOCACHE set "GOCACHE=%LOCALAPPDATA%\go-build"

title Multica Canary (dev)
echo Starting Multica from this repo (pnpm dev:desktop)...
echo Close this window to stop the app.
echo.

call pnpm dev:desktop
if errorlevel 1 (
  echo.
  echo dev:desktop exited with an error.
  pause
)
