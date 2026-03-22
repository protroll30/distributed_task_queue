$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$sqlPath = Join-Path $root "migrations/001_initial.sql"
Get-Content -Raw $sqlPath | docker compose exec -T cockroach cockroach sql --insecure --host=localhost
if ($LASTEXITCODE -ne 0) {
    Write-Error "Migration failed. Is the Cockroach container up? From the repo root run: docker compose up -d"
    exit $LASTEXITCODE
}
Write-Host "migration applied"
