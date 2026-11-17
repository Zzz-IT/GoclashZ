@echo off
setlocal
REM build_all.bat - Build GoclashZ (Wails v3) main app + Helper service + runtime assets
REM
REM v3 layout: wails3 build runs the Taskfile chain
REM   build -> windows:build -> bindings + frontend build -> go build -> bin\GoclashZ.exe
REM All artefacts land in bin\ (v2 used build\bin\).

set "PATH=%USERPROFILE%\go\bin;C:\Program Files\Go\bin;C:\Program Files\nodejs;%PATH%"
pushd "%~dp0"

echo === Building GoclashZ ===

echo [1/4] Building GoclashZ.exe (wails3 build)...
call wails3 build
if errorlevel 1 goto fail_wails

echo [2/4] Building GoclashZHelper.exe...
go build -ldflags "-s -w" -o bin\GoclashZHelper.exe .\cmd\goclashz-helper\
if errorlevel 1 goto fail_helper

echo [3/4] Fetching Runtime Assets...
powershell -ExecutionPolicy Bypass -File .\scripts\fetch_runtime_assets.ps1
if errorlevel 1 goto fail_assets

echo [4/4] Validating Assets...
powershell -ExecutionPolicy Bypass -File .\scripts\validate_assets.ps1 -AssetRoot .\build\runtime-assets\core\bin
if errorlevel 1 goto fail_validate

echo === Build complete ===
dir bin\*.exe
popd
endlocal
exit /b 0

:fail_wails
echo ERROR: Wails build failed
popd
endlocal
exit /b 1

:fail_helper
echo ERROR: Helper build failed
popd
endlocal
exit /b 1

:fail_assets
echo ERROR: Fetch runtime assets failed
popd
endlocal
exit /b 1

:fail_validate
echo ERROR: Asset validation failed
popd
endlocal
exit /b 1
