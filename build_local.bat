@echo off
REM build_local.bat - 仅编译，不下载运行时资产

echo === Building GoclashZ (local only) ===

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

echo === Build complete ===
dir build\bin\*.exe
