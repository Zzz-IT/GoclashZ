param(
    [Parameter(Mandatory=$true)]
    [string]$Version
)

$Version = $Version.TrimStart("v")

Write-Host "Setting GoclashZ version to $Version..."

# 1. Update version.go
$vgo = Get-Content core/version/version.go -Raw
$vgo = $vgo -replace 'var AppVersion = ".*"', "var AppVersion = `"v$Version`""
Set-Content core/version/version.go $vgo -Encoding UTF8
Write-Host "Updated core/version/version.go"

# 2. Update wails.json
$wails = Get-Content wails.json -Raw | ConvertFrom-Json
$wails.info.productVersion = $Version
$wails | ConvertTo-Json -Depth 20 | Set-Content wails.json -Encoding UTF8
Write-Host "Updated wails.json"

# 3. Update frontend/package.json
$pkg = Get-Content frontend/package.json -Raw | ConvertFrom-Json
$pkg.version = $Version
$pkg | ConvertTo-Json -Depth 20 | Set-Content frontend/package.json -Encoding UTF8
Write-Host "Updated frontend/package.json"

# 4. Update package.iss
$iss = Get-Content package.iss -Raw
$iss = $iss -replace '#define MyAppVersion ".*"', "#define MyAppVersion `"$Version`""
$iss = $iss -replace 'VersionInfoVersion=.*', "VersionInfoVersion=$Version.0"
Set-Content package.iss $iss -Encoding UTF8
Write-Host "Updated package.iss"

Write-Host "Version successfully updated to $Version! You can now commit the changes."
