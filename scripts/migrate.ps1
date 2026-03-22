# Apply migrations/001_initial.sql
# - If CRDB_DSN is set: runs `cockroach sql` on the host (no Docker). Requires the Cockroach CLI on PATH.
# - Otherwise: pipes SQL into `docker compose exec ...` (Cockroach service must be up).

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$sqlPath = Join-Path $root "migrations/001_initial.sql"

function Fail([string]$msg) {
    Write-Host $msg -ForegroundColor Red
    exit 1
}

if ($env:CRDB_DSN -and $env:CRDB_DSN.Trim() -ne "") {
    Write-Host "Using CRDB_DSN (direct, no Docker container)."
    cockroach sql --url=$env:CRDB_DSN -f $sqlPath
    if ($LASTEXITCODE -ne 0) {
        Fail "Migration failed. Check CRDB_DSN and that the Cockroach CLI is installed and on PATH."
    }
    Write-Host "migration applied"
    exit 0
}

Get-Content -Raw $sqlPath | docker compose exec -T cockroach cockroach sql --insecure --host=localhost
if ($LASTEXITCODE -ne 0) {
    Fail @'
Migration failed. Either:
  - Start Docker and the stack:  docker compose up -d
  - Or set CRDB_DSN and use the Cockroach CLI:
      $env:CRDB_DSN = "postgresql://root@127.0.0.1:26257/defaultdb?sslmode=disable"
      .\scripts\migrate.ps1
'@
}
Write-Host "migration applied"
