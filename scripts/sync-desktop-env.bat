@echo off
setlocal
cd /d "%~dp0.."

set "GIT_BASH=%ProgramFiles%\Git\bin\bash.exe"
if not exist "%GIT_BASH%" set "GIT_BASH=%ProgramFiles(x86)%\Git\bin\bash.exe"
if not exist "%GIT_BASH%" (
  echo WARN: Git Bash not found — desktop may use stale VITE_API_URL.
  exit /b 0
)

"%GIT_BASH%" -lc "./scripts/sync-desktop-env.sh"
exit /b %ERRORLEVEL%
