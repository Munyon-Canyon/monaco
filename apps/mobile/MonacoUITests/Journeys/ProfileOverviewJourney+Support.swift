import XCTest

extension ProfileOverviewJourney {
    static let signOutEverywhereTitle = "Sign out of all devices?"
    static let signOutEverywhereMessage =
        "This ends every session on your account, including a lost phone. "
        + "You can sign back in with your existing account. This does not withdraw funds or leave your cabals."

    static func row(_ app: XCUIApplication, _ cabal: Cabal) -> XCUIElement {
        app.element("profile-cabal-row-\(cabal.id)")
    }

    static func reachCabal(_ app: XCUIApplication, _ target: XCUIElement) {
        let screen = app.windows.firstMatch.frame
        var attempts = 0
        while attempts < 40 {
            let inside = target.exists && screen.contains(target.frame.insetBy(dx: 4, dy: 4))
            if inside && app.isAboveTabBar(target) { break }
            app.swipeUp()
            attempts += 1
        }
    }

    static func tapCabalsTab(_ app: XCUIApplication) {
        let tab = app.element("profile-tab-cabals")
        var swipes = 0
        while !(tab.exists && tab.isHittable && app.isAboveTabBar(tab)) && swipes < 8 {
            app.swipeUp()
            swipes += 1
        }
        tab.tap()
    }

    static func positionsFigure(_ app: XCUIApplication) -> String {
        let texts = app.element("profile-positions").staticTexts.allElementsBoundByIndex
        return texts.map(\.label).first { $0.hasPrefix("$") } ?? ""
    }

    static func cabalsCount(_ app: XCUIApplication) -> Int? {
        let label = app.element("profile-stat-cabals").label
        return label.split(whereSeparator: { !$0.isNumber }).first.flatMap { Int($0) }
    }

    static func shareDock(_ app: XCUIApplication) {
        let send = app.buttons["share-send"]
        let copy = app.buttons["share-copy-image"]
        XCTAssertTrue(send.waitForExistence(timeout: 15), "S5.1: no Share button within 15 s")
        XCTAssertTrue(copy.waitForExistence(timeout: 15), "S5.1: no Copy image button within 15 s")
        XCTAssertTrue(send.isEnabled && copy.isEnabled, "S5.1: Share or Copy image is disabled")
        XCTAssertEqual(send.frame.minY, copy.frame.minY, accuracy: 1, "S5.1: Share and Copy image are not on one row")
        XCTAssertGreaterThan(copy.frame.minX, send.frame.maxX - 1, "S5.1: Copy image does not start right of Share")
        let toggles = [
            ("ledger-share-toggle-return", "Show return"), ("ledger-share-toggle-calls", "Show calls"),
            ("ledger-share-toggle-code", "Show code"),
        ]
        for (id, title) in toggles {
            let toggle = app.element(id)
            XCTAssertTrue(toggle.waitForExistence(timeout: 5), "S5.1: no '\(title)' toggle (\(id))")
            XCTAssertEqual(toggle.label, title, "S5.1: the toggle \(id) reads '\(toggle.label)', not '\(title)'")
        }
    }

    static func standingLine(_ app: XCUIApplication) {
        let standing = app.element("profile-standing")
        XCTAssertTrue(standing.waitForExistence(timeout: 15), "S1.2: no month standing line")
        let ranked = standing.label.range(
            of: #"^\d+ of \d+ right this month · \d+(st|nd|rd|th) of \d+$"#, options: .regularExpression)
        XCTAssertTrue(
            ranked != nil || standing.label.contains("resolved this month"),
            "S1.2: the standing line reads '\(standing.label)', not the ranked or the unranked month line")
        XCTAssertFalse(
            standing.label.contains("%") || standing.label.contains("#"),
            "S1.2: the standing line reads '\(standing.label)', with a '%' or '#'")
    }

    static func copyImage(_ app: XCUIApplication) {
        app.buttons["share-copy-image"].tap()
        let toast = app.toastText(timeout: 10)
        XCTAssertEqual(toast, "Image copied.", "S5.3: the toast reads '\(toast ?? "nothing")', not 'Image copied.'")
        XCTAssertNotEqual(toast, "Couldn\u{2019}t make the image. Try again.", "S5.3: the image failed to render")
        app.waitForToastGone()
    }
}
