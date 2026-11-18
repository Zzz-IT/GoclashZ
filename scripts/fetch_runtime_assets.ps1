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
#
# 取"latest"的内核资产有两条路径，API 优先、Web 兜底：
#   A) GitHub REST API /repos/MetaCubeX/mihomo/releases/latest
#   B) github.com/.../releases/latest 的 302 重定向 + expanded_assets 网页端点
#
# 为什么要 B 兜底（2026-11-19 实测）：CI runner 走共享出口 IP，未认证的 GitHub API
# 配额只有 60 次/小时，同 IP 的其他任务很容易把它耗尽，表现为这条步骤毫无征兆地
# exit code 1（本地复现全部下载都正常）。路径 B 完全不消耗 API 配额。
# API 是结构化数据、更好解析；Web 路径只是兜底，所以顺序是 A -> B。
function Get-LatestMihomoAsset {
  # 路径 A：REST API
  try {
    $release = Invoke-RestMethod "https://api.github.com/repos/MetaCubeX/mihomo/releases/latest" -Headers @{ "User-Agent" = "GoclashZ-Builder" } -TimeoutSec 30
    $asset = $release.assets | Where-Object { $_.name -match "^mihomo-windows-amd64.*\.zip$" } | Select-Object -First 1
    if ($null -ne $asset) {
      Write-Host "mihomo asset resolved via REST API: $($asset.name)"
      return @{ tag = $release.tag_name; name = $asset.name; url = $asset.browser_download_url }
    }
    Write-Host "REST API 返回的资产里没有 windows-amd64 zip，改用 Web 兜底路径"
  } catch {
    Write-Host "REST API 调用失败（通常是未认证配额耗尽），改用 Web 兜底路径: $($_.Exception.Message)"
  }

  # 路径 B：302 重定向拿 tag + expanded_assets 网页端点拿文件名（两者都不消耗 API 配额）
  # 用 curl.exe 探测重定向：Invoke-WebRequest 对 302 + MaximumRedirection 0
  # 在 PS 5.1 / PS 7 下行为不一致（抛错或吞响应），curl 行为稳定且 runner 自带。
  $tag = $null
  $curlHead = & curl.exe -sI "https://github.com/MetaCubeX/mihomo/releases/latest" 2>$null | Out-String
  $m = [regex]::Match($curlHead, "Location:\s*\S*/releases/tag/(?<tag>[^\s/]+)")
  if ($m.Success) {
    $tag = $m.Groups["tag"].Value
  }
  if ([string]::IsNullOrWhiteSpace($tag)) {
    throw "无法解析 mihomo latest 版本（REST API 与 302 重定向都失败）"
  }

  $html = (Invoke-WebRequest -Uri "https://github.com/MetaCubeX/mihomo/releases/expanded_assets/$tag" -UseBasicParsing -TimeoutSec 60).Content
  $pattern = 'href="(/MetaCubeX/mihomo/releases/download/[^"]+/(?<name>mihomo-windows-amd64[^"]*\.zip))"'
  $m2 = [regex]::Matches($html, $pattern)
  if ($m2.Count -eq 0) {
    throw "mihomo windows amd64 asset not found (Web 兜底路径, tag=$tag)"
  }

  $name = $m2[0].Groups["name"].Value
  $url = "https://github.com" + $m2[0].Groups[1].Value
  Write-Host "mihomo asset resolved via Web fallback: $name (tag=$tag)"
  return @{ tag = $tag; name = $name; url = $url }
}

$asset = Get-LatestMihomoAsset

$tmp = Join-Path $env:TEMP $asset.name
Download-File $asset.url $tmp

$extract = Join-Path $env:TEMP ("mihomo_" + [guid]::NewGuid())
Expand-Archive -Path $tmp -DestinationPath $extract -Force

$exe = Get-ChildItem $extract -Recurse -Filter "*.exe" | Select-Object -First 1
if ($null -eq $exe) {
  throw "mihomo exe not found in archive"
}

Copy-Item $exe.FullName (Join-Path $out "clash.exe") -Force
Add-AssetManifest "clash.exe" (Join-Path $out "clash.exe") $asset.url $asset.tag

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
