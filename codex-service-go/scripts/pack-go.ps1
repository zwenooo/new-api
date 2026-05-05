#requires -Version 5
param(
  [string]$OutDir = "dist"
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function New-Dir($p){ if(!(Test-Path $p)){ New-Item -ItemType Directory -Force -Path $p | Out-Null } }

pushd $PSScriptRoot\..
try {
  $out = Resolve-Path $OutDir
} catch {
  New-Dir $OutDir
  $out = Resolve-Path $OutDir
}

Write-Host "Building codex-service-go (server) ..."
go build -o "$out/codex-service-go.exe" ./cmd/server

Write-Host "Building sse-replay tool ..."
go build -o "$out/sse-replay.exe" ./cmd/sse-replay

Copy-Item .env.example "$out/.env.example" -Force

@"
@echo off
setlocal
set DIR=%~dp0
"%DIR%codex-service-go.exe"
"@ | Set-Content "$out\start.bat"

Write-Host "Packed to $out"
popd

