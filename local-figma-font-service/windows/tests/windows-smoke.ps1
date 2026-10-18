# Run in Windows PowerShell 5.1 or later as the current, non-administrator user.
# Install requires Figma desktop's font agent running on localhost:44950.
# After a normal Windows sign-out/sign-in, run VerifyAfterLogin. Then Uninstall.
param(
  [ValidateSet('Install','VerifyAfterLogin','Uninstall','PortConflict')][string]$Phase = 'Install',
  [string]$Executable,
  [string]$OutputDirectory = (Join-Path $PSScriptRoot 'windows-smoke-results')
)
$ErrorActionPreference = 'Stop'
$runKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
$valueName = 'FigmaPDFExporterFontService'
$installed = Join-Path $env:LOCALAPPDATA 'FigmaPDFExporter\FontService\font-service.exe'
$statePath = Join-Path $OutputDirectory 'state.json'
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
$report = [ordered]@{ phase=$Phase; time=(Get-Date).ToUniversalTime().ToString('o'); os=[Environment]::OSVersion.ToString(); architecture=$env:PROCESSOR_ARCHITECTURE; tests=@() }
function Assert($condition, [string]$message) { if (-not $condition) { throw $message }; $script:report.tests += $message }
function Invoke-ServiceCommand([string]$path, [string]$argument) {
  # Start-Process -Wait waits for descendants too, including long-running
  # --serve. Wait on the CLI process itself so install can finish normally.
  $process=Start-Process -FilePath $path -ArgumentList @($argument) -PassThru
  try {
    $null=$process.Handle
    if (-not $process.WaitForExit(30000)) { throw 'service command did not finish within 30 seconds' }
    $process.Refresh()
    return $process.ExitCode
  } finally { $process.Dispose() }
}
function Get-CleanupArtifacts {
  $local=[Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
  $installationDirectory=Split-Path -Parent $installed
  if (Test-Path -LiteralPath $local) {
    Get-ChildItem -LiteralPath $local -Filter 'figma-font-cleanup-*' -Directory -Force | ForEach-Object { $_.FullName }
  }
  if (Test-Path -LiteralPath $installationDirectory) {
    Get-ChildItem -LiteralPath $installationDirectory -Filter '.font-service-remove-*.exe' -File -Force | ForEach-Object { $_.FullName }
  }
}
function Wait-NewCleanupArtifacts([string[]]$baseline) {
  $existing=@{}
  foreach ($path in $baseline) { $existing[$path]=$true }
  $deadline=[DateTime]::UtcNow.AddSeconds(20)
  do {
    $pending=@(Get-CleanupArtifacts | Where-Object { -not $existing.ContainsKey($_) })
    if ($pending.Count -eq 0) { return }
    Start-Sleep -Milliseconds 200
  } while ([DateTime]::UtcNow -lt $deadline)
  throw ('new uninstall cleanup outputs remain after 20 seconds: '+($pending -join ', '))
}
function Request([string]$url, [string]$origin) {
  $r = [System.Net.HttpWebRequest]::Create($url)
  $r.Proxy = $null; $r.AllowAutoRedirect = $false; $r.Timeout = 15000
  $r.Headers.Add('Origin',$origin)
  try { $response = $r.GetResponse() } catch [System.Net.WebException] { if ($_.Exception.Response) { $response=$_.Exception.Response } else { throw } }
  try {
    $stream=$response.GetResponseStream(); $buffer=New-Object System.IO.MemoryStream
    $stream.CopyTo($buffer)
    return @{ status=[int]$response.StatusCode; bytes=$buffer.ToArray(); contentType=$response.ContentType }
  } finally { $response.Close() }
}
function Health {
  $r=Request 'http://localhost:38474/health' 'null'
  Assert ($r.status -eq 200) 'health returns 200'
  $health=[Text.Encoding]::UTF8.GetString($r.bytes) | ConvertFrom-Json
  Assert ($health.service -eq 'figma-local-font-service' -and $health.fontRelay) 'health identifies font relay'
  Assert ($health.activationKind -eq 'login-autostart') 'health identifies Windows login autostart'
  return $health
}
function VerifyFonts {
  $catalog=Request 'http://localhost:38474/figma/font-files?freetype_minimum_api_version=20&isolate=true&' 'null'
  Assert ($catalog.status -eq 200) 'live Figma catalog returns 200'
  $data=[Text.Encoding]::UTF8.GetString($catalog.bytes) | ConvertFrom-Json
  $path=$data.fontFiles.PSObject.Properties.Name | Where-Object { $_ -match '\.(ttf|otf)$' } | Select-Object -First 1
  Assert ([bool]$path) 'live catalog advertises a TTF/OTF path'
  $encoded=[Uri]::EscapeDataString($path)
  $relay=Request ('http://localhost:38474/figma/font-file?file='+$encoded) 'null'
  $direct=Request ('http://localhost:44950/figma/font-file?file='+$encoded) 'https://www.figma.com'
  Assert ($relay.status -eq 200 -and $relay.bytes.Length -gt 0) 'relay returns nonempty binary font'
  $sha=[Security.Cryptography.SHA256]::Create()
  $relayHash=[BitConverter]::ToString($sha.ComputeHash($relay.bytes))
  $directHash=[BitConverter]::ToString($sha.ComputeHash($direct.bytes))
  Assert ($direct.status -eq 200 -and $relayHash -eq $directHash) 'relay font SHA256 equals original Figma font'
  $script:report.fontBytes=$relay.bytes.Length; $script:report.fontSHA256=$relayHash
  $denied=Request 'http://localhost:38474/figma/font-file?file=C%3A%5Cnot-advertised.ttf' 'null'
  Assert ($denied.status -eq 403) 'unadvertised filesystem path is rejected'
}
try {
  if ($Phase -eq 'Install') {
    Assert ([bool]$Executable -and (Test-Path -LiteralPath $Executable)) 'supplied executable exists'
    # Fail early without changing installation if the real upstream is absent.
    $probe=Request 'http://localhost:44950/figma/font-files?freetype_minimum_api_version=20&isolate=true&' 'https://www.figma.com'
    Assert ($probe.status -eq 200) 'real Figma desktop font agent is available'
    $source=Join-Path $OutputDirectory '한글 공백 소스'
    New-Item -ItemType Directory -Path $source -Force | Out-Null
    $copy=Join-Path $source 'font-service.exe'; Copy-Item -LiteralPath $Executable -Destination $copy -Force
    $exitCode=Invoke-ServiceCommand $copy '--install'
    Assert ($exitCode -eq 0) 'install from Unicode and space source path succeeds'
    Assert (Test-Path -LiteralPath $installed) 'per-user installed executable exists'
    $first=(Get-ItemProperty -LiteralPath $runKey).$valueName
    Assert ($first -eq ('"'+$installed+'" --serve')) 'HKCU login command is correctly quoted'
    $exitCode=Invoke-ServiceCommand $copy '--install'
    Assert ($exitCode -eq 0) 'repeated install succeeds'
    Assert ((Get-ItemProperty -LiteralPath $runKey).$valueName -eq $first) 'repeated install preserves login command'
    $health=Health
    VerifyFonts
    @{ sessionId=$health.sessionId; pid=$health.pid; runCommand=$first } | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding UTF8
    $report.nextStep='Sign out and sign in (or reboot), open Figma desktop, then run -Phase VerifyAfterLogin. This script does not claim login/reboot verification until that phase succeeds.'
  } elseif ($Phase -eq 'VerifyAfterLogin') {
    $state=Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
    $health=Health
    Assert ($health.sessionId -ne $state.sessionId) 'new login starts a fresh service session'
    Assert ((Get-ItemProperty -LiteralPath $runKey).$valueName -eq $state.runCommand) 'HKCU login command survives sign-in'
    $process=Get-Process -Id $health.pid
    Assert ($process.Path -eq $installed) 'autostart runs the installed per-user executable'
    VerifyFonts
  } elseif ($Phase -eq 'PortConflict') {
    Assert ([bool]$Executable -and (Test-Path -LiteralPath $Executable)) 'supplied executable exists'
    Assert (-not (Test-Path -LiteralPath $installed)) 'port-conflict test requires a clean uninstalled state'
    Assert ($null -eq (Get-ItemProperty -LiteralPath $runKey -Name $valueName -ErrorAction SilentlyContinue)) 'port-conflict test requires no own startup entry'
    $blocker=New-Object System.Net.Sockets.TcpListener([Net.IPAddress]::Parse('127.0.0.1'),38474)
    $blocker.Start()
    try {
      $exitCode=Invoke-ServiceCommand $Executable '--install'
      Assert ($exitCode -ne 0) 'install reports occupied local port'
      Assert (-not (Test-Path -LiteralPath $installed)) 'failed first install rolls back installed image'
      Assert ($null -eq (Get-ItemProperty -LiteralPath $runKey -Name $valueName -ErrorAction SilentlyContinue)) 'failed first install rolls back own startup entry'
      Assert $blocker.Server.IsBound 'failed install preserves unrelated port owner'
    } finally { $blocker.Stop() }
  } else {
    # A unique unrelated Run entry proves uninstall does not erase the Run key.
    $sentinel='FigmaPDFExporterSmoke-'+[Guid]::NewGuid().ToString('N')
    $sentinelValue='"C:\unrelated application\keep.exe"'
    New-ItemProperty -LiteralPath $runKey -Name $sentinel -Value $sentinelValue -PropertyType String | Out-Null
    try {
      $cleanupBaseline=@(Get-CleanupArtifacts)
      $exitCode=Invoke-ServiceCommand $installed '--uninstall'
      Assert ($exitCode -eq 0) 'uninstall succeeds'
      # Existing matching files/directories are excluded, never deleted or
      # required to disappear. This does not simulate a concurrent reinstall.
      Wait-NewCleanupArtifacts $cleanupBaseline
      Assert $true 'new self-uninstall helper directories and renamed images disappear within 20 seconds'
      Assert (-not (Test-Path -LiteralPath $installed)) 'uninstall removes the fixed installed image'
      Assert ($null -eq (Get-ItemProperty -LiteralPath $runKey -Name $valueName -ErrorAction SilentlyContinue)) 'own login entry is removed'
      Assert ((Get-ItemProperty -LiteralPath $runKey).$sentinel -eq $sentinelValue) 'unrelated login entry survives uninstall'
      $healthStopped=$false
      try { $r=Request 'http://localhost:38474/health' 'null' } catch { $healthStopped=$true }
      Assert $healthStopped 'uninstalled relay listener is stopped'
    } finally { Remove-ItemProperty -LiteralPath $runKey -Name $sentinel -ErrorAction SilentlyContinue }
  }
  $report.result='passed'
} catch { $report.result='failed'; $report.error=$_.Exception.Message; throw }
finally { $report | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $OutputDirectory ($Phase+'.json')) -Encoding UTF8 }
