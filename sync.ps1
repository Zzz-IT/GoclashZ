# Get latest tag
$latestTag = git tag -l --sort=-v:refname | Select-Object -First 1

if (-not $latestTag) {
    $nextTag = "v0.1.0"
} else {
    # Simple logic to increment patch version
    if ($latestTag -match 'v(\d+)\.(\d+)\.(\d+)') {
        $major = [int]$Matches[1]
        $minor = [int]$Matches[2]
        $patch = [int]$Matches[3] + 1
        $nextTag = "v$major.$minor.$patch"
    } else {
        $nextTag = "v0.1.0"
    }
}

Write-Host "Current tag: $latestTag" -ForegroundColor Cyan
Write-Host "Next tag: $nextTag" -ForegroundColor Green

# Add changes
git add .

# Commit message
$commitMsg = Read-Host "Enter commit message (default: '更新 $nextTag')"
if (-not $commitMsg) { $commitMsg = "更新 $nextTag" }

# Commit
git commit -m $commitMsg

# Tag
git tag -a $nextTag -m $commitMsg

# Push
Write-Host "Pushing to origin..." -ForegroundColor Yellow
git push origin main
git push origin $nextTag

Write-Host "Successfully synced to GitHub with tag $nextTag" -ForegroundColor Green
