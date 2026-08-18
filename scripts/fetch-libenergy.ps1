<#
.SYNOPSIS
  获取 libenergy-amd64.dll（从 energye/designer 内嵌 zip，E2 结论的权威来源）。

  下载并解压到指定目录（默认 ref/designer-lib，build.ps1 会自动找到）。
  版本、来源与 SHA-256 统一锁定在 packaging/dependencies.lock.json。
  DLL 必须与 Go 包版本严格一致，见 docs/phase0-e2-libenergy-mapping.md。

.EXAMPLE
  .\scripts\fetch-libenergy.ps1
  .\scripts\fetch-libenergy.ps1 -OutputDir .\ref\designer-lib
#>
param(
    [string]$OutputDir = (Join-Path (Split-Path -Parent $PSScriptRoot) "ref\designer-lib"),
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$lockPath = Join-Path $root "packaging\dependencies.lock.json"
$lock = Get-Content -LiteralPath $lockPath -Raw | ConvertFrom-Json
$verifyScript = Join-Path $PSScriptRoot "verify-dependencies.ps1"

$DesignerCommit = [string]$lock.runtime.source.commit
$Url = [string]$lock.runtime.source.url
$DllFileName = [string]$lock.runtime.fileName

function Assert-LibenergyDll {
    param([string]$Path)
    & $verifyScript -DllPath $Path -Arch ([string]$lock.runtime.goarch)
}

$outDll = Join-Path $OutputDir $DllFileName
if ((Test-Path $outDll) -and -not $Force) {
    Assert-LibenergyDll $outDll
    Write-Host "[fetch] DLL 已存在且哈希通过: $outDll（-Force 强制重新获取）"
    return
}

Write-Host "[fetch] 下载 designer@$($DesignerCommit.Substring(0,7)) libenergy-amd64.zip ..."
$zip = Join-Path $env:TEMP "libenergy-amd64-$($DesignerCommit.Substring(0, 12)).zip"
try {
    Invoke-WebRequest -Uri $Url -OutFile $zip
    New-Item -ItemType Directory -Force $OutputDir | Out-Null
    Expand-Archive -LiteralPath $zip -DestinationPath $OutputDir -Force
} finally {
    Remove-Item -LiteralPath $zip -Force -ErrorAction SilentlyContinue
}

Assert-LibenergyDll $outDll
Write-Host "[fetch] OK: $outDll ($((Get-Item $outDll).Length) bytes; SHA-256 verified)"
