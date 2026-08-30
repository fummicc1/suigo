package suigo

// Text は文字列を表示する。
func Text(s string) Node {
	return newNode(TypeText).withProp("text", s)
}

// Button はタップで onTap を呼ぶ。
func Button(label string, onTap func()) Node {
	n := newNode(TypeButton).withProp("label", label)
	if onTap != nil {
		n = n.withHandler("tap", func(Payload) { onTap() })
	}
	return n
}

func VStack(children ...Node) Node { return newNode(TypeVStack, children...) }
func HStack(children ...Node) Node { return newNode(TypeHStack, children...) }
func ZStack(children ...Node) Node { return newNode(TypeZStack, children...) }
func List(children ...Node) Node   { return newNode(TypeList, children...) }

func Spacer() Node  { return newNode(TypeSpacer) }
func Divider() Node { return newNode(TypeDivider) }

// ScrollView は縦方向のスクロール領域。横方向は Horizontal() で切り替える。
func ScrollView(children ...Node) Node { return newNode(TypeScrollView, children...) }

// Horizontal は ScrollView の軸を横に変える。
func (n Node) Horizontal() Node { return n.withProp("axis", "h") }

// TextField は Swift 側でローカルの下書きを持ち、確定時にだけ onCommit を呼ぶ。
// 既定の確定契機は submit。CommitOn で変えられる。
func TextField(label, value string, onCommit func(string)) Node {
	n := newNode(TypeTextField).
		withProp("label", label).
		withProp("value", value).
		withProp("commit", string(CommitOnSubmit))
	if onCommit != nil {
		n = n.withHandler("commit", func(p Payload) { onCommit(p.String) })
	}
	return n
}

// Toggle はオンオフの切り替えで onChange を呼ぶ。
func Toggle(label string, on bool, onChange func(bool)) Node {
	n := newNode(TypeToggle).withProp("label", label).withProp("on", on)
	if onChange != nil {
		n = n.withHandler("change", func(p Payload) { onChange(p.Bool) })
	}
	return n
}

// ProgressView は 0.0〜1.0 の進捗を表示する。
func ProgressView(value float64) Node {
	return newNode(TypeProgressView).withProp("value", value)
}

// IndeterminateProgress は終了時刻が分からない処理を示す回転表示。
func IndeterminateProgress() Node { return newNode(TypeProgressView) }

// Symbol は SF Symbol を表示する。
func Symbol(name string) Node { return newNode(TypeImage).withProp("symbol", name) }

// Asset はホストアプリの bundle にある画像を表示する。
func Asset(name string) Node { return newNode(TypeImage).withProp("asset", name) }
