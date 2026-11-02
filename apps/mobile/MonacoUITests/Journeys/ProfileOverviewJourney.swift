import XCTest

enum ProfileOverviewJourney {
    static let id = "profile/overview"
    static let version = 13

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
            app.element("profile-card").waitForExistence(timeout: 15), "\(step): the Profile card did not show")
    }

    static func sections(_ app: XCUIApplication, cabal: Cabal, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open Profile and read the header") {
            openProfile(app, step: "S1.1")
            XCTAssertFalse(
                app.staticTexts["profile-display-name"].label.isEmpty, "S1.1: no display name on the header")
            let handle = app.element("profile-handle")
            XCTAssertTrue(
                handle.exists && handle.label.hasPrefix("@"), "S1.1: the handle does not read '@' and the handle")
            XCTAssertTrue(
                handle.label.contains("joined"),
                "S1.1: the handle line reads '\(handle.label)', not '@handle · joined …'")
        }

        recorder.step("S1.2", "read the identity card stats") {
            XCTAssertTrue(
                waitUntil(15) { (cabalsCount(app) ?? 0) >= 1 },
                "S1.2: 'Cabals' reads '\(app.element("profile-stat-cabals").label)', not a count of 1 or more")
            XCTAssertTrue(
                app.element("profile-positions").staticTexts["Positions"].exists,
                "S1.2: no 'Positions' column")
            XCTAssertTrue(
                app.element("profile-pnl-1m").label.contains("P&L \u{00B7} 1M"), "S1.2: no 'P&L · 1M' column")
            XCTAssertTrue(
                app.element("profile-stat-cabals").label.contains("cabal"), "S1.2: no 'cabal' or 'cabals' count")
            if (try? JourneyHandoff.read("hasStanding")) == "1" {
                let standing = app.element("profile-standing")
                XCTAssertTrue(standing.waitForExistence(timeout: 15), "S1.2: no month standing line")
                XCTAssertTrue(
                    standing.label.contains("right calls this month") || standing.label.contains("resolved"),
                    "S1.2: the standing line reads '\(standing.label)', not the month's right calls or the unranked line"
                )
            }
        }

        recorder.step("S1.3", "find the cabal under Your cabals") {
            tapCabalsTab(app)
            let cabalRow = row(app, cabal)
            reachCabal(app, cabalRow)
            XCTAssertTrue(cabalRow.waitForExistence(timeout: 10), "S1.3: no row for '\(cabal.name)' on the Cabals tab")
            XCTAssertTrue(
                cabalRow.label.contains(cabal.name), "S1.3: the row reads '\(cabalRow.label)', not '\(cabal.name)'")
            XCTAssertTrue(
                cabalRow.label.contains("admin") || cabalRow.label.contains("voter"),
                "S1.3: the row reads '\(cabalRow.label)', with no 'admin' or 'voter' role")
        }

        recorder.step("S1.4", "find the Settings button") {
            let settings = app.element("profile-settings-row")
            XCTAssertTrue(settings.waitForExistence(timeout: 5), "S1.4: no Settings button on Profile")
            XCTAssertTrue(
                settings.label.contains("Settings"), "S1.4: the button reads '\(settings.label)', not 'Settings'")
        }

        recorder.step("S1.5", "read the cash row and the Deposit dock") {
            let cash = app.element("profile-cash-row")
            XCTAssertTrue(cash.waitForExistence(timeout: 10), "S1.5: no cash row")
            for text in ["Cash", "USDC"] {
                XCTAssertTrue(cash.label.contains(text), "S1.5: the cash row reads '\(cash.label)', with no '\(text)'")
            }
            XCTAssertFalse(app.staticTexts["3.5% APY"].exists, "S1.5: APY text beside the cash row")
            XCTAssertFalse(app.element("profile-apy-info").exists, "S1.5: an APY info button on the cash row")
            let dock = app.element("profile-add-money-link")
            XCTAssertTrue(dock.waitForExistence(timeout: 5), "S1.5: no Deposit dock above the tab bar")
            XCTAssertTrue(dock.label.contains("Deposit"), "S1.5: the dock reads '\(dock.label)', not 'Deposit'")
        }
    }

    static func pullToRefresh(_ app: XCUIApplication, cabal: Cabal, recorder: JourneyRecorder) {
        let header = app.element("profile-card")

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
            tapCabalsTab(app)
            let cabalRow = row(app, cabal)
            reachCabal(app, cabalRow)
            XCTAssertTrue(
                cabalRow.waitForExistence(timeout: 15), "S2.2: the row for '\(cabal.name)' was gone after the refresh")
        }
    }

    static func openCabal(_ app: XCUIApplication, cabal: Cabal, recorder: JourneyRecorder) {
        recorder.step("S3.1", "open the cabal from Your cabals") {
            openProfile(app, step: "S3.1")
            tapCabalsTab(app)
            let cabalRow = row(app, cabal)
            reachCabal(app, cabalRow)
            XCTAssertTrue(cabalRow.waitForExistence(timeout: 15), "S3.1: no row for '\(cabal.name)'")
            cabalRow.tap()
            XCTAssertTrue(
                app.element("cabal-header-name").waitForExistence(timeout: 15), "S3.1: the cabal screen did not show"
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
        recorder.step("S5.1", "open the share screen") {
            openProfile(app, step: "S5.1")
            let button = app.buttons["profile-share-button"]
            XCTAssertTrue(button.waitForExistence(timeout: 15), "S5.1: no share button within 15 s")
            button.tap()
            XCTAssertTrue(
                app.element("share-screen").waitForExistence(timeout: 15),
                "S5.1: the share screen did not open within 15 s")
        }

        recorder.step("S5.2", "close the share screen") {
            app.tapBack()
            XCTAssertTrue(
                app.element("profile-card").waitForExistence(timeout: 5),
                "S5.2: the Profile tab did not come back within 5 s")
        }
    }

    static func signOut(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let button = app.buttons["profileSignOutButton"]
        let confirm = app.buttons["profile-sign-out-confirm"]
        let stay = app.buttons["profile-sign-out-cancel"]
        let everywhere = app.buttons["settings-sign-out-everywhere"]
        let everywhereConfirm = app.buttons["profile-sign-out-everywhere-confirm"]
        let everywhereStay = app.buttons["profile-sign-out-everywhere-cancel"]

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

        recorder.step("S6.3", "ask to sign out of all devices") {
            app.scrollIntoReach(everywhere)
            XCTAssertTrue(everywhere.waitForExistence(timeout: 5), "S6.3: no 'Sign out of all devices' row in Settings")
            everywhere.tap()
            XCTAssertTrue(everywhereConfirm.waitForExistence(timeout: 5), "S6.3: the row did not ask to confirm")
            XCTAssertTrue(
                app.staticTexts[signOutEverywhereTitle].exists,
                "S6.3: the confirm does not read '\(signOutEverywhereTitle)'")
            XCTAssertTrue(
                app.staticTexts.matching(NSPredicate(format: "label == %@", signOutEverywhereMessage))
                    .firstMatch.exists,
                "S6.3: the confirm does not read '\(signOutEverywhereMessage)'")
            XCTAssertTrue(everywhereStay.exists, "S6.3: the confirm has no 'Stay signed in' button")
        }

        recorder.step("S6.4", "stay signed in") {
            everywhereStay.tap()
            XCTAssertTrue(
                everywhereConfirm.waitForNonExistence(timeout: 5), "S6.4: 'Stay signed in' did not close the confirm")
            XCTAssertTrue(
                app.navigationBars["Settings"].exists, "S6.4: Settings is gone after the confirm closed")
        }

        recorder.step("S6.5", "sign out") {
            app.scrollIntoReach(button)
            button.tap()
            XCTAssertTrue(confirm.waitForExistence(timeout: 5), "S6.5: Sign out did not ask to confirm")
            confirm.tap()
            XCTAssertTrue(app.tab("Home").waitForNonExistence(timeout: 15), "S6.5: the tab bar stayed after sign-out")
            if SignInJourney.currentScreen(app) == .invite {
                app.buttons["invite-skip-button"].tap()
            }
            XCTAssertEqual(SignInJourney.currentScreen(app), .login, "S6.5: the login form did not show")
        }
    }

    static func totals(_ app: XCUIApplication, cabal: Cabal, recorder: JourneyRecorder) {
        recorder.step("S7.1", "the identity card shows the member's totals") {
            openProfile(app, step: "S7.1")
            let performance = app.element("profile-pnl-1m")
            let pending = ["", "loading", "unavailable"]
            XCTAssertTrue(
                waitUntil(15) { positionsFigure(app).hasPrefix("$") },
                "S7.1: 'Positions' reads '\(positionsFigure(app))', not a dollar figure")
            XCTAssertFalse(
                pending.contains(performance.value as? String ?? ""),
                "S7.1: 'P&L · 1M' reads '\(performance.value as? String ?? "")', not a figure")
        }

        recorder.step("S7.2", "the cabal card shows its value") {
            tapCabalsTab(app)
            let card = row(app, cabal)
            reachCabal(app, card)
            XCTAssertTrue(card.waitForExistence(timeout: 15), "S7.2: no row for '\(cabal.name)'")
            XCTAssertTrue(card.label.contains("$"), "S7.2: the row reads '\(card.label)', not a share in dollars")
        }
    }
}
