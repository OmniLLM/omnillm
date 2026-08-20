$hostTriple = rustc -vV 2>$null |
  ForEach-Object {
    if ($_ -match '^host: (.+)$') { $Matches[1] }
  } |
  Select-Object -First 1

if (-not $hostTriple) {
  Write-Error 'rustc not found; cannot build desktop sidecar'
  exit 1
}

$out = "desktop/src-tauri/binaries/omnillm-$hostTriple.exe"
Write-Host "Building $out"
& go build -o $out .
exit $LASTEXITCODE
