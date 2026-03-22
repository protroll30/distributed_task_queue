@echo off
setlocal EnableExtensions
cd /d "%~dp0.."

if not exist "migrations\001_initial.sql" (
  echo migrations\001_initial.sql not found
  exit /b 1
)

type "migrations\001_initial.sql" | docker compose exec -T cockroach cockroach sql --insecure --host=localhost
if errorlevel 1 (
  echo Migration failed. Start the stack: docker compose up -d
  exit /b 1
)
echo migration applied
