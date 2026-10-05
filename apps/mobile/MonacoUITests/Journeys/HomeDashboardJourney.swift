import XCTest

enum HomeDashboardJourney {
    static let id = "home/dashboard"
    static let version = 1

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

        recorder.step("S1.1", "the hero names your money in cabals") {
            openHome(app)
            XCTAssertTrue(
                text(app, "Your money in cabals").waitForExistence(timeout: 15),
                "S1.1: no \"Your money in cabals\" hero within 15 s")
        }

        recorder.step("S1.2", "the balance row with Add money and Withdraw") {
            XCTAssertTrue(
                text(app, "Account balance").waitForExistence(timeout: 10), "S1.2: no \"Account balance\" row")
            XCTAssertTrue(
                app.element("platform-balance-value").waitForExistence(timeout: 10), "S1.2: no balance amount")
            XCTAssertTrue(app.element("home-add-money-link").exists, "S1.2: no \"Add money\" under the balance")
            XCTAssertTrue(app.element("home-withdraw-link").exists, "S1.2: no \"Withdraw\" under the balance")
        }

        recorder.step("S1.3", "Your cabals lists the seeded cabal") {
            app.scrollIntoReach(row)
            XCTAssertTrue(text(app, "Your cabals").exists, "S1.3: no \"Your cabals\" header")
            XCTAssertTrue(row.waitForExistence(timeout: 15), "S1.3: no Home row for \(cabalName) within 15 s")
        }

        recorder.step("S1.4", "pull to refresh keeps the cabals") {
            app.swipeDown()
            app.swipeDown()
            let start = text(app, "Your money in cabals").coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5))
            start.press(forDuration: 0.1, thenDragTo: start.withOffset(CGVector(dx: 0, dy: 400)))
            app.scrollIntoReach(row)
            XCTAssertTrue(
                ProfileOverviewJourney.waitUntil(15) { row.exists && text(app, "Your cabals").exists },
                "S1.4: Your cabals or the row for \(cabalName) was gone after the refresh")
        }

        recorder.step("S1.5", "the row opens the cabal") {
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

        recorder.step("S1.7", "the hero shows the total and the all-time chip") {
            openHome(app)
            app.swipeDown()
            XCTAssertTrue(
                text(app, "all time").waitForExistence(timeout: 10),
                "S1.7: no total or \"all time\" chip in the hero within 10 s (known failure, #660)")
        }

        recorder.step("S1.8", "the hero shows the P&L chart and its range chips") {
            XCTAssertTrue(
                app.element("home-pnl-chart").waitForExistence(timeout: 10),
                "S1.8: no P&L chart in the hero within 10 s (known failure, #660)")
            XCTAssertTrue(app.buttons["1D"].firstMatch.exists, "S1.8: no \"1D\" range chip (known failure, #660)")
        }
    }

    static func emptySendsToBrowse(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S2.1", "no cabal shows the empty state") {
            openHome(app)
            let title = text(app, "No cabals yet")
            app.scrollIntoReach(title)
            XCTAssertTrue(title.waitForExistence(timeout: 15), "S2.1: no \"No cabals yet\" within 15 s")
            XCTAssertTrue(
                text(app, "Start one with friends or join an open one.").exists,
                "S2.1: no \"Start one with friends or join an open one.\"")
        }

        recorder.step("S2.2", "Browse cabals selects the Cabals tab") {
            let browse = app.buttons["Browse cabals"].firstMatch
            app.scrollIntoReach(browse)
            browse.tap()
            XCTAssertTrue(app.element("cabals-root").waitForExistence(timeout: 10), "S2.2: the Cabals tab did not show")
            XCTAssertTrue(app.tab("Cabals").isSelected, "S2.2: the Cabals tab is not selected")
        }
    }
}
