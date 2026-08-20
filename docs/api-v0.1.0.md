# v0.1.0 候选 API 冻结清单

本文档以当前根包 `github.com/xiaowumin-mark/flux-vcl` 和默认后端包
`github.com/xiaowumin-mark/flux-vcl/native` 的 `go doc -all` 输出与源码导出项为
基线，记录已冻结的候选公开 API，包括产品化、控件扩充批次 3 与 P7.6
Accessibility/i18n。

> **候选状态：** 当前 `Version` 仍为 `0.1.0-dev`，尚未正式发布；本文列出的
> v0.1.0 公开构造器、Opt、事件签名、默认值和受控语义已冻结为首发基线。
> 标签前只接受保持源码兼容的缺陷修复；正式 SemVer 兼容承诺从 v0.1.0 标签开始。

## 1. 包常量

```go
const Version = "0.1.0-dev"
```

事件类型 `EventType`：

```go
const (
	EventClick EventType = iota
	EventMouseDown
	EventMouseUp
	EventMouseMove
	EventMouseEnter
	EventMouseLeave
	EventKeyDown
	EventKeyUp
	EventKeyPress
)
```

鼠标按键 `MouseButton`：

```go
const (
	ButtonNone MouseButton = iota
	ButtonLeft
	ButtonRight
	ButtonMiddle
)
```

修饰键 `Modifier`，可按位组合：

```go
const (
	ModShift Modifier = 1 << iota
	ModCtrl
	ModAlt
	ModWin
)
```

PaintBox 命令：

```go
const (
	PaintClear PaintCommandKind = iota + 1
	PaintCircle
)
```

Flex 对齐：

```go
const (
	MainAxisStart MainAxisAlignment = iota
	MainAxisCenter
	MainAxisEnd
	MainAxisSpaceBetween
	MainAxisSpaceAround
	MainAxisSpaceEvenly
)

const (
	CrossAxisStart CrossAxisAlignment = iota
	CrossAxisCenter
	CrossAxisEnd
	CrossAxisStretch
)
```

## 2. 包变量

主题预设：

```go
var LightTheme Theme
var DarkTheme Theme
```

两者当前的 `FontSize` 均为 14、`Radius` 均为 4；`LightTheme.DarkTitleBar`
为 `false`，`DarkTheme.DarkTitleBar` 为 `true`。它们是可变的包变量，调用方需要
隔离修改时应先复制值。

应用与插件错误哨兵：

```go
var ErrAppCloseDuringRender error
var ErrPluginInvalid error
var ErrPluginReserved error
var ErrPluginAlreadyRegistered error
var ErrPluginNotRegistered error
var ErrPluginInUse error
var ErrPluginPanic error
var ErrPluginCycle error
var ErrAppClosed error
var ErrInvalidCatalog error
```

这些对象的 identity 稳定，可用于 `errors.Is`；`Error()` 显示文本按当前诊断 Catalog
解析，因此不能用字符串比较代替 identity 判断。

插件可选能力令牌：

```go
var RendererDPI Capability[int]       // 名称 "flux.renderer.dpi"
var RendererBackend Capability[string] // 名称 "flux.renderer.backend"
```

缺少 `RendererDPI` 时插件应按 96 DPI 退化；当前默认后端的
`RendererBackend` 值为 `"lcl"`。

## 3. 核心类型

```go
type Widget = widget.Widget // 有效接口：Create() *Node
type Node = widget.Node
type Opt interface { /* 包内 apply 方法，外部不能自行实现 */ }
```

`Widget` 是声明式 UI 描述。`Node` 的有效导出字段为 `Type string`、
`Props *widget.Props`、`Children []*Node` 和 `Key string`，并带有
`func (n *Node) Add(c *Node) *Node`。通常应通过根包构造器创建树；`Node` 和
`Widget` 目前是内部类型的公开别名。

```go
type State[T any] struct { /* unexported fields */ }
type Binding[T any] struct { /* unexported fields */ }

func NewState[T any](initial T) *State[T]
func (s *State[T]) Get() T
func (s *State[T]) Set(v T)
func Bind[T any](s *State[T]) *Binding[T]
```

`State.Get`/`Set` 线程安全；`Set` 通知已订阅的 App。`Binding` 可用于单向文本
显示；`string` 和 `int` 还支持 Input/Memo 文本回写，其他类型只作单向显示。

```go
type Locale string
type MessageID string
type Messages map[MessageID]string
type Resources map[Locale]Messages
type Catalog struct { /* unexported fields */ }
type MessageBinding struct { /* unexported fields */ }

func NewCatalog(fallback Locale, resources Resources) (*Catalog, error)
func MustCatalog(fallback Locale, resources Resources) *Catalog
func (c *Catalog) Fallback() Locale
func (c *Catalog) Resources() Resources
func (c *Catalog) Lookup(locale Locale, id MessageID) (string, bool)
func (c *Catalog) Format(locale Locale, id MessageID, args ...any) string
func (c *Catalog) Bind(locale *State[Locale], id MessageID, args ...any) *MessageBinding
```

Catalog 在构造时校验并深复制 Resources；精确 locale 缺项时回落 fallback，仍缺失
则 `Format` 返回 Message ID。`MessageBinding` 是只读文本绑定，可传给 Text、Button、
CheckBox、RadioButton 或 Memo；locale State 变化时原地 patch 文本。

```go
type Ref struct { /* unexported fields */ }

func (r *Ref) Get() any
func (r *Ref) SetNative(obj any)
func BindRef(r *Ref) Opt
func Native[T any](fn func(c T)) Opt
```

`Ref.Get` 在尚未绑定时返回 `nil`。`Native` 和 `Ref` 是后端原生对象逃逸口；
类型断言、线程归属和原生对象生命周期由调用方负责。

## 4. 控件与组合构造器

| 签名 | 当前契约 |
| --- | --- |
| `func Window(args ...any) Widget` | 顶层窗体；参数可混排 `Widget` 与 `Opt`。 |
| `func Column(args ...any) Widget` | 透明垂直 flex 容器；参数可混排子节点与容器 Opt。 |
| `func Row(args ...any) Widget` | 透明水平 flex 容器；参数可混排子节点与容器 Opt。 |
| `func ScrollBox(child Widget) Widget` | 单子垂直滚动容器。 |
| `func PageControl(args ...any) Widget` | 参数只允许 `TabPage` 与 Opt；每页必须有唯一非空 Key。 |
| `func TabPage(title string, child Widget, opts ...Opt) Widget` | PageControl 页面；必须有唯一子树和非空 Key。 |
| `func Text(text any, opts ...Opt) Widget` | 文本标签；`text` 为 `string` 或 Binding。 |
| `func Button(text any, opts ...Opt) Widget` | 按钮；`text` 为 `string` 或 Binding。 |
| `func Input(opts ...Opt) Widget` | 单行输入框；可传 `Bind(state)`。 |
| `func Memo(text any, opts ...Opt) Widget` | 多行输入框；`text` 为 `string` 或 Binding。 |
| `func CheckBox(text any, opts ...Opt) Widget` | 显式受控复选框。 |
| `func RadioButton(text any, opts ...Opt) Widget` | 显式受控单选框，同一原生父级内按 `GroupIndex` 分组。 |
| `func ComboBox(opts ...Opt) Widget` | `Items`、`SelectedIndex` 显式受控的下拉框。 |
| `func ProgressBar(opts ...Opt) Widget` | 整数进度条；统一规范化 Minimum/Maximum/Value。 |
| `func Slider(opts ...Opt) Widget` | 水平整数滑块；批次 3，详见第 12 节。 |
| `func StringGrid(rows, columns int, opts ...Opt) Widget` | 有界字符串表格；批次 3，详见第 12 节。 |
| `func PaintBox(commands []PaintCommand, opts ...Opt) Widget` | 值命令驱动的绘图控件；批次 3，详见第 12 节。 |
| `func ListView(count, itemHeight int, builder func(index int) Widget, opts ...Opt) Widget` | 固定行高的虚拟列表；需要有界 viewport。 |
| `func Component(build func() Widget, opts ...Opt) Widget` | 透明组合组件；动态身份应显式设置 Key。 |
| `func Expanded(child Widget, flex ...int) Widget` | tight flex，默认因子 1，因子必须大于 0。 |
| `func Flexible(child Widget, flex ...int) Widget` | loose flex，默认因子 1，因子必须大于 0。 |
| `func PluginWidget(name string, properties PluginProperties, args ...any) Widget` | 已注册组合式插件节点；参数可混排子节点与 Opt。 |

`ScrollBox` 的滚动轴内部使用无界约束，因此其中的 `Expanded` 当前会压缩为 0。
`ListView` 应放在 `Expanded` 或显式 Height 等有界布局中。

## 5. 属性 Opt

通用、布局与外观：

```go
func Title(s string) Opt
func Key(k string) Opt
func Width(v int) Opt
func Height(v int) Opt
func Visible(v bool) Opt
func Enabled(v bool) Opt
func MainAxis(a MainAxisAlignment) Opt
func CrossAxis(a CrossAxisAlignment) Opt
func DarkTitleBar(dark bool) Opt
func Color(c ColorValue) Opt
func FontColor(c ColorValue) Opt
func AccessibleName(name string) Opt
func AccessibleDescription(description string) Opt
func AccessibleValue(value string) Opt
func TabStop(enabled bool) Opt
func DefaultButton(enabled bool) Opt
func CancelButton(enabled bool) Opt
```

尺寸与坐标均为 DIP。`Key` 是 reconciliation 的稳定身份，不是树路径；动态
列表、可变子树、Component 和 `App.SetBounds` 目标应使用来自业务模型的稳定 Key。
可访问属性由后端可选 capability 接收；默认后端会写入 LCL 对象，但锁定 runtime
不会把这些值投射到 Windows UIA，因此该 API 不等于当前屏幕阅读器支持。Tab 顺序
由声明树自动生成，`TabStop` 移除时恢复控件类型默认值。DefaultButton/CancelButton
仅对 Button 生效，同一窗体应分别最多声明一个。

选择、数值和列表：

```go
func Checked(v bool) Opt
func Items(items []string) Opt
func SelectedIndex(index int) Opt
func Minimum(value int) Opt
func Maximum(value int) Opt
func Value(value int) Opt
func Step(value int) Opt
func GroupIndex(index int) Opt
func ScrollOffset(s *State[int]) Opt
```

`Items` 会防御性复制。`SelectedIndex(-1)` 表示未选择；ComboBox/PageControl
会把越界索引规范化。Minimum/Maximum/Value 适用于 ProgressBar 和 Slider。
`ScrollOffset` 为 ListView 的双向滚动位置绑定。

StringGrid：

```go
func Cells(values [][]string) Opt
func Headers(values []string) Opt
func ColumnWidths(values []int) Opt
func SelectedCell(row, column int) Opt
func SelectedRow(row int) Opt
func Editable(value bool) Opt
```

维度、复制和选择规则见第 12 节。

## 6. 事件与生命周期 Opt

统一事件：

```go
func OnClick(fn func(Event)) Opt
func OnMouseDown(fn func(Event)) Opt
func OnMouseUp(fn func(Event)) Opt
func OnMouseMove(fn func(Event)) Opt
func OnMouseEnter(fn func(Event)) Opt
func OnMouseLeave(fn func(Event)) Opt
func OnKeyDown(fn func(Event)) Opt
func OnKeyUp(fn func(Event)) Opt
func OnKeyPress(fn func(Event)) Opt
```

鼠标坐标为事件源客户区内的 DIP。KeyDown/KeyUp 的 `Event.Key` 是虚拟键码；
KeyPress 的 `Event.Text` 是 UTF-8 字符或 IME 组合结果。事件回调会在每次 render
重新绑定。

类型化值事件：

```go
func OnChange(fn func(text string)) Opt
func OnCheckedChange(fn func(checked bool)) Opt
func OnSelectionChange(fn func(index int)) Opt
func OnValueChange(fn func(value int)) Opt
func OnCellSelect(fn func(cell GridCell)) Opt
func OnCellEdit(fn func(cell GridCell, value string)) Opt
```

这些回调是受控值的回写入口；调用方应更新业务 State，并在下一次 render
继续声明值。`Input`/`Memo` 的程序化 `Text` patch 会抑制同步原生 `OnChange`，因此
公开 `OnChange` 只表示用户编辑；`Bind` 也只会把用户编辑后的文本写回 State。其他
控件的程序化 patch 回调语义由各自章节明确，不应把程序化 patch 当作用户交互处理。

生命周期：

```go
func OnMount(fn func()) Opt
func OnUpdate(fn func()) Opt
func OnUnmount(fn func()) Opt
```

Mount 在首次挂载后调用，Update 在节点成功复用后调用，Unmount 在原生控件释放前
调用。

## 7. 事件数据

```go
type Event = render.Event

// Event 的有效字段
type Event struct {
	Type   EventType
	X, Y   int
	Key    uint16
	Text   string
	Button MouseButton
	Mods   Modifier
	Source string
}

type EventType = render.EventType
type MouseButton = render.MouseButton
type Modifier = render.Modifier
```

`EventType` 通过别名带有 `func (t EventType) String() string`。`Source` 在有 Key
时为 `"Type#Key"`，无 Key 时回落为包含树路径的标识。

## 8. App 与异步工作

```go
type App struct { /* unexported fields */ }
type Element = diff.Element

func (e *Element) FindByPath(path string) *Element

func NewApp(r render.Renderer) *App
func (a *App) Mount(build func() Widget) error
func (a *App) Render(w Widget) error
func (a *App) Close() error
func (a *App) LastError() error
func (a *App) Root() *Element
func (a *App) Animate(duration time.Duration, curve Curve, onStep func(v float64)) (stop func())
func (a *App) SetBounds(key string, r Rect)
func (a *App) FindByPath(path string) *Element
func (a *App) Inspect() []NodeDiag
func (a *App) LastLayoutDiags() []LayoutDiag
func (a *App) ObserveInspector(observer InspectorObserver) (unsubscribe func())
func (a *App) InspectorSnapshot() InspectorSnapshot

func Async[T any](a *App, load func() (T, error), onSuccess func(T), onError ...func(error))
```

`Element` 别名当前可见的导出字段如下；它们只用于 Inspector、诊断和测试读取，
应用不得修改协调树或把 `Props` / `Handle` 当作后端扩展接口：

| 字段 | 类型 / 语义 |
|---|---|
| `Type` / `Key` / `Path` | `string`；控件类型、稳定 Key 与当前树路径 |
| `Props` | opaque `*widget.Props`；最近一次成功协调的属性快照 |
| `Handle` | opaque `render.Handle`；当前原生句柄或透明节点继承的父句柄 |
| `Parent` | `*Element`；父元素，根为 nil |
| `Children` | `[]*Element`；按当前声明顺序排列的子元素 |

`Element.FindByPath` 从接收元素向下按同一位置路径规则查找；nil 接收者或未命中均
返回 nil。`Parent`、`Children`、`Props` 和 `Handle` 的底层类型身份不构成公开扩展点。

`Mount` 注册可重复构建的根函数并首次 render；State 更新会触发后续 render。
`Render` 是手动兼容路径，不建立 State 自动订阅。`Close` 幂等，并可能返回
`ErrAppCloseDuringRender`。插件或异步生命周期错误可由 `LastError` 读取。

`SetBounds` 按稳定 Key 直接应用 DIP 几何，不重跑 diff/layout；透明节点、Window
和 TabPage 不作为直接目标。`FindByPath` 使用形如
`"Window/0/Column/1/Text"` 的位置路径，结构改变后路径也会改变。

`Animate` 由 UI 线程定时器推进；时长为 0 时立即回调 1。`Async` 在后台执行
`load`，再通过 Renderer 的 UI 调度调用成功或错误回调。

## 9. 布局、颜色与动画

几何与约束：

```go
type Size struct{ W, H int }
type Point struct{ X, Y int }
type Rect = render.Rect // 有效字段：X, Y, W, H int

type BoxConstraints struct {
	MinW, MaxW int
	MinH, MaxH int
}

func Tight(w, h int) BoxConstraints
func Loose(w, h int) BoxConstraints
func Unbounded() BoxConstraints
func (c BoxConstraints) Constrain(w, h int) Size
func (c BoxConstraints) IsUnboundedW() bool
func (c BoxConstraints) IsUnboundedH() bool

type MainAxisAlignment int
type CrossAxisAlignment int
```

所有值均为 DIP；`MaxW`/`MaxH` 的 `-1` 表示无上界。

颜色与主题：

```go
type ColorValue = render.Color // 0xAARRGGBB

func RGB(r, g, b uint8) ColorValue

type Theme struct {
	Primary      ColorValue
	Background   ColorValue
	Surface      ColorValue
	Text         ColorValue
	Accent       ColorValue
	DarkTitleBar bool
	FontSize     int
	Radius       int
}
```

`RGB` 产生 alpha 为 `0xFF` 的不透明颜色。当前 Win32 后端可靠呈现 Window 背景、
文字颜色和暗色标题栏；`FontSize`、`Radius` 目前仍是文档字段，尚未统一接入原生
字体缩放和圆角。

动画：

```go
type Curve func(t float64) float64

func LinearCurve(t float64) float64
func EaseIn(t float64) float64
func EaseOut(t float64) float64
func EaseInOut(t float64) float64
func ElasticOut(t float64) float64
func Tween[T ~float64 | ~int | ~int32 | ~int64](from, to T, t float64) T

type AnimationController struct { /* unexported fields */ }

func NewAnimationController(duration time.Duration, curve Curve) *AnimationController
func (c *AnimationController) Start(onStep func(v float64), onEnd ...func())
func (c *AnimationController) Stop()
func (c *AnimationController) Running() bool
func (c *AnimationController) Step(dt time.Duration) (v float64, done bool)
```

曲线输入假定在 `[0,1]`；`ElasticOut` 可短暂越界。`AnimationController` 不持有
定时器，线程安全，可由 `App.Animate` 或测试代码驱动。

## 10. 布局诊断与 Inspector

基础诊断：

```go
type LayoutDiag struct {
	Type      string
	Key       string
	Path      string
	OverflowW int
	OverflowH int
}

type NodeDiag struct {
	Type, Key   string
	Path        string
	Constraints BoxConstraints
	Size        Size
	Frame       render.Rect
	Flex        int
}
```

Inspector observer 与快照：

```go
type InspectorObserver interface {
	OnInspectorCommit(InspectorCommit)
	OnInspectorEvent(InspectorEventRecord)
}

type InspectorObserverFuncs struct {
	Commit func(InspectorCommit)
	Event  func(InspectorEventRecord)
}

func (f InspectorObserverFuncs) OnInspectorCommit(c InspectorCommit)
func (f InspectorObserverFuncs) OnInspectorEvent(e InspectorEventRecord)

type InspectorSnapshot struct {
	RenderID uint64
	Root     *InspectorNode
	Commit   InspectorCommit
}

type InspectorNode struct {
	WidgetType  string
	ElementType string
	Key         string
	Path        string
	ParentPath  string
	Props       []InspectorProperty
	Layout      InspectorLayout
	Native      InspectorNative
	Overflow    *LayoutDiag
	Rebuilt     bool
	Children    []*InspectorNode
}

type InspectorProperty struct {
	Name  string
	Value string
}

type InspectorLayout struct {
	Constraints BoxConstraints
	Size        Size
	Bounds      Rect
	Flex        int
}

type InspectorNative struct {
	ID        uint64
	ParentID  uint64
	Type      string
	Allocated bool
	Shared    bool
}
```

提交、mutation、rebuild 与事件记录：

```go
type InspectorCommit struct {
	RenderID  uint64
	Direct    bool
	Mutations []InspectorMutation
	Rebuilds  []InspectorRebuild
	Stats     InspectorCommitStats
}

type InspectorCommitStats struct {
	Total, Create, Destroy, Reparent, Property, Event, Bounds int
}

type InspectorMutation struct {
	Index      int
	Kind       string
	Path       string
	ParentPath string
	NativeID   uint64
	ParentID   uint64
	Property   string
	Value      string
}

type InspectorRebuild struct {
	Path                    string
	OldPath                 string
	OldType, OldKey         string
	NewType, NewKey         string
	Reason                  string
	TypeChanged, KeyChanged bool
}

type InspectorEventRecord struct {
	Sequence uint64
	RenderID uint64
	Name     string
	Path     string
	Source   string
	Event    Event
	Value    string
}
```

有界历史记录器：

```go
type InspectorHistory struct { /* unexported fields */ }

func NewInspectorHistory(limit int) *InspectorHistory
func (h *InspectorHistory) OnInspectorCommit(commit InspectorCommit)
func (h *InspectorHistory) OnInspectorEvent(event InspectorEventRecord)
func (h *InspectorHistory) Commits() []InspectorCommit
func (h *InspectorHistory) Events() []InspectorEventRecord
```

`limit < 1` 时使用 100。快照与历史读取返回副本；observer 回调运行在 UI 提交或
事件分发路径上，不应阻塞。

## 11. 插件 SDK

错误和属性：

```go
type PluginError struct {
	Name  string
	Stage string
	Err   error
}

func (e *PluginError) Error() string
func (e *PluginError) Unwrap() error

type PluginProperty struct { /* unexported fields */ }
type PluginProperties struct { /* unexported fields */ }

func PluginString(name, value string) PluginProperty
func PluginInt(name string, value int) PluginProperty
func PluginBool(name string, value bool) PluginProperty
func PluginFloat(name string, value float64) PluginProperty
func NewPluginProperties(properties ...PluginProperty) PluginProperties
func (p PluginProperties) Keys() []string
func (p PluginProperties) String(name string) (value string, ok bool)
func (p PluginProperties) Int(name string) (value int, ok bool)
func (p PluginProperties) Bool(name string) (value bool, ok bool)
func (p PluginProperties) Float(name string) (value float64, ok bool)
```

同名属性最后一个值生效，`Keys` 保留名称首次出现的稳定顺序并返回副本。读取不存在
或类型不符的属性时 `ok=false`。

类型安全能力：

```go
type Capability[T any] struct { /* unexported fields */ }
type PluginContext struct { /* unexported fields */ }

func NewCapability[T any](name string) Capability[T]
func (c Capability[T]) Name() string
func LookupCapability[T any](ctx PluginContext, capability Capability[T]) (value T, ok bool)
```

能力名称必须使用点分命名空间。`PluginContext` 是每次插件回调开始时捕获的只读
能力快照，不暴露 Renderer 或原生句柄。

插件上下文与描述符：

```go
type PluginBuildContext struct {
	PluginContext
	Type       string
	Key        string
	Properties PluginProperties
	Children   []Widget
}

type PluginMeasureContext struct {
	PluginContext
	Type        string
	Key         string
	Properties  PluginProperties
	Constraints BoxConstraints
	ChildSize   Size
}

type PluginLayout struct {
	Size        Size
	ChildOffset Point
}

type PluginInstanceContext struct {
	PluginContext
	Type       string
	Key        string
	Properties PluginProperties
}

type WidgetPlugin struct {
	Build     func(PluginBuildContext) (Widget, error)
	Measure   func(PluginMeasureContext) (PluginLayout, error)
	Init      func(PluginContext) error
	Close     func(PluginContext) error
	OnMount   func(PluginInstanceContext) error
	OnUpdate  func(PluginInstanceContext) error
	OnUnmount func(PluginInstanceContext) error
}

func RegisterWidget(name string, descriptor WidgetPlugin) error
func UnregisterWidget(name string) error
func RegisteredWidgets() []string
```

`Build` 必填，只能返回一棵公开 Widget 子树；插件节点自身透明，不创建原生控件。
`Measure` 可选。Init/Close 每个 App 各执行一次，Close 按初始化逆序执行。
`UnregisterWidget` 在仍有 App 使用插件时返回包装了 `ErrPluginInUse` 的
`*PluginError`。

## 12. 批次 3 精确契约

### Slider

```go
func Slider(opts ...Opt) Widget
func Step(value int) Opt
func OnValueChange(fn func(value int)) Opt
```

- 默认值为 `Minimum(0)`、`Maximum(100)`、`Value(0)`、`Step(1)`。
- `Maximum < Minimum` 时 Maximum 收敛到 Minimum，Value 钳制到闭区间。
- Step 必须为正整数；它控制键盘/行步进，不把鼠标拖动值吸附到步长。
- Slider 是显式受控控件。用户操作只调用 `OnValueChange`；回调应更新 State，
  下一次 render 再通过 Value 声明当前值。程序化 Value patch 不触发回调。

### StringGrid

```go
type GridCell = render.GridCell // 有效字段：Row, Column int

func StringGrid(rows, columns int, opts ...Opt) Widget
func Cells(values [][]string) Opt
func Headers(values []string) Opt
func ColumnWidths(values []int) Opt
func SelectedCell(row, column int) Opt
func SelectedRow(row int) Opt
func Editable(value bool) Opt
func OnCellSelect(fn func(cell GridCell)) Opt
func OnCellEdit(fn func(cell GridCell, value string)) Opt
```

- `rows >= 0`，`columns > 0`；非法尺寸会确定性 panic。
- Cells 是严格的 `rows x columns` 矩阵。未传 Cells 时框架创建同尺寸空字符串
  矩阵；显式传入的矩阵尺寸不符会 panic。
- Headers 可为空；非空时长度必须等于 columns。ColumnWidths 可为空，表示每列
  默认 96 DIP；非空时长度必须等于 columns 且每项大于 0。
- 表头不计入逻辑 rows。GridCell 的 Row/Column 从 0 开始；无数据行时默认选择为
  `GridCell{Row: -1, Column: -1}`。
- `SelectedCell` 是单元格选择；`SelectedRow` 是整行选择，但事件仍返回实际焦点列。
- Cells、Headers、ColumnWidths 在公开 API、diff、Mock 和 native 所有权边界均
  防御性复制，其中 Cells 是深复制。
- Editable 默认 false。用户选择和编辑分别触发 OnCellSelect、OnCellEdit；程序化
  patch 不触发回调，编辑回调应更新受控 Cells。
- 默认后端因锁定 LCL 的选择事件桥接限制，以单个主线程 16ms 轮询器补充
  `OnAfterSelection` 并去重；因此真实选择回调最多可能延后一个轮询周期。最后一个
  订阅解除时轮询停止。ColumnWidths 在 DPI 变化后按保存的 DIP 值重施。

### PaintBox

```go
type PaintCommandKind = render.PaintCommandKind
type PaintCommand = render.PaintCommand

// PaintCommand 的有效字段
type PaintCommand struct {
	Kind        PaintCommandKind
	X           int
	Y           int
	Radius      int
	Color       ColorValue
	FillColor   ColorValue
	StrokeColor ColorValue
	StrokeWidth int
}

func PaintBox(commands []PaintCommand, opts ...Opt) Widget
```

- 命令按 slice 顺序执行，输入会被防御性复制；命令是稳定值，不持有回调或后端对象。
- 所有几何值以 PaintBox 客户区左上角为原点并使用 DIP；坐标可为负以允许裁剪。
- PaintClear 使用非零 Color 填满 surface。
- PaintCircle 要求 Radius 大于 0、StrokeWidth 不小于 0，并至少指定非零
  FillColor 或 StrokeColor。指定 StrokeColor 时 StrokeWidth 必须大于 0；未指定
  StrokeColor 时 StrokeWidth 必须为 0。
- 颜色零值表示未指定；其他颜色必须为 `A=0xFF` 的不透明 ARGB。`A=0` 但 RGB
  非零以及 `0 < A < 0xFF` 的值会在构造时拒绝，不能在 LCL `TColor` 边界静默丢 alpha。
- 非法或未知命令会在构造时 panic。命令值变化只 patch 命令并请求重绘，不重建
  原生 PaintBox；值未变化时不重复 invalidate。
- OnMouseDown 等通用鼠标 Opt 可用于应用层命中测试，坐标仍为 DIP。
- DPI 变化会请求重绘，命令几何在下一次 paint 时按新 DPI 换算。

## 13. Accessibility / i18n 与诊断

公开框架诊断 ID 是 [diagnostics.go](../diagnostics.go) 中全部
`Diagnostic... MessageID` 常量。常量名和 `flux.*` 字符串值是候选兼容面；应用应
用 ID 替换显示资源，不应匹配内建中英文文本。

```go
func SetDiagnosticCatalog(catalog *Catalog, locale Locale) (restore func())
func SetDiagnosticLocale(locale Locale) (restore func())
func DiagnosticText(id MessageID, args ...any) string
```

- `SetDiagnosticLocale` 选择内建 `zh-CN` 或 `en`；未知 locale 回落内建 `zh-CN`。
- `SetDiagnosticCatalog` 设置进程级目录；自定义目录缺失的 ID 回落内建目录。
- 两个 setter 返回幂等 restore；若此后已有更新的全局设置，旧 restore 不覆盖它。
- `DiagnosticText` 使用 printf 模板格式化；未知 ID 原样返回其字符串值。
- Catalog 资源和绑定的精确行为见 [Accessibility / i18n 能力表](accessibility-i18n.md)。

默认后端在每个原生父级内按声明树生成连续 TabOrder；透明布局和插件组合不会创建
额外顺序范围，keyed 重排只 patch TabOrder。RadioButton 的 Left/Up/Right/Down 在
同 parent + GroupIndex 内循环选择。高对比度时标准控件使用系统默认色、PaintBox
使用系统窗口/高亮/文字色；`FLUXVCL_FORCE_HIGH_CONTRAST=1` 仅为自动化覆盖。

## 14. 默认原生后端入口

下游应用通过公开的 `github.com/xiaowumin-mark/flux-vcl/native` 包启动默认
energye/lcl 后端，不应导入 `internal/native`：

```go
type Renderer = backend.Renderer

func Init(dllPath string) error
func NewRenderer() *Renderer
func Run()
func (r *Renderer) HighContrast() bool
```

- `Init` 在创建任何 Renderer 前加载与 `energye/lcl v1.0.3` 精确匹配的 DLL。
- `NewRenderer` 创建一个窗口对应的 Renderer；其值可直接传给根包 `NewApp`。
- 主窗口挂载后由 `Run` 显示并进入消息循环；额外窗口可调用 `Renderer.Show`。
- `Renderer` 当前是默认后端实现的类型别名。应用层应优先通过根包 `App`、`State`
  与公开 capability 操作 UI，不依赖别名暴露的后端实现细节。

## 15. 兼容范围

本文档只描述根包与公开 `native` 启动包当前导出的声明。`internal/*` 包、具体
LCL/VCL 对象、未导出的 Props key、Mock 辅助 API 和后端实现细节不属于公开兼容
承诺。`Rect`、`Element` 等根包别名是供应用使用的公开名称；其底层类型身份不能
据此把对应 internal 包当作公共扩展点。

本文档冻结 v0.1.0 候选公开面，但 `0.1.0-dev` 仍不是正式发布版本。正式兼容
承诺在移除 `-dev`、创建版本标签并完成发布检查单后生效。
