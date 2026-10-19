import XCTest

enum ProfileOverviewJourney {
    static let id = "profile/overview"
    static let version = 7

    static let signOutTitle = "Sign out of Monaco?"
    static let signOutMessage =
        "You can sign back in with your existing account. This does not withdraw funds or leave your cabals."

    struct Cabal {
        let id: String
        let name: String

        static func handedOff() throws -> Cabal {
            Cabal(id: try JourneyHandoff.read("cabalID"), name: try JourneyHandoff.read("cabalName"))
        }
    }

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func waitUntil(_ timeout: TimeInterval, _ condition: () -> Bool) -> Bool {
        FirstRunJourney.waitUntil(timeout, condition)
    }

    static func openProfile(_ app: XCUIApplication, step: String) {
        app.tab("Profile").tap()
        app.popToRoot()
        XCTAssertTrue(
            app.element("profile-header").waitForExistence(timeout: 15), "\(step): the Profile header did not show")
    }

    static func row(_ app: XCUIApplication, _ cabal: Cabal) -> XCUIElement {
        app.element("cabal-row-\(cabal.id)")
    }

    static func reachCabal(_ app: XCUIApplication, _ target: XCUIElement) {
        let screen = app.windows.firstMatch.frame
        let cards = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'cabal-cover-card-'"))
        var attempts = 0
        while !(target.exists && screen.contains(target.frame.insetBy(dx: 4, dy: 4))) && attempts < 40 {
            let card = cards.firstMatch
            if card.exists, card.frame.midY > 0, card.frame.midY < screen.maxY - 90 {
                let y = card.frame.midY / screen.height
                let from = app.coordinate(withNormalizedOffset: CGVector(dx: 0.9, dy: y))
                let to = app.coordinate(withNormalizedOffset: CGVector(dx: 0.1, dy: y))
                from.press(forDuration: 0.05, thenDragTo: to, withVelocity: .fast, thenHoldForDuration: 0)
            } else {
                app.swipeUp()
            }
            attempts += 1
        }
    }

    static func cabalsCount(_ app: XCUIApplication) -> Int? {
        let label = app.element("profile-stat-cabals").label
        return label.split(whereSeparator: { !$0.isNumber }).first.flatMap { Int($0) }
    }

    static func sections(_ app: XCUIApplication, cabal: Cabal, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open Profile and read the header") {
            openProfile(app, step: "S1.1")
            XCTAssertFalse(
                app.staticTexts["profile-display-name"].label.isEmpty, "S1.1: no display name on the header")
            let handle = app.element("profile-handle")
            XCTAssertTrue(
                handle.exists && handle.label.hasPrefix("@"), "S1.1: the handle does not read '@' and the handle")
            let since = app.staticTexts["profile-member-since"].label
            XCTAssertNotNil(
                since.range(of: #"^Joined \S+ \d{4}$"#, options: .regularExpression),
                "S1.1: the header reads '\(since)', not 'Joined' with a month and year")
        }

        recorder.step("S1.2", "read the identity card stats") {
            XCTAssertTrue(
                waitUntil(15) { (cabalsCount(app) ?? 0) >= 1 },
                "S1.2: 'Cabals' reads '\(app.element("profile-stat-cabals").label)', not a count of 1 or more")
            XCTAssertTrue(
                app.element("profile-stat-in-cabals").label.contains("Positions value"),
                "S1.2: no 'Positions value' column")
            XCTAssertTrue(
                app.element("profile-stat-return").label.contains("Total P&L"), "S1.2: no 'Total P&L' column")
            XCTAssertTrue(
                app.element("profile-stat-cabals").label.contains("cabal"), "S1.2: no 'cabal' or 'cabals' count")
        }

        recorder.step("S1.3", "find the cabal under Your cabals") {
            let cabalRow = row(app, cabal)
            reachCabal(app, cabalRow)
            XCTAssertTrue(cabalRow.waitForExistence(timeout: 10), "S1.3: no row for '\(cabal.name)' under Your cabals")
            XCTAssertTrue(app.staticTexts["Your cabals"].exists, "S1.3: no 'Your cabals' header")
            let card = app.element("cabal-cover-card-\(cabal.id)")
            XCTAssertTrue(
                card.label.contains(cabal.name), "S1.3: the card reads '\(card.label)', not '\(cabal.name)'")
        }

        recorder.step("S1.4", "find the Settings button") {
            let settings = app.element("profile-settings-row")
            XCTAssertTrue(settings.waitForExistence(timeout: 5), "S1.4: no Settings button on Profile")
            XCTAssertTrue(
                settings.label.contains("Settings"), "S1.4: the button reads '\(settings.label)', not 'Settings'")
        }
    }

    static func pullToRefresh(_ app: XCUIApplication, cabal: Cabal, recorder: JourneyRecorder) {
        let header = app.element("profile-header")

        recorder.step("S2.1", "open Profile") {
            openProfile(app, step: "S2.1")
        }

        recorder.step("S2.2", "pull down and keep every section") {
            let start = header.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.2))
            start.press(forDuration: 0.1, thenDragTo: start.withOffset(CGVector(dx: 0, dy: 400)))
            XCTAssertTrue(
                waitUntil(15) {
                    header.exists && app.element("profile-stat-cabals").exists && (cabalsCount(app) ?? 0) >= 1
                }, "S2.2: the header or the stats band was gone after the refresh")
            let cabalRow = row(app, cabal)
            reachCabal(app, cabalRow)
            XCTAssertTrue(
                cabalRow.waitForExistence(timeout: 15), "S2.2: the row for '\(cabal.name)' was gone after the refresh")
        }
    }

    static func openCabal(_ app: XCUIApplication, cabal: Cabal, recorder: JourneyRecorder) {
        recorder.step("S3.1", "open the cabal from Your cabals") {
            openProfile(app, step: "S3.1")
            let cabalRow = row(app, cabal)
            reachCabal(app, cabalRow)
            XCTAssertTrue(cabalRow.waitForExistence(timeout: 15), "S3.1: no row for '\(cabal.name)'")
            cabalRow.tap()
            XCTAssertTrue(
                app.element("cabal-details-button").waitForExistence(timeout: 15), "S3.1: the cabal screen did not show"
            )
            XCTAssertTrue(
                app.staticTexts[cabal.name].firstMatch.exists, "S3.1: the cabal screen does not show '\(cabal.name)'")
        }
    }

    static func openExplorer(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let advanced = app.element("settings-advanced")
        let solscan = app.element("settings-explorer-solscan")

        recorder.step("S4.1", "open Settings") {
            openProfile(app, step: "S4.1")
            let settings = app.element("profile-settings-row")
            settings.tap()
            XCTAssertTrue(app.navigationBars["Settings"].waitForExistence(timeout: 10), "S4.1: Settings did not open")
            app.scrollIntoReach(advanced)
            XCTAssertTrue(
                advanced.label.contains("Block explorers"),
                "S4.1: the row reads '\(advanced.label)', not 'Block explorers'")
        }

        recorder.step("S4.2", "open Advanced") {
            advanced.tap()
            XCTAssertTrue(
                app.navigationBars["Block explorers"].waitForExistence(timeout: 5),
                "S4.2: Block explorers did not open")
            XCTAssertTrue(solscan.label.contains("Solscan explorer"), "S4.2: no 'Solscan explorer' row")
        }

        recorder.step("S4.3", "open Solscan in Safari") {
            solscan.tap()
            let safari = XCUIApplication(bundleIdentifier: "com.apple.mobilesafari")
            XCTAssertTrue(safari.wait(for: .runningForeground, timeout: 15), "S4.3: Safari did not open")
            app.activate()
        }
    }

    static func shareReturn(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S5.1", "open the return card") {
            openProfile(app, step: "S5.1")
            let button = app.buttons["profile-share-button"]
            XCTAssertTrue(button.waitForExistence(timeout: 15), "S5.1: no share button within 15 s")
            button.tap()
            XCTAssertTrue(
                app.element("share-card-image").waitForExistence(timeout: 15),
                "S5.1: the share sheet drew no card within 15 s")
        }

        recorder.step("S5.2", "copy the link") {
            app.buttons["share-card-copy"].tap()
            XCTAssertTrue(
                app.staticTexts["Link copied."].waitForExistence(timeout: 5), "S5.2: no 'Link copied.' toast within 5 s"
            )
        }

        recorder.step("S5.3", "close the sheet") {
            app.buttons["share-card-done"].tap()
            XCTAssertTrue(
                app.element("profile-header").waitForExistence(timeout: 5),
                "S5.3: the Profile tab did not come back within 5 s")
        }
    }

    static func signOut(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let button = app.buttons["profileSignOutButton"]
        let confirm = app.buttons["profile-sign-out-confirm"]
        let stay = app.buttons["profile-sign-out-cancel"]

        recorder.step("S6.1", "ask to sign out") {
            openProfile(app, step: "S6.1")
            let settings = app.element("profile-settings-row")
            settings.tap()
            XCTAssertTrue(app.navigationBars["Settings"].waitForExistence(timeout: 10), "S6.1: Settings did not open")
            app.scrollIntoReach(button)
            button.tap()
            XCTAssertTrue(confirm.waitForExistence(timeout: 5), "S6.1: Sign out did not ask to confirm")
            XCTAssertTrue(app.staticTexts[signOutTitle].exists, "S6.1: the confirm does not read '\(signOutTitle)'")
            XCTAssertTrue(app.staticTexts[signOutMessage].exists, "S6.1: the confirm does not read '\(signOutMessage)'")
        }

        recorder.step("S6.2", "stay signed in") {
            XCTAssertTrue(stay.exists, "S6.2: the confirm has no 'Stay signed in' button")
            stay.tap()
            XCTAssertTrue(confirm.waitForNonExistence(timeout: 5), "S6.2: 'Stay signed in' did not close the confirm")
            XCTAssertTrue(
                app.navigationBars["Settings"].exists, "S6.2: Settings is gone after the confirm closed")
        }

        recorder.step("S6.3", "sign out") {
            app.scrollIntoReach(button)
            button.tap()
            XCTAssertTrue(confirm.waitForExistence(timeout: 5), "S6.3: Sign out did not ask to confirm")
            confirm.tap()
            XCTAssertTrue(app.tab("Home").waitForNonExistence(timeout: 15), "S6.3: the tab bar stayed after sign-out")
            XCTAssertEqual(SignInJourney.currentScreen(app), .login, "S6.3: the login form did not show")
        }
    }

    static func totals(_ app: XCUIApplication, cabal: Cabal, recorder: JourneyRecorder) {
        recorder.step("S7.1", "the identity card shows the member's totals") {
            openProfile(app, step: "S7.1")
            let positions = app.element("profile-stat-in-cabals")
            let performance = app.element("profile-stat-return")
            let pending = ["", "loading", "unavailable"]
            XCTAssertTrue(
                waitUntil(15) { !pending.contains(positions.value as? String ?? "") },
                "S7.1: 'Positions value' reads '\(positions.value as? String ?? "")', not a dollar figure")
            XCTAssertTrue(
                (positions.value as? String)?.hasPrefix("$") == true,
                "S7.1: 'Positions value' reads '\(positions.value as? String ?? "")', not a dollar figure")
            XCTAssertFalse(
                pending.contains(performance.value as? String ?? ""),
                "S7.1: 'Total P&L' reads '\(performance.value as? String ?? "")', not a figure")
        }

        recorder.step("S7.2", "the cabal card shows its value") {
            let card = app.element("cabal-cover-card-\(cabal.id)")
            reachCabal(app, card)
            XCTAssertTrue(card.waitForExistence(timeout: 15), "S7.2: no card for '\(cabal.name)'")
            XCTAssertTrue(card.label.contains("your share $"), "S7.2: the card reads '\(card.label)', not a share")
        }
    }
}
