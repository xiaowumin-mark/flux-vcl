<#
.SYNOPSIS
  FluxVCL 构建脚本（Phase 0.3 构建脚手架）。

  一键完成：生成 Windows 资源（go-winres：PerMonitorV2 manifest + 图标 + 版本信息）
  -> go build 产出 windowsgui exe -> 复制 libenergy DLL 到 exe 旁。

.EXAMPLE
  .\scripts\build.ps1                     # 构建 examples/basic -> bin/basic.exe
  .\scripts\build.ps1 -Target basic -Arch amd64
  $env:FVCL_LIBENERGY_DLL = "D:\lib\libenergy-amd64.dll"; .\scripts\build.ps1

.NOTES
  libenergy-amd64.dll 必须与 lcl 包版本严格一致（见 docs/phase0-e2-libenergy-mapping.md）。
  默认从 ref/ 下已验证副本取 DLL；也可用环境变量 FVCL_LIBENERGY_DLL 指定。
  module、DLL 来源与 SHA-256 统一锁定在 packaging/dependencies.lock.json。
#>
param(
    [string]$Target = "basic",   # 目标应用目录：examples/<Target>
    [string]$Arch   = "amd64",   # GOARCH
    [string]$Output = "bin"      # 输出目录（仓库根相对）
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$lockPath = Join-Path $root "packaging\dependencies.lock.json"
$lock = Get-Content -LiteralPath $lockPath -Raw | ConvertFrom-Json
$runtimeFileName = [string]$lock.runtime.fileName
$verifyScript = Join-Path $PSScriptRoot "verify-dependencies.ps1"

# ---------- 1. 定位 libenergy DLL ----------
$dllCandidates = @(
    $env:FVCL_LIBENERGY_DLL,
    (Join-Path $root "ref\e1-smoke\$runtimeFileName"),
    (Join-Path $root "ref\designer-lib\$runtimeFileName")
)
$dllCandidates = @($dllCandidates | Where-Object { $_ -and (Test-Path $_) })
if ($dllCandidates.Count -eq 0) {
    throw "libenergy-amd64.dll 未找到。请设置 FVCL_LIBENERGY_DLL，或运行 scripts/fetch-libenergy.ps1（见 docs/phase0-e2-libenergy-mapping.md）"
}
$dll = $dllCandidates[0]
& $verifyScript -DllPath $dll -Arch $Arch

# ---------- 2. 生成 Windows 资源（manifest/icon/version -> *.syso） ----------
Push-Location (Join-Path $root "examples\$Target")
try {
    # 固定工具版本，避免 @latest 漂移改变发布资源或在 CI 中突然失效。
    $goWinres = "$($lock.tools.goWinresModule)@$($lock.tools.goWinresVersion)"
    go run $goWinres make --arch $Arch
    if ($LASTEXITCODE -ne 0) { throw "go-winres make 失败" }
} finally { Pop-Location }

# ---------- 3. go build ----------
$outDir = if ([IO.Path]::IsPathRooted($Output)) { $Output } else { Join-Path $root $Output }
New-Item -ItemType Directory -Force $outDir | Out-Null
$outExe = Join-Path $outDir "$Target.exe"

$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = $Arch

Push-Location $root
try {
    go build -buildmode=exe -ldflags "-H=windowsgui -s -w" -o $outExe ".\examples\$Target"
    if ($LASTEXITCODE -ne 0) { throw "go build 失败" }
} finally { Pop-Location }

# ---------- 4. 复制 DLL 到 exe 旁 ----------
Copy-Item -LiteralPath $dll (Join-Path $outDir $runtimeFileName) -Force

Write-Host "[build] OK: $outExe"
