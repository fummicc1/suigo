import Foundation
import Testing
@testable import Suigo

@Suite("Node decoding")
struct NodeDecodingTests {

    @Test("counter golden decodes with expected structure")
    func counter() throws {
        let tree = try Fixtures.render("render_counter")
        #expect(tree.id == "0")
        #expect(tree.type == "VStack")
        #expect(tree.mods.map(\.k) == ["spacing"])
        #expect(tree.children.count == 2)
        #expect(tree.children[0].text == "0")
        #expect(tree.children[0].mods.map(\.k) == ["font", "foregroundColor"])
        #expect(tree.children[1].label == "Tap")
    }

    @Test("all 14 primitives decode and count matches Go side")
    func allPrimitives() throws {
        let tree = try Fixtures.render("render_all_primitives")
        #expect(tree.count == 21)
        let types = Set(collect(tree).map(\.type))
        let expected: Set<String> = ["Text", "Button", "VStack", "HStack", "ZStack", "Spacer", "Divider",
                                     "ScrollView", "List", "ForEach", "TextField", "Toggle", "ProgressView", "Image"]
        #expect(types == expected)
    }

    @Test("modifier order is preserved (TS-002 precondition)")
    func modifierOrder() throws {
        let tree = try Fixtures.render("render_all_modifiers")
        #expect(tree.children[0].mods.map(\.k) == ["padding", "background"])
        #expect(tree.children[1].mods.map(\.k) == ["background", "padding"])
        let all = tree.children[2].mods
        #expect(all.map(\.k) == ["padding", "font", "foregroundColor", "background", "frame",
                                 "cornerRadius", "disabled", "hidden"])
        #expect(all[4].v.object?["maxWidth"] == .string("infinity"))
        #expect(all[6].v == .bool(true))
    }

    @Test("props are typed accessors, bool stays bool and number stays number")
    func typedProps() throws {
        let tree = try Fixtures.render("render_all_primitives")
        let nodes = collect(tree)
        let toggle = try #require(nodes.first { $0.type == "Toggle" })
        #expect(toggle.isOn == true)
        #expect(toggle.props["on"] == .bool(true))
        let progress = try #require(nodes.first { $0.type == "ProgressView" && $0.progress != nil })
        #expect(progress.progress == 0.42)
        let field = try #require(nodes.first { $0.type == "TextField" })
        #expect(field.commitPolicy == "debounce:300")
        #expect(field.value == "abc")
    }

    @Test("todo3: ForEach rows carry explicit IDs and are not flattened")
    func todo3() throws {
        let tree = try Fixtures.render("render_todo3")
        let list = tree.children[2]
        #expect(list.type == "List")
        let forEach = list.children[0]
        #expect(forEach.type == "ForEach")
        #expect(forEach.children.map(\.id) == ["todo-1", "todo-2", "todo-3"])
        // ForEach の後ろの兄弟は行数に関係なく 0/3
        #expect(tree.children[3].id == "0/3")
        #expect(tree.children[3].text == "3 件")
    }

    @Test("deep_100 golden decodes to depth 100")
    func deep100() throws {
        let tree = try Fixtures.render("render_deep_100")
        #expect(tree.count == 100)
        #expect(depth(of: tree) == 100)
    }

    // TS-201: 深さ 10,000 は上限で打ち切り、クラッシュしない。
    // 実際に弾くのは JSONSerialization だが、呼び出し側には tooDeep として見える。
    @Test("depth 10000 is rejected as tooDeep, not by stack overflow")
    func tooDeep() {
        #expect(throws: NodeDecodingError.tooDeep) {
            _ = try Node.decode(deepTreeJSON(depth: 10_000))
        }
    }

    // TS-204: ノード深さ maxNodeDepth（256）ちょうどは復号できる。257 は tooDeep。
    // 実測（2026-08-30）: deepTreeJSON(255) → nodeDepth 256 ok / deepTreeJSON(256) → tooDeep。
    // 拒否するのは JSONSerialization であり、maxNodeDepth はそれと同じ「含む上限」である。
    @Test("depth maxNodeDepth decodes; maxNodeDepth+1 is rejected as tooDeep")
    func atLimit() throws {
        let ok = try Node.decode(deepTreeJSON(depth: maxNodeDepth - 1))
        #expect(depth(of: ok) == maxNodeDepth)
        #expect(throws: NodeDecodingError.tooDeep) {
            _ = try Node.decode(deepTreeJSON(depth: maxNodeDepth))
        }
    }

    // TS-204 の本体: 512 KB スタックのスレッド（別スレッドの既定）で上限ちょうどまで復号しても溢れない。
    // 再帰版はここで深さ 229 で SIGBUS した。反復版はスタック消費が深さに依存しない。
    // continuation を Thread から直接 resume すると strict concurrency で落ちるため、
    // 結果はセマフォで受け渡す。
    @Test("maxNodeDepth is safe on a 512 KB stack thread")
    func atLimitOnSmallStack() throws {
        final class Box: @unchecked Sendable { var result: Result<Int, Error>? }
        let box = Box()
        let done = DispatchSemaphore(value: 0)
        let t = Thread {
            box.result = Result { depth(of: try Node.decode(deepTreeJSON(depth: maxNodeDepth - 1))) }
            done.signal()
        }
        t.stackSize = 512 * 1024
        t.start()
        #expect(done.wait(timeout: .now() + 10) == .success, "decode thread did not finish (crash or hang)")
        #expect(try box.result?.get() == maxNodeDepth)
    }

    // TS-202: 巨大な文字列
    @Test("10 MB text decodes")
    func hugeString() throws {
        let big = String(repeating: "x", count: 10 << 20)
        let json = #"{"id":"0","type":"Text","props":{"text":"\#(big)"}}"#
        let n = try Node.decode(Data(json.utf8))
        #expect(n.text.count == 10 << 20)
    }

    // TS-203: 空ツリー
    @Test("empty VStack")
    func emptyTree() throws {
        let n = try Node.decode(Data(#"{"id":"0","type":"VStack","children":[]}"#.utf8))
        #expect(n.children.isEmpty)
        #expect(n.count == 1)
    }

    // TS-107: children が null
    @Test("null children is treated as empty")
    func nullChildren() throws {
        let n = try Node.decode(Data(#"{"id":"0","type":"VStack","children":null}"#.utf8))
        #expect(n.children.isEmpty)
    }

    @Test("missing props / mods / children keys are treated as empty")
    func omittedKeys() throws {
        let n = try Node.decode(Data(#"{"id":"0","type":"Spacer"}"#.utf8))
        #expect(n.props.isEmpty && n.mods.isEmpty && n.children.isEmpty)
    }

    @Test("missing id or type throws, does not crash")
    func missingRequired() {
        #expect(throws: NodeDecodingError.missingID) {
            _ = try Node.decode(Data(#"{"type":"Text"}"#.utf8))
        }
        #expect(throws: NodeDecodingError.missingType) {
            _ = try Node.decode(Data(#"{"id":"0"}"#.utf8))
        }
    }
}

func collect(_ n: Node) -> [Node] { [n] + n.children.flatMap(collect) }
func depth(of n: Node) -> Int { 1 + (n.children.map(depth(of:)).max() ?? 0) }
