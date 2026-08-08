package widget

// Props 是控件属性集合（有序，diff 稳定）。
//
// 顺序与插入一致：diff 引擎按 Keys() 顺序比较，属性集相同的两棵树得到
// 相同顺序的 mutation —— 保证输出确定、可断言（D2）。
type Props struct {
	keys []string
	m    map[string]any
}

// NewProps 创建空属性集。
func NewProps() *Props { return &Props{m: map[string]any{}} }

// Set 设置属性。同一 key 重复设置不改变顺序。
func (p *Props) Set(key string, v any) {
	if _, ok := p.m[key]; !ok {
		p.keys = append(p.keys, key)
	}
	p.m[key] = v
}

// Get 读取属性。
func (p *Props) Get(key string) (any, bool) {
	v, ok := p.m[key]
	return v, ok
}

// String 读取字符串属性，不存在或类型不符时返回零值。
func (p *Props) String(key string) string { v, _ := p.m[key].(string); return v }

// Int 读取整数属性，不存在或类型不符时返回零值。
func (p *Props) Int(key string) int { v, _ := p.m[key].(int); return v }

// Bool 读取布尔属性，不存在或类型不符时返回 false。
func (p *Props) Bool(key string) bool { v, _ := p.m[key].(bool); return v }

// Keys 返回按插入顺序的属性名副本。
func (p *Props) Keys() []string { return append([]string(nil), p.keys...) }

// Len 返回属性个数。
func (p *Props) Len() int { return len(p.m) }

// Diff 返回 p 相对 o 需要 patch 的属性名：p 有而 o 无、或值不同。
// 函数值（事件回调、逃逸口）无法比较相等性，恒判定为需要 patch
// —— 每次 render 重新绑定（React 同款行为）。
func (p *Props) Diff(o *Props) []string {
	var out []string
	for _, k := range p.keys {
		nv, _ := p.Get(k)
		ov, ok := o.Get(k)
		if !ok || !ValuesEqual(ov, nv) {
			out = append(out, k)
		}
	}
	return out
}

// Equal 报告两个属性集是否完全一致（含顺序）。
func (p *Props) Equal(o *Props) bool {
	if p == nil || o == nil {
		return p == o
	}
	if len(p.m) != len(o.m) {
		return false
	}
	return len(p.Diff(o)) == 0
}
