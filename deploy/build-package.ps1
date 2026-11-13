param(
    [ValidateSet("amd64", "arm64", "all")]
    [string]$Arch = "all"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root "dist"
$go = Join-Path $root ".toolchains\go\bin\go.exe"
if (-not (Test-Path -LiteralPath $go)) {
    $go = "go"
}

$arches = if ($Arch -eq "all") { @("amd64", "arm64") } else { @($Arch) }
New-Item -ItemType Directory -Force -Path $dist | Out-Null

foreach ($item in $arches) {
    $stage = Join-Path $dist "codexlink-relay-linux-$item"
    $resolvedStage = [System.IO.Path]::GetFullPath($stage)
    $resolvedDist = [System.IO.Path]::GetFullPath($dist)
    if (-not $resolvedStage.StartsWith($resolvedDist, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to clean a stage directory outside dist: $resolvedStage"
    }
    if (Test-Path -LiteralPath $resolvedStage) {
        Remove-Item -LiteralPath $resolvedStage -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $resolvedStage | Out-Null

    Write-Host "Building relay for linux/$item ..."
    $env:PATH = "$root\.toolchains\go\bin;$env:PATH"
    $env:GOOS = "linux"
    $env:GOARCH = $item
    $env:CGO_ENABLED = "0"
    & $go build -trimpath -ldflags="-s -w" -o (Join-Path $resolvedStage "codexlink-relay") ./cmd/codexlink-relay
    if ($LASTEXITCODE -ne 0) { throw "go build failed for $item" }
    Remove-Item Env:\GOOS, Env:\GOARCH, Env:\CGO_ENABLED -ErrorAction SilentlyContinue

    Copy-Item -LiteralPath (Join-Path $PSScriptRoot "install.sh") -Destination $resolvedStage
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot "Caddyfile.example") -Destination $resolvedStage
    @"
CodexLink relay package
=======================

1. Upload this archive to your Tencent Cloud Linux server:
   scp codexlink-relay-linux-$item.tar.gz root@YOUR_SERVER:/root/

2. Extract and install:
   tar -xzf codexlink-relay-linux-$item.tar.gz
   cd codexlink-relay-linux-$item
   sudo bash ./install.sh --http-public YOUR_SERVER_IP

   Or, with a filed domain and HTTPS:
   sudo bash ./install.sh --domain relay.example.com --email you@example.com

3. Copy the relay URL and desktop token printed by the installer into
   CodexLink Desktop -> Devices -> Cloud Relay.

The installer creates a hardened systemd service.
--http-public uses plaintext HTTP/WS on TCP 80. This is convenient for a
quick self-hosted test, but the desktop token and conversation traffic are
not encrypted. Use a filed domain with --domain for production HTTPS.
TCP 80 or 443 must be allowed in the Tencent Cloud security group.
"@ | Set-Content -LiteralPath (Join-Path $resolvedStage "README.txt") -Encoding utf8

    $archive = Join-Path $dist "codexlink-relay-linux-$item.tar.gz"
    if (Test-Path -LiteralPath $archive) {
        Remove-Item -LiteralPath $archive -Force
    }
    tar -czf $archive -C $dist "codexlink-relay-linux-$item"
    if ($LASTEXITCODE -ne 0) { throw "tar failed for $item" }
    $hash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash
    "$hash  codexlink-relay-linux-$item.tar.gz" | Set-Content -LiteralPath "$archive.sha256" -Encoding ascii
    Write-Host "Created $archive"
    Write-Host "SHA256 $hash"
}
