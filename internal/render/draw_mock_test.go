package render

import "testing"

func TestDrawControllerMockSeparatesSetResetAndInvalidate(t *testing.T) {
	mock := NewMock()
	h := mock.Create("DrawSurface")
	list := MustDrawList(Clear(RGB(1, 2, 3)))
	mock.SetDrawList(h, list)
	if !mock.DrawList(h).Equal(list) || mock.DrawInvalidations(h) != 0 {
		t.Fatalf("SetDrawList list=%v invalidations=%d", mock.DrawList(h), mock.DrawInvalidations(h))
	}
	mock.InvalidateDraw(h)
	if mock.DrawInvalidations(h) != 1 {
		t.Fatalf("InvalidateDraw count=%d", mock.DrawInvalidations(h))
	}
	mock.ResetDrawList(h)
	if mock.DrawList(h).Len() != 0 || mock.DrawInvalidations(h) != 1 {
		t.Fatalf("ResetDrawList list=%v invalidations=%d", mock.DrawList(h), mock.DrawInvalidations(h))
	}
}

func TestDrawControllerMockDefensiveCopy(t *testing.T) {
	mock := NewMock()
	h := mock.Create("DrawSurface")
	list := MustDrawList(Clear(RGB(1, 2, 3)))
	mock.SetDrawList(h, list)
	returned := mock.DrawList(h)
	ops := returned.Ops()
	ops[0] = Clear(RGB(3, 2, 1))
	if !mock.DrawList(h).Equal(list) {
		t.Fatal("mutating returned DrawList changed Mock state")
	}
}
