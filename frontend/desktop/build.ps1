# Baut S-Check Desktop unter Windows:
#   build\s-check.exe          Programm
#   build\s-check-setup.exe    Installation für den Benutzer (NSIS)
#   build\s-check.msi          Installation für alle Benutzer, Softwareverteilung (WiX Toolset 3)
#
# Benötigt: Go 1.24, Node 20, NSIS 3 (makensis), WiX Toolset 3.14 (candle, light).
# Fehlt NSIS oder WiX, wird der jeweilige Installer übersprungen.
# Signieren (optional): $env:SIGN_PFX = Pfad zur .pfx-Datei, $env:SIGN_PASS = Kennwort (signtool aus dem Windows SDK).
#
#   powershell -ExecutionPolicy Bypass -File build.ps1

$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$version = if ($env:VERSION) { $env:VERSION } else { (Get-Content VERSION -Raw).Trim() }
$out = Join-Path $PSScriptRoot 'build'
if (Test-Path $out) { Remove-Item $out -Recurse -Force }
New-Item -ItemType Directory $out | Out-Null

function Find-Tool($name, $candidates) {
  $cmd = Get-Command $name -ErrorAction SilentlyContinue
  if ($cmd) { return $cmd.Source }
  foreach ($c in $candidates) { if (Test-Path $c) { return $c } }
  return $null
}

function Sign($file) {
  if (-not $env:SIGN_PFX) { return }
  $signtool = Find-Tool 'signtool.exe' (Get-ChildItem "${env:ProgramFiles(x86)}\Windows Kits\10\bin\*\x64\signtool.exe" -ErrorAction SilentlyContinue | Sort-Object FullName -Descending | ForEach-Object FullName)
  if (-not $signtool) { throw 'signtool.exe nicht gefunden (Windows SDK)' }
  & $signtool sign /f $env:SIGN_PFX /p $env:SIGN_PASS /fd sha256 /tr http://timestamp.digicert.com /td sha256 /d 'S-Check' $file
  if ($LASTEXITCODE) { throw "Signieren fehlgeschlagen: $file" }
}

Write-Host '== Oberfläche'
Push-Location ..
if (-not (Test-Path node_modules)) { npm ci --no-audit --no-fund; if ($LASTEXITCODE) { throw 'npm ci' } }
npm run build | Out-Null
if ($LASTEXITCODE) { throw 'npm run build' }
Pop-Location
if (Test-Path web) { Remove-Item web -Recurse -Force }
Copy-Item ..\dist web -Recurse

Write-Host '== Programm'
# rsrc_windows_amd64.syso (Symbol, Versionsangaben, Manifest) liegt im Repository; build.sh erzeugt sie neu.
$env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
go build -trimpath -ldflags "-s -w -H windowsgui -X main.version=$version" -o "$out\s-check.exe" .
if ($LASTEXITCODE) { throw 'go build' }
Remove-Item Env:GOOS, Env:GOARCH
Copy-Item winres\s-check.ico $out
Sign "$out\s-check.exe"

$makensis = Find-Tool 'makensis.exe' @("${env:ProgramFiles(x86)}\NSIS\makensis.exe", "$env:ProgramFiles\NSIS\makensis.exe")
if ($makensis) {
  Write-Host '== Installation (NSIS)'
  & $makensis -V2 "-DVERSION=$version" "-DSRC=$out" "-DOUT=$out\s-check-setup.exe" installer\s-check.nsi
  if ($LASTEXITCODE) { throw 'makensis' }
  Sign "$out\s-check-setup.exe"
} else { Write-Warning 'NSIS nicht gefunden: s-check-setup.exe wird nicht gebaut' }

$candle = Find-Tool 'candle.exe' @("${env:ProgramFiles(x86)}\WiX Toolset v3.14\bin\candle.exe", "${env:ProgramFiles(x86)}\WiX Toolset v3.11\bin\candle.exe")
if ($candle) {
  Write-Host '== Installation (MSI)'
  $light = Join-Path (Split-Path $candle) 'light.exe'
  & $candle -nologo -arch x64 "-dVersion=$version" "-dSrc=$out" -out "$out\s-check.wixobj" installer\s-check.wxs
  if ($LASTEXITCODE) { throw 'candle' }
  & $light -nologo -sval -cultures:de-DE -out "$out\s-check.msi" "$out\s-check.wixobj"
  if ($LASTEXITCODE) { throw 'light' }
  Remove-Item "$out\s-check.wixobj", "$out\s-check.wixpdb" -ErrorAction SilentlyContinue
  Sign "$out\s-check.msi"
} else { Write-Warning 'WiX Toolset 3 nicht gefunden: s-check.msi wird nicht gebaut' }

Remove-Item "$out\s-check.ico"
Get-ChildItem $out -Filter 's-check*' | ForEach-Object { "{0}  {1}" -f (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower(), $_.Name } | Set-Content "$out\SHA256SUMS.txt"
Get-ChildItem $out
