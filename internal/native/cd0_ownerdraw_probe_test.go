//go:build windows && !race

package native

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/energye/lcl/lcl"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

const (
	cd0WMDrawItem          = 0x002b
	cd0WMNotify            = 0x004e
	cd0WMNCDestroy         = 0x0082
	cd0WMMouseMove         = 0x0200
	cd0WMProbeMarker       = 0x8000 + 0x4d
	cd0BMSetState          = 0x00f3
	cd0GWLPStyle           = int32(-16)
	cd0BSOwnerDraw         = 0x0000000b
	cd0BSTypeMask          = 0x0000000f
	cd0SWPNoSize           = 0x0001
	cd0SWPNoMove           = 0x0002
	cd0SWPNoZOrder         = 0x0004
	cd0SWPNoActivate       = 0x0010
	cd0SWPFrameChanged     = 0x0020
	cd0ODSSelected         = 0x0001
	cd0ODSGrayed           = 0x0002
	cd0ODSDisabled         = 0x0004
	cd0ODSChecked          = 0x0008
	cd0ODSFocus            = 0x0010
	cd0ODSDefault          = 0x0020
	cd0ODSHotLight         = 0x0040
	cd0ODSInactive         = 0x0080
	cd0ODSNoAccel          = 0x0100
	cd0ODSNoFocusRect      = 0x0200
	cd0NMCustomDrawCode    = uint32(0xfffffff4) // NM_FIRST - 12
	cd0CDDSPrePaint        = 0x00000001
	cd0CDDSPostPaint       = 0x00000002
	cd0CDDSItem            = 0x00010000
	cd0CDDSItemPrePaint    = cd0CDDSItem | cd0CDDSPrePaint
	cd0CDRFDoDefault       = 0x00000000
	cd0CDRFNotifyPostPaint = 0x00000010
	cd0CDRFNotifyItemDraw  = 0x00000020
)

var (
	cd0User32                   = syscall.NewLazyDLL("user32.dll")
	cd0Comctl32                 = syscall.NewLazyDLL("comctl32.dll")
	cd0Kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	cd0ProcGetParent            = cd0User32.NewProc("GetParent")
	cd0ProcGetClassName         = cd0User32.NewProc("GetClassNameW")
	cd0ProcSendMessage          = cd0User32.NewProc("SendMessageW")
	cd0ProcInvalidateRect       = cd0User32.NewProc("InvalidateRect")
	cd0ProcUpdateWindow         = cd0User32.NewProc("UpdateWindow")
	cd0ProcSetFocus             = cd0User32.NewProc("SetFocus")
	cd0ProcEnableWindow         = cd0User32.NewProc("EnableWindow")
	cd0ProcSetForegroundWindow  = cd0User32.NewProc("SetForegroundWindow")
	cd0ProcSetWindowPos         = cd0User32.NewProc("SetWindowPos")
	cd0ProcGetWindowLongPtr     = cd0WindowLongProc("GetWindowLongPtrW", "GetWindowLongW")
	cd0ProcSetWindowLongPtr     = cd0WindowLongProc("SetWindowLongPtrW", "SetWindowLongW")
	cd0ProcSetWindowSubclass    = cd0Comctl32.NewProc("SetWindowSubclass")
	cd0ProcRemoveWindowSubclass = cd0Comctl32.NewProc("RemoveWindowSubclass")
	cd0ProcDefSubclassProc      = cd0Comctl32.NewProc("DefSubclassProc")
	cd0ProcRtlMoveMemory        = cd0Kernel32.NewProc("RtlMoveMemory")
	cd0RuntimeSubclassCallback  = syscall.NewCallback(cd0RuntimeSubclassProc)
	cd0RuntimeRouterSequence    atomic.Uint64
	cd0RuntimeRouters           sync.Map
)

type cd0RuntimeRect struct {
	Left, Top, Right, Bottom int32
}

// These runtime copies are intentionally probe-local. The production ABI
// definitions and compile-time 386/amd64 assertions live in the separate ABI
// probe so a successful callback cannot substitute for a layout assertion.
type cd0RuntimeDrawItem struct {
	CtlType    uint32
	CtlID      uint32
	ItemID     uint32
	ItemAction uint32
	ItemState  uint32
	HWNDItem   uintptr
	HDC        uintptr
	Rect       cd0RuntimeRect
	ItemData   uintptr
}

type cd0RuntimeNMHDR struct {
	HWNDFrom uintptr
	IDFrom   uintptr
	Code     uint32
}

type cd0RuntimeNMCustomDraw struct {
	Header     cd0RuntimeNMHDR
	DrawStage  uint32
	HDC        uintptr
	Rect       cd0RuntimeRect
	ItemSpec   uintptr
	ItemState  uint32
	ItemLParam uintptr
}

type cd0DrawItemRecord struct {
	Phase      string         `json:"phase"`
	Control    string         `json:"control"`
	ParentHWND uintptr        `json:"parentHwnd"`
	ItemHWND   uintptr        `json:"itemHwnd"`
	CtlType    uint32         `json:"ctlType"`
	CtlID      uint32         `json:"ctlId"`
	ItemAction uint32         `json:"itemAction"`
	ItemState  uint32         `json:"itemState"`
	States     []string       `json:"states"`
	HDC        uintptr        `json:"hdc"`
	Rect       cd0RuntimeRect `json:"rect"`
}

type cd0CustomDrawRecord struct {
	Control    string         `json:"control"`
	ParentHWND uintptr        `json:"parentHwnd"`
	HWNDFrom   uintptr        `json:"hwndFrom"`
	DrawStage  uint32         `json:"drawStage"`
	Stage      string         `json:"stage"`
	ItemSpec   uintptr        `json:"itemSpec"`
	ItemState  uint32         `json:"itemState"`
	HDC        uintptr        `json:"hdc"`
	Rect       cd0RuntimeRect `json:"rect"`
}

type cd0RuntimeParentRoute struct {
	HWND     uintptr
	Children map[uintptr]string
}

type cd0RuntimeRouter struct {
	id uintptr

	mu               sync.Mutex
	phase            string
	parents          map[uintptr]*cd0RuntimeParentRoute
	childParents     map[uintptr]uintptr
	destroyedParents map[uintptr]bool
	drawItems        []cd0DrawItemRecord
	customDraws      []cd0CustomDrawRecord
	callbackCounts   map[uintptr]int
}

type cd0OwnerDrawRouteEvidence struct {
	LogicalParent   string  `json:"logicalParent"`
	ControlHWND     uintptr `json:"controlHwnd"`
	ActualParent    uintptr `json:"actualParentHwnd"`
	ExpectedParent  uintptr `json:"expectedParentHwnd"`
	ParentClass     string  `json:"parentClass"`
	DrawItems       int     `json:"drawItems"`
	Routed          bool    `json:"routed"`
	Unbound         bool    `json:"unbound"`
	CallbackRemoved bool    `json:"callbackRemoved"`
}

type cd0OwnerDrawEvidence struct {
	Routes                  []cd0OwnerDrawRouteEvidence `json:"routes"`
	StateCoverage           map[string][]string         `json:"stateCoverage"`
	StateObserved           map[string]bool             `json:"stateObserved"`
	ParentNCDESTROYObserved bool                        `json:"parentNcDestroyObserved"`
	DestroyRouteCleanup     bool                        `json:"destroyRouteCleanup"`
	DrawItems               []cd0DrawItemRecord         `json:"drawItems"`
}

func TestCD0OwnerDrawAndSubclassRuntimeProbe(t *testing.T) {
	dll := radioProbeDLL(t)
	artifactDir := os.Getenv(cd0ArtifactDirEnv)
	if artifactDir == "" {
		artifactDir = t.TempDir()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCD0OwnerDrawHelper$", "-test.v", "-test.timeout=45s")
	cmd.Env = append(os.Environ(),
		"FVCL_CD0_OWNER_HELPER=1",
		"FVCL_LIBENERGY_DLL="+dll,
		cd0ArtifactDirEnv+"="+artifactDir,
	)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("CD0.3/CD0.4 helper timed out: %v\n%s", ctx.Err(), output)
	}
	if err != nil {
		t.Fatalf("CD0.3/CD0.4 helper failed: %v\n%s", err, output)
	}
	t.Logf("CD0.3/CD0.4 helper output:\n%s", output)

	path := filepath.Join(artifactDir, "cd0-ownerdraw-probe.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read CD0.3/CD0.4 helper artifact %s: %v", path, err)
	}
	var envelope cd0ProbeEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode CD0.3/CD0.4 helper artifact: %v", err)
	}
	if envelope.Probe != "CD0.3-CD0.4-owner-draw-subclass" {
		t.Fatalf("unexpected CD0.3/CD0.4 probe name %q", envelope.Probe)
	}
	if envelope.Status != "supported" && envelope.Status != "deferred" {
		t.Fatalf("unexpected CD0.3/CD0.4 probe status %q", envelope.Status)
	}
}

// TestCD0OwnerDrawHelper runs the callback-heavy portion in a short-lived
// process. Go 1.25 can report exitsyscall after syscall.NewCallback returns
// through LCL during normal test-binary shutdown; ExitProcess avoids claiming
// that this runtime limitation is a clean production teardown proof.
func TestCD0OwnerDrawHelper(t *testing.T) {
	if os.Getenv("FVCL_CD0_OWNER_HELPER") != "1" {
		t.Skip("CD0 owner-draw helper is launched by the parent probe")
	}
	defer func() {
		if t.Failed() {
			syscall.ExitProcess(1)
		}
		syscall.ExitProcess(0)
	}()
	runCD0OwnerDrawAndSubclassRuntimeProbe(t, radioProbeDLL(t))
}

func runCD0OwnerDrawAndSubclassRuntimeProbe(t *testing.T, dll string) {
	runtime.LockOSThread()
	// The enclosing helper process exits with ExitProcess after its real callback
	// run; keep all LCL work on this one OS thread until then.

	if err := Init(dll); err != nil {
		t.Fatal(err)
	}
	t.Log("CD0.3/CD0.4 checkpoint: initialized")
	r := NewRenderer()
	window := r.Create("Window")

	formButton := r.Create("Button")
	r.SetParent(formButton, window)
	r.SetBounds(formButton, render.Rect{X: 20, Y: 30, W: 180, H: 44})
	r.SetText(formButton, "Window parent")

	scroll := r.Create("ScrollBox")
	r.SetParent(scroll, window)
	r.SetBounds(scroll, render.Rect{X: 20, Y: 100, W: 280, H: 150})
	scrollButton := r.Create("Button")
	r.SetParent(scrollButton, scroll)
	r.SetBounds(scrollButton, render.Rect{X: 18, Y: 20, W: 180, H: 44})
	r.SetText(scrollButton, "ScrollBox parent")

	pageControl := r.Create("PageControl")
	r.SetParent(pageControl, window)
	r.SetBounds(pageControl, render.Rect{X: 330, Y: 100, W: 330, H: 220})
	tabPage := r.Create("TabPage")
	r.SetText(tabPage, "Probe")
	r.SetParent(tabPage, pageControl)
	r.SetPageSelectedIndex(pageControl, 0)
	r.SyncPages(pageControl, []render.Handle{tabPage})
	tabButton := r.Create("Button")
	r.SetParent(tabButton, tabPage)
	r.SetBounds(tabButton, render.Rect{X: 18, Y: 24, W: 180, H: 44})
	r.SetText(tabButton, "TabPage parent")

	r.formRef.Show()
	lcl.Application.ProcessMessages()
	cd0ProcSetForegroundWindow.Call(uintptr(r.formRef.Handle()))
	t.Log("CD0.3/CD0.4 checkpoint: form shown")

	router, err := cd0NewRuntimeRouter()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Remove every Go callback and close all native windows before the test
		// goroutine returns. The callback trampoline must never outlive its LCL
		// owner, even though the OS thread itself is retired below.
		router.close()
		r.formRef.SetOnWndProc(nil)
		r.formRef.Close()
		lcl.Application.ProcessMessages()
		r.DrainDestroy()
	})
	defer router.close()

	type routeSetup struct {
		name         string
		handle       render.Handle
		parentHandle render.Handle
		hwnd         uintptr
		parentHWND   uintptr
		expectedHWND uintptr
	}
	setups := []routeSetup{
		{name: "Window", handle: formButton, parentHandle: window},
		{name: "ScrollBox", handle: scrollButton, parentHandle: scroll},
		{name: "TabPage", handle: tabButton, parentHandle: tabPage},
	}
	for index := range setups {
		setup := &setups[index]
		setup.hwnd = cd0ControlHWND(t, r.controls[setup.handle])
		setup.parentHWND = cd0ParentHWND(setup.hwnd)
		setup.expectedHWND = cd0ControlHWND(t, r.controls[setup.parentHandle])
		if err := router.add(setup.parentHWND, setup.hwnd, setup.name); err != nil {
			t.Fatalf("install %s parent subclass: %v", setup.name, err)
		}
		if err := cd0SetOwnerDrawStyle(setup.hwnd); err != nil {
			t.Fatalf("set %s button BS_OWNERDRAW: %v", setup.name, err)
		}
	}
	t.Log("CD0.3/CD0.4 checkpoint: subclasses installed")

	router.setPhase("normal")
	for _, setup := range setups {
		cd0RepaintWindow(setup.hwnd)
	}

	// Drive real button messages. A missing ODS flag is recorded as deferred;
	// no synthetic DRAWITEMSTRUCT is sent through the router.
	formHWND := setups[0].hwnd
	router.setPhase("hovered")
	cd0ProcSendMessage.Call(formHWND, cd0WMMouseMove, 0, uintptr(uint32(22)|(uint32(18)<<16)))
	cd0RepaintWindow(formHWND)

	router.setPhase("focused")
	cd0ProcSetFocus.Call(formHWND)
	cd0RepaintWindow(formHWND)

	router.setPhase("pressed")
	cd0ProcSendMessage.Call(formHWND, cd0BMSetState, 1, 0)
	cd0RepaintWindow(formHWND)
	cd0ProcSendMessage.Call(formHWND, cd0BMSetState, 0, 0)

	router.setPhase("disabled")
	cd0ProcEnableWindow.Call(formHWND, 0)
	cd0RepaintWindow(formHWND)
	cd0ProcEnableWindow.Call(formHWND, 1)

	router.setPhase("default")
	if button, ok := r.controls[formButton].(lcl.ICustomButton); ok {
		button.SetDefault(true)
		cd0RepaintWindow(formHWND)
		button.SetDefault(false)
	}
	t.Log("CD0.3/CD0.4 checkpoint: states driven")

	drawItems := router.drawItemRecords()
	routes := make([]cd0OwnerDrawRouteEvidence, 0, len(setups))
	for _, setup := range setups {
		count := 0
		for _, record := range drawItems {
			if record.Control == setup.name {
				count++
			}
		}
		routes = append(routes, cd0OwnerDrawRouteEvidence{
			LogicalParent:  setup.name,
			ControlHWND:    setup.hwnd,
			ActualParent:   setup.parentHWND,
			ExpectedParent: setup.expectedHWND,
			ParentClass:    cd0WindowClass(setup.parentHWND),
			DrawItems:      count,
			Routed:         count > 0 && setup.parentHWND == setup.expectedHWND,
		})
	}

	stateCoverage := make(map[string][]string)
	for _, phase := range []string{"normal", "hovered", "focused", "pressed", "disabled", "default"} {
		set := make(map[string]struct{})
		for _, record := range drawItems {
			if record.Control != "Window" || record.Phase != phase {
				continue
			}
			for _, state := range record.States {
				set[state] = struct{}{}
			}
		}
		for state := range set {
			stateCoverage[phase] = append(stateCoverage[phase], state)
		}
		sort.Strings(stateCoverage[phase])
	}
	stateObserved := map[string]bool{
		"normal":   cd0PhaseHasDraw(drawItems, "normal", "Window"),
		"hovered":  cd0PhaseHasState(drawItems, "hovered", "Window", cd0ODSHotLight),
		"focused":  cd0PhaseHasState(drawItems, "focused", "Window", cd0ODSFocus),
		"pressed":  cd0PhaseHasState(drawItems, "pressed", "Window", cd0ODSSelected),
		"disabled": cd0PhaseHasState(drawItems, "disabled", "Window", cd0ODSDisabled),
		"default":  cd0PhaseHasState(drawItems, "default", "Window", cd0ODSDefault),
	}

	for index := range setups {
		setup := &setups[index]
		removed := router.removeChild(setup.hwnd)
		before := router.callbackCount(setup.parentHWND)
		cd0ProcSendMessage.Call(setup.parentHWND, cd0WMProbeMarker, 0, 0)
		after := router.callbackCount(setup.parentHWND)
		routes[index].Unbound = removed
		routes[index].CallbackRemoved = before == after
		r.Destroy(setup.handle)
	}
	r.DrainDestroy()
	t.Log("CD0.3/CD0.4 checkpoint: manual routes removed")

	// SetWindowSubclass owns its automatic removal on WM_NCDESTROY. Exercise
	// that path with an isolated parent/child pair so manual child teardown
	// above cannot make lifecycle cleanup look successful by construction.
	lifecycleParent := lcl.NewPanel(r.formRef)
	lifecycleParent.SetParent(r.formRef)
	lifecycleParent.SetBounds(20, 340, 240, 80)
	lifecycleChild := lcl.NewButton(lifecycleParent)
	lifecycleChild.SetParent(lifecycleParent)
	lifecycleChild.SetBounds(12, 14, 160, 36)
	lifecycleParentHWND := cd0ControlHWND(t, lifecycleParent)
	lifecycleChildHWND := cd0ControlHWND(t, lifecycleChild)
	if err := router.add(lifecycleParentHWND, lifecycleChildHWND, "Lifecycle"); err != nil {
		t.Fatalf("install lifecycle parent subclass: %v", err)
	}
	t.Log("CD0.3/CD0.4 checkpoint: lifecycle subclass installed")
	lifecycleParent.Free()
	t.Log("CD0.3/CD0.4 checkpoint: lifecycle parent freed")
	lcl.Application.ProcessMessages()
	parentNCDESTROYObserved := router.parentWasDestroyed(lifecycleParentHWND)
	destroyRouteCleanup := !router.hasParent(lifecycleParentHWND) && !router.hasChild(lifecycleChildHWND)

	status := "supported"
	var notes []string
	for _, route := range routes {
		if !route.Routed || !route.Unbound || !route.CallbackRemoved {
			status = "unsupported"
			notes = append(notes, fmt.Sprintf("%s direct-parent routing or teardown was not observed", route.LogicalParent))
		}
	}
	if !parentNCDESTROYObserved || !destroyRouteCleanup {
		status = "unsupported"
		notes = append(notes, "WM_NCDESTROY did not remove the lifecycle parent and child routes")
	}
	for _, phase := range []string{"normal", "hovered", "focused", "pressed", "disabled", "default"} {
		if !stateObserved[phase] {
			if status == "supported" {
				status = "deferred"
			}
			notes = append(notes, fmt.Sprintf("native ODS evidence for %s was not observed on this run", phase))
		}
	}
	evidence := cd0OwnerDrawEvidence{
		Routes:                  routes,
		StateCoverage:           stateCoverage,
		StateObserved:           stateObserved,
		ParentNCDESTROYObserved: parentNCDESTROYObserved,
		DestroyRouteCleanup:     destroyRouteCleanup,
		DrawItems:               drawItems,
	}
	cd0WriteJSON(t, "cd0-ownerdraw-probe.json", cd0ProbeEnvelope{
		Probe:     "CD0.3-CD0.4-owner-draw-subclass",
		Status:    status,
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		GoVersion: runtime.Version(),
		Evidence:  evidence,
		Notes:     notes,
	})
	t.Logf("CD0.3/CD0.4 result=%s routes=%+v states=%v", status, routes, stateObserved)
	if status == "unsupported" {
		t.Errorf("CD0.3/CD0.4 required owner-draw routing or teardown invariant failed: notes=%v", notes)
	}
}

func cd0WindowLongProc(pointerName, legacyName string) *syscall.LazyProc {
	if unsafe.Sizeof(uintptr(0)) == 4 {
		return cd0User32.NewProc(legacyName)
	}
	return cd0User32.NewProc(pointerName)
}

func cd0NewRuntimeRouter() (*cd0RuntimeRouter, error) {
	id := uintptr(cd0RuntimeRouterSequence.Add(1))
	router := &cd0RuntimeRouter{
		id:               id,
		parents:          make(map[uintptr]*cd0RuntimeParentRoute),
		childParents:     make(map[uintptr]uintptr),
		destroyedParents: make(map[uintptr]bool),
		callbackCounts:   make(map[uintptr]int),
	}
	cd0RuntimeRouters.Store(id, router)
	return router, nil
}

func (r *cd0RuntimeRouter) add(parent, child uintptr, label string) error {
	if parent == 0 || child == 0 {
		return fmt.Errorf("invalid HWND parent=%#x child=%#x", parent, child)
	}
	r.mu.Lock()
	entry := r.parents[parent]
	if entry == nil {
		entry = &cd0RuntimeParentRoute{HWND: parent, Children: make(map[uintptr]string)}
		r.parents[parent] = entry
	}
	needsSubclass := len(entry.Children) == 0
	entry.Children[child] = label
	r.childParents[child] = parent
	r.mu.Unlock()
	if !needsSubclass {
		return nil
	}
	ok, _, callErr := cd0ProcSetWindowSubclass.Call(parent, cd0RuntimeSubclassCallback, r.id, r.id)
	if ok == 0 {
		r.mu.Lock()
		delete(entry.Children, child)
		delete(r.childParents, child)
		delete(r.parents, parent)
		r.mu.Unlock()
		return fmt.Errorf("SetWindowSubclass(%#x) returned FALSE: %v", parent, callErr)
	}
	return nil
}

func (r *cd0RuntimeRouter) removeChild(child uintptr) bool {
	r.mu.Lock()
	parent := r.childParents[child]
	entry := r.parents[parent]
	if entry == nil {
		r.mu.Unlock()
		return false
	}
	delete(entry.Children, child)
	delete(r.childParents, child)
	removeSubclass := len(entry.Children) == 0
	if removeSubclass {
		delete(r.parents, parent)
	}
	r.mu.Unlock()
	if !removeSubclass {
		return true
	}
	ok, _, _ := cd0ProcRemoveWindowSubclass.Call(parent, cd0RuntimeSubclassCallback, r.id)
	return ok != 0
}

func (r *cd0RuntimeRouter) close() {
	r.mu.Lock()
	parents := make([]uintptr, 0, len(r.parents))
	for parent := range r.parents {
		parents = append(parents, parent)
	}
	r.parents = make(map[uintptr]*cd0RuntimeParentRoute)
	r.childParents = make(map[uintptr]uintptr)
	r.mu.Unlock()
	for _, parent := range parents {
		cd0ProcRemoveWindowSubclass.Call(parent, cd0RuntimeSubclassCallback, r.id)
	}
	cd0RuntimeRouters.Delete(r.id)
}

func (r *cd0RuntimeRouter) setPhase(phase string) {
	r.mu.Lock()
	r.phase = phase
	r.mu.Unlock()
}

func (r *cd0RuntimeRouter) callbackCount(parent uintptr) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callbackCounts[parent]
}

func (r *cd0RuntimeRouter) noteCallback(parent uintptr) {
	r.mu.Lock()
	r.callbackCounts[parent]++
	r.mu.Unlock()
}

func (r *cd0RuntimeRouter) drawItemRecords() []cd0DrawItemRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]cd0DrawItemRecord(nil), r.drawItems...)
}

func (r *cd0RuntimeRouter) customDrawRecords() []cd0CustomDrawRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]cd0CustomDrawRecord(nil), r.customDraws...)
}

func (r *cd0RuntimeRouter) parentWasDestroyed(parent uintptr) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.destroyedParents[parent]
}

func (r *cd0RuntimeRouter) hasParent(parent uintptr) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.parents[parent]
	return ok
}

func (r *cd0RuntimeRouter) hasChild(child uintptr) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.childParents[child]
	return ok
}

func (r *cd0RuntimeRouter) handleDrawItem(parent, lParam uintptr) bool {
	if lParam == 0 {
		return false
	}
	item := cd0ReadRuntimeDrawItem(lParam)
	r.mu.Lock()
	entry := r.parents[parent]
	label := ""
	if entry != nil {
		label = entry.Children[item.HWNDItem]
	}
	if label != "" {
		r.drawItems = append(r.drawItems, cd0DrawItemRecord{
			Phase:      r.phase,
			Control:    label,
			ParentHWND: parent,
			ItemHWND:   item.HWNDItem,
			CtlType:    item.CtlType,
			CtlID:      item.CtlID,
			ItemAction: item.ItemAction,
			ItemState:  item.ItemState,
			States:     cd0OwnerDrawStateNames(item.ItemState),
			HDC:        item.HDC,
			Rect:       item.Rect,
		})
	}
	r.mu.Unlock()
	return label != ""
}

func (r *cd0RuntimeRouter) handleCustomDraw(parent, lParam uintptr) (uintptr, bool) {
	if lParam == 0 {
		return 0, false
	}
	draw := cd0ReadRuntimeNMCustomDraw(lParam)
	if draw.Header.Code != cd0NMCustomDrawCode {
		return 0, false
	}
	r.mu.Lock()
	entry := r.parents[parent]
	label := ""
	if entry != nil {
		label = entry.Children[draw.Header.HWNDFrom]
	}
	if label != "" {
		r.customDraws = append(r.customDraws, cd0CustomDrawRecord{
			Control:    label,
			ParentHWND: parent,
			HWNDFrom:   draw.Header.HWNDFrom,
			DrawStage:  draw.DrawStage,
			Stage:      cd0CustomDrawStageName(draw.DrawStage),
			ItemSpec:   draw.ItemSpec,
			ItemState:  draw.ItemState,
			HDC:        draw.HDC,
			Rect:       draw.Rect,
		})
	}
	r.mu.Unlock()
	if label == "" {
		return 0, false
	}
	if draw.DrawStage == cd0CDDSPrePaint {
		return cd0CDRFNotifyItemDraw | cd0CDRFNotifyPostPaint, true
	}
	return cd0CDRFDoDefault, true
}

// The message payload belongs to Win32, not Go. Copy it while the callback is
// active instead of reinterpreting an external uintptr as a Go pointer.
func cd0ReadRuntimeDrawItem(address uintptr) cd0RuntimeDrawItem {
	var item cd0RuntimeDrawItem
	cd0ProcRtlMoveMemory.Call(uintptr(unsafe.Pointer(&item)), address, unsafe.Sizeof(item))
	return item
}

func cd0ReadRuntimeNMCustomDraw(address uintptr) cd0RuntimeNMCustomDraw {
	var draw cd0RuntimeNMCustomDraw
	cd0ProcRtlMoveMemory.Call(uintptr(unsafe.Pointer(&draw)), address, unsafe.Sizeof(draw))
	return draw
}

func (r *cd0RuntimeRouter) parentDestroyed(parent uintptr) {
	r.mu.Lock()
	r.destroyedParents[parent] = true
	entry := r.parents[parent]
	if entry != nil {
		for child := range entry.Children {
			delete(r.childParents, child)
		}
		delete(r.parents, parent)
	}
	r.mu.Unlock()
}

func cd0RuntimeSubclassProc(hwnd, msg, wParam, lParam, _ uintptr, refData uintptr) uintptr {
	value, ok := cd0RuntimeRouters.Load(refData)
	if !ok {
		result, _, _ := cd0ProcDefSubclassProc.Call(hwnd, msg, wParam, lParam)
		return result
	}
	router := value.(*cd0RuntimeRouter)
	router.noteCallback(hwnd)
	switch uint32(msg) {
	case cd0WMDrawItem:
		if router.handleDrawItem(hwnd, lParam) {
			return 1
		}
	case cd0WMNotify:
		if result, handled := router.handleCustomDraw(hwnd, lParam); handled {
			return result
		}
	case cd0WMNCDestroy:
		router.parentDestroyed(hwnd)
	}
	result, _, _ := cd0ProcDefSubclassProc.Call(hwnd, msg, wParam, lParam)
	return result
}

func cd0SetOwnerDrawStyle(hwnd uintptr) error {
	styleIndex := cd0GWLPStyle
	style, _, getErr := cd0ProcGetWindowLongPtr.Call(hwnd, uintptr(styleIndex))
	if style == 0 {
		return fmt.Errorf("GetWindowLongPtrW(GWL_STYLE): %v", getErr)
	}
	updated := (style &^ uintptr(cd0BSTypeMask)) | uintptr(cd0BSOwnerDraw)
	previous, _, setErr := cd0ProcSetWindowLongPtr.Call(hwnd, uintptr(styleIndex), updated)
	if previous == 0 {
		return fmt.Errorf("SetWindowLongPtrW(GWL_STYLE): %v", setErr)
	}
	flags := uintptr(cd0SWPNoSize | cd0SWPNoMove | cd0SWPNoZOrder | cd0SWPNoActivate | cd0SWPFrameChanged)
	ok, _, posErr := cd0ProcSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, flags)
	if ok == 0 {
		return fmt.Errorf("SetWindowPos(SWP_FRAMECHANGED): %v", posErr)
	}
	return nil
}

func cd0ControlHWND(t *testing.T, control lcl.IControl) uintptr {
	t.Helper()
	win, ok := control.(lcl.IWinControl)
	if !ok {
		t.Fatalf("CD0 native probe control %T has no HWND", control)
	}
	win.HandleNeeded()
	if win.Handle() == 0 {
		t.Fatalf("CD0 native probe control %T did not allocate an HWND", control)
	}
	return uintptr(win.Handle())
}

func cd0ParentHWND(child uintptr) uintptr {
	parent, _, _ := cd0ProcGetParent.Call(child)
	return parent
}

func cd0RepaintWindow(hwnd uintptr) {
	cd0ProcInvalidateRect.Call(hwnd, 0, 1)
	cd0ProcUpdateWindow.Call(hwnd)
	lcl.Application.ProcessMessages()
}

func cd0WindowClass(hwnd uintptr) string {
	buffer := make([]uint16, 256)
	length, _, _ := cd0ProcGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if length == 0 {
		return ""
	}
	return syscall.UTF16ToString(buffer[:length])
}

func cd0OwnerDrawStateNames(state uint32) []string {
	known := []struct {
		mask uint32
		name string
	}{
		{cd0ODSSelected, "selected"},
		{cd0ODSGrayed, "grayed"},
		{cd0ODSDisabled, "disabled"},
		{cd0ODSChecked, "checked"},
		{cd0ODSFocus, "focus"},
		{cd0ODSDefault, "default"},
		{cd0ODSHotLight, "hotlight"},
		{cd0ODSInactive, "inactive"},
		{cd0ODSNoAccel, "no-accel"},
		{cd0ODSNoFocusRect, "no-focus-rect"},
	}
	var result []string
	for _, item := range known {
		if state&item.mask != 0 {
			result = append(result, item.name)
		}
	}
	return result
}

func cd0CustomDrawStageName(stage uint32) string {
	switch stage {
	case cd0CDDSPrePaint:
		return "prepaint"
	case cd0CDDSPostPaint:
		return "postpaint"
	case cd0CDDSItemPrePaint:
		return "item-prepaint"
	default:
		return fmt.Sprintf("0x%08X", stage)
	}
}

func cd0PhaseHasDraw(records []cd0DrawItemRecord, phase, control string) bool {
	for _, record := range records {
		if record.Phase == phase && record.Control == control {
			return true
		}
	}
	return false
}

func cd0PhaseHasState(records []cd0DrawItemRecord, phase, control string, state uint32) bool {
	for _, record := range records {
		if record.Phase == phase && record.Control == control && record.ItemState&state != 0 {
			return true
		}
	}
	return false
}
