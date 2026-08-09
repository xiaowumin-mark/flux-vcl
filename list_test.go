package flux_test

import (
	"fmt"
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/diff"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// Phase 6 列表与虚拟化无头测试：ListView（稳定 slot key）＋ 虚拟化（10 万行控件池）
// ＋ 滚动双向绑定（ScrollOffset）。全部走 App+Mock，不依赖 DLL/显示环境。

// buildList 构造一棵 10 万行虚拟列表的 Widget 树（Phase 6 demo 的最小等价物）。
// 行内容 = 单 Text（"row <idx>"），每行等高 itemH=20（DIP）。ScrollOffset 把
// 滚动位置绑定到 scroll State（双向：编程 Set / 用户滚动回写）。
func buildList(scroll *flux.State[int], count, itemH int) func() flux.Widget {
	return func() flux.Widget {
		return flux.Window(
			flux.ListView(count, itemH, func(idx int) flux.Widget {
				return flux.Text(fmt.Sprintf("row %d", idx))
			}, flux.ScrollOffset(scroll)),
		)
	}
}

// findListView 深度优先查找 ListView Element（测试驱动用；控件池 slot 与行内容
// 都挂在它下面）。
func findListView(t *testing.T, e *diff.Element) *diff.Element {
	t.Helper()
	if e.Type == "ListView" {
		return e
	}
	for _, c := range e.Children {
		if f := findListView(t, c); f != nil {
			return f
		}
	}
	return nil
}

// rowText 返回 slot key="row-<i>" 内 Text 的渲染文本（验证内容随滚动原地 patch）。
func rowText(t *testing.T, lv *diff.Element, i int) string {
	t.Helper()
	for _, c := range lv.Children {
		if c.Key == fmt.Sprintf("row-%d", i) {
			// slot 是透明包装（ListViewRow），其唯一子即 builder 产物
			if len(c.Children) == 0 {
				return ""
			}
			return c.Children[0].Props.String("Text")
		}
	}
	return ""
}

// TestVirtualizationBoundedControls 6.2 虚拟化：10 万行只挂载"可见区±overscan"的
// 行控件（mock 视口 400x300、行高 20 → 可见 15 行 + 上下各 3 行缓冲 = 18 个 slot）。
// 10 万行若逐个建控件（Create 10 万次）即失败 —— 控件池是内存有界的硬保证。
func TestVirtualizationBoundedControls(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	scroll := flux.NewState(0)

	app.Mount(buildList(scroll, 100000, 20))

	lv := findListView(t, app.Root())
	if lv == nil {
		t.Fatal("ListView 未挂载")
	}
	if got := m.ScrollContent(lv.Handle); got != 100000*20 {
		t.Errorf("ScrollContent = %d，期望 %d（内容总高 = 行数×行高）", got, 100000*20)
	}
	nText := 0
	for _, op := range m.Ops() {
		if op.Type == render.OpCreate && op.Key == "Text" {
			nText++
		}
	}
	if nText != 18 {
		t.Errorf("虚拟化失败：Text 控件数 = %d，期望 18（可见 15 + overscan 6）", nText)
	}
	// 控件总数 = Window + ListView + 18 行 Text（无按 10 万扩展）
	if got := m.Count(render.OpCreate); got != 20 {
		t.Errorf("总控件数 = %d，期望 20", got)
	}
}

// TestVirtualizedScrollReusesControls 滚动 = 属性 patch，不重建：中段滚动（窗口
// 稳定，无上下界钳制）时控件池零 Create/Destroy，行内容原地 SetText，滚动位置
// 同步到原生侧（SetScrollPos）。
func TestVirtualizedScrollReusesControls(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	scroll := flux.NewState(5000) // 中段起始（第一帧即稳定窗口）

	app.Mount(buildList(scroll, 100000, 20))
	lv := findListView(t, app.Root())
	if lv == nil {
		t.Fatal("ListView 未挂载")
	}
	if got := m.ScrollPos(lv.Handle); got != 5000 {
		t.Errorf("初始 ScrollPos = %d，期望 5000（ScrollOffset 编程定位）", got)
	}

	base := len(m.Ops())
	scroll.Set(5200) // 编程滚动（State → re-render → 布局重算可见区）
	ops := m.Ops()[base:]

	if n := countOps(ops, render.OpCreate); n != 0 {
		t.Errorf("滚动后 Create = %d，期望 0（控件池复用）", n)
	}
	if n := countOps(ops, render.OpDestroy); n != 0 {
		t.Errorf("滚动后 Destroy = %d，期望 0（无销毁）", n)
	}
	if n := countOps(ops, render.OpSetText); n == 0 {
		t.Error("滚动后应有行内容 SetText（同 slot 内容原地 patch）")
	}
	if got := m.ScrollPos(lv.Handle); got != 5200 {
		t.Errorf("滚动后 ScrollPos = %d，期望 5200", got)
	}
	// 首槽内容 = 数据行 (offset/20 - overscan 3) = 260-3 = 257
	if got := rowText(t, lv, 0); got != "row 257" {
		t.Errorf("首槽文本 = %q，期望 row 257（滚动后原地更新）", got)
	}
}

// TestScrollEventRoundTrip 用户滚动回写：FireScroll 模拟滚动输入 → OnScroll 回调 →
// State.Set → re-render → 布局按新 offset 重算并 SetScrollPos（双向绑定闭环）。
func TestScrollEventRoundTrip(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	scroll := flux.NewState(0)

	app.Mount(buildList(scroll, 100000, 20))
	lv := findListView(t, app.Root())
	if lv == nil {
		t.Fatal("ListView 未挂载")
	}

	m.FireScroll(lv.Handle, 200) // 模拟滚轮/滚动条拖动到 200
	if got := scroll.Get(); got != 200 {
		t.Errorf("用户滚动后 State = %d，期望 200（OnScroll 回写）", got)
	}
	if got := m.ScrollPos(lv.Handle); got != 200 {
		t.Errorf("re-render 后 ScrollPos = %d，期望 200", got)
	}
	// 首槽内容 = 200/20 - 3 = 7
	if got := rowText(t, lv, 0); got != "row 7" {
		t.Errorf("首槽文本 = %q，期望 row 7", got)
	}
}

// TestScrollClampedToRange 滚动位置钳制：超出内容范围 → 布局钳到 [0, content-viewH]，
// 原生侧（ScrollPos）与 State 同步。负数同理。
func TestScrollClampedToRange(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	scroll := flux.NewState(999999) // 远超 maxOffset

	app.Mount(buildList(scroll, 100, 20)) // 内容 2000，视口 300 → maxOffset 1700
	lv := findListView(t, app.Root())
	if lv == nil {
		t.Fatal("ListView 未挂载")
	}
	if got := m.ScrollPos(lv.Handle); got != 1700 {
		t.Errorf("ScrollPos = %d，期望 1700（钳到内容−视口）", got)
	}
	if got := scroll.Get(); got != 1700 {
		t.Errorf("State = %d，期望 1700（布局钳制回写）", got)
	}

	scroll.Set(-50)
	if got := m.ScrollPos(lv.Handle); got != 0 {
		t.Errorf("负偏移 ScrollPos = %d，期望 0", got)
	}
}

// TestListViewZeroMutationOnSameTree D7c：相同 ListView 树二次 render 零 mutation。
// Builder（func）每次重建不可比 → 但 diff 对其显式忽略（不产生 op）；Scroll prop
// 是可比值类型（同 State 指针）→ 不重复绑定 OnScroll。列表内容不变时槽位原地复用。
func TestListViewZeroMutationOnSameTree(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	scroll := flux.NewState(100)

	app.Mount(buildList(scroll, 100000, 20))
	base := len(m.Ops())
	app.Render(buildList(scroll, 100000, 20)())
	if len(m.Ops()) != base {
		ops := m.Ops()[base:]
		t.Errorf("相同树二次 render 应零 mutation，实际 %d 条：%v", len(ops), ops)
	}
}

// TestListViewRequiresBoundedConstraints 无界约束（Column 直接放、未给高度）→
// 明确 panic（虚拟列表必须有 viewport；Flutter Expanded 语义，勿静默退化）。
func TestListViewRequiresBoundedConstraints(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	scroll := flux.NewState(0)

	defer func() {
		if r := recover(); r == nil {
			t.Error("无界约束应 panic（提示放 Expanded/固定高度内）")
		}
	}()
	app.Render(flux.Window(flux.Column(
		flux.ListView(100, 20, func(int) flux.Widget { return flux.Text("x") },
			flux.ScrollOffset(scroll)),
	)))
}

// TestListViewRowBounds 行局部坐标布局：第 i 槽位于 Y = (数据行 − offset)，视口内
// 只出现可见行（首槽随滚动平移，diff 用 Bounds 属性级 patch 行位置）。
func TestListViewRowBounds(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	scroll := flux.NewState(40) // 行高 20 → 滚两行

	app.Mount(buildList(scroll, 1000, 20))
	lv := findListView(t, app.Root())
	if lv == nil {
		t.Fatal("ListView 未挂载")
	}
	// 首槽（row-0）显示数据行 0（first=40/20-3=-1 钳到 0），Y = 0*20-40 = -40（滚出视口顶）
	if got := boundsOf(lv.Children[0]).Y; got != -40 {
		t.Errorf("row-0 Y = %d，期望 -40（局部坐标，内容上移 offset）", got)
	}
	// 数据行 2（= 首可见非负行）在第 3 槽（row-2，first+i=0+2）：Y = 2*20-40 = 0
	if got := boundsOf(lv.Children[2]).Y; got != 0 {
		t.Errorf("row-2 Y = %d，期望 0（数据行 2 恰好回到视口顶）", got)
	}
}

// TestListViewBuilderStateRequiresBind 回归（Phase 6 实测：demo 点击行标记要等 resize
// 才看到反应 —— 根因即本用例）。State 只被 ListView 行 builder 读取（sel.Get()）、
// 未被 Bind → sel.Set 只更新内存值、不触发 re-render（state.go Set 只通知 collectBindings
// 登记过的 App，即"State 须 Bind 才触发 re-render"，design §9；phase5 主题 chip
// commit e1528aa 同坑）。要响应变化须把该 State 也 Bind 出来（如 header 读数）。
func TestListViewBuilderStateRequiresBind(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	sel := flux.NewState(-1)
	scroll := flux.NewState(0)

	mk := func(bindSel bool) func() flux.Widget {
		return func() flux.Widget {
			lv := flux.ListView(100, 20, func(idx int) flux.Widget {
				mark := "○"
				if sel.Get() == idx {
					mark = "●"
				}
				return flux.Text(mark)
			}, flux.ScrollOffset(scroll))
			var header flux.Widget
			if bindSel {
				header = flux.Text(flux.Bind(sel)) // 订阅 sel → Set 触发 re-render
			} else {
				header = flux.Text("sel 未绑定")
			}
			return flux.Window(flux.Column(header, flux.Expanded(lv)))
		}
	}

	// 1) 未绑定：sel.Set 零新 op（无 re-render，行标记仍 ○ —— 与 demo 点击无反应一致）
	app.Mount(mk(false))
	base := len(m.Ops())
	sel.Set(5)
	if got := len(m.Ops()) - base; got != 0 {
		t.Errorf("未绑定的 sel.Set 不应触发 re-render，实际 %d 条 op：%v", got, m.Ops()[base:])
	}

	// 2) 绑定后：sel.Set 触发 re-render → 行标记 patch 为 ●（无需 resize）。
	// 注：part 1 的 sel.Set(5) 已把 sel 置为 5，故此处换 7 才能观察到变化。
	app.Mount(mk(true))
	base = len(m.Ops())
	sel.Set(7)
	found := false
	for _, op := range m.Ops()[base:] {
		if op.Type == render.OpSetText && op.Value == "●" {
			found = true
		}
	}
	if !found {
		t.Errorf("绑定后 sel.Set 应把行标记 patch 为 ●，ops=%v", m.Ops()[base:])
	}
}
