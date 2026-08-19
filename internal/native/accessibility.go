package native

import (
	"github.com/energye/lcl/lcl"
	"github.com/energye/lcl/types"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

func (r *Renderer) SetAccessibleName(h render.Handle, name string) {
	if control := r.controls[h]; control != nil {
		control.SetAccessibleName(name)
	}
}

func (r *Renderer) SetAccessibleDescription(h render.Handle, description string) {
	if control := r.controls[h]; control != nil {
		control.SetAccessibleDescription(description)
	}
}

func (r *Renderer) SetAccessibleValue(h render.Handle, value string) {
	if control := r.controls[h]; control != nil {
		control.SetAccessibleValue(value)
	}
}

func (r *Renderer) SetTabOrder(h render.Handle, order int) {
	order = clampTabOrder(order)
	if r.tabOrders == nil {
		r.tabOrders = make(map[render.Handle]int)
	}
	r.tabOrders[h] = order
	if radio := r.radios[h]; radio != nil {
		radio.tabOrder = order
		if radio.host != nil {
			radio.host.panel.SetTabOrder(types.TTabOrder(order))
		}
		if control, ok := r.controls[h].(lcl.IWinControl); ok {
			control.SetTabOrder(0)
		}
		return
	}
	r.applyTabOrder(h)
}

func (r *Renderer) applyTabOrder(h render.Handle) {
	if control, ok := r.controls[h].(lcl.IWinControl); ok {
		control.SetTabOrder(types.TTabOrder(clampTabOrder(r.tabOrders[h])))
	}
}

func clampTabOrder(order int) int {
	if order < 0 {
		return 0
	}
	const maximum = int(^uint16(0) >> 1)
	if order > maximum {
		return maximum
	}
	return order
}

func (r *Renderer) SetTabStop(h render.Handle, enabled bool) {
	if control, ok := r.controls[h].(lcl.IWinControl); ok {
		control.SetTabStop(enabled)
	}
}

func (r *Renderer) ResetTabStop(h render.Handle) {
	if control, ok := r.controls[h].(lcl.IWinControl); ok {
		control.SetTabStop(r.tabStopDefaults[h])
	}
}

func (r *Renderer) SetDefaultButton(h render.Handle, enabled bool) {
	if button, ok := r.controls[h].(lcl.ICustomButton); ok {
		button.SetDefault(enabled)
	}
}

func (r *Renderer) SetCancelButton(h render.Handle, enabled bool) {
	if button, ok := r.controls[h].(lcl.ICustomButton); ok {
		button.SetCancel(enabled)
	}
}

var (
	_ render.AccessibilityController = (*Renderer)(nil)
	_ render.TabOrderController      = (*Renderer)(nil)
)
