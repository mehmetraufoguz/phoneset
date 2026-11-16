# Builds the Windows camera piece: softcam (MIT, github.com/tshino/softcam), a DirectShow virtual camera, made into
# "phoneset Camera" with a CLSID and shared memory names of its own, and with a slate of its own for when no sender
# is there (windows\filter).
#
#   windows\build.ps1
#
# Needs Git and Visual Studio 2022 (or its Build Tools) with the C++ desktop workload and a Windows 10/11 SDK.
# Output, all under windows\dist (ignored by git): softcam.dll (the camera), phoneset-camera.exe (the helper that carries the
# phone's picture to it, needs softcam.dll next to it) and sender.exe (softcam's example that sends a moving test picture,
# 1280 x 720 here so that it matches what the camera offers, for a first look).
$ErrorActionPreference = 'Stop'

# The softcam commit this was written against; windows\filter\softcam-slate.patch applies to exactly this one.
$Commit = 'd1acde4617782f87a3929d7a7528fa3e524ec56b'
$Clsid = @{ Text = '43750394-889D-43A4-807F-5FB98F8FCA5D'; Cpp = '0x43750394, 0x889d, 0x43a4, 0x80, 0x7f, 0x5f, 0xb9, 0x8f, 0x8f, 0xca, 0x5d' }

$dist = Join-Path $PSScriptRoot 'dist'
$src = Join-Path $dist 'softcam-src'
New-Item -ItemType Directory -Force $dist | Out-Null

if (-not (Test-Path (Join-Path $src '.git'))) {
    git clone https://github.com/tshino/softcam $src
    if ($LASTEXITCODE -ne 0) { throw 'git clone failed' }
}
# Plain LF files whatever the machine's autocrlf says: the patch has LF line endings.
git -C $src config core.autocrlf false
git -C $src fetch --quiet origin
git -C $src checkout --quiet --force $Commit
if ($LASTEXITCODE -ne 0) { throw "git checkout $Commit failed" }
# Write every file again from the index (a reset leaves files alone that git thinks are unchanged, even with the other
# line endings), then drop whatever else is there.
git -C $src read-tree $Commit
git -C $src checkout-index --all --force
git -C $src clean --quiet -fdx

# 1. The slate: new files, and the changes to the filter that use them.
Copy-Item (Join-Path $PSScriptRoot 'filter\Slate.h') (Join-Path $src 'src\softcamcore') -Force
Copy-Item (Join-Path $PSScriptRoot 'filter\Slate.cpp') (Join-Path $src 'src\softcamcore') -Force
git -C $src apply --whitespace=nowarn (Join-Path $PSScriptRoot 'filter\softcam-slate.patch')
if ($LASTEXITCODE -ne 0) { throw 'windows\filter\softcam-slate.patch does not apply: softcam changed?' }

# 2. The camera's name and identity (a string replace, it fails when the text is not there).
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

# 3. The example sender sends what the camera offers when no sender is there: 1280 x 720.
Edit 'examples\sender\sender.cpp' 'const int WIDTH = 320;' 'const int WIDTH = 1280;'
Edit 'examples\sender\sender.cpp' 'const int HEIGHT = 240;' 'const int HEIGHT = 720;'

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

# 4. The helper that carries the phone's picture to the camera (helper\): it uses softcam's sender library and header.
& $msbuild (Join-Path $PSScriptRoot 'helper\phoneset-camera.vcxproj') /p:Configuration=Release /p:Platform=x64 "/p:SoftcamDist=$src\dist\" /v:m /nologo
if ($LASTEXITCODE -ne 0) { throw 'building phoneset-camera.exe failed' }
Write-Host "Built $dist\softcam.dll, $dist\sender.exe and $dist\phoneset-camera.exe (it needs softcam.dll next to it)"
