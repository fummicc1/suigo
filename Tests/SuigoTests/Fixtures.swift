import Foundation
@testable import Suigo

/// `testdata/wire/` のフィクスチャ。Go 側と同じファイルを参照する。
enum Fixtures {
    static func load(_ relative: String) throws -> Data {
        let url = Bundle.module.bundleURL.appendingPathComponent("wire").appendingPathComponent(relative)
        guard FileManager.default.fileExists(atPath: url.path) else {
            throw FixtureError.notFound(url.path)
        }
        return try Data(contentsOf: url)
    }

    static func render(_ name: String) throws -> Node {
        guard case .render(_, let tree) = try IncomingFrame.decode(try load("golden/\(name).json")) else {
            throw FixtureError.notRender(name)
        }
        return tree
    }

    enum FixtureError: Error { case notFound(String), notRender(String) }
}

/// 深さ `depth` の VStack 入れ子を JSON として作る。
func deepTreeJSON(depth: Int) -> Data {
    var s = ""
    for i in 0..<depth {
        let id = i == 0 ? "0" : "0" + String(repeating: "/0", count: i)
        s += #"{"id":"\#(id)","type":"VStack","children":["#
    }
    s += #"{"id":"leaf","type":"Text","props":{"text":"bottom"}}"#
    s += String(repeating: "]}", count: depth)
    return Data(s.utf8)
}
