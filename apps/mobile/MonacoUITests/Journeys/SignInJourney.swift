import XCTest

private typealias S = SignInJourneySteps

enum QASignOutOnLaunch {
    static let key = "MONACO_QA_SIGN_OUT_ON_LAUNCH"
}

enum SignInJourney {
    private static let launchTimeout: TimeInterval = 30

    enum Screen {
        case login
        case invite
        case firstRunStep
        case findFriends
        case tabs
    }

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: S.id, version: S.version)
    }

    private static func addressField(_ app: XCUIApplication, _ channel: JourneyAccount.Channel) -> XCUIElement {
        app.textFields[channel == .sms ? "smsPhoneField" : "emailAddressField"]
    }

    private static let firstRunSignOuts = [
        "new-account-step-sign-in", "onboarding-handle-step-sign-out", "onboarding-phone-step-sign-out",
        "onboarding-socials-step-sign-out",
    ]

    static func currentScreen(_ app: XCUIApplication, timeout: TimeInterval = launchTimeout) -> Screen? {
        let home: [(XCUIElement, Screen)] = [(app.tab("Home"), .tabs)]
        let firstRun: [(XCUIElement, Screen)] = firstRunSignOuts.map { (app.buttons[$0], Screen.firstRunStep) }
        let others: [(XCUIElement, Screen)] = [
            (app.buttons["friends-not-now"], .findFriends),
            (app.textFields["smsPhoneField"], .login),
            (app.textFields["emailAddressField"], .login),
            (app.element("invite-paste-screen"), .invite),
        ]
        let candidates = home + firstRun + others
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
        if detectLoginNeedingSignOut(app, recorder: recorder) {
            signOut(app, recorder: recorder)
        }
    }

    private static func detectLoginNeedingSignOut(_ app: XCUIApplication, recorder: JourneyRecorder) -> Bool {
        app.dismissPushPrePromptIfShown(timeout: 1)
        switch currentScreen(app) {
        case .login:
            return false
        case .invite:
            app.buttons["invite-skip-button"].tap()
            return detectLoginNeedingSignOut(app, recorder: recorder)
        case .findFriends:
            app.buttons["friends-not-now"].tap()
            return detectLoginNeedingSignOut(app, recorder: recorder)
        case .tabs:
            return true
        case .firstRunStep:
            let signOut = firstRunSignOuts.map { app.buttons[$0] }.first(where: \.exists)
            signOut?.tap()
            XCTAssertEqual(
                currentScreen(app), .login, "P1: Sign out on the first-run step did not reach the login form")
            return false
        case nil:
            XCTFail("P1: neither the login form nor the tab bar showed within \(Int(launchTimeout)) s")
            return false
        }
    }

    static func switchActor(_ app: XCUIApplication, to account: JourneyAccount, viaUI: Bool) {
        let recorder = recorder()
        recorder.step("P1.launch", "get the app running") {
            if viaUI {
                switch app.state {
                case .notRunning, .unknown:
                    app.launch()
                case .runningForeground:
                    break
                default:
                    app.activate()
                }
            } else {
                app.terminate()
                app.launchEnvironment[QASignOutOnLaunch.key] = "1"
                app.launch()
                app.launchEnvironment.removeValue(forKey: QASignOutOnLaunch.key)
            }
        }
        if viaUI { app.revealTabBar() }
        var needsSignOut = false
        recorder.step("P1.detect", "find the login form or the tab bar") {
            needsSignOut = detectLoginNeedingSignOut(app, recorder: recorder)
        }
        recorder.step("P1.signout", "sign out to the login form") {
            if needsSignOut { signOut(app, recorder: recorder) }
        }
        signIn(app, as: account, recorder: recorder)
    }

    static func enterCode(_ app: XCUIApplication, as account: JourneyAccount, recorder: JourneyRecorder) {
        let prefix = account.channel.rawValue

        recorder.step(S.s1p1) {
            let field = addressField(app, account.channel)
            if !field.exists {
                let segment = app.buttons[account.channel == .sms ? "Text message" : "Email"]
                XCTAssertTrue(
                    segment.waitForExistence(timeout: S.s1p1.seconds),
                    "\(S.s1p1.id): no '\(segment.label)' sign-in method on the login form"
                )
                segment.tap()
            }
            XCTAssertTrue(
                field.waitForExistence(timeout: S.s1p1.seconds),
                "\(S.s1p1.id): the \(prefix) address field did not show")
        }

        recorder.step(S.s1p2) {
            let field = addressField(app, account.channel)
            field.tap()
            field.typeText(account.address)
            let send = app.buttons["\(prefix)SendCodeButton"]
            XCTAssertTrue(send.waitForExistence(timeout: 5), "\(S.s1p2.id): no Send code button")
            XCTAssertEqual(send.label, S.s1p2.copy[0], "\(S.s1p2.id): Send code button has an unexpected label")
            XCTAssertTrue(send.isEnabled, "\(S.s1p2.id): Send code stayed disabled for '\(account.address)'")
            send.tap()
        }

        recorder.step(S.s1p3) {
            let codeField = app.textFields["\(prefix)CodeField"]
            XCTAssertTrue(
                codeField.waitForExistence(timeout: S.s1p3.seconds),
                "\(S.s1p3.id): the code field did not show within \(Int(S.s1p3.seconds)) s of Send code"
            )
            codeField.tap()
            codeField.typeText(account.code)
        }
    }

    static func signIn(_ app: XCUIApplication, as account: JourneyAccount, recorder: JourneyRecorder) {
        enterCode(app, as: account, recorder: recorder)
        let prefix = account.channel.rawValue

        var landed: Screen?
        recorder.step(S.s1p4) {
            landed = currentScreen(app, timeout: S.s1p4.seconds)
            XCTAssertNotEqual(
                landed, .firstRunStep,
                "\(S.s1p4.id): the first-run gate opened the handle or phone step. Actor A needs a handle and an auth_state past CREATED (P4)"
            )
            XCTAssertTrue(
                landed == .tabs || landed == .findFriends,
                "\(S.s1p4.id): the tab bar did not show within \(Int(S.s1p4.seconds)) s of the code")
        }

        recorder.step(S.s1p5) {
            if landed == .findFriends {
                app.buttons["friends-not-now"].tap()
            }
            XCTAssertTrue(
                app.tab("Home").waitForExistence(timeout: S.s1p5.seconds),
                "\(S.s1p5.id): the tab bar did not show within \(Int(S.s1p5.seconds)) s of skipping Find friends")
            XCTAssertFalse(
                app.textFields["\(prefix)CodeField"].exists, "\(S.s1p5.id): the code field is still on screen")
        }

        recorder.step(S.s1p6) {
            let titles = ["Home", "Cabals", "Markets", "Feed", "Profile"]
            for title in titles {
                XCTAssertTrue(app.tab(title).exists, "\(S.s1p6.id): no \(title) tab")
            }
            let leftToRight = titles.sorted { app.tab($0).frame.minX < app.tab($1).frame.minX }
            XCTAssertEqual(
                leftToRight, titles, "\(S.s1p6.id): the tabs are not Home, Cabals, Markets, Feed, Profile in order")
        }
    }

    static func signOut(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let signOut = app.buttons["profileSignOutButton"]
        app.dismissPushPrePromptIfShown()

        recorder.step(S.s3p1) {
            openSettings(app, step: S.s3p1.id)
            app.scrollIntoReach(signOut)
            XCTAssertTrue(signOut.waitForExistence(timeout: 15), "\(S.s3p1.id): no Sign out row in Settings")
        }

        let confirm = app.buttons["profile-sign-out-confirm"].firstMatch

        recorder.step(S.s3p2) {
            app.scrollIntoReach(signOut)
            signOut.tap()
            XCTAssertTrue(
                confirm.waitForExistence(timeout: S.s3p2.seconds), "\(S.s3p2.id): Sign out did not ask to confirm")
        }

        recorder.step(S.s3p3) {
            confirm.tap()
            XCTAssertTrue(
                app.tab("Home").waitForNonExistence(timeout: S.s3p3.seconds),
                "\(S.s3p3.id): the tab bar remained on screen within \(Int(S.s3p3.seconds)) s after sign-out"
            )
            if currentScreen(app) == .invite {
                app.buttons["invite-skip-button"].tap()
            }
            XCTAssertEqual(currentScreen(app), .login, "\(S.s3p3.id): the login form did not come back after sign-out")
            XCTAssertFalse(app.tab("Home").exists, "\(S.s3p3.id): the tab bar is still on screen after sign-out")
        }
    }

    static func signOutEverywhere(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let row = app.buttons["settings-sign-out-everywhere"]
        let confirm = app.buttons["profile-sign-out-everywhere-confirm"].firstMatch

        recorder.step(S.s3p5) {
            app.dismissPushPrePromptIfShown()
            openSettings(app, step: S.s3p5.id)
            app.scrollIntoReach(row)
            XCTAssertTrue(
                row.waitForExistence(timeout: 15), "\(S.s3p5.id): no 'Sign out of all devices' row in Settings")
            row.tap()
            XCTAssertTrue(confirm.waitForExistence(timeout: 5), "\(S.s3p5.id): the row did not ask to confirm")
            XCTAssertTrue(
                app.staticTexts[S.s3p5.copy[1]].exists,
                "\(S.s3p5.id): the confirm does not read 'Sign out of all devices?'")
        }

        recorder.step(S.s3p6) {
            confirm.tap()
            XCTAssertTrue(
                app.tab("Home").waitForNonExistence(timeout: S.s3p6.seconds),
                "\(S.s3p6.id): the tab bar remained on screen within \(Int(S.s3p6.seconds)) s after signing out of all devices"
            )
            XCTAssertEqual(currentScreen(app), .login, "\(S.s3p6.id): the login form did not come back")
            XCTAssertFalse(app.tab("Home").exists, "\(S.s3p6.id): the tab bar is still on screen")
        }
    }

    private static func openSettings(_ app: XCUIApplication, step: String) {
        app.waitForToastGone()
        app.dismissPushPrePromptIfShown()
        app.popToRoot()
        app.dismissKeyboard()
        app.tab("Profile").tap()
        if !app.element("profile-card").waitForExistence(timeout: 5) {
            app.tab("Profile").tap()
        }
        let settings = app.element("profile-settings-row")
        app.scrollIntoReach(settings, maxSwipes: 30)
        XCTAssertTrue(settings.waitForExistence(timeout: 15), "\(step): no Settings row on Profile")
        settings.tap()
        XCTAssertTrue(app.navigationBars["Settings"].waitForExistence(timeout: 10), "\(step): Settings did not open")
    }

    static func ensureSignedIn(_ app: XCUIApplication, as account: JourneyAccount) {
        let recorder = recorder()
        startSignedOut(app, recorder: recorder)
        signIn(app, as: account, recorder: recorder)
    }
}
