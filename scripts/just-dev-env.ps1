# Windows entry point for `just` recipes. Invokes dev-env.sh through Git Bash so
# REPO_ROOT uses the same /c/Users/... path as `make up` (not WSL /mnt/c/...).
param(
    [Parameter(Mandatory)]
    [ValidateSet('up', 'down', 'restart', 'status', 'list', 'build', 'repair', 'setup', 'login')]
    [string]$Action
)

$ErrorActionPreference = 'Stop'
$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path

function Find-GitBash {
    foreach ($candidate in @(
        "${env:ProgramFiles}\Git\bin\bash.exe",
        "${env:ProgramFiles(x86)}\Git\bin\bash.exe"
    )) {
        if ($candidate -and (Test-Path -LiteralPath $candidate)) {
            return $candidate
        }
    }
    throw @"
Git Bash not found. Install Git for Windows (https://git-scm.com/download/win)
or run dev-env from Git Bash: bash scripts/dev-env.sh up --production --components api,web,daemon
"@
}

$GitBash = Find-GitBash
$Components = 'api,web,daemon'
$BashRepo = ($RepoRoot -replace '\\', '/')

function Invoke-DevEnv {
    param([Parameter(Mandatory)][string[]]$Args)

    $escaped = $Args | ForEach-Object { $_ -replace "'", "'\\''" }
    $argLine = ($escaped | ForEach-Object { "'$_'" }) -join ' '
    $command = "cd '$BashRepo' && ./scripts/dev-env.sh $argLine"
    & $GitBash -lc $command
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

function Invoke-JustBuild {
    $command = "cd '$BashRepo' && ./scripts/just-build.sh"
    & $GitBash -lc $command
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}

switch ($Action) {
    'build' {
        Invoke-JustBuild
    }
    'up' {
        Invoke-DevEnv @('up', '--production', '--components', $Components)
    }
    'down' {
        Invoke-DevEnv @('down', '--components', $Components)
    }
    'restart' {
        Invoke-DevEnv @('down', '--components', $Components)
        Invoke-DevEnv @('up', '--production', '--components', $Components)
    }
    'status' {
        Invoke-DevEnv @('status')
    }
    'list' {
        Invoke-DevEnv @('list')
    }
    'repair' {
        Invoke-DevEnv @('repair-cli')
    }
    'login' {
        Invoke-DevEnv @('login-cli')
    }
    'setup' {
        Invoke-JustBuild
        Invoke-DevEnv @('up', '--production', '--components', $Components)
        Invoke-DevEnv @('repair-cli')
    }
    default {
        throw "Unknown action: $Action"
    }
}
