# Register a logon scheduled task to start multica-fork + Tailscale Serve/Funnel.
# Run in PowerShell (no admin required for current user).

$ErrorActionPreference = 'Stop'
$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$Script = Join-Path $RepoRoot 'scripts\windows\fork-tailscale-autostart.ps1'
$TaskName = 'MulticaForkTailscale'

$Action = New-ScheduledTaskAction `
  -Execute 'powershell.exe' `
  -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$Script`""

$Trigger = New-ScheduledTaskTrigger -AtLogOn

$Settings = New-ScheduledTaskSettingsSet `
  -AllowStartIfOnBatteries `
  -DontStopIfGoingOnBatteries `
  -StartWhenAvailable `
  -ExecutionTimeLimit (New-TimeSpan -Hours 2)

try {
  Register-ScheduledTask `
    -TaskName $TaskName `
    -Action $Action `
    -Trigger $Trigger `
    -Settings $Settings `
    -Description 'Start multica-fork (api/web/daemon) and Tailscale Serve/Funnel for remote devices.' `
    -Force
  Write-Host "Registered scheduled task: $TaskName (runs at logon)."
} catch {
  $StartupCmd = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Startup\MulticaForkTailscale.cmd'
  @"
@echo off
powershell -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "$Script"
"@ | Set-Content -Path $StartupCmd -Encoding ASCII
  Write-Host "Scheduled task failed ($($_.Exception.Message)); installed Startup folder launcher instead:"
  Write-Host "  $StartupCmd"
}

Write-Host "Log: $env:LOCALAPPDATA\MulticaFork\autostart.log"
