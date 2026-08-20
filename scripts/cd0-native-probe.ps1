<#
.SYNOPSIS
  Run the CD0 native probes as separate Windows test processes.

  Each probe initializes LCL once in its own process. FVCL_CD0_ARTIFACT_DIR is
  shared so the JSON/PNG evidence survives Go's per-test temporary directory.

.EXAMPLE
  .\scripts\cd0-native-probe.ps1
  .\scripts\cd0-native-probe.ps1 -ArtifactDir .\artifacts\cd0
#>
param(
    [string]$ArtifactDir = "",
    [string]$DllPath = ""
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

if (-not $ArtifactDir) {
    $ArtifactDir = Join-Path $root "bin\cd0-native"
}
$ArtifactDir = [System.IO.Path]::GetFullPath($ArtifactDir)
New-Item -ItemType Directory -Force -Path $ArtifactDir | Out-Null

if (-not $DllPath) {
    $DllPath = $env:FVCL_LIBENERGY_DLL
}
if (-not $DllPath) {
    $DllPath = Join-Path $root "ref\designer-lib\libenergy-amd64.dll"
}
if (-not (Test-Path -LiteralPath $DllPath -PathType Leaf)) {
    throw "libenergy DLL not found: $DllPath"
}
$env:FVCL_LIBENERGY_DLL = (Resolve-Path -LiteralPath $DllPath).Path
$env:FVCL_CD0_ARTIFACT_DIR = $ArtifactDir

$probes = @(
    [PSCustomObject]@{ Name = "canvas"; Pattern = "^TestCD0CanvasRuntimeProbe$" },
    [PSCustomObject]@{ Name = "ownerdraw"; Pattern = "^TestCD0OwnerDrawAndSubclassRuntimeProbe$" },
    [PSCustomObject]@{ Name = "control-draw"; Pattern = "^TestCD0ControlDrawRuntimeProbe$" },
    [PSCustomObject]@{ Name = "win32-abi"; Pattern = "^TestCD0Win32DrawABILayout$" }
)

$failures = New-Object System.Collections.Generic.List[string]
foreach ($probe in $probes) {
    $logPath = Join-Path $ArtifactDir ("cd0-{0}.log" -f $probe.Name)
    Write-Host "[cd0] running $($probe.Name)"
    & go test -count=1 -v ./internal/native -run $probe.Pattern 2>&1 | Tee-Object -FilePath $logPath
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        $failures.Add("$($probe.Name) exited with code $exitCode")
    }
}

# The DLL is amd64-only, but ABI records are pure Go declarations. Build and
# run the same layout assertion as a 32-bit Windows process rather than
# assuming the amd64 layout covers pointer-sized fields.
$savedGOARCH = $env:GOARCH
try {
    $env:GOARCH = "386"
    $abiLogPath = Join-Path $ArtifactDir "cd0-win32-abi-386.log"
    Write-Host "[cd0] running win32-abi-386"
    & go test -count=1 -v ./internal/native -run '^TestCD0Win32DrawABILayout$' 2>&1 | Tee-Object -FilePath $abiLogPath
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        $failures.Add("win32-abi-386 exited with code $exitCode")
    }
} finally {
    if ($null -eq $savedGOARCH) {
        Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    } else {
        $env:GOARCH = $savedGOARCH
    }
}

function Get-CD0Evidence {
    param(
        [string]$Name,
        [string[]]$AllowedStatus
    )

    $path = Join-Path $ArtifactDir $Name
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        $failures.Add("CD0 evidence JSON is missing: $path")
        return $null
    }
    try {
        $evidence = Get-Content -LiteralPath $path -Raw | ConvertFrom-Json
    } catch {
        $failures.Add("CD0 evidence JSON is invalid: $path ($($_.Exception.Message))")
        return $null
    }
    if ($AllowedStatus -notcontains [string]$evidence.status) {
        $failures.Add("CD0 evidence '$Name' has status '$($evidence.status)', expected $($AllowedStatus -join ', ')")
    }
    return $evidence
}

$canvasEvidence = Get-CD0Evidence "cd0-canvas-probe.json" @("supported")
$canvasPNGPath = Join-Path $ArtifactDir "cd0-canvas-probe.png"
if (-not (Test-Path -LiteralPath $canvasPNGPath -PathType Leaf)) {
    $failures.Add("CD0.2 PNG evidence is missing: $canvasPNGPath")
}

$ownerEvidence = Get-CD0Evidence "cd0-ownerdraw-probe.json" @("supported", "deferred")
if ($null -ne $ownerEvidence) {
    foreach ($route in @($ownerEvidence.evidence.routes)) {
        if (-not $route.routed -or -not $route.unbound -or -not $route.callbackRemoved) {
            $failures.Add("CD0.4 route '$($route.logicalParent)' lacks routing or teardown evidence")
        }
    }
    if (-not $ownerEvidence.evidence.parentNcDestroyObserved -or -not $ownerEvidence.evidence.destroyRouteCleanup) {
        $failures.Add("CD0.4 WM_NCDESTROY cleanup was not observed")
    }
}

# CD0.5 is a capability matrix, not a smoke test that can pass with an empty
# log. The Win32/LCL contract requires these four real callback paths on the
# pinned designer DLL; Progress custom draw is allowed to remain deferred.
$controlEvidence = Get-CD0Evidence "cd0-control-draw-probe.json" @("supported", "deferred")
if ($null -ne $controlEvidence) {
    $requiredControlProtocols = @(
        "ComboBox.DrawItem",
        "StringGrid.Prepare",
        "StringGrid.Draw",
        "TrackBar"
    )
    foreach ($requiredControl in $requiredControlProtocols) {
        $entry = @($controlEvidence.evidence.capabilities | Where-Object { $_.control -eq $requiredControl })
        if ($entry.Count -ne 1 -or $entry[0].status -ne "supported") {
            $failures.Add("CD0.5 required capability '$requiredControl' was not supported")
        }
    }
}

if ($failures.Count -ne 0) {
    $failures | ForEach-Object { Write-Error "[cd0] $_" }
    exit 1
}

$artifacts = @(Get-ChildItem -LiteralPath $ArtifactDir -File | Select-Object -ExpandProperty Name)
Write-Host "[cd0] PASS: artifacts=$($artifacts -join ', ')"
