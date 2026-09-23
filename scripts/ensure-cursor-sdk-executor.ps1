# Build the cursor_sdk Node executor and print its absolute path.
$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$ExecutorDir = Join-Path $RepoRoot "fork\packages\cursor-sdk-executor"
$ExecutorScript = Join-Path $ExecutorDir "dist\cli.js"

if (-not (Test-Path (Join-Path $ExecutorDir "package.json"))) {
  exit 0
}

$node = Get-Command node -ErrorAction SilentlyContinue
if (-not $node) {
  Write-Host "cursor_sdk: node not on PATH; skipping executor build"
  exit 0
}

$nodeMajor = [int](node -p "Number(process.versions.node.split('.')[0])")
if ($nodeMajor -lt 22) {
  Write-Host "cursor_sdk: node $nodeMajor < 22; skipping executor build"
  exit 0
}

Write-Host "==> Building cursor_sdk executor..."
Push-Location $ExecutorDir
npm ci --silent
npm run build --silent
Pop-Location
Write-Output $ExecutorScript
