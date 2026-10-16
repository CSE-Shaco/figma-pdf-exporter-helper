# Run on a stock Windows runner with Windows PowerShell 5.1.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$folder = Join-Path $env:LOCALAPPDATA 'Microsoft\Windows\Fonts'
[IO.Directory]::CreateDirectory($folder) | Out-Null
$fixture = Join-Path $folder 'codex-helper-fixture-extensionless'
$out = Join-Path $env:TEMP ([guid]::NewGuid().ToString() + '.out')
$err = Join-Path $env:TEMP ([guid]::NewGuid().ToString() + '.err')
function BE16([int]$v) { return [byte[]]@((($v -shr 8) -band 255), ($v -band 255)) }
function BE32([int]$v) { return [byte[]]@((($v -shr 24) -band 255), (($v -shr 16) -band 255), (($v -shr 8) -band 255), ($v -band 255)) }
$legacy = [Text.Encoding]::BigEndianUnicode.GetBytes('Legacy Fixture')
$typographic = [Text.Encoding]::BigEndianUnicode.GetBytes('Typographic Fixture')
$table = [byte[]]@(0, 0, 0, 2, 0, 30)
$table += (BE16 3) + (BE16 1) + (BE16 1033) + (BE16 1) + (BE16 $legacy.Length) + (BE16 0)
$table += (BE16 3) + (BE16 1) + (BE16 1033) + (BE16 16) + (BE16 $typographic.Length) + (BE16 $legacy.Length)
$table += $legacy + $typographic
$data = [byte[]]@(0, 1, 0, 0) + (BE16 1) + (BE16 0) + (BE16 0) + (BE16 0) + [Text.Encoding]::ASCII.GetBytes('name') + (BE32 0) + (BE32 28) + (BE32 $table.Length) + $table
[IO.File]::WriteAllBytes($fixture, [byte[]]$data)
$process = $null
try {
  $script = Join-Path $PSScriptRoot 'helper-windows.ps1'
  $process = Start-Process powershell.exe -ArgumentList @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', ('"' + $script + '"')) -PassThru -RedirectStandardOutput $out -RedirectStandardError $err -WindowStyle Hidden
  $token = $null
  for ($i = 0; $i -lt 60; $i++) {
    Start-Sleep -Milliseconds 500
    if ($process.HasExited) { throw "Helper exited: $(Get-Content $err -Raw)" }
    $text = if (Test-Path $out) { Get-Content $out -Raw } else { '' }
    if ($text -match '([0-9a-f]{64})') { $token = $Matches[1]; break }
  }
  if (-not $token) { throw 'Helper did not print a pairing token' }
  $url = 'http://127.0.0.1:38473'
  $headers = @{ Authorization = "Bearer $token" }
  $preflight = Invoke-WebRequest "$url/v1/match" -Method Options -Headers @{ 'Access-Control-Request-Private-Network' = 'true' } -UseBasicParsing
  if ($preflight.StatusCode -ne 204 -or $preflight.Headers['Access-Control-Allow-Private-Network'] -ne 'true') { throw 'CORS preflight failed' }
  $match = Invoke-WebRequest "$url/v1/match" -Method Post -Headers $headers -ContentType 'application/json' -Body '{"families":["Ｌｅｇａｃｙ Ｆｉｘｔｕｒｅ"]}' -UseBasicParsing
  $fonts = ($match.Content | ConvertFrom-Json).fonts
  $font = @($fonts | Where-Object { $_.name -eq 'codex-helper-fixture-extensionless' })[0]
  if (-not $font -or $font.family -ne 'Typographic Fixture' -or $font.families -notcontains 'Legacy Fixture' -or $font.PSObject.Properties.Name -contains 'path') { throw 'Family alias matching or response privacy failed' }
  $download = Join-Path $env:TEMP ([guid]::NewGuid().ToString() + '.ttf')
  try {
    Invoke-WebRequest "$url/v1/fonts/$($font.id)" -Headers $headers -OutFile $download -UseBasicParsing
    if (([IO.File]::ReadAllBytes($download)).Length -ne $data.Length) { throw 'Font bytes failed' }
  } finally { Remove-Item -LiteralPath $download -ErrorAction SilentlyContinue }
  try {
    Invoke-WebRequest "$url/v1/fonts/$($font.id)" -Headers @{ Authorization = 'Bearer wrong' } -UseBasicParsing | Out-Null
    throw 'Unauthorized download succeeded'
  } catch [System.Net.WebException] {
    if ($_.Exception.Response.StatusCode.value__ -ne 401) { throw }
  }
  [IO.File]::WriteAllBytes($fixture, ([byte[]]$data + [byte[]]@(1)))
  try {
    Invoke-WebRequest "$url/v1/fonts/$($font.id)" -Headers $headers -UseBasicParsing | Out-Null
    throw 'Changed font download succeeded'
  } catch [System.Net.WebException] {
    if ($_.Exception.Response.StatusCode.value__ -ne 404) { throw }
  }
  Write-Host 'Windows PowerShell helper integration passed.'
} finally {
  if ($process -and -not $process.HasExited) { $process.Kill(); $process.WaitForExit() }
  Remove-Item -LiteralPath $fixture, $out, $err -ErrorAction SilentlyContinue
}
