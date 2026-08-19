//go:build windows && !race

package native

import (
	"testing"

	"github.com/energye/lcl/lcl"
	"github.com/energye/lcl/types"
	"github.com/energye/lcl/types/colors"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

func TestAccessibilityNativeProbe(t *testing.T) {
	if err := Init(radioProbeDLL(t)); err != nil {
		t.Fatal(err)
	}
	r := NewRenderer()
	parent := r.Create("Window")

	inputHandle := r.Create("Input")
	r.SetParent(inputHandle, parent)
	input := r.controls[inputHandle]
	r.SetAccessibleName(inputHandle, "Account name")
	r.SetAccessibleDescription(inputHandle, "Required")
	r.SetAccessibleValue(inputHandle, "Ada")
	if input.AccessibleName() != "Account name" || input.AccessibleDescription() != "Required" ||
		input.AccessibleValue() != "Ada" {
		t.Fatalf("LCL accessible properties = %q/%q/%q", input.AccessibleName(),
			input.AccessibleDescription(), input.AccessibleValue())
	}
	winInput, ok := input.(lcl.IWinControl)
	if !ok {
		t.Fatal("Input does not implement IWinControl")
	}
	defaultTabStop := winInput.TabStop()
	buttonHandle := r.Create("Button")
	r.SetParent(buttonHandle, parent)
	r.SetTabOrder(buttonHandle, 0)
	r.SetTabOrder(inputHandle, 1)
	r.SetTabStop(inputHandle, !defaultTabStop)
	if winInput.TabOrder() != 1 || winInput.TabStop() == defaultTabStop {
		t.Fatalf("Input TabOrder/TabStop = %d/%v", winInput.TabOrder(), winInput.TabStop())
	}
	r.ResetTabStop(inputHandle)
	if winInput.TabStop() != defaultTabStop {
		t.Fatalf("Input TabStop reset = %v, want %v", winInput.TabStop(), defaultTabStop)
	}

	r.SetDefaultButton(buttonHandle, true)
	r.SetCancelButton(buttonHandle, true)
	button, ok := r.controls[buttonHandle].(lcl.ICustomButton)
	if !ok || !button.Default() || !button.Cancel() {
		t.Fatalf("Button default/cancel not applied: %T %v/%v", r.controls[buttonHandle],
			button.Default(), button.Cancel())
	}

	radioHandle := r.Create("RadioButton")
	// Set the order before parenting to cover the render sequence used by diff.
	// LCL normalizes out-of-range values, so this is the next valid sibling index.
	r.SetTabOrder(radioHandle, 2)
	r.SetParent(radioHandle, parent)
	radio := r.radios[radioHandle]
	if radio == nil || radio.host == nil || radio.host.panel.TabOrder() != 2 {
		t.Fatalf("RadioButton logical TabOrder host not synchronized: %+v", radio)
	}

	comboHandle := r.Create("ComboBox")
	r.SetParent(comboHandle, parent)
	r.SetItems(comboHandle, []string{"Design", "Engineering", "Research"})
	r.SetSelectedIndex(comboHandle, 2)
	r.SetItems(comboHandle, []string{"设计", "工程", "研究"})
	combo := r.controls[comboHandle].(comboBoxControl)
	if combo.ItemIndex() != 2 || combo.Items().Strings(2) != "研究" {
		t.Fatalf("localized ComboBox selection = %d/%q, want 2/研究", combo.ItemIndex(), combo.Items().Strings(2))
	}

	gridHandle := r.Create("StringGrid")
	paintHandle := r.Create("PaintBox")
	r.SetAccessibleName(gridHandle, "Results grid")
	r.SetAccessibleName(paintHandle, "Drawing surface")
	if r.controls[gridHandle].AccessibleName() != "Results grid" ||
		r.controls[paintHandle].AccessibleName() != "Drawing surface" {
		t.Fatal("Grid/PaintBox accessible names were not forwarded to LCL")
	}

	custom := render.Color(0xFF123456)
	r.highContrast.Store(false)
	r.SetColor(inputHandle, custom)
	if got := input.Color(); got != colorToTColor(custom) {
		t.Fatalf("normal custom color = %#x", got)
	}
	r.highContrast.Store(true)
	r.SetColor(inputHandle, custom)
	r.SetFontColor(inputHandle, custom)
	if got := input.Color(); got != types.TColor(colors.ClDefault) {
		t.Fatalf("high contrast background = %#x, want system default", got)
	}
	if got := input.Font().Color(); got != types.TColor(colors.ClDefault) {
		t.Fatalf("high contrast font = %#x, want system default", got)
	}
	r.highContrast.Store(false)
	r.applyRequestedColor(inputHandle)
	r.applyRequestedFontColor(inputHandle)
	if got := input.Color(); got != colorToTColor(custom) {
		t.Fatalf("restored background = %#x, want declared color", got)
	}
	if got := input.Font().Color(); got != colorToTColor(custom) {
		t.Fatalf("restored font = %#x, want declared color", got)
	}
	r.SetColor(inputHandle, 0)
	if got := input.Color(); got != types.TColor(colors.ClDefault) {
		t.Fatalf("removed background = %#x, want system default", got)
	}
}
