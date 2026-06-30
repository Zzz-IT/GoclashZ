param(
    [string]$AssetRoot = ".\build\runtime-assets"
)

$ErrorCount = 0

function Assert-FileExists {
    param([string]$Path, [string]$Label)
    if (!(Test-Path $Path)) {
        Write-Host "FAIL: $Label 不存在: $Path" -ForegroundColor Red
        $script:ErrorCount++
        return $false
    }
    return $true
}

function Assert-MinSize {
    param([string]$Path, [string]$Label, [int64]$MinBytes)
    $item = Get-Item $Path
    if ($item.Length -lt $MinBytes) {
        Write-Host "FAIL: $Label 体积过小: $($item.Length) bytes (最小预期 $MinBytes bytes)" -ForegroundColor Red
        $script:ErrorCount++
        return $false
    }
    return $true
}

function Assert-MZHeader {
    param([string]$Path, [string]$Label)
    $bytes = [System.IO.File]::ReadAllBytes($Path)
    if ($bytes.Length -lt 2 -or $bytes[0] -ne 0x4D -or $bytes[1] -ne 0x5A) {
        Write-Host "FAIL: $Label 没有有效的 MZ 头 (不是 Windows PE 文件)" -ForegroundColor Red
        $script:ErrorCount++
        return $false
    }
    return $true
}

function Assert-NotHTML {
    param([string]$Path, [string]$Label)
    $header = Get-Content -Path $Path -TotalCount 1 -ErrorAction SilentlyContinue
    if ($header -and ($header -match '<html|<!doctype html|<head')) {
        Write-Host "FAIL: $Label 内容像 HTML 错误页，不是有效的数据文件" -ForegroundColor Red
        $script:ErrorCount++
        return $false
    }
    return $true
}

Write-Host "===== GoclashZ 构建资产校验 =====" -ForegroundColor Cyan
Write-Host "资产目录: $AssetRoot"
Write-Host ""

# === 1. 二进制可执行文件校验 ===

# 主程序
if (Assert-FileExists ".\build\bin\GoclashZ.exe" "主程序") {
    Assert-MZHeader ".\build\bin\GoclashZ.exe" "主程序" | Out-Null
}

# Helper
if (Assert-FileExists ".\build\bin\GoclashZHelper.exe" "Helper 服务") {
    Assert-MZHeader ".\build\bin\GoclashZHelper.exe" "Helper 服务" | Out-Null
}

# === 2. Mihomo 内核校验 ===
$clashPath = "$AssetRoot\clash.exe"
if (Assert-FileExists $clashPath "Mihomo 内核") {
    if (Assert-MZHeader $clashPath "Mihomo 内核") {
        Assert-MinSize $clashPath "Mihomo 内核" (5 * 1024 * 1024) | Out-Null

        # 尝试执行 -v 验证架构
        try {
            $versionOutput = & $clashPath -v 2>&1
            if ($LASTEXITCODE -eq 0 -or $versionOutput) {
                Write-Host "OK: Mihomo 内核可执行, 版本: $($versionOutput | Select-Object -First 1)" -ForegroundColor Green
            } else {
                Write-Host "WARN: Mihomo 内核 -v 无输出 (可能架构不匹配)" -ForegroundColor Yellow
            }
        } catch {
            Write-Host "WARN: Mihomo 内核 -v 执行失败: $_" -ForegroundColor Yellow
        }
    }
}

# === 3. Wintun 驱动 DLL 校验 ===
$wintunPath = "$AssetRoot\wintun.dll"
if (Assert-FileExists $wintunPath "Wintun DLL") {
    Assert-MZHeader $wintunPath "Wintun DLL" | Out-Null
    Assert-MinSize $wintunPath "Wintun DLL" (32 * 1024) | Out-Null
}

# === 4. Geo 数据库文件校验 ===
$geoFiles = @(
    @{ Name = "geoip.metadb"; Label = "GeoIP"; MinSize = 64 * 1024 },
    @{ Name = "geosite.dat"; Label = "GeoSite"; MinSize = 64 * 1024 },
    @{ Name = "country.mmdb"; Label = "MMDB"; MinSize = 64 * 1024 },
    @{ Name = "asn.dat"; Label = "ASN"; MinSize = 64 * 1024 }
)

foreach ($geo in $geoFiles) {
    $geoPath = "$AssetRoot\$($geo.Name)"
    if (Assert-FileExists $geoPath $geo.Label) {
        Assert-MinSize $geoPath $geo.Label $geo.MinSize | Out-Null
        Assert-NotHTML $geoPath $geo.Label | Out-Null
    }
}

# === 5. Manifest SHA256 校验 ===
$manifestPath = "$AssetRoot\asset-manifest.json"
if (Test-Path $manifestPath) {
    try {
        $manifest = Get-Content $manifestPath -Raw | ConvertFrom-Json
        Write-Host "OK: asset-manifest.json 存在, 包含 $($manifest.assets.Count) 个资产" -ForegroundColor Green

        foreach ($asset in $manifest.assets) {
            $assetPath = "$AssetRoot\$($asset.name)"
            if (Test-Path $assetPath) {
                $actualHash = (Get-FileHash -Path $assetPath -Algorithm SHA256).Hash.ToLower()
                if ($asset.sha256 -and $actualHash -ne $asset.sha256.ToLower()) {
                    Write-Host "FAIL: $($asset.name) SHA256 不匹配 (期望: $($asset.sha256), 实际: $actualHash)" -ForegroundColor Red
                    $ErrorCount++
                }
            }
        }
    } catch {
        Write-Host "WARN: asset-manifest.json 解析失败: $_" -ForegroundColor Yellow
    }
} else {
    Write-Host "WARN: asset-manifest.json 不存在，跳过 SHA256 校验" -ForegroundColor Yellow
}

# === 结果汇总 ===
Write-Host ""
if ($ErrorCount -gt 0) {
    Write-Host "===== 校验失败: $ErrorCount 个错误 =====" -ForegroundColor Red
    exit 1
} else {
    Write-Host "===== 全部校验通过 =====" -ForegroundColor Green
    exit 0
}
