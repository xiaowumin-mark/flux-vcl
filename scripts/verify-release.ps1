<#
.SYNOPSIS
  验证发布 EXE 的 DLL 映射、版本资源与 Windows manifest。

.EXAMPLE
  .\scripts\verify-release.ps1 -ExePath .\bin\basic.exe `
      -DllPath .\bin\libenergy-amd64.dll -Target basic -Version 0.1.0
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$ExePath,
    [Parameter(Mandatory = $true)][string]$DllPath,
    [string]$Target = "basic",
    [string]$Version = "0.1.0"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
$lockPath = Join-Path $root "packaging\dependencies.lock.json"
$lock = Get-Content -LiteralPath $lockPath -Raw | ConvertFrom-Json

if ($Version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+$') {
    throw "安装包版本必须是三段数字 semver: $Version"
}
if ($Target -notmatch '^[a-z0-9]+(?:-[a-z0-9]+)*$') {
    throw "示例 target 非法: $Target"
}
if (-not (Test-Path -LiteralPath $ExePath -PathType Leaf)) {
    throw "未找到发布 EXE: $ExePath"
}

$resolvedExe = (Resolve-Path -LiteralPath $ExePath).Path
$resolvedDll = (Resolve-Path -LiteralPath $DllPath).Path
& (Join-Path $PSScriptRoot "verify-dependencies.ps1") -DllPath $resolvedDll -Arch ([string]$lock.runtime.goarch)

$versionSource = Get-Content -LiteralPath (Join-Path $root "flux.go") -Raw
$versionMatch = [regex]::Match($versionSource, 'const\s+Version\s*=\s*"([^"]+)"')
if (-not $versionMatch.Success) {
    throw "无法从 flux.go 读取 flux.Version"
}
$frameworkVersion = $versionMatch.Groups[1].Value
$frameworkReleaseVersion = $frameworkVersion -replace '-dev$', ''
if ($frameworkReleaseVersion -ne $Version) {
    throw "安装包版本与 flux.Version 不一致: package=$Version flux=$frameworkVersion"
}

$expectedFourPartVersion = "$Version.0"
$fileInfo = [System.Diagnostics.FileVersionInfo]::GetVersionInfo($resolvedExe)
if ($fileInfo.FileVersionRaw.ToString() -ne $expectedFourPartVersion -or
    $fileInfo.ProductVersionRaw.ToString() -ne $expectedFourPartVersion) {
    throw "EXE 固定版本资源不一致: file=$($fileInfo.FileVersionRaw) product=$($fileInfo.ProductVersionRaw) expected=$expectedFourPartVersion"
}
if ($fileInfo.ProductName -ne "FluxVCL" -or $fileInfo.ProductVersion -ne $Version -or
    $fileInfo.OriginalFilename -ne "$Target.exe") {
    throw "EXE 字符串版本资源不完整: product='$($fileInfo.ProductName)' version='$($fileInfo.ProductVersion)' original='$($fileInfo.OriginalFilename)'"
}

$extractDir = Join-Path ([IO.Path]::GetTempPath()) ("flux-vcl-winres-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $extractDir | Out-Null
try {
    $goWinres = "$($lock.tools.goWinresModule)@$($lock.tools.goWinresVersion)"
    & go run $goWinres extract --dir $extractDir --xml-manifest $resolvedExe
    if ($LASTEXITCODE -ne 0) { throw "go-winres extract 失败" }

    $manifestPath = Join-Path $extractDir "app.manifest"
    if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
        throw "EXE 缺少 Windows manifest"
    }
    [xml]$manifest = Get-Content -LiteralPath $manifestPath -Raw

    $dpiNode = $manifest.SelectSingleNode("//*[local-name()='dpiAwareness']")
    if ($null -eq $dpiNode -or $dpiNode.InnerText.Trim().ToLowerInvariant() -notmatch '^permonitorv2(?:,system)?$') {
        throw "manifest 未启用 perMonitorV2"
    }

    $commonControls = $manifest.SelectSingleNode("//*[local-name()='assemblyIdentity' and @name='Microsoft.Windows.Common-Controls']")
    if ($null -eq $commonControls -or $commonControls.GetAttribute("version") -ne "6.0.0.0") {
        throw "manifest 未启用 common controls v6"
    }

    $identity = $manifest.SelectSingleNode("/*[local-name()='assembly']/*[local-name()='assemblyIdentity']")
    if ($null -eq $identity -or $identity.GetAttribute("version") -ne $expectedFourPartVersion) {
        throw "manifest identity 版本不一致"
    }

    $icon = Get-ChildItem -LiteralPath $extractDir -Filter "*.ico" -File | Select-Object -First 1
    if ($null -eq $icon -or $icon.Length -le 0) {
        throw "EXE 缺少有效图标资源"
    }
} finally {
    if (Test-Path -LiteralPath $extractDir) {
        Remove-Item -LiteralPath $extractDir -Recurse -Force
    }
}

Write-Host "[verify-release] OK: $resolvedExe"
Write-Host "[verify-release] resources: version=$expectedFourPartVersion perMonitorV2 common-controls-v6 icon"
