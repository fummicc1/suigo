import Foundation
import Observation

/// フレームを受けて復号し、最新のツリーを保持する。SwiftUI からはこれを観測する。
///
/// - 描画要求が画面の更新より速く届いた場合、最新のツリーだけを保持する（FR-4.5）
/// - 古い `seq` は捨てる
/// - どのエラーでも直前のツリーを保持し、画面を消さない（design §5.1）
@MainActor
@Observable
public final class SuigoRuntime {
    public private(set) var tree: Node?
    public private(set) var lastSeq: UInt64 = 0
    public private(set) var lastError: RuntimeError?
    public private(set) var isConnected = false

    private let transport: any SuigoTransport
    private var receiveTask: Task<Void, Never>?

    public struct RuntimeError: Equatable, Sendable {
        public let severity: String
        public let code: String
        public let message: String
    }

    public init(transport: any SuigoTransport) {
        self.transport = transport
    }

    /// 受信を開始し、mount を送る。
    public func start(restore: Data? = nil) {
        guard receiveTask == nil else { return }
        receiveTask = Task { [transport] in
            for await data in transport.frames() {
                self.handle(data)
            }
            self.isConnected = false
        }
        isConnected = true
        Task { try? await transport.send(try OutgoingFrame.mount(restore: restore).encode()) }
    }

    public func stop() {
        receiveTask?.cancel()
        receiveTask = nil
        Task { await transport.close() }
    }

    /// SwiftUI の操作から呼ぶ。メインスレッドで即座に戻る。
    public func send(nodeID: String, kind: String, payload: EventPayload) {
        let frame = OutgoingFrame.event(nodeID: nodeID, kind: kind, payload: payload)
        Task { [transport] in
            do { try await transport.send(try frame.encode()) }
            catch { SuigoLog.transport.error("send failed: \(error, privacy: .public)") }
        }
    }

    public func sendEnv(colorScheme: String, safeArea: SafeArea, sizeClass: SizeClass) {
        let frame = OutgoingFrame.env(colorScheme: colorScheme, safeArea: safeArea, sizeClass: sizeClass)
        Task { [transport] in try? await transport.send(try frame.encode()) }
    }

    /// テストからも直接呼べるよう internal にしている。
    func handle(_ data: Data) {
        let frame: IncomingFrame
        do {
            frame = try IncomingFrame.decode(data)
        } catch {
            SuigoLog.runtime.error("frame decode failed, keeping previous tree: \(error, privacy: .public)")
            return
        }
        switch frame {
        case .render(let seq, let newTree):
            guard seq > lastSeq else {
                SuigoLog.runtime.debug("dropping stale frame seq=\(seq) (have \(self.lastSeq))")
                return
            }
            lastSeq = seq
            tree = newTree
            lastError = nil
        case .error(let sev, let code, let msg):
            lastError = RuntimeError(severity: sev, code: code, message: msg)
            SuigoLog.runtime.warning("\(sev, privacy: .public)/\(code, privacy: .public): \(msg, privacy: .public)")
        case .snapshotResult:
            break
        case .unknown(let type):
            SuigoLog.runtime.warning("unknown frame type \(type, privacy: .public); ignored")
        }
    }
}
