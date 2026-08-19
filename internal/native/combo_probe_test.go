//go:build windows && !race

package native

import (
	"syscall"
	"testing"

	"github.com/energye/lcl/lcl"
	"github.com/energye/lcl/types"
	"github.com/energye/lcl/types/keys"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

var procComboProbeSendMessage = syscall.NewLazyDLL("user32.dll").NewProc("SendMessageW")

func TestComboBoxNativeKeyboardSelection(t *testing.T) {
	if err := Init(radioProbeDLL(t)); err != nil {
		t.Fatal(err)
	}
	r := NewRenderer()
	parent := r.Create("Window")
	h := r.Create("ComboBox")
	r.SetParent(h, parent)
	r.SetItems(h, []string{"Design", "Engineering", "Research"})
	r.SetSelectedIndex(h, 1)

	combo, ok := r.controls[h].(comboBoxControl)
	if !ok {
		t.Fatalf("ComboBox native control = %T", r.controls[h])
	}
	if raw, ok := r.controls[h].(lcl.IComboBox); !ok || raw.Style() != types.CsDropDownList {
		t.Fatalf("ComboBox style = %v, want CsDropDownList", raw.Style())
	}

	calls := make([]int, 0, 1)
	r.OnSelectionChange(h, func(index int) { calls = append(calls, index) })
	var keyEvents []uint16
	r.SetEvent(h, "OnKeyDown", func(event render.Event) {
		if event.Type != render.EventKeyDown {
			t.Errorf("OnKeyDown type = %v", event.Type)
		}
		keyEvents = append(keyEvents, event.Key)
	})
	r.SetEvent(h, "OnKeyUp", func(event render.Event) {
		if event.Type != render.EventKeyUp {
			t.Errorf("OnKeyUp type = %v", event.Type)
		}
		keyEvents = append(keyEvents, event.Key)
	})
	r.SetSelectedIndex(h, 1)
	if len(calls) != 0 {
		t.Fatalf("controlled SetSelectedIndex triggered %d user callbacks", len(calls))
	}

	win, ok := r.controls[h].(lcl.IWinControl)
	if !ok {
		t.Fatalf("ComboBox does not implement IWinControl: %T", r.controls[h])
	}
	win.HandleNeeded()
	if hwnd := win.Handle(); hwnd == 0 {
		t.Fatal("ComboBox did not allocate an HWND")
	} else {
		sendComboProbeKey(t, hwnd, keys.VkDown)
	}
	if got := combo.ItemIndex(); got != 2 {
		t.Fatalf("VK_DOWN selected native index %d, want 2", got)
	}
	if len(calls) != 1 || calls[0] != 2 {
		t.Fatalf("VK_DOWN callbacks = %v, want [2]", calls)
	}
	if len(keyEvents) != 2 || keyEvents[0] != keys.VkDown || keyEvents[1] != keys.VkDown {
		t.Fatalf("public keyboard callbacks = %v, want [%d %d]", keyEvents, keys.VkDown, keys.VkDown)
	}

	// OnChange and OnSelect may both have fired above; a final keyboard check
	// must therefore be idempotent rather than report the same selection twice.
	r.emitComboSelection(h)
	if len(calls) != 1 {
		t.Fatalf("duplicate native selection callback: %v", calls)
	}
}

func TestNativeEscapeRestoresAlign(t *testing.T) {
	if err := Init(radioProbeDLL(t)); err != nil {
		t.Fatal(err)
	}
	r := NewRenderer()
	parent := r.Create("Window")
	h := r.Create("Button")
	r.SetParent(h, parent)
	r.SetBounds(h, render.Rect{X: 17, Y: 23, W: 113, H: 37})
	wantBounds := r.controls[h].BoundsRect()

	r.ApplyNative(h, func(obj any) {
		control, ok := obj.(lcl.IControl)
		if !ok {
			t.Fatalf("Native object = %T, want lcl.IControl", obj)
		}
		control.SetAlign(types.AlClient)
	})
	if got := r.controls[h].Align(); got != types.AlNone {
		t.Fatalf("Native Align after callback = %v, want AlNone", got)
	}
	if got := r.controls[h].BoundsRect(); got != wantBounds {
		t.Fatalf("Native bounds after callback = %+v, want %+v", got, wantBounds)
	}
}

func sendComboProbeKey(t *testing.T, hwnd types.HWND, key uint16) {
	t.Helper()
	const (
		wmKeyDown = 0x0100
		wmKeyUp   = 0x0101
	)
	for _, message := range []uintptr{wmKeyDown, wmKeyUp} {
		_, _, err := procComboProbeSendMessage.Call(uintptr(hwnd), message, uintptr(key), 0)
		if err != syscall.Errno(0) {
			t.Fatalf("SendMessageW(%#x) failed: %v", message, err)
		}
	}
}
