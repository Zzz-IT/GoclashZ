@echo off
setlocal
chcp 65001 >nul
REM build_local.bat - quick local build (no runtime asset download / validation)
set "PATH=%USERPROFILE%\go\bin;C:\Program Files\Go\bin;C:\Program Files\nodejs;%PATH%"
pushd "%~dp0"

echo === Building GoclashZ (local only) ===

echo [1/2] Building GoclashZ.exe (wails3 build)...
call wails3 build
if errorlevel 1 goto fail_wails

echo [2/3] Building GoclashZHelper.exe...
go build -ldflags "-s -w" -o bin\GoclashZHelper.exe .\cmd\goclashz-helper\
if errorlevel 1 goto fail_helper

REM [3/3] 把内置运行时资产（clash.exe / wintun.dll / geo 数据）同步到 bin\seed，
REM 否则从 bin 直接运行时 RepairFromSeed 找不到种子库，会一直报"缺少 Wintun 驱动"。
echo [3/3] Seeding runtime assets...
if not exist "build\runtime-assets\core\bin" (
  echo WARN: build\runtime-assets\core\bin 不存在，跳过 seed（可运行 scripts\fetch_runtime_assets.ps1）
  goto after_seed
)
if not exist "bin\seed\core\bin" mkdir "bin\seed\core\bin"
robocopy "build\runtime-assets\core\bin" "bin\seed\core\bin" /NFL /NDL /NJH /NJS /NP >nul
if exist "build\runtime-assets\core\asset-manifest.json" copy /Y "build\runtime-assets\core\asset-manifest.json" "bin\seed\core\asset-manifest.json" >nul
REM fetch_runtime_assets.ps1 有时不产出 clash.exe，这里从本地数据目录补齐，保证 seed 完整可自愈
if not exist "bin\seed\core\bin\clash.exe" if exist "data\core\bin\clash.exe" copy /Y "data\core\bin\clash.exe" "bin\seed\core\bin\clash.exe" >nul
if errorlevel 8 echo WARN: seed 复制异常，请检查 build\runtime-assets

:after_seed
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
