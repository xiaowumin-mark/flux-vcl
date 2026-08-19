# FluxVCL Windows 打包

> 状态：P7.4 实现完成，发布门待验证；当前候选基线为 Windows amd64、NSIS 3.11、
> `FluxVCL 0.1.0-dev`。
> 尚待两项外部门禁：新 workflow 的 hosted clean Windows VM 首跑，以及 opaque DLL 的完整
> 静态组件许可/源码义务审计。

## 1. 交付形式

v0.1.0 采用 **exe + 同目录 DLL**，NSIS 是主安装方案。默认安装包以
`examples/basic` 作为可启动示例入口；`package.ps1 -Target <name>` 也可为其他公开示例
生成同构安装包。

安装范围是当前用户，不请求管理员权限，默认目录为
`%LOCALAPPDATA%\Programs\FluxVCL\<target>`。安装器写入 Windows“已安装的应用”注册表项，
并创建示例和卸载两个开始菜单入口。安装内容如下：

- `<target>.exe`；
- `libenergy-amd64.dll`；
- FluxVCL `LICENSE.txt`；
- `THIRD-PARTY-NOTICES.txt`、`energye-Apache-2.0.txt`、`Go-LICENSE.txt` 与
  `Go-PATENTS.txt`；
- 可审计的 `dependencies.lock.json`；
- `uninstall.exe`。

卸载器只移除上述文件、当前 target 的快捷方式与注册表项；同一开始菜单目录中其他 target
的入口不受影响。

## 2. 构建

前提：Go 1.22+、NSIS 3.11，以及已获取的匹配 DLL。NSIS 版本也是发布工具链的一部分，
CI 使用 Chocolatey 包 `nsis 3.11.0`，不取浮动版本。

```powershell
.\scripts\fetch-libenergy.ps1
.\scripts\package.ps1 -AllowDevVersion

# 输出
# bin/FluxVCL-0.1.0-basic-setup.exe
# bin/FluxVCL-0.1.0-basic-setup.exe.sha256
```

`verify-release.ps1` 默认要求 `flux.Version` 与 `-Version` 逐字一致，因此会拒绝
`0.1.0-dev` 作为正式 `0.1.0` 发布。当前候选只能显式传递 `-AllowDevVersion`，它仅允许
精确的 `$Version-dev` 进行内部安装验证，并会发出警告；该参数不得用于 tag、Release 或公开
分发。正式打 tag 前必须把 `flux.Version` 改为无后缀的 `0.1.0`，并在不带该参数的情况下
重新完成资源、安装和许可门验证。

## 3. 依赖锁与来源校验

唯一发布锁是 `packaging/dependencies.lock.json`：

| 项 | 锁定值 |
|---|---|
| Go module | `github.com/energye/lcl v1.0.3` |
| DLL | `libenergy-amd64.dll` |
| designer commit | `5c4ec54834ce00641920c6c79616e8f4d58b5a68` |
| archive | `resources/frameworks/lib/windows/libenergy-amd64.zip` |
| DLL SHA-256 | `2D13987CB5505D56C24D073F5CE8C1CE981A9BD1BD78D8BDE16C8EDBD8641300` |

`fetch-libenergy.ps1`、`build.ps1` 与 `package.ps1` 都读取该锁。
`verify-dependencies.ps1` 在构建前检查：

1. `go list -m -json` 的 module 路径和版本完全一致，且不存在 `replace`；
2. 目标只能是锁定的 `windows/amd64`；
3. 来源 URL 同时包含完整 designer commit 和固定 archive path，拒绝 `latest/main/master`；
4. 待打包 DLL 的 SHA-256 完全一致。

哈希将任意本地 DLL 副本绑定到锁中唯一的上游字节内容，因此仅改文件名或从未知来源取一个
“同版本”文件不能绕过校验。升级 lcl 时必须在同一变更中更新 module、designer commit、
archive path、DLL 文件名/架构与 SHA-256，并重新跑安装器闭环。

## 4. PE 资源门禁

`verify-release.ps1` 不信任源配置，而是从最终 EXE 反向读取和提取资源，逐项断言：

- `FileVersionRaw` / `ProductVersionRaw` 为 `0.1.0.0`；
- Windows 可读的 `ProductName`、`ProductVersion`、`OriginalFilename` 完整；
- manifest identity 为同一四段版本；
- `<dpiAwareness>` 含 `permonitorv2`；
- `Microsoft.Windows.Common-Controls` 为 `6.0.0.0`；
- 图标资源存在且非空。

所有示例的 `winres.json` 使用 go-winres 要求的双层语言映射，避免只有固定版本数值、字符串
版本信息却被写入错误语言键的伪通过。

## 5. 安装、启动、卸载验证

本地可执行与发布 CI 相同的闭环：

```powershell
.\scripts\test-installer.ps1 `
  -InstallerPath .\bin\FluxVCL-0.1.0-basic-setup.exe
```

脚本静默安装到真实默认目录，检查全部文件、DLL 哈希、依赖锁、开始菜单入口及其目标和卸载
注册表；随后直接从安装目录启动现有 Win32 交互 smoke，验证窗口、按钮 State 回写、截图与
退出码。运行中重装必须返回非零；卸载器通过 Restart Manager 按精确 EXE/DLL 路径在删除前
拒绝，测试直接调用卸载体断言非零并另测标准启动路径仍保留可重试入口。关闭应用后重装必须
沿用原目录，最后静默卸载并确认目录、产品父目录、快捷方式和注册表均消失。

NSIS 标准卸载入口会先复制自身到 `%TEMP%`，外层 bootstrap 固定先返回 0，无法传播内部
`SetErrorLevel`。因此自动化既用 `_?=$INSTDIR` 验证真实卸载体的拒绝退出码，也以文件、快捷
方式和注册表全部保留作为标准入口的最终断言；关闭应用后的成功卸载仍使用标准入口。

GitHub Actions 已配置 `package-installer` job，在每次全新的 `windows-latest` VM 上执行同一
闭环。许可门解决前 artifact 只上传 SHA-256、依赖锁和安装后运行截图，不上传 setup.exe，避免
把验收包变成新的公开分发物。该 job 是干净 Windows 安装/启动/卸载的发布门，
不能用仅解压文件、本机测试或仅编译 NSIS 代替；只有相关 revision 的该 job 成功后，
才能把 clean VM 项标为通过。截图 artifact 保留 30 天，clean-VM 验证证据保留 90 天；候选
验证即使成功也不解除下一节的 DLL 许可阻塞。

## 6. 第三方许可边界

当前安装包携带 FluxVCL MIT、Go 工具链许可证，以及 `energye/lcl v1.0.3` 和固定 designer
提交声明的 Apache-2.0 文本。`go version -m` 可追溯 EXE 中唯一的第三方 Go module。

`libenergy-amd64.dll` 是上游提供的 opaque 二进制。对锁定字节做字符串、导出表和 PE 检查可
观察到 Lazarus LCL 4.4.0.0、Free Pascal 3.2.2、LazControls、VirtualTrees、IJG JPEG 派生代码
以及数百个 CEF4Delphi 符号。固定 designer 仓库中的 LCL/Energy source zip 为 0 字节，DLL zip
仅含二进制，也没有 build recipe、SBOM、NOTICE 或 Pascal 源码；公开 liblcl HEAD 的版本和时间
亦与该 DLL 不对应。仓库根 Apache-2.0 不能替代 modified LGPL LCL/FPC、MPL/LGPL
VirtualTreeView、IJG PasJPEG、MPL/LGPL CEF4Delphi 等组件自身的许可与源码义务。因此在上游
提供 exact source commit + build recipe + 组件 SBOM/NOTICE，或换用可复现构建并完成审计前，
安装包仅用于验收，不得公开分发或作为已通过许可门的正式发布物。

## 7. 单 EXE 评估

当前不采用单 EXE，原因如下：

- energye/lcl 最终通过 Windows loader 从路径加载 `libenergy-amd64.dll`；`go:embed` 只能把
  DLL 放进 Go 二进制，仍需释放到磁盘才能交给现有加载链，实质不是无 DLL 部署。
- 自写内存 PE loader 必须正确实现重定位、imports、TLS、异常处理和依赖搜索，并承担杀毒
  误报、签名、升级和临时文件锁定风险；当前上游没有与 windows/amd64 + v1.0.3 组合对应、
  可验证的受支持实现。
- 把二进制改装进 RCDATA 或临时目录不会取消第三方通知义务，也会弱化当前由独立 DLL 哈希
  提供的来源审计。energye/lcl 与固定 designer commit 均声明 Apache-2.0，但 DLL 的静态组件
  仍须按上一节完成独立审计。

因此 v0.1.0 明确使用 side-by-side exe + DLL。只有上游提供受支持的静态/内存加载方案，或
独立实现完成 loader 正确性、杀毒兼容、签名和许可复核后，才重新评估单 EXE；该评估不阻塞
当前发布。
