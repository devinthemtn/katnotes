# Sets up an installed katnotes for the current Windows user (no admin):
# a Start Menu shortcut for katnotes-gui.exe, the install folder on the user
# PATH (so katnotes.exe works in PowerShell and cmd), and optionally the WSL
# notes folder as notes_dir. Run by `task install-windows` from WSL.
param(
    [Parameter(Mandatory)][string]$Dir,
    # The WSL notes folder (\\wsl.localhost\<distro>\...). It becomes notes_dir
    # only while the Windows config still uses the default folder and that
    # folder has no notes, so a choice made on Windows is never overwritten.
    [string]$NotesDir
)

$ErrorActionPreference = 'Stop'
$gui = Join-Path $Dir 'katnotes-gui.exe'
$cli = Join-Path $Dir 'katnotes.exe'

$lnk = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\KatNotes.lnk'
$s = (New-Object -ComObject WScript.Shell).CreateShortcut($lnk)
$s.TargetPath = $gui
$s.WorkingDirectory = $env:USERPROFILE
$s.IconLocation = "$gui,0"
$s.Description = 'Markdown notes: pages and a daily journal'
$s.Save()
Write-Host "Start Menu: KatNotes -> $gui"

$path = [Environment]::GetEnvironmentVariable('Path', 'User')
$parts = @($path -split ';' | Where-Object { $_ })
if ($parts -notcontains $Dir) {
    [Environment]::SetEnvironmentVariable('Path', (($parts + $Dir) -join ';'), 'User')
    Write-Host "Added $Dir to your PATH (new terminals pick it up)"
}

if ($NotesDir) {
    $default = Join-Path $env:USERPROFILE 'KatNotes'
    $current = (& $cli config show --json | ConvertFrom-Json).notes_dir
    if ($current -eq $NotesDir) {
        Write-Host "Notes: $NotesDir"
    } elseif ($current -ne $default) {
        Write-Host "Notes: keeping notes_dir = $current (set in the Windows config)"
    } elseif ((Test-Path $default) -and (Get-ChildItem $default -Recurse -Filter *.md -ErrorAction SilentlyContinue | Select-Object -First 1)) {
        Write-Host "Notes: $default already has notes, so it stays; run 'katnotes config set notes_dir $NotesDir' to use the WSL notes"
    } else {
        & $cli config set notes_dir $NotesDir
        if ($LASTEXITCODE -ne 0) { throw "katnotes config set notes_dir failed" }
        Write-Host "Notes: $NotesDir (shared with WSL)"
    }
}
