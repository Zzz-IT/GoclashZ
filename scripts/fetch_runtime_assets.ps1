$ErrorActionPreference = "Stop"

$root = Resolve-Path "."
$out = Join-Path $root "build\runtime-assets\core\bin"
New-Item -ItemType Directory -Force -Path $out | Out-Null

$manifest = @{
  generatedAt = (Get-Date).ToUniversalTime().ToString("o")
  assets = @()
}

function Get-Sha256($path) {
  return (Get-FileHash -Algorithm SHA256 $path).Hash.ToLower()
}

function Add-AssetManifest($name, $path, $source, $version) {
  $item = @{
    name = $name
    path = "core/bin/$name"
    source = $source
    version = $version
    size = (Get-Item $path).Length
    sha256 = Get-Sha256 $path
  }
  $manifest.assets += $item
}

function Download-File($url, $dest) {
  Write-Host "Downloading $url"
  Invoke-WebRequest -Uri $url -OutFile $dest -UseBasicParsing
  if (!(Test-Path $dest) -or ((Get-Item $dest).Length -le 0)) {
    throw "Downloaded file is empty: $dest"
  }
}

# 1. mihomo latest
$release = Invoke-RestMethod "https://api.github.com/repos/MetaCubeX/mihomo/releases/latest"
$asset = $release.assets | Where-Object { $_.name -match "mihomo-windows-amd64.*\.zip$" } | Select-Object -First 1
if ($null -eq $asset) {
  throw "mihomo windows amd64 asset not found"
}

$tmp = Join-Path $env:TEMP $asset.name
Download-File $asset.browser_download_url $tmp

$extract = Join-Path $env:TEMP ("mihomo_" + [guid]::NewGuid())
Expand-Archive -Path $tmp -DestinationPath $extract -Force

$exe = Get-ChildItem $extract -Recurse -Filter "*.exe" | Select-Object -First 1
if ($null -eq $exe) {
  throw "mihomo exe not found in archive"
}

Copy-Item $exe.FullName (Join-Path $out "clash.exe") -Force
Add-AssetManifest "clash.exe" (Join-Path $out "clash.exe") $asset.browser_download_url $release.tag_name

# 2. geo assets
$downloads = @(
  @{ name = "geoip.metadb"; url = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.metadb" },
  @{ name = "geosite.dat"; url = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat" },
  @{ name = "country.mmdb"; url = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb" },
  @{ name = "asn.dat"; url = "https://github.com/xishang0128/geoip/releases/download/latest/GeoLite2-ASN.mmdb" }
)

foreach ($d in $downloads) {
  $dest = Join-Path $out $d.name
  Download-File $d.url $dest
  Add-AssetManifest $d.name $dest $d.url "latest"
}

# 3. wintun
$wintunZip = Join-Path $env:TEMP "wintun.zip"
Download-File "https://www.wintun.net/builds/wintun-0.14.1.zip" $wintunZip

$wintunExtract = Join-Path $env:TEMP ("wintun_" + [guid]::NewGuid())
Expand-Archive -Path $wintunZip -DestinationPath $wintunExtract -Force

$wintunDll = Get-ChildItem $wintunExtract -Recurse -Filter "wintun.dll" |
  Where-Object { $_.FullName -match "amd64" } |
  Select-Object -First 1

if ($null -eq $wintunDll) {
  throw "wintun amd64 dll not found"
}

Copy-Item $wintunDll.FullName (Join-Path $out "wintun.dll") -Force
Add-AssetManifest "wintun.dll" (Join-Path $out "wintun.dll") "https://www.wintun.net/builds/wintun-0.14.1.zip" "0.14.1"

# 4. validate
$required = @(
  "clash.exe",
  "wintun.dll",
  "geoip.metadb",
  "geosite.dat",
  "country.mmdb",
  "asn.dat"
)

foreach ($name in $required) {
  $path = Join-Path $out $name
  if (!(Test-Path $path)) {
    throw "Missing runtime asset: $name"
  }
  if ((Get-Item $path).Length -le 1024) {
    throw "Runtime asset too small: $name"
  }
}

$manifestPath = Join-Path $root "build\runtime-assets\core\asset-manifest.json"
New-Item -ItemType Directory -Force -Path (Split-Path $manifestPath) | Out-Null
$manifest | ConvertTo-Json -Depth 10 | Set-Content -Encoding UTF8 $manifestPath

Write-Host "Runtime assets prepared successfully."
