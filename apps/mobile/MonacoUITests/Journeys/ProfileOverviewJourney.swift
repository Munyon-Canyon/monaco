import XCTest

enum ProfileOverviewJourney {
    static let id = "profile/overview"
    static let version = 2

    static let notYet = "Not available yet"
    static let signOutTitle = "Sign out of Monaco?"
    static let signOutMessage = "Your money stays where it is. You'll need a new code to sign back in."

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
        XCTAssertTrue(
            app.element("profile-header").waitForExistence(timeout: 15), "\(step): the Profile header did not show")
    }

    static func row(_ app: XCUIApplication, _ cabal: Cabal) -> XCUIElement {
        app.element("cabal-row-\(cabal.id)")
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
                since.range(of: #"^Member since \S+ \d{4}$"#, options: .regularExpression),
                "S1.1: the header reads '\(since)', not 'Member since' with a month and year")
        }

        recorder.step("S1.2", "read the stats band") {
            XCTAssertTrue(
                waitUntil(15) { (cabalsCount(app) ?? 0) >= 1 },
                "S1.2: 'Cabals' reads '\(app.element("profile-stat-cabals").label)', not a count of 1 or more")
            XCTAssertTrue(
                app.element("profile-stat-in-cabals").label.contains("In cabals"), "S1.2: no 'In cabals' column")
            XCTAssertTrue(app.element("profile-stat-all-time").label.contains("All time"), "S1.2: no 'All time' column")
            XCTAssertTrue(app.element("profile-stat-cabals").label.contains("Cabals"), "S1.2: no 'Cabals' column")
        }

        recorder.step("S1.3", "find the cabal under Your cabals") {
            let cabalRow = row(app, cabal)
            app.scrollIntoReach(cabalRow)
            XCTAssertTrue(cabalRow.waitForExistence(timeout: 10), "S1.3: no row for '\(cabal.name)' under Your cabals")
            XCTAssertTrue(app.staticTexts["Your cabals"].exists, "S1.3: no 'Your cabals' header")
            XCTAssertTrue(
                cabalRow.label.contains(cabal.name), "S1.3: the row reads '\(cabalRow.label)', not '\(cabal.name)'")
        }

        recorder.step("S1.4", "find the Settings row") {
            let settings = app.element("profile-settings-row")
            app.scrollIntoReach(settings)
            XCTAssertTrue(settings.exists, "S1.4: no Settings row on Profile")
            XCTAssertTrue(
                settings.label.contains("Settings"), "S1.4: the row reads '\(settings.label)', not 'Settings'")
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
            app.scrollIntoReach(cabalRow)
            XCTAssertTrue(
                cabalRow.waitForExistence(timeout: 15), "S2.2: the row for '\(cabal.name)' was gone after the refresh")
        }
    }

    static func openCabal(_ app: XCUIApplication, cabal: Cabal, recorder: JourneyRecorder) {
        recorder.step("S3.1", "open the cabal from Your cabals") {
            openProfile(app, step: "S3.1")
            let cabalRow = row(app, cabal)
            app.scrollIntoReach(cabalRow)
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
            app.scrollIntoReach(settings)
            settings.tap()
            XCTAssertTrue(app.navigationBars["Settings"].waitForExistence(timeout: 10), "S4.1: Settings did not open")
            XCTAssertTrue(
                advanced.label.contains("Advanced") && advanced.label.contains("Block explorers"),
                "S4.1: the row reads '\(advanced.label)', not 'Advanced' and 'Block explorers'")
        }

        recorder.step("S4.2", "open Advanced") {
            advanced.tap()
            XCTAssertTrue(app.navigationBars["Advanced"].waitForExistence(timeout: 5), "S4.2: Advanced did not open")
            XCTAssertTrue(solscan.label.contains("Solscan explorer"), "S4.2: no 'Solscan explorer' row")
        }

        recorder.step("S4.3", "open Solscan in Safari") {
            solscan.tap()
            let safari = XCUIApplication(bundleIdentifier: "com.apple.mobilesafari")
            XCTAssertTrue(safari.wait(for: .runningForeground, timeout: 15), "S4.3: Safari did not open")
            app.activate()
        }
    }

    static func signOut(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let button = app.buttons["profileSignOutButton"]
        let confirm = app.confirmDialogButton("profile-sign-out-confirm")

        recorder.step("S5.1", "ask to sign out") {
            openProfile(app, step: "S5.1")
            app.scrollIntoReach(button)
            button.tap()
            XCTAssertTrue(confirm.waitForExistence(timeout: 5), "S5.1: Sign out did not ask to confirm")
            XCTAssertTrue(app.staticTexts[signOutTitle].exists, "S5.1: the confirm does not read '\(signOutTitle)'")
            XCTAssertTrue(app.staticTexts[signOutMessage].exists, "S5.1: the confirm does not read '\(signOutMessage)'")
        }

        recorder.step("S5.2", "tap outside the confirm") {
            app.dismissConfirmDialog(title: signOutTitle)
            XCTAssertTrue(confirm.waitForNonExistence(timeout: 5), "S5.2: a tap outside did not close the confirm")
            XCTAssertTrue(
                app.element("profile-header").exists, "S5.2: the Profile header is gone after the confirm closed")
        }

        recorder.step("S5.3", "sign out") {
            app.scrollIntoReach(button)
            button.tap()
            XCTAssertTrue(confirm.waitForExistence(timeout: 5), "S5.3: Sign out did not ask to confirm")
            confirm.tap()
            XCTAssertTrue(app.tab("Home").waitForNonExistence(timeout: 15), "S5.3: the tab bar stayed after sign-out")
            XCTAssertEqual(SignInJourney.currentScreen(app), .login, "S5.3: the login form did not show")
        }
    }

    static func totals(_ app: XCUIApplication, cabal: Cabal, recorder: JourneyRecorder) {
        recorder.step("S6.1", "the band shows the member's totals") {
            openProfile(app, step: "S6.1")
            XCTAssertTrue(waitUntil(15) { cabalsCount(app) != nil }, "S6.1: the stats band did not load")
            XCTAssertFalse(
                app.element("profile-stat-in-cabals").label.contains(notYet),
                "S6.1: 'In cabals' shows no figure (#2140)")
            XCTAssertFalse(
                app.element("profile-stat-all-time").label.contains(notYet), "S6.1: 'All time' shows no figure (#2140)")
            XCTAssertFalse(
                app.element("profile-stats-coming").exists, "S6.1: 'Your totals show up here soon.' shows (#2140)")
        }

        recorder.step("S6.2", "the cabal row shows its value") {
            let cabalRow = row(app, cabal)
            app.scrollIntoReach(cabalRow)
            XCTAssertTrue(cabalRow.waitForExistence(timeout: 15), "S6.2: no row for '\(cabal.name)'")
            XCTAssertFalse(
                app.element("profile-cabals-coming").exists,
                "S6.2: 'Pot values show up here soon.' shows (#2140, #2136)")
        }
    }
}
