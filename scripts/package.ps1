<#
.SYNOPSIS
  构建 FluxVCL 示例并生成 NSIS Windows 安装包。

.EXAMPLE
  .\scripts\package.ps1
  .\scripts\package.ps1 -Target page-control -Version 0.1.0
#>
[CmdletBinding()]
param(
    [string]$Target = "basic",
    [string]$Arch = "amd64",
    [string]$Version = "0.1.0",
    [string]$Output = "bin",
    [string]$MakensisPath = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
$lockPath = Join-Path $root "packaging\dependencies.lock.json"
$lock = Get-Content -LiteralPath $lockPath -Raw | ConvertFrom-Json

if ($Version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+$') {
    throw "安装包版本必须是三段数字 semver: $Version"
}
if ($Target -notmatch '^[a-z0-9]+(?:-[a-z0-9]+)*$' -or
    -not (Test-Path -LiteralPath (Join-Path $root "examples\$Target\main.go") -PathType Leaf)) {
    throw "未知或非法的示例 target: $Target"
}

& (Join-Path $PSScriptRoot "verify-dependencies.ps1") -Arch $Arch

if (-not $MakensisPath) {
    $makensisCommand = Get-Command makensis.exe -ErrorAction SilentlyContinue
    if ($null -ne $makensisCommand) {
        $MakensisPath = $makensisCommand.Source
    } else {
        $programFilesX86 = [Environment]::GetFolderPath("ProgramFilesX86")
        $knownPaths = @(
            (Join-Path $programFilesX86 "NSIS\makensis.exe"),
            (Join-Path $programFilesX86 "NSIS\Bin\makensis.exe")
        )
        $MakensisPath = @($knownPaths | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf } | Select-Object -First 1)
        if ($MakensisPath.Count -eq 1) { $MakensisPath = [string]$MakensisPath[0] }
    }
}
if (-not $MakensisPath -or -not (Test-Path -LiteralPath $MakensisPath -PathType Leaf)) {
    throw "未找到 makensis.exe；安装 NSIS $($lock.tools.nsisVersion) 或传入 -MakensisPath"
}
$nsisVersion = ((& $MakensisPath /VERSION) -join "").Trim().TrimStart('v')
if ($LASTEXITCODE -ne 0 -or $nsisVersion -ne [string]$lock.tools.nsisVersion) {
    throw "NSIS 版本不一致: actual=$nsisVersion expected=$($lock.tools.nsisVersion)"
}

$outDir = if ([IO.Path]::IsPathRooted($Output)) { $Output } else { Join-Path $root $Output }
$stageName = "$Target-$Version-$PID-$([guid]::NewGuid().ToString('N'))"
$stageDir = Join-Path $outDir "package-stage\$stageName"
New-Item -ItemType Directory -Force -Path $stageDir | Out-Null

try {
    $LASTEXITCODE = 0
    & (Join-Path $PSScriptRoot "build.ps1") -Target $Target -Arch $Arch -Output $stageDir
    if ($LASTEXITCODE -ne 0) {
        throw "示例构建失败，退出码 $LASTEXITCODE"
    }

    $appExe = "$Target.exe"
    $runtimeDll = [string]$lock.runtime.fileName
    $stagedExe = Join-Path $stageDir $appExe
    $stagedDll = Join-Path $stageDir $runtimeDll

    Copy-Item -LiteralPath (Join-Path $root "LICENSE") (Join-Path $stageDir "LICENSE.txt") -Force
    Copy-Item -LiteralPath (Join-Path $root "packaging\THIRD-PARTY-NOTICES.txt") $stageDir -Force
    Copy-Item -LiteralPath $lockPath $stageDir -Force

    $goRoot = ((& go env GOROOT) -join "`n").Trim()
    if ($LASTEXITCODE -ne 0 -or -not $goRoot) {
        throw "无法定位构建所用 Go 工具链许可证"
    }
    $goLicense = Join-Path $goRoot "LICENSE"
    $goPatents = Join-Path $goRoot "PATENTS"
    if (-not (Test-Path -LiteralPath $goLicense -PathType Leaf) -or
        -not (Test-Path -LiteralPath $goPatents -PathType Leaf)) {
        throw "Go 工具链 LICENSE/PATENTS 不完整: $goRoot"
    }
    Copy-Item -LiteralPath $goLicense (Join-Path $stageDir "Go-LICENSE.txt") -Force
    Copy-Item -LiteralPath $goPatents (Join-Path $stageDir "Go-PATENTS.txt") -Force

    Push-Location $root
    try {
        $moduleJson = (& go list -m -json ([string]$lock.goModule.path) 2>&1) -join "`n"
        if ($LASTEXITCODE -ne 0) { throw "无法定位 energye/lcl 许可证: $moduleJson" }
    } finally {
        Pop-Location
    }
    $module = $moduleJson | ConvertFrom-Json
    $upstreamLicense = Join-Path ([string]$module.Dir) "LICENSE"
    if (-not (Test-Path -LiteralPath $upstreamLicense -PathType Leaf)) {
        throw "energye/lcl 许可证不存在: $upstreamLicense"
    }
    Copy-Item -LiteralPath $upstreamLicense (Join-Path $stageDir "energye-Apache-2.0.txt") -Force

    $LASTEXITCODE = 0
    & (Join-Path $PSScriptRoot "verify-release.ps1") `
        -ExePath $stagedExe -DllPath $stagedDll -Target $Target -Version $Version
    if ($LASTEXITCODE -ne 0) {
        throw "发布资源校验失败，退出码 $LASTEXITCODE"
    }

    $installerPath = Join-Path $outDir "FluxVCL-$Version-$Target-setup.exe"
    $installerScript = Join-Path $root "packaging\installer.nsi"
    $nsisArgs = @(
        "/V2",
        "/DVERSION=$Version",
        "/DTARGET=$Target",
        "/DAPP_EXE=$appExe",
        "/DRUNTIME_DLL=$runtimeDll",
        "/DSTAGE_DIR=$stageDir",
        "/DOUTPUT_FILE=$installerPath",
        $installerScript
    )
    & $MakensisPath @nsisArgs
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $installerPath -PathType Leaf)) {
        throw "NSIS 安装包构建失败"
    }

    $installerHashInfo = $null
    for ($hashAttempt = 0; $hashAttempt -lt 20; $hashAttempt++) {
        try {
            $installerHashInfo = Get-FileHash -LiteralPath $installerPath -Algorithm SHA256 -ErrorAction Stop
            break
        } catch {
            if ($hashAttempt -eq 19) { throw }
            Start-Sleep -Milliseconds 250
        }
    }
    $installerHash = $installerHashInfo.Hash.ToLowerInvariant()
    $checksumPath = "$installerPath.sha256"
    [IO.File]::WriteAllText($checksumPath, "$installerHash  $(Split-Path $installerPath -Leaf)`r`n", [Text.Encoding]::ASCII)
} finally {
    if (Test-Path -LiteralPath $stageDir) {
        try {
            Remove-Item -LiteralPath $stageDir -Recurse -Force
        } catch {
            Write-Warning "临时 staging 清理失败: $stageDir ($($_.Exception.Message))"
        }
    }
}

Write-Host "[package] NSIS $nsisVersion"
Write-Host "[package] OK: $installerPath"
Write-Host "[package] SHA-256: $installerHash"
