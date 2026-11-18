param(
    [Parameter(Mandatory=$true)]
    [string]$Version
)

# GoclashZ (Wails v3) - 版本号注入脚本
# 单一版本来源: build/config.yml 的 info.version；本脚本把它同步到所有需要的文件。
#   1. core/version/version.go         -> var AppVersion = "vX.Y.Z"
#   2. build/config.yml                -> info.version: "X.Y.Z"
#   3. build/windows/info.json         -> Windows VERSIONINFO (file_version/product_version/FileVersion/ProductVersion)
#   4. build/windows/wails.exe.manifest-> assemblyIdentity version
#   5. package.iss                     -> #define MyAppVersion 默认值
#   6. frontend/package.json           -> "version"
# v2 的 wails.json 与 frontend/package.json.md5 已随迁移删除，不再写入。
# 脚本可重复执行（幂等）：内容未变化的文件不会被重写。

$Version = $Version.Trim()

if ($Version -match '^[vV]') { $Version = $Version.Substring(1) }

if ($Version -notmatch '^\d+\.\d+\.\d+$') {
    throw "Invalid version: $Version. Expected X.Y.Z"
}

$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

Write-Host "Setting GoclashZ version to $Version..."

function Read-Utf8PreserveBom([string]$Path) {
    $bytes = [System.IO.File]::ReadAllBytes($Path)
    $hasBom = ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF)
    $offset = 0
    if ($hasBom) { $offset = 3 }
    $text = (New-Object System.Text.UTF8Encoding($false)).GetString($bytes, $offset, $bytes.Length - $offset)
    return [pscustomobject]@{ Text = $text; HasBom = $hasBom }
}

function Write-Utf8PreserveBom([string]$Path, [string]$Text, [bool]$HasBom) {
    $enc = New-Object System.Text.UTF8Encoding($HasBom)
    [System.IO.File]::WriteAllText($Path, $Text, $enc)
}

# 用正则就地替换，保持文件其余内容/换行/编码不变；返回是否有改动。
function Update-File([string]$Path, [string]$Pattern, [string]$Replacement, [string]$Label) {
    if (-not (Test-Path $Path)) {
        throw "Missing file: $Path ($Label)"
    }
    $state = Read-Utf8PreserveBom $Path
    $updated = [regex]::Replace($state.Text, $Pattern, $Replacement)
    if ($updated -ceq $state.Text) {
        Write-Host "  [same]  $Label"
        return $false
    }
    Write-Utf8PreserveBom $Path $updated $state.HasBom
    Write-Host "  [write] $Label"
    return $true
}

$changed = 0

# 1. core/version/version.go
if (Update-File "core/version/version.go" '(?m)^(var AppVersion = ")[^"]*(")' ('${1}v' + $Version + '${2}') "core/version/version.go") { $changed++ }

# 2. build/config.yml (info.version, 2 空格缩进)
if (Update-File "build/config.yml" '(?m)^([ \t]{2}version:[ \t]*")[^"]*(")' ('${1}' + $Version + '${2}') "build/config.yml (info.version)") { $changed++ }

# 3. build/windows/info.json (VERSIONINFO -> syso)
if (Update-File "build/windows/info.json" '("(?:file_version|product_version|FileVersion|ProductVersion)":[ \t]*")[^"]*(")' ('${1}' + $Version + '${2}') "build/windows/info.json") { $changed++ }

# 4. build/windows/wails.exe.manifest (assemblyIdentity version)
if (Update-File "build/windows/wails.exe.manifest" '(name="com\.zzzdream\.goclashz" version=")[^"]*(")' ('${1}' + $Version + '${2}') "build/windows/wails.exe.manifest") { $changed++ }

# 5. package.iss (默认值；CI 仍可用 /DMyAppVersion= 覆盖)
if (Update-File "package.iss" '(?m)^([ \t]*#define MyAppVersion ")[^"]*(")' ('${1}' + $Version + '${2}') "package.iss (MyAppVersion)") { $changed++ }

# 6. frontend/package.json
if (Update-File "frontend/package.json" '(?m)^([ \t]{2}"version":[ \t]*")[^"]*(")' ('${1}' + $Version + '${2}') "frontend/package.json") { $changed++ }

# 7. build/linux/nfpm/nfpm.yaml（Linux 打包元数据；Windows 发布不用，但版本必须一致）
if (Update-File "build/linux/nfpm/nfpm.yaml" '(?m)^(version:[ \t]*")[^"]*(")' ('${1}' + $Version + '${2}') "build/linux/nfpm/nfpm.yaml") { $changed++ }

# 8. frontend/package-lock.json（只改根包与 packages."" 两处，绝不动依赖版本）
#    依赖条目形如 "node_modules/x": { "version": "1.2.3" }，所以按"只替换前 2 处"处理。
if (Test-Path "frontend/package-lock.json") {
    $lockState = Read-Utf8PreserveBom "frontend/package-lock.json"
    $lockRegex = [regex]'"version"[ \t]*:[ \t]*"[^"]*"'
    $lockUpdated = $lockRegex.Replace($lockState.Text, ('"version": "' + $Version + '"'), 2)
    if ($lockUpdated -cne $lockState.Text) {
        Write-Utf8PreserveBom "frontend/package-lock.json" $lockUpdated $lockState.HasBom
        Write-Host "  [write] frontend/package-lock.json (root version x2)"
        $changed++
    } else {
        Write-Host "  [same]  frontend/package-lock.json"
    }
}

Write-Host "Version successfully updated to $Version ($changed file(s) changed)."
