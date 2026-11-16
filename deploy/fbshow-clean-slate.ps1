# Installed beside fbshow/common.ps1 after review. Default is a read-only plan.
# Real run: -Execute -School 'exact English name' -Admin1 'Name:email' -Admin2 'Name:email'.
param(
  [switch]$Execute,
  [Parameter(Mandatory=$true)][string]$School,
  [string]$Admin1 = '',
  [string]$Admin2 = ''
)
. (Join-Path $PSScriptRoot 'common.ps1')

if ($Project -ne 'fbshow' -or $Override -ne (Join-Path $Here 'quick-login-off.override.yaml')) {
  throw 'Install the versioned clean-slate scripts first; demo quick-login must stay off.'
}
if (-not (HasData)) { throw 'fbshow has no database volume' }
$dc = Compose
$configuration = (docker @dc config --format json | ConvertFrom-Json)
Check 'read Compose configuration'
if ($configuration.services.backend.environment.DEMO_QUICK_LOGIN -ne 'false') {
  throw 'The effective backend configuration does not disable DEMO_QUICK_LOGIN'
}

Step 'Read-only reset plan'
docker @dc exec -T backend /app/reset-school-data -school $School -dry-run
Check 'reset dry run'
if (-not $Execute) { return }
if (-not $Admin1) { $Admin1 = Read-Host 'First administrator (Name:email)' }
if (-not $Admin2) { $Admin2 = Read-Host 'Second administrator (Name:email)' }
if (-not $Admin1 -or -not $Admin2 -or $Admin1 -eq $Admin2) {
  throw 'Pass two distinct administrators as Name:email; passwords are requested interactively.'
}

Step 'Create rollback backup using the existing fbshow backup convention'
$rollback = Backup 'pre-clean-slate'
foreach ($name in @('db.dump', 'uploads.tgz', 'commit')) {
  if (-not (Test-Path (Join-Path $rollback $name))) { throw "Rollback backup is missing $name" }
}
docker run --rm -v "${rollback}:/in:ro" postgres:17-alpine pg_restore -f /dev/null /in/db.dump
Check 'verify PostgreSQL dump payload'
docker run --rm -v "${rollback}:/in:ro" alpine:3 tar tzf /in/uploads.tgz > $null
Check 'verify uploads archive'

Step 'Create an application archive with checksums for the reset command'
$line = (docker @dc exec -T backend /app/create-backup | Select-Object -Last 1).Trim()
Check 'create application backup'
if ($line -notmatch '^(flybirds-backup-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8}\.tar\.gz) ([0-9]+) ([0-9a-f]{64})$') {
  throw 'create-backup returned an unexpected receipt'
}
$archive = $Matches[1]
$sha = $Matches[3]
$backend = (docker @dc ps -q backend).Trim()
Check 'find backend container'
if (-not $backend) { throw 'backend container is not running' }
docker cp "${backend}:/app/backups/$archive" (Join-Path $rollback $archive)
Check 'copy application archive outside the Docker volume'
$copiedSHA = (Get-FileHash -Algorithm SHA256 (Join-Path $rollback $archive)).Hash.ToLowerInvariant()
if ($copiedSHA -ne $sha) { throw 'Copied application archive checksum differs' }
$receipt = @("db.dump $((Get-FileHash -Algorithm SHA256 (Join-Path $rollback 'db.dump')).Hash.ToLowerInvariant())", "uploads.tgz $((Get-FileHash -Algorithm SHA256 (Join-Path $rollback 'uploads.tgz')).Hash.ToLowerInvariant())", "$archive $sha")
Set-Content -LiteralPath (Join-Path $rollback 'sha256.txt') -Encoding ascii -Value $receipt
Write-Host "Verified rollback backup: $(Split-Path $rollback -Leaf)" -ForegroundColor Green

Step 'Stop writers, confirm school name and type the two passwords in the terminal'
docker @dc stop web backend
Check 'stop web and backend'
try {
  docker @dc run --rm --no-deps backend /app/reset-school-data -school $School -admin1 $Admin1 -admin2 $Admin2 -backup "/app/backups/$archive" -backup-sha256 $sha -confirm
  Check 'reset school data'
} finally {
  docker @dc up --detach --wait --wait-timeout 600 --force-recreate backend web
  Check 'restart backend and web with quick-login disabled'
}
Wait-Ready
Write-Host "Clean slate complete. Rollback source: $(Split-Path $rollback -Leaf)" -ForegroundColor Green
