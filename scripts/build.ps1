# Copyright (c) 2026 Paulo Vidal (https://x.com/inevitable360, https://github.com/qlpqlp)
#
# SPDX-License-Identifier: MIT

# Compile DogeVM Connect with the website embedded.
# Usage (from anywhere):
#   powershell -ExecutionPolicy Bypass -File scripts\build.ps1
#
# Optional:
#   .\scripts\build.ps1 -OutName "DogeVM-Connect.exe"
#   .\scripts\build.ps1 -All   # also linux/darwin amd64 + arm64

param(
    [string]$OutName = "dogevm-connect.exe",
    [switch]$All
)

$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go is not on PATH. Install Go, then retry."
}

New-Item -ItemType Directory -Force -Path (Join-Path $Root "dist") | Out-Null

Get-Process -Name "dogevm-connect", "DogeVM-Connect" -ErrorAction SilentlyContinue |
    Stop-Process -Force -ErrorAction SilentlyContinue

$env:CGO_ENABLED = "0"

$Out = Join-Path $Root "dist\$OutName"
Write-Host "Building $Out (website embedded)..."
go build -trimpath -ldflags "-s -w" -o $Out ./cmd/dogevm-connect
if ($LASTEXITCODE -ne 0) {
    throw "go build failed ($LASTEXITCODE)"
}

$item = Get-Item $Out
Write-Host ("OK  {0}  ({1:N1} MB)" -f $item.FullName, ($item.Length / 1MB))

if ($All) {
    $targets = @(
        @{ GOOS = "linux"; GOARCH = "amd64"; Name = "dogevm-connect-linux-amd64" },
        @{ GOOS = "linux"; GOARCH = "arm64"; Name = "dogevm-connect-linux-arm64" },
        @{ GOOS = "darwin"; GOARCH = "amd64"; Name = "dogevm-connect-darwin-amd64" },
        @{ GOOS = "darwin"; GOARCH = "arm64"; Name = "dogevm-connect-darwin-arm64" }
    )
    foreach ($t in $targets) {
        $env:GOOS = $t.GOOS
        $env:GOARCH = $t.GOARCH
        $path = Join-Path $Root ("dist\{0}" -f $t.Name)
        Write-Host "Building $path ..."
        go build -trimpath -ldflags "-s -w" -o $path ./cmd/dogevm-connect
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed for $($t.GOOS)/$($t.GOARCH)"
        }
    }
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Write-Host "All extra targets written to dist\"
}

Write-Host "Run:  .\dist\$OutName"
