<#
.SYNOPSIS
  FluxVCL 无头冒烟脚本（Phase 0.3，0.5 CI 复用）。

  启动目标 exe，用 Win32 API 验证：主窗口出现 -> 按钮存在 -> 模拟点击 ->
  按钮数字恰好 +1（点击生效）-> 目标专用断言 -> WM_CLOSE 后进程干净退出。

  注意：LCL 的 TLabel 无独立 HWND（自绘在父窗体表面），冒烟通过按钮文本
  观测"点击生效"，这也是 FluxVCL 需要记住的工程约束。PageControl 目标还会
  连续重排页序、校验 native identity，并严格验证 PrintWindow 截图像素。

.EXAMPLE
  .\scripts\build.ps1; .\scripts\smoke.ps1
  .\scripts\smoke.ps1 -Target basic
#>
param(
    [string]$Target = "basic",   # 目标应用：examples/<Target>
    [string]$Output = "bin",     # 与 build.ps1 的 Output 一致
    [string]$Screenshot = ""     # 非空则点击验证后保存截图到此路径（CI artifact）
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$exe = Join-Path $root "$Output\$Target.exe"
if (-not (Test-Path $exe)) { Write-Error "未找到 $exe，先执行 scripts/build.ps1"; exit 2 }

Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public static class W {
    [DllImport("user32.dll")]
    public static extern bool EnumWindows(WEnum cb, IntPtr lp);
    [DllImport("user32.dll")]
    public static extern bool EnumChildWindows(IntPtr parent, WEnum cb, IntPtr lp);
    public delegate bool WEnum(IntPtr h, IntPtr l);
    [DllImport("user32.dll", CharSet=CharSet.Unicode)]
    public static extern int GetWindowTextW(IntPtr h, StringBuilder sb, int max);
    [DllImport("user32.dll")]
    public static extern int GetWindowTextLengthW(IntPtr h);
    [DllImport("user32.dll", CharSet=CharSet.Unicode)]
    public static extern int GetClassNameW(IntPtr h, StringBuilder sb, int max);
    [DllImport("user32.dll")]
    public static extern bool IsWindowVisible(IntPtr h);
    [DllImport("user32.dll")]
    public static extern IntPtr GetParent(IntPtr h);
    [DllImport("user32.dll")]
    public static extern bool GetWindowRect(IntPtr h, out RECT rect);
    [DllImport("user32.dll")]
    public static extern bool SetForegroundWindow(IntPtr h);
    [DllImport("user32.dll")]
    public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint flags);
    [DllImport("user32.dll")]
    public static extern uint GetWindowThreadProcessId(IntPtr h, out uint processId);
    [DllImport("user32.dll", SetLastError=true)]
    public static extern IntPtr SendMessageTimeout(IntPtr h, uint m, IntPtr wp, IntPtr lp,
        uint flags, uint timeout, out UIntPtr result);
    [DllImport("user32.dll", EntryPoint="SendMessageTimeoutW", CharSet=CharSet.Unicode,
        SetLastError=true)]
    public static extern IntPtr SendMessageTimeoutText(IntPtr h, uint m, IntPtr wp, StringBuilder lp,
        uint flags, uint timeout, out UIntPtr result);
    [DllImport("user32.dll", SetLastError=true)]
    public static extern bool PostMessage(IntPtr h, uint m, IntPtr wp, IntPtr lp);
    [StructLayout(LayoutKind.Sequential)]
    public struct RECT { public int Left, Top, Right, Bottom; }
}
'@

function Send-WindowMessage {
    param(
        [IntPtr]$Handle,
        [uint32]$Message,
        [IntPtr]$WParam = [IntPtr]::Zero,
        [IntPtr]$LParam = [IntPtr]::Zero,
        [uint32]$TimeoutMilliseconds = 2000
    )

    [UIntPtr]$result = [UIntPtr]::Zero
    # SMTO_ABORTIFHUNG prevents an unresponsive GUI thread from hanging the smoke job.
    $sent = [W]::SendMessageTimeout(
        $Handle, $Message, $WParam, $LParam, 0x0002, $TimeoutMilliseconds, [ref]$result
    )
    if ($sent -eq [IntPtr]::Zero) {
        $errorCode = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
        throw "SendMessageTimeout failed/expired: hwnd=$Handle message=0x$($Message.ToString('X4')) error=$errorCode"
    }
    return $result.ToUInt64()
}

function Send-WindowTextMessage {
    param(
        [IntPtr]$Handle,
        [uint32]$Message,
        [IntPtr]$WParam,
        [Text.StringBuilder]$LParam,
        [uint32]$TimeoutMilliseconds = 2000
    )

    [UIntPtr]$result = [UIntPtr]::Zero
    $sent = [W]::SendMessageTimeoutText(
        $Handle, $Message, $WParam, $LParam, 0x0002, $TimeoutMilliseconds, [ref]$result
    )
    if ($sent -eq [IntPtr]::Zero) {
        $errorCode = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
        throw "SendMessageTimeout failed/expired: hwnd=$Handle message=0x$($Message.ToString('X4')) error=$errorCode"
    }
    return $result.ToUInt64()
}

function Test-BitmapContent {
    param([Drawing.Bitmap]$Bitmap)

    $bounds = [Drawing.Rectangle]::new(0, 0, $Bitmap.Width, $Bitmap.Height)
    $bitmapData = $null
    try {
        $bitmapData = $Bitmap.LockBits(
            $bounds,
            [Drawing.Imaging.ImageLockMode]::ReadOnly,
            [Drawing.Imaging.PixelFormat]::Format24bppRgb
        )
        $byteCount = [Math]::Abs($bitmapData.Stride) * $bitmapData.Height
        $pixels = [byte[]]::new($byteCount)
        [Runtime.InteropServices.Marshal]::Copy($bitmapData.Scan0, $pixels, 0, $byteCount)

        # Sample a dense grid. A real LCL window contains background, borders, and text;
        # an all-black/all-uniform PrintWindow result does not provide visual evidence.
        $xStep = [Math]::Max(1, [int][Math]::Floor($Bitmap.Width / 96.0))
        $yStep = [Math]::Max(1, [int][Math]::Floor($Bitmap.Height / 96.0))
        $sampleCount = 0
        $nonBlackCount = 0
        $minLuminance = 255
        $maxLuminance = 0
        for ($y = 0; $y -lt $Bitmap.Height; $y += $yStep) {
            $row = if ($bitmapData.Stride -ge 0) {
                $y * $bitmapData.Stride
            } else {
                ($Bitmap.Height - 1 - $y) * (-$bitmapData.Stride)
            }
            for ($x = 0; $x -lt $Bitmap.Width; $x += $xStep) {
                $offset = $row + ($x * 3)
                $blue = [int]$pixels[$offset]
                $green = [int]$pixels[$offset + 1]
                $red = [int]$pixels[$offset + 2]
                $luminance = [int](($red * 299 + $green * 587 + $blue * 114) / 1000)
                $sampleCount++
                if ($red -ne 0 -or $green -ne 0 -or $blue -ne 0) { $nonBlackCount++ }
                if ($luminance -lt $minLuminance) { $minLuminance = $luminance }
                if ($luminance -gt $maxLuminance) { $maxLuminance = $luminance }
            }
        }

        $minimumNonBlack = [Math]::Max(1, [int][Math]::Ceiling($sampleCount / 100.0))
        [PSCustomObject]@{
            Valid = $nonBlackCount -ge $minimumNonBlack -and ($maxLuminance - $minLuminance) -ge 8
            Samples = $sampleCount
            NonBlack = $nonBlackCount
            LuminanceRange = $maxLuminance - $minLuminance
        }
    } finally {
        if ($null -ne $bitmapData) { $Bitmap.UnlockBits($bitmapData) }
    }
}

function Save-WindowScreenshot {
    param(
        [IntPtr]$Handle,
        [string]$Path,
        [bool]$AllowScreenFallback = $true
    )

    Add-Type -AssemblyName System.Drawing
    $bitmap = $null
    $graphics = $null
    $hdc = [IntPtr]::Zero
    try {
        [W]::SetForegroundWindow($Handle) | Out-Null
        Start-Sleep -Milliseconds 200
        $rect = New-Object W+RECT
        if (-not [W]::GetWindowRect($Handle, [ref]$rect)) { throw "GetWindowRect failed" }
        $width = $rect.Right - $rect.Left
        $height = $rect.Bottom - $rect.Top
        if ($width -le 0 -or $height -le 0) { throw "invalid window bounds ${width}x${height}" }

        $bitmap = [Drawing.Bitmap]::new(
            $width, $height, [Drawing.Imaging.PixelFormat]::Format24bppRgb
        )
        $graphics = [Drawing.Graphics]::FromImage($bitmap)
        $printSucceeded = $false
        try {
            $hdc = $graphics.GetHdc()
            $printSucceeded = [W]::PrintWindow($Handle, $hdc, 0)
        } finally {
            if ($hdc -ne [IntPtr]::Zero) {
                $hdcToRelease = $hdc
                $hdc = [IntPtr]::Zero
                $graphics.ReleaseHdc($hdcToRelease)
            }
        }

        $captureMethod = "PrintWindow"
        $evidence = if ($printSucceeded) { Test-BitmapContent $bitmap } else { $null }
        if (-not $printSucceeded -or -not $evidence.Valid) {
            if (-not $AllowScreenFallback) {
                if (-not $printSucceeded) { throw "PrintWindow returned failure" }
                throw "PrintWindow bitmap has no visible content (samples=$($evidence.Samples) nonBlack=$($evidence.NonBlack) luminanceRange=$($evidence.LuminanceRange))"
            }
            # PrintWindow can report success while returning a black bitmap. Retry from
            # the target rectangle after foregrounding, then validate that result too.
            $graphics.CopyFromScreen($rect.Left, $rect.Top, 0, 0, $bitmap.Size)
            $captureMethod = "CopyFromScreen fallback"
            $evidence = Test-BitmapContent $bitmap
        }
        if (-not $evidence.Valid) {
            throw "captured bitmap has no visible content (samples=$($evidence.Samples) nonBlack=$($evidence.NonBlack) luminanceRange=$($evidence.LuminanceRange))"
        }

        $bitmap.Save($Path, [Drawing.Imaging.ImageFormat]::Png)
        return "$captureMethod; samples=$($evidence.Samples) nonBlack=$($evidence.NonBlack) luminanceRange=$($evidence.LuminanceRange)"
    } finally {
        if ($hdc -ne [IntPtr]::Zero -and $null -ne $graphics) {
            try { $graphics.ReleaseHdc($hdc) } catch { }
        }
        try {
            if ($null -ne $graphics) { $graphics.Dispose() }
        } finally {
            if ($null -ne $bitmap) { $bitmap.Dispose() }
        }
    }
}

function Get-PageControlSnapshot {
    param([IntPtr]$Window)

    $script:pageSnapshotTab = [IntPtr]::Zero
    $script:pageSnapshotEdits = New-Object System.Collections.Generic.List[object]
    $callback = [W+WEnum]{ param($h,$l)
        $className = New-Object System.Text.StringBuilder 256
        [W]::GetClassNameW($h, $className, 256) | Out-Null
        if ($className.ToString() -eq "SysTabControl32") {
            $script:pageSnapshotTab = $h
        }
        if ($className.ToString() -eq "Edit") {
            $text = New-Object System.Text.StringBuilder 256
            [W]::GetWindowTextW($h, $text, 256) | Out-Null
            $script:pageSnapshotEdits.Add([PSCustomObject]@{
                Handle = $h
                Parent = [W]::GetParent($h)
                Text = $text.ToString()
                Visible = [W]::IsWindowVisible($h)
            })
        }
        return $true
    }
    [W]::EnumChildWindows($Window, $callback, [IntPtr]::Zero) | Out-Null
    if ($script:pageSnapshotTab -eq [IntPtr]::Zero) {
        throw "[smoke] FAIL: native PageControl not found"
    }

    $edits = @($script:pageSnapshotEdits.ToArray())
    $identityParts = @($edits | Sort-Object Text | ForEach-Object {
        "$($_.Text):$($_.Handle.ToInt64()):$($_.Parent.ToInt64())"
    })
    [PSCustomObject]@{
        Tab = $script:pageSnapshotTab
        PageCount = [int](Send-WindowMessage $script:pageSnapshotTab 0x1304) # TCM_GETITEMCOUNT
        Selected = [int](Send-WindowMessage $script:pageSnapshotTab 0x130B)  # TCM_GETCURSEL
        Edits = $edits
        Identity = "$($script:pageSnapshotTab.ToInt64())|$($identityParts -join '|')"
    }
}

function Assert-PageControlSnapshot {
    param(
        [object]$Snapshot,
        [int]$ExpectedSelection,
        [string]$ExpectedIdentity = "",
        [string]$ExpectedVisibleText = "first-input"
    )

    $parents = @($Snapshot.Edits | ForEach-Object { $_.Parent.ToInt64() } | Sort-Object -Unique)
    $first = @($Snapshot.Edits | Where-Object { $_.Text -eq "first-input" })
    $second = @($Snapshot.Edits | Where-Object { $_.Text -eq "second-input" })
    if ($Snapshot.PageCount -ne 2 -or $Snapshot.Selected -ne $ExpectedSelection) {
        throw "[smoke] FAIL: PageControl pages=$($Snapshot.PageCount) selected=$($Snapshot.Selected) (want 2/$ExpectedSelection)"
    }
    if ($Snapshot.Edits.Count -ne 2 -or $parents.Count -ne 2 -or
        $parents -contains $Snapshot.Tab.ToInt64() -or $first.Count -ne 1 -or $second.Count -ne 1) {
        throw "[smoke] FAIL: page Edit identity invalid edits=$($Snapshot.Edits.Count) parents=$($parents -join ',')"
    }
    $visible = @($Snapshot.Edits | Where-Object { $_.Visible })
    if ($visible.Count -ne 1 -or $visible[0].Text -ne $ExpectedVisibleText) {
        throw "[smoke] FAIL: selected index $ExpectedSelection shows '$($visible.Text -join ',')' (want $ExpectedVisibleText)"
    }
    if ($ExpectedIdentity -and $Snapshot.Identity -ne $ExpectedIdentity) {
        throw "[smoke] FAIL: keyed reorder recreated/reparented PageControl children (before=$ExpectedIdentity after=$($Snapshot.Identity))"
    }
}

function Get-CounterButton {
    param([IntPtr]$Window)

    $script:counterButtons = New-Object System.Collections.Generic.List[IntPtr]
    $callback = [W+WEnum]{ param($h,$l)
        $className = New-Object System.Text.StringBuilder 256
        [W]::GetClassNameW($h, $className, 256) | Out-Null
        if ($className.ToString() -eq "Button") {
            $caption = New-Object System.Text.StringBuilder 256
            [W]::GetWindowTextW($h, $caption, 256) | Out-Null
            $counter = 0
            if ([int]::TryParse($caption.ToString(), [ref]$counter)) {
                $script:counterButtons.Add($h) | Out-Null
            }
        }
        return $true
    }
    [W]::EnumChildWindows($Window, $callback, [IntPtr]::Zero) | Out-Null
    if ($script:counterButtons.Count -ne 1) {
        throw "[smoke] FAIL: expected exactly one numeric counter Button, found $($script:counterButtons.Count)"
    }
    return $script:counterButtons[0]
}

$p = Start-Process -FilePath $exe -PassThru
Write-Host "[smoke] started pid=$($p.Id) exe=$exe"
$hwnd = [IntPtr]::Zero
try {

# 找当前进程的目标主窗口。Inspector 示例有第二个 "FluxVCL Inspector" 窗口，
# 必须排除它，避免 EnumWindows 顺序导致误选。
$cbTop = [W+WEnum]{ param($h,$l)
    if ([W]::IsWindowVisible($h)) {
        [uint32]$windowPid = 0
        [W]::GetWindowThreadProcessId($h, [ref]$windowPid) | Out-Null
        $sb = New-Object System.Text.StringBuilder 512
        [W]::GetWindowTextW($h, $sb, 512) | Out-Null
        if ($windowPid -eq $p.Id -and $sb.ToString() -like "FluxVCL *" -and
            $sb.ToString() -ne "FluxVCL Inspector") { $script:hwnd = $h }
    }
    return $true
}
for ($i = 0; $i -lt 30; $i++) {
    $script:hwnd = [IntPtr]::Zero
    [W]::EnumWindows($cbTop, [IntPtr]::Zero) | Out-Null
    if ($script:hwnd -ne [IntPtr]::Zero) { $hwnd = $script:hwnd; break }
    Start-Sleep -Milliseconds 400
}
if ($hwnd -eq [IntPtr]::Zero) { Write-Host "[smoke] FAIL: window not found"; exit 1 }
Write-Host "[smoke] window found hwnd=$hwnd"

# CheckBox/RadioButton 也使用 Win32 Button class；只接受唯一数字 Caption 的
# smoke counter，避免枚举顺序把表单控件误当作点击目标。
$btn = Get-CounterButton $hwnd

$b0 = New-Object System.Text.StringBuilder 256
[W]::GetWindowTextW($btn, $b0, 256) | Out-Null
Write-Host "[smoke] button before click: '$($b0.ToString())'"

$pageBaseline = $null
if ($Target -eq "page-control") {
    $pageBaseline = Get-PageControlSnapshot $hwnd
    Assert-PageControlSnapshot $pageBaseline 0
}

Send-WindowMessage $btn 0x00F5 | Out-Null   # BM_CLICK
Start-Sleep -Milliseconds 500

$b1 = New-Object System.Text.StringBuilder 256
[W]::GetWindowTextW($btn, $b1, 256) | Out-Null
Write-Host "[smoke] button after click: '$($b1.ToString())'"
# 唯一 Button 的数字 Caption 由 State 驱动；严格验证点击后恰好 +1。
$beforeClick = 0
$afterClick = 0
if ([int]::TryParse($b0.ToString(), [ref]$beforeClick) -and
    [int]::TryParse($b1.ToString(), [ref]$afterClick) -and
    $afterClick -eq $beforeClick + 1) { Write-Host "[smoke] PASS: click handled" }
else { Write-Host "[smoke] FAIL: click not handled"; exit 1 }

if ($Target -eq "page-control") {
    # 三次连续切换同时反转 keyed 页序。选中数字索引 0->1->0->1，业务页面
    # 始终保持 first；PageControl、两个 TabSheet parent 与 Edit HWND 必须全程不变。
    $identity = $pageBaseline.Identity
    $snapshot = Get-PageControlSnapshot $hwnd
    Assert-PageControlSnapshot $snapshot 1 $identity
    foreach ($expected in @(0, 1)) {
        Send-WindowMessage $btn 0x00F5 | Out-Null
        Start-Sleep -Milliseconds 400
        $snapshot = Get-PageControlSnapshot $hwnd
        Assert-PageControlSnapshot $snapshot $expected $identity
    }
    $b3Page = New-Object System.Text.StringBuilder 256
    [W]::GetWindowTextW($btn, $b3Page, 256) | Out-Null
    if ($b3Page.ToString() -ne "3") {
        throw "[smoke] FAIL: PageControl continuous switch count='$($b3Page.ToString())' (want 3)"
    }

    # Drive the real SysTabControl32 keyboard path. Each selection must raise LCL
    # OnChange -> Flux callback, observed as one additional numeric Button update.
    Send-WindowMessage $snapshot.Tab 0x0100 ([IntPtr]0x25) | Out-Null # WM_KEYDOWN/VK_LEFT
    Send-WindowMessage $snapshot.Tab 0x0101 ([IntPtr]0x25) | Out-Null # WM_KEYUP/VK_LEFT
    Start-Sleep -Milliseconds 400
    $snapshot = Get-PageControlSnapshot $hwnd
    Assert-PageControlSnapshot $snapshot 0 $identity "second-input"
    $b4Page = New-Object System.Text.StringBuilder 256
    [W]::GetWindowTextW($btn, $b4Page, 256) | Out-Null
    if ($b4Page.ToString() -ne "4") {
        throw "[smoke] FAIL: native PageControl callback count='$($b4Page.ToString())' (want 4)"
    }

    Send-WindowMessage $snapshot.Tab 0x0100 ([IntPtr]0x27) | Out-Null # WM_KEYDOWN/VK_RIGHT
    Send-WindowMessage $snapshot.Tab 0x0101 ([IntPtr]0x27) | Out-Null # WM_KEYUP/VK_RIGHT
    Start-Sleep -Milliseconds 400
    $snapshot = Get-PageControlSnapshot $hwnd
    Assert-PageControlSnapshot $snapshot 1 $identity "first-input"
    $b5Page = New-Object System.Text.StringBuilder 256
    [W]::GetWindowTextW($btn, $b5Page, 256) | Out-Null
    if ($b5Page.ToString() -ne "5") {
        throw "[smoke] FAIL: native PageControl callback count='$($b5Page.ToString())' (want 5)"
    }
    Write-Host "[smoke] PASS: PageControl reorder identities and native selection callbacks verified"
}

if ($Target -eq "inspector") {
    # 找同进程的 Inspector 窗口与 Memo，继续点到 3，验证事件和两类重建可见。
    $script:inspectorHwnd = [IntPtr]::Zero
    $cbInspector = [W+WEnum]{ param($h,$l)
        if ([W]::IsWindowVisible($h)) {
            [uint32]$windowPid = 0
            [W]::GetWindowThreadProcessId($h, [ref]$windowPid) | Out-Null
            $sb = New-Object System.Text.StringBuilder 512
            [W]::GetWindowTextW($h, $sb, 512) | Out-Null
            if ($windowPid -eq $p.Id -and $sb.ToString() -eq "FluxVCL Inspector") {
                $script:inspectorHwnd = $h
            }
        }
        return $true
    }
    [W]::EnumWindows($cbInspector, [IntPtr]::Zero) | Out-Null
    if ($script:inspectorHwnd -eq [IntPtr]::Zero) {
        Write-Host "[smoke] FAIL: Inspector window not found"; exit 1
    }

    for ($click = 0; $click -lt 2; $click++) {
        $btn = Get-CounterButton $hwnd
        Send-WindowMessage $btn 0x00F5 | Out-Null
        Start-Sleep -Milliseconds 300
    }
    $b3 = New-Object System.Text.StringBuilder 256
    [W]::GetWindowTextW($btn, $b3, 256) | Out-Null
    if ($b3.ToString() -ne "3") {
        Write-Host "[smoke] FAIL: Inspector demo did not reach rebuild state (button='$($b3.ToString())')"; exit 1
    }

    $script:memo = [IntPtr]::Zero
    $cbMemo = [W+WEnum]{ param($h,$l)
        $cs = New-Object System.Text.StringBuilder 256
        [W]::GetClassNameW($h, $cs, 256) | Out-Null
        if ($cs.ToString() -eq "Edit") { $script:memo = $h }
        return $true
    }
    [W]::EnumChildWindows($script:inspectorHwnd, $cbMemo, [IntPtr]::Zero) | Out-Null
    if ($script:memo -eq [IntPtr]::Zero) { Write-Host "[smoke] FAIL: Inspector Memo not found"; exit 1 }
    $content = ""
    for ($i = 0; $i -lt 20; $i++) {
        $memoLength = [int](Send-WindowMessage $script:memo 0x000E)
        $memoText = New-Object System.Text.StringBuilder ($memoLength + 1)
        Send-WindowTextMessage $script:memo 0x000D ([IntPtr]($memoLength + 1)) $memoText | Out-Null
        $content = $memoText.ToString()
        if ($content -like "*source=Button#counter*" -and
            $content -like "*type-mismatch*" -and $content -like "*key-mismatch*") { break }
        Start-Sleep -Milliseconds 100
    }
    if ($content -notlike "*source=Button#counter*" -or
        $content -notlike "*type-mismatch*" -or
        $content -notlike "*key-mismatch*" -or
        $content -notmatch "parent=[1-9][0-9]* allocated=true") {
        $hasClick = $content -like "*source=Button#counter*"
        $hasType = $content -like "*type-mismatch*"
        $hasKey = $content -like "*key-mismatch*"
        $hasParent = $content -match "parent=[1-9][0-9]* allocated=true"
        Write-Host "[smoke] Inspector evidence event=$hasClick type=$hasType key=$hasKey parent=$hasParent chars=$($content.Length)"
        Write-Host "[smoke] FAIL: Inspector content missing event/rebuild evidence"; exit 1
    }
    Write-Host "[smoke] PASS: Inspector event and rebuild views verified"

    if ($Screenshot) {
        try {
            $captureEvidence = Save-WindowScreenshot `
                -Handle $script:inspectorHwnd `
                -Path $Screenshot `
                -AllowScreenFallback $true
            Write-Host "[smoke] screenshot saved: $Screenshot ($captureEvidence)"
        } catch {
            Write-Host "[smoke] FAIL: Inspector screenshot is not valid: $_"
            exit 1
        }
    }

    # 关闭工具窗后目标 App 仍应可交互，证明关闭只影响 Inspector。
    [W]::PostMessage($script:inspectorHwnd, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
    Start-Sleep -Milliseconds 300
    $btn = Get-CounterButton $hwnd
    Send-WindowMessage $btn 0x00F5 | Out-Null
    Start-Sleep -Milliseconds 300
    $b4 = New-Object System.Text.StringBuilder 256
    [W]::GetWindowTextW($btn, $b4, 256) | Out-Null
    if ($b4.ToString() -ne "4") {
        Write-Host "[smoke] FAIL: target stopped after Inspector close"; exit 1
    }
    Write-Host "[smoke] PASS: target remains interactive after Inspector close"
} elseif ($Screenshot) {
    # 只截目标窗口；PrintWindow 的返回值与像素内容都必须形成有效证据。
    try {
        $captureEvidence = Save-WindowScreenshot `
            -Handle $hwnd `
            -Path $Screenshot `
            -AllowScreenFallback:($Target -ne "page-control")
        Write-Host "[smoke] screenshot saved: $Screenshot ($captureEvidence)"
    } catch {
        Write-Host "[smoke] FAIL: screenshot is not valid: $_"
        exit 1
    }
}

if ($Screenshot) {
    if (-not (Test-Path -LiteralPath $Screenshot -PathType Leaf) -or
        (Get-Item -LiteralPath $Screenshot).Length -le 0) {
        Write-Host "[smoke] FAIL: screenshot artifact is missing or empty: $Screenshot"
        exit 1
    }
}

[W]::PostMessage($hwnd, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null   # WM_CLOSE
if (-not $p.WaitForExit(8000)) {
    Write-Host "[smoke] FAIL: process did not exit"
    Stop-Process -Id $p.Id -Force
    exit 1
}
if ($p.ExitCode -ne 0) {
    Write-Host "[smoke] FAIL: process exited with code $($p.ExitCode)"
    exit 1
}
Write-Host "[smoke] PASS: process exited cleanly (code 0)"

Write-Host "[smoke] RESULT: PASS"
} finally {
    # Every early validation exit must release the process started by this invocation.
    try {
        if (-not $p.HasExited) {
            if ($hwnd -ne [IntPtr]::Zero) {
                [W]::PostMessage($hwnd, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
            }
            if (-not $p.WaitForExit(2000)) {
                Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
                $p.WaitForExit(2000) | Out-Null
            }
        }
    } catch {
        Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
    } finally {
        $p.Dispose()
    }
}
