# Phase 0 · E2 实验：libenergy DLL 获取路径与版本映射

> 目标：确认 energye/lcl 所需的 `libenergy` 运行时 DLL 从哪里获得、文件名规则、
> Go 包版本 ↔ 运行时版本 ↔ DLL 文件名三者的锁定关系，并验证 `libname.LibName`
> 手动指定路径的加载机制。
>
> 状态：**完成**（2026-08-09，由 E1 冒烟调试直接验证，非单独实验）。

---

## 1. 结论速览

| 项 | 结论 |
|---|---|
| **Go 包版本** | `github.com/energye/lcl` 必须与 DLL 版本**严格一致** |
| **DLL 文件名（Windows amd64）** | `libenergy-amd64.dll` |
| **DLL 权威来源** | **energye/designer 仓库内嵌资源**（不是 SourceForge / GitHub Releases） |
| **DLL 在 designer 中的位置** | `resources/frameworks/lib/windows/libenergy-amd64.zip`（`//go:embed`） |
| **DLL 与 Go 包版本的锁定** | designer 的 `go.mod` 锁定 `lcl v1.0.3` → DLL 也是 v1.0.3 编译 |
| **导出命名代际** | v1.0.3：`TControl_Caption` 等 **T 前缀**；v1.0.9：`TControl_SetCaption` 等**拆分 setter** |
| **加载机制** | `libname.LibName` 手动指定绝对路径即可，DLL 按名字惰性解析 |
| **版本错位后果** | `SysCallN` 按名字 `Dll.Proc()` 解析失败 → **静默返回 0**，窗口不创建、无报错 |

---

## 2. 版本 ↔ DLL 映射表

| 平台 | Go 包版本（designer 锁定） | DLL 文件名 | 内嵌 zip 路径（designer） |
|---|---|---|---|
| Windows amd64 | v1.0.3 | `libenergy-amd64.dll` | `resources/frameworks/lib/windows/libenergy-amd64.zip` |
| Windows 386 | v1.0.3 | `libenergy-386.dll` | `resources/frameworks/lib/windows/libenergy-386.zip` |
| Linux amd64 gtk2 | v1.0.3 | `libenergy-amd64-gtk2.so` | `resources/frameworks/lib/linux/libenergy-amd64-gtk2.zip` |
| Linux amd64 gtk3 | v1.0.3 | `libenergy-amd64-gtk3.so` | `resources/frameworks/lib/linux/libenergy-amd64-gtk3.zip` |
| macOS amd64 | v1.0.3 | `libenergy-amd64.dylib` | `resources/frameworks/lib/darwin/libenergy-amd64.zip` |
| macOS arm64 | v1.0.3 | `libenergy-arm64.dylib` | `resources/frameworks/lib/darwin/libenergy-arm64.zip` |

> 文件名规则来自 `api/libname.GetDLLName()`：
> `libenergy-<GOARCH>[--ws].<ext>`，其中 windows→`.dll`、linux→`.so`（`--ws` 取环境变量 `--ws`，默认 gtk2）、macos→`.dylib`。

---

## 3. DLL 权威获取路径（为何不是 SourceForge/GitHub Releases）

调研 `energye/lcl` 的官方分发渠道，结论如下：

| 渠道 | 内容 | 是否匹配 energye/lcl |
|---|---|---|
| SourceForge `liblcl` 项目 v2.5.4 | `liblcl.dll`（无 T 前缀导出） | ❌ 匹配 **govcl**（ying32），不匹配 energye/lcl |
| SourceForge v3.0.0 | 仅 CEF/WebView2 Windows 包 | ❌ |
| SourceForge v3.0.1 | 仅 Linux 包（`libenergy-linux-amd64-gtk3-*.zip`） | ❌ Windows 无 |
| GitHub Releases（energye/lcl） | 无 DLL 二进制资源 | ❌ |
| **energye/designer 仓库内嵌 zip** | `resources/frameworks/lib/windows/libenergy-amd64.zip` → 解压出 13MB `libenergy-amd64.dll`（T 前缀导出） | ✅ **唯一可靠来源** |

issue #2 官方回复也证实：**"请使用 energye/designer 创建 LCL 应用"** —— designer 是官方分发 DLL 的唯一途径。

### 获取步骤（已验证）

1. `git clone https://github.com/energye/designer`
2. 路径：`designer/resources/frameworks/lib/windows/libenergy-amd64.zip`
3. 解压即得 `libenergy-amd64.dll`（约 13MB），拷到 exe 同级目录
4. Go 侧 `go.mod` 锁定 `github.com/energye/lcl v1.0.3`（与 designer 的 `go.mod` 一致）

---

## 4. 加载机制验证

`api/imports/imports.go` 的关键实现：

```go
func (m *Imports) internalGetImportFunc(index int) ProcAddr {
    item := m.Table[index]
    if ... item.addr == nil {
        item.addr, err = m.Dll.Proc(item.name)   // 按名字解析，非按序号！
        if err != nil {
            println("[ERROR] GetImport Name:", item.name, ...)
            return 0                              // 失败→返回 0，SysCall 静默
        }
    }
    return item.addr
}
```

- **`SysCallN(index)` 中的 `index` 只是 `Table` 数组下标**，真正解析是 `Dll.Proc(item.name)` 按**函数名**查找。
- 因此 Go 包版本的 `NewTable` 名字全集，必须与 DLL 导出名全集匹配。
- 加载优先级：`libname.LibName`（手动指定）> exe 目录 > config FrameworkPath/runtime > temp dir。

### E1 验证的加载用法（可靠）

```go
libname.LibName = dllPath   // dllPath = exe 旁 libenergy-amd64.dll 的绝对路径
lcl.Init(nil, nil)
```

---

## 5. 版本错位的故障模式（E1 调试中实锤）

| 现象 | 原因 |
|---|---|
| 进程存活、`"window shown"` 打印、**但无任何窗口** | v1.0.9 Go 包 + v1.0.3 DLL：`TControl_Show` 在 DLL 中不存在 → 返回 0 → 窗口不创建 |
| `form.Handle()` 返回非零但 `IsWindow=false` | Handle 函数解析失败返回占位值，非真实 HWND |
| stderr 为空（误以为正常） | `-H=windowsgui` 构建吞掉控制台；换控制台版可见 `[ERROR] GetImport Name` |
| `HandleAllocated=true` 但无窗口 | 即便部分函数可用，关键 Show/Initialize 缺失即失败 |

### 版本差异核心

- **v1.0.3**（designer 时代）：`TControl_Caption` 一个函数同时做 get/set（参数 `0`=get，`1`=set）。
- **v1.0.9**（新版）：拆分为 `TControl_SetCaption` / `TControl_GetCaption`。
- 新增大量 Action 类导出（`TEditAction_*`、`TEditCopy_*`、`THelpAction_*` 等），v1.0.3 DLL 中没有。

> **结论：DLL 一旦定版，Go 包必须锁定到同一版本。** 升级 Go 包必须同步更换 DLL，二者不可解耦。

---

## 6. 给 FluxVCL 的工程约束

1. **依赖锁定**：`go.mod` 锁定 `lcl v1.0.3`（与 designer 一致），DLL 由 `libenergy-amd64.dll` 提供。
2. **DLL 分发**：FluxVCL 不应自行托管 DLL 到第三方分发渠道；跟随 designer 的获取方式，或将 DLL 纳入自己的资源 embed（如 designer 的 `//go:embed` 模式）。
3. **版本自检**：构建/运行时校验 `api.Widget() == api.WtWIN32` 与 `api.LCLVersion()`，可在 `Init` 后断言，防止静默错位。
4. **初始化顺序**（E1 验证通过的标准序列）：
   ```go
   libname.LibName = dllPath
   lcl.Init(nil, nil)                    // 参数为 emfs.IEmbedFS（无嵌入传 nil）
   lcl.Application.Initialize()          // 必须，否则窗口不创建
   lcl.Application.SetMainFormOnTaskBar(true)
   form := &TEngForm{}                   // 内嵌 lcl.TEngForm
   lcl.Application.NewForms(form)        // 必须先注册再建控件，否则控件绑定旧实例
   // ... 创建控件 ...
   lcl.Application.Run()
   ```
5. **线程纪律**：退出用 `lcl.RunOnMainThreadAsync(func(uint32){ lcl.Application.Terminate() })`，控件操作必须在 UI 线程。

---

## 7. 实验产物

| 路径 | 说明 |
|---|---|
| `ref/e1-smoke/` | E1 冒烟工程（main.go + go.mod 锁 v1.0.3） |
| `ref/e1-smoke/libenergy-amd64.dll` | 从 designer 内嵌 zip 解压的正确 DLL（13MB） |
| `ref/e1-smoke/compare_v109_vs_dll.py` | Go 包表名 vs DLL 导出名对比脚本 |
| `ref/designer-src/` | energye/designer 源码 clone（DLL 来源） |
| `ref/designer-lib/` | 解压出的 DLL 原始副本 |
