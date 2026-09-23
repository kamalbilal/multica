# Sync this fork with multica-ai/multica (upstream).
# Usage: .\scripts\sync-upstream.ps1 [-Push]

param(
    [switch]$Push
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $root

if (-not (Test-Path .git)) {
    Write-Error "Not a git repository: $root"
}

$remotes = git remote
if ($remotes -notcontains "upstream") {
    Write-Host "Adding upstream remote..."
    git remote add upstream https://github.com/multica-ai/multica.git
}

Write-Host "Fetching upstream..."
git fetch upstream

$branch = git branch --show-current
if ($branch -ne "main") {
    Write-Host "Checking out main..."
    git checkout main
}

$behind = (git rev-list --count main..upstream/main 2>$null)
if ($behind -eq "0") {
    Write-Host "Already up to date with upstream/main."
    exit 0
}

Write-Host "Merging upstream/main ($behind commit(s) behind)..."
git merge upstream/main --no-edit

if ($LASTEXITCODE -ne 0) {
    Write-Host ""
    Write-Host "Merge conflicts. Resolve them, then:"
    Write-Host "  git add -A"
    Write-Host "  git commit"
    if ($Push) { Write-Host "  git push origin main" }
    exit 1
}

Write-Host "Merge complete."

if ($Push) {
    Write-Host "Pushing to origin..."
    git push origin main
}

Write-Host "Done. See FORK.md for fork-specific workflow."
