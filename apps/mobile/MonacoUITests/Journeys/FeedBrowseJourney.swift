import XCTest

enum FeedBrowseJourney {
    static let id = "feed/browse"
    static let version = 3
    static let searchPlaceholder = "Search the feed"
    static let chips = ["All", "Proposals", "Trades", "Price moves", "Cabals"]

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func joinedCabal(run: String) -> String { "QA feed \(run)" }
    static func ownCabal(run: String) -> String { "QA own \(run)" }

    static func text(_ app: XCUIApplication, containing fragment: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", fragment)).firstMatch
    }

    static func openFeed(_ app: XCUIApplication, step: String) {
        app.tab("Feed").tap()
        XCTAssertTrue(app.element("feed-root").waitForExistence(timeout: 15), "\(step): no Feed tab within 15 s")
    }

    static func tapChip(_ app: XCUIApplication, _ title: String) {
        let screen = app.frame
        func onScreen(_ element: XCUIElement) -> Bool {
            let frame = element.frame
            return frame.width > 0 && frame.minX >= 0 && frame.maxX <= screen.maxX
        }
        let chip = app.element("feed-chip-\(title)")
        var swipes = 0
        while !onScreen(chip), swipes < 4,
            let anchor = chips.map({ app.element("feed-chip-\($0)") }).first(where: onScreen)
        {
            if chip.frame.minX > anchor.frame.minX { anchor.swipeLeft() } else { anchor.swipeRight() }
            swipes += 1
        }
        chip.tap()
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
            XCTAssertTrue(app.element("feed-scope").exists, "S1.1: no Everyone / Following toggle")
            for chip in chips {
                XCTAssertTrue(app.element("feed-chip-\(chip)").exists, "S1.1: no '\(chip)' chip")
            }
        }

        recorder.step("S1.2", "the Cabals chip shows the seeded cabal") {
            tapChip(app, "Cabals")
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
                JoinJourney.waitForLabel(app.element("feed-empty"), containing: "Nothing matches", timeout: 10),
                "S1.4: no 'Nothing matches' within 10 s"
            )
        }
    }

    static func following(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "Following hides people A does not follow") {
            openFeed(app, step: "S2.1")
            let following = app.buttons.matching(identifier: "feed-scope").matching(
                NSPredicate(format: "label == 'Following'")
            ).firstMatch
            XCTAssertTrue(following.waitForExistence(timeout: 5), "S2.1: no 'Following' in feed-scope")
            following.tap()
            XCTAssertFalse(
                text(app, containing: "started \(joinedCabal(run: run))").waitForExistence(timeout: 5),
                "S2.1: Following shows a cabal started by someone A does not follow"
            )
        }

        recorder.step("S2.2", "Following with no follows says how to fix it") {
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("feed-empty"), containing: "Follow people to see what they do.", timeout: 10),
                "S2.2: no 'Follow people to see what they do.' within 10 s (known failure, #671)"
            )
        }
    }

    static func cellTaps(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S3.1", "a cabal cell opens the cabal") {
            openFeed(app, step: "S3.1")
            tapChip(app, "Cabals")
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
            tapChip(app, "Proposals")
            let cell = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH 'feed-cell-'")).firstMatch
            XCTAssertTrue(
                cell.waitForExistence(timeout: 15), "S3.2: no proposal cell within 15 s (known failure, #612)")
            cell.tap()
            XCTAssertTrue(
                app.element("proposal-detail").waitForExistence(timeout: 15),
                "S3.2: the proposal did not open within 15 s (known failure, #612)"
            )
        }

        recorder.step("S3.3", "hide a cell") {
            app.navigationBars.buttons.element(boundBy: 0).tap()
            tapChip(app, "Cabals")
            let cell = text(app, containing: "started \(joinedCabal(run: run))")
            XCTAssertTrue(cell.waitForExistence(timeout: 15), "S3.3: no 'started \(joinedCabal(run: run))' cell")
            cell.press(forDuration: 1)
            let hide = app.buttons["Hide"].firstMatch
            XCTAssertTrue(hide.waitForExistence(timeout: 5), "S3.3: no 'Hide' action (known failure, #702)")
            hide.tap()
            XCTAssertFalse(cell.waitForExistence(timeout: 5), "S3.3: the hidden cell is still there")
        }
    }
}
