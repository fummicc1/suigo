import Foundation

/// フレームの送受信を抽象化する。Interpreter はどのトランスポートを使っているかを知らない（AR-1）。
///
/// dev（TCP）と embedded（C ABI）はどちらも「ストリームからフレームを読む」だけなので、
/// 文字どおり同じ形になる。
public protocol SuigoTransport: Sendable {
    func send(_ frame: Data) async throws
    func frames() -> AsyncStream<Data>
    func close() async
}

/// テストと開発用。固定フレームを流し、送信されたフレームを記録する。
public final class MockTransport: SuigoTransport, @unchecked Sendable {
    private let lock = NSLock()
    private var sentFrames: [Data] = []
    private var continuation: AsyncStream<Data>.Continuation?
    private let stream: AsyncStream<Data>

    public init() {
        var c: AsyncStream<Data>.Continuation!
        stream = AsyncStream { c = $0 }
        continuation = c
    }

    /// テストから Go 側の役として呼ぶ。
    public func inject(_ frame: Data) { continuation?.yield(frame) }
    public func injectJSON(_ s: String) { inject(Data(s.utf8)) }

    /// 送信されたフレームを JSON 辞書として返す。
    public var sent: [[String: Any]] {
        lock.withLock {
            sentFrames.compactMap { try? JSONSerialization.jsonObject(with: $0) as? [String: Any] }
        }
    }

    public func send(_ frame: Data) async throws {
        lock.withLock { sentFrames.append(frame) }
    }
    public func frames() -> AsyncStream<Data> { stream }
    public func close() async { continuation?.finish() }
}
