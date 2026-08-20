package flux

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Stable message IDs used by framework validation and diagnostic errors.
const (
	DiagnosticCatalogNilLocale         MessageID = "flux.catalog.nil_locale"
	DiagnosticCatalogNil               MessageID = "flux.catalog.nil"
	DiagnosticCatalogFallbackEmpty     MessageID = "flux.catalog.fallback_empty"
	DiagnosticCatalogResourcesEmpty    MessageID = "flux.catalog.resources_empty"
	DiagnosticCatalogFallbackMissing   MessageID = "flux.catalog.fallback_missing"
	DiagnosticCatalogLocaleEmpty       MessageID = "flux.catalog.locale_empty"
	DiagnosticCatalogMessageIDEmpty    MessageID = "flux.catalog.message_id_empty"
	DiagnosticWindowArguments          MessageID = "flux.window.arguments"
	DiagnosticPageChild                MessageID = "flux.page.child"
	DiagnosticPageContent              MessageID = "flux.page.content"
	DiagnosticPageKey                  MessageID = "flux.page.key"
	DiagnosticPageKeyUnique            MessageID = "flux.page.key_unique"
	DiagnosticPageArguments            MessageID = "flux.page.arguments"
	DiagnosticTabChildNil              MessageID = "flux.tab.child_nil"
	DiagnosticTabChildCreateNil        MessageID = "flux.tab.child_create_nil"
	DiagnosticTabKey                   MessageID = "flux.tab.key"
	DiagnosticFlexPositive             MessageID = "flux.flex.positive"
	DiagnosticContainerArguments       MessageID = "flux.container.arguments"
	DiagnosticTextArgument             MessageID = "flux.text.argument"
	DiagnosticSliderStepPositive       MessageID = "flux.slider.step_positive"
	DiagnosticStepValuePositive        MessageID = "flux.step.value_positive"
	DiagnosticGridRows                 MessageID = "flux.grid.rows"
	DiagnosticGridColumns              MessageID = "flux.grid.columns"
	DiagnosticGridCellsType            MessageID = "flux.grid.cells_type"
	DiagnosticGridCellsInvalid         MessageID = "flux.grid.cells_invalid"
	DiagnosticGridCellsRowCount        MessageID = "flux.grid.cells_row_count"
	DiagnosticGridCellsColumnCount     MessageID = "flux.grid.cells_column_count"
	DiagnosticGridHeadersLength        MessageID = "flux.grid.headers_length"
	DiagnosticGridWidthsLength         MessageID = "flux.grid.widths_length"
	DiagnosticGridWidthsPositive       MessageID = "flux.grid.widths_positive"
	DiagnosticGridSelection            MessageID = "flux.grid.selection"
	DiagnosticCellsRectangular         MessageID = "flux.cells.rectangular"
	DiagnosticColumnWidthsPositive     MessageID = "flux.column_widths.positive"
	DiagnosticPaintCommands            MessageID = "flux.paint.commands"
	DiagnosticPaintClearColor          MessageID = "flux.paint.clear_color"
	DiagnosticPaintCircleRadius        MessageID = "flux.paint.circle_radius"
	DiagnosticPaintStrokeWidthNegative MessageID = "flux.paint.stroke_width_negative"
	DiagnosticPaintCirclePaint         MessageID = "flux.paint.circle_paint"
	DiagnosticPaintStrokeWidthRequired MessageID = "flux.paint.stroke_width_required"
	DiagnosticPaintStrokeColorRequired MessageID = "flux.paint.stroke_color_required"
	DiagnosticPaintPartialAlpha        MessageID = "flux.paint.partial_alpha"
	DiagnosticPaintUnknownKind         MessageID = "flux.paint.unknown_kind"
	DiagnosticListBounded              MessageID = "flux.list.bounded"
	DiagnosticCloseUIThread            MessageID = "flux.app.close_ui_thread"
	DiagnosticPluginError              MessageID = "flux.plugin.error"
	DiagnosticPluginPropertyName       MessageID = "flux.plugin.property_name"
	DiagnosticPluginProperties         MessageID = "flux.plugin.properties"
	DiagnosticCapabilityName           MessageID = "flux.plugin.capability_name"
	DiagnosticPluginWidgetName         MessageID = "flux.plugin.widget_name"
	DiagnosticPluginWidgetArguments    MessageID = "flux.plugin.widget_arguments"
	DiagnosticPluginNameFormat         MessageID = "flux.plugin.name_format"
	DiagnosticPluginBuildRequired      MessageID = "flux.plugin.build_required"
	DiagnosticRootWidgetNil            MessageID = "flux.widget.root_nil"
	DiagnosticWidgetCreatePanic        MessageID = "flux.widget.create_panic"
	DiagnosticWidgetCreateNil          MessageID = "flux.widget.create_nil"
	DiagnosticWidgetUnknown            MessageID = "flux.widget.unknown"
	DiagnosticPluginBuildNil           MessageID = "flux.plugin.build_nil"
	DiagnosticPluginNodePropsNil       MessageID = "flux.plugin.node_props_nil"
	DiagnosticPluginNodeCycle          MessageID = "flux.plugin.node_cycle"
	DiagnosticPluginWidgetUnknown      MessageID = "flux.plugin.widget_unknown"
	DiagnosticPageTreeNil              MessageID = "flux.page.tree_nil"
	DiagnosticTabPageParent            MessageID = "flux.page.parent"
	DiagnosticTabPagePropsNil          MessageID = "flux.page.props_nil"
	DiagnosticTabPageTitleMissing      MessageID = "flux.page.title_missing"
	DiagnosticTabPageTitleType         MessageID = "flux.page.title_type"
	DiagnosticExpandedPageKey          MessageID = "flux.page.expanded_key"
	DiagnosticExpandedPageContent      MessageID = "flux.page.expanded_content"
	DiagnosticPagePropsNil             MessageID = "flux.page.page_props_nil"
	DiagnosticExpandedPageChild        MessageID = "flux.page.expanded_child"
	DiagnosticExpandedPageKeyDuplicate MessageID = "flux.page.key_duplicate"
	DiagnosticPageSelectedIndexType    MessageID = "flux.page.selected_index_type"
	DiagnosticPluginMeasureSingleChild MessageID = "flux.plugin.measure_single_child"
	DiagnosticInspectorTargetNil       MessageID = "flux.inspector.target_nil"
	DiagnosticErrInvalidCatalog        MessageID = "flux.error.invalid_catalog"
	DiagnosticErrAppCloseDuringRender  MessageID = "flux.error.app_close_during_render"
	DiagnosticErrPluginInvalid         MessageID = "flux.error.plugin_invalid"
	DiagnosticErrPluginReserved        MessageID = "flux.error.plugin_reserved"
	DiagnosticErrPluginRegistered      MessageID = "flux.error.plugin_registered"
	DiagnosticErrPluginNotRegistered   MessageID = "flux.error.plugin_not_registered"
	DiagnosticErrPluginInUse           MessageID = "flux.error.plugin_in_use"
	DiagnosticErrPluginPanic           MessageID = "flux.error.plugin_panic"
	DiagnosticErrPluginCycle           MessageID = "flux.error.plugin_cycle"
	DiagnosticErrAppClosed             MessageID = "flux.error.app_closed"
)

var builtinDiagnosticCatalog = &Catalog{
	fallback: "zh-CN",
	resources: Resources{
		"zh-CN": {
			DiagnosticCatalogNilLocale:         "flux: Catalog.Bind locale state 不能为空",
			DiagnosticCatalogNil:               "flux: diagnostic Catalog 不能为空",
			DiagnosticCatalogFallbackEmpty:     "fallback locale 不能为空",
			DiagnosticCatalogResourcesEmpty:    "resources 不能为空",
			DiagnosticCatalogFallbackMissing:   "fallback locale %q 未注册",
			DiagnosticCatalogLocaleEmpty:       "locale 不能为空",
			DiagnosticCatalogMessageIDEmpty:    "locale %q 包含空消息 ID",
			DiagnosticWindowArguments:          "flux.Window: 参数必须是 Widget 或 Opt",
			DiagnosticPageChild:                "flux.PageControl: 子节点必须是 TabPage",
			DiagnosticPageContent:              "flux.PageControl: TabPage 必须包含唯一非空子树",
			DiagnosticPageKey:                  "flux.PageControl: TabPage 必须设置非空 Key",
			DiagnosticPageKeyUnique:            "flux.PageControl: TabPage Key 必须唯一",
			DiagnosticPageArguments:            "flux.PageControl: 参数必须是 TabPage 或 Opt",
			DiagnosticTabChildNil:              "flux.TabPage: child 不能为空",
			DiagnosticTabChildCreateNil:        "flux.TabPage: child.Create() 不能返回 nil",
			DiagnosticTabKey:                   "flux.TabPage: 必须设置非空 Key",
			DiagnosticFlexPositive:             "flux: flex 因子必须 > 0",
			DiagnosticContainerArguments:       "flux.%s: 参数必须是 Widget 或 Opt",
			DiagnosticTextArgument:             "flux: 文本参数必须是 string、Bind(...) 或 Catalog.Bind(...)",
			DiagnosticSliderStepPositive:       "flux.Slider: Step 必须 > 0",
			DiagnosticStepValuePositive:        "flux.Step: value 必须 > 0",
			DiagnosticGridRows:                 "flux.StringGrid: rows 必须 >= 0",
			DiagnosticGridColumns:              "flux.StringGrid: columns 必须 > 0",
			DiagnosticGridCellsType:            "flux.StringGrid: Cells 类型无效",
			DiagnosticGridCellsInvalid:         "flux.StringGrid: %v",
			DiagnosticGridCellsRowCount:        "flux.StringGrid: Cells 行数=%d，期望 %d",
			DiagnosticGridCellsColumnCount:     "flux.StringGrid: Cells 第 %d 行列数=%d，期望 %d",
			DiagnosticGridHeadersLength:        "flux.StringGrid: Headers 长度必须为 0 或 %d",
			DiagnosticGridWidthsLength:         "flux.StringGrid: ColumnWidths 长度必须为 0 或 %d",
			DiagnosticGridWidthsPositive:       "flux.StringGrid: ColumnWidths 必须全部 > 0",
			DiagnosticGridSelection:            "flux.StringGrid: 选择坐标超出逻辑行列范围",
			DiagnosticCellsRectangular:         "flux.Cells: 矩阵必须为严格矩形",
			DiagnosticColumnWidthsPositive:     "flux.ColumnWidths: 所有宽度必须 > 0",
			DiagnosticPaintCommands:            "flux.PaintBox: %v",
			DiagnosticPaintClearColor:          "flux.PaintBox: 第 %d 条命令的清屏颜色必须非零",
			DiagnosticPaintCircleRadius:        "flux.PaintBox: 第 %d 条圆形命令的半径必须 > 0",
			DiagnosticPaintStrokeWidthNegative: "flux.PaintBox: 第 %d 条圆形命令的描边宽度必须 >= 0",
			DiagnosticPaintCirclePaint:         "flux.PaintBox: 第 %d 条圆形命令必须包含填充或描边颜色",
			DiagnosticPaintStrokeWidthRequired: "flux.PaintBox: 第 %d 条圆形命令设置描边色时描边宽度必须 > 0",
			DiagnosticPaintStrokeColorRequired: "flux.PaintBox: 第 %d 条圆形命令设置描边宽度时必须设置描边色",
			DiagnosticPaintPartialAlpha:        "flux.PaintBox: 第 %d 条命令不支持半透明颜色（首版仅允许零值或不透明色）",
			DiagnosticPaintUnknownKind:         "flux.PaintBox: 第 %d 条命令类型 %d 未知",
			DiagnosticListBounded:              "flux.ListView: 需要有界的宽高约束（虚拟列表必须有 viewport，请放在 Expanded/固定尺寸容器内）",
			DiagnosticCloseUIThread:            "flux: Renderer 未执行 App.Close 的 UI 线程任务",
			DiagnosticPluginError:              "flux: 插件 %q 在 %s 阶段失败: %v",
			DiagnosticPluginPropertyName:       "flux.PluginProperty: 属性名 %q 无效",
			DiagnosticPluginProperties:         "flux.NewPluginProperties: 属性必须由 PluginString/PluginInt/PluginBool/PluginFloat 创建",
			DiagnosticCapabilityName:           "flux.NewCapability: 能力名称 %q 无效",
			DiagnosticPluginWidgetName:         "flux.PluginWidget: 插件名称 %q 无效或已保留",
			DiagnosticPluginWidgetArguments:    "flux.PluginWidget: 参数必须是 Widget 或 Opt",
			DiagnosticPluginNameFormat:         "名称格式错误",
			DiagnosticPluginBuildRequired:      "Build 不能为空",
			DiagnosticRootWidgetNil:            "flux: 根 Widget 不能为空",
			DiagnosticWidgetCreatePanic:        "flux: Widget.Create panic: %v",
			DiagnosticWidgetCreateNil:          "flux: Widget.Create 返回 nil Node",
			DiagnosticWidgetUnknown:            "flux: 未知 Widget 类型 %q",
			DiagnosticPluginBuildNil:           "Build 返回 nil Widget",
			DiagnosticPluginNodePropsNil:       "builder 返回 nil Props",
			DiagnosticPluginNodeCycle:          "builder 返回循环 Node 树",
			DiagnosticPluginWidgetUnknown:      "builder 返回未知 Widget 类型 %q",
			DiagnosticPageTreeNil:              "flux: 分页树包含 nil Node",
			DiagnosticTabPageParent:            "flux: TabPage 只能直接属于 PageControl",
			DiagnosticTabPagePropsNil:          "flux: TabPage %q Props 不能为空",
			DiagnosticTabPageTitleMissing:      "flux: TabPage %q 必须声明字符串标题",
			DiagnosticTabPageTitleType:         "flux: TabPage %q 标题必须是 string",
			DiagnosticExpandedPageKey:          "flux: PageControl 的 TabPage 必须设置非空 Key",
			DiagnosticExpandedPageContent:      "flux: PageControl 的 TabPage %q 必须包含唯一非空子树",
			DiagnosticPagePropsNil:             "flux: PageControl Props 不能为空",
			DiagnosticExpandedPageChild:        "flux: PageControl 子节点必须是 TabPage",
			DiagnosticExpandedPageKeyDuplicate: "flux: PageControl 的 TabPage Key %q 重复",
			DiagnosticPageSelectedIndexType:    "flux: PageControl SelectedIndex 必须是 int",
			DiagnosticPluginMeasureSingleChild: "builder 必须产生唯一子树",
			DiagnosticInspectorTargetNil:       "inspector.Open: target 不能为空",
			DiagnosticErrInvalidCatalog:        "flux: i18n 目录无效",
			DiagnosticErrAppCloseDuringRender:  "flux: 不能在 render 或生命周期回调中调用 App.Close",
			DiagnosticErrPluginInvalid:         "flux: 插件定义无效",
			DiagnosticErrPluginReserved:        "flux: 插件名称已保留",
			DiagnosticErrPluginRegistered:      "flux: 插件已注册",
			DiagnosticErrPluginNotRegistered:   "flux: 插件未注册",
			DiagnosticErrPluginInUse:           "flux: 插件仍在使用",
			DiagnosticErrPluginPanic:           "flux: 插件回调 panic",
			DiagnosticErrPluginCycle:           "flux: 插件 builder 循环",
			DiagnosticErrAppClosed:             "flux: App 已关闭",
		},
		"en": {
			DiagnosticCatalogNilLocale:         "flux: Catalog.Bind locale state must not be nil",
			DiagnosticCatalogNil:               "flux: diagnostic Catalog must not be nil",
			DiagnosticCatalogFallbackEmpty:     "fallback locale is empty",
			DiagnosticCatalogResourcesEmpty:    "resources are empty",
			DiagnosticCatalogFallbackMissing:   "fallback locale %q is not registered",
			DiagnosticCatalogLocaleEmpty:       "locale is empty",
			DiagnosticCatalogMessageIDEmpty:    "locale %q contains an empty message ID",
			DiagnosticWindowArguments:          "flux.Window: arguments must be Widget or Opt",
			DiagnosticPageChild:                "flux.PageControl: children must be TabPage",
			DiagnosticPageContent:              "flux.PageControl: TabPage must contain one non-nil subtree",
			DiagnosticPageKey:                  "flux.PageControl: TabPage requires a non-empty Key",
			DiagnosticPageKeyUnique:            "flux.PageControl: TabPage Key must be unique",
			DiagnosticPageArguments:            "flux.PageControl: arguments must be TabPage or Opt",
			DiagnosticTabChildNil:              "flux.TabPage: child must not be nil",
			DiagnosticTabChildCreateNil:        "flux.TabPage: child.Create() returned nil",
			DiagnosticTabKey:                   "flux.TabPage: a non-empty Key is required",
			DiagnosticFlexPositive:             "flux: flex factor must be > 0",
			DiagnosticContainerArguments:       "flux.%s: arguments must be Widget or Opt",
			DiagnosticTextArgument:             "flux: text must be string, Bind(...), or Catalog.Bind(...)",
			DiagnosticSliderStepPositive:       "flux.Slider: Step must be > 0",
			DiagnosticStepValuePositive:        "flux.Step: value must be > 0",
			DiagnosticGridRows:                 "flux.StringGrid: rows must be >= 0",
			DiagnosticGridColumns:              "flux.StringGrid: columns must be > 0",
			DiagnosticGridCellsType:            "flux.StringGrid: Cells has an invalid type",
			DiagnosticGridCellsInvalid:         "flux.StringGrid: %v",
			DiagnosticGridCellsRowCount:        "flux.StringGrid: Cells has %d rows; expected %d",
			DiagnosticGridCellsColumnCount:     "flux.StringGrid: Cells row %d has %d columns; expected %d",
			DiagnosticGridHeadersLength:        "flux.StringGrid: Headers length must be 0 or %d",
			DiagnosticGridWidthsLength:         "flux.StringGrid: ColumnWidths length must be 0 or %d",
			DiagnosticGridWidthsPositive:       "flux.StringGrid: every ColumnWidth must be > 0",
			DiagnosticGridSelection:            "flux.StringGrid: selection is outside the logical grid",
			DiagnosticCellsRectangular:         "flux.Cells: matrix must be strictly rectangular",
			DiagnosticColumnWidthsPositive:     "flux.ColumnWidths: every width must be > 0",
			DiagnosticPaintCommands:            "flux.PaintBox: %v",
			DiagnosticPaintClearColor:          "flux.PaintBox: command %d clear color must be non-zero",
			DiagnosticPaintCircleRadius:        "flux.PaintBox: command %d circle radius must be > 0",
			DiagnosticPaintStrokeWidthNegative: "flux.PaintBox: command %d circle stroke width must be >= 0",
			DiagnosticPaintCirclePaint:         "flux.PaintBox: command %d circle needs a fill or stroke color",
			DiagnosticPaintStrokeWidthRequired: "flux.PaintBox: command %d circle stroke width must be > 0 when stroke is set",
			DiagnosticPaintStrokeColorRequired: "flux.PaintBox: command %d circle stroke width requires a stroke color",
			DiagnosticPaintPartialAlpha:        "flux.PaintBox: command %d does not support partial alpha (use zero or an opaque color)",
			DiagnosticPaintUnknownKind:         "flux.PaintBox: command %d has unknown kind %d",
			DiagnosticListBounded:              "flux.ListView: bounded width and height are required (use Expanded or a fixed-size container)",
			DiagnosticCloseUIThread:            "flux: Renderer did not execute the App.Close UI-thread task",
			DiagnosticPluginError:              "flux: plugin %q failed during %s: %v",
			DiagnosticPluginPropertyName:       "flux.PluginProperty: property name %q is invalid",
			DiagnosticPluginProperties:         "flux.NewPluginProperties: properties must be created by PluginString/PluginInt/PluginBool/PluginFloat",
			DiagnosticCapabilityName:           "flux.NewCapability: capability name %q is invalid",
			DiagnosticPluginWidgetName:         "flux.PluginWidget: plugin name %q is invalid or reserved",
			DiagnosticPluginWidgetArguments:    "flux.PluginWidget: arguments must be Widget or Opt",
			DiagnosticPluginNameFormat:         "invalid name format",
			DiagnosticPluginBuildRequired:      "Build must not be nil",
			DiagnosticRootWidgetNil:            "flux: root Widget must not be nil",
			DiagnosticWidgetCreatePanic:        "flux: Widget.Create panic: %v",
			DiagnosticWidgetCreateNil:          "flux: Widget.Create returned a nil Node",
			DiagnosticWidgetUnknown:            "flux: unknown Widget type %q",
			DiagnosticPluginBuildNil:           "Build returned a nil Widget",
			DiagnosticPluginNodePropsNil:       "builder returned nil Props",
			DiagnosticPluginNodeCycle:          "builder returned a cyclic Node tree",
			DiagnosticPluginWidgetUnknown:      "builder returned unknown Widget type %q",
			DiagnosticPageTreeNil:              "flux: page tree contains a nil Node",
			DiagnosticTabPageParent:            "flux: TabPage must be a direct child of PageControl",
			DiagnosticTabPagePropsNil:          "flux: TabPage %q has nil Props",
			DiagnosticTabPageTitleMissing:      "flux: TabPage %q must declare a string title",
			DiagnosticTabPageTitleType:         "flux: TabPage %q title must be a string",
			DiagnosticExpandedPageKey:          "flux: PageControl TabPage requires a non-empty Key",
			DiagnosticExpandedPageContent:      "flux: PageControl TabPage %q must contain one non-nil subtree",
			DiagnosticPagePropsNil:             "flux: PageControl has nil Props",
			DiagnosticExpandedPageChild:        "flux: PageControl children must be TabPage",
			DiagnosticExpandedPageKeyDuplicate: "flux: PageControl TabPage Key %q is duplicated",
			DiagnosticPageSelectedIndexType:    "flux: PageControl SelectedIndex must be an int",
			DiagnosticPluginMeasureSingleChild: "builder must produce exactly one subtree",
			DiagnosticInspectorTargetNil:       "inspector.Open: target must not be nil",
			DiagnosticErrInvalidCatalog:        "flux: invalid i18n catalog",
			DiagnosticErrAppCloseDuringRender:  "flux: App.Close cannot be called during render or a lifecycle callback",
			DiagnosticErrPluginInvalid:         "flux: invalid plugin definition",
			DiagnosticErrPluginReserved:        "flux: plugin name is reserved",
			DiagnosticErrPluginRegistered:      "flux: plugin is already registered",
			DiagnosticErrPluginNotRegistered:   "flux: plugin is not registered",
			DiagnosticErrPluginInUse:           "flux: plugin is still in use",
			DiagnosticErrPluginPanic:           "flux: plugin callback panic",
			DiagnosticErrPluginCycle:           "flux: plugin builder cycle",
			DiagnosticErrAppClosed:             "flux: App is closed",
		},
	},
}

type diagnosticConfig struct {
	catalog *Catalog
	locale  Locale
}

var currentDiagnostics atomic.Pointer[diagnosticConfig]

// diagnosticError keeps errors.Is identity stable while resolving its display
// text from the active diagnostic catalog at the point Error is called.
type diagnosticError struct{ id MessageID }

func (e *diagnosticError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return DiagnosticText(e.id)
}

func newDiagnosticError(id MessageID) error { return &diagnosticError{id: id} }

func init() {
	currentDiagnostics.Store(&diagnosticConfig{catalog: builtinDiagnosticCatalog, locale: "zh-CN"})
}

// SetDiagnosticCatalog replaces process-wide framework diagnostic resources.
// Missing IDs fall back to FluxVCL's built-in catalog. The returned restore
// function is idempotent and will not overwrite a newer concurrent setting.
func SetDiagnosticCatalog(catalog *Catalog, locale Locale) (restore func()) {
	if catalog == nil {
		panic(DiagnosticText(DiagnosticCatalogNil))
	}
	next := &diagnosticConfig{catalog: catalog, locale: locale}
	previous := currentDiagnostics.Swap(next)
	var once sync.Once
	return func() {
		once.Do(func() { currentDiagnostics.CompareAndSwap(next, previous) })
	}
}

// SetDiagnosticLocale selects one of the built-in diagnostic locales. Unknown
// locales use the built-in zh-CN fallback.
func SetDiagnosticLocale(locale Locale) (restore func()) {
	return SetDiagnosticCatalog(builtinDiagnosticCatalog, locale)
}

// DiagnosticText formats the active framework diagnostic resource for id.
func DiagnosticText(id MessageID, args ...any) string {
	config := currentDiagnostics.Load()
	if config != nil && config.catalog != nil {
		if message, ok := config.catalog.Lookup(config.locale, id); ok {
			return formatDiagnostic(message, args...)
		}
		if message, ok := builtinDiagnosticCatalog.Lookup(config.locale, id); ok {
			return formatDiagnostic(message, args...)
		}
	}
	if message, ok := builtinDiagnosticCatalog.Lookup("zh-CN", id); ok {
		return formatDiagnostic(message, args...)
	}
	return string(id)
}

func formatDiagnostic(message string, args ...any) string {
	if len(args) == 0 {
		return message
	}
	return fmt.Sprintf(message, args...)
}
