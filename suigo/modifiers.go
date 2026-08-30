package suigo

// FontStyle は SwiftUI の標準テキストスタイル。docs/wire-format.md §4.1。
type FontStyle string

const (
	LargeTitle  FontStyle = "largeTitle"
	Title       FontStyle = "title"
	Title2      FontStyle = "title2"
	Title3      FontStyle = "title3"
	Headline    FontStyle = "headline"
	Subheadline FontStyle = "subheadline"
	Body        FontStyle = "body"
	Callout     FontStyle = "callout"
	Footnote    FontStyle = "footnote"
	Caption     FontStyle = "caption"
	Caption2    FontStyle = "caption2"
)

// Color は名前つきの色、または "#RRGGBB"。docs/wire-format.md §4.2。
type Color string

const (
	Primary   Color = "primary"
	Secondary Color = "secondary"
	Red       Color = "red"
	Orange    Color = "orange"
	Yellow    Color = "yellow"
	Green     Color = "green"
	Mint      Color = "mint"
	Teal      Color = "teal"
	Cyan      Color = "cyan"
	Blue      Color = "blue"
	Indigo    Color = "indigo"
	Purple    Color = "purple"
	Pink      Color = "pink"
	Brown     Color = "brown"
	Gray      Color = "gray"
	Black     Color = "black"
	White     Color = "white"
	Clear     Color = "clear"
)

// Hex は "#RRGGBB" 形式の色を作る。
func Hex(rrggbb string) Color { return Color(rrggbb) }

type Alignment string

const (
	AlignLeading        Alignment = "leading"
	AlignCenter         Alignment = "center"
	AlignTrailing       Alignment = "trailing"
	AlignTop            Alignment = "top"
	AlignBottom         Alignment = "bottom"
	AlignTopLeading     Alignment = "topLeading"
	AlignTopTrailing    Alignment = "topTrailing"
	AlignBottomLeading  Alignment = "bottomLeading"
	AlignBottomTrailing Alignment = "bottomTrailing"
)

// EdgeInsets は辺ごとの余白。ゼロの辺は省略される。
type EdgeInsets struct {
	Top, Bottom, Leading, Trailing float64
}

// Infinity は Frame の MaxWidth / MaxHeight に渡すと SwiftUI の .infinity になる。
const Infinity = -1

// FrameOptions はゼロ値のフィールドを省略する。Infinity は "infinity" として出力される。
type FrameOptions struct {
	Width, Height, MaxWidth, MaxHeight float64
	Alignment                          Alignment
}

// CommitPolicy は TextField がいつ Go へ値を送るかを決める。
type CommitPolicy string

const (
	CommitOnSubmit CommitPolicy = "submit"
	CommitOnBlur   CommitPolicy = "blur"
)

// CommitDebounce は入力が止まってから ms ミリ秒後に確定する。
func CommitDebounce(ms int) CommitPolicy {
	return CommitPolicy("debounce:" + itoa(ms))
}

func (n Node) Padding(v float64) Node            { return n.withMod("padding", v) }
func (n Node) PaddingEdges(e EdgeInsets) Node    { return n.withMod("padding", e.toMap()) }
func (n Node) Font(f FontStyle) Node             { return n.withMod("font", string(f)) }
func (n Node) ForegroundColor(c Color) Node      { return n.withMod("foregroundColor", string(c)) }
func (n Node) Background(c Color) Node           { return n.withMod("background", string(c)) }
func (n Node) Frame(o FrameOptions) Node         { return n.withMod("frame", o.toMap()) }
func (n Node) CornerRadius(r float64) Node       { return n.withMod("cornerRadius", r) }
func (n Node) Spacing(v float64) Node            { return n.withMod("spacing", v) }
func (n Node) Disabled(b bool) Node              { return n.withMod("disabled", b) }
func (n Node) Hidden(b bool) Node                { return n.withMod("hidden", b) }
func (n Node) Alignment(a Alignment) Node        { return n.withProp("alignment", string(a)) }
func (n Node) CommitOn(p CommitPolicy) Node      { return n.withProp("commit", string(p)) }

func (e EdgeInsets) toMap() map[string]any {
	m := map[string]any{}
	if e.Top != 0 {
		m["top"] = e.Top
	}
	if e.Bottom != 0 {
		m["bottom"] = e.Bottom
	}
	if e.Leading != 0 {
		m["leading"] = e.Leading
	}
	if e.Trailing != 0 {
		m["trailing"] = e.Trailing
	}
	return m
}

func (o FrameOptions) toMap() map[string]any {
	m := map[string]any{}
	if o.Width != 0 {
		m["width"] = o.Width
	}
	if o.Height != 0 {
		m["height"] = o.Height
	}
	if o.MaxWidth != 0 {
		m["maxWidth"] = dimension(o.MaxWidth)
	}
	if o.MaxHeight != 0 {
		m["maxHeight"] = dimension(o.MaxHeight)
	}
	if o.Alignment != "" {
		m["alignment"] = string(o.Alignment)
	}
	return m
}

func dimension(v float64) any {
	if v == Infinity {
		return "infinity"
	}
	return v
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	p := len(buf)
	for i > 0 {
		p--
		buf[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		buf[p] = '-'
	}
	return string(buf[p:])
}
