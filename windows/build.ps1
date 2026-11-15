# Builds the Windows camera piece: softcam (MIT, github.com/tshino/softcam), a DirectShow virtual camera, made into
# "phoneset Camera" with a CLSID and shared memory names of its own.
#
#   windows\build.ps1
#
# Needs Git and Visual Studio 2022 (or its Build Tools) with the C++ desktop workload and a Windows 10/11 SDK.
# Output, all under windows\dist (ignored by git): softcam.dll (the camera) and sender.exe (softcam's example that
# sends a moving test picture, for a first look).
$ErrorActionPreference = 'Stop'

# The softcam commit this was written against; the name and the CLSID below are replaced in a fresh checkout.
$Commit = 'd1acde4617782f87a3929d7a7528fa3e524ec56b'
$Clsid = @{ Text = '43750394-889D-43A4-807F-5FB98F8FCA5D'; Cpp = '0x43750394, 0x889d, 0x43a4, 0x80, 0x7f, 0x5f, 0xb9, 0x8f, 0x8f, 0xca, 0x5d' }

$dist = Join-Path $PSScriptRoot 'dist'
$src = Join-Path $dist 'softcam-src'
New-Item -ItemType Directory -Force $dist | Out-Null

if (-not (Test-Path (Join-Path $src '.git'))) {
    git clone https://github.com/tshino/softcam $src
    if ($LASTEXITCODE -ne 0) { throw 'git clone failed' }
}
git -C $src fetch --quiet origin
git -C $src checkout --quiet --force $Commit
if ($LASTEXITCODE -ne 0) { throw "git checkout $Commit failed" }
git -C $src clean --quiet -fdx

# Replace the camera's name and identity (a bytes-for-bytes replace keeps the line endings of the files).
function Edit([string]$File, [string]$From, [string]$To) {
    $path = Join-Path $src $File
    $text = [IO.File]::ReadAllText($path)
    if (-not $text.Contains($From)) { throw "$File does not contain '$From': softcam changed?" }
    [IO.File]::WriteAllText($path, $text.Replace($From, $To))
}
Edit 'src\softcam\softcam.cpp' '// {AEF3B972-5FA5-4647-9571-358EB472BC9E}' "// {$($Clsid.Text)}"
Edit 'src\softcam\softcam.cpp' '0xaef3b972, 0x5fa5, 0x4647, 0x95, 0x71, 0x35, 0x8e, 0xb4, 0x72, 0xbc, 0x9e' $Clsid.Cpp
Edit 'src\softcam\softcam.cpp' 'L"DirectShow Softcam"' 'L"phoneset Camera"'
Edit 'src\softcamcore\DShowSoftcam.cpp' 'NAME("DirectShow Softcam")' 'NAME("phoneset Camera")'
Edit 'src\softcamcore\FrameBuffer.cpp' '"DirectShow Softcam/' '"phoneset Camera/'

$vswhere = Join-Path ${env:ProgramFiles(x86)} 'Microsoft Visual Studio\Installer\vswhere.exe'
if (-not (Test-Path $vswhere)) { throw 'Visual Studio is not installed (vswhere.exe is missing).' }
$msbuild = & $vswhere -latest -products * -requires Microsoft.Component.MSBuild -find 'MSBuild\**\Bin\MSBuild.exe' | Select-Object -First 1
if (-not $msbuild) { throw 'MSBuild was not found.' }

# Only the two projects we need: the solution also has tests that want NuGet packages. A project built on its own
# needs SolutionDir to be told, its post-build step copies the header and the library there.
$solutionDir = "$src\"
& $msbuild (Join-Path $src 'src\softcam\softcam.vcxproj') /p:Configuration=Release /p:Platform=x64 "/p:SolutionDir=$solutionDir" /v:m /nologo
if ($LASTEXITCODE -ne 0) { throw 'building softcam.dll failed' }
& $msbuild (Join-Path $src 'examples\sender\sender.vcxproj') /p:Configuration=Release /p:Platform=x64 "/p:SolutionDir=$solutionDir" /v:m /nologo
if ($LASTEXITCODE -ne 0) { throw 'building sender.exe failed' }

Copy-Item (Join-Path $src 'dist\bin\x64\softcam.dll') $dist -Force
Copy-Item (Join-Path $src 'x64\Release\sender.exe') $dist -Force
Write-Host "Built $dist\softcam.dll and $dist\sender.exe"
