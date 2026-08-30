import Foundation
import Testing
@testable import Suigo

@Suite("SuigoRuntime")
@MainActor
struct RuntimeTests {

    private func render(seq: Int, text: String) -> Data {
        Data(#"{"type":"render","seq":\#(seq),"tree":{"id":"0","type":"Text","props":{"text":"\#(text)"}}}"#.utf8)
    }

    // TS-301: 古い seq を捨てる
    @Test("stale seq is dropped")
    func staleSeq() {
        let rt = SuigoRuntime(transport: MockTransport())
        rt.handle(render(seq: 5, text: "five"))
        rt.handle(render(seq: 3, text: "three"))
        #expect(rt.tree?.text == "five")
        #expect(rt.lastSeq == 5)
    }

    // TS-302: 高頻度到着では最新だけが残る
    @Test("burst of frames keeps only the latest")
    func burst() {
        let rt = SuigoRuntime(transport: MockTransport())
        for i in 1...10 { rt.handle(render(seq: i, text: "\(i)")) }
        #expect(rt.tree?.text == "10")
        #expect(rt.lastSeq == 10)
    }

    @Test("equal seq is also dropped (no re-render for duplicates)")
    func equalSeq() {
        let rt = SuigoRuntime(transport: MockTransport())
        rt.handle(render(seq: 1, text: "a"))
        rt.handle(render(seq: 1, text: "b"))
        #expect(rt.tree?.text == "a")
    }

    // TS-106 + design §5.1: 壊れたフレームは直前のツリーを維持
    @Test("broken frame keeps previous tree")
    func brokenKeepsPrevious() {
        let rt = SuigoRuntime(transport: MockTransport())
        rt.handle(render(seq: 1, text: "good"))
        rt.handle(Data(#"{"type":"render","seq":2,"tree":{"id":"0""#.utf8))
        #expect(rt.tree?.text == "good")
        #expect(rt.lastSeq == 1)
    }

    @Test("error frame is surfaced and cleared by the next render")
    func errorThenRender() {
        let rt = SuigoRuntime(transport: MockTransport())
        rt.handle(render(seq: 1, text: "a"))
        rt.handle(Data(#"{"type":"error","severity":"build","code":"build_failed","message":"boom"}"#.utf8))
        #expect(rt.lastError?.code == "build_failed")
        #expect(rt.tree?.text == "a", "error must not clear the tree")
        rt.handle(render(seq: 2, text: "b"))
        #expect(rt.lastError == nil)
        #expect(rt.tree?.text == "b")
    }

    // §2.3: 未知フレーム型は無視される
    @Test("unknown frame type is ignored")
    func unknownFrame() throws {
        let rt = SuigoRuntime(transport: MockTransport())
        rt.handle(render(seq: 1, text: "a"))
        rt.handle(try Fixtures.load("invalid/unknown_frame.json"))
        #expect(rt.tree?.text == "a")
        #expect(rt.lastError == nil)
    }

    @Test("start sends mount, then frames flow from the transport")
    func startAndReceive() async throws {
        let mock = MockTransport()
        let rt = SuigoRuntime(transport: mock)
        rt.start()
        try await Task.sleep(for: .milliseconds(50))
        #expect(mock.sent.contains { $0["type"] as? String == "mount" })

        mock.inject(render(seq: 1, text: "from go"))
        try await Task.sleep(for: .milliseconds(50))
        #expect(rt.tree?.text == "from go")
        rt.stop()
    }
}
