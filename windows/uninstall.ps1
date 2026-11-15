# Removes what windows\install.ps1 put on the PC: the registration of "phoneset Camera" and its folder.
#
#   windows\uninstall.ps1
#
# Asks for administrator rights (the registration is in HKLM).
$ErrorActionPreference = 'Stop'
$log = Join-Path $env:TEMP 'phoneset-camera-uninstall.log'

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host 'Asking for administrator rights: the camera is removed from the registry (HKLM).'
    Remove-Item $log -ErrorAction SilentlyContinue
    $p = Start-Process powershell.exe -Verb RunAs -Wait -PassThru -ArgumentList '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', "`"$PSCommandPath`""
    if ($p.ExitCode -ne 0 -and (Test-Path $log)) { Get-Content $log | Write-Host }
    exit $p.ExitCode
}

try {
    $dest = Join-Path $env:ProgramFiles 'phoneset\camera'
    $dll = Join-Path $dest 'softcam.dll'
    if (Test-Path $dll) {
        # regsvr32 is a Windows (not console) program: Start-Process is what waits for it and gives its exit code.
        $reg = Start-Process regsvr32.exe -ArgumentList '/u', '/s', "`"$dll`"" -Wait -PassThru
        if ($reg.ExitCode -ne 0) { throw "regsvr32 /u failed with $($reg.ExitCode)" }
        try {
            Remove-Item $dest -Recurse -Force
        } catch {
            # A browser's camera process keeps a DLL it has used loaded; the registration is gone already.
            $holders = (& tasklist.exe /m softcam.dll | Select-String 'softcam' | ForEach-Object { ($_ -split '\s+')[0] }) -join ', '
            throw "The camera is unregistered, but $dll is still loaded by: $holders. Close it (a browser that used the camera) and run uninstall.ps1 again."
        }
    }
    # The parent folder goes only when nothing else is in it.
    $parent = Split-Path $dest
    if ((Test-Path $parent) -and -not (Get-ChildItem $parent -Force)) { Remove-Item $parent -Force }
    Write-Host 'Removed the registration of phoneset Camera and its folder.'
} catch {
    $_ | Out-String | Set-Content $log
    exit 1
}
exit 0
