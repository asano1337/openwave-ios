$ErrorActionPreference = "Stop"
$Publisher = "Lo5ev Corp"
$AppName = "OpenWave"
$InstallDir = Join-Path $env:LOCALAPPDATA "Lo5ev\OpenWave"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path

$exe = Get-ChildItem -Path $Root -Filter "*.exe" -File |
  Where-Object { $_.Name -notmatch "Install|Setup" } |
  Select-Object -First 1
if (-not $exe) {
  throw "OpenWave EXE не найден рядом с установщиком."
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item $exe.FullName (Join-Path $InstallDir "OpenWave.exe") -Force
if (Test-Path (Join-Path $Root "icon.ico")) {
  Copy-Item (Join-Path $Root "icon.ico") (Join-Path $InstallDir "OpenWave.ico") -Force
}

$shell = New-Object -ComObject WScript.Shell
$desktop = Join-Path ([Environment]::GetFolderPath("Desktop")) "OpenWave.lnk"
$start = Join-Path ([Environment]::GetFolderPath("StartMenu")) "OpenWave.lnk"
foreach ($shortcutPath in @($desktop, $start)) {
  $shortcut = $shell.CreateShortcut($shortcutPath)
  $shortcut.TargetPath = Join-Path $InstallDir "OpenWave.exe"
  $shortcut.WorkingDirectory = $InstallDir
  $shortcut.Description = "$AppName — $Publisher"
  $shortcut.WindowStyle = 1
  $icon = Join-Path $InstallDir "OpenWave.ico"
  if (Test-Path $icon) { $shortcut.IconLocation = "$icon,0" }
  $shortcut.Save()
}

Start-Process (Join-Path $InstallDir "OpenWave.exe")
Write-Host "$AppName установлен в $InstallDir ($Publisher)."