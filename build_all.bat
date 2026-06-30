@echo off
REM build_all.bat - 构建 GoclashZ 主程序和 Helper 服务

echo === Building GoclashZ ===

echo [1/2] Building GoclashZ.exe (wails)...
wails build -platform windows/amd64
if %ERRORLEVEL% neq 0 (
    echo ERROR: Wails build failed
    exit /b 1
)

echo [2/2] Building GoclashZHelper.exe...
go build -ldflags "-s -w" -o build\bin\GoclashZHelper.exe .\cmd\goclashz-helper\
if %ERRORLEVEL% neq 0 (
    echo ERROR: Helper build failed
    exit /b 1
)

echo === Fetching Runtime Assets ===
powershell -ExecutionPolicy Bypass -File .\scripts\fetch_runtime_assets.ps1
if %ERRORLEVEL% neq 0 (
    echo ERROR: Fetch runtime assets failed
    exit /b 1
)

echo === Validating Assets ===
powershell -ExecutionPolicy Bypass -File .\scripts\validate_assets.ps1 -AssetRoot .\build\runtime-assets\core\bin
if %ERRORLEVEL% neq 0 (
    echo ERROR: Asset validation failed
    exit /b 1
)

echo === Build complete ===
dir build\bin\*.exe
