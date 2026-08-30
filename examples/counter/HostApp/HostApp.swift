import SwiftUI
import Suigo
import SuigoEmbedded

@main
struct HostApp: App {
    var body: some Scene {
        WindowGroup {
            // SUIGO_DEV_PORT があれば dev（DEBUG のみ）、無ければ embedded（Go を c-archive でリンク）。
            SuigoView.standard()
        }
    }
}
