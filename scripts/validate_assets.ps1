$required = @(
  ".\build\bin\GoclashZ.exe",
  ".\build\bin\GoclashZHelper.exe",
  ".\data\core\bin\clash.exe",
  ".\data\core\bin\wintun.dll",
  ".\data\core\bin\geoip.metadb",
  ".\data\core\bin\geosite.dat",
  ".\data\core\bin\country.mmdb"
)

$missing = $false
foreach ($f in $required) {
  if (!(Test-Path $f)) {
    Write-Host "ERROR: Missing required package asset: $f" -ForegroundColor Red
    $missing = $true
  }
}

if ($missing) {
  Write-Host "Asset validation failed. Please download required core assets to .\data\core\bin before packaging." -ForegroundColor Red
  exit 1
} else {
  Write-Host "Asset validation passed." -ForegroundColor Green
  exit 0
}
