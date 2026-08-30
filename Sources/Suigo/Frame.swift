import Foundation

/// Go → ホストのフレーム。docs/wire-format.md §2.1。
public enum IncomingFrame: Sendable {
    case render(seq: UInt64, tree: Node)
    case error(severity: String, code: String, message: String)
    case snapshotResult(model: Data?, error: String?)
    /// 未知の type は無視してログに出す（§2.3）。プロトコルに版番号を持たない根拠。
    case unknown(type: String)

    public static func decode(_ data: Data) throws -> IncomingFrame {
        let raw = try Node.parseJSON(data)
        guard let dict = raw as? [String: Any], let type = dict["type"] as? String else {
            throw FrameDecodingError.missingType
        }
        switch type {
        case "render":
            guard let tree = dict["tree"] else { throw FrameDecodingError.missingField("tree") }
            let seq = (dict["seq"] as? NSNumber)?.uint64Value ?? 0
            return .render(seq: seq, tree: try Node.decode(any: tree, depth: 0))
        case "error":
            return .error(
                severity: dict["severity"] as? String ?? "runtime",
                code: dict["code"] as? String ?? "",
                message: dict["message"] as? String ?? "")
        case "snapshot_result":
            var model: Data? = nil
            if let m = dict["model"] { model = try? JSONSerialization.data(withJSONObject: m) }
            return .snapshotResult(model: model, error: dict["error"] as? String)
        default:
            return .unknown(type: type)
        }
    }
}

/// ホスト → Go のフレーム。docs/wire-format.md §2.2。
public enum OutgoingFrame: Sendable {
    case mount(restore: Data?)
    case event(nodeID: String, kind: String, payload: EventPayload)
    case env(colorScheme: String, safeArea: SafeArea, sizeClass: SizeClass)
    case snapshot

    public func encode() throws -> Data {
        let obj: [String: Any]
        switch self {
        case .mount(let restore):
            var d: [String: Any] = ["type": "mount"]
            if let r = restore, let o = try? JSONSerialization.jsonObject(with: r) { d["restore"] = o }
            obj = d
        case .event(let nodeID, let kind, let payload):
            obj = ["type": "event", "nodeID": nodeID, "kind": kind, "payload": payload.json]
        case .env(let cs, let sa, let sc):
            obj = ["type": "env", "colorScheme": cs,
                   "safeArea": ["top": sa.top, "bottom": sa.bottom, "leading": sa.leading, "trailing": sa.trailing],
                   "sizeClass": ["h": sc.h, "v": sc.v]]
        case .snapshot:
            obj = ["type": "snapshot"]
        }
        return try JSONSerialization.data(withJSONObject: obj, options: [.sortedKeys])
    }
}

/// 閉じたタグ付き共用体。docs/wire-format.md §5。
public enum EventPayload: Sendable, Equatable {
    case none
    case string(String)
    case bool(Bool)
    case int(Int)
    case double(Double)
    case index(Int)

    var json: [String: Any] {
        switch self {
        case .none: ["t": "none"]
        case .string(let s): ["t": "string", "v": s]
        case .bool(let b): ["t": "bool", "v": b]
        case .int(let i): ["t": "int", "v": i]
        case .double(let d): ["t": "double", "v": d]
        case .index(let i): ["t": "index", "v": i]
        }
    }
}

public struct SafeArea: Sendable, Equatable {
    public var top, bottom, leading, trailing: Double
    public init(top: Double = 0, bottom: Double = 0, leading: Double = 0, trailing: Double = 0) {
        self.top = top; self.bottom = bottom; self.leading = leading; self.trailing = trailing
    }
}

public struct SizeClass: Sendable, Equatable {
    public var h, v: String
    public init(h: String, v: String) { self.h = h; self.v = v }
}

public enum FrameDecodingError: Error, Equatable {
    case missingType
    case missingField(String)
}
