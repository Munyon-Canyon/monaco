import XCTest

enum StocksBrowseJourney {
    static let id = "stocks/browse"
    static let version = 7

    static let searchPlaceholder = "Stocks and Pre-IPO"
    static let alpha = SeededAsset(symbol: "JRNYAx", ticker: "JRNYA", name: "Journey Alpha", price: "$123.45")
    static let preIpo = SeededAsset(symbol: "JRNYPx", ticker: "JRNYPx", name: "Journey Private", price: "$50.00")
    static let zulu = SeededAsset(symbol: "JRNYZx", ticker: "JRNYZ", name: "Journey Zulu", price: "$10.00")

    struct SeededAsset {
        let symbol: String
        let ticker: String
        let name: String
        let price: String

        var rowID: String { "assets-row-\(symbol)" }
        var preIpoRowID: String { "markets-preipo-row-\(symbol)" }
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

    static func preIpoRow(_ app: XCUIApplication, _ asset: SeededAsset) -> XCUIElement {
        app.element(asset.preIpoRowID)
    }

    static let chips = ["All", "Watchlist", "Pre-IPO", "Held"]

    static func chip(_ app: XCUIApplication, _ title: String) -> XCUIElement {
        app.element("markets-chip-\(title)")
    }

    static func openStocks(_ app: XCUIApplication, step: String) {
        app.element("tab-assets").tap()
        XCTAssertTrue(app.element("assets-root").waitForExistence(timeout: 15), "\(step): the Markets tab did not open")
        XCTAssertTrue(app.navigationBars["Markets"].exists, "\(step): the Markets tab is not titled 'Markets'")
    }

    static func waitForRow(_ app: XCUIApplication, _ asset: SeededAsset, step: String, timeout: TimeInterval) {
        XCTAssertTrue(
            scrollTo(app, row(app, asset), timeout: timeout),
            "\(step): no \(asset.rowID) within \(Int(timeout)) s")
    }

    static func assertRowReads(
        _ app: XCUIApplication, _ asset: SeededAsset, price: Bool, step: String, preIpoSection: Bool = false
    ) {
        let id = preIpoSection ? asset.preIpoRowID : asset.rowID
        let label = app.element(id).label
        XCTAssertTrue(label.contains(asset.name), "\(step): \(id) reads '\(label)', not '\(asset.name)'")
        if !preIpoSection {
            XCTAssertTrue(label.contains(asset.ticker), "\(step): \(id) reads '\(label)', not '\(asset.ticker)'")
        }
        if price {
            XCTAssertTrue(label.contains(asset.price), "\(step): \(id) reads '\(label)', not '\(asset.price)'")
        }
    }

    static func scrollTo(_ app: XCUIApplication, _ element: XCUIElement, timeout: TimeInterval) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        var swipes = 0
        while Date() < deadline {
            if element.exists && element.isHittable { return true }
            if swipes % 24 < 12 {
                app.element("assets-root").swipeUp()
            } else {
                app.element("assets-root").swipeDown()
            }
            swipes += 1
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

    static func cancelSearch(_ app: XCUIApplication, step: String) {
        let cancel = app.element("markets-search-cancel")
        XCTAssertTrue(cancel.waitForExistence(timeout: 10), "\(step): no markets-search-cancel while searching")
        cancel.tap()
    }

    static func chooseChip(_ app: XCUIApplication, _ title: String, step: String) {
        let target = chip(app, title)
        XCTAssertTrue(target.waitForExistence(timeout: 15), "\(step): no markets-chip-\(title) within 15 s")
        target.tap()
        let selected = NSPredicate(format: "isSelected == true")
        let waiter = XCTNSPredicateExpectation(predicate: selected, object: target)
        XCTAssertEqual(XCTWaiter().wait(for: [waiter], timeout: 10), .completed, "\(step): '\(title)' is not selected")
    }

    static func browseChips(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the Markets tab") {
            openStocks(app, step: "S1.1")
            let field = searchField(app)
            XCTAssertTrue(field.waitForExistence(timeout: 15), "S1.1: no search field on the Markets tab")
            XCTAssertEqual(
                field.placeholderValue, searchPlaceholder, "S1.1: the search field's placeholder is not the spec's")
            for title in chips {
                XCTAssertTrue(chip(app, title).waitForExistence(timeout: 10), "S1.1: no markets-chip-\(title)")
            }
            XCTAssertTrue(chip(app, "All").isSelected, "S1.1: 'All' is not selected")
        }

        recorder.step("S1.2", "search for the seeded rows") {
            replaceQuery(app, with: "Journey")
            waitForRow(app, alpha, step: "S1.2", timeout: 15)
            assertRowReads(app, alpha, price: true, step: "S1.2")
            XCTAssertFalse(app.element("stocks-sort").exists, "S1.2: a Sort select shows")
            XCTAssertFalse(app.element("stocks-filter").exists, "S1.2: a Filter select shows")
        }

        recorder.step("S1.3", "choose Pre-IPO") {
            cancelSearch(app, step: "S1.3")
            chooseChip(app, "Pre-IPO", step: "S1.3")
            let row = preIpoRow(app, preIpo)
            XCTAssertTrue(row.waitForExistence(timeout: 10), "S1.3: no \(preIpo.preIpoRowID) under Pre-IPO")
            XCTAssertTrue(row.label.contains(preIpo.name), "S1.3: the row reads '\(row.label)', not '\(preIpo.name)'")
            XCTAssertTrue(
                row.label.contains("Eligible") || row.label.contains("Research only"),
                "S1.3: the row reads '\(row.label)', with no eligibility")
            XCTAssertFalse(preIpoRow(app, alpha).exists, "S1.3: \(alpha.preIpoRowID) shows under Pre-IPO")
        }

        recorder.step("S1.4", "search pages to the last row") {
            chooseChip(app, "All", step: "S1.4")
            replaceQuery(app, with: "Journey")
            XCTAssertTrue(scrollTo(app, row(app, zulu), timeout: 30), "S1.4: no \(zulu.rowID) within 30 s")
            assertRowReads(app, zulu, price: false, step: "S1.4")
        }

        recorder.step("S1.5", "choose Held with no holding") {
            cancelSearch(app, step: "S1.5")
            chooseChip(app, "Held", step: "S1.5")
            let empty = app.element("markets-held-empty")
            XCTAssertTrue(
                empty.waitForExistence(timeout: 10), "S1.5: no markets-held-empty for a member with no holding"
            )
        }
    }

    static func search(_ app: XCUIApplication, recorder: JourneyRecorder) throws {
        let nonsense = "zzqj\(try JourneyRun.id())"

        recorder.step("S2.1", "search by name") {
            openStocks(app, step: "S2.1")
            chooseChip(app, "All", step: "S2.1")
            let field = searchField(app)
            XCTAssertTrue(field.waitForExistence(timeout: 15), "S2.1: no search field on the Markets tab")
            replaceQuery(app, with: alpha.name)
            waitForRow(app, alpha, step: "S2.1", timeout: 10)
            XCTAssertFalse(row(app, zulu).exists, "S2.1: \(zulu.rowID) shows for '\(alpha.name)'")
        }

        recorder.step("S2.2", "search for nothing") {
            replaceQuery(app, with: nonsense)
            let empty = app.element("assets-empty")
            XCTAssertTrue(empty.waitForExistence(timeout: 10), "S2.2: no empty result for '\(nonsense)'")
            XCTAssertTrue(
                empty.label.hasPrefix("No results for"), "S2.2: the empty result reads '\(empty.label)', not the spec's"
            )
        }

        recorder.step("S2.3", "clear the search") {
            cancelSearch(app, step: "S2.3")
            XCTAssertTrue(app.element("assets-empty").waitForNonExistence(timeout: 10), "S2.3: the empty result stayed")
            XCTAssertTrue(chip(app, "All").waitForExistence(timeout: 10), "S2.3: the chips did not come back")
            XCTAssertTrue(chip(app, "All").isSelected, "S2.3: 'All' is not selected after clearing")
        }
    }

    static func pullToRefresh(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "open the Pre-IPO list") {
            openStocks(app, step: "S3.1")
            chooseChip(app, "Pre-IPO", step: "S3.1")
            XCTAssertTrue(
                preIpoRow(app, preIpo).waitForExistence(timeout: 15), "S3.1: no \(preIpo.preIpoRowID) under Pre-IPO")
        }

        recorder.step("S3.2", "pull to refresh") {
            for _ in 0..<6 { app.element("assets-root").swipeDown() }
            app.element("assets-root").swipeDown(velocity: .slow)
            XCTAssertTrue(
                preIpoRow(app, preIpo).waitForExistence(timeout: 15), "S3.2: no \(preIpo.preIpoRowID) after a refresh")
            assertRowReads(app, preIpo, price: false, step: "S3.2", preIpoSection: true)
            XCTAssertFalse(app.element("assets-failed").exists, "S3.2: 'Couldn't load this view' after a refresh")
            XCTAssertFalse(app.element("monaco-toast-banner").exists, "S3.2: a toast showed after a refresh")
        }
    }
}
