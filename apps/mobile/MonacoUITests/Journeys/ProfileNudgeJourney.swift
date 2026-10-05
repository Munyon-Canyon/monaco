import XCTest

enum ProfileNudgeJourney {
    static let id = "profile/nudge"
    static let version = 1

    static let nudgeCopy = "Connect X to find people you follow"
    static let rateLimitCopy = "Too many requests. Try again in a moment."

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func photoRateLimit(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let picker = app.buttons["profile-photo-picker"]

        recorder.step("S1.1", "pick faces until the rate limit answers") {
            ProfileEditJourney.openProfile(app, step: "S1.1")
            var labels: [String] = []
            for _ in 0..<8 where !labels.contains(where: { $0.contains(rateLimitCopy) }) {
                XCTAssertTrue(
                    picker.waitForExistence(timeout: 10) && picker.isEnabled, "S1.1: the avatar is not tappable")
                picker.tap()
                let fox = app.buttons["face-option-fox"]
                XCTAssertTrue(fox.waitForExistence(timeout: 5), "S1.1: no fox on the face sheet")
                fox.tap()
                labels.append(ProfileEditJourney.toastLabel(app, step: "S1.1", timeout: 30))
            }
            XCTAssertTrue(
                labels.contains(where: { $0.contains(rateLimitCopy) }),
                "S1.1: no rate-limit toast in 8 picks, toasts were \(labels)")
        }
    }

    static func nudgeBanner(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let banner = app.element("onboarding-nudge")

        recorder.step("S2.1", "see the nudge on Profile") {
            ProfileEditJourney.openProfile(app, step: "S2.1")
            XCTAssertTrue(banner.waitForExistence(timeout: 15), "S2.1: no nudge banner on Profile")
            XCTAssertTrue(app.staticTexts[nudgeCopy].exists, "S2.1: the banner does not read '\(nudgeCopy)'")
        }

        recorder.step("S2.2", "see the nudge on Home") {
            app.tab("Home").tap()
            XCTAssertTrue(banner.waitForExistence(timeout: 10), "S2.2: no nudge banner on Home")
        }

        recorder.step("S2.3", "open the X sheet and close it with Not now") {
            app.buttons["onboarding-nudge-open"].tap()
            let step = app.element("onboarding-socials-step")
            XCTAssertTrue(step.waitForExistence(timeout: 5), "S2.3: the X sheet did not show")
            let notNow = app.buttons["socials-step-skip"]
            XCTAssertEqual(notNow.label, "Not now", "S2.3: the sheet's skip button does not read 'Not now'")
            notNow.tap()
            XCTAssertTrue(step.waitForNonExistence(timeout: 5), "S2.3: Not now did not close the sheet")
        }

        recorder.step("S2.4", "close the banner") {
            app.buttons["onboarding-nudge-close"].tap()
            XCTAssertTrue(banner.waitForNonExistence(timeout: 5), "S2.4: the banner stayed after close")
        }

        recorder.step("S2.5", "the banner stays closed on Profile") {
            ProfileEditJourney.openProfile(app, step: "S2.5")
            XCTAssertFalse(banner.exists, "S2.5: the closed banner came back on Profile")
        }
    }

    static func noBannerWhenComplete(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let banner = app.element("onboarding-nudge")

        recorder.step("S3.1", "no banner on Profile") {
            ProfileEditJourney.openProfile(app, step: "S3.1")
            XCTAssertFalse(banner.exists, "S3.1: a nudge banner showed for a completed account")
        }

        recorder.step("S3.2", "no banner on Home") {
            app.tab("Home").tap()
            XCTAssertFalse(banner.waitForExistence(timeout: 5), "S3.2: a nudge banner showed on Home")
        }
    }

    static func openHandleEditor(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S4.1", "tap the handle") {
            ProfileEditJourney.openProfile(app, step: "S4.1")
            let row = app.element("profile-handle")
            let handle = String(row.label.dropFirst())
            row.tap()
            let field = app.textFields["handle-step-field"]
            XCTAssertTrue(field.waitForExistence(timeout: 5), "S4.1: the handle editor did not open")
            XCTAssertEqual(field.value as? String, handle, "S4.1: the handle field does not hold '\(handle)'")
        }
    }
}
