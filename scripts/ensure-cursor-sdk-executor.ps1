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

function Test-CursorSdkExecutorDeps {
    param([string]$Dir)
    $sdkPackage = Join-Path $Dir "node_modules\@cursor\sdk\package.json"
    if (-not (Test-Path $sdkPackage)) {
        return $false
    }
    Push-Location $Dir
    try {
        node --input-type=module -e "import '@cursor/sdk'" 2>$null | Out-Null
        return $?
    } finally {
        Pop-Location
    }
}

Write-Host "==> Building cursor_sdk executor..."
Push-Location $ExecutorDir
npm ci --silent
if (-not (Test-CursorSdkExecutorDeps -Dir $ExecutorDir)) {
    Write-Host "cursor_sdk: @cursor/sdk install incomplete; retrying npm ci..."
    Remove-Item -Recurse -Force node_modules -ErrorAction SilentlyContinue
    npm ci --silent
    if (-not (Test-CursorSdkExecutorDeps -Dir $ExecutorDir)) {
        throw "cursor_sdk executor dependencies are broken after npm ci (missing @cursor/sdk). Run 'npm ci' in $ExecutorDir and retry."
    }
}
npm run build --silent
Pop-Location
Write-Output $ExecutorScript
