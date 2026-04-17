# OpenAIDE Build Script

# Create dist directory structure
New-Item -ItemType Directory -Path 'dist' -Force
New-Item -ItemType Directory -Path 'dist\frontend' -Force
New-Item -ItemType Directory -Path 'dist\storage' -Force

# Build backend service (Windows version)
go build -o dist\openaide-server.exe backend\src

# Copy frontend files
Copy-Item -Path 'frontend\*' -Destination 'dist\frontend' -Recurse -Force

# Create environment config file
Set-Content -Path 'dist\.env' -Value "PORT=19375`nSTORAGE_PATH=./storage"

# Create Windows start script
Set-Content -Path 'dist\start.bat' -Value "@echo off`necho Starting OpenAIDE...`nstart /B .\openaide-server.exe`nstart python frontend\serve.py`necho OpenAIDE is running`necho Web: http://localhost:19375`necho API: http://localhost:19375/api`npause"

# Show build result
Write-Host "Build complete"
Write-Host "Generated files:"
Get-ChildItem -Path 'dist'
