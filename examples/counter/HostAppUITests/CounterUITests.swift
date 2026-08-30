import XCTest

/// TE-001 / M2: 本物の SwiftUI Button をタップし、Go 側で状態が変わり、画面が更新されることを検証する。
///
/// 前提: `suigo dev -port 7391 ./examples/counter/dev` が起動しており、
/// ホストアプリ（dev.suigo.example.counter）がインストール済みであること。
/// このテストは GUI 自動化ツール（idb 等）に依存せず、XCUITest だけで動く。
final class CounterUITests: XCTestCase {

    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication(bundleIdentifier: "dev.suigo.example.counter")
        // SUIGO_TEST_TRANSPORT=dev なら suigo dev（:7391）に接続、それ以外は embedded（Go を c-archive でリンク）。
        // 同じテストを両経路で走らせ、AR-1（インタープリタはトランスポートを知らない）を実証する。
        if ProcessInfo.processInfo.environment["SUIGO_TEST_TRANSPORT"] == "dev" {
            app.launchEnvironment["SUIGO_DEV_PORT"] = "7391"
        }
        app.launch()
    }

    override func tearDown() {
        app.terminate()
    }

    private func counterValue(timeout: TimeInterval = 5) -> String? {
        // 大きな青い数字。Go 側では VStack の 2 番目の Text。
        let texts = app.staticTexts.allElementsBoundByIndex
        for t in texts where Int(t.label) != nil { return t.label }
        return nil
    }

    private func waitForCounter(_ expected: String, timeout: TimeInterval = 5) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if counterValue() == expected { return true }
            RunLoop.current.run(until: Date().addingTimeInterval(0.1))
        }
        return false
    }

    /// M2 の核心: + を押すと Go の状態が増え、SwiftUI が更新される。
    func testPlusIncrementsViaGo() {
        XCTAssertTrue(app.staticTexts["suigo counter"].waitForExistence(timeout: 10),
                      "Go 由来の UI が表示されること（M1 の前提）")
        let plus = app.buttons["+"]
        XCTAssertTrue(plus.waitForExistence(timeout: 5))

        guard let before = counterValue(), let b = Int(before) else {
            XCTFail("counter text not found"); return
        }
        plus.tap()
        XCTAssertTrue(waitForCounter(String(b + 1)),
                      "tap → event → Go Update → render → SwiftUI 再描画 の往復が成立すること")
    }

    /// − と Reset も Go 側のハンドラに届く。Reset は Count==0 のとき disabled になる（修飾子の往復）。
    func testMinusAndResetRoundTrip() {
        XCTAssertTrue(app.buttons["+"].waitForExistence(timeout: 10))
        app.buttons["+"].tap()
        app.buttons["+"].tap()
        guard let v = counterValue(), let n = Int(v) else { XCTFail(); return }
        XCTAssertGreaterThanOrEqual(n, 2)

        app.buttons["−"].tap()
        XCTAssertTrue(waitForCounter(String(n - 1)))

        let reset = app.buttons["Reset"]
        XCTAssertTrue(reset.isEnabled, "Count != 0 なら Reset は有効")
        reset.tap()
        XCTAssertTrue(waitForCounter("0"))
        // disabled 修飾子が Go から届いて適用されている
        XCTAssertTrue(waitUntil(timeout: 3) { !reset.isEnabled }, "Count == 0 なら Reset は disabled")
    }

    private func waitUntil(timeout: TimeInterval, _ cond: () -> Bool) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if cond() { return true }
            RunLoop.current.run(until: Date().addingTimeInterval(0.1))
        }
        return cond()
    }
}
