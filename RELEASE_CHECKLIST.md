# v0.1.0 Release Checklist

> **硬门：** 本清单任一未勾选项都阻止创建 `v0.1.0` tag、GitHub Release 或公开上传安装包。
> `0.1.0-dev` 候选可用 `-AllowDevVersion` 做内部 CI/安装验证，但不能借此绕过正式版本、
> Hosted CI、clean-VM 或 DLL 许可门。

## Source and API

- [x] `docs/api-v0.1.0.md` matches exported constructors, Opts, and events.
- [x] `README.md` and `README.en.md` quick starts and example tables agree.
- [x] `docs/design.md`, `docs/development-plan.md`, migration, and maintenance
      notes describe known LCL limitations.
- [x] `CHANGELOG.md` contains the release notes and any breaking-change note.
- [x] `docs/accessibility-i18n.md` separates native keyboard behavior, framework
      metadata, and the verified energye/lcl UIA limitations.

## Accessibility and i18n

- [x] Public accessibility Opts patch/remove without rebuilding controls; keyed
      reorder patches per-parent TabOrder; logical radio groups support arrows.
- [x] Catalog fallback, defensive copies, locale bindings, and replaceable
      framework diagnostics have headless tests in both built-in locales.
- [x] Windows smoke verifies real Tab/Shift+Tab, arrows, Space, Enter, Esc and
      focus through `SendInput` plus `GetGUIThreadInfo`.
- [x] Windows smoke records representative desktop-root and `FromHandle` Win32 proxy patterns,
      missing AccessibleName/HelpText projection, and StringGrid's missing Grid Pattern.
- [x] Forced high-contrast capture and English/Chinese switching preserve HWND,
      focus, input/selection state, and a layout without overflow diagnostics.

## Verification

- [x] `gofmt -l $(rg --files -g '*.go' -g '!ref/**')` is empty (PowerShell:
      `gofmt -l (rg --files -g '*.go' -g '!ref/**')`), and `git diff --check`
      is clean. The vendored research snapshots under `ref/` are excluded.
- [ ] On the current supported CI matrix (Go 1.22.x–1.26.x and 1.27.0-rc.3),
      Windows `go test -count=1 ./...` passes; 1.27.0-rc.3 also passes
      `go test -race -count=1 ./...` and `go vet ./...`. Record the successful
      hosted run URL/SHA here before release.
- [x] DLL hash/module lock verification passes.
- [x] All 17 public examples and all seven 7GUIs targets build on Windows.
- [x] Every target passes interaction smoke, exits with code 0, and produces a
      non-empty pixel-validated screenshot.
- [x] Native probes cover Slider, StringGrid editing/selection, PaintBox
      invalidate, accessibility properties, focus semantics, radio arrows, and
      high-contrast color fallback without recreation.

## Installer and hand-off

- [ ] A clean Windows VM installs, launches an example within five minutes,
      rejects in-use uninstall, and leaves no files after uninstall.
- [ ] The release payload contains the checked LICENSE, Go notices, upstream
      notices, and `dependencies.lock.json`, and each matches the audited source.
- [ ] The locked opaque `libenergy-amd64.dll` has an exact source commit, build
      recipe, per-component SBOM/NOTICE, and a completed license/source-obligation
      review. Until this is checked, no tag, GitHub Release, installer upload, or
      other public binary distribution is permitted.
- [ ] Inspector shows no unexpected rebuilds during the 7GUIs workflows.
- [ ] `flux.Version` exactly matches the release version without `-dev`;
      `verify-release.ps1` passes without `-AllowDevVersion` from the tagged SHA.
- [ ] The release tag and checksum are recorded by the maintainer and bound to
      the verified hosted run SHA.
