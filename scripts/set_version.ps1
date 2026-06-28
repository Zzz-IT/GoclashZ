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
$vgo = Get-Content core/version/version.go -Raw
$vgo = $vgo -replace 'var AppVersion = ".*"', "var AppVersion = `"v$Version`""
Write-Utf8NoBom "core/version/version.go" $vgo

# wails.json
$wails = Get-Content wails.json -Raw
$wails = $wails -replace '"productVersion"\s*:\s*"[^"]*"', "`"productVersion`": `"$Version`""
Write-Utf8NoBom "wails.json" $wails

# frontend/package.json
$pkg = Get-Content frontend/package.json -Raw
$pkg = $pkg -replace '"version"\s*:\s*"[^"]*"', "`"version`": `"$Version`""
Write-Utf8NoBom "frontend/package.json" $pkg

# package.iss
$iss = Get-Content package.iss -Raw
$iss = $iss -replace '#define MyAppVersion ".*"', "#define MyAppVersion `"$Version`""
$iss = $iss -replace 'VersionInfoVersion=.*', "VersionInfoVersion=$Version.0"
Write-Utf8NoBom "package.iss" $iss

Write-Host "Version successfully updated to $Version."
