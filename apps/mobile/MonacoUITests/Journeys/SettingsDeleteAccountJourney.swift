import XCTest

enum SettingsDeleteAccountJourney {
    static let id = "settings/delete-account"
    static let version = 3

    static let explainer =
        "Deleting your account removes your name, photo, phone and X from Monaco. Your handle stays reserved. "
        + "Your transaction history stays, because cabal records need it. This can't be undone."
    static let cashOutStep = "Cash out of every cabal"
    static let withdrawStep = "Withdraw your balance"
    static let noCabalMoney = "No cabal holds money of yours."
    static let confirmTitle = "Delete your Monaco account?"
    static let deleted = "Your account was deleted."
    static let sliceComingSoon = "Your slice in each cabal shows up here soon."

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func waitUntil(_ timeout: TimeInterval, _ condition: () -> Bool) -> Bool {
        FirstRunJourney.waitUntil(timeout, condition)
    }

    static func stepText(_ app: XCUIApplication, _ identifier: String) -> String {
        let step = app.element(identifier)
        return step.descendants(matching: .any).allElementsBoundByIndex.map(\.label).filter { !$0.isEmpty }
            .joined(separator: " | ")
    }

    static func devSignIn(_ app: XCUIApplication, devToken: String, step: String) {
        app.launchEnvironment["MONACO_DEV_TOKEN"] = devToken
        SignInJourney.startSignedOut(app, recorder: SignInJourney.recorder())
        let devSignIn = app.buttons["devSignInButton"]
        XCTAssertTrue(
            devSignIn.waitForExistence(timeout: 30),
            "\(step): no Dev: sign in button. The setup script hands the test a dev token (P1)")
        devSignIn.tap()
        XCTAssertTrue(app.tab("Home").waitForExistence(timeout: 30), "\(step): the tab bar did not show")
    }

    static func openSettings(_ app: XCUIApplication, step: String) {
        app.tab("Profile").tap()
        let settings = app.element("profile-settings-row")
        XCTAssertTrue(settings.waitForExistence(timeout: 15), "\(step): no Settings row on Profile")
        app.scrollIntoReach(settings)
        settings.tap()
        XCTAssertTrue(app.navigationBars["Settings"].waitForExistence(timeout: 10), "\(step): Settings did not open")
    }

    static func openDeleteAccount(_ app: XCUIApplication, step: String) {
        openSettings(app, step: step)
        let row = app.element("settings-delete-account")
        app.scrollIntoReach(row)
        row.tap()
        XCTAssertTrue(
            app.element("delete-account-explainer").waitForExistence(timeout: 10),
            "\(step): the Delete account screen did not show")
    }

    static func readAndBackOut(_ app: XCUIApplication, devToken: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "sign in as the throwaway dev user") {
            devSignIn(app, devToken: devToken, step: "S1.1")
        }

        recorder.step("S1.2", "open Settings") {
            openSettings(app, step: "S1.2")
            let row = app.element("settings-delete-account")
            app.scrollIntoReach(row)
            XCTAssertTrue(row.label.contains("Delete account"), "S1.2: the row reads '\(row.label)'")
        }

        recorder.step("S1.3", "open Delete account and read the explainer") {
            app.element("settings-delete-account").tap()
            let text = app.element("delete-account-explainer")
            XCTAssertTrue(text.waitForExistence(timeout: 10), "S1.3: the Delete account screen did not show")
            XCTAssertTrue(app.navigationBars["Delete account"].exists, "S1.3: no 'Delete account' title")
            XCTAssertEqual(text.label, explainer, "S1.3: the explainer does not match the spec")
        }

        recorder.step("S1.4", "both checklist steps read Done") {
            XCTAssertTrue(
                waitUntil(15) { stepText(app, "delete-account-step-cash-out").contains(noCabalMoney) },
                "S1.4: '\(cashOutStep)' reads '\(stepText(app, "delete-account-step-cash-out"))'")
            let cashOut = stepText(app, "delete-account-step-cash-out")
            XCTAssertTrue(
                cashOut.contains(cashOutStep) && cashOut.contains("Done"), "S1.4: '\(cashOutStep)' is not Done")
            let withdraw = stepText(app, "delete-account-step-withdraw")
            XCTAssertTrue(
                withdraw.contains(withdrawStep) && withdraw.contains("Done") && withdraw.contains("$0.00"),
                "S1.4: '\(withdrawStep)' reads '\(withdraw)', not Done with $0.00")
        }

        let confirm = app.confirmDialogButton("delete-account-confirm")

        recorder.step("S1.5", "ask to delete") {
            app.buttons["delete-account-button"].tap()
            XCTAssertTrue(confirm.waitForExistence(timeout: 5), "S1.5: Delete account did not ask to confirm")
            XCTAssertTrue(
                app.staticTexts[confirmTitle].firstMatch.exists, "S1.5: the confirm does not read '\(confirmTitle)'")
            XCTAssertEqual(confirm.label, "Delete", "S1.5: the confirm button does not read 'Delete'")
        }

        recorder.step("S1.6", "tap outside the confirm") {
            app.dismissConfirmDialog(title: confirmTitle)
            XCTAssertTrue(confirm.waitForNonExistence(timeout: 5), "S1.6: a tap outside did not close the confirm")
            XCTAssertTrue(app.element("delete-account-explainer").exists, "S1.6: the Delete account screen is gone")
        }
    }

    static func delete(_ app: XCUIApplication, devToken: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open Delete account as the throwaway dev user") {
            devSignIn(app, devToken: devToken, step: "S2.1")
            openDeleteAccount(app, step: "S2.1")
            XCTAssertTrue(
                waitUntil(15) { stepText(app, "delete-account-step-withdraw").contains("Done") },
                "S2.1: '\(withdrawStep)' is not Done")
        }

        recorder.step("S2.2", "delete the account") {
            app.buttons["delete-account-button"].tap()
            let confirm = app.confirmDialogButton("delete-account-confirm")
            XCTAssertTrue(confirm.waitForExistence(timeout: 5), "S2.2: Delete account did not ask to confirm")
            confirm.tap()
            let toast = app.element("monaco-toast-banner")
            XCTAssertTrue(
                waitUntil(20) { toast.exists && toast.label.contains(deleted) }, "S2.2: no '\(deleted)' toast")
            XCTAssertTrue(
                app.buttons["devSignInButton"].waitForExistence(timeout: 20), "S2.2: the login form did not show")
        }
    }

    static func cabalOnChecklist(
        _ app: XCUIApplication, devToken: String, cabalID: String, cabalName: String, recorder: JourneyRecorder
    ) {
        recorder.step("S3.1", "open Delete account as a cabal member") {
            devSignIn(app, devToken: devToken, step: "S3.1")
            openDeleteAccount(app, step: "S3.1")
        }

        recorder.step("S3.2", "the cabal is on the cash-out step") {
            let row = app.element("delete-account-cabal-\(cabalID)")
            XCTAssertTrue(row.waitForExistence(timeout: 15), "S3.2: no row for '\(cabalName)'")
            XCTAssertTrue(row.label.contains(cabalName), "S3.2: the row reads '\(row.label)', not '\(cabalName)'")
            XCTAssertFalse(
                stepText(app, "delete-account-step-cash-out").contains(noCabalMoney),
                "S3.2: '\(cashOutStep)' reads Done for a cabal member")
        }

        recorder.step("S3.3", "the cash-out step shows the member's slice") {
            XCTAssertFalse(
                stepText(app, "delete-account-step-cash-out").contains(sliceComingSoon),
                "S3.3: the step reads '\(sliceComingSoon)' instead of the slice (#2136)")
        }
    }
}
