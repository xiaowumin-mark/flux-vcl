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
    [DllImport("user32.dll", CharSet=CharSet.Unicode)]
    public static extern int GetClassNameW(IntPtr h, StringBuilder sb, int max);
    [DllImport("user32.dll")]
    public static extern bool IsWindowVisible(IntPtr h);
    [DllImport("user32.dll")]
    public static extern IntPtr SendMessage(IntPtr h, uint m, IntPtr wp, IntPtr lp);
    [DllImport("user32.dll")]
    public static extern bool PostMessage(IntPtr h, uint m, IntPtr wp, IntPtr lp);
}
'@

Get-Process $Target -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 300

$p = Start-Process -FilePath $exe -PassThru
Write-Host "[smoke] started pid=$($p.Id) exe=$exe"

# 找主窗口（标题前缀 "FluxVCL "）
$cbTop = [W+WEnum]{ param($h,$l)
    if ([W]::IsWindowVisible($h)) {
        $sb = New-Object System.Text.StringBuilder 512
        [W]::GetWindowTextW($h, $sb, 512) | Out-Null
        if ($sb.ToString() -like "FluxVCL *") { $script:hwnd = $h }
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
if ($b1.ToString() -match "Clicked \d") { Write-Host "[smoke] PASS: click handled" }
else { Write-Host "[smoke] FAIL: click not handled"; exit 1 }

# 可选截图（CI artifact；无头会话可能黑屏，失败仅告警不中断）
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

[W]::PostMessage($hwnd, 0x0010, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null   # WM_CLOSE
if ($p.WaitForExit(8000)) { Write-Host "[smoke] PASS: process exited cleanly (code $($p.ExitCode))" }
else { Write-Host "[smoke] FAIL: process did not exit"; Stop-Process -Id $p.Id -Force; exit 1 }

Write-Host "[smoke] RESULT: PASS"
