$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$sqlPath = Join-Path $root "migrations/001_initial.sql"
Get-Content -Raw $sqlPath | docker compose exec -T cockroach cockroach sql --insecure --host=localhost
Write-Host "migration applied"
