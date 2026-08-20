package render

import (
	"reflect"
	"testing"
)

func drawBenchmarkList(count int) DrawList {
	ops := make([]DrawOp, count)
	for index := range ops {
		ops[index] = FillRect(
			Rect{X: index, Y: index, W: 20, H: 20},
			FillStyle{Color: RGB(20, 80, 160)},
		)
	}
	return MustDrawList(ops...)
}

func BenchmarkDrawListBuild(b *testing.B) {
	ops := drawBenchmarkList(64).Ops()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := NewDrawList(ops...); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDrawListEqual(b *testing.B) {
	left, right := drawBenchmarkList(64), drawBenchmarkList(64)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if !left.Equal(right) {
			b.Fatal("lists differ")
		}
	}
}

func BenchmarkDrawListDeepEqual(b *testing.B) {
	left, right := drawBenchmarkList(64), drawBenchmarkList(64)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if !reflect.DeepEqual(left, right) {
			b.Fatal("lists differ")
		}
	}
}

func BenchmarkDrawListHash(b *testing.B) {
	list := drawBenchmarkList(64)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if drawListHash(list) == 0 {
			b.Fatal("unexpected zero hash")
		}
	}
}

func TestDrawListHashIsDeterministicButNotEquality(t *testing.T) {
	left, right := drawBenchmarkList(8), drawBenchmarkList(8)
	if drawListHash(left) != drawListHash(right) || !left.Equal(right) {
		t.Fatal("equal DrawLists must have the same hash")
	}
	changed := MustDrawList(FillRect(Rect{W: 1, H: 1}, FillStyle{Color: RGB(1, 2, 3)}))
	if drawListHash(left) == drawListHash(changed) {
		t.Fatal("hash sample did not distinguish changed input")
	}
}
