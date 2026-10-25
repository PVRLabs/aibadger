# Small binary-driven checks shared by Windows CI and installer validation.
[CmdletBinding()]
param([Parameter(Mandatory = $true)][string]$BinaryPath)
$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT') { throw 'Windows required' }
$exe = (Resolve-Path -LiteralPath $BinaryPath).Path
$root = Join-Path ([IO.Path]::GetTempPath()) ('Badger smoke ' + [guid]::NewGuid())
$oldLocation = Get-Location
$oldEncoding = [Console]::OutputEncoding
$failures = 0
function Invoke-Badger([string[]]$Arguments) {
    $result = & $exe @Arguments 2>&1
    if ($LASTEXITCODE -ne 0) { throw 'CLI failed' }
    return ($result -join "`n")
}
function Assert($Condition) { if (-not $Condition) { throw 'Assertion failed' } }
function Check($Name, [scriptblock]$Body) {
    try { & $Body; Write-Host "PASS $Name" }
    catch { $script:failures++; Write-Host "FAIL $Name" }
}
try {
    [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
    New-Item -ItemType Directory -Path (Join-Path $root 'project with spaces\nested folder') | Out-Null
    $utf8 = [System.Text.UTF8Encoding]::new($false)
    $unicode = 'Unicode ' + [char]0x00e9 + ' ' + [char]0x4e2d + ' ' + [char]0x03a9
    [IO.File]::WriteAllText((Join-Path $root 'goal with spaces.txt'), "Explain this fixture. $unicode", $utf8)
    [IO.File]::WriteAllText((Join-Path $root 'project with spaces\nested folder\hello.txt'), 'Disposable fixture.', $utf8)
    Set-Location $root
    Check 'startup/help' { Assert ((Invoke-Badger -Arguments @('--help')) -match 'Usage:') }
    Check 'version' {
        $v = Invoke-Badger -Arguments @('--version')
        Assert ($v -match '^badger \S+')
        Assert ((Invoke-Badger -Arguments @('version')).Trim() -ceq $v.Trim())
    }
    Check 'diagnose Windows' {
        $script:diagnosis = Invoke-Badger -Arguments @('diagnose')
        Assert ($script:diagnosis -match 'AI Badger Diagnose' -and $script:diagnosis -match 'windows')
    }
    foreach ($focus in @('code', 'design')) {
        Check "API $focus prompt with Unicode and Windows paths" {
            $project = if ($focus -eq 'code') { Join-Path $root 'project with spaces' } else { '.\project with spaces' }
            $goal = if ($focus -eq 'code') { Join-Path $root 'goal with spaces.txt' } else { '.\goal with spaces.txt' }
            $prompt = Invoke-Badger -Arguments @('api', 'prompt', '--root', $project, '--focus', $focus, '--input', $goal)
            Assert ($prompt.Length -gt 100 -and $prompt.Contains($unicode))
        }
    }
    foreach ($tool in @(@('mvn', 'Maven'), @('gradle', 'Gradle'), @('npm', 'npm'))) {
        $command = Get-Command ($tool[0] + '.cmd'), ($tool[0] + '.bat') -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
        if (-not $command) { Write-Host "SKIP $($tool[0]) batch launcher (not installed)"; continue }
        Check "$($tool[0]) batch launcher diagnose" {
            $status = [regex]::Match($script:diagnosis, ('(?m)^\s*' + [regex]::Escape($tool[1]) + ': (\d[0-9A-Za-z.+_-]*|unavailable|not found)\r?$'))
            if (-not $status.Success -or $status.Groups[1].Value -notmatch '^\d') {
                # Only emit the normalized public status, never raw tool output.
                $safeStatus = if ($status.Success) { $status.Groups[1].Value } else { 'unrecognized status' }
                Write-Host "INFO $($tool[0]) diagnose: $safeStatus"
                throw 'Expected numeric tool version'
            }
        }
    }
} finally {
    Set-Location $oldLocation
    [Console]::OutputEncoding = $oldEncoding
    Remove-Item -LiteralPath $root -Recurse -Force
}
if ($failures) { throw "$failures smoke check(s) failed" }
