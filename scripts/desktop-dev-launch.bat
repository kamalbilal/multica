@echo off
setlocal
cd /d "%~dp0.."

if not defined USERPROFILE set "USERPROFILE=%HOMEDRIVE%%HOMEPATH%"
if not defined LOCALAPPDATA set "LOCALAPPDATA=%USERPROFILE%\AppData\Local"
if not defined GOCACHE set "GOCACHE=%LOCALAPPDATA%\go-build"

call "%~dp0sync-desktop-env.bat"

title Multica Canary (local build)
echo Starting Multica from this repo (production desktop build + preview)...
echo Backend must be running: run just up in this repo if login codes fail.
echo Local login fixed code: 888888 (see MULTICA_DEV_VERIFICATION_CODE in .env)
echo First launch builds the app; later launches rebuild then start.
echo Close this window to stop the app.
echo.

call pnpm preview:desktop
if errorlevel 1 (
  echo.
  echo preview:desktop exited with an error.
  pause
)
