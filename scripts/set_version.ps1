param(
    [Parameter(Mandatory=$true)]
    [string]$Version
)

$Version = $Version.TrimStart("v")

if ($Version -notmatch '^\d+\.\d+\.\d+$') {
    throw "Invalid version: $Version. Expected X.Y.Z"
}

Write-Host "Setting GoclashZ version to $Version..."

function Write-Utf8NoBom($Path, $Content) {
    $utf8NoBom = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText((Resolve-Path $Path), $Content, $utf8NoBom)
}

# version.go
$vgo = Get-Content core/version/version.go -Raw -Encoding UTF8
$vgo = $vgo -replace 'var AppVersion = ".*"', "var AppVersion = `"v$Version`""
Write-Utf8NoBom "core/version/version.go" $vgo

# wails.json
$wails = Get-Content wails.json -Raw -Encoding UTF8
$wails = $wails -replace '"productVersion"\s*:\s*"[^"]*"', "`"productVersion`": `"$Version`""
Write-Utf8NoBom "wails.json" $wails

# frontend/package.json
$pkg = Get-Content frontend/package.json -Raw -Encoding UTF8
$pkg = $pkg -replace '"version"\s*:\s*"[^"]*"', "`"version`": `"$Version`""
Write-Utf8NoBom "frontend/package.json" $pkg

# frontend/package.json.md5
# Wails v2 用这个 md5 判断是否需要重新 npm install；不同步会导致每次构建多跑一次安装
$md5 = $null
try { $md5 = (Get-FileHash -Path "frontend\package.json" -Algorithm MD5).Hash.ToLower() } catch { $md5 = $null }
if (-not $md5 -or $md5.Length -ne 32) {
    # 兜底：某些环境下 Get-FileHash 会拿不到句柄，直接用文件流算
    $fs = [System.IO.File]::OpenRead((Join-Path (Get-Location) "frontend\package.json"))
    $alg = New-Object System.Security.Cryptography.MD5CryptoServiceProvider
    $md5 = ([System.BitConverter]::ToString($alg.ComputeHash($fs))).Replace("-", "").ToLower()
    $fs.Close()
    $alg.Dispose()
}
if (-not $md5 -or $md5.Length -ne 32) { throw "package.json.md5 计算失败" }
Write-Utf8NoBom "frontend/package.json.md5" $md5

# package.iss
$iss = Get-Content package.iss -Raw -Encoding UTF8
$iss = $iss -replace '#define MyAppVersion ".*"', "#define MyAppVersion `"$Version`""
$iss = $iss -replace 'VersionInfoVersion=.*', "VersionInfoVersion=$Version.0"
Write-Utf8NoBom "package.iss" $iss

Write-Host "Version successfully updated to $Version."
