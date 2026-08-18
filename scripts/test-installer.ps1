<#
.SYNOPSIS
  对安装包执行安装、真实示例 smoke 与卸载闭环。

.DESCRIPTION
  CI 在一次性 windows-latest VM 上运行此脚本，覆盖用户安装目录中的 EXE、
  DLL、许可证、开始菜单入口、卸载注册表与最终清理。

.EXAMPLE
  .\scripts\test-installer.ps1 -InstallerPath .\bin\FluxVCL-0.1.0-basic-setup.exe
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$InstallerPath,
    [string]$Target = "basic",
    [string]$Version = "0.1.0",
    [string]$InstallDir = ""
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$root = Split-Path -Parent $PSScriptRoot
$installer = (Resolve-Path -LiteralPath $InstallerPath).Path
$usingDefaultInstallDir = -not $InstallDir
if (-not $InstallDir) {
    $InstallDir = Join-Path ([Environment]::GetFolderPath("LocalApplicationData")) "Programs\FluxVCL\$Target"
}
$InstallDir = [IO.Path]::GetFullPath($InstallDir).TrimEnd([IO.Path]::DirectorySeparatorChar)
$installRoot = [IO.Path]::GetPathRoot($InstallDir).TrimEnd([IO.Path]::DirectorySeparatorChar)
if ($InstallDir.TrimEnd([IO.Path]::DirectorySeparatorChar) -eq $installRoot) {
    throw "测试安装目录不能是卷根目录: $InstallDir"
}
if (Test-Path -LiteralPath $InstallDir) {
    throw "测试安装目录必须不存在: $InstallDir"
}

$lockPath = Join-Path $root "packaging\dependencies.lock.json"
$lock = Get-Content -LiteralPath $lockPath -Raw | ConvertFrom-Json
$runtimeDll = [string]$lock.runtime.fileName
$uninstallKey = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\FluxVCL-$Target"
$productKey = "HKCU:\Software\FluxVCL\Examples\$Target"
$programsDir = [Environment]::GetFolderPath("Programs")
$appShortcut = Join-Path $programsDir "FluxVCL\$Target example.lnk"
$uninstallShortcut = Join-Path $programsDir "FluxVCL\Uninstall $Target example.lnk"
$uninstaller = Join-Path $InstallDir "uninstall.exe"
$installedExe = Join-Path $InstallDir "$Target.exe"
$installedDll = Join-Path $InstallDir $runtimeDll
$productRoot = Split-Path -Parent $InstallDir
$productRootExisted = Test-Path -LiteralPath $productRoot
$redirectRoot = [IO.Path]::GetFullPath((Join-Path $root "bin\installer-smoke"))
$reinstallRedirect = [IO.Path]::GetFullPath((Join-Path $redirectRoot ("reinstall-redirect-$Target-" + [guid]::NewGuid().ToString("N"))))
if (-not $reinstallRedirect.StartsWith($redirectRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase) -or
    (Test-Path -LiteralPath $reinstallRedirect)) {
    throw "重装重定向测试目录不安全或已存在: $reinstallRedirect"
}
$ownsReinstallRedirect = $true
$installArguments = @("/S")
if (-not $usingDefaultInstallDir) { $installArguments += "/D=$InstallDir" }
$installed = $false

if ((Test-Path $uninstallKey) -or (Test-Path $productKey) -or
    (Test-Path -LiteralPath $appShortcut -PathType Leaf) -or
    (Test-Path -LiteralPath $uninstallShortcut -PathType Leaf)) {
    throw "已存在 FluxVCL $Target 安装；测试拒绝覆盖真实安装"
}

try {
    $installProcess = Start-Process -FilePath $installer -ArgumentList $installArguments -Wait -PassThru
    if ($installProcess.ExitCode -ne 0) {
        throw "静默安装失败，退出码 $($installProcess.ExitCode)"
    }
    $installed = $true

    $requiredFiles = @(
        "$Target.exe",
        $runtimeDll,
        "LICENSE.txt",
        "THIRD-PARTY-NOTICES.txt",
        "energye-Apache-2.0.txt",
        "Go-LICENSE.txt",
        "Go-PATENTS.txt",
        "dependencies.lock.json",
        "uninstall.exe"
    )
    foreach ($file in $requiredFiles) {
        $path = Join-Path $InstallDir $file
        if (-not (Test-Path -LiteralPath $path -PathType Leaf) -or (Get-Item -LiteralPath $path).Length -le 0) {
            throw "安装文件缺失或为空: $path"
        }
    }
    if ((Get-FileHash -LiteralPath (Join-Path $InstallDir "dependencies.lock.json") -Algorithm SHA256).Hash -ne
        (Get-FileHash -LiteralPath $lockPath -Algorithm SHA256).Hash) {
        throw "安装的 dependencies.lock.json 与构建锁不一致"
    }
    if (-not (Test-Path -LiteralPath $appShortcut -PathType Leaf) -or
        -not (Test-Path -LiteralPath $uninstallShortcut -PathType Leaf)) {
        throw "开始菜单示例/卸载入口缺失"
    }
    if (-not (Test-Path $uninstallKey) -or -not (Test-Path $productKey)) {
        throw "Windows 安装/卸载注册表项缺失"
    }
    $uninstallRegistration = Get-ItemProperty $uninstallKey
    $productRegistration = Get-ItemProperty $productKey
    $registeredInstallDir = [IO.Path]::GetFullPath([string]$uninstallRegistration.InstallLocation).TrimEnd([IO.Path]::DirectorySeparatorChar)
    if (-not [string]::Equals($registeredInstallDir, $InstallDir, [StringComparison]::OrdinalIgnoreCase)) {
        throw "安装器未使用请求目录: actual=$registeredInstallDir expected=$InstallDir"
    }
    $registeredProductDir = [IO.Path]::GetFullPath([string]$productRegistration.InstallDir).TrimEnd([IO.Path]::DirectorySeparatorChar)
    $expectedUninstallString = '"' + $uninstaller + '"'
    if (-not [string]::Equals($registeredProductDir, $InstallDir, [StringComparison]::OrdinalIgnoreCase) -or
        [string]$uninstallRegistration.DisplayName -ne "FluxVCL $Target example" -or
        [string]$uninstallRegistration.DisplayVersion -ne $Version -or
        [string]$uninstallRegistration.Publisher -ne "FluxVCL" -or
        -not [string]::Equals([string]$uninstallRegistration.DisplayIcon, $installedExe, [StringComparison]::OrdinalIgnoreCase) -or
        [string]$uninstallRegistration.UninstallString -ne $expectedUninstallString -or
        [string]$uninstallRegistration.QuietUninstallString -ne "$expectedUninstallString /S" -or
        [int]$uninstallRegistration.NoModify -ne 1 -or
        [int]$uninstallRegistration.NoRepair -ne 1) {
        throw "安装/卸载注册表内容不一致"
    }

    $shortcutShell = New-Object -ComObject WScript.Shell
    try {
        $appShortcutTarget = $shortcutShell.CreateShortcut($appShortcut).TargetPath
        $uninstallShortcutTarget = $shortcutShell.CreateShortcut($uninstallShortcut).TargetPath
    } finally {
        [Runtime.InteropServices.Marshal]::FinalReleaseComObject($shortcutShell) | Out-Null
    }
    if (-not [string]::Equals($appShortcutTarget, $installedExe, [StringComparison]::OrdinalIgnoreCase) -or
        -not [string]::Equals($uninstallShortcutTarget, $uninstaller, [StringComparison]::OrdinalIgnoreCase)) {
        throw "开始菜单快捷方式目标不一致"
    }

    $LASTEXITCODE = 0
    & (Join-Path $PSScriptRoot "verify-release.ps1") `
        -ExePath $installedExe -DllPath $installedDll -Target $Target -Version $Version
    if ($LASTEXITCODE -ne 0) {
        throw "安装目录发布资源校验失败，退出码 $LASTEXITCODE"
    }
    $LASTEXITCODE = 0
    & (Join-Path $PSScriptRoot "smoke.ps1") `
        -Target $Target -ExePath $installedExe `
        -Screenshot (Join-Path $root "bin\$Target-installer-smoke.png")
    if ($LASTEXITCODE -ne 0) {
        throw "安装目录交互 smoke 失败，退出码 $LASTEXITCODE"
    }

    if (Test-Path -LiteralPath $reinstallRedirect) {
        throw "重装重定向测试目录必须不存在: $reinstallRedirect"
    }
    $runningApp = Start-Process -FilePath $installedExe -PassThru
    try {
        if (-not $runningApp.WaitForInputIdle(5000)) {
            throw "运行中重装测试无法等待示例窗口就绪"
        }
        $lockedReinstall = Start-Process -FilePath $installer `
            -ArgumentList @("/S", "/D=$reinstallRedirect") -Wait -PassThru
        if ($lockedReinstall.ExitCode -eq 0) {
            throw "应用运行时重装应失败但返回成功"
        }
        if (-not (Test-Path -LiteralPath $installedExe -PathType Leaf) -or
            -not (Test-Path -LiteralPath $installedDll -PathType Leaf) -or
            -not (Test-Path -LiteralPath $uninstaller -PathType Leaf) -or
            -not (Test-Path $uninstallKey) -or -not (Test-Path $productKey) -or
            -not (Test-Path -LiteralPath $appShortcut -PathType Leaf) -or
            -not (Test-Path -LiteralPath $uninstallShortcut -PathType Leaf)) {
            throw "运行中重装失败后未保留原安装与卸载入口"
        }
        if (Test-Path -LiteralPath $reinstallRedirect) {
            throw "运行中重装错误创建了新目录: $reinstallRedirect"
        }
        Write-Host "[installer-smoke] PASS: running application blocks reinstall without losing entry"
    } finally {
        if (-not $runningApp.HasExited) {
            Stop-Process -Id $runningApp.Id -Force -ErrorAction SilentlyContinue
            $runningApp.WaitForExit(5000) | Out-Null
        }
        $runningApp.Dispose()
    }

    $reinstallProcess = Start-Process -FilePath $installer `
        -ArgumentList @("/S", "/D=$reinstallRedirect") -Wait -PassThru
    if ($reinstallProcess.ExitCode -ne 0) {
        throw "关闭应用后原目录重装失败，退出码 $($reinstallProcess.ExitCode)"
    }
    $registeredAfterReinstall = [IO.Path]::GetFullPath([string](Get-ItemProperty $uninstallKey).InstallLocation).TrimEnd([IO.Path]::DirectorySeparatorChar)
    if (-not [string]::Equals($registeredAfterReinstall, $InstallDir, [StringComparison]::OrdinalIgnoreCase) -or
        (Test-Path -LiteralPath $reinstallRedirect)) {
        throw "重装未沿用原安装目录"
    }
    Write-Host "[installer-smoke] PASS: reinstall keeps the registered install directory"

    $runningApp = Start-Process -FilePath $installedExe -PassThru
    try {
        if (-not $runningApp.WaitForInputIdle(5000)) {
            throw "运行中卸载测试无法等待示例窗口就绪"
        }

        # _?= bypasses NSIS's temporary bootstrapper so the real uninstaller's
        # SetErrorLevel is observable. It is safe here because the guarded path
        # exits before attempting to remove uninstall.exe.
        $directLockedUninstall = Start-Process -FilePath $uninstaller `
            -ArgumentList @("/S", "_?=$InstallDir") -Wait -PassThru
        if ($directLockedUninstall.ExitCode -eq 0) {
            throw "应用运行时卸载体应失败但返回成功"
        }

        # The normal NSIS launcher copies the uninstaller to %TEMP% and its
        # bootstrap process always returns 0. Validate its durable state instead.
        $lockedUninstall = Start-Process -FilePath $uninstaller -ArgumentList "/S" -Wait -PassThru
        if (-not (Test-Path -LiteralPath $installedExe -PathType Leaf) -or
            -not (Test-Path -LiteralPath $installedDll -PathType Leaf) -or
            -not (Test-Path -LiteralPath $uninstaller -PathType Leaf) -or
            -not (Test-Path $uninstallKey) -or -not (Test-Path $productKey) -or
            -not (Test-Path -LiteralPath $appShortcut -PathType Leaf) -or
            -not (Test-Path -LiteralPath $uninstallShortcut -PathType Leaf)) {
            throw "运行中卸载失败后未保留可重试的安装与卸载入口"
        }
        Write-Host "[installer-smoke] PASS: running application blocks uninstall without losing entry (inner=$($directLockedUninstall.ExitCode), bootstrap=$($lockedUninstall.ExitCode))"
    } finally {
        if (-not $runningApp.HasExited) {
            Stop-Process -Id $runningApp.Id -Force -ErrorAction SilentlyContinue
            $runningApp.WaitForExit(5000) | Out-Null
        }
        $runningApp.Dispose()
    }

    $uninstallProcess = Start-Process -FilePath $uninstaller -ArgumentList "/S" -Wait -PassThru
    if ($uninstallProcess.ExitCode -ne 0) {
        throw "静默卸载失败，退出码 $($uninstallProcess.ExitCode)"
    }
    $installed = $false

    for ($attempt = 0; $attempt -lt 100; $attempt++) {
        $uninstallPending = (Test-Path -LiteralPath $InstallDir) -or
            (Test-Path -LiteralPath $appShortcut -PathType Leaf) -or
            (Test-Path -LiteralPath $uninstallShortcut -PathType Leaf) -or
            (Test-Path $uninstallKey) -or (Test-Path $productKey)
        if (-not $uninstallPending) { break }
        Start-Sleep -Milliseconds 100
    }
    if (Test-Path -LiteralPath $InstallDir) { throw "卸载后安装目录仍存在: $InstallDir" }
    if ((Test-Path -LiteralPath $appShortcut -PathType Leaf) -or
        (Test-Path -LiteralPath $uninstallShortcut -PathType Leaf)) {
        throw "卸载后开始菜单入口仍存在"
    }
    if ((Test-Path $uninstallKey) -or (Test-Path $productKey)) {
        throw "卸载后注册表项仍存在"
    }
    if ($usingDefaultInstallDir -and -not $productRootExisted -and
        (Test-Path -LiteralPath $productRoot)) {
        throw "卸载后空的产品父目录仍存在: $productRoot"
    }

    Write-Host "[installer-smoke] PASS: install -> launch -> uninstall"
} finally {
    if ($installed -or (Test-Path $uninstallKey) -or (Test-Path $productKey) -or
        (Test-Path -LiteralPath $uninstaller -PathType Leaf)) {
        $cleanupUninstaller = $uninstaller
        if (-not (Test-Path -LiteralPath $cleanupUninstaller -PathType Leaf) -and (Test-Path $uninstallKey)) {
            $registeredDir = [string](Get-ItemProperty $uninstallKey -ErrorAction SilentlyContinue).InstallLocation
            if ($registeredDir) { $cleanupUninstaller = Join-Path $registeredDir "uninstall.exe" }
        }
        if (Test-Path -LiteralPath $cleanupUninstaller -PathType Leaf) {
            try { Start-Process -FilePath $cleanupUninstaller -ArgumentList "/S" -Wait | Out-Null } catch { }
        }
    }
    if (Test-Path -LiteralPath $InstallDir) {
        try {
            Remove-Item -LiteralPath $InstallDir -Recurse -Force -ErrorAction Stop
        } catch {
            Write-Warning "测试安装目录清理失败: $InstallDir ($($_.Exception.Message))"
        }
    }
    if ($ownsReinstallRedirect -and (Test-Path -LiteralPath $reinstallRedirect)) {
        try {
            Remove-Item -LiteralPath $reinstallRedirect -Recurse -Force -ErrorAction Stop
        } catch {
            Write-Warning "重装重定向目录清理失败: $reinstallRedirect ($($_.Exception.Message))"
        }
    }
    foreach ($shortcut in @($appShortcut, $uninstallShortcut)) {
        if (Test-Path -LiteralPath $shortcut -PathType Leaf) {
            Remove-Item -LiteralPath $shortcut -Force -ErrorAction SilentlyContinue
        }
    }
    foreach ($registryKey in @($uninstallKey, $productKey)) {
        if (Test-Path $registryKey) {
            Remove-Item -LiteralPath $registryKey -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
    if ($usingDefaultInstallDir -and -not $productRootExisted -and
        (Test-Path -LiteralPath $productRoot -PathType Container)) {
        try { [IO.Directory]::Delete($productRoot, $false) } catch { }
    }
}
