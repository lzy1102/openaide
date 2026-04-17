@echo off
chcp 65001 >nul
echo ==============================
echo OpenAIDE Build Script
echo ==============================

:: Check Go environment
go version >nul 2>&1
if %errorlevel% neq 0 (
    echo Please install Go first. Go 1.20+
    echo Download: https://golang.org/dl/
    pause
    exit /b 1
)

:: Create output directory
echo Creating output directory...
mkdir -p dist\frontend
mkdir -p dist\storage

:: Build backend
echo Building backend...
cd backend

:: Build Windows version
echo Building Windows version...
go build -o ..\dist\openaide-server.exe .\src

:: Build Linux version
echo Building Linux version...
set GOOS=linux
set GOARCH=amd64
go build -o ..\dist\openaide-server-linux-amd64 .\src

:: Build macOS version
echo Building macOS version...
set GOOS=darwin
set GOARCH=amd64
go build -o ..\dist\openaide-server-darwin-amd64 .\src

:: Reset GOOS and GOARCH
set GOOS=
set GOARCH=

cd ..

:: Copy frontend
echo Copying frontend...
xcopy frontend\* dist\frontend\ /s /e

:: Create config
echo Creating config...
echo PORT=19375 > dist\.env
echo STORAGE_PATH=./storage >> dist\.env

:: Create start script
echo Creating start script...

:: Windows start script
echo @echo off > dist\start.bat
echo echo Starting OpenAIDE... >> dist\start.bat
echo start /B .\openaide-server.exe >> dist\start.bat
echo start python frontend\serve.py >> dist\start.bat
echo echo OpenAIDE is running... >> dist\start.bat
echo echo Web: http://localhost:19375 >> dist\start.bat
echo echo API: http://localhost:19375/api >> dist\start.bat
echo pause >> dist\start.bat

:: Linux/macOS start script
echo #!/bin/bash > dist\start.sh
echo echo "Starting OpenAIDE..." >> dist\start.sh
echo ./openaide-server-linux-amd64 & >> dist\start.sh
echo python3 frontend/serve.py & >> dist\start.sh
echo echo "OpenAIDE is running..." >> dist\start.sh
echo echo "Web: http://localhost:19375" >> dist\start.sh
echo echo "API: http://localhost:19375/api" >> dist\start.sh

:: Create archive
echo Creating archive...
cd dist
powershell Compress-Archive -Path * -DestinationPath ..\openaide-windows.zip
powershell Compress-Archive -Path * -DestinationPath ..\openaide-linux.zip
powershell Compress-Archive -Path * -DestinationPath ..\openaide-macos.zip
cd ..

:: Done
echo ==============================
echo Build Complete!
echo ==============================
echo Output files:
echo - openaide-windows.zip
echo - openaide-linux.zip
echo - openaide-macos.zip
echo ==============================
pause
