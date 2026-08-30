import SwiftUI

/// Toggle は「Go 所有（宣言的）」に近いが、操作の瞬間だけ Swift が値を持つ。
/// 変更のたびに Go へ送り、次の render で Go の値に揃う。
struct ToggleNodeView: View {
    let node: Node
    let runtime: SuigoRuntime

    var body: some View {
        Toggle(node.label, isOn: Binding(
            get: { node.isOn },
            set: { runtime.send(nodeID: node.id, kind: "change", payload: .bool($0)) }
        ))
    }
}
