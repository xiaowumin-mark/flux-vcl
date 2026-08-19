package flux_test

import (
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

func TestAccessibilityPropertiesPatchRemoveAndDefaultActions(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	build := func(configured bool) flux.Widget {
		opts := []flux.Opt{flux.Key("editor")}
		buttonOpts := []flux.Opt{flux.Key("action")}
		if configured {
			opts = append(opts,
				flux.AccessibleName("Report title"),
				flux.AccessibleDescription("Required field"),
				flux.AccessibleValue("Quarterly report"),
				flux.TabStop(false),
			)
			buttonOpts = append(buttonOpts, flux.DefaultButton(true), flux.CancelButton(true))
		}
		return flux.Window(flux.Column(
			flux.Input(opts...),
			flux.Button("Apply", buttonOpts...),
		))
	}

	if err := app.Render(build(true)); err != nil {
		t.Fatal(err)
	}
	editor := findByKey(t, app.Root(), "editor")
	action := findByKey(t, app.Root(), "action")
	name, description, value := mock.AccessibleSnapshot(editor.Handle)
	if name != "Report title" || description != "Required field" || value != "Quarterly report" {
		t.Fatalf("accessible snapshot = %q/%q/%q", name, description, value)
	}
	_, tabStop, _, _ := mock.FocusSnapshot(editor.Handle)
	_, _, defaultButton, cancelButton := mock.FocusSnapshot(action.Handle)
	if tabStop || !defaultButton || !cancelButton {
		t.Fatalf("focus semantics tabStop/default/cancel = %v/%v/%v", tabStop, defaultButton, cancelButton)
	}

	editorHandle, actionHandle := editor.Handle, action.Handle
	if err := app.Render(build(false)); err != nil {
		t.Fatal(err)
	}
	if findByKey(t, app.Root(), "editor").Handle != editorHandle ||
		findByKey(t, app.Root(), "action").Handle != actionHandle {
		t.Fatal("removing accessibility properties rebuilt a stateful control")
	}
	name, description, value = mock.AccessibleSnapshot(editorHandle)
	_, tabStop, _, _ = mock.FocusSnapshot(editorHandle)
	_, _, defaultButton, cancelButton = mock.FocusSnapshot(actionHandle)
	if name != "" || description != "" || value != "" || !tabStop || defaultButton || cancelButton {
		t.Fatalf("removed properties did not restore defaults: %q/%q/%q %v/%v/%v",
			name, description, value, tabStop, defaultButton, cancelButton)
	}
}

func TestDeclarativeTabOrderFollowsKeyedReorderWithoutRebuild(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	build := func(reverse bool) flux.Widget {
		first := flux.Input(flux.Key("first"), flux.AccessibleName("First"))
		second := flux.Button("Second", flux.Key("second"))
		children := []any{first, second}
		if reverse {
			children = []any{second, first}
		}
		return flux.Window(flux.Column(
			flux.Text("Heading"),
			flux.Row(children...),
			flux.PaintBox(nil, flux.Key("paint")),
			flux.StringGrid(1, 1, flux.Key("grid"), flux.AccessibleName("Results")),
		))
	}

	if err := app.Render(build(false)); err != nil {
		t.Fatal(err)
	}
	first := findByKey(t, app.Root(), "first")
	second := findByKey(t, app.Root(), "second")
	grid := findByKey(t, app.Root(), "grid")
	firstHandle, secondHandle := first.Handle, second.Handle
	firstOrder, _, _, _ := mock.FocusSnapshot(firstHandle)
	secondOrder, _, _, _ := mock.FocusSnapshot(secondHandle)
	gridOrder, _, _, _ := mock.FocusSnapshot(grid.Handle)
	if firstOrder != 0 || secondOrder != 1 || gridOrder != 2 {
		t.Fatalf("initial Tab order first=%d second=%d grid=%d", firstOrder, secondOrder, gridOrder)
	}

	if err := app.Render(build(true)); err != nil {
		t.Fatal(err)
	}
	if findByKey(t, app.Root(), "first").Handle != firstHandle ||
		findByKey(t, app.Root(), "second").Handle != secondHandle {
		t.Fatal("keyed reorder rebuilt controls")
	}
	firstOrder, _, _, _ = mock.FocusSnapshot(firstHandle)
	secondOrder, _, _, _ = mock.FocusSnapshot(secondHandle)
	if secondOrder >= firstOrder {
		t.Fatalf("reordered Tab order first=%d second=%d", firstOrder, secondOrder)
	}

	base := len(mock.Ops())
	if err := app.Render(build(true)); err != nil {
		t.Fatal(err)
	}
	if ops := mock.Ops()[base:]; len(ops) != 0 {
		t.Fatalf("identical accessibility tree produced mutations: %+v", ops)
	}
}

type accessibilityCapabilitylessRenderer struct{ render.Renderer }

func TestAccessibilityCapabilityIsOptional(t *testing.T) {
	base := render.NewMock()
	app := flux.NewApp(accessibilityCapabilitylessRenderer{Renderer: base})
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("missing optional accessibility capability panicked: %v", recovered)
		}
	}()
	if err := app.Render(flux.Window(flux.Input(
		flux.AccessibleName("Name"), flux.TabStop(false),
	))); err != nil {
		t.Fatal(err)
	}
}
