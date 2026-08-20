//go:build windows

package native

import (
	"testing"
	"unsafe"
)

// These test-only declarations mirror the Win32 records consumed by
// WM_DRAWITEM and NM_CUSTOMDRAW. Keeping them local to the CD0 probe avoids
// presenting an unverified binding as production API before CD5/CD6.
type cd0Rect struct {
	Left, Top, Right, Bottom int32
}

type cd0DrawItemStruct struct {
	CtlType    uint32
	CtlID      uint32
	ItemID     uint32
	ItemAction uint32
	ItemState  uint32
	HwndItem   uintptr
	HDC        uintptr
	RcItem     cd0Rect
	ItemData   uintptr
}

type cd0NMHDR struct {
	HwndFrom uintptr
	IDFrom   uintptr
	Code     uint32
}

type cd0NMCustomDraw struct {
	Hdr        cd0NMHDR
	DrawStage  uint32
	HDC        uintptr
	Rc         cd0Rect
	ItemSpec   uintptr
	ItemState  uint32
	ItemLParam uintptr
}

func TestCD0Win32DrawABILayout(t *testing.T) {
	type layout struct {
		pointerSize uintptr
		drawSize    uintptr
		nmhdrSize   uintptr
		customSize  uintptr
		drawOffsets []uintptr
		nmhdrOffset []uintptr
		customOff   []uintptr
	}

	layouts := map[uintptr]layout{
		4: {
			pointerSize: 4,
			drawSize:    48,
			nmhdrSize:   12,
			customSize:  48,
			drawOffsets: []uintptr{0, 4, 8, 12, 16, 20, 24, 28, 44},
			nmhdrOffset: []uintptr{0, 4, 8},
			customOff:   []uintptr{0, 12, 16, 20, 36, 40, 44},
		},
		8: {
			pointerSize: 8,
			drawSize:    64,
			nmhdrSize:   24,
			customSize:  80,
			drawOffsets: []uintptr{0, 4, 8, 12, 16, 24, 32, 40, 56},
			nmhdrOffset: []uintptr{0, 8, 16},
			customOff:   []uintptr{0, 24, 32, 40, 56, 64, 72},
		},
	}

	pointerSize := unsafe.Sizeof(uintptr(0))
	want, ok := layouts[pointerSize]
	if !ok {
		t.Fatalf("unsupported pointer size %d", pointerSize)
	}
	if pointerSize != want.pointerSize {
		t.Fatalf("pointer size = %d, want %d", pointerSize, want.pointerSize)
	}

	var draw cd0DrawItemStruct
	assertCD0Layout(t, "DRAWITEMSTRUCT", unsafe.Sizeof(draw), want.drawSize,
		[]uintptr{
			unsafe.Offsetof(draw.CtlType), unsafe.Offsetof(draw.CtlID),
			unsafe.Offsetof(draw.ItemID), unsafe.Offsetof(draw.ItemAction),
			unsafe.Offsetof(draw.ItemState), unsafe.Offsetof(draw.HwndItem),
			unsafe.Offsetof(draw.HDC), unsafe.Offsetof(draw.RcItem),
			unsafe.Offsetof(draw.ItemData),
		}, want.drawOffsets)

	var header cd0NMHDR
	assertCD0Layout(t, "NMHDR", unsafe.Sizeof(header), want.nmhdrSize,
		[]uintptr{
			unsafe.Offsetof(header.HwndFrom), unsafe.Offsetof(header.IDFrom),
			unsafe.Offsetof(header.Code),
		}, want.nmhdrOffset)

	var custom cd0NMCustomDraw
	assertCD0Layout(t, "NMCUSTOMDRAW", unsafe.Sizeof(custom), want.customSize,
		[]uintptr{
			unsafe.Offsetof(custom.Hdr), unsafe.Offsetof(custom.DrawStage),
			unsafe.Offsetof(custom.HDC), unsafe.Offsetof(custom.Rc),
			unsafe.Offsetof(custom.ItemSpec), unsafe.Offsetof(custom.ItemState),
			unsafe.Offsetof(custom.ItemLParam),
		}, want.customOff)
}

func assertCD0Layout(t *testing.T, name string, gotSize, wantSize uintptr, gotOffsets, wantOffsets []uintptr) {
	t.Helper()
	if gotSize != wantSize {
		t.Errorf("%s size = %d, want %d", name, gotSize, wantSize)
	}
	if len(gotOffsets) != len(wantOffsets) {
		t.Fatalf("%s offset count = %d, want %d", name, len(gotOffsets), len(wantOffsets))
	}
	for i := range gotOffsets {
		if gotOffsets[i] != wantOffsets[i] {
			t.Errorf("%s field %d offset = %d, want %d", name, i, gotOffsets[i], wantOffsets[i])
		}
	}
}
