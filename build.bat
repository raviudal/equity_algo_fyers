@echo off
setlocal enabledelayedexpansion

echo ==========================================================
echo  Building AlgoEngine for GCP Linux (e2-micro amd64)
echo ==========================================================

:: 1. Search for go binary
set "GOCMD=go"

where go >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    if exist "C:\Users\ravi\go_sdk\bin\go.exe" (
        set "GOCMD=C:\Users\ravi\go_sdk\bin\go.exe"
    ) else if exist "C:\Go\bin\go.exe" (
        set "GOCMD=C:\Go\bin\go.exe"
    ) else if exist "C:\Program Files\Go\bin\go.exe" (
        set "GOCMD=C:\Program Files\Go\bin\go.exe"
    ) else if exist "%LOCALAPPDATA%\Programs\Go\bin\go.exe" (
        set "GOCMD=%LOCALAPPDATA%\Programs\Go\bin\go.exe"
    ) else (
        echo [ERROR] Go compiler is not installed or not in PATH!
        echo.
        echo Please install Go by running PowerShell as Administrator:
        echo   powershell -ExecutionPolicy Bypass -File setup_go.ps1
        echo Or download Go manually from https://go.dev/dl/
        echo.
        exit /b 1
    )
)

echo Using Go compiler: %GOCMD%
%GOCMD% version

:: 2. Cross-compile for GCP Linux amd64
set GOOS=linux
set GOARCH=amd64
%GOCMD% build -ldflags="-s -w" -o algoengine main.go

if %ERRORLEVEL% EQU 0 (
    echo ==========================================================
    echo [SUCCESS] Linux binary compiled: algoengine
    echo Ready to deploy to GCP VM!
    echo ==========================================================
) else (
    echo [ERROR] Build failed! Check Go compilation errors above.
)
