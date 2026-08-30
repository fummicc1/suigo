import Foundation

/// ワイヤ上のノード。docs/wire-format.md §3 に従う。
///
/// `props` は `(type, key)` で型が決まる閉じた集合なので、辞書として舐めずに
/// ノード型ごとの取り出し関数を通す（`text`, `label` など）。
///
/// `Equatable` は性能に直結する。SwiftUI は view の格納プロパティを比較して body の再評価を
/// 省くが、比較できない型は「変わったかもしれない」と扱う。`Node` が Equatable でないと、
/// 子を持つ行は親が再評価されるたびに全部再評価される（TP-004 で 511 ノードのツリーの
/// 1 か所変更が 3 body ではなく 172 body になった）。view 側の `.equatable()` は不要だが、
/// 型側の適合は必要である。
public struct Node: Sendable, Equatable {
    public let id: String
    public let type: String
    public let props: [String: PropValue]
    public let mods: [Modifier]
    public let children: [Node]

    public init(id: String, type: String, props: [String: PropValue] = [:],
                mods: [Modifier] = [], children: [Node] = []) {
        self.id = id
        self.type = type
        self.props = props
        self.mods = mods
        self.children = children
    }
}

/// 修飾子。配列の順序に意味がある。
public struct Modifier: Sendable, Equatable {
    public let k: String
    public let v: PropValue
}

/// `props` と `mods.v` に現れる JSON 値。生の値であり、タグは持たない。
public enum PropValue: Sendable, Equatable {
    case string(String)
    case number(Double)
    case bool(Bool)
    case object([String: PropValue])
    case null

    public var string: String? { if case .string(let s) = self { s } else { nil } }
    public var double: Double? { if case .number(let d) = self { d } else { nil } }
    public var bool: Bool? { if case .bool(let b) = self { b } else { nil } }
    public var object: [String: PropValue]? { if case .object(let o) = self { o } else { nil } }
}

/// 復号の深さ上限。docs/wire-format.md §7。
///
/// 実測（2026-08-30, iOS 26.5 Simulator）: JSONSerialization 自体がノード深さ 256 で
/// "Too many nested arrays or dictionaries" を投げる（ノード 1 段が {"children":[...]} で
/// 2 段を消費するため）。この再帰デコーダは 512 KB スタックでもその手前で溢れない。
/// つまり実効的な上限は Foundation 側にあり、この定数はそれと同じ値である。
/// 上限超過は `NodeDecodingError.tooDeep` に正規化して呼び出し側へ返す。
public let maxNodeDepth = 256

public enum NodeDecodingError: Error, Equatable {
    case tooDeep
    case missingID
    case missingType
}

// MARK: - 復号

extension Node {
    /// JSON からノードを復号する。深さ上限を超えたら `tooDeep` を投げる。
    public static func decode(_ data: Data) throws -> Node {
        let raw = try parseJSON(data)
        return try decode(any: raw, depth: 0)
    }

    /// JSONSerialization のネスト上限エラー（NSCocoaErrorDomain 3840 "Too many nested"）を
    /// `tooDeep` に正規化する。呼び出し側が Foundation の内部表現に依存しないようにするため。
    static func parseJSON(_ data: Data) throws -> Any {
        do {
            return try JSONSerialization.jsonObject(with: data, options: [])
        } catch let e as NSError where e.domain == NSCocoaErrorDomain && e.code == 3840
            && (e.userInfo[NSDebugDescriptionErrorKey] as? String)?.contains("Too many nested") == true {
            throw NodeDecodingError.tooDeep
        }
    }

    /// 再帰ではなく明示的なスタックで復号する。
    ///
    /// 再帰版は -Onone × 512 KB スレッドで深さ 229 のときにスタック溢れした（実測 2026-08-30、
    /// クラッシュレポートは `Node.decode(any:depth:)` の連続フレームと SIGBUS を示していた）。
    /// 1 段あたり約 2 KB 消費し、デバッグビルドの worker thread では maxNodeDepth に届かない。
    /// 反復に直せばスタック消費は深さに依存せず、上限は JSONSerialization 側の 256 だけになる。
    static func decode(any raw: Any, depth startDepth: Int) throws -> Node {
        // 各フレームは「復号中の辞書」と「復号済みの子」を持ち、子が全部揃ったら親へ畳む。
        struct Frame {
            let id: String
            let type: String
            let props: [String: PropValue]
            let mods: [Modifier]
            let pendingChildren: [Any]
            var nextChild = 0
            var doneChildren: [Node] = []
        }

        func open(_ raw: Any, depth: Int) throws -> Frame {
            guard depth < maxNodeDepth else { throw NodeDecodingError.tooDeep }
            guard let dict = raw as? [String: Any] else { throw NodeDecodingError.missingType }
            guard let id = dict["id"] as? String else { throw NodeDecodingError.missingID }
            guard let type = dict["type"] as? String else { throw NodeDecodingError.missingType }

            var props: [String: PropValue] = [:]
            if let p = dict["props"] as? [String: Any] {
                for (k, v) in p { props[k] = PropValue(any: v) }
            }
            var mods: [Modifier] = []
            if let ms = dict["mods"] as? [[String: Any]] {
                mods.reserveCapacity(ms.count)
                for m in ms {
                    guard let k = m["k"] as? String else { continue }
                    mods.append(Modifier(k: k, v: PropValue(any: m["v"] ?? NSNull())))
                }
            }
            let children = dict["children"] as? [Any] ?? []
            var f = Frame(id: id, type: type, props: props, mods: mods, pendingChildren: children)
            f.doneChildren.reserveCapacity(children.count)
            return f
        }

        var stack: [Frame] = [try open(raw, depth: startDepth)]
        while true {
            let top = stack.count - 1
            if stack[top].nextChild < stack[top].pendingChildren.count {
                let childRaw = stack[top].pendingChildren[stack[top].nextChild]
                stack[top].nextChild += 1
                stack.append(try open(childRaw, depth: startDepth + stack.count))
                continue
            }
            let f = stack.removeLast()
            let node = Node(id: f.id, type: f.type, props: f.props, mods: f.mods, children: f.doneChildren)
            if stack.isEmpty { return node }
            stack[stack.count - 1].doneChildren.append(node)
        }
    }
}

extension PropValue {
    init(any v: Any) {
        switch v {
        case let s as String: self = .string(s)
        case let n as NSNumber:
            // JSONSerialization は true/false も NSNumber にする。CFBoolean かどうかで見分ける。
            if CFGetTypeID(n) == CFBooleanGetTypeID() { self = .bool(n.boolValue) }
            else { self = .number(n.doubleValue) }
        case let o as [String: Any]:
            var out: [String: PropValue] = [:]
            for (k, x) in o { out[k] = PropValue(any: x) }
            self = .object(out)
        default: self = .null
        }
    }
}

// MARK: - 型付きの取り出し（props を辞書として舐めない）

extension Node {
    public var text: String { props["text"]?.string ?? "" }
    public var label: String { props["label"]?.string ?? "" }
    public var value: String { props["value"]?.string ?? "" }
    public var isOn: Bool { props["on"]?.bool ?? false }
    public var symbol: String? { props["symbol"]?.string }
    public var asset: String? { props["asset"]?.string }
    public var progress: Double? { props["value"]?.double }
    public var alignment: String { props["alignment"]?.string ?? "center" }
    public var isHorizontal: Bool { props["axis"]?.string == "h" }
    public var commitPolicy: String { props["commit"]?.string ?? "submit" }

    /// 総ノード数。テストと警告用。
    public var count: Int { 1 + children.reduce(0) { $0 + $1.count } }
}
