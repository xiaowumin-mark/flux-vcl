//go:build windows && !race

package native

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/energye/lcl/lcl"
	"github.com/energye/lcl/types"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

const cd0ArtifactDirEnv = "FVCL_CD0_ARTIFACT_DIR"

var (
	cd0ProcSaveDC            = syscall.NewLazyDLL("gdi32.dll").NewProc("SaveDC")
	cd0ProcRestoreDC         = syscall.NewLazyDLL("gdi32.dll").NewProc("RestoreDC")
	cd0ProcIntersectClipRect = syscall.NewLazyDLL("gdi32.dll").NewProc("IntersectClipRect")
	cd0ProcGetTextFace       = syscall.NewLazyDLL("gdi32.dll").NewProc("GetTextFaceW")
)

type cd0ProbeEnvelope struct {
	Probe     string   `json:"probe"`
	Status    string   `json:"status"`
	GOOS      string   `json:"goos"`
	GOARCH    string   `json:"goarch"`
	GoVersion string   `json:"goVersion"`
	Evidence  any      `json:"evidence"`
	Notes     []string `json:"notes,omitempty"`
}

type cd0CanvasDPIEvidence struct {
	DPI           int   `json:"dpi"`
	FontHeightPX  int32 `json:"fontHeightPx"`
	TextWidthPX   int32 `json:"textWidthPx"`
	TextHeightPX  int32 `json:"textHeightPx"`
	TextWidthDIP  int   `json:"textWidthDip"`
	TextHeightDIP int   `json:"textHeightDip"`
}

type cd0CanvasEvidence struct {
	Width                int                    `json:"width"`
	Height               int                    `json:"height"`
	Samples              map[string]string      `json:"samples"`
	TextInkPixels        int                    `json:"textInkPixels"`
	HalfOpenFillRect     bool                   `json:"halfOpenFillRect"`
	ClipInsidePainted    bool                   `json:"clipInsidePainted"`
	ClipOutsidePreserved bool                   `json:"clipOutsidePreserved"`
	StrokeRectObserved   bool                   `json:"strokeRectObserved"`
	LineObserved         bool                   `json:"lineObserved"`
	FallbackRequested    string                 `json:"fallbackRequested"`
	FallbackResolved     string                 `json:"fallbackResolved"`
	FallbackTextExtent   [2]int32               `json:"fallbackTextExtent"`
	DPI                  []cd0CanvasDPIEvidence `json:"dpi"`
	PNG                  string                 `json:"png"`
}

func TestCD0CanvasRuntimeProbe(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := Init(radioProbeDLL(t)); err != nil {
		t.Fatal(err)
	}

	const width, height = 360, 280
	bmp := lcl.NewBitmap()
	if bmp == nil {
		t.Fatal("CD0 canvas probe: NewBitmap returned nil")
	}
	defer bmp.Free()
	bmp.SetSize(width, height)
	canvas := bmp.Canvas()
	if canvas == nil {
		t.Fatal("CD0 canvas probe: bitmap Canvas returned nil")
	}

	brush := canvas.BrushToBrush()
	pen := canvas.PenToPen()
	brush.SetStyle(types.BsSolid)
	brush.SetColor(cd0TColor(0xff, 0xff, 0xff))
	pen.SetStyle(types.PsClear)
	canvas.FillRectWithIntX4(0, 0, width, height)

	// The solid centers and boundary samples deliberately avoid antialiasing.
	brush.SetColor(cd0TColor(0x1f, 0x6f, 0xd1))
	canvas.FillRectWithIntX4(16, 16, 96, 64)

	brush.SetColor(cd0TColor(0x2e, 0x9d, 0x50))
	canvas.RoundRectWithIntX6(112, 16, 208, 64, 18, 18)

	brush.SetColor(cd0TColor(0xd1, 0x43, 0x43))
	canvas.EllipseWithIntX4(224, 16, 320, 64)

	font := canvas.FontToFont()
	font.SetName("Segoe UI")
	font.SetPixelsPerInch(96)
	font.SetHeight(-18)
	font.SetColor(cd0TColor(0x20, 0x20, 0x20))
	brush.SetStyle(types.BsClear)
	textRect := types.Rect(16, 82, 208, 128)
	canvas.TextRectWithRectIntX2StrTStyle(textRect, 18, 84, "FluxVCL CD0", lcl.TTextStyle{
		SingleLine: 1,
		Clipping:   1,
	})

	hdc := canvas.Handle()
	if hdc == 0 {
		t.Fatal("CD0 canvas probe: bitmap Canvas has no HDC")
	}
	saved, _, saveErr := cd0ProcSaveDC.Call(uintptr(hdc))
	if saved == 0 {
		t.Fatalf("CD0 canvas probe: SaveDC failed: %v", saveErr)
	}
	clipResult, _, clipErr := cd0ProcIntersectClipRect.Call(uintptr(hdc), 248, 94, 304, 126)
	if int32(clipResult) <= 0 {
		t.Fatalf("CD0 canvas probe: IntersectClipRect failed: result=%d err=%v", clipResult, clipErr)
	}
	brush.SetStyle(types.BsSolid)
	brush.SetColor(cd0TColor(0xb4, 0x3b, 0xb8))
	canvas.FillRectWithIntX4(224, 82, 336, 138)
	restored, _, restoreErr := cd0ProcRestoreDC.Call(uintptr(hdc), saved)
	if restored == 0 {
		t.Fatalf("CD0 canvas probe: RestoreDC failed: %v", restoreErr)
	}

	dpiEvidence := make([]cd0CanvasDPIEvidence, 0, 3)
	for _, dpi := range []int{96, 144, 192} {
		font.SetPixelsPerInch(int32(dpi))
		fontHeight := int32(render.DIPToPX(16, dpi))
		font.SetHeight(-fontHeight)
		extent := canvas.TextExtentWithStr("FluxVCL")
		dpiEvidence = append(dpiEvidence, cd0CanvasDPIEvidence{
			DPI:           dpi,
			FontHeightPX:  fontHeight,
			TextWidthPX:   extent.Cx,
			TextHeightPX:  extent.Cy,
			TextWidthDIP:  render.PXToDIP(int(extent.Cx), dpi),
			TextHeightDIP: render.PXToDIP(int(extent.Cy), dpi),
		})

		// Contact-sheet evidence uses one identical DIP font size materialized
		// at each target DPI. It is diagnostic only; the metric assertions below
		// remain numeric and do not compare antialiased glyph pixels.
		font.SetColor(cd0TColor(0x20, 0x20, 0x20))
		brush.SetStyle(types.BsClear)
		x := int32(112 + (len(dpiEvidence)-1)*80)
		canvas.TextOutWithIntX2Str(x, 158, fmt.Sprintf("%d", dpi))
	}

	brush.SetStyle(types.BsClear)
	pen.SetStyle(types.PsSolid)
	pen.SetColor(cd0TColor(0x7b, 0x3f, 0x00))
	pen.SetWidth(3)
	canvas.RectangleWithIntX4(16, 146, 96, 196)
	canvas.LineWithIntX4(16, 218, 96, 218)

	const missingFamily = "__FluxVCL_CD0_Missing_Font__"
	font.SetName(missingFamily)
	font.SetPixelsPerInch(96)
	font.SetHeight(-16)
	fallbackExtent := canvas.TextExtentWithStr("Fallback")
	canvas.GetUpdatedHandle(types.NewSet(int32(types.CsHandleValid), int32(types.CsFontValid)))
	fallbackFace := cd0TextFace(uintptr(canvas.Handle()))

	samplePoints := map[string]image.Point{
		"background":         {X: 4, Y: 4},
		"rect-center":        {X: 48, Y: 40},
		"rect-right-edge":    {X: 96, Y: 40},
		"roundrect-center":   {X: 160, Y: 40},
		"roundrect-corner":   {X: 122, Y: 22},
		"roundrect-outside":  {X: 112, Y: 16},
		"ellipse-center":     {X: 272, Y: 40},
		"ellipse-edge":       {X: 224, Y: 40},
		"ellipse-outside":    {X: 223, Y: 40},
		"clip-inside":        {X: 276, Y: 110},
		"clip-outside-left":  {X: 236, Y: 110},
		"clip-outside-right": {X: 316, Y: 110},
		"stroke-edge":        {X: 17, Y: 170},
		"stroke-interior":    {X: 50, Y: 170},
		"line-center":        {X: 50, Y: 218},
	}
	samples := make(map[string]string, len(samplePoints))
	for name, point := range samplePoints {
		samples[name] = cd0ColorHex(canvas.Pixels(int32(point.X), int32(point.Y)))
	}

	textInk := 0
	for y := int32(82); y < 128; y++ {
		for x := int32(16); x < 208; x++ {
			if cd0ColorHex(canvas.Pixels(x, y)) != "#FFFFFF" {
				textInk++
			}
		}
	}

	pngPath := cd0ArtifactPath(t, "cd0-canvas-probe.png")
	if err := cd0WriteCanvasPNG(canvas, width, height, pngPath); err != nil {
		t.Fatalf("CD0 canvas probe: write PNG: %v", err)
	}

	evidence := cd0CanvasEvidence{
		Width:                width,
		Height:               height,
		Samples:              samples,
		TextInkPixels:        textInk,
		HalfOpenFillRect:     samples["rect-center"] == "#1F6FD1" && samples["rect-right-edge"] == "#FFFFFF",
		ClipInsidePainted:    samples["clip-inside"] == "#B43BB8",
		ClipOutsidePreserved: samples["clip-outside-left"] == "#FFFFFF" && samples["clip-outside-right"] == "#FFFFFF",
		StrokeRectObserved:   samples["stroke-edge"] == "#7B3F00" && samples["stroke-interior"] == "#FFFFFF",
		LineObserved:         samples["line-center"] == "#7B3F00",
		FallbackRequested:    missingFamily,
		FallbackResolved:     fallbackFace,
		FallbackTextExtent:   [2]int32{fallbackExtent.Cx, fallbackExtent.Cy},
		DPI:                  dpiEvidence,
		PNG:                  filepath.Base(pngPath),
	}
	supported := evidence.HalfOpenFillRect && evidence.ClipInsidePainted &&
		evidence.ClipOutsidePreserved && evidence.TextInkPixels > 0 &&
		evidence.StrokeRectObserved && evidence.LineObserved &&
		evidence.FallbackResolved != "" && evidence.FallbackTextExtent[0] > 0 && evidence.FallbackTextExtent[1] > 0 &&
		evidence.FallbackResolved != evidence.FallbackRequested &&
		samples["roundrect-center"] == "#2E9D50" && samples["roundrect-corner"] == "#2E9D50" && samples["roundrect-outside"] == "#FFFFFF" &&
		samples["ellipse-center"] == "#D14343" && samples["ellipse-edge"] == "#D14343" && samples["ellipse-outside"] == "#FFFFFF"
	status := "supported"
	var notes []string
	if !supported {
		status = "unsupported"
		notes = append(notes, "one or more real LCL Canvas pixel invariants were not observed; inspect the emitted PNG and samples")
	}
	for _, item := range dpiEvidence {
		if item.TextWidthPX <= 0 || item.TextHeightPX <= 0 {
			status = "unsupported"
			notes = append(notes, fmt.Sprintf("TextExtent returned an empty metric at %d DPI", item.DPI))
		}
	}
	minWidth, maxWidth := dpiEvidence[0].TextWidthDIP, dpiEvidence[0].TextWidthDIP
	minHeight, maxHeight := dpiEvidence[0].TextHeightDIP, dpiEvidence[0].TextHeightDIP
	for _, item := range dpiEvidence[1:] {
		if item.TextWidthDIP < minWidth {
			minWidth = item.TextWidthDIP
		}
		if item.TextWidthDIP > maxWidth {
			maxWidth = item.TextWidthDIP
		}
		if item.TextHeightDIP < minHeight {
			minHeight = item.TextHeightDIP
		}
		if item.TextHeightDIP > maxHeight {
			maxHeight = item.TextHeightDIP
		}
	}
	if maxWidth-minWidth > 2 {
		status = "unsupported"
		notes = append(notes, fmt.Sprintf("normalized TextExtent width drifted by %d DIP", maxWidth-minWidth))
	}
	if maxHeight-minHeight > 2 {
		status = "unsupported"
		notes = append(notes, fmt.Sprintf("normalized TextExtent height drifted by %d DIP", maxHeight-minHeight))
	}
	cd0WriteJSON(t, "cd0-canvas-probe.json", cd0ProbeEnvelope{
		Probe:     "CD0.2-lcl-canvas",
		Status:    status,
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		GoVersion: runtime.Version(),
		Evidence:  evidence,
		Notes:     notes,
	})
	if status != "supported" {
		t.Errorf("CD0.2 required Canvas invariants failed: status=%s samples=%v notes=%v", status, samples, notes)
	}
}

func cd0ArtifactPath(t *testing.T, name string) string {
	t.Helper()
	dir := os.Getenv(cd0ArtifactDirEnv)
	if dir == "" {
		dir = t.TempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create CD0 artifact directory: %v", err)
	}
	return filepath.Join(dir, name)
}

func cd0WriteJSON(t *testing.T, name string, value any) {
	t.Helper()
	path := cd0ArtifactPath(t, name)
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	t.Logf("CD0 artifact: %s", path)
}

func cd0WriteCanvasPNG(canvas lcl.ICanvas, width, height int, path string) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, cd0RGBA(canvas.Pixels(int32(x), int32(y))))
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

func cd0TColor(red, green, blue byte) types.TColor {
	return types.TColor(uint32(blue)<<16 | uint32(green)<<8 | uint32(red))
}

func cd0RGBA(value types.TColor) color.RGBA {
	raw := uint32(value)
	return color.RGBA{R: byte(raw), G: byte(raw >> 8), B: byte(raw >> 16), A: 0xff}
}

func cd0ColorHex(value types.TColor) string {
	c := cd0RGBA(value)
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

func cd0TextFace(hdc uintptr) string {
	buffer := make([]uint16, 128)
	length, _, _ := cd0ProcGetTextFace.Call(hdc, uintptr(len(buffer)), uintptr(unsafe.Pointer(&buffer[0])))
	if length == 0 {
		return ""
	}
	return syscall.UTF16ToString(buffer[:length])
}
