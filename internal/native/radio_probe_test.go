//go:build windows && !race

package native

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// TestRadioButtonNativeProbe verifies Flux's default logical group: two radios below
// one resolved native parent remain mutually exclusive.
func TestRadioButtonNativeProbe(t *testing.T) {
	dll := radioProbeDLL(t)
	if err := Init(dll); err != nil {
		t.Fatal(err)
	}

	r := NewRenderer()
	parent := r.Create("Window")
	left := r.Create("RadioButton")
	right := r.Create("RadioButton")
	r.SetParent(left, parent)
	r.SetParent(right, parent)

	r.SetChecked(left, true)
	assertNativeChecked(t, r, left, true)
	assertNativeChecked(t, r, right, false)
	r.SetChecked(right, true)
	assertNativeChecked(t, r, left, false)
	assertNativeChecked(t, r, right, true)
}

// TestRadioButtonNativeGroupsAreIndependent verifies that GroupIndex is Flux-owned:
// distinct groups below the same resolved native parent can both remain selected.
func TestRadioButtonNativeGroupsAreIndependent(t *testing.T) {
	dll := radioProbeDLL(t)
	if err := Init(dll); err != nil {
		t.Fatal(err)
	}

	r := NewRenderer()
	parent := r.Create("Window")
	left := r.Create("RadioButton")
	right := r.Create("RadioButton")
	r.SetParent(left, parent)
	r.SetParent(right, parent)
	r.SetGroupIndex(left, 1)
	r.SetGroupIndex(right, 2)

	r.SetChecked(left, true)
	r.SetChecked(right, true)
	assertNativeChecked(t, r, left, true)
	assertNativeChecked(t, r, right, true)
}

func radioProbeDLL(t *testing.T) string {
	t.Helper()
	if configured := os.Getenv("FVCL_LIBENERGY_DLL"); configured != "" {
		if _, err := os.Stat(configured); err != nil {
			t.Fatalf("FVCL_LIBENERGY_DLL 不可用: %v", err)
		}
		return configured
	}
	dll, err := filepath.Abs(filepath.Join("..", "..", "ref", "designer-lib", "libenergy-amd64.dll"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dll); err != nil {
		t.Skip("native LCL probe requires FVCL_LIBENERGY_DLL or a local ref DLL")
	}
	return dll
}

func assertNativeChecked(t *testing.T, r *Renderer, h render.Handle, want bool) {
	t.Helper()
	if got := nativeChecked(t, r, h); got != want {
		t.Fatalf("control %d checked=%v, want %v", h, got, want)
	}
}

func nativeChecked(t *testing.T, r *Renderer, h render.Handle) bool {
	t.Helper()
	c, ok := r.controls[h].(checkableControl)
	if !ok {
		t.Fatalf("control %d is not checkable", h)
	}
	return c.Checked()
}
