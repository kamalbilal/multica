# Logon startup: Tailscale Serve/Funnel for multica-fork + production api/web/daemon.
# Install once: scripts/windows/install-fork-autostart-task.ps1

$ErrorActionPreference = 'Stop'
$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$LogDir = Join-Path $env:LOCALAPPDATA 'MulticaFork'
$LogFile = Join-Path $LogDir 'autostart.log'

New-Item -ItemType Directory -Force -Path $LogDir | Out-Null
Start-Transcript -Path $LogFile -Append | Out-Null

try {
  Write-Host "==> $(Get-Date -Format o) multica-fork autostart"

  & (Join-Path $RepoRoot 'scripts\windows\configure-fork-tailscale.ps1')
  if ($LASTEXITCODE -ne 0) { throw "configure-fork-tailscale failed ($LASTEXITCODE)" }

  & powershell -NoProfile -ExecutionPolicy Bypass `
    -File (Join-Path $RepoRoot 'scripts\just-dev-env.ps1') -Action up
  if ($LASTEXITCODE -ne 0) { throw "just up failed ($LASTEXITCODE)" }

  Write-Host "==> multica-fork autostart finished OK"
} catch {
  Write-Error $_
  exit 1
} finally {
  Stop-Transcript | Out-Null
}
