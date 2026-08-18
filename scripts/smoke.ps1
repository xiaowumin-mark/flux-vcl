<#
.SYNOPSIS
  FluxVCL 无头冒烟脚本（Phase 0.3，0.5 CI 复用）。

  启动目标 exe，用 Win32 API 验证主窗口、真实业务交互、可选截图和干净退出。
  既有示例继续使用唯一数字按钮；7GUIs 使用各自的业务级专用断言，不向产品
  界面插入测试专用控件。

  注意：LCL 的 TLabel 无独立 HWND（自绘在父窗体表面），冒烟通过按钮文本
  观测"点击生效"，这也是 FluxVCL 需要记住的工程约束。PageControl 目标还会
  连续重排页序、校验 native identity，并严格验证 PrintWindow 截图像素。

.EXAMPLE
  .\scripts\build.ps1; .\scripts\smoke.ps1
  .\scripts\smoke.ps1 -Target basic
  .\scripts\smoke.ps1 -Target basic -ExePath C:\Apps\FluxVCL\basic.exe
#>
param(
    [string]$Target = "basic",   # 目标应用：examples/<Target>
    [string]$Output = "bin",     # 与 build.ps1 的 Output 一致
    [string]$ExePath = "",       # 非空时直接验证指定 EXE（安装包 smoke）
    [string]$Screenshot = ""     # 非空则点击验证后保存截图到此路径（CI artifact）
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$exe = if ($ExePath) { $ExePath } else { Join-Path $root "$Output\$Target.exe" }
if (-not (Test-Path $exe)) { Write-Error "未找到 $exe，先执行 scripts/build.ps1"; exit 2 }
$exe = (Resolve-Path -LiteralPath $exe).Path

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
    public static extern bool IsWindowEnabled(IntPtr h);
    [DllImport("user32.dll")]
    public static extern IntPtr GetParent(IntPtr h);
    [DllImport("user32.dll")]
    public static extern bool GetWindowRect(IntPtr h, out RECT rect);
    [DllImport("user32.dll")]
    public static extern bool GetClientRect(IntPtr h, out RECT rect);
    [DllImport("user32.dll")]
    public static extern bool SetForegroundWindow(IntPtr h);
    [DllImport("user32.dll")]
    public static extern IntPtr SetFocus(IntPtr h);
    [DllImport("user32.dll")]
    public static extern bool AttachThreadInput(uint idAttach, uint idAttachTo, bool attach);
    [DllImport("kernel32.dll")]
    public static extern uint GetCurrentThreadId();
    [DllImport("user32.dll")]
    public static extern uint GetDpiForWindow(IntPtr h);
    [DllImport("user32.dll")]
    public static extern bool SetCursorPos(int x, int y);
    [DllImport("user32.dll")]
    public static extern void mouse_event(uint flags, uint dx, uint dy, uint data, UIntPtr extraInfo);
    [DllImport("user32.dll")]
    public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint flags);
    [DllImport("user32.dll")]
    public static extern uint GetWindowThreadProcessId(IntPtr h, out uint processId);
    [DllImport("user32.dll", SetLastError=true)]
    public static extern bool GetGUIThreadInfo(uint idThread, ref GUITHREADINFO info);
    [DllImport("user32.dll", SetLastError=true)]
    public static extern IntPtr SendMessageTimeout(IntPtr h, uint m, IntPtr wp, IntPtr lp,
        uint flags, uint timeout, out UIntPtr result);
    [DllImport("user32.dll", EntryPoint="SendMessageW", SetLastError=true)]
    public static extern IntPtr SendMessageSelection(IntPtr h, uint m, out uint start, out uint end);
    [DllImport("user32.dll", EntryPoint="SendMessageTimeoutW", CharSet=CharSet.Unicode,
        SetLastError=true)]
    public static extern IntPtr SendMessageTimeoutText(IntPtr h, uint m, IntPtr wp, StringBuilder lp,
        uint flags, uint timeout, out UIntPtr result);
    [DllImport("user32.dll", SetLastError=true)]
    public static extern bool PostMessage(IntPtr h, uint m, IntPtr wp, IntPtr lp);
    [StructLayout(LayoutKind.Sequential)]
    public struct RECT { public int Left, Top, Right, Bottom; }
    [StructLayout(LayoutKind.Sequential)]
    public struct GUITHREADINFO {
        public int cbSize;
        public uint flags;
        public IntPtr hwndActive;
        public IntPtr hwndFocus;
        public IntPtr hwndCapture;
        public IntPtr hwndMenuOwner;
        public IntPtr hwndMoveSize;
        public IntPtr hwndCaret;
        public RECT rcCaret;
    }
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

function Get-ChildWindowSnapshot {
    param([IntPtr]$Window)

    $script:smokeChildWindows = New-Object System.Collections.Generic.List[object]
    $callback = [W+WEnum]{ param($h,$l)
        $className = New-Object System.Text.StringBuilder 256
        [W]::GetClassNameW($h, $className, 256) | Out-Null
        $rect = New-Object W+RECT
        [W]::GetWindowRect($h, [ref]$rect) | Out-Null
        $script:smokeChildWindows.Add([PSCustomObject]@{
            Handle = $h
            Class = $className.ToString()
            Text = ""
            Rect = $rect
            Visible = [W]::IsWindowVisible($h)
            Enabled = [W]::IsWindowEnabled($h)
        }) | Out-Null
        return $true
    }
    [W]::EnumChildWindows($Window, $callback, [IntPtr]::Zero) | Out-Null
    $windows = @($script:smokeChildWindows.ToArray())
    foreach ($child in $windows) {
        # GetWindowTextW cannot retrieve another process's Edit contents reliably.
        # WM_GETTEXT executes in the owning GUI thread and reflects controlled patches.
        $child.Text = Get-ChildWindowText $child.Handle
    }
    return $windows
}

function Get-UniqueChildWindow {
    param(
        [IntPtr]$Window,
        [string]$ClassName,
        [string]$Text = ""
    )

    $matches = @(Get-ChildWindowSnapshot $Window | Where-Object {
        $_.Class -eq $ClassName -and (-not $Text -or $_.Text -eq $Text)
    })
    if ($matches.Count -ne 1) {
        throw "[smoke] FAIL: expected one child class='$ClassName' text='$Text', found $($matches.Count)"
    }
    return $matches[0]
}

function Get-ChildWindowText {
    param([IntPtr]$Handle)

    $capacity = 4096
    $text = New-Object System.Text.StringBuilder $capacity
    Send-WindowTextMessage $Handle 0x000D ([IntPtr]$capacity) $text | Out-Null # WM_GETTEXT
    return $text.ToString()
}

function Set-ChildFocus {
    param(
        [IntPtr]$Window,
        [IntPtr]$Handle
    )

    [uint32]$targetProcess = 0
    $targetThread = [W]::GetWindowThreadProcessId($Handle, [ref]$targetProcess)
    $currentThread = [W]::GetCurrentThreadId()
    $attached = $false
    if ($currentThread -ne $targetThread) {
        $attached = [W]::AttachThreadInput($currentThread, $targetThread, $true)
    }
    try {
        [W]::SetForegroundWindow($Window) | Out-Null
        [W]::SetFocus($Handle) | Out-Null
    } finally {
        if ($attached) {
            [W]::AttachThreadInput($currentThread, $targetThread, $false) | Out-Null
        }
    }
}

function Get-FocusedWindow {
    param([IntPtr]$Handle)

    [uint32]$targetProcess = 0
    $targetThread = [W]::GetWindowThreadProcessId($Handle, [ref]$targetProcess)
    $info = New-Object W+GUITHREADINFO
    $info.cbSize = [Runtime.InteropServices.Marshal]::SizeOf($info)
    if (-not [W]::GetGUIThreadInfo($targetThread, [ref]$info)) {
        $errorCode = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
        throw "[smoke] FAIL: GetGUIThreadInfo failed for hwnd=$Handle error=$errorCode"
    }
    return $info.hwndFocus
}

function Assert-EditFocusAndCaret {
    param(
        [object]$Edit,
        [string]$Context
    )

    $focused = Get-FocusedWindow $Edit.Handle
    if ($focused -ne $Edit.Handle) {
        throw "[smoke] FAIL: $Context moved focus from hwnd=$($Edit.Handle) to hwnd=$focused"
    }
    [uint32]$start = 0
    [uint32]$end = 0
    [W]::SendMessageSelection($Edit.Handle, 0x00B0, [ref]$start, [ref]$end) | Out-Null # EM_GETSEL
    $length = (Get-ChildWindowText $Edit.Handle).Length
    if ($start -ne $length -or $end -ne $length) {
        throw "[smoke] FAIL: $Context moved the caret/selection: start=$start end=$end textLength=$length"
    }
}

function Replace-EditTextWithASCII {
    param(
        [IntPtr]$Window,
        [object]$Edit,
        [string]$Text
    )

    $x = [int](($Edit.Rect.Left + $Edit.Rect.Right) / 2)
    $y = [int](($Edit.Rect.Top + $Edit.Rect.Bottom) / 2)
    Invoke-ScreenClick $Window $x $y
    Start-Sleep -Milliseconds 100
    Set-ChildFocus $Window $Edit.Handle
    $localX = [int](($Edit.Rect.Right - $Edit.Rect.Left) / 2)
    $localY = [int](($Edit.Rect.Bottom - $Edit.Rect.Top) / 2)
    $point = [IntPtr](($localY -shl 16) -bor ($localX -band 0xFFFF))
    Send-WindowMessage $Edit.Handle 0x0201 ([IntPtr]1) $point | Out-Null # WM_LBUTTONDOWN
    Send-WindowMessage $Edit.Handle 0x0202 ([IntPtr]0) $point | Out-Null # WM_LBUTTONUP
    Send-WindowMessage $Edit.Handle 0x00B1 ([IntPtr]0) ([IntPtr](-1)) | Out-Null # EM_SETSEL all
    foreach ($character in $Text.ToCharArray()) {
        $code = [int][char]$character
        if ($character -ge 'a' -and $character -le 'z') {
            $code = [int][char]([char]::ToUpperInvariant($character))
        } elseif ($character -eq '-') {
            $code = 0xBD # VK_OEM_MINUS
        } elseif ($character -lt '0' -or $character -gt '9') {
            throw "Replace-EditTextWithASCII does not support '$character'"
        }
        Send-WindowMessage $Edit.Handle 0x0102 ([IntPtr]$code) | Out-Null # WM_CHAR
    }
}

function Invoke-ScreenClick {
    param(
        [IntPtr]$Window,
        [int]$X,
        [int]$Y
    )

    [W]::SetForegroundWindow($Window) | Out-Null
    $windowRect = New-Object W+RECT
    if (-not [W]::GetWindowRect($Window, [ref]$windowRect)) {
        throw "[smoke] FAIL: GetWindowRect failed before pointer input"
    }
    $titleX = [int](($windowRect.Left + $windowRect.Right) / 2)
    $titleY = $windowRect.Top + 10
    if (-not [W]::SetCursorPos($titleX, $titleY)) {
        throw "[smoke] FAIL: SetCursorPos failed for title activation"
    }
    [W]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 70
    [W]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 200
    if (-not [W]::SetCursorPos($X, $Y)) {
        throw "[smoke] FAIL: SetCursorPos failed for ($X,$Y)"
    }
    Start-Sleep -Milliseconds 100
    [W]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero) # MOUSEEVENTF_LEFTDOWN
    Start-Sleep -Milliseconds 70
    [W]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero) # MOUSEEVENTF_LEFTUP
}

function Assert-CounterInteraction {
    param([IntPtr]$Window)

    $button = Get-UniqueChildWindow $Window "Button" "Count"
    $initial = Get-ChildWindowText $Window
    if ($initial -notlike "*7GUIs Counter (0)") {
        throw "[smoke] FAIL: Counter initial title='$initial' (want count 0)"
    }
    foreach ($expected in @(1, 2)) {
        Send-WindowMessage $button.Handle 0x00F5 | Out-Null # BM_CLICK
        Start-Sleep -Milliseconds 250
        $title = Get-ChildWindowText $Window
        if ($title -notlike "*7GUIs Counter ($expected)") {
            throw "[smoke] FAIL: Counter title='$title' after click (want count $expected)"
        }
    }
    Write-Host "[smoke] PASS: Counter State update 0 -> 1 -> 2 verified"
}

function Assert-TemperatureInteraction {
    param([IntPtr]$Window)

    $edits = @(Get-ChildWindowSnapshot $Window |
        Where-Object { $_.Class -eq "Edit" -and $_.Visible } |
        Sort-Object { $_.Rect.Left })
    if ($edits.Count -ne 2) {
        throw "[smoke] FAIL: Temperature Converter expected 2 edits, found $($edits.Count)"
    }
    Replace-EditTextWithASCII $Window $edits[0] "100"
    Start-Sleep -Milliseconds 350
    $celsius = Get-ChildWindowText $edits[0].Handle
    $fahrenheit = Get-ChildWindowText $edits[1].Handle
    if ($celsius -ne "100" -or $fahrenheit -ne "212") {
        throw "[smoke] FAIL: Temperature conversion produced '$celsius' C / '$fahrenheit' F (want 100/212)"
    }
    Assert-EditFocusAndCaret $edits[0] "valid Celsius rerender"
    Replace-EditTextWithASCII $Window $edits[0] "x"
    Start-Sleep -Milliseconds 250
    if ((Get-ChildWindowText $edits[1].Handle) -ne "212") {
        throw "[smoke] FAIL: invalid Celsius input overwrote the last valid Fahrenheit value"
    }
    Assert-EditFocusAndCaret $edits[0] "invalid Celsius rerender"
    Write-Host "[smoke] PASS: Temperature conversion, invalid input, focus, and caret verified"
}

function Assert-FlightBookerInteraction {
    param([IntPtr]$Window)

    $combos = @(Get-ChildWindowSnapshot $Window | Where-Object { $_.Class -like "LCLComboBox*" })
    if ($combos.Count -ne 1) {
        throw "[smoke] FAIL: Flight Booker expected one LCL ComboBox, found $($combos.Count)"
    }
    $combo = $combos[0]
    $book = Get-UniqueChildWindow $Window "Button" "Book"
    $edits = @(Get-ChildWindowSnapshot $Window |
        Where-Object { $_.Class -eq "Edit" -and $_.Visible -and $_.Text -match '^\d{4}-\d{2}-\d{2}$' } |
        Sort-Object { $_.Rect.Top })
    if ($edits.Count -ne 2 -or -not $edits[0].Enabled -or $edits[1].Enabled -or -not $book.Enabled) {
        throw "[smoke] FAIL: Flight Booker initial one-way enabled state is invalid"
    }

    $comboX = [int](($combo.Rect.Left + $combo.Rect.Right) / 2)
    $comboY = [int](($combo.Rect.Top + $combo.Rect.Bottom) / 2)
    Invoke-ScreenClick $Window $comboX $comboY
    Set-ChildFocus $Window $combo.Handle
    Send-WindowMessage $combo.Handle 0x0100 ([IntPtr]0x28) | Out-Null # WM_KEYDOWN/VK_DOWN
    Send-WindowMessage $combo.Handle 0x0101 ([IntPtr]0x28) | Out-Null # WM_KEYUP/VK_DOWN
    Send-WindowMessage $combo.Handle 0x0100 ([IntPtr]0x0D) | Out-Null # WM_KEYDOWN/VK_RETURN
    Send-WindowMessage $combo.Handle 0x0101 ([IntPtr]0x0D) | Out-Null # WM_KEYUP/VK_RETURN
    Start-Sleep -Milliseconds 350
    $edits = @(Get-ChildWindowSnapshot $Window |
        Where-Object { $_.Class -eq "Edit" -and $_.Visible -and $_.Text -match '^\d{4}-\d{2}-\d{2}$' } |
        Sort-Object { $_.Rect.Top })
    $book = Get-UniqueChildWindow $Window "Button" "Book"
    if ($edits.Count -ne 2 -or -not $edits[1].Enabled -or -not $book.Enabled) {
        $comboIndex = [int](Send-WindowMessage $combo.Handle 0x0147) # CB_GETCURSEL
        $editStates = @($edits | ForEach-Object { "$($_.Text):$($_.Enabled)" }) -join ','
        throw "[smoke] FAIL: selecting return flight did not enable its date field and Book; combo=$comboIndex edits=$editStates book=$($book.Enabled)"
    }

    Replace-EditTextWithASCII $Window $edits[0] "x"
    Start-Sleep -Milliseconds 300
    $book = Get-UniqueChildWindow $Window "Button" "Book"
    if ($book.Enabled) {
        throw "[smoke] FAIL: invalid outbound date did not disable Book"
    }
    Write-Host "[smoke] PASS: Flight type, validation, and controlled Enabled behavior verified"
}

function Assert-TimerInteraction {
    param([IntPtr]$Window)

    $track = Get-UniqueChildWindow $Window "msctls_trackbar32"
    $progress = Get-UniqueChildWindow $Window "msctls_progress32"
    $reset = Get-UniqueChildWindow $Window "Button" "Reset"

    # Home is a real TrackBar keyboard action. Merely observing TBM_GETPOS would be a
    # false positive because the native control can move without Flux State writeback.
    # The controlled duration becomes 0.1 s only when OnValueChange runs, making the
    # ProgressBar converge to 100 within this short deadline.
    Send-WindowMessage $track.Handle 0x0100 ([IntPtr]0x24) | Out-Null # WM_KEYDOWN/VK_HOME
    Send-WindowMessage $track.Handle 0x0101 ([IntPtr]0x24) | Out-Null # WM_KEYUP/VK_HOME
    Start-Sleep -Milliseconds 350
    $position = [int](Send-WindowMessage $track.Handle 0x0400) # TBM_GETPOS
    $completed = [int](Send-WindowMessage $progress.Handle 0x0408) # PBM_GETPOS
    if ($position -ne 1 -or $completed -ne 100) {
        throw "[smoke] FAIL: Timer Slider callback did not control duration: position=$position progress=$completed (want 1/100)"
    }

    Send-WindowMessage $reset.Handle 0x00F5 | Out-Null # BM_CLICK
    $resetProgress = [int](Send-WindowMessage $progress.Handle 0x0408)
    if ($resetProgress -gt 10) {
        throw "[smoke] FAIL: Timer Reset did not clear progress immediately: $resetProgress"
    }
    Start-Sleep -Milliseconds 300
    $replayed = [int](Send-WindowMessage $progress.Handle 0x0408)
    if ($replayed -ne 100) {
        throw "[smoke] FAIL: Timer did not complete again after Reset: $resetProgress->$replayed"
    }
    Write-Host "[smoke] PASS: Timer Slider State callback, Reset, and animation verified"
}

function Assert-GridInteraction {
    param(
        [IntPtr]$Window,
        [string]$GridTarget
    )

    $grid = Get-UniqueChildWindow $Window "Window"
    $dpi = [int][W]::GetDpiForWindow($grid.Handle)
    if ($dpi -le 0) { $dpi = 96 }
    $scale = $dpi / 96.0

    if ($GridTarget -eq "7guis-crud") {
        $x = $grid.Rect.Left + [int][Math]::Round(75 * $scale)
        $y = $grid.Rect.Top + [int][Math]::Round(60 * $scale)
        $expected = @("Max", "Mustermann")
    } else {
        $x = $grid.Rect.Left + [int][Math]::Round(102 * $scale)
        $y = $grid.Rect.Top + [int][Math]::Round(36 * $scale)
        $expected = @("=A1+2")
    }
    Write-Host "[smoke] Grid input target hwnd=$($grid.Handle) dpi=$dpi point=$x,$y"

    $observed = @()
    for ($attempt = 0; $attempt -lt 3; $attempt++) {
        Invoke-ScreenClick $Window $x $y
        Start-Sleep -Milliseconds 500
        Send-WindowMessage $Window 0x0000 | Out-Null # WM_NULL: drain queued UI work before reading HWND text
        $observed = @(Get-ChildWindowSnapshot $Window |
            Where-Object { $_.Class -eq "Edit" } |
            ForEach-Object { $_.Text })
        $missing = @($expected | Where-Object { $observed -notcontains $_ })
        if ($missing.Count -eq 0) { break }
    }
    $missing = @($expected | Where-Object { $observed -notcontains $_ })
    if ($missing.Count -ne 0) {
        throw "[smoke] FAIL: $GridTarget native Grid selection did not update controlled inputs; texts='$($observed -join '|')'"
    }
    if ($GridTarget -eq "7guis-cells") {
        [W]::SetFocus($grid.Handle) | Out-Null
        Send-WindowMessage $grid.Handle 0x0100 ([IntPtr]0x71) | Out-Null # WM_KEYDOWN/VK_F2
        Send-WindowMessage $grid.Handle 0x0101 ([IntPtr]0x71) | Out-Null # WM_KEYUP/VK_F2
        Start-Sleep -Milliseconds 200
        $children = @(Get-ChildWindowSnapshot $Window)
        $formula = @($children | Where-Object { $_.Class -eq "Edit" -and $_.Text -eq "=A1+2" })
        $inplace = @($children | Where-Object { $_.Class -eq "Edit" -and $_.Text -eq "3" })
        if ($formula.Count -ne 1 -or $inplace.Count -ne 1) {
            throw "[smoke] FAIL: Cells could not identify formula/in-place editors after B1 selection"
        }

        Send-WindowMessage $inplace[0].Handle 0x00B1 ([IntPtr]0) ([IntPtr](-1)) | Out-Null # EM_SETSEL all
        Send-WindowMessage $inplace[0].Handle 0x0102 ([IntPtr]0x37) | Out-Null # WM_CHAR '7'
        Start-Sleep -Milliseconds 150
        $formulaX = [int](($formula[0].Rect.Left + $formula[0].Rect.Right) / 2)
        $formulaY = [int](($formula[0].Rect.Top + $formula[0].Rect.Bottom) / 2)
        Invoke-ScreenClick $Window $formulaX $formulaY # focus loss commits the in-place edit
        Start-Sleep -Milliseconds 400
        $formulaText = Get-ChildWindowText $formula[0].Handle
        if ($formulaText -ne "7") {
            throw "[smoke] FAIL: Cells native Grid edit did not reach controlled formula State: '$formulaText'"
        }
        Write-Host "[smoke] PASS: Cells native TStringGrid selection and edit callbacks verified"
        return
    }
    Write-Host "[smoke] PASS: $GridTarget native TStringGrid selection callback verified"
}

function Measure-CentralBitmapDifference {
    param(
        [string]$BeforePath,
        [string]$AfterPath
    )

    Add-Type -AssemblyName System.Drawing
    $before = $null
    $after = $null
    try {
        $before = [Drawing.Bitmap]::new($BeforePath)
        $after = [Drawing.Bitmap]::new($AfterPath)
        if ($before.Width -ne $after.Width -or $before.Height -ne $after.Height) {
            throw "PaintBox screenshots changed dimensions"
        }
        $left = [int]($before.Width * 0.2)
        $right = [int]($before.Width * 0.8)
        $top = [int]($before.Height * 0.2)
        $bottom = [int]($before.Height * 0.8)
        $different = 0
        for ($y = $top; $y -lt $bottom; $y++) {
            for ($x = $left; $x -lt $right; $x++) {
                if ($before.GetPixel($x, $y).ToArgb() -ne $after.GetPixel($x, $y).ToArgb()) {
                    $different++
                }
            }
        }
        return $different
    } finally {
        if ($null -ne $before) { $before.Dispose() }
        if ($null -ne $after) { $after.Dispose() }
    }
}

function Assert-PaintInteraction {
    param([IntPtr]$Window)

    $undo = Get-UniqueChildWindow $Window "Button" "Undo"
    $redo = Get-UniqueChildWindow $Window "Button" "Redo"
    if ($undo.Enabled) {
        throw "[smoke] FAIL: Circle Drawer Undo unexpectedly enabled before drawing"
    }
    if ($redo.Enabled) {
        throw "[smoke] FAIL: Circle Drawer Redo unexpectedly enabled before drawing"
    }

    $beforePath = Join-Path ([IO.Path]::GetTempPath()) "flux-vcl-paint-$([Guid]::NewGuid()).before.png"
    $afterPath = Join-Path ([IO.Path]::GetTempPath()) "flux-vcl-paint-$([Guid]::NewGuid()).after.png"
    try {
        Save-WindowScreenshot -Handle $Window -Path $beforePath -AllowScreenFallback $true | Out-Null
        $client = New-Object W+RECT
        if (-not [W]::GetClientRect($Window, [ref]$client)) {
            throw "[smoke] FAIL: Circle Drawer GetClientRect failed"
        }
        $x = [int](($client.Right - $client.Left) / 2)
        $y = [int](($client.Bottom - $client.Top) / 2)
        $point = [IntPtr](($y -shl 16) -bor ($x -band 0xFFFF))
        Send-WindowMessage $Window 0x0201 ([IntPtr]1) $point | Out-Null # WM_LBUTTONDOWN
        Send-WindowMessage $Window 0x0202 ([IntPtr]0) $point | Out-Null # WM_LBUTTONUP
        Start-Sleep -Milliseconds 500

        $undo = Get-UniqueChildWindow $Window "Button" "Undo"
        $redo = Get-UniqueChildWindow $Window "Button" "Redo"
        if (-not $undo.Enabled) {
            throw "[smoke] FAIL: Circle Drawer mouse event did not enable Undo"
        }
        if ($redo.Enabled) {
            throw "[smoke] FAIL: Circle Drawer Redo became enabled before Undo"
        }
        Save-WindowScreenshot -Handle $Window -Path $afterPath -AllowScreenFallback $true | Out-Null
        $different = Measure-CentralBitmapDifference $beforePath $afterPath
        if ($different -lt 100) {
            throw "[smoke] FAIL: PaintBox center did not visibly repaint (different samples=$different)"
        }

        Send-WindowMessage $undo.Handle 0x00F5 | Out-Null # BM_CLICK
        Start-Sleep -Milliseconds 350
        $undo = Get-UniqueChildWindow $Window "Button" "Undo"
        $redo = Get-UniqueChildWindow $Window "Button" "Redo"
        if ($undo.Enabled -or -not $redo.Enabled) {
            throw "[smoke] FAIL: Circle Drawer Undo/Redo state is invalid after Undo"
        }
        $undoPath = Join-Path ([IO.Path]::GetTempPath()) "flux-vcl-paint-$([Guid]::NewGuid()).undo.png"
        try {
            Save-WindowScreenshot -Handle $Window -Path $undoPath -AllowScreenFallback $true | Out-Null
            $restored = Measure-CentralBitmapDifference $beforePath $undoPath
            if ($restored -ge 100) {
                throw "[smoke] FAIL: PaintBox surface did not return after Undo (different samples=$restored)"
            }
        } finally {
            Remove-Item -LiteralPath $undoPath -Force -ErrorAction SilentlyContinue
        }

        Send-WindowMessage $redo.Handle 0x00F5 | Out-Null # BM_CLICK
        Start-Sleep -Milliseconds 350
        $undo = Get-UniqueChildWindow $Window "Button" "Undo"
        $redo = Get-UniqueChildWindow $Window "Button" "Redo"
        if (-not $undo.Enabled -or $redo.Enabled) {
            throw "[smoke] FAIL: Circle Drawer Redo did not restore the undo state"
        }

        $radiusEdit = Get-UniqueChildWindow $Window "Edit"
        if (-not $radiusEdit.Enabled -or $radiusEdit.Text -ne "30") {
            throw "[smoke] FAIL: Circle Drawer selected radius editor is invalid after Redo"
        }
        Replace-EditTextWithASCII $Window $radiusEdit "60"
        Start-Sleep -Milliseconds 400
        if ((Get-ChildWindowText $radiusEdit.Handle) -ne "60") {
            throw "[smoke] FAIL: Circle Drawer radius edit did not reach controlled State"
        }
        $resizedPath = Join-Path ([IO.Path]::GetTempPath()) "flux-vcl-paint-$([Guid]::NewGuid()).resized.png"
        try {
            Save-WindowScreenshot -Handle $Window -Path $resizedPath -AllowScreenFallback $true | Out-Null
            $resizedDifference = Measure-CentralBitmapDifference $afterPath $resizedPath
            if ($resizedDifference -lt 100) {
                throw "[smoke] FAIL: Circle Drawer radius update did not visibly repaint (different samples=$resizedDifference)"
            }
        } finally {
            Remove-Item -LiteralPath $resizedPath -Force -ErrorAction SilentlyContinue
        }

        $secondX = $x + 180
        if ($secondX -gt $client.Right - 80) { $secondX = $x - 180 }
        $secondPoint = [IntPtr](($y -shl 16) -bor ($secondX -band 0xFFFF))
        Send-WindowMessage $Window 0x0201 ([IntPtr]1) $secondPoint | Out-Null # WM_LBUTTONDOWN
        Send-WindowMessage $Window 0x0202 ([IntPtr]0) $secondPoint | Out-Null # WM_LBUTTONUP
        Start-Sleep -Milliseconds 400
        $radiusEdit = Get-UniqueChildWindow $Window "Edit"
        Replace-EditTextWithASCII $Window $radiusEdit "25"
        Start-Sleep -Milliseconds 350
        Send-WindowMessage $Window 0x0201 ([IntPtr]1) $point | Out-Null # select the first circle
        Send-WindowMessage $Window 0x0202 ([IntPtr]0) $point | Out-Null
        Start-Sleep -Milliseconds 400
        if ((Get-ChildWindowText $radiusEdit.Handle) -ne "60") {
            throw "[smoke] FAIL: Circle Drawer did not select the original circle or restore its radius"
        }
        Write-Host "[smoke] PASS: PaintBox create/select/radius, repaint, Undo, and Redo verified (different samples=$different/$resizedDifference)"
    } finally {
        Remove-Item -LiteralPath $beforePath -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $afterPath -Force -ErrorAction SilentlyContinue
    }
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

$pageBaseline = $null
$sevenGuiTargets = @(
    "7guis-counter", "7guis-temperature-converter", "7guis-flight-booker",
    "7guis-timer", "7guis-crud", "7guis-circle-drawer", "7guis-cells"
)
$isSevenGui = $sevenGuiTargets -contains $Target
if ($isSevenGui) {
    switch ($Target) {
        "7guis-counter" { Assert-CounterInteraction $hwnd }
        "7guis-temperature-converter" { Assert-TemperatureInteraction $hwnd }
        "7guis-flight-booker" { Assert-FlightBookerInteraction $hwnd }
        "7guis-timer" { Assert-TimerInteraction $hwnd }
        "7guis-crud" { Assert-GridInteraction $hwnd $Target }
        "7guis-cells" { Assert-GridInteraction $hwnd $Target }
        "7guis-circle-drawer" { Assert-PaintInteraction $hwnd }
    }
} else {
    # Existing examples keep the historical unique numeric button contract. The
    # 7GUIs targets above deliberately use business controls instead.
    $btn = Get-CounterButton $hwnd

    $b0 = New-Object System.Text.StringBuilder 256
    [W]::GetWindowTextW($btn, $b0, 256) | Out-Null
    Write-Host "[smoke] button before click: '$($b0.ToString())'"

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
}

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
