<#
.SYNOPSIS
  FluxVCL 无头冒烟脚本（Phase 0.3，0.5 CI 复用）。

  启动目标 exe，用 Win32 API 验证：主窗口出现 -> 按钮存在 -> 模拟点击 ->
  按钮文本变化（点击生效）-> WM_CLOSE 后进程干净退出。

  注意：LCL 的 TLabel 无独立 HWND（自绘在父窗体表面），冒烟通过按钮文本
  观测"点击生效"，这也是 FluxVCL 需要记住的工程约束。

.EXAMPLE
  .\scripts\build.ps1; .\scripts\smoke.ps1
  .\scripts\smoke.ps1 -Target basic
#>
param(
    [string]$Target = "basic",   # 目标应用：examples/<Target>
    [string]$Output = "bin",     # 与 build.ps1 的 Output 一致
    [string]$Screenshot = ""     # 非空则点击验证后截主屏保存到此路径（CI artifact）
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
    public static extern uint GetWindowThreadProcessId(IntPtr h, out uint processId);
    [DllImport("user32.dll")]
    public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr wp, IntPtr lp);
    [DllImport("user32.dll", EntryPoint="SendMessageW", CharSet=CharSet.Unicode)]
    public static extern IntPtr SendMessageText(IntPtr h, uint m, IntPtr wp, StringBuilder lp);
    [DllImport("user32.dll")]
    public static extern bool PostMessage(IntPtr h, uint m, IntPtr wp, IntPtr lp);
}
'@

Get-Process $Target -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 300

$p = Start-Process -FilePath $exe -PassThru
Write-Host "[smoke] started pid=$($p.Id) exe=$exe"

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
$hwnd = [IntPtr]::Zero
for ($i = 0; $i -lt 30; $i++) {
    $script:hwnd = [IntPtr]::Zero
    [W]::EnumWindows($cbTop, [IntPtr]::Zero) | Out-Null
    if ($script:hwnd -ne [IntPtr]::Zero) { $hwnd = $script:hwnd; break }
    Start-Sleep -Milliseconds 400
}
if ($hwnd -eq [IntPtr]::Zero) { Write-Host "[smoke] FAIL: window not found"; exit 1 }
Write-Host "[smoke] window found hwnd=$hwnd"

# 找按钮（EnumChildWindows，class=Button）
$cbBtn = [W+WEnum]{ param($h,$l)
    $cs = New-Object System.Text.StringBuilder 256
    [W]::GetClassNameW($h, $cs, 256) | Out-Null
    if ($cs.ToString() -eq "Button") { $script:btn = $h }
    return $true
}
$script:btn = [IntPtr]::Zero
[W]::EnumChildWindows($hwnd, $cbBtn, [IntPtr]::Zero) | Out-Null
if ($script:btn -eq [IntPtr]::Zero) { Write-Host "[smoke] FAIL: button not found"; exit 1 }
$btn = $script:btn

$b0 = New-Object System.Text.StringBuilder 256
[W]::GetWindowTextW($btn, $b0, 256) | Out-Null
Write-Host "[smoke] button before click: '$($b0.ToString())'"

[W]::SendMessage($btn, 0x00F5, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null   # BM_CLICK
Start-Sleep -Milliseconds 500

$b1 = New-Object System.Text.StringBuilder 256
[W]::GetWindowTextW($btn, $b1, 256) | Out-Null
Write-Host "[smoke] button after click: '$($b1.ToString())'"
# 按钮文本由 State 驱动（Button(Bind(count)) 显示数字）：点击 +1 即"点击生效"。
if ($b1.ToString() -match "^\d+$") { Write-Host "[smoke] PASS: click handled" }
else { Write-Host "[smoke] FAIL: click not handled"; exit 1 }

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
        $script:btn = [IntPtr]::Zero
        [W]::EnumChildWindows($hwnd, $cbBtn, [IntPtr]::Zero) | Out-Null
        if ($script:btn -eq [IntPtr]::Zero) { Write-Host "[smoke] FAIL: rebuilt button not found"; exit 1 }
        [W]::SendMessage($script:btn, 0x00F5, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
        Start-Sleep -Milliseconds 300
    }
    $btn = $script:btn
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
        $memoLength = [W]::SendMessage($script:memo, 0x000E, [IntPtr]::Zero, [IntPtr]::Zero).ToInt32()
        $memoText = New-Object System.Text.StringBuilder ($memoLength + 1)
        [W]::SendMessageText($script:memo, 0x000D, [IntPtr]($memoLength + 1), $memoText) | Out-Null
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
            Add-Type -AssemblyName System.Windows.Forms
            Add-Type -AssemblyName System.Drawing
            $bounds = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
            $bmp = New-Object System.Drawing.Bitmap $bounds.Width, $bounds.Height
            $g = [System.Drawing.Graphics]::FromImage($bmp)
            $g.CopyFromScreen($bounds.Location, [System.Drawing.Point]::Empty, $bounds.Size)
            $bmp.Save($Screenshot, [System.Drawing.Imaging.ImageFormat]::Png)
            $g.Dispose(); $bmp.Dispose()
            Write-Host "[smoke] screenshot saved: $Screenshot"
        } catch {
            Write-Host "[smoke] WARN screenshot failed (headless?): $_"
        }
    }

    # 关闭工具窗后目标 App 仍应可交互，证明关闭只影响 Inspector。
    [W]::PostMessage($script:inspectorHwnd, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
    Start-Sleep -Milliseconds 300
    $script:btn = [IntPtr]::Zero
    [W]::EnumChildWindows($hwnd, $cbBtn, [IntPtr]::Zero) | Out-Null
    if ($script:btn -eq [IntPtr]::Zero) { Write-Host "[smoke] FAIL: target button missing after Inspector close"; exit 1 }
    $btn = $script:btn
    [W]::SendMessage($btn, 0x00F5, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
    Start-Sleep -Milliseconds 300
    $b4 = New-Object System.Text.StringBuilder 256
    [W]::GetWindowTextW($btn, $b4, 256) | Out-Null
    if ($b4.ToString() -ne "4") {
        Write-Host "[smoke] FAIL: target stopped after Inspector close"; exit 1
    }
    Write-Host "[smoke] PASS: target remains interactive after Inspector close"
} elseif ($Screenshot) {
    # 可选截图（CI artifact；无头会话可能黑屏，失败仅告警不中断）。
    try {
        Add-Type -AssemblyName System.Windows.Forms
        Add-Type -AssemblyName System.Drawing
        $bounds = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
        $bmp = New-Object System.Drawing.Bitmap $bounds.Width, $bounds.Height
        $g = [System.Drawing.Graphics]::FromImage($bmp)
        $g.CopyFromScreen($bounds.Location, [System.Drawing.Point]::Empty, $bounds.Size)
        $bmp.Save($Screenshot, [System.Drawing.Imaging.ImageFormat]::Png)
        $g.Dispose(); $bmp.Dispose()
        Write-Host "[smoke] screenshot saved: $Screenshot"
    } catch {
        Write-Host "[smoke] WARN screenshot failed (headless?): $_"
    }
}

[W]::PostMessage($hwnd, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null   # WM_CLOSE
if ($p.WaitForExit(8000)) { Write-Host "[smoke] PASS: process exited cleanly (code $($p.ExitCode))" }
else { Write-Host "[smoke] FAIL: process did not exit"; Stop-Process -Id $p.Id -Force; exit 1 }

Write-Host "[smoke] RESULT: PASS"
