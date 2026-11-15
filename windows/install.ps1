# Puts the camera DLL in place and registers it, so that "phoneset Camera" shows in the camera lists.
#
#   windows\install.ps1 [-Dll <path to softcam.dll>]
#
# Registering writes to HKLM, so this asks for administrator rights (the UAC window). The DLL is copied to
# "C:\Program Files\phoneset\camera": a place that every process may read. That matters: Chrome and Edge read the
# camera in a sandboxed process and leave the camera out of their list when the DLL sits in a user folder.
param([string]$Dll = (Join-Path $PSScriptRoot 'dist\softcam.dll'))
$ErrorActionPreference = 'Stop'
$log = Join-Path $env:TEMP 'phoneset-camera-install.log'

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host 'Asking for administrator rights: the camera is registered in the registry (HKLM).'
    Remove-Item $log -ErrorAction SilentlyContinue
    $p = Start-Process powershell.exe -Verb RunAs -Wait -PassThru -ArgumentList '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', "`"$PSCommandPath`"", '-Dll', "`"$Dll`""
    if ($p.ExitCode -ne 0 -and (Test-Path $log)) { Get-Content $log | Write-Host }
    exit $p.ExitCode
}

try {
    if (-not (Test-Path $Dll)) { throw "$Dll does not exist: run windows\build.ps1 first." }
    $dest = Join-Path $env:ProgramFiles 'phoneset\camera'
    New-Item -ItemType Directory -Force $dest | Out-Null
    Copy-Item $Dll (Join-Path $dest 'softcam.dll') -Force
    # regsvr32 is a Windows (not console) program: Start-Process is what waits for it and gives its exit code.
    $reg = Start-Process regsvr32.exe -ArgumentList '/s', "`"$(Join-Path $dest 'softcam.dll')`"" -Wait -PassThru
    if ($reg.ExitCode -ne 0) { throw "regsvr32 failed with $($reg.ExitCode)" }
    Write-Host "Registered $dest\softcam.dll. Restart a browser that was open to see the camera."
} catch {
    $_ | Out-String | Set-Content $log
    exit 1
}
exit 0
