import XCTest

enum ProfileEditJourney {
    static let id = "profile/edit"
    static let version = 1

    static let nudgeCopy = "Connect X to find people you follow"
    static let rateLimitCopy = "Too many requests. Try again in a moment."
    static let restoredName = "Alfred"

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func displayName(_ app: XCUIApplication) -> String {
        app.staticTexts["profile-display-name"].label
    }

    static func openProfile(_ app: XCUIApplication, step: String) {
        app.tab("Profile").tap()
        XCTAssertTrue(
            app.element("profile-header").waitForExistence(timeout: 15), "\(step): the Profile header did not show")
    }

    static func toastLabel(_ app: XCUIApplication, step: String, timeout: TimeInterval) -> String {
        let toast = app.element("monaco-toast-banner")
        XCTAssertTrue(toast.waitForExistence(timeout: timeout), "\(step): no toast within \(Int(timeout)) s")
        let label = toast.label
        _ = toast.waitForNonExistence(timeout: 10)
        return label
    }

    static func rename(_ app: XCUIApplication, to name: String, step: String) {
        let edit = app.buttons["profile-edit-button"]
        XCTAssertTrue(edit.waitForExistence(timeout: 10), "\(step): no edit button on the Profile header")
        edit.tap()
        let field = app.textFields["profile-name-field"]
        XCTAssertTrue(field.waitForExistence(timeout: 5), "\(step): the name field did not show")
        field.tap()
        let current = field.value as? String ?? ""
        field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: current.count))
        field.typeText(name)
        let save = app.buttons["profile-name-save"]
        XCTAssertTrue(save.isEnabled, "\(step): Save stayed disabled for '\(name)'")
        save.tap()
    }

    static func ensureOnProfile(_ app: XCUIApplication, as account: JourneyAccount) {
        SignInJourney.ensureSignedIn(app, as: account)
        let recorder = recorder()
        recorder.step("P2", "start from A's own name") {
            openProfile(app, step: "P2")
            guard displayName(app) == "QA Name" else { return }
            rename(app, to: restoredName, step: "P2")
            XCTAssertTrue(
                app.staticTexts[restoredName].waitForExistence(timeout: 15),
                "P2: the name did not go back to \(restoredName)")
        }
    }

    static func nudgeBanner(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let banner = app.element("onboarding-nudge")

        recorder.step("S1.1", "see the nudge and the handle on Profile") {
            openProfile(app, step: "S1.1")
            XCTAssertTrue(banner.waitForExistence(timeout: 15), "S1.1: no nudge banner on Profile")
            XCTAssertTrue(app.staticTexts[nudgeCopy].exists, "S1.1: the banner does not read '\(nudgeCopy)'")
            let handle = app.element("profile-handle")
            XCTAssertTrue(handle.exists, "S1.1: no @handle under the name")
            XCTAssertTrue(
                handle.label.hasPrefix("@"), "S1.1: the handle reads '\(handle.label)', not '@' and the handle")
        }

        recorder.step("S1.2", "see the nudge on Home") {
            app.tab("Home").tap()
            XCTAssertTrue(banner.waitForExistence(timeout: 10), "S1.2: no nudge banner on Home")
        }

        recorder.step("S1.3", "open the placeholder link screen") {
            app.buttons["onboarding-nudge-open"].tap()
            XCTAssertTrue(
                app.navigationBars["Connect X"].waitForExistence(timeout: 5),
                "S1.3: the 'Connect X' sheet did not show")
            app.buttons["Done"].tap()
            XCTAssertTrue(
                app.navigationBars["Connect X"].waitForNonExistence(timeout: 5),
                "S1.3: Done did not close the sheet")
        }

        recorder.step("S1.4", "close the banner") {
            app.buttons["onboarding-nudge-close"].tap()
            XCTAssertTrue(banner.waitForNonExistence(timeout: 5), "S1.4: the banner stayed after close")
        }

        recorder.step("S1.5", "the banner stays closed on Profile") {
            openProfile(app, step: "S1.5")
            XCTAssertFalse(banner.exists, "S1.5: the closed banner came back on Profile")
        }
    }

    static func renameAndRestore(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S2.1", "see the name and handle") {
            openProfile(app, step: "S2.1")
            XCTAssertTrue(app.staticTexts["profile-display-name"].exists, "S2.1: no display name on Profile")
            XCTAssertTrue(app.element("profile-handle").exists, "S2.1: no @handle on Profile")
        }

        recorder.step("S2.2", "open the name editor") {
            app.buttons["profile-edit-button"].tap()
            XCTAssertTrue(
                app.textFields["profile-name-field"].waitForExistence(timeout: 5), "S2.2: the name field did not show")
            app.buttons["Done"].tap()
        }

        recorder.step("S2.3", "save a padded name") {
            rename(app, to: "  QA   Name ", step: "S2.3")
            let label = toastLabel(app, step: "S2.3", timeout: 15)
            XCTAssertTrue(label.contains("Name updated."), "S2.3: the toast read '\(label)', not 'Name updated.'")
            XCTAssertEqual(displayName(app), "QA Name", "S2.3: the saved name is not 'QA Name'")
        }

        recorder.step("S2.4", "put the name back") {
            rename(app, to: restoredName, step: "S2.4")
            XCTAssertTrue(
                app.staticTexts[restoredName].waitForExistence(timeout: 15),
                "S2.4: the name did not go back to \(restoredName)")
        }
    }

    static func openHandleEditor(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "tap the handle") {
            openProfile(app, step: "S3.1")
            app.element("profile-handle").tap()
            XCTAssertTrue(
                app.staticTexts["Edit handle isn't on the new backend yet."].waitForExistence(timeout: 5),
                "S3.1: the handle editor did not open")
        }
    }

    static func changeFace(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let picker = app.buttons["profile-photo-picker"]

        recorder.step("S4.1", "open the face sheet") {
            openProfile(app, step: "S4.1")
            picker.tap()
            XCTAssertTrue(app.element("face-picker-sheet").waitForExistence(timeout: 5), "S4.1: no face sheet")
        }

        recorder.step("S4.2", "upload a library photo") {
            app.element("face-choose-photo").tap()
            let photo = app.images.matching(NSPredicate(format: "label BEGINSWITH 'Photo'")).firstMatch
            XCTAssertTrue(photo.waitForExistence(timeout: 15), "S4.2: the photo library showed no photo")
            photo.tap()
            let label = toastLabel(app, step: "S4.2", timeout: 30)
            XCTAssertTrue(
                label.contains("Profile photo updated."),
                "S4.2: the toast read '\(label)', not 'Profile photo updated.'")
        }

        recorder.step("S4.3", "pick faces until the rate limit answers") {
            var labels: [String] = []
            for _ in 0..<4 where !labels.contains(where: { $0.contains(rateLimitCopy) }) {
                XCTAssertTrue(
                    picker.waitForExistence(timeout: 10) && picker.isEnabled, "S4.3: the avatar is not tappable")
                picker.tap()
                let fox = app.buttons["face-option-fox"]
                XCTAssertTrue(fox.waitForExistence(timeout: 5), "S4.3: no fox on the face sheet")
                fox.tap()
                labels.append(toastLabel(app, step: "S4.3", timeout: 30))
            }
            XCTAssertTrue(
                labels.contains(where: { $0.contains(rateLimitCopy) }),
                "S4.3: no rate-limit toast in 4 picks, toasts were \(labels)")
        }
    }

    static func noBannerWhenComplete(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let banner = app.element("onboarding-nudge")

        recorder.step("S5.1", "no banner on Profile") {
            openProfile(app, step: "S5.1")
            XCTAssertFalse(banner.exists, "S5.1: a nudge banner showed for a completed account")
        }

        recorder.step("S5.2", "no banner on Home") {
            app.tab("Home").tap()
            XCTAssertFalse(banner.waitForExistence(timeout: 5), "S5.2: a nudge banner showed on Home")
        }
    }
}
