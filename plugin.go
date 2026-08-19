package flux

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/xiaowumin-mark/flux-vcl/internal/diff"
	"github.com/xiaowumin-mark/flux-vcl/internal/widget"
)

const pluginTypePrefix = "Plugin:"

var (
	// ErrPluginInvalid 表示插件名称、描述符或属性不符合公开契约。
	ErrPluginInvalid = newDiagnosticError(DiagnosticErrPluginInvalid)
	// ErrPluginReserved 表示插件名称与内建控件或框架保留前缀冲突。
	ErrPluginReserved = newDiagnosticError(DiagnosticErrPluginReserved)
	// ErrPluginAlreadyRegistered 表示进程注册表已有同名插件。
	ErrPluginAlreadyRegistered = newDiagnosticError(DiagnosticErrPluginRegistered)
	// ErrPluginNotRegistered 表示渲染或注销时找不到指定插件。
	ErrPluginNotRegistered = newDiagnosticError(DiagnosticErrPluginNotRegistered)
	// ErrPluginInUse 表示仍有 App 使用插件，不能逻辑注销。
	ErrPluginInUse = newDiagnosticError(DiagnosticErrPluginInUse)
	// ErrPluginPanic 表示框架已捕获插件回调 panic。
	ErrPluginPanic = newDiagnosticError(DiagnosticErrPluginPanic)
	// ErrPluginCycle 表示插件 builder 递归展开超过安全深度。
	ErrPluginCycle = newDiagnosticError(DiagnosticErrPluginCycle)
	// ErrAppClosed 表示 App 已关闭，不能继续渲染。
	ErrAppClosed = newDiagnosticError(DiagnosticErrAppClosed)
)

// PluginError 描述插件错误发生的类型名、阶段与底层原因。
// Stage 为 register、unregister、resolve、init、build、measure、mount、update、
// unmount 或 close。
type PluginError struct {
	Name  string
	Stage string
	Err   error
}

// Error 返回包含插件名称和阶段的错误文本。
func (e *PluginError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return DiagnosticText(DiagnosticPluginError, e.Name, e.Stage, e.Err)
}

// Unwrap 返回底层错误，供 errors.Is/errors.As 使用。
func (e *PluginError) Unwrap() error { return e.Err }

// PluginProperty 是一个经类型化构造器创建的不可变插件属性。
// 请使用 PluginString/PluginInt/PluginBool/PluginFloat 创建，不直接构造。
type PluginProperty struct {
	name  string
	value any
}

// PluginString 创建字符串插件属性。
func PluginString(name, value string) PluginProperty {
	return newPluginProperty(name, value)
}

// PluginInt 创建整数插件属性。
func PluginInt(name string, value int) PluginProperty {
	return newPluginProperty(name, value)
}

// PluginBool 创建布尔插件属性。
func PluginBool(name string, value bool) PluginProperty {
	return newPluginProperty(name, value)
}

// PluginFloat 创建浮点插件属性。
func PluginFloat(name string, value float64) PluginProperty {
	return newPluginProperty(name, value)
}

func newPluginProperty(name string, value any) PluginProperty {
	if !validPluginPropertyName(name) {
		panic(DiagnosticText(DiagnosticPluginPropertyName, name))
	}
	return PluginProperty{name: name, value: value}
}

// PluginProperties 是传给插件 builder、布局与生命周期的不可变属性集。
// 重名属性采用最后一个值，Keys 仍保留首次出现的稳定顺序。
type PluginProperties struct {
	keys   []string
	values map[string]any
}

// NewPluginProperties 创建插件属性集，并防御性复制内部数据。
func NewPluginProperties(properties ...PluginProperty) PluginProperties {
	p := PluginProperties{values: make(map[string]any, len(properties))}
	for _, property := range properties {
		if !validPluginPropertyName(property.name) {
			panic(DiagnosticText(DiagnosticPluginProperties))
		}
		if _, exists := p.values[property.name]; !exists {
			p.keys = append(p.keys, property.name)
		}
		p.values[property.name] = property.value
	}
	return p
}

// Keys 返回属性名的稳定顺序副本。
func (p PluginProperties) Keys() []string { return append([]string(nil), p.keys...) }

// String 读取字符串属性；类型不符或不存在时 ok=false。
func (p PluginProperties) String(name string) (value string, ok bool) {
	value, ok = p.values[name].(string)
	return
}

// Int 读取整数属性；类型不符或不存在时 ok=false。
func (p PluginProperties) Int(name string) (value int, ok bool) {
	value, ok = p.values[name].(int)
	return
}

// Bool 读取布尔属性；类型不符或不存在时 ok=false。
func (p PluginProperties) Bool(name string) (value bool, ok bool) {
	value, ok = p.values[name].(bool)
	return
}

// Float 读取浮点属性；类型不符或不存在时 ok=false。
func (p PluginProperties) Float(name string) (value float64, ok bool) {
	value, ok = p.values[name].(float64)
	return
}

func (p PluginProperties) clone() PluginProperties {
	out := PluginProperties{keys: append([]string(nil), p.keys...), values: make(map[string]any, len(p.values))}
	for key, value := range p.values {
		out.values[key] = value
	}
	return out
}

// Capability 是 Renderer 可选能力的类型安全令牌。
// 插件通常复用框架预定义令牌；自定义后端能力可用 NewCapability 声明。
type Capability[T any] struct{ name string }

// NewCapability 创建具名能力令牌。名称必须采用点分命名空间，如 example.chart.export。
func NewCapability[T any](name string) Capability[T] {
	if !validCapabilityName(name) {
		panic(DiagnosticText(DiagnosticCapabilityName, name))
	}
	return Capability[T]{name: name}
}

// Name 返回能力的稳定名称。
func (c Capability[T]) Name() string { return c.name }

var (
	// RendererDPI 是当前插件回调开始时的 Renderer DPI；值为 int，缺失时插件应按 96 DPI 退化。
	RendererDPI = Capability[int]{name: "flux.renderer.dpi"}
	// RendererBackend 是 Renderer 后端标识的可选能力；当前默认后端返回 "lcl"。
	RendererBackend = Capability[string]{name: "flux.renderer.backend"}
)

// PluginContext 是插件级上下文。它只暴露当前回调开始时捕获的只读、具名
// Renderer 能力快照，不泄露 Renderer、原生句柄或 LCL/VCL 对象（D4/D6）。
// 插件可以保存该值或跨 goroutine 读取；保存的上下文不会继续访问 Renderer，
// 也不会在 App.Close 后获得更新值。
type PluginContext struct{ capabilities map[string]any }

// LookupCapability 从插件上下文读取类型安全的可选能力。
// 能力缺失或后端返回类型不匹配时 ok=false，插件必须安全退化。
func LookupCapability[T any](ctx PluginContext, capability Capability[T]) (value T, ok bool) {
	if ctx.capabilities == nil || capability.name == "" {
		return value, false
	}
	raw, exists := ctx.capabilities[capability.name]
	if !exists {
		return value, false
	}
	value, ok = raw.(T)
	return value, ok
}

// PluginBuildContext 是 WidgetPlugin.Build 的只读输入。
// Children 是 PluginWidget 调用方声明的子节点副本；builder 返回唯一 Widget 子树。
type PluginBuildContext struct {
	PluginContext
	Type       string
	Key        string
	Properties PluginProperties
	Children   []Widget
}

// PluginMeasureContext 是 WidgetPlugin.Measure 的只读输入，所有尺寸均为 DIP。
// ChildSize 是 builder 子树按当前 Constraints 完成布局后的尺寸。
type PluginMeasureContext struct {
	PluginContext
	Type        string
	Key         string
	Properties  PluginProperties
	Constraints BoxConstraints
	ChildSize   Size
}

// PluginLayout 是插件布局回调的输出。Size 会再次经 Constraints.Constrain 钳制；
// ChildOffset 是 builder 子树相对插件布局框左上角的 DIP 偏移。
type PluginLayout struct {
	Size        Size
	ChildOffset Point
}

// PluginInstanceContext 是插件节点 mount/update/unmount 生命周期的只读输入。
type PluginInstanceContext struct {
	PluginContext
	Type       string
	Key        string
	Properties PluginProperties
}

// WidgetPlugin 描述一个进程内组合式插件。
//
// Build 必填，只能返回公开 Widget 子树；插件节点本身是透明 Element，不创建原生
// 控件。Init/Close 按 App 各执行一次，Close 按初始化逆序执行。实例生命周期按
// Element mount/update/unmount 执行。Measure 可选，缺省直接采用 builder 子树尺寸。
type WidgetPlugin struct {
	Build     func(PluginBuildContext) (Widget, error)
	Measure   func(PluginMeasureContext) (PluginLayout, error)
	Init      func(PluginContext) error
	Close     func(PluginContext) error
	OnMount   func(PluginInstanceContext) error
	OnUpdate  func(PluginInstanceContext) error
	OnUnmount func(PluginInstanceContext) error
}

type pluginRegistration struct {
	descriptor WidgetPlugin
	activeApps int
}

type pluginRegistry struct {
	mu      sync.RWMutex
	plugins map[string]*pluginRegistration
}

var widgets = pluginRegistry{plugins: make(map[string]*pluginRegistration)}

type pluginRuntime struct {
	name       string
	descriptor WidgetPlugin
	context    func() PluginContext
	report     func(error)
}

type pluginCapabilityProvider interface {
	// PluginCapabilitySnapshot 在插件回调所在的 UI 执行域内返回只读能力值。
	// 返回值及其元素必须可安全保存和跨 goroutine 读取。
	PluginCapabilitySnapshot() map[string]any
}

// RegisterWidget 把组合式 Widget 插件注册到进程全局注册表。
// 名称区分大小写且进程内唯一；重复、保留名称或缺少 Build 均返回可判定错误。
func RegisterWidget(name string, descriptor WidgetPlugin) error {
	if err := validatePlugin(name, descriptor); err != nil {
		return err
	}
	widgets.mu.Lock()
	defer widgets.mu.Unlock()
	if _, exists := widgets.plugins[name]; exists {
		return &PluginError{Name: name, Stage: "register", Err: ErrPluginAlreadyRegistered}
	}
	widgets.plugins[name] = &pluginRegistration{descriptor: descriptor}
	return nil
}

// UnregisterWidget 从进程注册表逻辑注销插件。
// 仍有 App 已成功初始化该插件时返回 ErrPluginInUse；本 API 不卸载 Go 二进制代码。
func UnregisterWidget(name string) error {
	widgets.mu.Lock()
	defer widgets.mu.Unlock()
	registration, exists := widgets.plugins[name]
	if !exists {
		return &PluginError{Name: name, Stage: "unregister", Err: ErrPluginNotRegistered}
	}
	if registration.activeApps > 0 {
		return &PluginError{Name: name, Stage: "unregister", Err: ErrPluginInUse}
	}
	delete(widgets.plugins, name)
	return nil
}

// RegisteredWidgets 返回当前已注册插件名称的排序副本。
func RegisteredWidgets() []string {
	widgets.mu.RLock()
	names := make([]string, 0, len(widgets.plugins))
	for name := range widgets.plugins {
		names = append(names, name)
	}
	widgets.mu.RUnlock()
	sort.Strings(names)
	return names
}

// PluginWidget 声明一个已注册插件节点。注册解析和 builder 执行发生在 App 渲染前，
// 因此未知类型通过 Mount/Render 返回 PluginError，而不会进入 native Create switch。
// args 可混合 Widget 子节点与 Opt；非法参数 panic。
func PluginWidget(name string, properties PluginProperties, args ...any) Widget {
	if !validPluginName(name) || reservedPluginName(name) {
		panic(DiagnosticText(DiagnosticPluginWidgetName, name))
	}
	n := widgetNodeForPlugin(name)
	n.Props.Set("PluginProperties", properties.clone())
	for _, arg := range args {
		switch value := arg.(type) {
		case Widget:
			n.Add(value.Create())
		case Opt:
			value.apply(n)
		default:
			panic(DiagnosticText(DiagnosticPluginWidgetArguments))
		}
	}
	return widgetNode{n}
}

func widgetNodeForPlugin(name string) *Node {
	return newNode(pluginTypePrefix + name)
}

// newNode 收敛插件节点创建，避免把 internal/widget 包暴露给第三方插件。
func newNode(t string) *Node {
	return widgetNewNode(t)
}

func validatePlugin(name string, descriptor WidgetPlugin) error {
	if !validPluginName(name) {
		return &PluginError{Name: name, Stage: "register", Err: fmt.Errorf("%w: %s", ErrPluginInvalid, DiagnosticText(DiagnosticPluginNameFormat))}
	}
	if reservedPluginName(name) {
		return &PluginError{Name: name, Stage: "register", Err: ErrPluginReserved}
	}
	if descriptor.Build == nil {
		return &PluginError{Name: name, Stage: "register", Err: fmt.Errorf("%w: %s", ErrPluginInvalid, DiagnosticText(DiagnosticPluginBuildRequired))}
	}
	return nil
}

var (
	pluginNamePattern     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)
	pluginPropertyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*$`)
	capabilityPattern     = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)+$`)
)

func validPluginName(name string) bool {
	return len(name) <= 128 && pluginNamePattern.MatchString(name)
}

func validPluginPropertyName(name string) bool {
	return len(name) <= 128 && pluginPropertyPattern.MatchString(name)
}

func validCapabilityName(name string) bool {
	return len(name) <= 160 && capabilityPattern.MatchString(name)
}

func reservedPluginName(name string) bool {
	if strings.HasPrefix(name, pluginTypePrefix) {
		return true
	}
	_, reserved := builtInWidgetTypes[name]
	return reserved
}

var builtInWidgetTypes = map[string]struct{}{
	"Window": {}, "Column": {}, "Row": {}, "Expanded": {}, "Flexible": {},
	"Component": {}, "ScrollBox": {}, "ListView": {}, "ListViewRow": {},
	"Text": {}, "Button": {}, "Input": {}, "Memo": {}, "CheckBox": {},
	"ComboBox": {}, "RadioButton": {}, "ProgressBar": {}, "Slider": {}, "StringGrid": {}, "PaintBox": {},
	"PageControl": {}, "TabPage": {},
}

func acquireWidget(name string) (WidgetPlugin, error) {
	widgets.mu.Lock()
	defer widgets.mu.Unlock()
	registration, exists := widgets.plugins[name]
	if !exists {
		return WidgetPlugin{}, &PluginError{Name: name, Stage: "resolve", Err: ErrPluginNotRegistered}
	}
	registration.activeApps++
	return registration.descriptor, nil
}

func releaseWidget(name string) {
	widgets.mu.Lock()
	if registration := widgets.plugins[name]; registration != nil && registration.activeApps > 0 {
		registration.activeApps--
	}
	widgets.mu.Unlock()
}

func pluginNameFromType(t string) (string, bool) {
	if !strings.HasPrefix(t, pluginTypePrefix) || len(t) == len(pluginTypePrefix) {
		return "", false
	}
	name := strings.TrimPrefix(t, pluginTypePrefix)
	return name, validPluginName(name) && !reservedPluginName(name)
}

func pluginCall(name, stage string, fn func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &PluginError{Name: name, Stage: stage, Err: fmt.Errorf("%w: %v", ErrPluginPanic, recovered)}
		}
	}()
	if callErr := fn(); callErr != nil {
		return &PluginError{Name: name, Stage: stage, Err: callErr}
	}
	return nil
}

func (a *App) pluginContext() PluginContext {
	provider, ok := a.r.(pluginCapabilityProvider)
	if !ok {
		return PluginContext{}
	}
	var source map[string]any
	func() {
		defer func() {
			if recover() != nil {
				source = nil
			}
		}()
		source = provider.PluginCapabilitySnapshot()
	}()
	capabilities := make(map[string]any, len(source))
	for name, value := range source {
		if validCapabilityValue(value) {
			capabilities[name] = value
		}
	}
	return PluginContext{capabilities: capabilities}
}

func validCapabilityValue(value any) bool {
	switch value.(type) {
	case nil, bool, string,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, uintptr,
		float32, float64:
		return true
	default:
		return false
	}
}

func (a *App) ensurePlugin(name string) (*pluginRuntime, bool, error) {
	if runtime, exists := a.plugins[name]; exists {
		return runtime, false, nil
	}
	descriptor, err := acquireWidget(name)
	if err != nil {
		return nil, false, err
	}
	ctx := a.pluginContext()
	if descriptor.Init != nil {
		if err := pluginCall(name, "init", func() error { return descriptor.Init(ctx) }); err != nil {
			releaseWidget(name)
			return nil, false, err
		}
	}
	runtime := &pluginRuntime{name: name, descriptor: descriptor, context: a.pluginContext, report: a.reportError}
	a.plugins[name] = runtime
	a.pluginOrder = append(a.pluginOrder, name)
	return runtime, true, nil
}

func (a *App) expandWidget(source Widget) (*Node, error) {
	if source == nil {
		return nil, fmt.Errorf("%s", DiagnosticText(DiagnosticRootWidgetNil))
	}
	pluginStart := len(a.pluginOrder)
	var sourceNode *Node
	if err := widgetCall(func() error {
		sourceNode = source.Create()
		return nil
	}); err != nil {
		return nil, err
	}
	root, err := a.expandNode(sourceNode, 0)
	if err == nil {
		err = validateAndNormalizePageControls(root)
	}
	if err == nil {
		return root, nil
	}
	errs := []error{err}
	errs = append(errs, a.rollbackPluginsFrom(pluginStart)...)
	return nil, errors.Join(errs...)
}

func widgetCall(fn func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%s", DiagnosticText(DiagnosticWidgetCreatePanic, recovered))
		}
	}()
	return fn()
}

func (a *App) rollbackPluginsFrom(start int) []error {
	if start < 0 {
		start = 0
	}
	if start > len(a.pluginOrder) {
		start = len(a.pluginOrder)
	}
	var errs []error
	for i := len(a.pluginOrder) - 1; i >= start; i-- {
		name := a.pluginOrder[i]
		runtime := a.plugins[name]
		if runtime.descriptor.Close != nil {
			if err := pluginCall(name, "close", func() error { return runtime.descriptor.Close(runtime.context()) }); err != nil {
				errs = append(errs, err)
			}
		}
		delete(a.plugins, name)
		releaseWidget(name)
	}
	a.pluginOrder = a.pluginOrder[:start]
	return errs
}

const maxPluginBuildDepth = 64

func (a *App) expandNode(node *Node, depth int) (*Node, error) {
	if node == nil {
		return nil, fmt.Errorf("%s", DiagnosticText(DiagnosticWidgetCreateNil))
	}
	if depth > maxPluginBuildDepth {
		return nil, &PluginError{Stage: "build", Err: ErrPluginCycle}
	}
	name, isPlugin := pluginNameFromType(node.Type)
	if !isPlugin {
		if _, ok := builtInWidgetTypes[node.Type]; !ok {
			return nil, fmt.Errorf("%s", DiagnosticText(DiagnosticWidgetUnknown, node.Type))
		}
		for i, child := range node.Children {
			expanded, err := a.expandNode(child, depth)
			if err != nil {
				return nil, err
			}
			node.Children[i] = expanded
		}
		return node, nil
	}

	runtime, _, err := a.ensurePlugin(name)
	if err != nil {
		return nil, err
	}
	properties, _ := node.Props.Get("PluginProperties")
	pluginProperties, ok := properties.(PluginProperties)
	if !ok {
		pluginProperties = NewPluginProperties()
	}
	children := make([]Widget, len(node.Children))
	for i, child := range node.Children {
		children[i] = widgetNode{child}
	}
	var built Widget
	ctx := PluginBuildContext{
		PluginContext: runtime.context(),
		Type:          name, Key: node.Key, Properties: pluginProperties.clone(), Children: children,
	}
	if err := pluginCall(name, "build", func() error {
		builtWidget, buildErr := runtime.descriptor.Build(ctx)
		if buildErr != nil {
			return buildErr
		}
		if builtWidget == nil {
			return fmt.Errorf("%w: %s", ErrPluginInvalid, DiagnosticText(DiagnosticPluginBuildNil))
		}
		built = builtWidget
		return nil
	}); err != nil {
		return nil, err
	}
	var builtNode *Node
	if err := pluginCall(name, "build", func() error {
		builtNode = built.Create()
		if builtNode == nil {
			return fmt.Errorf("%w: %s", ErrPluginInvalid, DiagnosticText(DiagnosticWidgetCreateNil))
		}
		return validatePluginNodeTypes(builtNode)
	}); err != nil {
		return nil, err
	}
	expanded, err := a.expandNode(builtNode, depth+1)
	if err != nil {
		return nil, err
	}
	// PageControl semantics depend on the final direct-child shape. Validate only
	// after nested plugins have consumed and expanded their declared Children;
	// keep failures attributed to this builder and inside the prepare rollback.
	if err := pluginCall(name, "build", func() error {
		if pageErr := validateAndNormalizePageControls(expanded); pageErr != nil {
			return fmt.Errorf("%w: %v", ErrPluginInvalid, pageErr)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	node.Children = []*Node{expanded}
	node.Props.Set("_pluginRuntime", runtime)
	widget.MarkPlugin(node)
	return node, nil
}

func validatePluginNodeTypes(root *Node) error {
	visiting := make(map[*Node]bool)
	var validate func(*Node) error
	validate = func(node *Node) error {
		if node == nil {
			return fmt.Errorf("%w: %s", ErrPluginInvalid, DiagnosticText(DiagnosticWidgetCreateNil))
		}
		if node.Props == nil {
			return fmt.Errorf("%w: %s", ErrPluginInvalid, DiagnosticText(DiagnosticPluginNodePropsNil))
		}
		if visiting[node] {
			return fmt.Errorf("%w: %s", ErrPluginInvalid, DiagnosticText(DiagnosticPluginNodeCycle))
		}
		if _, ok := builtInWidgetTypes[node.Type]; !ok {
			if _, isPlugin := pluginNameFromType(node.Type); !isPlugin {
				return fmt.Errorf("%w: %s", ErrPluginInvalid, DiagnosticText(DiagnosticPluginWidgetUnknown, node.Type))
			}
		}
		visiting[node] = true
		defer delete(visiting, node)
		for _, child := range node.Children {
			if err := validate(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := validate(root); err != nil {
		return err
	}
	return nil
}

// validateAndNormalizePageControls enforces the PageControl/TabPage contract
// after plugin expansion, so hand-written Nodes and plugin builders cannot
// bypass the public constructors' validation. Returning the error through
// expandWidget also keeps plugin initialization rollback on the existing path.
func validateAndNormalizePageControls(root *Node) error {
	var validate func(*Node, *Node) error
	validate = func(node, parent *Node) error {
		if node == nil {
			return fmt.Errorf("%s", DiagnosticText(DiagnosticPageTreeNil))
		}
		if node.Type == "TabPage" {
			if parent == nil || parent.Type != "PageControl" {
				return fmt.Errorf("%s", DiagnosticText(DiagnosticTabPageParent))
			}
			if node.Props == nil {
				return fmt.Errorf("%s", DiagnosticText(DiagnosticTabPagePropsNil, node.Key))
			}
			if title, exists := node.Props.Get("Text"); !exists {
				return fmt.Errorf("%s", DiagnosticText(DiagnosticTabPageTitleMissing, node.Key))
			} else if _, ok := title.(string); !ok {
				return fmt.Errorf("%s", DiagnosticText(DiagnosticTabPageTitleType, node.Key))
			}
			if node.Key == "" {
				return fmt.Errorf("%s", DiagnosticText(DiagnosticExpandedPageKey))
			}
			if len(node.Children) != 1 || node.Children[0] == nil {
				return fmt.Errorf("%s", DiagnosticText(DiagnosticExpandedPageContent, node.Key))
			}
		}
		if node.Type == "PageControl" {
			if node.Props == nil {
				return fmt.Errorf("%s", DiagnosticText(DiagnosticPagePropsNil))
			}
			keys := make(map[string]struct{}, len(node.Children))
			for _, page := range node.Children {
				if page == nil || page.Type != "TabPage" {
					return fmt.Errorf("%s", DiagnosticText(DiagnosticExpandedPageChild))
				}
				if page.Key == "" {
					return fmt.Errorf("%s", DiagnosticText(DiagnosticExpandedPageKey))
				}
				if _, exists := keys[page.Key]; exists {
					return fmt.Errorf("%s", DiagnosticText(DiagnosticExpandedPageKeyDuplicate, page.Key))
				}
				keys[page.Key] = struct{}{}
				if len(page.Children) != 1 || page.Children[0] == nil {
					return fmt.Errorf("%s", DiagnosticText(DiagnosticExpandedPageContent, page.Key))
				}
			}

			selected := 0
			if value, exists := node.Props.Get("SelectedIndex"); exists {
				var ok bool
				selected, ok = value.(int)
				if !ok {
					return fmt.Errorf("%s", DiagnosticText(DiagnosticPageSelectedIndexType))
				}
			}
			node.Props.Set("SelectedIndex", normalizePageSelectedIndex(len(node.Children), selected))
		}
		for _, child := range node.Children {
			if err := validate(child, node); err != nil {
				return err
			}
		}
		return nil
	}
	return validate(root, nil)
}

func pluginPropertiesFromProps(props *widget.Props) PluginProperties {
	if props != nil {
		if value, ok := props.Get("PluginProperties"); ok {
			if properties, ok := value.(PluginProperties); ok {
				return properties.clone()
			}
		}
	}
	return NewPluginProperties()
}

func (runtime *pluginRuntime) instanceContext(key string, props *widget.Props) PluginInstanceContext {
	return PluginInstanceContext{
		PluginContext: runtime.context(),
		Type:          runtime.name, Key: key, Properties: pluginPropertiesFromProps(props),
	}
}

func (runtime *pluginRuntime) PluginMount(key string, props *widget.Props) {
	if runtime.descriptor.OnMount == nil {
		return
	}
	err := pluginCall(runtime.name, "mount", func() error {
		return runtime.descriptor.OnMount(runtime.instanceContext(key, props))
	})
	if err != nil {
		runtime.report(err)
	}
}

func (runtime *pluginRuntime) PluginUpdate(key string, props *widget.Props) {
	if runtime.descriptor.OnUpdate == nil {
		return
	}
	err := pluginCall(runtime.name, "update", func() error {
		return runtime.descriptor.OnUpdate(runtime.instanceContext(key, props))
	})
	if err != nil {
		runtime.report(err)
	}
}

func (runtime *pluginRuntime) PluginUnmount(key string, props *widget.Props) {
	if runtime.descriptor.OnUnmount == nil {
		return
	}
	err := pluginCall(runtime.name, "unmount", func() error {
		return runtime.descriptor.OnUnmount(runtime.instanceContext(key, props))
	})
	if err != nil {
		runtime.report(err)
	}
}

var _ diff.PluginLifecycle = (*pluginRuntime)(nil)
