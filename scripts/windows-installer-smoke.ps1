# Fresh-install checks only; running-executable upgrades and locked backups are not covered.
# Only for a disposable GitHub-hosted Windows runner: exercises persistent PATH.
[CmdletBinding()]
param([Parameter(Mandatory = $true)][ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+$')][string]$Version)
$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or $env:OS -ne 'Windows_NT') {
    throw 'Installer smoke requires a disposable GitHub-hosted Windows runner'
}
$root = Join-Path $env:RUNNER_TEMP ('Badger installer ' + [guid]::NewGuid())
$installDir = Join-Path $root 'installation with spaces'
$asset = 'badger_' + $Version.Substring(1) + '_windows_amd64.zip'
$expectedUrl = "https://github.com/PVRLabs/aibadger/releases/download/$Version/$asset"
$oldPath = $env:PATH
$oldUserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$names = @('BADGER_REPO', 'BADGER_VERSION', 'BADGER_INSTALL_DIR')
$saved = @{}
foreach ($name in $names) { $saved[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
try {
    New-Item -ItemType Directory -Path $root | Out-Null
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    $checksumPath = Join-Path $root 'checksum.txt'
    Microsoft.PowerShell.Utility\Invoke-WebRequest "$expectedUrl.sha256" -OutFile $checksumPath -UseBasicParsing
    $checksum = [IO.File]::ReadAllText($checksumPath).Trim()
    if ($checksum -notmatch ('\A([0-9a-fA-F]{64})\s+\*?' + [regex]::Escape($asset) + '\z')) {
        throw 'Published archive checksum unavailable or malformed'
    }
    $expectedHash = $Matches[1]
    $tempPrefix = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    # Intercept only the download boundary; execute the installer unchanged.
    # Verification finishes before control returns to its Expand-Archive call.
    $adapter = {
        param([string]$Uri, [string]$OutFile, [switch]$UseBasicParsing)
        if ($Uri -cne $expectedUrl -or [string]::IsNullOrWhiteSpace($OutFile)) { throw 'Unexpected download' }
        if (-not [IO.Path]::IsPathRooted($OutFile)) { throw 'Expected absolute destination' }
        $destination = [IO.Path]::GetFullPath($OutFile)
        if (-not $destination.StartsWith($tempPrefix, [StringComparison]::OrdinalIgnoreCase) -or
            [IO.Path]::GetFileName($destination) -cne $asset) { throw 'Unexpected download destination' }
        Microsoft.PowerShell.Utility\Invoke-WebRequest -Uri $Uri -OutFile $destination -UseBasicParsing
        if ((Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash -ne $expectedHash) {
            throw 'Published archive checksum mismatch'
        }
        Write-Host 'PASS published archive SHA-256 before extraction'
    }.GetNewClosure()
    Set-Item Function:Invoke-WebRequest -Value $adapter
    $env:BADGER_REPO = 'PVRLabs/aibadger'
    $env:BADGER_VERSION = $Version
    $env:BADGER_INSTALL_DIR = $installDir
    & (Join-Path $PSScriptRoot '..\install.ps1')
    $installed = Join-Path $installDir 'badger.exe'
    $v = & $installed --version
    if ($LASTEXITCODE -ne 0 -or ($v -join '') -notmatch ('^badger ' + [regex]::Escape($Version) + '$')) {
        throw 'Installed version mismatch'
    }
    if (([Environment]::GetEnvironmentVariable('Path', 'User') -split ';') -notcontains $installDir) {
        throw 'Persistent user PATH was not updated'
    }
    if ((Get-Command badger.exe -CommandType Application).Source -ne $installed) { throw 'Session PATH discovery failed' }
    Write-Host 'PASS isolated installation, version, persistent and session PATH'
    & (Join-Path $PSScriptRoot 'windows-smoke.ps1') -BinaryPath $installed
    # Share the installed binary with the subsequent PowerShell 7 smoke step.
    "BADGER_INSTALLED_BIN=$installed" | Out-File -FilePath $env:GITHUB_ENV -Encoding utf8 -Append
} finally {
    [Environment]::SetEnvironmentVariable('Path', $oldUserPath, 'User')
    $env:PATH = $oldPath
    foreach ($name in $names) { [Environment]::SetEnvironmentVariable($name, $saved[$name], 'Process') }
    # Keep the installation for the next shell; workflow cleanup removes it.
}
