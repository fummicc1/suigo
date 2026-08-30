import Foundation
import SwiftUI
import Testing
@testable import Suigo

/// インタープリタは `AnyView` を返すため、View の内部構造は直接検査できない。
/// ここでは「クラッシュせず View を返す」「ログ経路が正しい」「修飾子の適用順が保たれる」を検証し、
/// 描画結果そのもの（ピクセル）は対象外とする（test_design §1.2）。
@Suite("Interpreter")
@MainActor
struct InterpreterTests {

    private func runtime() -> SuigoRuntime { SuigoRuntime(transport: MockTransport()) }

    // TS-001: 14 種すべてが View を返す（クラッシュしない）
    @Test("all 14 primitives interpret without crashing")
    func allPrimitives() throws {
        let tree = try Fixtures.render("render_all_primitives")
        let rt = runtime()
        for node in collect(tree) {
            _ = interpret(node, runtime: rt)
        }
        #expect(Set(collect(tree).map(\.type)).count == 14)
    }

    // TS-004: 深いネストも解釈できる
    @Test("deep_100 interprets")
    func deep() throws {
        _ = interpret(try Fixtures.render("render_deep_100"), runtime: runtime())
    }

    // TS-101: 未知ノード型 → EmptyView、クラッシュしない
    @Test("unknown node type yields a view and does not crash")
    func unknownNode() throws {
        guard case .render(_, let tree) = try IncomingFrame.decode(try Fixtures.load("invalid/unknown_node.json")) else {
            Issue.record("not render"); return
        }
        _ = interpret(tree, runtime: runtime())
        #expect(tree.children[1].type == "Hologram")
    }

    // TS-103: 既知部分は描画される（未知ノードが混ざっていても兄弟は健在）
    @Test("known siblings survive an unknown node")
    func knownSiblings() throws {
        guard case .render(_, let tree) = try IncomingFrame.decode(try Fixtures.load("invalid/unknown_node.json")) else {
            Issue.record("not render"); return
        }
        #expect(tree.children.count == 3)
        #expect(tree.children[0].text == "前は描かれる")
        #expect(tree.children[2].text == "後ろも描かれる")
        _ = interpret(tree, runtime: runtime())
    }

    // TS-102: 未知の修飾子は無視され、残りは適用される
    @Test("unknown modifier is skipped, others still apply")
    func unknownModifier() throws {
        guard case .render(_, let tree) = try IncomingFrame.decode(try Fixtures.load("invalid/unknown_modifier.json")) else {
            Issue.record("not render"); return
        }
        #expect(tree.mods.map(\.k) == ["glow", "padding"])
        _ = applyModifiers(tree.mods, to: AnyView(Text("x")), nodeID: tree.id)
    }

    // TS-104: props の型違い → 既定値へ
    @Test("wrong prop type falls back to default")
    func wrongPropType() throws {
        guard case .render(_, let tree) = try IncomingFrame.decode(try Fixtures.load("invalid/wrong_prop_type.json")) else {
            Issue.record("not render"); return
        }
        #expect(tree.type == "Toggle")
        #expect(tree.props["on"] == .string("yes"))
        #expect(tree.isOn == false)   // string は bool として読めないので既定値
        _ = interpret(tree, runtime: runtime())
    }

    // TS-105: 必須 props の欠落 → 空文字
    @Test("missing required prop falls back to empty")
    func missingProp() throws {
        guard case .render(_, let tree) = try IncomingFrame.decode(try Fixtures.load("invalid/missing_prop.json")) else {
            Issue.record("not render"); return
        }
        #expect(tree.children[0].text == "")
        #expect(tree.children[1].isOn == false)
        _ = interpret(tree, runtime: runtime())
    }

    @Test("spacing is read from mods, not props")
    func spacing() throws {
        let tree = try Fixtures.render("render_counter")
        #expect(tree.spacing == 12)
        #expect(tree.children[0].spacing == nil)
    }

    @Test("font / color / alignment lookups cover the contract")
    func lookups() {
        for f in ["largeTitle", "title", "title2", "title3", "headline", "subheadline",
                  "body", "callout", "footnote", "caption", "caption2"] {
            #expect(font(f) != nil, "font \(f)")
        }
        #expect(font("comic-sans") == nil)
        for c in ["primary", "secondary", "red", "orange", "yellow", "green", "mint", "teal", "cyan",
                  "blue", "indigo", "purple", "pink", "brown", "gray", "black", "white", "clear"] {
            #expect(color(c) != nil, "color \(c)")
        }
        #expect(color("#FF8800") != nil)
        #expect(color("#ff8800") != nil)
        #expect(color("#FF88") == nil)
        #expect(color("notacolor") == nil)
    }

    @Test("commit policy parsing per wire-format §3.3")
    func commitPolicy() {
        #expect(CommitPolicy("submit", id: "0") == .submit)
        #expect(CommitPolicy("blur", id: "0") == .blur)
        #expect(CommitPolicy("debounce:300", id: "0") == .debounce(ms: 300))
        #expect(CommitPolicy("debounce:0", id: "0") == .submit)
        #expect(CommitPolicy("nonsense", id: "0") == .submit)
    }

    @Test("Button tap sends a tap event through the runtime")
    func buttonSendsTap() async throws {
        let mock = MockTransport()
        let rt = SuigoRuntime(transport: mock)
        rt.send(nodeID: "0/1", kind: "tap", payload: .none)
        try await Task.sleep(for: .milliseconds(50))
        let ev = try #require(mock.sent.first { $0["type"] as? String == "event" })
        #expect(ev["nodeID"] as? String == "0/1")
        #expect(ev["kind"] as? String == "tap")
        #expect((ev["payload"] as? [String: Any])?["t"] as? String == "none")
    }
}
