import XCTest

enum HomeDashboardJourney {
    static let id = "home/dashboard"
    static let version = 4

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func text(_ app: XCUIApplication, _ label: String) -> XCUIElement {
        app.staticTexts[label].firstMatch
    }

    static func openHome(_ app: XCUIApplication) {
        app.tab("Home").tap()
    }

    static func readsHome(_ app: XCUIApplication, cabalID: String, cabalName: String, recorder: JourneyRecorder) {
        let row = app.element("cabal-row-\(cabalID)")
        let total = app.element("home-portfolio-total")

        recorder.step("S1.1", "the hero shows the total") {
            openHome(app)
            XCTAssertTrue(total.waitForExistence(timeout: 15), "S1.1: no total in the hero within 15 s")
            XCTAssertTrue(total.label.contains("$"), "S1.1: the hero total reads '\(total.label)', not dollars")
        }

        recorder.step("S1.2", "the money strip with Net worth, Cash and Deposit") {
            let netWorth = app.element("home-net-worth")
            XCTAssertTrue(netWorth.waitForExistence(timeout: 10), "S1.2: no \"Net worth\" in the money strip")
            XCTAssertEqual(netWorth.label, "Net worth", "S1.2: the net worth metric reads '\(netWorth.label)'")
            let cash = app.element("platform-balance-value")
            XCTAssertTrue(cash.waitForExistence(timeout: 10), "S1.2: no \"Cash\" amount in the money strip")
            XCTAssertEqual(cash.label, "Cash", "S1.2: the cash metric reads '\(cash.label)'")
            XCTAssertTrue(
                (cash.value as? String)?.hasPrefix("$") == true,
                "S1.2: the cash amount reads '\(cash.value as? String ?? "")', not dollars")
            let deposit = app.element("home-add-money-link")
            XCTAssertTrue(deposit.exists, "S1.2: no \"Deposit\" in the money strip")
            XCTAssertEqual(deposit.label, "Deposit", "S1.2: the deposit button reads '\(deposit.label)'")
        }

        recorder.step("S1.3", "Your cabals lists the seeded cabal") {
            let header = text(app, "Your cabals")
            app.scrollIntoReach(header)
            XCTAssertTrue(header.waitForExistence(timeout: 10), "S1.3: no \"Your cabals\" header")
            ProfileOverviewJourney.reachCabal(app, row)
            XCTAssertTrue(row.waitForExistence(timeout: 15), "S1.3: no Home card for \(cabalName) within 15 s")
        }

        recorder.step("S1.4", "pull to refresh keeps the cabals") {
            app.swipeDown()
            app.swipeDown()
            let start = total.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5))
            start.press(forDuration: 0.1, thenDragTo: start.withOffset(CGVector(dx: 0, dy: 400)))
            ProfileOverviewJourney.reachCabal(app, row)
            XCTAssertTrue(
                ProfileOverviewJourney.waitUntil(15) { row.exists && text(app, "Your cabals").exists },
                "S1.4: Your cabals or the card for \(cabalName) was gone after the refresh")
        }

        recorder.step("S1.5", "the card opens the cabal") {
            ProfileOverviewJourney.reachCabal(app, row)
            row.tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("cabal-header-name"), containing: cabalName, timeout: 15),
                "S1.5: the cabal screen for \(cabalName) did not show within 15 s")
        }

        recorder.step("S1.6", "the avatar opens Profile") {
            openHome(app)
            let avatar = app.element("home-profile-button")
            XCTAssertTrue(avatar.waitForExistence(timeout: 10), "S1.6: no avatar at the top right of Home")
            avatar.tap()
            XCTAssertTrue(app.element("profile-header").waitForExistence(timeout: 15), "S1.6: Profile did not show")
            XCTAssertTrue(app.tab("Profile").isSelected, "S1.6: the Profile tab is not selected")
        }

        recorder.step("S1.7", "the hero shows the return chip") {
            openHome(app)
            app.swipeDown()
            XCTAssertTrue(
                app.element("home-portfolio-chip").waitForExistence(timeout: 10),
                "S1.7: no return chip under the hero total within 10 s")
        }
    }

    static func emptySendsToBrowse(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S2.1", "no cabal shows the empty state") {
            openHome(app)
            let title = text(app, "Start with your people")
            app.scrollIntoReach(title)
            XCTAssertTrue(title.waitForExistence(timeout: 15), "S2.1: no \"Start with your people\" within 15 s")
            XCTAssertTrue(
                text(app, "Create a cabal or open an invitation from a friend.").exists,
                "S2.1: no \"Create a cabal or open an invitation from a friend.\"")
            XCTAssertTrue(app.buttons["Create a cabal"].firstMatch.exists, "S2.1: no \"Create a cabal\" button")
        }

        recorder.step("S2.2", "Explore cabals selects the Cabals tab") {
            let browse = app.buttons["Explore cabals"].firstMatch
            app.scrollIntoReach(browse)
            browse.tap()
            XCTAssertTrue(app.element("cabals-root").waitForExistence(timeout: 10), "S2.2: the Cabals tab did not show")
            XCTAssertTrue(app.tab("Cabals").isSelected, "S2.2: the Cabals tab is not selected")
        }
    }
}
