@echo off
setlocal enabledelayedexpansion
title OpenAIDE Installer

echo.
echo   OpenAIDE Installer for Windows
echo   ------------------------------
echo.

set INSTALL_DIR=%USERPROFILE%\.openaide
set BIN_DIR=%INSTALL_DIR%\bin
set CONFIG_FILE=%INSTALL_DIR%\config.yaml

:: 1. Create directories
echo   [1/3] Creating directories...
if not exist "%BIN_DIR%" mkdir "%BIN_DIR%"
if not exist "%INSTALL_DIR%\data\prompts" mkdir "%INSTALL_DIR%\data\prompts"
if not exist "%INSTALL_DIR%\logs" mkdir "%INSTALL_DIR%\logs"

:: 2. Download binary
echo   [2/3] Downloading binary...
set BIN_URL=https://github.com/lzy1102/openaide/releases/latest/download/openaide-windows-amd64.exe
set BIN_PATH=%BIN_DIR%\openaide.exe

curl -L -o "%BIN_PATH%" "%BIN_URL%" 2>nul
if %ERRORLEVEL% NEQ 0 (
    echo   Download failed. Trying PowerShell fallback...
    powershell -Command "Invoke-WebRequest -Uri '%BIN_URL%' -OutFile '%BIN_PATH%' -UseBasicParsing" 2>nul
)
if exist "%BIN_PATH%" (
    echo   Binary: %BIN_PATH%
) else (
    echo   ERROR: Could not download binary.
    echo   Please download manually from:
    echo   https://github.com/lzy1102/openaide/releases
    pause
    exit /b 1
)

:: 3. Add to PATH
echo   [3/3] Adding to PATH...
set PATH_CHECK=%PATH:!BIN_DIR!=%
if "!PATH_CHECK!"=="%PATH%" (
    setx PATH "%PATH%;%BIN_DIR%" >nul
    set PATH=%PATH%;%BIN_DIR%
    echo   Added to PATH
) else (
    echo   Already in PATH
)

:: 4. Create config if missing
if not exist "%CONFIG_FILE%" (
    echo llm:> "%CONFIG_FILE%"
    echo   providers:>> "%CONFIG_FILE%"
    echo     - name: deepseek>> "%CONFIG_FILE%"
    echo       type: openai>> "%CONFIG_FILE%"
    echo       base_url: https://api.deepseek.com/v1>> "%CONFIG_FILE%"
    echo       api_key: sk-your-api-key-here>> "%CONFIG_FILE%"
    echo       default_model: deepseek-v4-pro>> "%CONFIG_FILE%"
    echo       timeout: 300>> "%CONFIG_FILE%"
    echo       enabled: true>> "%CONFIG_FILE%"
    echo     - name: deepseek-flash>> "%CONFIG_FILE%"
    echo       type: openai>> "%CONFIG_FILE%"
    echo       base_url: https://api.deepseek.com/v1>> "%CONFIG_FILE%"
    echo       api_key: sk-your-api-key-here>> "%CONFIG_FILE%"
    echo       default_model: deepseek-v4-flash>> "%CONFIG_FILE%"
    echo       timeout: 120>> "%CONFIG_FILE%"
    echo       enabled: true>> "%CONFIG_FILE%"
    echo   model_routing:>> "%CONFIG_FILE%"
    echo     reasoning: deepseek-v4-pro>> "%CONFIG_FILE%"
    echo     execution: deepseek-v4-flash>> "%CONFIG_FILE%"
    echo kernel:>> "%CONFIG_FILE%"
    echo   max_rounds: 30>> "%CONFIG_FILE%"
    echo   min_rounds: 8>> "%CONFIG_FILE%"
    echo log:>> "%CONFIG_FILE%"
    echo   level: info>> "%CONFIG_FILE%"
    echo   lang: zh>> "%CONFIG_FILE%"
    echo Config created. EDIT IT and set your api_key!
)

echo.
echo   Done! Next steps:
echo   1. Edit %CONFIG_FILE% and set your API key
echo   2. Open a NEW terminal and run: openaide
echo.
pause
