# Offline file-operation tests. Does not run the installer, download, or modify PATH.
$ErrorActionPreference = 'Stop'
$tokens = $null
$parseErrors = $null
$installer = Join-Path $PSScriptRoot '../install.ps1'
$ast = [System.Management.Automation.Language.Parser]::ParseFile(
  (Resolve-Path $installer), [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
$function = $ast.Find({ param($node)
  $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
  $node.Name -eq 'Install-BadgerExecutable'
}, $true)
if (-not $function) { throw 'Installer replacement function missing' }
. ([scriptblock]::Create($function.Extent.Text))

# Simulate file-operation failures deterministically while using real temp files.
$script:lockedBackup = $null
$script:failCopy = $false
function Remove-Item {
  param([string]$LiteralPath, [switch]$Force, [string]$ErrorAction)
  if ($LiteralPath -eq $script:lockedBackup) {
    if ($ErrorAction -eq 'SilentlyContinue') { return }
    throw 'Simulated running backup lock'
  }
  Microsoft.PowerShell.Management\Remove-Item -LiteralPath $LiteralPath -Force:$Force -ErrorAction $ErrorAction
}
function Copy-Item {
  param([string]$LiteralPath, [string]$Destination, [switch]$Force, [string]$ErrorAction)
  if ($script:failCopy) {
    [IO.File]::WriteAllText($Destination, 'partial')
    throw 'Simulated copy failure'
  }
  Microsoft.PowerShell.Management\Copy-Item -LiteralPath $LiteralPath -Destination $Destination -Force:$Force -ErrorAction $ErrorAction
}
function Assert-Content($Path, $Expected) {
  if ([IO.File]::ReadAllText($Path) -ne $Expected) { throw "Unexpected content: $Path" }
}
$temp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temp | Out-Null
try {
  $source = Join-Path $temp 'new.exe'
  $target = Join-Path $temp 'badger.exe'
  [IO.File]::WriteAllText($source, 'new')
  Install-BadgerExecutable $source $target
  Assert-Content $target 'new'

  [IO.File]::WriteAllText($target, 'old')
  Install-BadgerExecutable $source $target
  Assert-Content $target 'new'
  if (Test-Path -LiteralPath "$target.old") { throw 'Unlocked backup was not removed' }

  [IO.File]::WriteAllText($target, 'old')
  $script:failCopy = $true
  $failed = $false
  try { Install-BadgerExecutable $source $target } catch { $failed = $true }
  if (-not $failed) { throw 'Copy failure was swallowed' }
  Assert-Content $target 'old'
  $script:failCopy = $false

  # First upgrade leaves a running backup; the next must choose another name.
  $script:lockedBackup = "$target.old"
  Install-BadgerExecutable $source $target
  Assert-Content $target 'new'
  Assert-Content "$target.old" 'old'
  [IO.File]::WriteAllText($target, 'second')
  Install-BadgerExecutable $source $target
  Assert-Content $target 'new'
  Assert-Content "$target.old" 'old'
  $backups = @(Get-ChildItem -LiteralPath $temp -Filter 'badger.exe.old.*')
  if ($backups.Count) { throw 'Unlocked unique backup was not removed' }
  Write-Host 'PASS: fresh install, upgrade, partial-copy rollback, locked backup, repeated upgrade'
} finally {
  Microsoft.PowerShell.Management\Remove-Item -LiteralPath $temp -Recurse -Force
}
