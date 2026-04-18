# Add changes
git add .

# Commit message
$commitMsg = Read-Host "Enter commit message (default: '更新')"
if (-not $commitMsg) { $commitMsg = "更新" }

# Commit
git commit -m $commitMsg

# Push
Write-Host "Pushing to origin main..." -ForegroundColor Yellow
git push origin main

Write-Host "Successfully synced to GitHub" -ForegroundColor Green
