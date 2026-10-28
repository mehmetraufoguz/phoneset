# Installs phoneset on Windows 10/11: downloads the latest release, puts phoneset.exe in
# %LOCALAPPDATA%\phoneset\bin and adds that folder to your PATH. No administrator rights needed.
#
#   irm https://raw.githubusercontent.com/mehmetraufoguz/phoneset/main/scripts/install-windows.ps1 | iex
#
# With options:
#
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/mehmetraufoguz/phoneset/main/scripts/install-windows.ps1))) -Start -Autostart
#
# Options:
#   -Start          start phoneset in the background now
#   -Autostart      start it in the background at every login
#   -WithVBCable    also try to install VB-CABLE (the virtual microphone) with winget
#   -Version vX.Y.Z install that release instead of the latest
#   -NoModifyPath   don't touch your PATH
#   -InstallDir     where to put phoneset.exe
#   -ArchivePath    install from a local .zip instead of downloading

function Install-Phoneset {
    [CmdletBinding()]
    param(
        [switch]$Start,
        [switch]$Autostart,
        [switch]$WithVBCable,
        [switch]$NoModifyPath,
        [string]$Version = "",
        [string]$InstallDir = (Join-Path $env:LOCALAPPDATA "phoneset\bin"),
        [string]$ArchivePath = ""
    )

    $ErrorActionPreference = "Stop"
    $ProgressPreference = "SilentlyContinue" # Invoke-WebRequest is very slow with the progress bar on 5.1
    try { [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 } catch { }

    $repo = "mehmetraufoguz/phoneset"

    # 1. Which build?
    $cpu = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    switch ($cpu) {
        "AMD64" { $arch = "amd64" }
        "ARM64" { $arch = "arm64" }
        default { throw "Unsupported processor: $cpu" }
    }
    $asset = "phoneset-windows-$arch.zip"

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("phoneset-" + [Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        # 2. Get the archive.
        $zip = Join-Path $tmp $asset
        if ($ArchivePath) {
            Write-Host "Using $ArchivePath"
            Copy-Item -LiteralPath $ArchivePath -Destination $zip
        }
        else {
            if ($Version) {
                $url = "https://github.com/$repo/releases/download/$Version/$asset"
                $label = $Version
            }
            else {
                $url = "https://github.com/$repo/releases/latest/download/$asset"
                $label = "(latest)"
            }
            Write-Host "Downloading phoneset $label for Windows $arch..."
            try {
                Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $zip
            }
            catch {
                # A private repository answers 404 to anonymous downloads; the GitHub CLI can sign in.
                $gh = Get-Command gh -ErrorAction SilentlyContinue
                if (-not $gh) { throw "Couldn't download $url ($($_.Exception.Message)). If the repository is private, install the GitHub CLI (winget install GitHub.cli), run 'gh auth login' and try again." }
                $ghArgs = @("release", "download")
                if ($Version) { $ghArgs += $Version }
                $ghArgs += @("--repo", $repo, "--pattern", $asset, "--dir", $tmp)
                & gh @ghArgs
                if ($LASTEXITCODE -ne 0) { throw "Couldn't download $asset with the GitHub CLI either." }
            }
        }

        # 3. Unpack.
        $extracted = Join-Path $tmp "unpacked"
        Expand-Archive -LiteralPath $zip -DestinationPath $extracted -Force
        $newExe = Get-ChildItem -Path $extracted -Filter "phoneset.exe" -Recurse | Select-Object -First 1
        if (-not $newExe) { throw "The archive doesn't contain phoneset.exe" }

        # 4. Stop a running copy so the file can be replaced, and bring it back afterwards.
        $exe = Join-Path $InstallDir "phoneset.exe"
        $wasRunning = $false
        if (Test-Path -LiteralPath $exe) {
            $status = & $exe status --json 2>$null | Out-String
            if ($status -match '"running":\s*true') {
                $wasRunning = $true
                Write-Host "Stopping the running phoneset..."
                & $exe stop | Out-Null
            }
        }

        # 5. Install.
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        Copy-Item -LiteralPath $newExe.FullName -Destination $exe -Force
        Unblock-File -LiteralPath $exe -ErrorAction SilentlyContinue
        Write-Host ("Installed " + (& $exe version) + " to $exe")

        # 6. PATH (this user only).
        if (-not $NoModifyPath) {
            $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
            $entries = @()
            if ($userPath) { $entries = $userPath.Split(";") | Where-Object { $_ } }
            if ($entries -notcontains $InstallDir) {
                $newPath = (@($entries) + $InstallDir) -join ";"
                [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
                Write-Host "Added $InstallDir to your PATH (open a new terminal window to pick it up)."
            }
            if (($env:Path.Split(";")) -notcontains $InstallDir) { $env:Path = "$env:Path;$InstallDir" }
        }
        else {
            Write-Host "Note: $InstallDir is not on your PATH."
        }

        # 7. Optional extras.
        if ($WithVBCable) {
            $winget = Get-Command winget -ErrorAction SilentlyContinue
            $done = $false
            if ($winget) {
                Write-Host "Installing VB-CABLE with winget (Windows will ask for permission)..."
                & winget install --id VB-Audio.VBCable --accept-package-agreements --accept-source-agreements
                $done = ($LASTEXITCODE -eq 0)
            }
            if (-not $done) {
                Write-Host "Get VB-CABLE from https://vb-audio.com/Cable/ (unzip, run VBCABLE_Setup_x64.exe as administrator, reboot)."
                Start-Process "https://vb-audio.com/Cable/"
            }
        }
        if ($Autostart) { & $exe autostart enable }
        if ($Start -or $wasRunning) { & $exe daemon }

        Write-Host ""
        Write-Host "Next:"
        if (-not ($Start -or $wasRunning)) {
            Write-Host "  phoneset daemon              run it in the background (tray icon, pairing page)"
        }
        Write-Host "  phoneset                     or run it in this terminal"
        Write-Host "  phoneset autostart enable    start it at every login"
        Write-Host "  Windows asks once to allow phoneset through the firewall: allow it on private networks."
        Write-Host "  The phone's microphone needs the VB-CABLE virtual device: https://vb-audio.com/Cable/"
    }
    finally {
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# `irm ... | iex` passes nothing; `& ([scriptblock]::Create(...)) -Start` and `.\install-windows.ps1 -Start`
# pass their switches through $args.
Install-Phoneset @args
