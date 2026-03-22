@echo off
setlocal EnableExtensions
cd /d "%~dp0.."

go mod verify || exit /b 1
go vet ./... || exit /b 1
go test ./... -count=1 || exit /b 1
go build -o NUL .\cmd\orchestrator .\cmd\worker || exit /b 1
echo verify ok
