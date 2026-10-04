import XCTest

enum FirstRunJourney {
    static let id = "onboarding/first-run"
    static let version = 1

    static let handle = "qa_cayman"
    static let addPhoneNudge = "Add your number to find friends"
    static let linkXNudge = "Connect X to find people you follow"

    private static let signedInTimeout: TimeInterval = 30
    private static let stepTimeout: TimeInterval = 10
    private static let checkTimeout: TimeInterval = 5
    private static let linkTimeout: TimeInterval = 20

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func member() throws -> JourneyAccount {
        try JourneyAccount.load(actor: "C", channel: .email)
    }

    static func linkNumber() throws -> JourneyAccount {
        try JourneyAccount.load(actor: "L", channel: .sms)
    }

    static func otherMember() throws -> JourneyAccount {
        try JourneyAccount.load(actor: "B", channel: .sms)
    }

    static func readBack(_ phone: String) -> String {
        let digits = Array(phone)
        guard digits.count == 10 else { return phone }
        return "+1 \(String(digits[0..<3])) \(String(digits[3..<6])) \(String(digits[6..<10]))"
    }

    static func waitUntil(_ timeout: TimeInterval, _ condition: () -> Bool) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if condition() { return true }
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        } while Date() < deadline
        return condition()
    }

    static func nudge(_ app: XCUIApplication) -> XCUIElement {
        app.buttons["onboarding-nudge-open"]
    }

    static func waitForNudge(_ app: XCUIApplication, reading message: String, timeout: TimeInterval) -> Bool {
        waitUntil(timeout) { nudge(app).exists && nudge(app).label == message }
    }

    static func onHandleStep(_ app: XCUIApplication) -> Bool {
        app.textFields["handle-step-field"].exists
    }

    static func onPhoneStep(_ app: XCUIApplication) -> Bool {
        app.textFields["phone-step-number-field"].exists || app.textFields["phone-step-code-field"].exists
    }

    static func onSocialsStep(_ app: XCUIApplication) -> Bool {
        app.buttons["socials-step-connect"].exists
    }

    static func pickHandleAndSkip(_ app: XCUIApplication, as member: JourneyAccount, takenHandle: String) {
        let recorder = recorder()
        let field = app.textFields["handle-step-field"]
        let status = app.element("handle-step-status")
        let continueButton = app.buttons["onboarding-handle-step-continue"]

        recorder.step("S1.1", "sign in by email and land on the handle step") {
            SignInJourney.startSignedOut(app, recorder: SignInJourney.recorder())
            SignInJourney.enterCode(app, as: member, recorder: SignInJourney.recorder())
            XCTAssertTrue(
                field.waitForExistence(timeout: signedInTimeout),
                "S1.1: the handle step did not show within \(Int(signedInTimeout)) s of the code. C must be a new member (P2)"
            )
            XCTAssertFalse(app.tab("Home").exists, "S1.1: the tab bar showed before the handle step")
            XCTAssertTrue(app.staticTexts["Pick your handle"].exists, "S1.1: no 'Pick your handle' title")
            XCTAssertEqual(
                app.staticTexts["handle-step-subtext"].label, "This is how people find you on Monaco.",
                "S1.1: the handle step subtext is not the first-run copy")
        }

        recorder.step("S1.2", "a taken handle is refused") {
            field.tap()
            field.typeText(takenHandle)
            XCTAssertTrue(
                waitUntil(checkTimeout) { status.label == "That handle is taken." },
                "S1.2: '\(takenHandle)' did not read 'That handle is taken.' within \(Int(checkTimeout)) s, it read '\(status.label)'"
            )
            XCTAssertFalse(continueButton.isEnabled, "S1.2: Continue is enabled for a taken handle")
        }

        recorder.step("S1.3", "a free handle is accepted") {
            field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: takenHandle.count))
            field.typeText(handle)
            XCTAssertTrue(
                waitUntil(checkTimeout) { status.label == "@\(handle) is available" },
                "S1.3: '\(handle)' did not read as available within \(Int(checkTimeout)) s, it read '\(status.label)'"
            )
            XCTAssertTrue(
                waitUntil(checkTimeout) { continueButton.isEnabled },
                "S1.3: Continue stayed disabled for an available handle")
        }

        recorder.step("S1.4", "Continue saves the handle and opens the phone step") {
            continueButton.tap()
            XCTAssertTrue(
                waitUntil(stepTimeout) { onPhoneStep(app) },
                "S1.4: the phone step did not show within \(Int(stepTimeout)) s of Continue")
            XCTAssertTrue(app.staticTexts["Add your number"].exists, "S1.4: no 'Add your number' title")
            XCTAssertTrue(
                app.staticTexts["We match your contacts to find friends. Your number stays private."].exists,
                "S1.4: no phone step subtext")
        }

        recorder.step("S1.5", "Skip opens the X step") {
            app.buttons["phone-step-skip"].tap()
            XCTAssertTrue(
                waitUntil(stepTimeout) { onSocialsStep(app) },
                "S1.5: the X step did not show within \(Int(stepTimeout)) s of Skip")
            XCTAssertTrue(app.staticTexts["Connect X"].exists, "S1.5: no 'Connect X' title")
            XCTAssertTrue(app.staticTexts["Find people you follow on Monaco."].exists, "S1.5: no X step subtext")
        }

        recorder.step("S1.6", "Skip opens Home with the add-number nudge") {
            app.buttons["socials-step-skip"].tap()
            XCTAssertTrue(
                app.tab("Home").waitForExistence(timeout: stepTimeout),
                "S1.6: the tab bar did not show within \(Int(stepTimeout)) s of Skip")
            XCTAssertTrue(
                waitForNudge(app, reading: addPhoneNudge, timeout: stepTimeout),
                "S1.6: the nudge did not read '\(addPhoneNudge)' on Home, it read '\(nudge(app).label)'")
        }
    }

    static func ensureSignedIn(_ app: XCUIApplication, as member: JourneyAccount) {
        recorder().step("P6", "start signed in as C") {
            app.launch()
            if SignInJourney.currentScreen(app) == .login {
                SignInJourney.enterCode(app, as: member, recorder: SignInJourney.recorder())
            }
        }
    }

    static func openHome(_ app: XCUIApplication, step: String) {
        XCTAssertTrue(
            app.tab("Home").waitForExistence(timeout: signedInTimeout),
            "\(step): C did not reach the tab bar within \(Int(signedInTimeout)) s. The setup script puts C past the first run (P6)"
        )
        app.tab("Home").tap()
    }

    static func numberLinkedElsewhere(_ app: XCUIApplication, other: JourneyAccount) {
        let recorder = recorder()
        let skip = app.buttons["phone-step-skip"]

        recorder.step("S2.1", "the nudge opens the phone sheet") {
            openHome(app, step: "S2.1")
            XCTAssertTrue(
                waitForNudge(app, reading: addPhoneNudge, timeout: stepTimeout),
                "S2.1: the nudge did not read '\(addPhoneNudge)', it read '\(nudge(app).label)'")
            nudge(app).tap()
            XCTAssertTrue(
                app.textFields["phone-step-number-field"].waitForExistence(timeout: checkTimeout),
                "S2.1: the phone sheet did not show within \(Int(checkTimeout)) s")
            XCTAssertTrue(app.staticTexts["Add your number"].exists, "S2.1: no 'Add your number' title")
            XCTAssertEqual(skip.label, "Not now", "S2.1: the sheet's skip button does not read 'Not now'")
        }

        sendCode(app, to: other.phone, step: "S2.2")

        recorder.step("S2.3", "a number linked to another account is refused") {
            enterCode(app, other.code, step: "S2.3")
            let caption = app.staticTexts["phone-step-caption"]
            XCTAssertTrue(
                waitUntil(linkTimeout) {
                    caption.exists && caption.label == "This number is linked to another account."
                },
                "S2.3: no 'This number is linked to another account.' within \(Int(linkTimeout)) s")
            XCTAssertEqual(skip.label, "Not now", "S2.3: the skip button does not read 'Not now'")
        }

        recorder.step("S2.4", "Not now closes the sheet and keeps the nudge") {
            skip.tap()
            XCTAssertTrue(
                app.element("onboarding-phone-step").waitForNonExistence(timeout: checkTimeout),
                "S2.4: the phone sheet did not close within \(Int(checkTimeout)) s")
            XCTAssertTrue(
                waitForNudge(app, reading: addPhoneNudge, timeout: checkTimeout),
                "S2.4: the nudge did not read '\(addPhoneNudge)', it read '\(nudge(app).label)'")
        }
    }

    static func relaunchPastTheGate(_ app: XCUIApplication) {
        let recorder = recorder()

        recorder.step("S3.1", "a relaunch opens Home, not a first-run step") {
            app.terminate()
            app.launch()
            var sawStep = false
            let landed = waitUntil(signedInTimeout) {
                if onHandleStep(app) || onPhoneStep(app) { sawStep = true }
                return app.tab("Home").exists
            }
            XCTAssertFalse(sawStep, "S3.1: the handle or phone step showed after the relaunch")
            XCTAssertTrue(landed, "S3.1: the tab bar did not show within \(Int(signedInTimeout)) s of the relaunch")
            XCTAssertTrue(
                waitForNudge(app, reading: addPhoneNudge, timeout: stepTimeout),
                "S3.1: the nudge did not read '\(addPhoneNudge)', it read '\(nudge(app).label)'")
        }

        recorder.step("S3.2", "Profile shows the handle") {
            app.tab("Profile").tap()
            let shown = app.element("profile-handle")
            XCTAssertTrue(
                waitUntil(15) { shown.exists && shown.label == "@\(handle)" },
                "S3.2: profile-handle did not read '@\(handle)' within 15 s")
        }
    }

    static func linkANumber(_ app: XCUIApplication, number: JourneyAccount) {
        let recorder = recorder()

        recorder.step("S4.1", "the nudge opens the phone sheet") {
            openHome(app, step: "S4.1")
            XCTAssertTrue(
                waitForNudge(app, reading: addPhoneNudge, timeout: stepTimeout),
                "S4.1: the nudge did not read '\(addPhoneNudge)', it read '\(nudge(app).label)'")
            nudge(app).tap()
            XCTAssertTrue(
                app.textFields["phone-step-number-field"].waitForExistence(timeout: checkTimeout),
                "S4.1: the phone sheet did not show within \(Int(checkTimeout)) s")
        }

        sendCode(app, to: number.phone, step: "S4.2")

        recorder.step("S4.3", "the code links the number and the nudge moves to X") {
            enterCode(app, number.code, step: "S4.3")
            XCTAssertTrue(
                app.element("onboarding-phone-step").waitForNonExistence(timeout: linkTimeout),
                "S4.3: the phone sheet did not close within \(Int(linkTimeout)) s of the code")
            let toast = app.element("monaco-toast-banner")
            XCTAssertTrue(
                waitUntil(checkTimeout) { toast.exists && toast.label.contains("Number added.") },
                "S4.3: no 'Number added.' toast")
            XCTAssertTrue(
                waitForNudge(app, reading: linkXNudge, timeout: stepTimeout),
                "S4.3: the nudge did not read '\(linkXNudge)', it read '\(nudge(app).label)'")
        }
    }
}

extension FirstRunJourney {
    static func linkFakeX(_ app: XCUIApplication, devToken: String) {
        let recorder = recorder()
        let devSignIn = app.buttons["devSignInButton"]

        recorder.step("S5.1", "launch with the dev token and reach the login form") {
            app.launchEnvironment["MONACO_DEV_TOKEN"] = devToken
            app.launchEnvironment["MONACO_FAKE_X"] = "1"
            SignInJourney.startSignedOut(app, recorder: SignInJourney.recorder())
            XCTAssertTrue(
                devSignIn.waitForExistence(timeout: signedInTimeout),
                "S5.1: no Dev: sign in button on the login form. The setup script hands the test a dev token (P7)")
        }

        recorder.step("S5.2", "Dev: sign in opens Home with the X nudge") {
            devSignIn.tap()
            XCTAssertTrue(
                app.tab("Home").waitForExistence(timeout: signedInTimeout),
                "S5.2: the tab bar did not show within \(Int(signedInTimeout)) s of Dev: sign in")
            XCTAssertTrue(
                waitForNudge(app, reading: linkXNudge, timeout: stepTimeout),
                "S5.2: the nudge did not read '\(linkXNudge)', it read '\(nudge(app).label)'")
        }

        recorder.step("S5.3", "the nudge opens the X sheet") {
            nudge(app).tap()
            XCTAssertTrue(
                app.buttons["socials-step-connect"].waitForExistence(timeout: checkTimeout),
                "S5.3: the X sheet did not show within \(Int(checkTimeout)) s")
            XCTAssertTrue(app.staticTexts["Connect X"].exists, "S5.3: no 'Connect X' title")
            XCTAssertEqual(
                app.buttons["socials-step-skip"].label, "Not now",
                "S5.3: the sheet's skip button does not read 'Not now'")
        }

        recorder.step("S5.4", "Connect X links the fake account and the nudge goes away") {
            app.buttons["socials-step-connect"].tap()
            XCTAssertTrue(
                app.buttons["socials-step-connect"].waitForNonExistence(timeout: linkTimeout),
                "S5.4: the X sheet did not close within \(Int(linkTimeout)) s of Connect X")
            XCTAssertFalse(app.webViews.firstMatch.exists, "S5.4: a web sheet opened for the fake X link")
            let toast = app.element("monaco-toast-banner")
            XCTAssertTrue(
                waitUntil(checkTimeout) { toast.exists && toast.label.contains("X connected.") },
                "S5.4: no 'X connected.' toast")
            XCTAssertTrue(
                app.element("onboarding-nudge").waitForNonExistence(timeout: stepTimeout),
                "S5.4: the onboarding nudge is still on Home after the X link")
        }
    }

    fileprivate static func sendCode(_ app: XCUIApplication, to phone: String, step: String) {
        recorder().step(step, "enter the number and send the code") {
            let field = app.textFields["phone-step-number-field"]
            field.tap()
            field.typeText(phone)
            let send = app.buttons["phone-step-send-code"]
            XCTAssertTrue(
                waitUntil(checkTimeout) { send.isEnabled }, "\(step): Send code stayed disabled for '\(phone)'")
            send.tap()
            let sentTo = app.staticTexts["phone-step-sent-to"]
            XCTAssertTrue(
                waitUntil(linkTimeout) { sentTo.exists && sentTo.label == "Code sent to \(readBack(phone))" },
                "\(step): no 'Code sent to \(readBack(phone))' within \(Int(linkTimeout)) s")
        }
    }

    fileprivate static func enterCode(_ app: XCUIApplication, _ code: String, step: String) {
        let field = app.textFields["phone-step-code-field"]
        XCTAssertTrue(field.waitForExistence(timeout: checkTimeout), "\(step): no code field")
        field.tap()
        field.typeText(code)
    }
}
