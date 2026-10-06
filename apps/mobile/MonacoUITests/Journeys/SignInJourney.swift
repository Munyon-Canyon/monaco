import XCTest

enum SignInJourney {
    static let id = "auth/sign-in"
    static let version = 5

    private static let launchTimeout: TimeInterval = 30
    private static let codeSentTimeout: TimeInterval = 20
    private static let signedInTimeout: TimeInterval = 30

    enum Screen {
        case login
        case firstRunStep
        case tabs
    }

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    private static func addressField(_ app: XCUIApplication, _ channel: JourneyAccount.Channel) -> XCUIElement {
        app.textFields[channel == .sms ? "smsPhoneField" : "emailAddressField"]
    }

    private static let firstRunSignOuts = [
        "onboarding-handle-step-sign-out", "onboarding-phone-step-sign-out", "onboarding-socials-step-sign-out",
    ]

    static func currentScreen(_ app: XCUIApplication, timeout: TimeInterval = launchTimeout) -> Screen? {
        let candidates: [(XCUIElement, Screen)] = [
            (app.tab("Home"), .tabs),
            (app.buttons[firstRunSignOuts[0]], .firstRunStep),
            (app.buttons[firstRunSignOuts[1]], .firstRunStep),
            (app.buttons[firstRunSignOuts[2]], .firstRunStep),
            (app.textFields["smsPhoneField"], .login),
            (app.textFields["emailAddressField"], .login),
        ]
        guard let index = app.waitForFirst(of: candidates.map(\.0), timeout: timeout) else { return nil }
        return candidates[index].1
    }

    static func startSignedOut(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("P1", "launch to the login form") {
            app.launch()
            reachLogin(app, recorder: recorder)
        }
    }

    private static func reachLogin(_ app: XCUIApplication, recorder: JourneyRecorder) {
        switch currentScreen(app) {
        case .login:
            return
        case .tabs:
            signOut(app, recorder: recorder)
        case .firstRunStep:
            let signOut = firstRunSignOuts.map { app.buttons[$0] }.first(where: \.exists)
            signOut?.tap()
            XCTAssertEqual(
                currentScreen(app), .login, "P1: Sign out on the first-run step did not reach the login form")
        case nil:
            XCTFail("P1: neither the login form nor the tab bar showed within \(Int(launchTimeout)) s")
        }
    }

    static func switchActor(_ app: XCUIApplication, to account: JourneyAccount) {
        let recorder = recorder()
        recorder.step("P1", "sign out to the login form") {
            switch app.state {
            case .notRunning, .unknown:
                app.launch()
            case .runningForeground:
                break
            default:
                app.activate()
            }
            reachLogin(app, recorder: recorder)
        }
        signIn(app, as: account, recorder: recorder)
    }

    static func enterCode(_ app: XCUIApplication, as account: JourneyAccount, recorder: JourneyRecorder) {
        let prefix = account.channel.rawValue

        recorder.step("S1.1", "choose the \(prefix) method") {
            let field = addressField(app, account.channel)
            if !field.exists {
                let segment = app.buttons[account.channel == .sms ? "Text message" : "Email"]
                XCTAssertTrue(
                    segment.waitForExistence(timeout: 5), "S1.1: no '\(segment.label)' sign-in method on the login form"
                )
                segment.tap()
            }
            XCTAssertTrue(field.waitForExistence(timeout: 5), "S1.1: the \(prefix) address field did not show")
        }

        recorder.step("S1.2", "enter the address and send the code") {
            let field = addressField(app, account.channel)
            field.tap()
            field.typeText(account.address)
            let send = app.buttons["\(prefix)SendCodeButton"]
            XCTAssertTrue(send.waitForExistence(timeout: 5), "S1.2: no Send code button")
            XCTAssertEqual(send.label, "Send code", "S1.2: Send code button has an unexpected label")
            XCTAssertTrue(send.isEnabled, "S1.2: Send code stayed disabled for '\(account.address)'")
            send.tap()
        }

        recorder.step("S1.3", "enter the code") {
            let codeField = app.textFields["\(prefix)CodeField"]
            XCTAssertTrue(
                codeField.waitForExistence(timeout: codeSentTimeout),
                "S1.3: the code field did not show within \(Int(codeSentTimeout)) s of Send code"
            )
            codeField.tap()
            codeField.typeText(account.code)
        }
    }

    static func signIn(_ app: XCUIApplication, as account: JourneyAccount, recorder: JourneyRecorder) {
        enterCode(app, as: account, recorder: recorder)
        let prefix = account.channel.rawValue

        recorder.step("S1.4", "open the backend session and land on the tab bar") {
            let landed = currentScreen(app, timeout: signedInTimeout)
            XCTAssertNotEqual(
                landed, .firstRunStep,
                "S1.4: the first-run gate opened the handle or phone step. Actor A needs a handle and an auth_state past CREATED (P4)"
            )
            XCTAssertEqual(
                landed, .tabs, "S1.4: the tab bar did not show within \(Int(signedInTimeout)) s of the code")
            XCTAssertFalse(app.textFields["\(prefix)CodeField"].exists, "S1.4: the code field is still on screen")
        }

        recorder.step("S1.6", "the tab bar has every tab") {
            for title in ["Home", "Feed", "Cabals", "Stocks", "Profile"] {
                XCTAssertTrue(app.tab(title).exists, "S1.6: no \(title) tab")
            }
        }
    }

    static func signOut(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let signOut = app.buttons["profileSignOutButton"]

        recorder.step("S3.1", "open Profile") {
            app.waitForToastGone()
            app.popToRoot()
            app.tab("Profile").tap()
            XCTAssertTrue(signOut.waitForExistence(timeout: 15), "S3.1: no Sign out button on Profile")
        }

        let confirm = app.buttons["profile-sign-out-confirm"].firstMatch

        recorder.step("S3.2", "tap Sign out and see the confirm") {
            app.scrollIntoReach(signOut)
            signOut.tap()
            XCTAssertTrue(confirm.waitForExistence(timeout: 5), "S3.2: Sign out did not ask to confirm")
        }

        recorder.step("S3.3", "confirm and land on the login form") {
            confirm.tap()
            XCTAssertTrue(
                app.tab("Home").waitForNonExistence(timeout: signedInTimeout),
                "S3.3: the tab bar remained on screen within \(Int(signedInTimeout)) s after sign-out"
            )
            XCTAssertEqual(currentScreen(app), .login, "S3.3: the login form did not come back after sign-out")
            XCTAssertFalse(app.tab("Home").exists, "S3.3: the tab bar is still on screen after sign-out")
        }
    }

    static func ensureSignedIn(_ app: XCUIApplication, as account: JourneyAccount) {
        let recorder = recorder()
        startSignedOut(app, recorder: recorder)
        signIn(app, as: account, recorder: recorder)
    }
}
