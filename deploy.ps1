# OpenAIDE Windows to Linux Deploy Script
# Usage: .\deploy.ps1 [-Server root@192.168.3.26] [-Restart]

param(
    [string]$Server = "root@192.168.3.26",
    [string]$RemoteDir = "/opt/openaide",
    [switch]$NoBuild,
    [switch]$Restart
)

$ErrorActionPreference = "Stop"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "       OpenAIDE Deploy Script           " -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Build backend
if (-not $NoBuild) {
    Write-Host "Building backend..." -ForegroundColor Yellow
    Set-Location $PSScriptRoot\backend\src
    $env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
    go build -ldflags "-s -w" -o openaide-server .
    if ($LASTEXITCODE -ne 0) { throw "Backend build failed" }
    Write-Host "  OK" -ForegroundColor Green

    Write-Host "Building CLI..." -ForegroundColor Yellow
    Set-Location $PSScriptRoot\terminal
    $env:GOOS = "linux"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
    go build -ldflags "-s -w" -o openaide .
    if ($LASTEXITCODE -ne 0) { throw "CLI build failed" }
    Write-Host "  OK" -ForegroundColor Green
} else {
    Write-Host "Skipping build (--NoBuild)" -ForegroundColor Yellow
}

# Upload files
Write-Host ""
Write-Host "Uploading to $Server ..." -ForegroundColor Yellow

# Upload backend
scp $PSScriptRoot\backend\src\openaide-server "${Server}:/usr/local/bin/openaide-server.new"
Write-Host "  Uploaded openaide-server" -ForegroundColor Green

# Upload CLI
scp $PSScriptRoot\terminal\openaide "${Server}:/usr/local/bin/openaide"
Write-Host "  Uploaded openaide (CLI)" -ForegroundColor Green

# Upload install scripts
scp $PSScriptRoot\install.sh "${Server}:/tmp/openaide-install.sh"
scp $PSScriptRoot\uninstall.sh "${Server}:/tmp/openaide-uninstall.sh"
Write-Host "  Uploaded install scripts" -ForegroundColor Green

# Remote update
Write-Host ""
Write-Host "Running remote update..." -ForegroundColor Yellow

$remoteScript = @"
systemctl stop openaide 2>/dev/null || true

# Backup old binary
cp /usr/local/bin/openaide-server /usr/local/bin/openaide-server.backup 2>/dev/null || true

# Switch new binary
mv /usr/local/bin/openaide-server.new /usr/local/bin/openaide-server
chmod +x /usr/local/bin/openaide-server
chmod +x /usr/local/bin/openaide

# Ensure config directory exists
mkdir -p /opt/openaide/.openaide

# Reload systemd
systemctl daemon-reload
"@

if ($Restart) {
    $remoteScript += "`nsystemctl start openaide`n"
    $remoteScript += "sleep 3`n"
    $remoteScript += "systemctl status openaide --no-pager`n"
}

$remoteScript += "`necho ''`n"
$remoteScript += "echo 'CLI test:'`n"
$remoteScript += "/usr/local/bin/openaide --help | head -3`n"

ssh $Server $remoteScript

Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "           Deploy Complete!             " -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "Commands:" -ForegroundColor Cyan
Write-Host "  Status:  ssh $Server 'systemctl status openaide'"
Write-Host "  Restart: ssh $Server 'systemctl restart openaide'"
Write-Host "  Logs:    ssh $Server 'tail -f /var/log/openaide/server.log'"
Write-Host "  CLI:     ssh $Server 'openaide'"
Write-Host ""
Write-Host "Install on server:" -ForegroundColor Cyan
Write-Host "  ssh $Server 'bash /tmp/openaide-install.sh'"
Write-Host ""
