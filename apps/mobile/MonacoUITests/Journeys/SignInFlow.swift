import XCTest

enum SignInFlow {
    static let id = "auth/sign-in"
    static let version = 1

    private static let launchTimeout: TimeInterval = 30
    private static let codeSentTimeout: TimeInterval = 20
    private static let signedInTimeout: TimeInterval = 30

    enum Screen {
        case login
        case tabs
    }

    static func recorder() -> FlowRecorder {
        FlowRecorder(flow: id, version: version)
    }

    private static func addressField(_ app: XCUIApplication, _ channel: FlowAccount.Channel) -> XCUIElement {
        app.textFields[channel == .sms ? "smsPhoneField" : "emailAddressField"]
    }

    static func currentScreen(_ app: XCUIApplication, timeout: TimeInterval = launchTimeout) -> Screen? {
        let candidates = [
            app.tab("Home"),
            app.textFields["smsPhoneField"],
            app.textFields["emailAddressField"],
        ]
        guard let index = app.waitForFirst(of: candidates, timeout: timeout) else { return nil }
        return index == 0 ? .tabs : .login
    }

    static func startSignedOut(_ app: XCUIApplication, recorder: FlowRecorder) {
        recorder.step("P1", "launch to the login form") {
            app.launch()
            switch currentScreen(app) {
            case .login:
                return
            case .tabs:
                signOut(app, recorder: recorder)
            case nil:
                XCTFail("P1: neither the login form nor the tab bar showed within \(Int(launchTimeout)) s of launch")
            }
        }
    }

    static func signIn(_ app: XCUIApplication, as account: FlowAccount, recorder: FlowRecorder) {
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

        recorder.step("S1.4", "land on the tab bar") {
            XCTAssertTrue(
                app.tab("Home").waitForExistence(timeout: signedInTimeout),
                "S1.4: the tab bar did not show within \(Int(signedInTimeout)) s of the code"
            )
            XCTAssertFalse(app.textFields["\(prefix)CodeField"].exists, "S1.4: the code field is still on screen")
            for title in ["Home", "Feed", "Cabals", "Stocks", "Profile"] {
                XCTAssertTrue(app.tab(title).exists, "S1.4: no \(title) tab")
            }
        }
    }

    static func signOut(_ app: XCUIApplication, recorder: FlowRecorder) {
        let signOut = app.buttons["profileSignOutButton"]

        recorder.step("S3.1", "open Profile") {
            app.tab("Profile").tap()
            XCTAssertTrue(signOut.waitForExistence(timeout: 15), "S3.1: no Sign out button on Profile")
        }

        recorder.step("S3.2", "tap Sign out and land on the login form") {
            app.scrollIntoReach(signOut)
            signOut.tap()
            XCTAssertTrue(
                app.tab("Home").waitForNonExistence(timeout: signedInTimeout),
                "S3.2: the tab bar remained on screen within \(Int(signedInTimeout)) s after sign-out"
            )
            XCTAssertEqual(currentScreen(app), .login, "S3.2: the login form did not come back after sign-out")
            XCTAssertFalse(app.tab("Home").exists, "S3.2: the tab bar is still on screen after sign-out")
        }
    }

    static func ensureSignedIn(_ app: XCUIApplication, as account: FlowAccount) {
        let recorder = recorder()
        startSignedOut(app, recorder: recorder)
        signIn(app, as: account, recorder: recorder)
    }
}
