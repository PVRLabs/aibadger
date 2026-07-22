#!/usr/bin/env pwsh
# Installer for AIBadger on Windows.
# Usage: irm https://raw.githubusercontent.com/PVRLabs/aibadger/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$repo = if ($env:BADGER_REPO) { $env:BADGER_REPO } else { "PVRLabs/aibadger" }
$installDir = if ($env:BADGER_INSTALL_DIR) { $env:BADGER_INSTALL_DIR } else { "" }
$version = if ($env:BADGER_VERSION) { $env:BADGER_VERSION } else { "" }

# Detect architecture
switch ($env:PROCESSOR_ARCHITECTURE) {
  "AMD64" { $arch = "amd64" }
  default { throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

# Resolve latest version from GitHub redirect
if (-not $version) {
  $latestUrl = try {
    (Invoke-WebRequest -Uri "https://github.com/$repo/releases/latest" -Method Head -UseBasicParsing -MaximumRedirection 0 -ErrorAction Stop).Headers.Location
  } catch {
    (Invoke-WebRequest -Uri "https://github.com/$repo/releases/latest" -UseBasicParsing).Links |
      Where-Object { $_.href -match "/releases/tag/v\d+\.\d+\.\d+" } |
      Select-Object -First 1 -ExpandProperty href
  }
  $version = $latestUrl -replace ".*/tag/", ""
}

if ($version -notmatch '^v\d+\.\d+\.\d+') {
  throw "Invalid version: $version"
}

$versionNumber = $version -replace "^v", ""
$binaryName = "badger.exe"
$archiveName = "badger_${versionNumber}_windows_amd64.zip"
$baseUrl = "https://github.com/$repo/releases/download/$version"
$archiveUrl = "$baseUrl/$archiveName"

# Determine install directory
if (-not $installDir) {
  $installDir = Join-Path $env:LOCALAPPDATA "Programs\Badger"
}
New-Item -ItemType Directory -Force -Path $installDir | Out-Null

$targetPath = Join-Path $installDir $binaryName

# Download to temp dir and extract
Write-Host "Downloading AIBadger $version for windows/amd64..."
$tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null
try {
  $archivePath = Join-Path $tmpDir $archiveName
  Invoke-WebRequest -Uri $archiveUrl -OutFile $archivePath -UseBasicParsing
  Expand-Archive -Path $archivePath -DestinationPath $tmpDir -Force
  $extractedBinary = Join-Path $tmpDir $binaryName
  if (-not (Test-Path $extractedBinary)) {
    throw "Archive did not contain $binaryName"
  }
  Copy-Item -Path $extractedBinary -Destination $targetPath -Force
} finally {
  Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue
}

Write-Host ""
$normalizedInstallDir = $installDir.TrimEnd("\")
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$pathEntries = if ($userPath) {
  $userPath -split ";" | ForEach-Object { $_.TrimEnd("\") } | Where-Object { $_ -ne "" }
} else {
  @()
}

if ($pathEntries -notcontains $normalizedInstallDir) {
  try {
    $newPath = (@($normalizedInstallDir) + $pathEntries) -join ";"
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Host "Added $normalizedInstallDir to your User PATH."
  } catch {
    Write-Warning "Could not update your User PATH: $($_.Exception.Message)"
    Write-Host "Run this command to add it to your User PATH manually:"
    Write-Host "  [Environment]::SetEnvironmentVariable('Path', '$normalizedInstallDir;' + [Environment]::GetEnvironmentVariable('Path', 'User'), 'User')"
  }
}

$sessionPaths = $env:Path -split ";" | ForEach-Object { $_.TrimEnd("\") }
if ($sessionPaths -notcontains $normalizedInstallDir) {
  $env:Path = "$normalizedInstallDir;$env:Path"
}

Write-Host "Installed badger to:"
Write-Host "  $targetPath"
Write-Host "Restart already-open terminals (or open a new one) if 'badger' is not found elsewhere."

# Print version
& $targetPath --version
