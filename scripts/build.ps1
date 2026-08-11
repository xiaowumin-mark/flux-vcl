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
  DLL 权威来源：energye/designer 内嵌 zip（resources/frameworks/lib/windows/libenergy-amd64.zip）。
#>
param(
    [string]$Target = "basic",   # 目标应用目录：examples/<Target>
    [string]$Arch   = "amd64",   # GOARCH
    [string]$Output = "bin"      # 输出目录（仓库根相对）
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

# ---------- 1. 定位 libenergy DLL ----------
$dllCandidates = @(
    $env:FVCL_LIBENERGY_DLL,
    (Join-Path $root "ref\e1-smoke\libenergy-amd64.dll"),
    (Join-Path $root "ref\designer-lib\libenergy-amd64.dll")
)
$dllCandidates = @($dllCandidates | Where-Object { $_ -and (Test-Path $_) })
if ($dllCandidates.Count -eq 0) {
    Write-Error "libenergy-amd64.dll 未找到。请设置环境变量 FVCL_LIBENERGY_DLL，"
    Write-Error "或从 energye/designer 内嵌 zip 解压到 ref/ 下（见 docs/phase0-e2-libenergy-mapping.md）。"
    exit 2
}
$dll = $dllCandidates[0]
Write-Host "[build] DLL: $dll"

# ---------- 2. 生成 Windows 资源（manifest/icon/version -> *.syso） ----------
Push-Location (Join-Path $root "examples\$Target")
try {
    go run github.com/tc-hib/go-winres@latest make --arch $Arch
    if ($LASTEXITCODE -ne 0) { throw "go-winres make 失败" }
} finally { Pop-Location }

# ---------- 3. go build ----------
$outDir = Join-Path $root $Output
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
Copy-Item $dll (Join-Path $outDir (Split-Path $dll -Leaf)) -Force

Write-Host "[build] OK: $outExe"
