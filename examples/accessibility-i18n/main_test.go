package main

import (
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

func elementByKey(t *testing.T, root *flux.Element, key string) *flux.Element {
	t.Helper()
	var found *flux.Element
	var visit func(*flux.Element)
	visit = func(element *flux.Element) {
		if element == nil || found != nil {
			return
		}
		if element.Key == key {
			found = element
			return
		}
		for _, child := range element.Children {
			visit(child)
		}
	}
	visit(root)
	if found == nil {
		t.Fatalf("element with key %q not found", key)
	}
	return found
}

func fireProperty[T any](t *testing.T, element *flux.Element, property string, value T) {
	t.Helper()
	raw, ok := element.Props.Get(property)
	if !ok {
		t.Fatalf("%s has no %s callback", element.Key, property)
	}
	callback, ok := raw.(func(T))
	if !ok {
		t.Fatalf("%s callback %s has type %T", element.Key, property, raw)
	}
	callback(value)
}

func countMutations(ops []render.Op, kind render.OpType) int {
	count := 0
	for _, op := range ops {
		if op.Type == kind {
			count++
		}
	}
	return count
}

func TestLocaleSwitchPreservesHandlesStateAndLayout(t *testing.T) {
	mock := render.NewMock()
	mock.SetClientSize(680, 440)
	app := flux.NewApp(mock)
	model := newDemoModel(app)
	if err := app.Mount(model.build); err != nil {
		t.Fatal(err)
	}

	fireProperty(t, elementByKey(t, app.Root(), "project-name"), "OnChange", "Grace Hopper")
	fireProperty(t, elementByKey(t, app.Root(), "category"), "OnSelectionChange", 2)
	fireProperty(t, elementByKey(t, app.Root(), "priority"), "OnValueChange", 5)
	fireProperty(t, elementByKey(t, app.Root(), "assignments"), "OnCellSelect", flux.GridCell{Row: 2, Column: 1})

	wantState := formSnapshot{name: "Grace Hopper", category: 2, priority: 5, selectedRow: 2, selected: 1}
	if got := model.snapshot(); got != wantState {
		t.Fatalf("state before locale switch = %+v, want %+v", got, wantState)
	}
	beforeHandles := keyedHandles(app.Root())
	base := len(mock.Ops())
	fireProperty(t, elementByKey(t, app.Root(), "language-zh"), "OnClick", flux.Event{})

	if got := model.snapshot(); got != wantState {
		t.Fatalf("locale switch changed entered/selected state: got %+v, want %+v", got, wantState)
	}
	if !sameHandles(beforeHandles, keyedHandles(app.Root())) {
		t.Fatalf("locale switch changed renderer handles: before=%v after=%v", beforeHandles, keyedHandles(app.Root()))
	}
	if diags := app.LastLayoutDiags(); len(diags) != 0 {
		t.Fatalf("Chinese locale introduced layout overflow: %+v", diags)
	}
	ops := mock.Ops()[base:]
	if creates, destroys := countMutations(ops, render.OpCreate), countMutations(ops, render.OpDestroy); creates != 0 || destroys != 0 {
		t.Fatalf("locale switch rebuilt controls: creates=%d destroys=%d ops=%+v", creates, destroys, ops)
	}
	if got := app.Root().Props.String("Text"); got != "FluxVCL 无障碍与国际化" {
		t.Fatalf("localized title = %q", got)
	}

	category := elementByKey(t, app.Root(), "category")
	items, _ := category.Props.Get("Items")
	if got := items.([]string); len(got) != 3 || got[2] != "研究" ||
		category.Props.Int("SelectedIndex") != 2 || mock.SelectedIndex(category.Handle) != 2 {
		t.Fatalf("localized category/state = %v props=%d renderer=%d", got,
			category.Props.Int("SelectedIndex"), mock.SelectedIndex(category.Handle))
	}
	grid := elementByKey(t, app.Root(), "assignments")
	name, description, value := mock.AccessibleSnapshot(grid.Handle)
	if name != "任务分配表" || description == "" || value != "第 3 行，第 2 列" {
		t.Fatalf("localized grid accessibility = %q/%q/%q", name, description, value)
	}
}

func TestDefaultAndCancelButtonContract(t *testing.T) {
	mock := render.NewMock()
	mock.SetClientSize(680, 440)
	app := flux.NewApp(mock)
	model := newDemoModel(app)
	if err := app.Mount(model.build); err != nil {
		t.Fatal(err)
	}

	save := elementByKey(t, app.Root(), "save")
	reset := elementByKey(t, app.Root(), "reset")
	_, _, defaultButton, _ := mock.FocusSnapshot(save.Handle)
	_, _, _, cancelButton := mock.FocusSnapshot(reset.Handle)
	if !defaultButton || !cancelButton {
		t.Fatalf("default/cancel semantics = %v/%v", defaultButton, cancelButton)
	}

	fireProperty(t, save, "OnClick", flux.Event{})
	if got := elementByKey(t, app.Root(), "save").Props.String("Text"); got != "Save (1)" {
		t.Fatalf("default action caption = %q", got)
	}
	fireProperty(t, elementByKey(t, app.Root(), "project-name"), "OnChange", "Changed")
	fireProperty(t, elementByKey(t, app.Root(), "reset"), "OnClick", flux.Event{})
	if got := model.name.Get(); got != "Ada Lovelace" {
		t.Fatalf("cancel action did not reset input: %q", got)
	}
}
