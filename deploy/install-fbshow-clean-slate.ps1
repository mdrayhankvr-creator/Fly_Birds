# Owner-only installer. Run after the reviewed commit is deployed. It edits
# common.ps1 (a script), while leaving fbshow.env, secrets and volumes alone.
param([string]$Target = 'C:\Users\Laptop Click\Fly_Birds_QA_env\fbshow')
$ErrorActionPreference = 'Stop'
$common = Join-Path $Target 'common.ps1'
if (-not (Test-Path -LiteralPath $common)) { throw "Missing $common" }
$old = '$Override = Join-Path $Here ''quick-login.override.yaml'''
$new = '$Override = Join-Path $Here ''quick-login-off.override.yaml'''
$body = [System.IO.File]::ReadAllText($common)
if (-not $body.Contains($old) -and -not $body.Contains($new)) {
  throw 'common.ps1 has an unknown override setting; review it before installation'
}
$source = $PSScriptRoot
$offSource = Join-Path $source 'fbshow-quick-login-off.override.yaml'
$offTarget = Join-Path $Target 'quick-login-off.override.yaml'
if ((Test-Path -LiteralPath $offTarget) -and
    (Get-FileHash -Algorithm SHA256 -LiteralPath $offSource).Hash -ne (Get-FileHash -Algorithm SHA256 -LiteralPath $offTarget).Hash) {
  throw "Existing override $offTarget differs; review it before installation"
}
foreach ($pair in @(@('fbshow-clean-slate.ps1', 'clean-slate.ps1'), @('fbshow-quick-login-off.override.yaml', 'quick-login-off.override.yaml'))) {
  $from = Join-Path $source $pair[0]
  $to = Join-Path $Target $pair[1]
  if (Test-Path -LiteralPath $to) {
    if ($pair[1] -like '*.yaml') {
      if ((Get-FileHash -Algorithm SHA256 -LiteralPath $from).Hash -ne (Get-FileHash -Algorithm SHA256 -LiteralPath $to).Hash) {
        throw "Existing override $to differs; review it before installation"
      }
      continue
    }
    $backup = "$to.$(Get-Date -Format 'yyyyMMdd-HHmmss').bak"
    Copy-Item -LiteralPath $to -Destination $backup
  }
  Copy-Item -LiteralPath $from -Destination $to -Force
}
if ($body.Contains($old)) {
  $backup = "$common.$(Get-Date -Format 'yyyyMMdd-HHmmss').bak"
  Copy-Item -LiteralPath $common -Destination $backup
  [System.IO.File]::WriteAllText($common, $body.Replace($old, $new), [System.Text.UTF8Encoding]::new($false))
}
Write-Host 'Installed versioned clean-slate wrapper and disabled quick-login override in common.ps1.'
Write-Host 'Run update.ps1 after the reviewed commit is merged; it will use the new override.'
