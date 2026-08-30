import Foundation
import Testing
@testable import Suigo

@Suite("Frame decoding / encoding")
struct FrameTests {

    @Test("render frame carries seq")
    func renderSeq() throws {
        guard case .render(let seq, let tree) = try IncomingFrame.decode(try Fixtures.load("golden/render_todo3.json")) else {
            Issue.record("not a render frame"); return
        }
        #expect(seq == 7)
        #expect(tree.type == "VStack")
    }

    @Test("all 6 error codes decode with matching severity")
    func errorCodes() throws {
        let raw = try JSONSerialization.jsonObject(with: try Fixtures.load("golden/errors.json")) as! [String: Any]
        let frames = raw["frames"] as! [[String: Any]]
        #expect(frames.count == 6)
        var seen: [String: String] = [:]
        for f in frames {
            let data = try JSONSerialization.data(withJSONObject: f)
            guard case .error(let sev, let code, let msg) = try IncomingFrame.decode(data) else {
                Issue.record("not an error frame"); continue
            }
            #expect(!msg.isEmpty)
            seen[code] = sev
        }
        #expect(seen == ["build_failed": "build", "duplicate_id": "runtime", "invalid_id": "runtime",
                         "handler_panic": "runtime", "node_count_warning": "warning", "restore_failed": "warning"])
    }

    // §2.3: 未知のフレーム型は無視される（クラッシュも例外も無し）
    @Test("unknown frame type is reported, not thrown")
    func unknownFrame() throws {
        guard case .unknown(let type) = try IncomingFrame.decode(try Fixtures.load("invalid/unknown_frame.json")) else {
            Issue.record("expected .unknown"); return
        }
        #expect(type == "telepathy")
    }

    @Test("event payloads round-trip against golden (all 6 tags)")
    func eventsGolden() throws {
        let raw = try JSONSerialization.jsonObject(with: try Fixtures.load("golden/events.json")) as! [String: Any]
        let frames = raw["frames"] as! [[String: Any]]
        #expect(frames.count == 6)

        let payloads: [EventPayload] = [.none, .string("入力した値"), .bool(true), .int(3), .double(1.5), .index(2)]
        for (f, p) in zip(frames, payloads) {
            let encoded = try OutgoingFrame.event(nodeID: f["nodeID"] as! String, kind: f["kind"] as! String, payload: p).encode()
            let got = try JSONSerialization.jsonObject(with: encoded) as! NSDictionary
            let want = f as NSDictionary
            #expect(got == want, "payload \(p)")
        }
    }

    @Test("env frame matches golden structure")
    func envGolden() throws {
        let encoded = try OutgoingFrame.env(colorScheme: "dark",
                                            safeArea: SafeArea(top: 59, bottom: 34),
                                            sizeClass: SizeClass(h: "compact", v: "regular")).encode()
        let got = try JSONSerialization.jsonObject(with: encoded) as! NSDictionary
        let want = try JSONSerialization.jsonObject(with: try Fixtures.load("golden/env.json")) as! NSDictionary
        #expect(got == want)
    }

    @Test("mount and snapshot encode minimal shapes")
    func mountSnapshot() throws {
        let m = try JSONSerialization.jsonObject(with: try OutgoingFrame.mount(restore: nil).encode()) as! NSDictionary
        #expect(m == ["type": "mount"])
        let r = try JSONSerialization.jsonObject(with: try OutgoingFrame.mount(restore: Data(#"{"Count":3}"#.utf8)).encode()) as! NSDictionary
        #expect(r == ["type": "mount", "restore": ["Count": 3]])
        let s = try JSONSerialization.jsonObject(with: try OutgoingFrame.snapshot.encode()) as! NSDictionary
        #expect(s == ["type": "snapshot"])
    }

    @Test("snapshot_result carries model or error")
    func snapshotResult() throws {
        guard case .snapshotResult(let model, let err) = try IncomingFrame.decode(Data(#"{"type":"snapshot_result","model":{"Count":3}}"#.utf8)) else {
            Issue.record("wrong case"); return
        }
        #expect(err == nil)
        #expect(try JSONSerialization.jsonObject(with: try #require(model)) as! NSDictionary == ["Count": 3])

        guard case .snapshotResult(let m2, let e2) = try IncomingFrame.decode(Data(#"{"type":"snapshot_result","error":"boom"}"#.utf8)) else {
            Issue.record("wrong case"); return
        }
        #expect(m2 == nil && e2 == "boom")
    }

    @Test("broken JSON throws, does not crash (TS-106)")
    func brokenJSON() {
        #expect(throws: (any Error).self) {
            _ = try IncomingFrame.decode(Data(#"{"type":"render","seq":1,"tree":{"id":"0""#.utf8))
        }
    }
}
