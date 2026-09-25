# Point Tailscale Serve/Funnel at this checkout's production web + API ports.
# Official Tailscale CLI: https://tailscale.com/kb/1247/funnel-serve-use-cases
# Requires: tailscale logged in; fork stack listening on $ApiPort / $WebPort.

param(
  [int]$ApiPort = 18451,
  [int]$WebPort = 13371,
  [int]$TailnetApiHttpsPort = 5722,
  [int]$TailnetWebHttpsPort = 5723,
  [int]$FunnelHttpsPort = 8443
)

$ErrorActionPreference = 'Stop'

function Wait-Tailscale {
  for ($i = 0; $i -lt 60; $i++) {
    try {
      $null = & tailscale status 2>$null
      if ($LASTEXITCODE -eq 0) { return }
    } catch {}
    Start-Sleep -Seconds 2
  }
  throw 'Tailscale is not ready after 2 minutes.'
}

Wait-Tailscale

& tailscale serve --bg --https=$TailnetApiHttpsPort "http://127.0.0.1:$ApiPort"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

& tailscale serve --bg --https=$TailnetWebHttpsPort "http://127.0.0.1:$WebPort"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

& tailscale funnel --bg --https=$FunnelHttpsPort "http://127.0.0.1:$WebPort"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "Tailscale Serve/Funnel configured for fork (API -> :$ApiPort, web -> :$WebPort)."
& tailscale serve status
& tailscale funnel status
