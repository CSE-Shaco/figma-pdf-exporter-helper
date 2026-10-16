param([ValidateRange(0, 65535)][int]$Port = 38473)
# Windows PowerShell 5.1 / .NET Framework only. No Python or administrator rights.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$MaxFont = 20000000
$MaxCollection = 64000000
$MaxBody = 32768

function U16([byte[]]$Data, [int]$At) {
  return (([int]$Data[$At] -shl 8) -bor [int]$Data[$At + 1])
}
function U32([byte[]]$Data, [int]$At) {
  return (([long]$Data[$At] -shl 24) -bor ([long]$Data[$At + 1] -shl 16) -bor ([long]$Data[$At + 2] -shl 8) -bor [long]$Data[$At + 3])
}
function Get-FontOffsets([byte[]]$Data) {
  if ($Data.Length -lt 12) { return @() }
  if ([Text.Encoding]::ASCII.GetString($Data, 0, 4) -ne 'ttcf') { return @(0) }
  $count = U32 $Data 8
  if ($count -le 0 -or $count -gt 256 -or (12 + $count * 4) -gt $Data.Length) { return @() }
  $offsets = [Collections.Generic.List[int]]::new()
  for ($i = 0; $i -lt $count; $i++) { $offsets.Add([int](U32 $Data (12 + $i * 4))) }
  return $offsets.ToArray()
}
function Get-FontNames([byte[]]$Data, [int]$Base = 0) {
  if ($Base -lt 0 -or ($Base + 12) -gt $Data.Length) { return $null }
  $tag = [Convert]::ToBase64String($Data, $Base, 4)
  if ($tag -notin @('AAEAAA==', 'T1RUTw==', 'dHJ1ZQ==')) { return $null }
  $count = U16 $Data ($Base + 4)
  if ($count -gt 512 -or ($Base + 12 + $count * 16) -gt $Data.Length) { return $null }
  for ($at = ($Base + 12); $at -lt ($Base + 12 + $count * 16); $at += 16) {
    if ([Text.Encoding]::ASCII.GetString($Data, $at, 4) -ne 'name') { continue }
    $offset = U32 $Data ($at + 8)
    $length = U32 $Data ($at + 12)
    if ($length -lt 6 -or ($offset + $length) -gt $Data.Length) { return $null }
    $records = U16 $Data ([int]$offset + 2)
    $strings = U16 $Data ([int]$offset + 4)
    if ($records -gt 4096 -or (6 + $records * 12) -gt $length) { return $null }
    $values = @{}
    for ($r = 0; $r -lt $records; $r++) {
      $pos = [int]$offset + 6 + $r * 12
      $platform = U16 $Data $pos
      $nameId = U16 $Data ($pos + 6)
      $size = U16 $Data ($pos + 8)
      $start = U16 $Data ($pos + 10)
      if ($nameId -notin @(1, 2, 4, 6, 16, 17, 21) -or ($strings + $start + $size) -gt $length) { continue }
      try {
        $encoding = if ($platform -in @(0, 3)) { [Text.Encoding]::BigEndianUnicode } else { [Text.Encoding]::GetEncoding(10000) }
        $value = $encoding.GetString($Data, ([int]$offset + $strings + $start), $size).Trim([char]0, [char]' ')
        if ($value -and $value.Length -lt 512) {
          if (-not $values.ContainsKey($nameId)) { $values[$nameId] = [Collections.Generic.List[string]]::new() }
          $values[$nameId].Add($value)
        }
      } catch { continue }
    }
    $families = [Collections.Generic.List[string]]::new()
    foreach ($id in @(16, 1, 21)) {
      if ($values.ContainsKey($id)) {
        foreach ($value in $values[$id]) { if (-not $families.Contains($value)) { $families.Add($value) } }
      }
    }
    if ($families.Count -eq 0) { return $null }
    foreach ($id in @(4, 6)) {
      if ($values.ContainsKey($id)) {
        foreach ($value in $values[$id]) { if (-not $families.Contains($value)) { $families.Add($value) } }
      }
    }
    $style = 'Regular'
    foreach ($id in @(17, 2)) { if ($values.ContainsKey($id) -and $values[$id].Count -gt 0) { $style = $values[$id][0]; break } }
    return [PSCustomObject]@{ family = $families[0]; families = @($families.ToArray()); style = $style }
  }
  return $null
}
function Normalize-Family([string]$Value) { return [regex]::Replace($Value.Normalize([Text.NormalizationForm]::FormKC).ToLowerInvariant(), '[^\p{L}\p{Nd}]', '') }
function Get-InstalledPaths {
  $system = Join-Path $env:WINDIR 'Fonts'
  $user = Join-Path $env:LOCALAPPDATA 'Microsoft\Windows\Fonts'
  $paths = [Collections.Generic.List[string]]::new()
  foreach ($entry in @(@('Registry::HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts', $system), @('Registry::HKEY_CURRENT_USER\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts', $user))) {
    try {
      $item = Get-ItemProperty -LiteralPath $entry[0] -ErrorAction Stop
      foreach ($property in $item.PSObject.Properties) {
        if ($property.Name -like 'PS*' -or $property.Value -isnot [string]) { continue }
        $value = [Environment]::ExpandEnvironmentVariables([string]$property.Value)
        $paths.Add($(if ([IO.Path]::IsPathRooted($value)) { $value } else { Join-Path $entry[1] $value }))
      }
    } catch { }
  }
  foreach ($root in @($system, $user)) {
    if (Test-Path -LiteralPath $root -PathType Container) {
      Get-ChildItem -LiteralPath $root -Recurse -File -ErrorAction SilentlyContinue | ForEach-Object { $paths.Add($_.FullName) }
    }
  }
  $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
  foreach ($path in $paths) {
    try {
      $full = [IO.Path]::GetFullPath($path)
      if (-not $seen.Add($full)) { continue }
      $item = Get-Item -LiteralPath $full -ErrorAction Stop
      if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { continue }
      # OS font registrations may use extensionless filenames.
      $source = [IO.File]::OpenRead($full)
      try {
        $header = [byte[]]::new(4)
        if ($source.Read($header, 0, 4) -ne 4) { continue }
        $signature = [BitConverter]::ToString($header)
        if ($signature -notin @('00-01-00-00', '4F-54-54-4F', '74-72-75-65', '74-74-63-66')) { continue }
        $limit = if ($signature -eq '74-74-63-66') { $MaxCollection } else { $MaxFont }
        if ($source.Length -gt $limit) { continue }
      } finally { $source.Dispose() }
      Write-Output $full
    } catch { continue }
  }
}
function Get-Sha([byte[]]$Data) {
  $sha = [Security.Cryptography.SHA256]::Create()
  try { return ([BitConverter]::ToString($sha.ComputeHash($Data))).Replace('-', '').ToLowerInvariant() }
  finally { $sha.Dispose() }
}
function Equal-Secret([string]$A, [string]$B) {
  if ($A.Length -ne $B.Length) { return $false }
  $difference = 0
  for ($i = 0; $i -lt $A.Length; $i++) { $difference = $difference -bor ([int][char]$A[$i] -bxor [int][char]$B[$i]) }
  return $difference -eq 0
}
function Read-Line([IO.Stream]$Stream, [int]$Limit) {
  $bytes = [Collections.Generic.List[byte]]::new()
  while ($bytes.Count -le $Limit) {
    $next = $Stream.ReadByte()
    if ($next -lt 0) { throw 'Connection closed' }
    if ($next -eq 10) {
      if ($bytes.Count -and $bytes[$bytes.Count - 1] -eq 13) { $bytes.RemoveAt($bytes.Count - 1) }
      return [Text.Encoding]::ASCII.GetString($bytes.ToArray())
    }
    $bytes.Add([byte]$next)
  }
  throw 'HTTP header too long'
}
function Read-Body([IO.Stream]$Stream, [int]$Length) {
  $body = [byte[]]::new($Length)
  $offset = 0
  while ($offset -lt $Length) {
    $read = $Stream.Read($body, $offset, $Length - $offset)
    if ($read -le 0) { throw 'Truncated request body' }
    $offset += $read
  }
  return ,$body
}
function Send-Response([IO.Stream]$Stream, [int]$Status, [byte[]]$Body, [string]$Mime = 'application/json') {
  $reason = switch ($Status) { 200 { 'OK' } 204 { 'No Content' } 400 { 'Bad Request' } 401 { 'Unauthorized' } 403 { 'Forbidden' } 404 { 'Not Found' } 413 { 'Payload Too Large' } default { 'Error' } }
  $head = "HTTP/1.1 $Status $reason`r`nAccess-Control-Allow-Origin: *`r`nAccess-Control-Allow-Methods: GET, POST, OPTIONS`r`nAccess-Control-Allow-Headers: Authorization, Content-Type`r`nAccess-Control-Allow-Private-Network: true`r`nCache-Control: no-store`r`nX-Content-Type-Options: nosniff`r`nContent-Type: $Mime`r`nContent-Length: $($Body.Length)`r`nConnection: close`r`n`r`n"
  $header = [Text.Encoding]::ASCII.GetBytes($head)
  $Stream.Write($header, 0, $header.Length)
  if ($Body.Length) { $Stream.Write($Body, 0, $Body.Length) }
  $Stream.Flush()
}
function Json-Bytes($Value) { return ,([Text.Encoding]::UTF8.GetBytes((ConvertTo-Json -InputObject $Value -Depth 6 -Compress))) }

[Console]::Out.WriteLine('설치된 TTF/OTF/TTC/OTC 글꼴을 검색합니다…')
$index = @{}
foreach ($path in Get-InstalledPaths) {
  try {
    $info = [IO.FileInfo]::new($path)
    if ($info.Length -le 0 -or $info.Length -gt $MaxCollection) { continue }
    $data = [IO.File]::ReadAllBytes($path)
    if ($data.Length -gt $MaxFont -and [Text.Encoding]::ASCII.GetString($data, 0, 4) -ne 'ttcf') { continue }
    $digest = Get-Sha $data
    $offsets = @(Get-FontOffsets $data)
    for ($faceIndex = 0; $faceIndex -lt $offsets.Count; $faceIndex++) {
      $names = Get-FontNames $data $offsets[$faceIndex]
      if ($null -eq $names) { continue }
      $id = Get-Sha ([Text.Encoding]::UTF8.GetBytes("${digest}:$faceIndex"))
      $member = if ([Text.Encoding]::ASCII.GetString($data, 0, 4) -eq 'ttcf') { $faceIndex } else { $null }
      $index[$id] = [PSCustomObject]@{ id = $id; name = $info.Name; family = $names.family; families = $names.families; style = $names.style; path = $path; digest = $digest; collectionIndex = $member }
    }
  } catch { continue }
}
$random = [byte[]]::new(32)
$rng = [Security.Cryptography.RandomNumberGenerator]::Create()
try { $rng.GetBytes($random) } finally { $rng.Dispose() }
$token = ([BitConverter]::ToString($random)).Replace('-', '').ToLowerInvariant()
$listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Parse('127.0.0.1'), $Port)
try { $listener.Start() } catch { throw '도우미를 시작할 수 없습니다. 포트 38473이 이미 사용 중인지 확인하세요.' }
$actualPort = ([Net.IPEndPoint]$listener.LocalEndpoint).Port
[Console]::Out.WriteLine("글꼴 $($index.Count)개 준비됨. 이 창을 열어 두세요. 종료: Ctrl+C")
[Console]::Out.WriteLine("플러그인 연결 코드: $token")
[Console]::Out.Flush()
try {
  while ($true) {
    $client = $listener.AcceptTcpClient()
    try {
      $client.ReceiveTimeout = 15000
      $client.SendTimeout = 15000
      $stream = $client.GetStream()
      $line = Read-Line $stream 8192
      $parts = $line.Split(' ')
      if ($parts.Length -ne 3 -or $parts[2] -notlike 'HTTP/1.*') { continue }
      $method = $parts[0]
      $path = $parts[1]
      $headers = @{}
      $total = 0
      while ($true) {
        $line = Read-Line $stream 8192
        $total += $line.Length
        if ($total -gt 32768) { throw 'Headers too long' }
        if ($line -eq '') { break }
        $colon = $line.IndexOf(':')
        if ($colon -lt 1) { throw 'Invalid header' }
        $headers[$line.Substring(0, $colon).Trim()] = $line.Substring($colon + 1).Trim()
      }
      if ($headers['Host'] -cne "localhost:$actualPort" -and $headers['Host'] -cne "127.0.0.1:$actualPort") { Send-Response $stream 403 ([Text.Encoding]::UTF8.GetBytes('{"error":"host"}')); continue }
      if ($method -eq 'OPTIONS') { Send-Response $stream 204 ([byte[]]::new(0)); continue }
      if (-not (Equal-Secret ([string]$headers['Authorization']) "Bearer $token")) { Send-Response $stream 401 ([Text.Encoding]::UTF8.GetBytes('{"error":"pairing"}')); continue }
      if ($method -eq 'POST' -and $path -ceq '/v1/match') {
        $length = 0
        if (-not [int]::TryParse([string]$headers['Content-Length'], [ref]$length) -or $length -le 0 -or $length -gt $MaxBody) { Send-Response $stream 413 ([byte[]]::new(0)); continue }
        try {
          $payload = [Text.Encoding]::UTF8.GetString((Read-Body $stream $length)) | ConvertFrom-Json
          if ($payload.families -isnot [Array] -or $payload.families.Count -gt 200) { throw 'Invalid families' }
          $requested = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
          foreach ($family in $payload.families) {
            if ($family -isnot [string] -or $family.Length -gt 256) { throw 'Invalid family' }
            [void]$requested.Add((Normalize-Family $family))
          }
          $matches = [Collections.Generic.List[object]]::new()
          foreach ($font in $index.Values) {
            foreach ($family in $font.families) {
              if ($requested.Contains((Normalize-Family $family))) {
                $matches.Add([PSCustomObject]@{ id = $font.id; name = $font.name; family = $font.family; families = @($font.families); style = $font.style; collectionIndex = $font.collectionIndex })
                break
              }
            }
          }
          Send-Response $stream 200 (Json-Bytes ([PSCustomObject]@{ fonts = @($matches.ToArray()) }))
        } catch { Send-Response $stream 400 ([Text.Encoding]::UTF8.GetBytes('{"error":"request"}')) }
        continue
      }
      if ($method -eq 'GET' -and $path -cmatch '^/v1/fonts/([0-9a-fA-F]{64})$') {
        $id = $Matches[1].ToLowerInvariant()
        if (-not $index.ContainsKey($id)) { Send-Response $stream 404 ([byte[]]::new(0)); continue }
        try {
          $font = $index[$id]
          $info = [IO.FileInfo]::new($font.path)
          if ($info.Length -le 0 -or $info.Length -gt $MaxCollection) { throw 'Size changed' }
          $data = [IO.File]::ReadAllBytes($font.path)
          if ((Get-Sha $data) -cne $font.digest) { throw 'File changed' }
          Send-Response $stream 200 $data 'application/octet-stream'
        } catch { Send-Response $stream 404 ([byte[]]::new(0)) }
        continue
      }
      Send-Response $stream 404 ([byte[]]::new(0))
    } catch { try { Send-Response $stream 400 ([byte[]]::new(0)) } catch { } }
    finally { $client.Close() }
  }
} finally { $listener.Stop() }
