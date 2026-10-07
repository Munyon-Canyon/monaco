import XCTest

enum StocksBrowseJourney {
    static let id = "stocks/browse"
    static let version = 2

    static let searchPlaceholder = "Search Apple, Tesla, NVDA…"
    static let alpha = SeededAsset(symbol: "JRNYAx", ticker: "JRNYA", name: "Journey Alpha", price: "$123.45")
    static let preIpo = SeededAsset(symbol: "JRNYPx", ticker: "JRNYPx", name: "Journey Private", price: "$50.00")
    static let zulu = SeededAsset(symbol: "JRNYZx", ticker: "JRNYZ", name: "Journey Zulu", price: "$10.00")

    struct SeededAsset {
        let symbol: String
        let ticker: String
        let name: String
        let price: String

        var rowID: String { "assets-row-\(symbol)" }
    }

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func searchField(_ app: XCUIApplication) -> XCUIElement {
        app.textFields["monaco-search-field"]
    }

    static func row(_ app: XCUIApplication, _ asset: SeededAsset) -> XCUIElement {
        app.element(asset.rowID)
    }

    static func openStocks(_ app: XCUIApplication, step: String) {
        app.element("tab-assets").tap()
        XCTAssertTrue(app.element("assets-root").waitForExistence(timeout: 15), "\(step): the Stocks tab did not open")
        XCTAssertTrue(app.navigationBars["Stocks"].exists, "\(step): the Stocks tab is not titled 'Stocks'")
    }

    static func waitForRow(_ app: XCUIApplication, _ asset: SeededAsset, step: String, timeout: TimeInterval) {
        XCTAssertTrue(
            row(app, asset).waitForExistence(timeout: timeout),
            "\(step): no \(asset.rowID) within \(Int(timeout)) s (the Stocks tab is blank on staging, #577)")
    }

    static func assertRowReads(_ app: XCUIApplication, _ asset: SeededAsset, price: Bool, step: String) {
        let label = row(app, asset).label
        XCTAssertTrue(label.contains(asset.ticker), "\(step): \(asset.rowID) reads '\(label)', not '\(asset.ticker)'")
        XCTAssertTrue(label.contains(asset.name), "\(step): \(asset.rowID) reads '\(label)', not '\(asset.name)'")
        if price {
            XCTAssertTrue(label.contains(asset.price), "\(step): \(asset.rowID) reads '\(label)', not '\(asset.price)'")
        }
    }

    static func scrollTo(_ app: XCUIApplication, _ element: XCUIElement, timeout: TimeInterval) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if element.exists && element.isHittable { return true }
            app.element("assets-grid").swipeUp()
        }
        return element.exists
    }

    static func replaceQuery(_ app: XCUIApplication, with text: String) {
        let field = searchField(app)
        field.tap()
        let current = field.value as? String ?? ""
        if current != searchPlaceholder, !current.isEmpty {
            field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: current.count))
        }
        field.typeText(text)
    }

    static func chip(_ app: XCUIApplication, _ title: String) -> XCUIElement {
        app.element("stocks-chip-\(title)")
    }

    static func browseSections(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the Stocks tab") {
            openStocks(app, step: "S1.1")
            let field = searchField(app)
            XCTAssertTrue(field.waitForExistence(timeout: 15), "S1.1: no search field on the Stocks tab")
            XCTAssertEqual(
                field.placeholderValue, searchPlaceholder, "S1.1: the search field's placeholder is not the spec's")
        }

        recorder.step("S1.2", "see the chips and All") {
            waitForRow(app, alpha, step: "S1.2", timeout: 15)
            for title in ["All", "Popular", "Pre-IPO"] {
                XCTAssertTrue(chip(app, title).exists, "S1.2: no stocks-chip-\(title)")
            }
            XCTAssertTrue(chip(app, "All").isSelected, "S1.2: All is not selected")
            assertRowReads(app, alpha, price: true, step: "S1.2")
        }

        recorder.step("S1.3", "tap Popular") {
            chip(app, "Popular").tap()
            waitForRow(app, alpha, step: "S1.3", timeout: 10)
            XCTAssertFalse(row(app, preIpo).exists, "S1.3: \(preIpo.rowID) shows under Popular")
            XCTAssertFalse(row(app, zulu).exists, "S1.3: \(zulu.rowID) shows under Popular")
        }

        recorder.step("S1.4", "tap Pre-IPO") {
            chip(app, "Pre-IPO").tap()
            waitForRow(app, preIpo, step: "S1.4", timeout: 10)
            assertRowReads(app, preIpo, price: false, step: "S1.4")
            XCTAssertFalse(row(app, alpha).exists, "S1.4: \(alpha.rowID) shows under Pre-IPO")
        }

        recorder.step("S1.5", "tap All and scroll to the last row") {
            chip(app, "All").tap()
            XCTAssertTrue(scrollTo(app, row(app, zulu), timeout: 30), "S1.5: no \(zulu.rowID) within 30 s")
            assertRowReads(app, zulu, price: false, step: "S1.5")
        }
    }

    static func search(_ app: XCUIApplication, recorder: JourneyRecorder) throws {
        let nonsense = "zzqj\(try JourneyRun.id())"

        recorder.step("S2.1", "search by name") {
            openStocks(app, step: "S2.1")
            let field = searchField(app)
            XCTAssertTrue(field.waitForExistence(timeout: 15), "S2.1: no search field on the Stocks tab")
            replaceQuery(app, with: alpha.name)
            let results = app.element("assets-grid-search")
            XCTAssertTrue(results.waitForExistence(timeout: 10), "S2.1: results did not replace the list")
            waitForRow(app, alpha, step: "S2.1", timeout: 10)
            XCTAssertFalse(row(app, zulu).exists, "S2.1: \(zulu.rowID) shows for '\(alpha.name)'")
        }

        recorder.step("S2.2", "search for nothing") {
            replaceQuery(app, with: nonsense)
            let empty = app.element("assets-search-empty")
            XCTAssertTrue(empty.waitForExistence(timeout: 10), "S2.2: no empty result for '\(nonsense)'")
            let copy = "No stocks match “\(nonsense)”"
            XCTAssertEqual(empty.label, copy, "S2.2: the empty result is not the spec's copy")
        }

        recorder.step("S2.3", "clear the search") {
            replaceQuery(app, with: "")
            let sections = app.element("assets-grid")
            XCTAssertTrue(sections.waitForExistence(timeout: 10), "S2.3: the list did not come back")
            waitForRow(app, alpha, step: "S2.3", timeout: 10)
            XCTAssertTrue(chip(app, "All").isSelected, "S2.3: the chip is not back on All after clearing")
        }
    }

    static func pullToRefresh(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "open the Stocks tab") {
            openStocks(app, step: "S3.1")
            waitForRow(app, alpha, step: "S3.1", timeout: 15)
        }

        recorder.step("S3.2", "pull to refresh") {
            app.element("assets-grid").swipeDown(velocity: .slow)
            waitForRow(app, alpha, step: "S3.2", timeout: 15)
            assertRowReads(app, alpha, price: true, step: "S3.2")
            XCTAssertFalse(app.element("assets-failed").exists, "S3.2: 'Couldn't load stocks.' after a refresh")
            XCTAssertFalse(app.element("monaco-toast-banner").exists, "S3.2: a toast showed after a refresh")
        }
    }
}
