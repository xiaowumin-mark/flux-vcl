<#
.SYNOPSIS
  获取 libenergy-amd64.dll（从 energye/designer 内嵌 zip，E2 结论的权威来源）。

  下载并解压到指定目录（默认 ref/designer-lib，build.ps1 会自动找到）。
  版本锁定：designer commit 5c4ec54（2026-04-22，其 go.mod 锁 lcl v1.0.3）。
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

# 锁定 designer 版本：其 go.mod 锁 lcl v1.0.3，DLL 与 lcl v1.0.3 严格配对
$DesignerCommit = "5c4ec54834ce00641920c6c79616e8f4d58b5a68"
$Url = "https://raw.githubusercontent.com/energye/designer/$DesignerCommit/resources/frameworks/lib/windows/libenergy-amd64.zip"

$outDll = Join-Path $OutputDir "libenergy-amd64.dll"
if ((Test-Path $outDll) -and -not $Force) {
    Write-Host "[fetch] DLL 已存在: $outDll（-Force 强制重新获取）"
    return
}

Write-Host "[fetch] 下载 designer@$($DesignerCommit.Substring(0,7)) libenergy-amd64.zip ..."
$zip = Join-Path $env:TEMP "libenergy-amd64.zip"
Invoke-WebRequest -Uri $Url -OutFile $zip

New-Item -ItemType Directory -Force $OutputDir | Out-Null
Expand-Archive -Path $zip -DestinationPath $OutputDir -Force

if (-not (Test-Path $outDll)) {
    Write-Error "解压后未找到 $outDll"
    exit 2
}
Write-Host "[fetch] OK: $outDll ($((Get-Item $outDll).Length) bytes)"
