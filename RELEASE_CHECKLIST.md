# v0.1.0 Release Checklist

## Source and API

- [x] `docs/api-v0.1.0.md` matches exported constructors, Opts, and events.
- [x] `README.md` and `README.en.md` quick starts and example tables agree.
- [x] `docs/design.md`, `docs/development-plan.md`, migration, and maintenance
      notes describe known LCL limitations.
- [x] `CHANGELOG.md` contains the release notes and any breaking-change note.

## Verification

- [ ] `gofmt -l $(rg --files -g '*.go' -g '!ref/**')` is empty (PowerShell:
      `gofmt -l (rg --files -g '*.go' -g '!ref/**')`), and `git diff --check`
      is clean. The vendored research snapshots under `ref/` are excluded.
- [ ] `go test -count=1 ./...`, `go test -race -count=1 ./...`, and
      `go vet ./...` pass on every supported Go version.
- [x] DLL hash/module lock verification passes.
- [x] All public examples and all seven 7GUIs targets build on Windows.
- [x] Every target passes interaction smoke, exits with code 0, and produces a
      non-empty pixel-validated screenshot.
- [x] Native probes cover Slider, StringGrid editing/selection, and PaintBox
      invalidate without recreation.

## Installer and hand-off

- [ ] A clean Windows VM installs, launches an example within five minutes,
      rejects in-use uninstall, and leaves no files after uninstall.
- [ ] License/NOTICE and dependency lock artifacts are present.
- [ ] Inspector shows no unexpected rebuilds during the 7GUIs workflows.
- [ ] The release tag and checksum are recorded by the maintainer.
