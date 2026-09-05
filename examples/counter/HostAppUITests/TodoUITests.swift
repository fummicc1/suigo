import XCTest

/// TE-002 / M5: TODO サンプル。行の同一性（List + ForEach 明示 ID）と入力の確定（TextField）を検証する。
///
/// 前提: `suigo dev -port 7392 ./examples/todo/dev` が起動していること。
/// ホストアプリは SUIGO_DEV_PORT で接続先を切り替えるだけなので、counter と同じバイナリを使う。
///
/// 注意: キーボードが出ている間は List が縮み、画面外の行が accessibility 階層から消える
/// （診断で `cells=1` を観測）。行の存在を確かめる前に必ずキーボードを閉じる。
final class TodoUITests: XCTestCase {

    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication(bundleIdentifier: "dev.suigo.example.counter")
        app.launchEnvironment["SUIGO_DEV_PORT"] = "7392"
        app.launch()
    }

    override func tearDown() { app.terminate() }

    private var field: XCUIElement { app.textFields["新しいタスク"] }
    private var addButton: XCUIElement { app.buttons["追加"] }

    /// 日本語入力では 1 回目の return が変換確定になりうるため、キーボードが消えるまで最大 3 回押す。
    private func submitAndDismissKeyboard(_ element: XCUIElement? = nil) {
        let target = element ?? field
        for _ in 0..<3 where app.keyboards.count > 0 {
            app.keyboards.buttons["return"].firstMatch.exists
                ? app.keyboards.buttons["return"].firstMatch.tap()
                : target.typeText("\n")
            RunLoop.current.run(until: Date().addingTimeInterval(0.4))
        }
        XCTAssertEqual(app.keyboards.count, 0, "キーボードが閉じていること（行の存在確認の前提）")
    }

    /// 前のテストで commit した値が Go から届いて残っていることがあるので、入力前に空にする。
    /// 空のときは placeholder がそのまま value に入るため、それも除外する。
    private func clear(_ element: XCUIElement, placeholder: String) {
        element.tap()
        if let v = element.value as? String, !v.isEmpty, v != placeholder {
            element.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: v.count))
        }
    }

    private func clearField() { clear(field, placeholder: "新しいタスク") }

    private func add(_ title: String) {
        clearField()
        field.typeText(title)
        submitAndDismissKeyboard()
        XCTAssertTrue(waitUntil(3) { self.addButton.isEnabled }, "commit 後に追加が有効になる（Go の Draft が更新された）")
        addButton.tap()
    }

    private func waitUntil(_ timeout: TimeInterval, _ cond: () -> Bool) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if cond() { return true }
            RunLoop.current.run(until: Date().addingTimeInterval(0.1))
        }
        return cond()
    }

    /// 既存の行数に依存しないよう、テスト開始時点の件数を基準にする（suigo dev の状態は前のテストから残る）。
    private func itemCount() -> Int {
        // フッター "d / n 件完了" の n
        for t in app.staticTexts.allElementsBoundByIndex {
            let parts = t.label.components(separatedBy: " ")
            if parts.count == 4, parts[3] == "件完了", let n = Int(parts[2]) { return n }
        }
        return -1
    }

    /// AC-2: 追加・トグル・削除が Go を往復して反映される。
    func testAddToggleDelete() {
        XCTAssertTrue(field.waitForExistence(timeout: 10), "TODO の UI が表示されること")
        let base = itemCount()
        XCTAssertGreaterThanOrEqual(base, 0, "フッターから件数を読めること")
        let a = "行A-\(Int.random(in: 100...999))"
        let b = "行B-\(Int.random(in: 100...999))"

        add(a)
        XCTAssertTrue(waitUntil(5) { self.itemCount() == base + 1 }, "追加が Go に届き件数が +1 になる")
        XCTAssertTrue(app.switches[a].waitForExistence(timeout: 5), "行が List に現れる")

        add(b)
        XCTAssertTrue(app.switches[b].waitForExistence(timeout: 5))

        // Toggle → Go の Done が true → 完了数が +1
        let doneBefore = doneCount()
        tapToggle(a)
        XCTAssertTrue(waitUntil(5) { self.doneCount() == doneBefore + 1 }, "toggle が Go に届き集計が更新される")

        // 削除 → 行が消え、残った行は同じ ID のまま（行の同一性）
        let rowA = app.cells.containing(.switch, identifier: a).firstMatch
        XCTAssertTrue(rowA.exists)
        rowA.buttons["削除"].firstMatch.tap()
        XCTAssertTrue(waitUntil(5) { !self.app.switches[a].exists }, "削除した行が消える")
        XCTAssertTrue(app.switches[b].exists, "残りの行は保たれる")
        XCTAssertTrue(waitUntil(5) { self.itemCount() == base + 1 }, "件数が -1 になる")
    }

    /// SwiftUI の Toggle は「ラベル付きの外側 Switch」の中に「実際のスイッチ（内側 Switch）」を持つ。
    /// 外側の中心はラベル文字の上で、そこを叩いても切り替わらない。内側を叩く。
    private func tapToggle(_ label: String) {
        let outer = app.switches[label].firstMatch
        XCTAssertTrue(outer.waitForExistence(timeout: 5))
        let inner = outer.switches.firstMatch
        (inner.exists ? inner : outer).tap()
    }

    private func doneCount() -> Int {
        for t in app.staticTexts.allElementsBoundByIndex {
            let parts = t.label.components(separatedBy: " ")
            if parts.count == 4, parts[3] == "件完了", let d = Int(parts[0]) { return d }
        }
        return -1
    }

    /// TS-401 の UI 側: 入力中は Go の Draft が更新されない（= 打鍵ごとに送っていない）。
    /// 打鍵ごとに commit していれば、途中で追加が有効になるはず。
    func testTypingDoesNotCommitPerKeystroke() {
        XCTAssertTrue(field.waitForExistence(timeout: 10))
        // 前のテストの Draft が残っていることがあるので、まず空で commit して disabled にする
        clearField()
        submitAndDismissKeyboard()
        XCTAssertTrue(waitUntil(3) { !self.addButton.isEnabled }, "空の Draft では追加は disabled")

        field.tap()
        field.typeText("abc")
        RunLoop.current.run(until: Date().addingTimeInterval(0.8))
        XCTAssertFalse(addButton.isEnabled, "入力中（未 commit）は Go に届かず、追加は disabled のまま（TS-401）")

        submitAndDismissKeyboard()
        XCTAssertTrue(waitUntil(3) { self.addButton.isEnabled }, "submit で commit され、追加が有効になる")
    }

    /// TS-404: debounce ポリシーの TextField は return 無しで、入力が止まってから commit される。
    /// 絞り込み欄（`CommitDebounce(300)`）に固有トークンを打ち、該当行だけが残ることで Go 到達を確かめる。
    func testDebounceFilterCommitsWithoutSubmit() {
        XCTAssertTrue(field.waitForExistence(timeout: 10))
        let token = "F\(Int.random(in: 1000...9999))"
        let a = "牛乳-\(token)"
        let b = "本-\(Int.random(in: 100...999))"
        add(a)
        add(b)
        XCTAssertTrue(app.switches[a].waitForExistence(timeout: 5))
        XCTAssertTrue(app.switches[b].waitForExistence(timeout: 5))

        let filter = app.textFields["絞り込み"]
        XCTAssertTrue(filter.exists)
        filter.tap()
        filter.typeText(token)
        // return は押さない。キーボードは閉じる必要があるが、debounce 欄では return は commit ではない。
        submitAndDismissKeyboard(filter)

        XCTAssertTrue(app.staticTexts["「\(token)」で 1 件"].waitForExistence(timeout: 5),
                      "入力が止まってから debounce で commit され、Go が絞り込みを反映する")
        XCTAssertTrue(app.switches[a].exists, "一致する行は残る")
        XCTAssertFalse(app.switches[b].exists, "一致しない行は消える")

        // 空に戻す → debounce で空文字が commit され、全行が戻る
        filter.tap()
        filter.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: token.count))
        submitAndDismissKeyboard(filter)
        XCTAssertTrue(waitUntil(5) { self.app.switches[b].exists }, "絞り込み解除も debounce で届く")
    }

    /// TS-403: blur ポリシーの TextField は、フォーカスを失ったときに commit される。
    /// リスト名欄（`CommitOnBlur`）に固有トークンを打ち、待っても見出しが変わらず（debounce ではない）、
    /// 別の欄をタップして初めて見出しが Go の値で描き直される（submit を待たない）ことで確かめる。
    func testBlurTitleCommitsOnFocusLoss() {
        XCTAssertTrue(field.waitForExistence(timeout: 10))
        let title = app.textFields["リスト名"]
        XCTAssertTrue(title.exists)
        let token = "T\(Int.random(in: 1000...9999))"

        clear(title, placeholder: "リスト名")
        title.typeText(token)
        RunLoop.current.run(until: Date().addingTimeInterval(1.0))
        XCTAssertFalse(app.staticTexts[token].exists, "フォーカス中は時間が経っても commit されない")

        let filter = app.textFields["絞り込み"]
        filter.tap()
        XCTAssertTrue(app.staticTexts[token].waitForExistence(timeout: 5),
                      "別の欄へ移ってフォーカスを失うと commit され、見出しに Go の値が現れる")
        submitAndDismissKeyboard(filter)
    }
}
