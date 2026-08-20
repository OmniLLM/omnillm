$ErrorActionPreference = 'Stop'

$InstallDir = go env GOBIN
if (-not $InstallDir) {
    $GoPath = go env GOPATH
    $InstallDir = Join-Path ($GoPath -split ';' | Select-Object -First 1) 'bin'
}

foreach ($Name in @('omnillm.exe', 'omniproxy.exe')) {
    $Path = Join-Path $InstallDir $Name
    if (Test-Path -LiteralPath $Path -PathType Leaf) {
        Remove-Item -LiteralPath $Path -Force
        Write-Host "Removed $Path"
    }
}
