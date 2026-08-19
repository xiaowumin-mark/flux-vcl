// FluxVCL Accessibility / i18n demo: keyboard semantics, accessible metadata,
// embedded resources, and an in-place English/Chinese locale switch.
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/native"
)

//go:embed en.json
var englishJSON []byte

//go:embed zh-CN.json
var chineseJSON []byte

var messages = mustEmbeddedCatalog()

var statefulKeys = []string{
	"language-en", "language-zh", "project-name", "category", "priority",
	"assignments", "reset", "save",
}

type demoModel struct {
	app       *flux.App
	locale    *flux.State[flux.Locale]
	name      *flux.State[string]
	category  *flux.State[int]
	priority  *flux.State[int]
	selected  *flux.State[flux.GridCell]
	status    *flux.State[string]
	saveCount int
}

type formSnapshot struct {
	name                  string
	category, priority    int
	selectedRow, selected int
}

func mustEmbeddedCatalog() *flux.Catalog {
	resources := make(flux.Resources, 2)
	for _, item := range []struct {
		locale flux.Locale
		data   []byte
	}{
		{locale: "en", data: englishJSON},
		{locale: "zh-CN", data: chineseJSON},
	} {
		decoded := make(map[string]string)
		if err := json.Unmarshal(item.data, &decoded); err != nil {
			panic(fmt.Errorf("decode embedded locale %q: %w", item.locale, err))
		}
		localized := make(flux.Messages, len(decoded))
		for id, text := range decoded {
			localized[flux.MessageID(id)] = text
		}
		resources[item.locale] = localized
	}
	return flux.MustCatalog("en", resources)
}

func newDemoModel(app *flux.App) *demoModel {
	locale := flux.Locale("en")
	return &demoModel{
		app:      app,
		locale:   flux.NewState(locale),
		name:     flux.NewState("Ada Lovelace"),
		category: flux.NewState(1),
		priority: flux.NewState(3),
		selected: flux.NewState(flux.GridCell{Row: 0, Column: 0}),
		status:   flux.NewState(messages.Format(locale, "status.ready")),
	}
}

func (m *demoModel) snapshot() formSnapshot {
	cell := m.selected.Get()
	return formSnapshot{
		name: m.name.Get(), category: m.category.Get(), priority: m.priority.Get(),
		selectedRow: cell.Row, selected: cell.Column,
	}
}

func (m *demoModel) setStatus(id flux.MessageID) {
	m.status.Set(messages.Format(m.locale.Get(), id))
}

func (m *demoModel) selectCategory(index int) {
	if index < 0 {
		index = 0
	}
	if index > 2 {
		index = 2
	}
	m.category.Set(index)
	m.setStatus("status.category")
}

func keyedHandles(root *flux.Element) map[string]uintptr {
	result := make(map[string]uintptr, len(statefulKeys))
	var visit func(*flux.Element)
	visit = func(element *flux.Element) {
		if element == nil {
			return
		}
		for _, key := range statefulKeys {
			if element.Key == key {
				result[key] = uintptr(element.Handle)
				break
			}
		}
		for _, child := range element.Children {
			visit(child)
		}
	}
	visit(root)
	return result
}

func sameHandles(left, right map[string]uintptr) bool {
	if len(left) != len(statefulKeys) || len(right) != len(statefulKeys) {
		return false
	}
	for _, key := range statefulKeys {
		if left[key] == 0 || left[key] != right[key] {
			return false
		}
	}
	return true
}

func (m *demoModel) switchLocale(next flux.Locale) {
	beforeState := m.snapshot()
	beforeHandles := keyedHandles(m.app.Root())
	m.locale.Set(next)
	valid := beforeState == m.snapshot() &&
		sameHandles(beforeHandles, keyedHandles(m.app.Root())) &&
		len(m.app.LastLayoutDiags()) == 0
	if valid {
		m.setStatus("status.locale")
	} else {
		m.setStatus("status.invariant_failed")
	}
}

func (m *demoModel) reset() {
	m.name.Set("Ada Lovelace")
	m.category.Set(1)
	m.priority.Set(3)
	m.selected.Set(flux.GridCell{Row: 0, Column: 0})
	m.setStatus("status.reset")
}

func (m *demoModel) build() flux.Widget {
	locale := m.locale.Get()
	text := func(id flux.MessageID, args ...any) string { return messages.Format(locale, id, args...) }
	categories := []string{text("category.design"), text("category.engineering"), text("category.research")}
	cell := m.selected.Get()
	cells := [][]string{
		{text("grid.task.keyboard"), text("grid.owner.alex")},
		{text("grid.task.contrast"), text("grid.owner.blair")},
		{text("grid.task.translation"), text("grid.owner.casey")},
	}

	return flux.Window(
		flux.Title(text("window.title")),
		flux.Width(680),
		flux.Height(440),
		flux.Column(
			flux.CrossAxis(flux.CrossAxisStretch),
			flux.Row(
				flux.Text(messages.Bind(m.locale, "language.label"), flux.Width(112)),
				flux.Button(messages.Bind(m.locale, "language.english"),
					flux.Key("language-en"),
					flux.AccessibleName(text("language.english.name")),
					flux.OnClick(func(flux.Event) { m.switchLocale("en") }),
				),
				flux.Button(messages.Bind(m.locale, "language.chinese"),
					flux.Key("language-zh"),
					flux.AccessibleName(text("language.chinese.name")),
					flux.OnClick(func(flux.Event) { m.switchLocale("zh-CN") }),
				),
			),
			flux.Row(
				flux.Text(messages.Bind(m.locale, "name.label"), flux.Width(112)),
				flux.Expanded(flux.Input(
					flux.Bind(m.name),
					flux.Key("project-name"),
					flux.AccessibleName(text("name.name")),
					flux.AccessibleDescription(text("name.description")),
					flux.AccessibleValue(m.name.Get()),
					flux.OnChange(func(value string) {
						m.name.Set(value)
						m.setStatus("status.edited")
					}),
				)),
			),
			flux.Row(
				flux.Text(messages.Bind(m.locale, "category.label"), flux.Width(112)),
				flux.Expanded(flux.ComboBox(
					flux.Key("category"),
					flux.Items(categories),
					flux.SelectedIndex(m.category.Get()),
					flux.AccessibleName(text("category.name")),
					flux.AccessibleDescription(text("category.description")),
					flux.AccessibleValue(categories[m.category.Get()]),
					flux.OnSelectionChange(m.selectCategory),
				)),
			),
			flux.Row(
				flux.Text(messages.Bind(m.locale, "priority.label"), flux.Width(112)),
				flux.Expanded(flux.Slider(
					flux.Key("priority"),
					flux.Minimum(1), flux.Maximum(5), flux.Value(m.priority.Get()), flux.Step(1),
					flux.AccessibleName(text("priority.name")),
					flux.AccessibleDescription(text("priority.description")),
					flux.AccessibleValue(text("priority.value", m.priority.Get())),
					flux.OnValueChange(func(value int) {
						m.priority.Set(value)
						m.setStatus("status.priority")
					}),
				)),
				flux.Text(text("priority.value", m.priority.Get()), flux.Width(150)),
			),
			flux.Expanded(flux.StringGrid(3, 2,
				flux.Key("assignments"),
				flux.Headers([]string{text("grid.header.task"), text("grid.header.owner")}),
				flux.ColumnWidths([]int{260, 190}),
				flux.Cells(cells),
				flux.SelectedCell(cell.Row, cell.Column),
				flux.Editable(false),
				flux.AccessibleName(text("grid.name")),
				flux.AccessibleDescription(text("grid.description")),
				flux.AccessibleValue(text("grid.value", cell.Row+1, cell.Column+1)),
				flux.OnCellSelect(func(next flux.GridCell) {
					m.selected.Set(next)
					m.setStatus("status.grid")
				}),
			)),
			flux.Text(flux.Bind(m.status), flux.AccessibleName(text("status.name"))),
			flux.Row(
				flux.Expanded(flux.Text("")),
				flux.Button(messages.Bind(m.locale, "actions.reset"),
					flux.Key("reset"),
					flux.CancelButton(true),
					flux.AccessibleName(text("actions.reset.name")),
					flux.AccessibleDescription(text("actions.reset.description")),
					flux.OnClick(func(flux.Event) { m.reset() }),
				),
				flux.Button(messages.Bind(m.locale, "actions.save", m.saveCount),
					flux.Key("save"),
					flux.DefaultButton(true),
					flux.AccessibleName(text("actions.save.name")),
					flux.AccessibleDescription(text("actions.save.description")),
					flux.OnClick(func(flux.Event) {
						m.saveCount++
						m.setStatus("status.saved")
					}),
				),
			),
		),
	)
}

func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot locate executable:", err)
		os.Exit(2)
	}
	dllPath := filepath.Join(filepath.Dir(exe), "libenergy-amd64.dll")
	if _, err := os.Stat(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "missing libenergy-amd64.dll; run scripts/build.ps1 -Target accessibility-i18n")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "native initialization failed:", err)
		os.Exit(2)
	}

	renderer := native.NewRenderer()
	app := flux.NewApp(renderer)
	model := newDemoModel(app)
	if err := app.Mount(model.build); err != nil {
		fmt.Fprintln(os.Stderr, "mount failed:", err)
		os.Exit(2)
	}
	native.Run()
}
