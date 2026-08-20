//go:build windows && !race

package native

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"testing"
	"time"

	"github.com/energye/lcl/lcl"
	"github.com/energye/lcl/types"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

type cd0LCLDrawRecord struct {
	Callback     string         `json:"callback"`
	Index        int32          `json:"index,omitempty"`
	Column       int32          `json:"column,omitempty"`
	Row          int32          `json:"row,omitempty"`
	Height       int32          `json:"height,omitempty"`
	Rect         cd0RuntimeRect `json:"rect"`
	State        uint32         `json:"state"`
	CanvasHandle uintptr        `json:"canvasHandle"`
}

type cd0ControlDrawCapability struct {
	Control       string   `json:"control"`
	Protocol      string   `json:"protocol"`
	Status        string   `json:"status"`
	CallbackCount int      `json:"callbackCount"`
	Stages        []string `json:"stages,omitempty"`
	ControlHWND   uintptr  `json:"controlHwnd,omitempty"`
	ParentHWND    uintptr  `json:"parentHwnd,omitempty"`
	ParentClass   string   `json:"parentClass,omitempty"`
	Notes         []string `json:"notes,omitempty"`
}

type cd0ControlDrawEvidence struct {
	Capabilities []cd0ControlDrawCapability `json:"capabilities"`
	ComboDraws   []cd0LCLDrawRecord         `json:"comboDraws"`
	GridDraws    []cd0LCLDrawRecord         `json:"gridDraws"`
	CustomDraws  []cd0CustomDrawRecord      `json:"customDraws"`
}

func TestCD0ControlDrawRuntimeProbe(t *testing.T) {
	dll := radioProbeDLL(t)
	artifactDir := os.Getenv(cd0ArtifactDirEnv)
	if artifactDir == "" {
		artifactDir = t.TempDir()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCD0ControlDrawHelper$", "-test.v", "-test.timeout=45s")
	cmd.Env = append(os.Environ(),
		"FVCL_CD0_CONTROL_HELPER=1",
		"FVCL_LIBENERGY_DLL="+dll,
		cd0ArtifactDirEnv+"="+artifactDir,
	)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("CD0.5 helper timed out: %v\n%s", ctx.Err(), output)
	}
	if err != nil {
		t.Fatalf("CD0.5 helper failed: %v\n%s", err, output)
	}
	t.Logf("CD0.5 helper output:\n%s", output)

	path := filepath.Join(artifactDir, "cd0-control-draw-probe.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read CD0.5 helper artifact %s: %v", path, err)
	}
	var envelope cd0ProbeEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode CD0.5 helper artifact: %v", err)
	}
	if envelope.Probe != "CD0.5-control-draw-capabilities" {
		t.Fatalf("unexpected CD0.5 probe name %q", envelope.Probe)
	}
	if envelope.Status != "supported" && envelope.Status != "deferred" {
		t.Fatalf("unexpected CD0.5 probe status %q", envelope.Status)
	}
}

// TestCD0ControlDrawHelper isolates LCL and subclass callbacks from normal Go
// test shutdown. See TestCD0OwnerDrawHelper for the Go 1.25 callback-exit
// limitation recorded by this CD0 Spike.
func TestCD0ControlDrawHelper(t *testing.T) {
	if os.Getenv("FVCL_CD0_CONTROL_HELPER") != "1" {
		t.Skip("CD0 control-draw helper is launched by the parent probe")
	}
	defer func() {
		if t.Failed() {
			syscall.ExitProcess(1)
		}
		syscall.ExitProcess(0)
	}()
	runCD0ControlDrawRuntimeProbe(t, radioProbeDLL(t))
}

func runCD0ControlDrawRuntimeProbe(t *testing.T, dll string) {
	runtime.LockOSThread()
	// This probe installs both LCL and Win32 callbacks. Keeping the goroutine
	// locked until it exits prevents reuse of a callback-contaminated OS thread.

	if err := Init(dll); err != nil {
		t.Fatal(err)
	}
	r := NewRenderer()
	window := r.Create("Window")

	comboHandle := r.Create("ComboBox")
	combo, ok := r.controls[comboHandle].(lcl.IComboBox)
	if !ok {
		t.Fatalf("CD0.5 ComboBox control has type %T, want lcl.IComboBox", r.controls[comboHandle])
	}
	combo.SetStyle(types.CsOwnerDrawFixed)
	combo.SetItemHeight(24)
	combo.Items().Add("Alpha")
	combo.Items().Add("Beta")
	combo.SetItemIndex(0)
	var comboDraws []cd0LCLDrawRecord
	combo.SetOnMeasureItem(func(_ lcl.IWinControl, index int32, height *int32) {
		if height != nil && *height < 24 {
			*height = 24
		}
		measured := int32(0)
		if height != nil {
			measured = *height
		}
		comboDraws = append(comboDraws, cd0LCLDrawRecord{
			Callback: "OnMeasureItem",
			Index:    index,
			Height:   measured,
		})
	})
	combo.SetOnDrawItem(func(_ lcl.IWinControl, index int32, rect types.TRect, state types.TOwnerDrawState) {
		canvas := combo.Canvas()
		canvas.BrushToBrush().SetStyle(types.BsSolid)
		canvas.BrushToBrush().SetColor(cd0TColor(0xe8, 0xf1, 0xfb))
		canvas.FillRectWithRect(rect)
		canvas.FontToFont().SetColor(cd0TColor(0x18, 0x28, 0x38))
		if index >= 0 && index < combo.Items().Count() {
			canvas.TextOutWithIntX2Str(rect.Left+5, rect.Top+3, combo.Items().Strings(index))
		}
		comboDraws = append(comboDraws, cd0LCLDrawRecord{
			Callback:     "OnDrawItem",
			Index:        index,
			Rect:         cd0ProbeRect(rect),
			State:        uint32(state),
			CanvasHandle: uintptr(canvas.Handle()),
		})
	})
	r.SetParent(comboHandle, window)
	r.SetBounds(comboHandle, render.Rect{X: 20, Y: 24, W: 220, H: 36})

	gridHandle := r.Create("StringGrid")
	grid, ok := r.controls[gridHandle].(lcl.IStringGrid)
	if !ok {
		t.Fatalf("CD0.5 StringGrid control has type %T, want lcl.IStringGrid", r.controls[gridHandle])
	}
	grid.SetColCount(2)
	grid.SetRowCount(2)
	grid.SetDefaultColWidth(100)
	grid.SetDefaultRowHeight(30)
	grid.SetCells(0, 0, "A1")
	grid.SetCells(1, 0, "B1")
	grid.SetCells(0, 1, "A2")
	grid.SetCells(1, 1, "B2")
	var gridDraws []cd0LCLDrawRecord
	grid.SetOnPrepareCanvas(func(_ lcl.IObject, col, row int32, state types.TGridDrawState) {
		canvas := grid.Canvas()
		canvas.BrushToBrush().SetStyle(types.BsSolid)
		canvas.BrushToBrush().SetColor(cd0TColor(0xf7, 0xf7, 0xf7))
		gridDraws = append(gridDraws, cd0LCLDrawRecord{
			Callback:     "OnPrepareCanvas",
			Column:       col,
			Row:          row,
			State:        uint32(state),
			CanvasHandle: uintptr(canvas.Handle()),
		})
	})
	grid.SetOnDrawCell(func(_ lcl.IObject, col, row int32, rect types.TRect, state types.TGridDrawState) {
		canvas := grid.Canvas()
		canvas.BrushToBrush().SetStyle(types.BsSolid)
		canvas.BrushToBrush().SetColor(cd0TColor(0xf7, 0xf7, 0xf7))
		canvas.FillRectWithRect(rect)
		canvas.FontToFont().SetColor(cd0TColor(0x20, 0x20, 0x20))
		canvas.TextOutWithIntX2Str(rect.Left+4, rect.Top+4, grid.Cells(col, row))
		gridDraws = append(gridDraws, cd0LCLDrawRecord{
			Callback:     "OnDrawCell",
			Column:       col,
			Row:          row,
			Rect:         cd0ProbeRect(rect),
			State:        uint32(state),
			CanvasHandle: uintptr(canvas.Handle()),
		})
	})
	r.SetParent(gridHandle, window)
	r.SetBounds(gridHandle, render.Rect{X: 20, Y: 82, W: 240, H: 110})

	trackHandle := r.Create("Slider")
	r.SetParent(trackHandle, window)
	r.SetBounds(trackHandle, render.Rect{X: 300, Y: 30, W: 280, H: 44})
	r.SetMinimum(trackHandle, 0)
	r.SetMaximum(trackHandle, 100)
	r.SetValue(trackHandle, 40)

	progressHandle := r.Create("ProgressBar")
	r.SetParent(progressHandle, window)
	r.SetBounds(progressHandle, render.Rect{X: 300, Y: 100, W: 280, H: 30})
	r.SetMinimum(progressHandle, 0)
	r.SetMaximum(progressHandle, 100)
	r.SetValue(progressHandle, 65)

	formHWND := cd0ControlHWND(t, r.formRef)
	trackHWND := cd0ControlHWND(t, r.controls[trackHandle])
	progressHWND := cd0ControlHWND(t, r.controls[progressHandle])
	router, err := cd0NewRuntimeRouter()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Unhook the Win32 subclass and close the LCL form before this callback
		// probe returns. Leaving either alive causes the Go callback trampoline to
		// be reached while the test binary is exiting.
		router.close()
		r.formRef.SetOnWndProc(nil)
		r.formRef.Close()
		lcl.Application.ProcessMessages()
		r.DrainDestroy()
	})
	defer router.close()
	if err := router.add(formHWND, trackHWND, "TrackBar"); err != nil {
		t.Fatalf("install TrackBar custom-draw route: %v", err)
	}
	if err := router.add(formHWND, progressHWND, "Progress"); err != nil {
		t.Fatalf("install Progress custom-draw route: %v", err)
	}

	r.formRef.Show()
	lcl.Application.ProcessMessages()
	combo.SetDroppedDown(true)
	lcl.Application.ProcessMessages()
	cd0RepaintWindow(cd0ControlHWND(t, combo))
	combo.SetDroppedDown(false)
	cd0RepaintWindow(cd0ControlHWND(t, grid))
	cd0RepaintWindow(trackHWND)
	cd0RepaintWindow(progressHWND)

	customDraws := router.customDrawRecords()
	capabilities := []cd0ControlDrawCapability{
		cd0LCLCallbackCapability("ComboBox.DrawItem", "CsOwnerDrawFixed + OnDrawItem", comboDraws, "OnDrawItem"),
		cd0CallbackCapability("ComboBox.MeasureItem", "CsOwnerDrawFixed + OnMeasureItem", comboDraws, "OnMeasureItem", false),
		cd0LCLCallbackCapability("StringGrid.Prepare", "OnPrepareCanvas", gridDraws, "OnPrepareCanvas"),
		cd0LCLCallbackCapability("StringGrid.Draw", "OnDrawCell", gridDraws, "OnDrawCell"),
		cd0NativeCustomDrawCapability("TrackBar", trackHWND, formHWND, customDraws),
		cd0NativeCustomDrawCapability("Progress", progressHWND, formHWND, customDraws),
	}
	overallStatus := "supported"
	var notes []string
	for _, capability := range capabilities {
		if capability.Status != "supported" {
			overallStatus = "deferred"
			notes = append(notes, capability.Control+" did not emit its real draw callback on this run")
		}
	}

	cd0WriteJSON(t, "cd0-control-draw-probe.json", cd0ProbeEnvelope{
		Probe:     "CD0.5-control-draw-capabilities",
		Status:    overallStatus,
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		GoVersion: runtime.Version(),
		Evidence: cd0ControlDrawEvidence{
			Capabilities: capabilities,
			ComboDraws:   comboDraws,
			GridDraws:    gridDraws,
			CustomDraws:  customDraws,
		},
		Notes: notes,
	})
	t.Logf("CD0.5 result=%s capabilities=%+v", overallStatus, capabilities)
}

func cd0ProbeRect(rect types.TRect) cd0RuntimeRect {
	return cd0RuntimeRect{Left: rect.Left, Top: rect.Top, Right: rect.Right, Bottom: rect.Bottom}
}

func cd0LCLCallbackCapability(control, protocol string, records []cd0LCLDrawRecord, callback string) cd0ControlDrawCapability {
	return cd0CallbackCapability(control, protocol, records, callback, true)
}

func cd0CallbackCapability(control, protocol string, records []cd0LCLDrawRecord, callback string, requireCanvas bool) cd0ControlDrawCapability {
	capability := cd0ControlDrawCapability{Control: control, Protocol: protocol, Status: "deferred"}
	validCanvas := false
	for _, record := range records {
		if record.Callback != callback {
			continue
		}
		capability.CallbackCount++
		validCanvas = validCanvas || record.CanvasHandle != 0
	}
	if capability.CallbackCount > 0 && (!requireCanvas || validCanvas) {
		capability.Status = "supported"
		return capability
	}
	if requireCanvas {
		capability.Notes = append(capability.Notes, "no real callback with a live LCL Canvas was observed")
	} else {
		capability.Notes = append(capability.Notes, "no real LCL callback was observed")
	}
	return capability
}

func cd0NativeCustomDrawCapability(control string, hwnd, expectedParent uintptr, records []cd0CustomDrawRecord) cd0ControlDrawCapability {
	capability := cd0ControlDrawCapability{
		Control:     control,
		Protocol:    "WM_NOTIFY/NM_CUSTOMDRAW",
		Status:      "deferred",
		ControlHWND: hwnd,
		ParentHWND:  cd0ParentHWND(hwnd),
	}
	capability.ParentClass = cd0WindowClass(capability.ParentHWND)
	stageSet := make(map[string]struct{})
	validRecord := false
	for _, record := range records {
		if record.Control != control {
			continue
		}
		capability.CallbackCount++
		stageSet[record.Stage] = struct{}{}
		validRecord = validRecord || (record.ParentHWND == expectedParent && record.HWNDFrom == hwnd && record.HDC != 0)
	}
	for stage := range stageSet {
		capability.Stages = append(capability.Stages, stage)
	}
	sort.Strings(capability.Stages)
	if capability.CallbackCount > 0 && validRecord && capability.ParentHWND == expectedParent {
		capability.Status = "supported"
		return capability
	}
	capability.Notes = append(capability.Notes, "no real direct-parent NM_CUSTOMDRAW record with a live HDC was observed")
	return capability
}
