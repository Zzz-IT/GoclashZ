param(
    [string]$AssetRoot = ".\data\core\bin"
)

$required = @(
  ".\build\bin\GoclashZ.exe",
  ".\build\bin\GoclashZHelper.exe",
  "$AssetRoot\clash.exe",
  "$AssetRoot\wintun.dll",
  "$AssetRoot\geoip.metadb",
  "$AssetRoot\geosite.dat",
  "$AssetRoot\country.mmdb",
  "$AssetRoot\asn.dat"
)

$missing = $false
foreach ($f in $required) {
  if (!(Test-Path $f)) {
    Write-Host "ERROR: Missing required package asset: $f" -ForegroundColor Red
    $missing = $true
  }
}

if ($missing) {
  Write-Host "Asset validation failed. Please ensure assets are fetched to $AssetRoot before packaging." -ForegroundColor Red
  exit 1
} else {
  Write-Host "Asset validation passed." -ForegroundColor Green
  exit 0
}
