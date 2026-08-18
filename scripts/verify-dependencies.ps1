<#
.SYNOPSIS
  校验发布依赖映射：Go module、DLL 固定来源、架构与 SHA-256。

.DESCRIPTION
  packaging/dependencies.lock.json 是唯一发布锁。DLL 的哈希绑定到其中固定的
  energye/designer commit 与 archivePath；任何 module 替换、版本漂移、latest
  URL、架构错配或 DLL 内容变化都会在构建前失败。

.EXAMPLE
  .\scripts\verify-dependencies.ps1
  .\scripts\verify-dependencies.ps1 -DllPath .\ref\designer-lib\libenergy-amd64.dll
#>
[CmdletBinding()]
param(
    [string]$DllPath = "",
    [string]$Arch = "amd64"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
$lockPath = Join-Path $root "packaging\dependencies.lock.json"
if (-not (Test-Path -LiteralPath $lockPath -PathType Leaf)) {
    throw "发布依赖锁不存在: $lockPath"
}

$lock = Get-Content -LiteralPath $lockPath -Raw | ConvertFrom-Json
if ($lock.schemaVersion -ne 1) {
    throw "不支持的发布依赖锁版本: $($lock.schemaVersion)"
}

$modulePath = [string]$lock.goModule.path
$moduleVersion = [string]$lock.goModule.version
$runtimeArch = [string]$lock.runtime.goarch
$runtimeOS = [string]$lock.runtime.goos
$runtimeFileName = [string]$lock.runtime.fileName
$runtimeHash = ([string]$lock.runtime.sha256).ToUpperInvariant()
$sourceRepository = [string]$lock.runtime.source.repository
$sourceCommit = [string]$lock.runtime.source.commit
$sourceArchivePath = ([string]$lock.runtime.source.archivePath).Replace("\", "/")
$sourceUrl = [string]$lock.runtime.source.url

if ($modulePath -ne "github.com/energye/lcl" -or $moduleVersion -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+$') {
    throw "依赖锁中的 energye/lcl 映射无效: $modulePath $moduleVersion"
}
if ($runtimeOS -ne "windows" -or $Arch -ne $runtimeArch) {
    throw "发布运行时只锁定 $runtimeOS/$runtimeArch，收到架构 windows/$Arch"
}
if ($runtimeFileName -ne "libenergy-$runtimeArch.dll") {
    throw "依赖锁中的 DLL 文件名与架构不一致: $runtimeFileName"
}
if ($runtimeHash -notmatch '^[0-9A-F]{64}$') {
    throw "依赖锁中的 DLL SHA-256 无效: $runtimeHash"
}
if ($sourceCommit -notmatch '^[0-9a-fA-F]{40}$') {
    throw "依赖锁中的 designer commit 必须是完整 40 位提交: $sourceCommit"
}
if ($sourceRepository -ne "https://github.com/energye/designer") {
    throw "DLL 权威仓库不一致: $sourceRepository"
}
if ($sourceUrl -match '(?i)(/latest/|@latest|/main/|/master/)') {
    throw "DLL 来源禁止使用浮动版本: $sourceUrl"
}
$expectedSourceUrl = "https://raw.githubusercontent.com/energye/designer/$sourceCommit/$sourceArchivePath"
if (-not [string]::Equals($sourceUrl, $expectedSourceUrl, [StringComparison]::Ordinal)) {
    throw "DLL 来源 URL 与权威仓库、commit、archivePath 不一致: $sourceUrl"
}
if ([string]$lock.tools.goWinresModule -ne "github.com/tc-hib/go-winres" -or
    [string]$lock.tools.goWinresVersion -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+$') {
    throw "go-winres 必须锁定具体 module 版本"
}
if ([string]$lock.tools.nsisVersion -notmatch '^[0-9]+\.[0-9]+$' -or
    [string]$lock.tools.nsisChocolateyVersion -notmatch '^[0-9]+\.[0-9]+\.[0-9]+$') {
    throw "NSIS 必须锁定具体工具与 Chocolatey 包版本"
}

Push-Location $root
try {
    $moduleJson = (& go list -m -json $modulePath 2>&1) -join "`n"
    if ($LASTEXITCODE -ne 0) {
        throw "go list 无法读取 $modulePath：$moduleJson"
    }
} finally {
    Pop-Location
}

$module = $moduleJson | ConvertFrom-Json
if ([string]$module.Path -ne $modulePath -or [string]$module.Version -ne $moduleVersion) {
    throw "Go module 与发布锁不一致: actual=$($module.Path)@$($module.Version) expected=$modulePath@$moduleVersion"
}
if ($module.PSObject.Properties.Name -contains "Replace" -and $null -ne $module.Replace) {
    throw "发布构建禁止 replace $modulePath；实际替换为 $($module.Replace.Path)@$($module.Replace.Version)"
}

if ($DllPath) {
    if (-not (Test-Path -LiteralPath $DllPath -PathType Leaf)) {
        throw "未找到运行时 DLL: $DllPath"
    }
    $resolvedDll = (Resolve-Path -LiteralPath $DllPath).Path
    $actualHash = (Get-FileHash -LiteralPath $resolvedDll -Algorithm SHA256).Hash.ToUpperInvariant()
    if ($actualHash -ne $runtimeHash) {
        throw "DLL 与发布锁不一致: actual=$actualHash expected=$runtimeHash path=$resolvedDll"
    }
    Write-Host "[verify-dependencies] DLL: $resolvedDll (SHA-256 verified)"
}

Write-Host "[verify-dependencies] module: $modulePath@$moduleVersion"
Write-Host "[verify-dependencies] source: designer@$($sourceCommit.Substring(0, 7))/$sourceArchivePath"
