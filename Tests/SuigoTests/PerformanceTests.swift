import Foundation
import SwiftUI
import UIKit
import Testing
@testable import Suigo

/// 性能回帰（test_design §8）。検証C / PRE-3 の実測値を回帰として固定する。
///
/// - TP-004: 1 か所変更時の body 再評価回数。実測 3。閾値 10。**最も価値のある回帰テスト**
/// - TP-002 / TP-003: 境界コスト（復号）。実測 1.0 ms / 4.6 ms。CI マシン差を吸収するため 10 倍以上の余裕を取る
///
/// 時間の閾値は環境差で揺れるため緩く、回数の閾値は決定的なので厳しく置く。
@Suite("Performance regression", .serialized)
@MainActor
struct PerformanceTests {

    /// rows 行の VStack。1 行 = HStack(Text, Button) の 3 ノード。総ノード数 = 1 + rows*3。
    /// changeLast=true なら最終行のテキストだけ変える（1 か所変更）。
    private func renderFrame(seq: Int, rows: Int, changeLast: Bool) -> Data {
        var children: [String] = []
        children.reserveCapacity(rows)
        for i in 0..<rows {
            let text = (changeLast && i == rows - 1) ? "CHANGED" : "item \(i)"
            children.append(#"{"id":"0/\#(i)","type":"HStack","children":[{"id":"0/\#(i)/0","type":"Text","props":{"text":"\#(text)"}},{"id":"0/\#(i)/1","type":"Button","props":{"label":"done"}}]}"#)
        }
        let tree = #"{"id":"0","type":"VStack","children":[\#(children.joined(separator: ","))]}"#
        return Data(#"{"type":"render","seq":\#(seq),"tree":\#(tree)}"#.utf8)
    }

    private struct Root: View {
        let runtime: SuigoRuntime
        var body: some View {
            if let t = runtime.tree { SuigoNodeView(node: t, runtime: runtime) } else { EmptyView() }
        }
    }

    /// 固定時間ではなく「評価回数が floor 以上になり、その後 2 回連続で増えなくなる」まで待つ。
    /// CI の遅いランナーでは 0.4 秒では描画が間に合わず、初回の CI でこの suite が一度落ちた
    /// （当時はログにテスト名が出ず特定できなかった）。上限は 5 秒。
    private func settle(_ runtime: SuigoRuntime, atLeast floor: Int) -> Int {
        let deadline = Date().addingTimeInterval(5)
        var last = -1, stable = 0
        while Date() < deadline {
            RunLoop.main.run(until: Date().addingTimeInterval(0.1))
            let now = SuigoDebug.bodyEvaluations(for: runtime)
            if now >= floor && now == last { stable += 1; if stable >= 2 { return now } } else { stable = 0 }
            last = now
        }
        return SuigoDebug.bodyEvaluations(for: runtime)
    }

    // TP-004
    @Test("one-node change re-evaluates at most 10 bodies in a 511-node tree")
    func oneNodeChangeBodyCount() throws {
        let runtime = SuigoRuntime(transport: MockTransport())
        let host = UIHostingController(rootView: Root(runtime: runtime))
        let window = UIWindow(frame: CGRect(x: 0, y: 0, width: 390, height: 844))
        window.rootViewController = host
        window.makeKeyAndVisible()

        SuigoDebug.resetCounter(for: runtime)
        runtime.handle(renderFrame(seq: 1, rows: 170, changeLast: false))
        host.view.setNeedsLayout()
        host.view.layoutIfNeeded()
        let initial = settle(runtime, atLeast: 511)
        #expect(initial >= 511, "初回は全ノードが評価されるはず（計数器の妥当性）: \(initial)")

        SuigoDebug.resetCounter(for: runtime)
        runtime.handle(renderFrame(seq: 2, rows: 170, changeLast: true))
        host.view.setNeedsLayout()
        host.view.layoutIfNeeded()
        let after = settle(runtime, atLeast: 1)
        print("TP-004 bodies re-evaluated after 1-node change (511-node tree): \(after) (measured 3, limit 10)")
        #expect(after >= 1, "変更が反映されていない")
        #expect(after <= 10, "1 か所変更で \(after) body が再評価された。ID 採番の不安定化か .id() の付けすぎを疑う")
    }

    // 対照: 全ノード変更なら N に比例して増える（計数器が機能している証拠）
    @Test("all-rows change re-evaluates proportionally (control)")
    func allChangeBodyCount() throws {
        let runtime = SuigoRuntime(transport: MockTransport())
        let host = UIHostingController(rootView: Root(runtime: runtime))
        let window = UIWindow(frame: CGRect(x: 0, y: 0, width: 390, height: 844))
        window.rootViewController = host
        window.makeKeyAndVisible()

        runtime.handle(renderFrame(seq: 1, rows: 30, changeLast: false))
        host.view.setNeedsLayout(); host.view.layoutIfNeeded(); _ = settle(runtime, atLeast: 91)

        SuigoDebug.resetCounter(for: runtime)
        var children: [String] = []
        for i in 0..<30 {
            children.append(#"{"id":"0/\#(i)","type":"HStack","children":[{"id":"0/\#(i)/0","type":"Text","props":{"text":"ALL \#(i)"}},{"id":"0/\#(i)/1","type":"Button","props":{"label":"done"}}]}"#)
        }
        runtime.handle(Data(#"{"type":"render","seq":2,"tree":{"id":"0","type":"VStack","children":[\#(children.joined(separator: ","))]}}"#.utf8))
        host.view.setNeedsLayout(); host.view.layoutIfNeeded()
        let after = settle(runtime, atLeast: 30)
        print("control: bodies re-evaluated after all-rows change (91-node tree): \(after)")
        #expect(after >= 30, "全行変更なのに \(after) しか再評価されていない。計数器が壊れている")
    }

    // TP-002 / TP-003: 復号時間。閾値は実測の 10 倍以上。
    @Test("decode cost stays within a generous budget")
    func decodeBudget() throws {
        func median(_ rows: Int) -> Double {
            let data = renderFrame(seq: 1, rows: rows, changeLast: false)
            var samples: [Double] = []
            for _ in 0..<20 {
                let t0 = DispatchTime.now().uptimeNanoseconds
                _ = try? IncomingFrame.decode(data)
                samples.append(Double(DispatchTime.now().uptimeNanoseconds - t0) / 1_000_000)
            }
            return samples.sorted()[samples.count / 2]
        }
        let ms91 = median(30), ms511 = median(170)
        print(String(format: "TP-002 decode 91 nodes: %.2f ms (measured 1.0, limit 20) | TP-003 decode 511 nodes: %.2f ms (measured 4.6, limit 60)", ms91, ms511))
        #expect(ms91 < 20)
        #expect(ms511 < 60)
    }
}
