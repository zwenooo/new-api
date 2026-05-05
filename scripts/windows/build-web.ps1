<#
  Build frontend (web/dist) on Windows.

  背景：
  - 后端通过 go:embed 把 `web/dist` 嵌入到二进制中（main.go / router/web-router.go）。
  - 因此只改 `web/src/**` 不重新打包 `web/dist` 的话，/console 页面不会更新，
    你会看不到新增的 UI（例如“请求链路(持久化)”）。

  用法（推荐在仓库根目录执行）：
    scripts\windows\build-web.cmd
  或：
    powershell -NoProfile -ExecutionPolicy Bypass -File scripts\windows\build-web.ps1

  可选：
    powershell ... -File scripts\windows\build-web.ps1 -RunBackend
#>

param(
  [string]$Version,
  [string]$NpmRegistry = 'https://registry.npmmirror.com',
  [switch]$RunBackend
)

$ErrorActionPreference = 'Stop'

if ($env:OS -ne 'Windows_NT') {
  Write-Error 'This script is intended to run in Windows PowerShell (not WSL/Linux).'
  exit 1
}

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot '..\..')
Set-Location $repoRoot

$versionFile = Join-Path $repoRoot 'VERSION'
$version = $Version
if ([string]::IsNullOrWhiteSpace($version) -and (Test-Path $versionFile)) {
  $version = (Get-Content $versionFile -Raw).Trim()
}

# Keep behavior consistent with Dockerfile/makefile.
$env:DISABLE_ESLINT_PLUGIN = 'true'
$env:VITE_REACT_APP_VERSION = $version
$env:NPM_CONFIG_REGISTRY = $NpmRegistry
$env:npm_config_registry = $NpmRegistry

Write-Host "Building frontend into web/dist (VITE_REACT_APP_VERSION=$version, registry=$NpmRegistry) ..."

Push-Location (Join-Path $repoRoot 'web')
try {
  $bun = Get-Command bun -ErrorAction SilentlyContinue
  if ($bun) {
    & bun install --frozen-lockfile --registry $NpmRegistry
    if ($LASTEXITCODE -ne 0) { throw "bun install failed (exit=$LASTEXITCODE)" }
    & bun run build
    if ($LASTEXITCODE -ne 0) { throw "bun run build failed (exit=$LASTEXITCODE)" }
  } else {
    $npm = Get-Command npm -ErrorAction SilentlyContinue
    if (-not $npm) {
      throw 'Neither bun nor npm is available. Install Node.js (npm) or Bun first.'
    }
    if (Test-Path 'package-lock.json') {
      & npm ci
      if ($LASTEXITCODE -ne 0) { throw "npm ci failed (exit=$LASTEXITCODE)" }
    } else {
      & npm install
      if ($LASTEXITCODE -ne 0) { throw "npm install failed (exit=$LASTEXITCODE)" }
    }
    & npm run build
    if ($LASTEXITCODE -ne 0) { throw "npm run build failed (exit=$LASTEXITCODE)" }
  }
} finally {
  Pop-Location
}

Write-Host 'OK: web/dist updated.'

if ($RunBackend) {
  Write-Host 'Starting backend: go run main.go'
  & go run main.go
}
