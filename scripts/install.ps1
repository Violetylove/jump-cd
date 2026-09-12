# jump-cd installer for Windows.
#
#   irm https://raw.githubusercontent.com/Violetylove/jump-cd/main/scripts/install.ps1 | iex
#
# What it does:
#   1. works out your architecture
#   2. downloads the matching release zip and verifies its checksum
#   3. installs jcd.exe into %LOCALAPPDATA%\Programs\jump-cd
#   4. adds that directory to your user PATH, once
#   5. appends the shell integration line to your PowerShell profile, once
#
# Overridable through the environment:
#   JCD_VERSION=<x.y.z>     install a specific version instead of the latest
#   JCD_INSTALL_DIR=<dir>   install somewhere other than %LOCALAPPDATA%
#   JCD_BASE_URL=<url>      download from a mirror instead of GitHub Releases
#   JCD_NO_SHELL_SETUP=1    download and install only, do not touch your profile
#
# There is deliberately no param() block: the documented way to run this is
# "irm ... | iex", and a param block is unreliable in that form. Use the
# environment variables above instead.
#
# ASCII-only on purpose: Windows PowerShell 5.1 reads .ps1 files using the
# system code page, so non-ASCII text here would be mis-decoded.

$ErrorActionPreference = 'Stop'
# Invoke-WebRequest draws a progress bar and becomes absurdly slow because of it.
$ProgressPreference = 'SilentlyContinue'

# PowerShell 5.1 on an older .NET defaults to TLS 1.0, which GitHub rejects.
if ($PSVersionTable.PSVersion.Major -lt 6) {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
}

$Repo    = 'Violetylove/jump-cd'
$App     = 'jump-cd'
$Exe     = 'jcd.exe'
$Version = $env:JCD_VERSION
$BaseUrl = $env:JCD_BASE_URL
$NoSetup = ($env:JCD_NO_SHELL_SETUP -eq '1')

function Step($Message) { Write-Host ''; Write-Host $Message }
function Info($Message) { Write-Host "  $Message" }
function Die($Message)  { Write-Host "error: $Message" -ForegroundColor Red; exit 1 }

# ---------------------------------------------------------------- platform

Step 'Detecting the platform'

# PROCESSOR_ARCHITECTURE is x86 when 32-bit PowerShell runs on 64-bit Windows;
# the real architecture is in PROCESSOR_ARCHITEW6432 in that case.
$arch = $env:PROCESSOR_ARCHITECTURE
if ($env:PROCESSOR_ARCHITEW6432) { $arch = $env:PROCESSOR_ARCHITEW6432 }

switch ($arch) {
    'AMD64' { $goarch = 'amd64' }
    'ARM64' { $goarch = 'arm64' }
    default { Die "unsupported architecture: $arch" }
}
Info "arch: $goarch"

# ---------------------------------------------------------------- version

Step 'Resolving the version'

if (-not $Version) {
    try {
        $latest  = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
        $Version = $latest.tag_name -replace '^v', ''
    } catch {
        Die "could not work out the latest version: $($_.Exception.Message)"
    }
}
if (-not $Version) { Die 'could not work out the latest version; set JCD_VERSION' }
Info "version: $Version"

if (-not $BaseUrl) {
    $BaseUrl = "https://github.com/$Repo/releases/download/v$Version"
}

# Built with -f rather than string interpolation so that no backtick escape is
# needed to separate the variable from the underscore that follows it.
$archive = '{0}_{1}_windows_{2}.zip' -f $App, $Version, $goarch
Info $archive

$tmp = Join-Path ([IO.Path]::GetTempPath()) ('jcd-install-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmp -Force | Out-Null

try {
    # ------------------------------------------------------------ download

    Step 'Downloading'
    Info "$BaseUrl/$archive"

    $zipPath = Join-Path $tmp $archive
    try {
        Invoke-WebRequest -Uri "$BaseUrl/$archive" -OutFile $zipPath -UseBasicParsing
    } catch {
        Die "download failed: $($_.Exception.Message)"
    }

    # A missing checksums.txt is a warning, not an error, so that a mirror can
    # drop it if it has to.
    $sumPath = Join-Path $tmp 'checksums.txt'
    try {
        Invoke-WebRequest -Uri "$BaseUrl/checksums.txt" -OutFile $sumPath -UseBasicParsing

        $want = $null
        foreach ($line in (Get-Content -Path $sumPath)) {
            $parts = $line -split '\s+'
            if ($parts.Count -ge 2 -and $parts[1] -eq $archive) { $want = $parts[0]; break }
        }

        if (-not $want) {
            Write-Warning "no checksum listed for $archive, skipping verification"
        } else {
            $got = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash
            if ($got -ne $want.ToUpper()) { Die "checksum mismatch for $archive" }
            Info 'checksum verified'
        }
    } catch {
        Write-Warning 'checksums.txt not available, skipping verification'
    }

    # ------------------------------------------------------------ install

    Step 'Installing'

    $dir = $env:JCD_INSTALL_DIR
    if (-not $dir) { $dir = Join-Path $env:LOCALAPPDATA 'Programs\jump-cd' }
    New-Item -ItemType Directory -Path $dir -Force | Out-Null

    Expand-Archive -Path $zipPath -DestinationPath $tmp -Force
    $source = Join-Path $tmp $Exe
    if (-not (Test-Path -Path $source)) { Die "the archive did not contain $Exe" }

    $target = Join-Path $dir $Exe
    Copy-Item -Path $source -Destination $target -Force
    Info $target

    # ------------------------------------------------------------ PATH

    Step 'Checking PATH'

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not $userPath) { $userPath = '' }

    $needPath = $false
    if (($userPath -split ';') -notcontains $dir) {
        $needPath = $true
        Info "$dir is not on PATH yet"
    } else {
        Info "$dir is already on PATH"
    }

    # Also make it work in this session, without waiting for a new window.
    $env:Path = "$dir;$env:Path"

    # ------------------------------------------------------------ profile

    if ($NoSetup) {
        Step 'Skipping shell setup (JCD_NO_SHELL_SETUP=1)'
    } else {
        Step 'Setting up PowerShell'

        if ($needPath) {
            [Environment]::SetEnvironmentVariable('Path', ($userPath.TrimEnd(';') + ';' + $dir), 'User')
            Info "added $dir to your user PATH"
        }

        $profilePath = $PROFILE
        $profileDir  = Split-Path -Parent $profilePath
        if ($profileDir -and -not (Test-Path -Path $profileDir)) {
            New-Item -ItemType Directory -Path $profileDir -Force | Out-Null
        }

        # v0.1.0 wrote a line that could not work: Invoke-Expression was handed a
        # string array instead of a string. Repair it in place, so that simply
        # re-running this installer fixes an affected profile.
        $brokenLine = 'Invoke-Expression (&jcd init powershell)'
        $goodLine   = 'Invoke-Expression (& { (jcd init powershell | Out-String) })'

        $profileText = ''
        if (Test-Path -Path $profilePath) {
            $profileText = Get-Content -Path $profilePath -Raw
            if (-not $profileText) { $profileText = '' }
        }

        if ($profileText.Contains($brokenLine)) {
            Set-Content -Path $profilePath -Value $profileText.Replace($brokenLine, $goodLine) -NoNewline
            Info "repaired the broken integration line in $profilePath"
        } elseif ($profileText.Contains('jcd init')) {
            Info "$profilePath already contains the integration, leaving it alone"
        } else {
            Add-Content -Path $profilePath -Value '', '# jump-cd', $goodLine
            Info "appended to $profilePath"
        }
    }

    # ------------------------------------------------------------ done

    Step 'Done'
    Info "jcd $Version is installed"

    Write-Host ''
    Write-Host 'Open a new PowerShell window, or run this in the current one:'
    Write-Host ''
    Write-Host '    Invoke-Expression (& { (jcd init powershell | Out-String) })'
    Write-Host ''
    Write-Host 'Then check everything with:'
    Write-Host ''
    Write-Host '    jcd doctor'
    Write-Host ''
} finally {
    Remove-Item -Path $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
