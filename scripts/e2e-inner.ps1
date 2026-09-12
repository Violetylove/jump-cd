# Called by scripts/e2e.sh; JCD_BIN / JCD_DATA_DIR / WORK are already in the environment.
#
# This checks the one thing unit tests can never check: that the shell function
# really changes the current shell's directory.
#
# ASCII-only on purpose: Windows PowerShell 5.1 reads .ps1 files using the
# system code page, so non-ASCII text here would be mis-decoded.
$ErrorActionPreference = 'Stop'

# Load the integration with exactly the line the docs and the installer hand to
# users. If this needs a workaround to work, then the documented line is wrong
# and every user gets a broken profile - which is exactly what happened once.
Invoke-Expression (& { (jcd init powershell | Out-String) })

if (-not (Get-Command jcd -CommandType Function -ErrorAction SilentlyContinue)) {
    throw 'the integration did not define a jcd function'
}

$target = [IO.Path]::GetFullPath((Join-Path $env:WORK 'targets/alpha-project'))
$elsewhere = [IO.Path]::GetFullPath((Join-Path $env:WORK 'targets'))

function Get-NativePwd { (Get-Location).Path }

# Step 1: get into a directory that has never been visited, by path.
jcd $target
if ((Get-NativePwd) -ne $target) {
    throw "could not enter by path: $(Get-NativePwd)"
}

# In a non-interactive run the prompt function is never called, so trigger the
# hook by hand - same thing an interactive shell does before each prompt.
__jcd_hook

# Move elsewhere so the visit above lands in the history.
Set-Location $elsewhere
__jcd_hook

if (-not (Test-Path (Join-Path $env:JCD_DATA_DIR 'journal'))) {
    throw 'the hot path did not write to the journal'
}

# Step 2: jump back by name. This is the whole point.
jcd alpha
if ((Get-NativePwd) -ne $target) {
    throw "could not jump back by name: $(Get-NativePwd)"
}

# Subcommands must be forwarded to the binary, not treated as keywords.
jcd version | Out-Null

Write-Output 'ok powershell'
