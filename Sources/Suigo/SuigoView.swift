import SwiftUI

/// 公開エントリポイント。ホストアプリはこれを置くだけでよい。
///
/// ```swift
/// @main struct CounterApp: App {
///     var body: some Scene { WindowGroup { SuigoView(transport: MockTransport()) } }
/// }
/// ```
public struct SuigoView: View {
    @State private var runtime: SuigoRuntime
    @Environment(\.colorScheme) private var colorScheme

    /// `standard()`（dev / embedded の自動選択）は SuigoEmbedded 製品が提供する。
    /// Suigo 単体では Go の c-archive を要求しないため、テストや dev 専用ホストはこちらだけで完結する。
    public init(transport: any SuigoTransport) {
        _runtime = State(initialValue: SuigoRuntime(transport: transport))
    }

    public var body: some View {
        ZStack {
            if let tree = runtime.tree {
                SuigoNodeView(node: tree, runtime: runtime)
            } else {
                ProgressView()
            }
            #if DEBUG
            if let err = runtime.lastError {
                ErrorOverlay(error: err)
            }
            #endif
        }
        .onAppear { runtime.start() }
        .onDisappear { runtime.stop() }
        .onChange(of: colorScheme, initial: true) { _, scheme in
            runtime.sendEnv(colorScheme: scheme == .dark ? "dark" : "light",
                            safeArea: SafeArea(), sizeClass: SizeClass(h: "compact", v: "regular"))
        }
    }
}

#if DEBUG
/// 開発時のみ。直前の描画は消さず、上に重ねて通知する（design §5.1）。
struct ErrorOverlay: View {
    let error: SuigoRuntime.RuntimeError

    var body: some View {
        VStack {
            Spacer()
            VStack(alignment: .leading, spacing: 4) {
                Text("\(error.severity) · \(error.code)").font(.caption.bold())
                Text(error.message).font(.caption).lineLimit(6)
            }
            .padding(12)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(error.severity == "warning" ? Color.orange.opacity(0.9) : Color.red.opacity(0.9))
            .foregroundStyle(.white)
        }
        .allowsHitTesting(false)
    }
}
#endif
