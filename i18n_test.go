package flux_test

import (
	"errors"
	"fmt"
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

func TestCatalogLookupFallbackAndMissing(t *testing.T) {
	catalog, err := flux.NewCatalog("en", flux.Resources{
		"en": {
			"empty": "",
			"hello": "Hello",
			"items": "%d items",
		},
		"fr": {
			"hello": "Bonjour",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		locale flux.Locale
		id     flux.MessageID
		want   string
		ok     bool
	}{
		{name: "exact", locale: "fr", id: "hello", want: "Bonjour", ok: true},
		{name: "missing message falls back", locale: "fr", id: "items", want: "%d items", ok: true},
		{name: "missing locale falls back", locale: "de", id: "hello", want: "Hello", ok: true},
		{name: "empty translation is present", locale: "en", id: "empty", want: "", ok: true},
		{name: "missing key", locale: "fr", id: "unknown", want: "", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := catalog.Lookup(tt.locale, tt.id)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("Lookup(%q, %q) = (%q, %v), want (%q, %v)",
					tt.locale, tt.id, got, ok, tt.want, tt.ok)
			}
		})
	}
	if got := catalog.Format("fr", "unknown", 1); got != "unknown" {
		t.Fatalf("missing Format = %q, want message ID", got)
	}
}

func TestCatalogDefensiveCopies(t *testing.T) {
	resources := flux.Resources{
		"en": {"hello": "Hello"},
	}
	catalog, err := flux.NewCatalog("en", resources)
	if err != nil {
		t.Fatal(err)
	}

	resources["en"]["hello"] = "changed"
	resources["en"] = flux.Messages{"hello": "replaced"}
	resources["fr"] = flux.Messages{"hello": "Bonjour"}
	if got := catalog.Format("en", "hello"); got != "Hello" {
		t.Fatalf("input map mutation changed Catalog: got %q", got)
	}
	if _, ok := catalog.Resources()["fr"]; ok {
		t.Fatal("input map replacement added a locale to Catalog")
	}

	snapshot := catalog.Resources()
	snapshot["en"]["hello"] = "snapshot changed"
	snapshot["en"] = flux.Messages{"hello": "snapshot replaced"}
	if got := catalog.Format("en", "hello"); got != "Hello" {
		t.Fatalf("Resources result mutation changed Catalog: got %q", got)
	}
	if got := catalog.Fallback(); got != "en" {
		t.Fatalf("Fallback = %q, want en", got)
	}
}

func TestCatalogFormatting(t *testing.T) {
	catalog, err := flux.NewCatalog("en", flux.Resources{
		"en": {
			"progress": "%s: %d%%",
			"raw":      "100% ready",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Format("en", "progress", "Build", 75); got != "Build: 75%" {
		t.Fatalf("Format = %q, want %q", got, "Build: 75%")
	}
	if got := catalog.Format("en", "raw"); got != "100% ready" {
		t.Fatalf("Format without args = %q, want unchanged template", got)
	}
}

func TestCatalogValidationIsDeterministic(t *testing.T) {
	restore := flux.SetDiagnosticLocale("en")
	defer restore()
	first := flux.Resources{
		"z": {"": "bad"},
		"":  {"": "bad"},
	}
	second := flux.Resources{}
	second[""] = flux.Messages{"": "bad"}
	second["z"] = flux.Messages{"": "bad"}

	_, errA := flux.NewCatalog("en", first)
	_, errB := flux.NewCatalog("en", second)
	if !errors.Is(errA, flux.ErrInvalidCatalog) || !errors.Is(errB, flux.ErrInvalidCatalog) {
		t.Fatalf("validation errors must wrap ErrInvalidCatalog: %v / %v", errA, errB)
	}
	if errA.Error() != errB.Error() {
		t.Fatalf("validation order differs:\nfirst:  %v\nsecond: %v", errA, errB)
	}
	want := "flux: invalid i18n catalog: fallback locale \"en\" is not registered; " +
		"locale is empty; locale \"\" contains an empty message ID; " +
		"locale \"z\" contains an empty message ID"
	if errA.Error() != want {
		t.Fatalf("validation error = %q, want %q", errA, want)
	}
}

func TestFrameworkDiagnosticsUseReplaceableCatalog(t *testing.T) {
	restoreChinese := flux.SetDiagnosticLocale("zh-CN")
	if got := panicText(func() { flux.Step(0) }); got != "flux.Step: value 必须 > 0" {
		t.Fatalf("built-in Chinese diagnostic = %q", got)
	}
	if got := flux.ErrPluginInvalid.Error(); got != "flux: 插件定义无效" {
		t.Fatalf("built-in Chinese sentinel = %q", got)
	}
	restoreChinese()

	restoreEnglish := flux.SetDiagnosticLocale("en")
	if got := panicText(func() { flux.Step(0) }); got != "flux.Step: value must be > 0" {
		t.Fatalf("built-in English diagnostic = %q", got)
	}
	if got := flux.ErrPluginInvalid.Error(); got != "flux: invalid plugin definition" {
		t.Fatalf("built-in English sentinel = %q", got)
	}
	restoreEnglish()

	custom := flux.MustCatalog("test", flux.Resources{
		"test": {
			flux.DiagnosticStepValuePositive: "CUSTOM STEP",
			flux.DiagnosticErrInvalidCatalog: "CUSTOM CATALOG",
		},
	})
	restoreCustom := flux.SetDiagnosticCatalog(custom, "test")
	defer restoreCustom()
	if got := panicText(func() { flux.Step(0) }); got != "CUSTOM STEP" {
		t.Fatalf("custom diagnostic = %q", got)
	}
	if got := flux.DiagnosticText(flux.DiagnosticGridRows); got == string(flux.DiagnosticGridRows) {
		t.Fatal("missing custom diagnostic did not fall back to built-in resources")
	}
	if got := flux.ErrInvalidCatalog.Error(); got != "CUSTOM CATALOG" {
		t.Fatalf("custom sentinel diagnostic = %q", got)
	}
	if !errors.Is(fmt.Errorf("wrapped: %w", flux.ErrInvalidCatalog), flux.ErrInvalidCatalog) {
		t.Fatal("localized sentinel no longer preserves errors.Is identity")
	}
}

func TestGridAndPaintDiagnosticsAreFullyReplaceable(t *testing.T) {
	catalog := flux.MustCatalog("test", flux.Resources{
		"test": {
			flux.DiagnosticGridCellsRowCount:    "GRID ROWS %d/%d",
			flux.DiagnosticGridCellsColumnCount: "GRID COLS %d/%d/%d",
			flux.DiagnosticPaintCircleRadius:    "PAINT RADIUS %d",
		},
	})
	restore := flux.SetDiagnosticCatalog(catalog, "test")
	defer restore()

	if got := panicText(func() {
		_ = flux.StringGrid(2, 1, flux.Cells([][]string{{"only one row"}}))
	}); got != "GRID ROWS 1/2" {
		t.Fatalf("localized grid row diagnostic = %q", got)
	}
	if got := panicText(func() {
		_ = flux.StringGrid(1, 2, flux.Cells([][]string{{"only one column"}}))
	}); got != "GRID COLS 0/1/2" {
		t.Fatalf("localized grid column diagnostic = %q", got)
	}
	if got := panicText(func() {
		_ = flux.PaintBox([]flux.PaintCommand{{
			Kind: flux.PaintCircle, FillColor: flux.RGB(1, 2, 3),
		}})
	}); got != "PAINT RADIUS 0" {
		t.Fatalf("localized PaintBox diagnostic = %q", got)
	}
}

func panicText(fn func()) (text string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			text = recovered.(string)
		}
	}()
	fn()
	return ""
}

func TestMessageBindingLocaleSwitchPatchesTextAndPreservesInput(t *testing.T) {
	catalog, err := flux.NewCatalog("en", flux.Resources{
		"en": {
			"action": "Save",
			"status": "Editing %s",
		},
		"fr": {
			"action": "Enregistrer",
			"status": "Modification de %s",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	mock := render.NewMock()
	app := flux.NewApp(mock)
	locale := flux.NewState[flux.Locale]("en")
	if err := app.Mount(func() flux.Widget {
		return flux.Window(flux.Column(
			flux.Input(flux.Key("editor")),
			flux.Text(catalog.Bind(locale, "status", "report"), flux.Key("status")),
			flux.Button(catalog.Bind(locale, "action"), flux.Key("action")),
		))
	}); err != nil {
		t.Fatal(err)
	}

	inputHandle := findByKey(t, app.Root(), "editor").Handle
	statusHandle := findByKey(t, app.Root(), "status").Handle
	actionHandle := findByKey(t, app.Root(), "action").Handle
	base := len(mock.Ops())

	locale.Set("fr")

	if got := findByKey(t, app.Root(), "editor").Handle; got != inputHandle {
		t.Fatalf("locale switch rebuilt Input: %d -> %d", inputHandle, got)
	}
	if got := findByKey(t, app.Root(), "status").Props.String("Text"); got != "Modification de report" {
		t.Fatalf("localized Text = %q", got)
	}
	if got := findByKey(t, app.Root(), "action").Props.String("Text"); got != "Enregistrer" {
		t.Fatalf("localized Button = %q", got)
	}

	ops := mock.Ops()[base:]
	if countOps(ops, render.OpCreate) != 0 || countOps(ops, render.OpDestroy) != 0 {
		t.Fatalf("locale switch rebuilt controls: %+v", ops)
	}
	if !hasOp(ops, render.OpSetText, statusHandle, "", "Modification de report") {
		t.Fatalf("localized Text was not patched: %+v", ops)
	}
	if !hasOp(ops, render.OpSetText, actionHandle, "", "Enregistrer") {
		t.Fatalf("localized Button was not patched: %+v", ops)
	}
}
