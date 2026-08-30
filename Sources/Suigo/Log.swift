import os

/// suigo のログ。subsystem `dev.suigo`、カテゴリは interpreter / transport / runtime。
enum SuigoLog {
    static let interpreter = Logger(subsystem: "dev.suigo", category: "interpreter")
    static let transport = Logger(subsystem: "dev.suigo", category: "transport")
    static let runtime = Logger(subsystem: "dev.suigo", category: "runtime")

    static func unknownNode(_ type: String, id: String) {
        interpreter.warning("unknown node type \(type, privacy: .public) at \(id, privacy: .public); rendering EmptyView")
    }

    static func unknownModifier(_ k: String, id: String) {
        interpreter.warning("unknown modifier \(k, privacy: .public) at \(id, privacy: .public); ignored")
    }

    static func badValue(_ what: String, _ value: String, id: String) {
        interpreter.warning("unrecognized \(what, privacy: .public) \(value, privacy: .public) at \(id, privacy: .public); ignored")
    }
}
