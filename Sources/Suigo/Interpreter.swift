import SwiftUI

/// ノード 1 つに対応する View。各位置に置くのは常にこの具体型であり、
/// `AnyView` は `body` の内側（`interpret`）にだけ現れる。
///
/// ノードツリーは深さが実行時に決まる再帰構造で、`some View` の静的型が無限に入れ子になる。
/// そのため型消去は避けられないが、生成箇所をここ 1 か所に閉じることで
/// SwiftUI から見た位置の同一性を安定させる（design §2.2 の手 1）。
struct SuigoNodeView: View, Equatable {
    let node: Node
    let runtime: SuigoRuntime

    // 明示的な == を持ち、子の生成箇所で .equatable() を付ける。
    // SwiftUI は非 Equatable な view の格納プロパティを発見的に比較するが、CI ランナー上で
    // その比較が効かず 1 か所変更が 173 body になる run が観測された（同じイメージ・同じ
    // シミュレータで 3 になる run もあり、非決定的）。EquatableView なら == が直接呼ばれる。
    // View は MainActor 隔離なので、Equatable の要件は nonisolated で満たす。
    // 比較するのは Sendable な Node の値と、ランタイムの参照同一性だけなので安全である。
    nonisolated static func == (a: Self, b: Self) -> Bool {
        a.runtime === b.runtime && a.node == b.node
    }

    var body: some View {
        #if DEBUG
        SuigoDebug.record(runtime)
        #endif
        return interpret(node, runtime: runtime)
    }
}

#if DEBUG
/// 性能回帰テスト用の計数器（TP-004）。ID 採番の不安定化や `.id()` の付けすぎは動作を壊さず
/// 再評価回数だけを静かに増やすため、回数そのものを CI で固定する。
/// 並列に走る他のテストの描画を数えないよう、ランタイム単位で数える。
/// SwiftUI の body は MainActor で評価されるので、ここも MainActor に隔離する。
@MainActor
public enum SuigoDebug {
    private static var counts: [ObjectIdentifier: Int] = [:]
    static func record(_ runtime: SuigoRuntime) { counts[ObjectIdentifier(runtime), default: 0] += 1 }
    public static func bodyEvaluations(for runtime: SuigoRuntime) -> Int { counts[ObjectIdentifier(runtime)] ?? 0 }
    public static func resetCounter(for runtime: SuigoRuntime) { counts[ObjectIdentifier(runtime)] = 0 }
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
            SuigoNodeView(node: child, runtime: runtime).equatable().id(child.id)
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

/// 子を `ForEach(id:)` で列挙し、`.equatable()` で == による比較を強制する（design §2.2 の手 2）。
/// 511 ノードのツリーを毎回差し替えても 1 か所変更なら body 評価は 3 個で済む（TP-004 が固定）。
@MainActor
private func children(of node: Node, runtime: SuigoRuntime) -> some View {
    ForEach(node.children, id: \.id) { child in
        SuigoNodeView(node: child, runtime: runtime).equatable()
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
