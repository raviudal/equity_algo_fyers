# Automated Go 1.22.5 SDK Downloader and PATH Configurator for Windows

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$installDir = "C:\Go"
$zipUrl = "https://go.dev/dl/go1.22.5.windows-amd64.zip"
$tempZip = "$env:TEMP\go1.22.5.zip"

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host " Installing Go 1.22.5 SDK for Windows" -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan

if (Test-Path "$installDir\bin\go.exe") {
    Write-Host "[+] Go SDK is already installed at $installDir" -ForegroundColor Green
} else {
    Write-Host "[1/3] Downloading Go 1.22.5..." -ForegroundColor Yellow
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.SecurityProtocolType]::Tls12
    (New-Object System.Net.WebClient).DownloadFile($zipUrl, $tempZip)

    Write-Host "[2/3] Extracting Go SDK to $installDir..." -ForegroundColor Yellow
    $tempExtract = "$env:TEMP\go_extract_temp"
    Remove-Item $tempExtract -Recurse -Force -ErrorAction SilentlyContinue
    Expand-Archive -Path $tempZip -DestinationPath $tempExtract -Force

    if (-not (Test-Path "C:\Go")) {
        New-Item -ItemType Directory -Path "C:\Go" -Force | Out-Null
    }
    Copy-Item -Path "$tempExtract\go\*" -Destination $installDir -Recurse -Force

    Remove-Item $tempZip -Force -ErrorAction SilentlyContinue
    Remove-Item $tempExtract -Recurse -Force -ErrorAction SilentlyContinue
}

# Add Go to User PATH environment variable
Write-Host "[3/3] Setting PATH environment variable..." -ForegroundColor Yellow
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*C:\Go\bin*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;C:\Go\bin", "User")
    $env:Path = "$env:Path;C:\Go\bin"
    Write-Host "[+] C:\Go\bin added to User PATH!" -ForegroundColor Green
}

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host " Go Installation Successful!" -ForegroundColor Green
& "C:\Go\bin\go.exe" version
Write-Host " You can now run build.bat to compile algoengine" -ForegroundColor Cyan
Write-Host "==========================================================" -ForegroundColor Cyan
