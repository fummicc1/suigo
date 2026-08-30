import Foundation
import Suigo
import CSuigo

/// 本番経路。Go を c-archive としてリンクし、C ABI でフレームを流す（design §2.2）。
///
/// Go → Swift は `suigo_next_frame` のブロッキング読み出しで受ける。関数ポインタは使わない。
/// 専用スレッドがブロックし続けるだけなので、Swift 6 の `Sendable` 問題が発生しない（RISK-6 の解消）。
/// `suigo_next_frame` の呼び出しはこの型の外へ漏らさない——メインスレッドから呼ぶと UI が固まる。
public final class EmbeddedTransport: SuigoTransport, @unchecked Sendable {
    // 保持するのは C のハンドル（Int32）だけで、同期は Go 側が担う。型で表現できないため @unchecked。
    private let handle: Int32
    private let stream: AsyncStream<Data>
    private let reader: Thread

    public init() {
        let h = suigo_mount()
        precondition(h > 0, "suigo_mount failed (\(h)); did the Go side call suigo.Register in init()?")
        handle = h

        var c: AsyncStream<Data>.Continuation!
        stream = AsyncStream { c = $0 }
        let cont = c!

        reader = Thread {
            // ブロッキング読み出し。unmount されると NULL が返り、このループが終わる（TB-003）。
            while true {
                var len: Int32 = 0
                guard let p = suigo_next_frame(h, &len), len > 0 else { break }
                cont.yield(Data(bytes: p, count: Int(len)))
                suigo_free(p)
            }
            cont.finish()
        }
        reader.name = "dev.suigo.reader"
        reader.start()
    }

    public func frames() -> AsyncStream<Data> { stream }

    /// 呼び出し中に Go がコピーするため、戻った直後にバッファは解放してよい（FR-7.2）。
    public func send(_ frame: Data) async throws {
        let rc = frame.withUnsafeBytes { buf -> Int32 in
            guard let base = buf.baseAddress else { return -2 }
            return suigo_send(handle, UnsafeMutableRawPointer(mutating: base), Int32(buf.count))
        }
        guard rc == 0 else { throw EmbeddedError.sendFailed(code: rc) }
    }

    public func close() async {
        suigo_unmount(handle)
    }

    public enum EmbeddedError: Error { case sendFailed(code: Int32) }
}
