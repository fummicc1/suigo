import Foundation
import Suigo

public extension SuigoView {
    /// 既定の構成。`SUIGO_DEV_PORT` があれば dev（DEBUG のみ）、無ければ embedded（design §4.3）。
    /// Release ビルドでは dev の分岐がコンパイルされないため、本番に TCP 経路が混入しない（AR-2）。
    ///
    /// ホストアプリ側の記述はこれだけになる。
    ///
    /// ```swift
    /// @main struct CounterApp: App {
    ///     var body: some Scene { WindowGroup { SuigoView.standard() } }
    /// }
    /// ```
    static func standard() -> SuigoView {
        #if DEBUG
        if let port = ProcessInfo.processInfo.environment["SUIGO_DEV_PORT"].flatMap(Int.init) {
            return SuigoView(transport: SocketTransport(port: port))
        }
        #endif
        return SuigoView(transport: EmbeddedTransport())
    }
}
