import SwiftUI

/// ノード 1 つに対応する View。各位置に置くのは常にこの具体型であり、
/// `AnyView` は `body` の内側（`interpret`）にだけ現れる。
///
/// ノードツリーは深さが実行時に決まる再帰構造で、`some View` の静的型が無限に入れ子になる。
/// そのため型消去は避けられないが、生成箇所をここ 1 か所に閉じることで
/// SwiftUI から見た位置の同一性を安定させる（design §2.2 の手 1）。
struct SuigoNodeView: View {
    let node: Node
    let runtime: SuigoRuntime

    var body: some View {
        #if DEBUG
        SuigoDebug.bodyEvaluations += 1
        #endif
        return interpret(node, runtime: runtime)
    }
}

#if DEBUG
/// 性能回帰テスト用の計数器（TP-004）。ID 採番の不安定化や `.id()` の付けすぎは動作を壊さず
/// 再評価回数だけを静かに増やすため、回数そのものを CI で固定する。
/// SwiftUI の body は MainActor で評価されるので、ここも MainActor に隔離する。
@MainActor
public enum SuigoDebug {
    public static var bodyEvaluations = 0
    public static func resetCounters() { bodyEvaluations = 0 }
}
#endif

/// ノード型 → SwiftUI View の switch。トランスポートには依存しない（FR-4.1）。
///
/// 未知のノード型は `EmptyView` + ログ（FR-4.2）。既知部分の描画は続く。
@MainActor
func interpret(_ node: Node, runtime: SuigoRuntime) -> AnyView {
    let view: AnyView
    switch node.type {
    case "Text":
        view = AnyView(Text(node.text))
    case "Button":
        // List の行内では既定スタイルの Button が行全体のタップを奪い、同じ行の Toggle のラベルを
        // 叩いただけで発火する（実測: 行ラベルのタップで「削除」が走り、行が消えた）。
        // borderless は List 外では既定と同じ見た目のまま、行内では自分の枠だけに反応する。
        view = AnyView(Button(node.label) { runtime.send(nodeID: node.id, kind: "tap", payload: .none) }
            .buttonStyle(.borderless))
    case "VStack":
        view = AnyView(VStack(alignment: horizontalAlignment(node.alignment), spacing: node.spacing) {
            children(of: node, runtime: runtime)
        })
    case "HStack":
        view = AnyView(HStack(alignment: verticalAlignment(node.alignment), spacing: node.spacing) {
            children(of: node, runtime: runtime)
        })
    case "ZStack":
        view = AnyView(ZStack(alignment: alignment(node.alignment)) {
            children(of: node, runtime: runtime)
        })
    case "Spacer":
        view = AnyView(Spacer())
    case "Divider":
        view = AnyView(Divider())
    case "ScrollView":
        view = AnyView(ScrollView(node.isHorizontal ? .horizontal : .vertical) {
            children(of: node, runtime: runtime)
        })
    case "List":
        view = AnyView(List { children(of: node, runtime: runtime) })
    case "ForEach":
        // 行にだけ .id() を付ける（FR-4.4 / design §2.2 の手 3）。全ノードに付けると
        // SwiftUI が不要な teardown を行い、ForEach の ID 差分による絞り込みも効かなくなる。
        view = AnyView(ForEach(node.children, id: \.id) { child in
            SuigoNodeView(node: child, runtime: runtime).id(child.id)
        })
    case "TextField":
        view = AnyView(TextFieldNodeView(node: node, runtime: runtime))
    case "Toggle":
        view = AnyView(ToggleNodeView(node: node, runtime: runtime))
    case "ProgressView":
        if let v = node.progress {
            view = AnyView(ProgressView(value: v))
        } else {
            view = AnyView(ProgressView())
        }
    case "Image":
        if let s = node.symbol {
            view = AnyView(Image(systemName: s))
        } else if let a = node.asset {
            view = AnyView(Image(a))
        } else {
            view = AnyView(EmptyView())
        }
    default:
        SuigoLog.unknownNode(node.type, id: node.id)
        view = AnyView(EmptyView())
    }
    return AnyView(applyModifiers(node.mods, to: view, nodeID: node.id))
}

/// 子を `ForEach(id:)` で列挙する（design §2.2 の手 2）。
/// PRE-3 の実測で、511 ノードのツリーを毎回差し替えても 1 か所変更なら body 評価は 3 個で済んだ。
/// SwiftUI は ID 差分だけでこれを達成しており、追加の Equatable 適合は不要だった。
@MainActor
private func children(of node: Node, runtime: SuigoRuntime) -> some View {
    ForEach(node.children, id: \.id) { child in
        SuigoNodeView(node: child, runtime: runtime)
    }
}

extension Node {
    /// `spacing` はコンテナの props ではなく mods に乗る（Go 側 `.Spacing()` が修飾子のため）。
    var spacing: CGFloat? {
        mods.last { $0.k == "spacing" }?.v.double.map { CGFloat($0) }
    }
}

func horizontalAlignment(_ s: String) -> HorizontalAlignment {
    switch s {
    case "leading": .leading
    case "trailing": .trailing
    default: .center
    }
}

func verticalAlignment(_ s: String) -> VerticalAlignment {
    switch s {
    case "top": .top
    case "bottom": .bottom
    default: .center
    }
}

func alignment(_ s: String) -> Alignment {
    switch s {
    case "leading": .leading
    case "trailing": .trailing
    case "top": .top
    case "bottom": .bottom
    case "topLeading": .topLeading
    case "topTrailing": .topTrailing
    case "bottomLeading": .bottomLeading
    case "bottomTrailing": .bottomTrailing
    default: .center
    }
}
