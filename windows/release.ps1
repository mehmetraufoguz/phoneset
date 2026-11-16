# Builds the Windows camera and packs it as phoneset-camera-windows.zip (+ .sha256): the file `phoneset camera install`
# downloads from a release (pc/internal/camera, backend_windows.go). The zip holds softcam.dll (the camera), phoneset-camera.exe
# (the helper that carries the picture to it) and the licence texts; nothing else, and no sub folders.
#
#   windows\release.ps1 [-Out <folder>] [-Upload <tag>]
#
# Needs what windows\build.ps1 needs (Git, Visual Studio 2022 or its Build Tools). Nothing is signed: a DirectShow filter
# does not need a signature to be registered, and the program downloads the zip itself (so it carries no "downloaded from the
# internet" mark), but an antivirus may still look at an unsigned DLL. See docs/virtual-camera.md.
#
# -Upload TAG puts the zip on the existing GitHub release TAG (needs `gh auth login`) and adds its line to the release's
# SHA256SUMS, which the program checks the download against. No certificate or key goes to GitHub: build here, on your PC,
# after the tag's release exists (the same way macos\release.sh does it).
param(
    [string]$Out = (Join-Path $PSScriptRoot 'release'),
    [string]$Upload = ''
)
$ErrorActionPreference = 'Stop'
$zipName = 'phoneset-camera-windows.zip'

if ($Upload -ne '') {
    if (-not (Get-Command gh -ErrorAction SilentlyContinue)) { throw '-Upload needs the GitHub CLI (gh), signed in: gh auth login' }
    # Fail now, not after the build, if the release is not there.
    gh release view $Upload *> $null
    if ($LASTEXITCODE -ne 0) { throw "there is no GitHub release $Upload (push the tag and wait for the release job first)" }
}

& (Join-Path $PSScriptRoot 'build.ps1')
if ($LASTEXITCODE -ne 0 -and $null -ne $LASTEXITCODE) { throw 'windows\build.ps1 failed' }

$dist = Join-Path $PSScriptRoot 'dist'
$stage = Join-Path $Out 'stage'
Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $stage | Out-Null
foreach ($f in 'softcam.dll', 'phoneset-camera.exe') {
    $src = Join-Path $dist $f
    if (-not (Test-Path $src)) { throw "$src is missing: windows\build.ps1 did not make it" }
    Copy-Item $src $stage
}
# The DLL is softcam (MIT): its licence goes with it, with ours.
Copy-Item (Join-Path $PSScriptRoot 'softcam-LICENSE.txt') $stage
Copy-Item (Join-Path $PSScriptRoot '..\LICENSE') (Join-Path $stage 'phoneset-LICENSE.txt')

$zip = Join-Path $Out $zipName
Remove-Item $zip, "$zip.sha256" -ErrorAction SilentlyContinue
Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $zip
Remove-Item $stage -Recurse -Force

$hash = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
$noBom = New-Object Text.UTF8Encoding $false
[IO.File]::WriteAllText("$zip.sha256", "$hash  $zipName`n", $noBom)
Write-Host "done: $zip"
Write-Host "$hash  $zipName"
Add-Type -AssemblyName System.IO.Compression.FileSystem
$zf = [IO.Compression.ZipFile]::OpenRead($zip)
try { $zf.Entries | ForEach-Object { Write-Host ('  {0}  {1} bytes' -f $_.FullName, $_.Length) } } finally { $zf.Dispose() }
Write-Host 'NOT signed: see the comment at the top of this script.'

if ($Upload -ne '') {
    Write-Host "==> upload to release $Upload"
    $tmp = Join-Path $Out 'sums'
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Force $tmp | Out-Null
    gh release upload $Upload $zip --clobber
    if ($LASTEXITCODE -ne 0) { throw 'gh release upload failed' }
    # The release's SHA256SUMS gets this file's line (an older line of it goes), the rest stays as it is.
    gh release download $Upload --pattern SHA256SUMS --dir $tmp --clobber 2> $null
    $sumsPath = Join-Path $tmp 'SHA256SUMS'
    $lines = @()
    if (Test-Path $sumsPath) { $lines = @(Get-Content $sumsPath | Where-Object { $_ -ne '' -and $_ -notmatch "  $([regex]::Escape($zipName))$" }) }
    $lines += "$hash  $zipName"
    [IO.File]::WriteAllText($sumsPath, (($lines -join "`n") + "`n"), $noBom)
    gh release upload $Upload $sumsPath --clobber
    if ($LASTEXITCODE -ne 0) { throw 'gh release upload of SHA256SUMS failed' }
    Write-Host 'uploaded; SHA256SUMS now:'
    Get-Content $sumsPath
}
