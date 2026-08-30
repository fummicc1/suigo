#if DEBUG
import Foundation
import Network

/// dev トランスポート。`suigo dev` に TCP で接続し、4 バイト LE 長さ接頭辞つきフレームを流す。
///
/// ファイル全体が `#if DEBUG` で囲われており、Release ビルドには一切含まれない（AR-2 / RISK-4）。
/// 切断時は 1 秒間隔で再接続する。接続先は `127.0.0.1` のみ（design §6.1）。
public final class SocketTransport: SuigoTransport, @unchecked Sendable {
    public static let maxFrameLength = 16 * 1024 * 1024

    private let port: UInt16
    private let queue = DispatchQueue(label: "dev.suigo.socket")
    private let lock = NSLock()
    private var connection: NWConnection?
    private var closed = false
    private var continuation: AsyncStream<Data>.Continuation?
    private let stream: AsyncStream<Data>

    public init(port: Int) {
        self.port = UInt16(clamping: port)
        var c: AsyncStream<Data>.Continuation!
        stream = AsyncStream { c = $0 }
        continuation = c
        connect()
    }

    public func frames() -> AsyncStream<Data> { stream }

    public func send(_ frame: Data) async throws {
        guard frame.count > 0, frame.count <= Self.maxFrameLength else { throw SocketError.frameTooLarge }
        let conn = lock.withLock { connection }
        guard let conn, conn.state == .ready else { throw SocketError.notConnected }
        var packet = Data(capacity: 4 + frame.count)
        var len = UInt32(frame.count).littleEndian
        withUnsafeBytes(of: &len) { packet.append(contentsOf: $0) }
        packet.append(frame)
        try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
            conn.send(content: packet, completion: .contentProcessed { err in
                if let err { cont.resume(throwing: err) } else { cont.resume() }
            })
        }
    }

    public func close() async {
        lock.withLock {
            closed = true
            connection?.cancel()
            connection = nil
        }
        continuation?.finish()
    }

    // MARK: - 接続と受信

    private func connect() {
        let conn = NWConnection(host: "127.0.0.1", port: NWEndpoint.Port(rawValue: port)!, using: .tcp)
        lock.withLock { connection = conn }
        conn.stateUpdateHandler = { [weak self] state in
            guard let self else { return }
            switch state {
            case .ready:
                SuigoLog.transport.info("connected to suigo dev on :\(self.port)")
                self.readHeader(conn)
            case .failed(let err):
                SuigoLog.transport.warning("connection failed: \(err, privacy: .public)")
                self.scheduleReconnect()
            case .cancelled:
                self.scheduleReconnect()
            default:
                break
            }
        }
        conn.start(queue: queue)
    }

    private func scheduleReconnect() {
        let stop = lock.withLock { closed }
        guard !stop else { return }
        queue.asyncAfter(deadline: .now() + 1) { [weak self] in
            guard let self, !self.lock.withLock({ self.closed }) else { return }
            self.connect()
        }
    }

    private func readHeader(_ conn: NWConnection) {
        conn.receive(minimumIncompleteLength: 4, maximumLength: 4) { [weak self] data, _, _, err in
            guard let self else { return }
            guard let data, data.count == 4, err == nil else {
                conn.cancel()
                return
            }
            let len = Int(data.withUnsafeBytes { $0.loadUnaligned(as: UInt32.self) }.littleEndian)
            // 長さ 0 と上限超過は接続を切る（docs/wire-format.md §1）。
            guard len > 0, len <= Self.maxFrameLength else {
                SuigoLog.transport.error("bad frame length \(len); disconnecting")
                conn.cancel()
                return
            }
            self.readBody(conn, length: len)
        }
    }

    private func readBody(_ conn: NWConnection, length: Int) {
        conn.receive(minimumIncompleteLength: length, maximumLength: length) { [weak self] data, _, _, err in
            guard let self else { return }
            guard let data, data.count == length, err == nil else {
                conn.cancel()
                return
            }
            self.continuation?.yield(data)
            self.readHeader(conn)
        }
    }

    public enum SocketError: Error { case notConnected, frameTooLarge }
}
#endif
