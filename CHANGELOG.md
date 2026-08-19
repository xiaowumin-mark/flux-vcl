# Changelog

## Unreleased / v0.1.0 candidate

- Added the productized 7GUIs examples: Counter, Temperature Converter, Flight
  Booker, Timer, CRUD, Circle Drawer, and Cells.
- Added the batch 3 controls: controlled horizontal Slider, native StringGrid
  with defensive-copying cells, and value-command PaintBox drawing.
- Added English documentation entry points, the frozen candidate API list,
  migration and maintenance policies, and the release checklist.
- Added accessibility metadata Opts, explicit tab-stop/default/cancel semantics,
  declaration-derived tab order, logical radio-group arrow navigation, and
  high-contrast system-color handling.
- Added immutable locale catalogs, reactive message bindings, stable replaceable
  framework diagnostic IDs, and the embedded English/Chinese
  `accessibility-i18n` verification example.
- Replaced mouse-only Text actions in the public examples with keyboard-reachable
  buttons and documented the locked LCL runtime's UIA boundary: stable desktop-root
  and `FromHandle` queries retain standard Win32 control patterns, Accessible
  overrides are not projected, and PaintBox/TLabel have no child HWND.
- Kept the existing D1-D7, native DLL lock, and Windows smoke requirements.

There are no breaking changes recorded for this candidate. Reopening the
candidate surface before the v0.1.0 tag requires the P7.5 gate and migration
notes; after v0.1.0, breaking changes require a new minor release until 1.0.
