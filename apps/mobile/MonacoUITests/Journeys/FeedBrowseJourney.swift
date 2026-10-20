import XCTest

enum FeedBrowseJourney {
    static let id = "feed/browse"
    static let version = 6
    static let searchPlaceholder = "Search the feed"
    static let filterOptions = [
        "feed-scope-Everyone", "feed-scope-My cabals", "feed-scope-Following",
        "feed-chip-All", "feed-chip-Proposals", "feed-chip-Completed trades", "feed-chip-Price moves",
        "feed-chip-Replies & votes", "feed-chip-Proposal outcomes", "feed-chip-Cabal activity", "feed-muted-link",
    ]

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func joinedCabal(run: String) -> String { "QA feed \(run)" }
    static func ownCabal(run: String) -> String { "QA own \(run)" }
    static func thesis(run: String) -> String { "QA proposal \(run)" }

    static func text(_ app: XCUIApplication, containing fragment: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", fragment)).firstMatch
    }

    static func openFeed(_ app: XCUIApplication, step: String) {
        app.tab("Feed").tap()
        XCTAssertTrue(app.element("feed-root").waitForExistence(timeout: 15), "\(step): no Feed tab within 15 s")
    }

    static func menuItem(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        let label =
            identifier == "feed-muted-link"
            ? "Muted"
            : identifier.replacingOccurrences(of: "feed-scope-", with: "").replacingOccurrences(
                of: "feed-chip-", with: "")
        return app.buttons.matching(NSPredicate(format: "identifier == %@ OR label == %@", identifier, label))
            .firstMatch
    }

    static func openFilter(_ app: XCUIApplication) {
        app.element("feed-filter").tap()
    }

    static func closeFilter(_ app: XCUIApplication) {
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.08, dy: 0.6)).tap()
    }

    static func choose(_ app: XCUIApplication, _ options: String...) {
        openFilter(app)
        for option in options {
            let item = menuItem(app, option)
            XCTAssertTrue(item.waitForExistence(timeout: 5), "no \(option) in the filter menu")
            item.tap()
        }
        closeFilter(app)
    }

    static func replaceQuery(_ app: XCUIApplication, with text: String) {
        let field = app.textFields["monaco-search-field"]
        field.tap()
        let current = field.value as? String ?? ""
        if current != searchPlaceholder, !current.isEmpty {
            field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: current.count))
        }
        field.typeText(text)
    }

    static func chipsAndSearch(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the Feed tab") {
            openFeed(app, step: "S1.1")
            let field = app.textFields["monaco-search-field"]
            XCTAssertTrue(field.waitForExistence(timeout: 5), "S1.1: no search field on the Feed tab")
            XCTAssertEqual(field.placeholderValue, searchPlaceholder, "S1.1: the search placeholder is not the spec's")
            openFilter(app)
            for option in filterOptions {
                XCTAssertTrue(menuItem(app, option).waitForExistence(timeout: 5), "S1.1: no \(option) in the menu")
            }
            XCTAssertTrue(menuItem(app, "feed-chip-All").isSelected, "S1.1: All is not checked by default")
            closeFilter(app)
        }

        recorder.step("S1.2", "the Cabal activity toggle shows the seeded cabal") {
            choose(app, "feed-chip-Cabal activity")
            XCTAssertTrue(
                text(app, containing: "started \(joinedCabal(run: run))").waitForExistence(timeout: 15),
                "S1.2: no 'started \(joinedCabal(run: run))' cell within 15 s"
            )
        }

        recorder.step("S1.3", "search narrows to one cabal") {
            replaceQuery(app, with: joinedCabal(run: run))
            XCTAssertTrue(
                text(app, containing: "joined \(joinedCabal(run: run))").waitForExistence(timeout: 10),
                "S1.3: no 'joined \(joinedCabal(run: run))' cell within 10 s"
            )
            XCTAssertFalse(
                text(app, containing: "started \(ownCabal(run: run))").exists,
                "S1.3: the search still shows \(ownCabal(run: run))"
            )
        }

        recorder.step("S1.4", "a search with no match is empty") {
            replaceQuery(app, with: "nomatch \(run)")
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("feed-empty"), containing: "No matching updates", timeout: 10),
                "S1.4: no 'No matching updates' within 10 s"
            )
        }
    }

    static func following(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "Following hides people A does not follow") {
            openFeed(app, step: "S2.1")
            choose(app, "feed-scope-Following")
            XCTAssertFalse(
                text(app, containing: "started \(joinedCabal(run: run))").waitForExistence(timeout: 5),
                "S2.1: Following shows a cabal started by someone A does not follow"
            )
        }

        recorder.step("S2.2", "Following with no follows says how to fix it") {
            XCTAssertTrue(
                app.staticTexts["Follow people to see what they do."].firstMatch.waitForExistence(timeout: 10),
                "S2.2: no 'Follow people to see what they do.' within 10 s"
            )
            XCTAssertEqual(
                app.element("feed-find-friends").label, "Find friends", "S2.2: no 'Find friends' under the message")
        }
    }

    static func cellTaps(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S3.1", "a cabal cell opens the cabal") {
            openFeed(app, step: "S3.1")
            let clear = app.buttons["Clear search"]
            if clear.exists { clear.tap() }
            app.dismissKeyboard()
            choose(app, "feed-scope-Everyone")
            let cell = text(app, containing: "started \(joinedCabal(run: run))")
            XCTAssertTrue(cell.waitForExistence(timeout: 15), "S3.1: no 'started \(joinedCabal(run: run))' cell")
            cell.tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("cabal-header-name"), containing: joinedCabal(run: run), timeout: 15),
                "S3.1: the cell did not open \(joinedCabal(run: run)) within 15 s"
            )
        }

        recorder.step("S3.2", "a proposal cell opens the proposal") {
            app.navigationBars.buttons.element(boundBy: 0).tap()
            choose(app, "feed-chip-Cabal activity", "feed-chip-Proposals")
            let cell = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH 'feed-cell-'")).firstMatch
            XCTAssertTrue(cell.waitForExistence(timeout: 15), "S3.2: no proposal cell within 15 s")
            cell.tap()
            XCTAssertTrue(
                app.element("proposal-votes").waitForExistence(timeout: 15),
                "S3.2: the proposal did not open within 15 s"
            )
            XCTAssertTrue(
                text(app, containing: thesis(run: run)).waitForExistence(timeout: 10),
                "S3.2: the proposal does not read '\(thesis(run: run))'"
            )
        }

        recorder.step("S3.3", "hide a cell") {
            app.navigationBars.buttons.element(boundBy: 0).tap()
            choose(app, "feed-chip-Proposals", "feed-chip-Cabal activity")
            let cell = text(app, containing: "started \(joinedCabal(run: run))")
            XCTAssertTrue(cell.waitForExistence(timeout: 15), "S3.3: no 'started \(joinedCabal(run: run))' cell")
            cell.press(forDuration: 1)
            let hide = app.buttons["Hide this post"].firstMatch
            XCTAssertTrue(hide.waitForExistence(timeout: 5), "S3.3: no 'Hide this post' action")
            hide.tap()
            XCTAssertFalse(cell.waitForExistence(timeout: 5), "S3.3: the hidden cell is still there")
        }
    }
}
