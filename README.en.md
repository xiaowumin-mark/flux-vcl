# FluxVCL

FluxVCL is a declarative desktop UI framework for Go. It renders a stable
widget tree to native Windows controls through the `energye/lcl` adapter.

```go
package main

import (
    "os"
    "path/filepath"

    flux "github.com/xiaowumin-mark/flux-vcl"
    "github.com/xiaowumin-mark/flux-vcl/native"
)

func main() {
    executable, err := os.Executable()
    if err != nil { panic(err) }
    if err := native.Init(filepath.Join(filepath.Dir(executable), "libenergy-amd64.dll")); err != nil {
        panic(err)
    }
    app := flux.NewApp(native.NewRenderer())
    count := flux.NewState(0)
    if err := app.Mount(func() flux.Widget {
        return flux.Window(flux.Button(flux.Bind(count), flux.OnClick(func(flux.Event) {
            count.Set(count.Get() + 1)
        })))
    }); err != nil { panic(err) }
    native.Run()
}
```

## Status

The current build is `0.1.0-dev`, so it is not a release. The v0.1.0 candidate
API surface is frozen as the first-release baseline; the formal SemVer promise
starts at the v0.1.0 tag. The completed P7.5 scope includes
declarative diffing, DIP-aware layout, State bindings, IME-aware
native input, virtualized lists, multiple windows, Inspector, plugins,
PageControl, Slider, StringGrid, and PaintBox. The seven 7GUIs tasks are in
`examples/7guis-*`.

The default backend is `energye/lcl v1.0.3` and requires a matching
`libenergy-amd64.dll`. See [the packaging guide](docs/packaging.md) for the
verified dependency and installer workflow.

## Quick start

```powershell
go test ./...
go vet ./...
.\scripts\build.ps1 -Target basic
.\scripts\smoke.ps1 -Target basic -Screenshot .\bin\basic-smoke.png
```

For a new application, create a renderer after `native.Init`, construct an
`App`, and call `Mount`. State changes are marshalled to the UI thread and the
diff engine patches only changed properties. Do not access LCL objects from a
goroutine; use `State.Set`, `App.SetBounds`, or the documented capability
interfaces.

## Examples

Every example is a standalone Windows target with a `winres/winres.json` file.
The 7GUIs examples are:

| Task | Target | Main capability |
|---|---|---|
| Counter | `7guis-counter` | State, Text, Button |
| Temperature Converter | `7guis-temperature-converter` | controlled Input |
| Flight Booker | `7guis-flight-booker` | ComboBox, validation, Enabled |
| Timer | `7guis-timer` | Animation, ProgressBar, Slider |
| CRUD | `7guis-crud` | StringGrid, selection and editing |
| Circle Drawer | `7guis-circle-drawer` | PaintBox, DIP mouse hit testing |
| Cells | `7guis-cells` | StringGrid and formula dependencies |

Build one with `scripts/build.ps1 -Target <target>` and run the matching
`scripts/smoke.ps1` target. The complete mapping and known limitations are in
[docs/7guis.md](docs/7guis.md).

## Candidate API and limitations

The frozen v0.1.0 candidate surface is listed in
[docs/api-v0.1.0.md](docs/api-v0.1.0.md). The `-dev` build is not yet a release;
reopening the candidate surface requires the P7.5 gate and migration notes. The framework deliberately
does not expose an ORM, an unbounded data source, an Excel compatibility layer,
a general vector engine, or a GPU scene graph.

Native controls retain widgetset behavior. Win32 theme drawing may ignore
custom Button/Label colors; PaintBox is a graphic control without a separate
child HWND; StringGrid accessibility and editor details inherit LCL limits.
These constraints are recorded in [docs/design.md](docs/design.md), not hidden
behind an escape hatch.

## Documentation

- [Chinese README](README.md)
- [Design](docs/design.md) and [development plan](docs/development-plan.md)
- [Development guide](docs/development-guide.md)
- [7GUIs mapping](docs/7guis.md)
- [Verifiable capability comparison](docs/capability-comparison.md)
- [Migration and compatibility](docs/migration.md)
- [Maintenance policy](docs/maintenance.md)
- [Changelog](CHANGELOG.md)
- [Release checklist](RELEASE_CHECKLIST.md)

## License

[MIT](LICENSE)
