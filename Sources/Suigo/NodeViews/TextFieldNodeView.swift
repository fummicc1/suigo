import SwiftUI

/// TextField は「Swift ローカル + コミット」の所有権を持つ（FR-5.2〜5.4）。
///
/// - 編集中の値は Swift 側の `draft` が正。1 打鍵ごとに Go へは送らない
/// - `commit` ポリシー（submit / blur / debounce:<ms>）で確定したときだけ送る
/// - Go から新しい値が届いても、**フォーカス中はローカルを上書きしない**（カーソル飛びの防止）
struct TextFieldNodeView: View {
    let node: Node
    let runtime: SuigoRuntime

    @State private var draft: String = ""
    @State private var lastRemote: String = ""
    @FocusState private var focused: Bool
    @State private var debounceTask: Task<Void, Never>?

    var body: some View {
        TextField(node.label, text: $draft)
            .focused($focused)
            .onSubmit { if policy == .submit { commit() } }
            .onChange(of: focused) { _, isFocused in
                if !isFocused, policy == .blur { commit() }
            }
            .onChange(of: draft) { _, _ in
                if case .debounce(let ms) = policy { scheduleDebounce(ms) }
            }
            .onChange(of: node.value) { _, remote in
                // FR-5.4: フォーカスされていないときだけ Go の値を反映する
                lastRemote = remote
                if !focused { draft = remote }
            }
            .onAppear {
                lastRemote = node.value
                draft = node.value
            }
    }

    private var policy: CommitPolicy { CommitPolicy(node.commitPolicy, id: node.id) }

    private func commit() {
        debounceTask?.cancel()
        guard draft != lastRemote else { return }
        runtime.send(nodeID: node.id, kind: "commit", payload: .string(draft))
    }

    private func scheduleDebounce(_ ms: Int) {
        debounceTask?.cancel()
        debounceTask = Task { @MainActor in
            try? await Task.sleep(for: .milliseconds(ms))
            if !Task.isCancelled { commit() }
        }
    }
}

/// docs/wire-format.md §3.3
enum CommitPolicy: Equatable {
    case submit
    case blur
    case debounce(ms: Int)

    init(_ s: String, id: String) {
        switch s {
        case "submit": self = .submit
        case "blur": self = .blur
        default:
            if s.hasPrefix("debounce:"), let ms = Int(s.dropFirst("debounce:".count)), ms > 0 {
                self = .debounce(ms: ms)
            } else {
                SuigoLog.badValue("commit policy", s, id: id)
                self = .submit
            }
        }
    }
}
