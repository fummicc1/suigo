import SwiftUI

/// 修飾子を配列順に適用する。順序に意味がある（docs/wire-format.md §4）ため、
/// ソートも正規化もしない。未知の `k` は無視してログに出し、残りの適用は続ける（FR-4.3）。
///
/// `spacing` はコンテナ生成時に読むため、ここでは扱わない。
@MainActor
func applyModifiers(_ mods: [Modifier], to view: AnyView, nodeID: String) -> AnyView {
    var v = view
    for m in mods {
        switch m.k {
        case "padding":
            if let d = m.v.double {
                v = AnyView(v.padding(CGFloat(d)))
            } else if let o = m.v.object {
                v = AnyView(v.padding(EdgeInsets(
                    top: CGFloat(o["top"]?.double ?? 0),
                    leading: CGFloat(o["leading"]?.double ?? 0),
                    bottom: CGFloat(o["bottom"]?.double ?? 0),
                    trailing: CGFloat(o["trailing"]?.double ?? 0))))
            } else {
                SuigoLog.badValue("padding", "\(m.v)", id: nodeID)
            }
        case "font":
            if let f = font(m.v.string ?? "") { v = AnyView(v.font(f)) }
            else { SuigoLog.badValue("font", m.v.string ?? "", id: nodeID) }
        case "foregroundColor":
            if let c = color(m.v.string ?? "") { v = AnyView(v.foregroundStyle(c)) }
            else { SuigoLog.badValue("color", m.v.string ?? "", id: nodeID) }
        case "background":
            if let c = color(m.v.string ?? "") { v = AnyView(v.background(c)) }
            else { SuigoLog.badValue("color", m.v.string ?? "", id: nodeID) }
        case "frame":
            if let o = m.v.object {
                v = AnyView(v.frame(
                    width: o["width"]?.double.map { CGFloat($0) },
                    height: o["height"]?.double.map { CGFloat($0) }))
                let maxW = dimension(o["maxWidth"])
                let maxH = dimension(o["maxHeight"])
                if maxW != nil || maxH != nil {
                    v = AnyView(v.frame(maxWidth: maxW, maxHeight: maxH,
                                        alignment: alignment(o["alignment"]?.string ?? "center")))
                }
            }
        case "cornerRadius":
            if let r = m.v.double { v = AnyView(v.clipShape(RoundedRectangle(cornerRadius: CGFloat(r)))) }
        case "spacing":
            break
        case "disabled":
            v = AnyView(v.disabled(m.v.bool ?? false))
        case "hidden":
            if m.v.bool == true { v = AnyView(v.hidden()) }
        default:
            SuigoLog.unknownModifier(m.k, id: nodeID)
        }
    }
    return v
}

/// `maxWidth` / `maxHeight` は数値、または `"infinity"`（docs/wire-format.md §4.3）。
private func dimension(_ p: PropValue?) -> CGFloat? {
    guard let p else { return nil }
    if let d = p.double { return CGFloat(d) }
    if p.string == "infinity" { return .infinity }
    return nil
}

/// docs/wire-format.md §4.1
func font(_ s: String) -> Font? {
    switch s {
    case "largeTitle": .largeTitle
    case "title": .title
    case "title2": .title2
    case "title3": .title3
    case "headline": .headline
    case "subheadline": .subheadline
    case "body": .body
    case "callout": .callout
    case "footnote": .footnote
    case "caption": .caption
    case "caption2": .caption2
    default: nil
    }
}

/// docs/wire-format.md §4.2。名前つき、または `#RRGGBB`。
func color(_ s: String) -> Color? {
    switch s {
    case "primary": return .primary
    case "secondary": return .secondary
    case "red": return .red
    case "orange": return .orange
    case "yellow": return .yellow
    case "green": return .green
    case "mint": return .mint
    case "teal": return .teal
    case "cyan": return .cyan
    case "blue": return .blue
    case "indigo": return .indigo
    case "purple": return .purple
    case "pink": return .pink
    case "brown": return .brown
    case "gray": return .gray
    case "black": return .black
    case "white": return .white
    case "clear": return .clear
    default: return hexColor(s)
    }
}

private func hexColor(_ s: String) -> Color? {
    guard s.hasPrefix("#"), s.count == 7, let v = UInt32(s.dropFirst(), radix: 16) else { return nil }
    return Color(red: Double((v >> 16) & 0xFF) / 255,
                 green: Double((v >> 8) & 0xFF) / 255,
                 blue: Double(v & 0xFF) / 255)
}
