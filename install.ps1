# OpenAIDE Windows Installer
# Usage: powershell -ExecutionPolicy Bypass -File install.ps1

$ErrorActionPreference = "Stop"
Write-Host ""
Write-Host "  OpenAIDE Installer for Windows" -ForegroundColor Cyan
Write-Host ""

$InstallDir = "$env:USERPROFILE\.openaide"
$BinDir = "$InstallDir\bin"
$ConfigFile = "$InstallDir\config.yaml"
$Version = "latest"

# 1. Create directories
Write-Host "  [1/4] Creating directories..." -ForegroundColor Gray
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
New-Item -ItemType Directory -Force -Path "$InstallDir\data\prompts" | Out-Null
New-Item -ItemType Directory -Force -Path "$InstallDir\logs" | Out-Null

# 2. Download binary
Write-Host "  [2/4] Downloading binary..." -ForegroundColor Gray
$BinUrl = "https://github.com/lzy1102/openaide/releases/download/$Version/openaide-windows-amd64.exe"
$BinPath = "$BinDir\openaide.exe"

try {
    Invoke-WebRequest -Uri $BinUrl -OutFile $BinPath -UseBasicParsing
} catch {
    Write-Host "  Download failed. Trying latest release..." -ForegroundColor Yellow
    $ReleaseUrl = "https://api.github.com/repos/lzy1102/openaide/releases/latest"
    $Release = Invoke-RestMethod -Uri $ReleaseUrl -UseBasicParsing
    $Asset = $Release.assets | Where-Object { $_.name -like "*windows*" }
    if ($Asset) {
        Invoke-WebRequest -Uri $Asset.browser_download_url -OutFile $BinPath -UseBasicParsing
    } else {
        Write-Host "  Error: Could not download binary. Please install manually." -ForegroundColor Red
        Write-Host "  Download from: https://github.com/lzy1102/openaide/releases" -ForegroundColor Yellow
        exit 1
    }
}
Write-Host "  Binary downloaded to $BinPath" -ForegroundColor Green

# 3. Add to PATH
Write-Host "  [3/4] Adding to PATH..." -ForegroundColor Gray
$currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($currentPath -notlike "*$BinDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$currentPath;$BinDir", "User")
    $env:Path = "$env:Path;$BinDir"
    Write-Host "  Added $BinDir to PATH" -ForegroundColor Green
} else {
    Write-Host "  $BinDir already in PATH" -ForegroundColor Gray
}

# 4. Create default config
Write-Host "  [4/4] Creating default config..." -ForegroundColor Gray
if (-not (Test-Path $ConfigFile)) {
    $Config = @"
llm:
  providers:
    - name: deepseek
      type: openai
      base_url: https://api.deepseek.com/v1
      api_key: sk-your-api-key-here
      default_model: deepseek-v4-pro
      timeout: 300
      enabled: true
    - name: deepseek-flash
      type: openai
      base_url: https://api.deepseek.com/v1
      api_key: sk-your-api-key-here
      default_model: deepseek-v4-flash
      timeout: 120
      enabled: true
  model_routing:
    reasoning: deepseek-v4-pro
    execution: deepseek-v4-flash
kernel:
  max_rounds: 30
  min_rounds: 8
log:
  level: info
  lang: zh
"@
    $Config | Out-File -FilePath $ConfigFile -Encoding utf8
    Write-Host "  Config created at $ConfigFile" -ForegroundColor Yellow
    Write-Host "  IMPORTANT: Edit this file and set your API key!" -ForegroundColor Red
} else {
    Write-Host "  Config already exists at $ConfigFile" -ForegroundColor Gray
}

# Done
Write-Host ""
Write-Host "  Installation complete!" -ForegroundColor Green
Write-Host ""
Write-Host "  Next steps:" -ForegroundColor White
Write-Host "  1. Edit $ConfigFile" -ForegroundColor Yellow
Write-Host "     Set your api_key (get one at https://platform.deepseek.com)" -ForegroundColor Gray
Write-Host "  2. Open a NEW terminal and run: openaide" -ForegroundColor Yellow
Write-Host ""
