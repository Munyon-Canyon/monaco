import XCTest

enum ProfileEditJourney {
    static let id = "profile/edit"
    static let version = 2

    static let nudgeCopy = "Connect X to find people you follow"
    static let rateLimitCopy = "Too many requests. Try again in a moment."
    static let emptyNameCopy = "Display name is required."
    static let withPhotoLabel = "Change your face"

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func newName() throws -> String {
        "Alfred \(try JourneyRun.id())"
    }

    static func displayName(_ app: XCUIApplication) -> String {
        app.staticTexts["profile-display-name"].label
    }

    static func openProfile(_ app: XCUIApplication, step: String) {
        app.tab("Profile").tap()
        XCTAssertTrue(
            app.element("profile-header").waitForExistence(timeout: 15), "\(step): the Profile header did not show")
    }

    static func openNameEditor(_ app: XCUIApplication, step: String) -> XCUIElement {
        let edit = app.buttons["profile-edit-button"]
        XCTAssertTrue(edit.waitForExistence(timeout: 10), "\(step): no edit button on the Profile header")
        edit.tap()
        let field = app.textFields["profile-name-field"]
        XCTAssertTrue(field.waitForExistence(timeout: 5), "\(step): the name field did not show")
        XCTAssertTrue(app.navigationBars["Edit profile"].exists, "\(step): the sheet is not titled 'Edit profile'")
        return field
    }

    static func closeNameEditor(_ app: XCUIApplication) {
        app.navigationBars["Edit profile"].buttons["Done"].tap()
    }

    static func clear(_ field: XCUIElement) {
        field.tap()
        let current = field.value as? String ?? ""
        field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: current.count))
    }

    static func saveName(_ app: XCUIApplication, _ name: String, step: String) {
        let field = openNameEditor(app, step: step)
        clear(field)
        field.typeText(name)
        let save = app.buttons["profile-name-save"]
        XCTAssertTrue(save.isEnabled, "\(step): Save stayed disabled for '\(name)'")
        save.tap()
    }

    static func waitForToast(_ app: XCUIApplication, step: String, timeout: TimeInterval) -> XCUIElement {
        let toast = app.element("monaco-toast-banner")
        XCTAssertTrue(toast.waitForExistence(timeout: timeout), "\(step): no toast within \(Int(timeout)) s")
        return toast
    }

    static func toastLabel(_ app: XCUIApplication, step: String, timeout: TimeInterval) -> String {
        let toast = waitForToast(app, step: step, timeout: timeout)
        let label = toast.label
        _ = toast.waitForNonExistence(timeout: 10)
        return label
    }

    static func uploadLibraryPhoto(_ app: XCUIApplication, step: String) {
        app.element("face-choose-photo").tap()
        let photo = app.images.matching(NSPredicate(format: "label BEGINSWITH 'Photo'")).firstMatch
        XCTAssertTrue(photo.waitForExistence(timeout: 15), "\(step): the photo library showed no photo")
        photo.tap()
    }

    static func ensureOnProfile(_ app: XCUIApplication, as account: JourneyAccount) {
        SignInJourney.ensureSignedIn(app, as: account)
    }

    static func changeName(_ app: XCUIApplication, recorder: JourneyRecorder) throws {
        let name = try newName()
        var shown = ""

        recorder.step("S1.1", "see the Profile header") {
            openProfile(app, step: "S1.1")
            let avatar = app.buttons["profile-photo-picker"]
            XCTAssertTrue(avatar.exists, "S1.1: no avatar on the Profile header")
            XCTAssertEqual(avatar.frame.width, 96, accuracy: 1, "S1.1: the avatar is not 96 pt wide")
            shown = displayName(app)
            XCTAssertFalse(shown.isEmpty, "S1.1: no display name on the Profile header")
            let handle = app.element("profile-handle")
            XCTAssertTrue(
                handle.exists && handle.label.hasPrefix("@"), "S1.1: the handle does not read '@' and the handle")
            let since = app.staticTexts["profile-member-since"].label
            XCTAssertNotNil(
                since.range(of: #"^Member since \S+ \d{4}$"#, options: .regularExpression),
                "S1.1: the header reads '\(since)', not 'Member since' with a month and year")
        }

        recorder.step("S1.2", "open Edit profile") {
            let field = openNameEditor(app, step: "S1.2")
            XCTAssertEqual(field.value as? String, shown, "S1.2: the field does not hold the current name")
            closeNameEditor(app)
        }

        recorder.step("S1.3", "save the new name") {
            saveName(app, name, step: "S1.3")
            let toast = waitForToast(app, step: "S1.3", timeout: 10)
            XCTAssertEqual(displayName(app), name, "S1.3: the header does not show '\(name)' with the toast")
            XCTAssertTrue(
                toast.label.contains("Name updated."), "S1.3: the toast read '\(toast.label)', not 'Name updated.'")
        }
    }

    static func emptyNameRefused(_ app: XCUIApplication, recorder: JourneyRecorder) {
        var before = ""
        let field = app.textFields["profile-name-field"]

        recorder.step("S2.1", "open Edit profile") {
            openProfile(app, step: "S2.1")
            before = displayName(app)
            _ = openNameEditor(app, step: "S2.1")
        }

        recorder.step("S2.2", "clear the name") {
            clear(field)
            let error = app.staticTexts["profile-name-error"]
            XCTAssertTrue(error.waitForExistence(timeout: 2), "S2.2: no inline message for an empty name")
            XCTAssertEqual(error.label, emptyNameCopy, "S2.2: the inline message is not '\(emptyNameCopy)'")
            XCTAssertFalse(app.buttons["profile-name-save"].isEnabled, "S2.2: Save is enabled for an empty name")
        }

        recorder.step("S2.3", "back out") {
            closeNameEditor(app)
            XCTAssertTrue(field.waitForNonExistence(timeout: 5), "S2.3: Done did not close Edit profile")
            XCTAssertEqual(displayName(app), before, "S2.3: the header no longer shows '\(before)'")
        }
    }

    static func changePhoto(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "open Your face") {
            openProfile(app, step: "S3.1")
            app.buttons["profile-photo-picker"].tap()
            XCTAssertTrue(app.element("face-picker-sheet").waitForExistence(timeout: 5), "S3.1: no face sheet")
            XCTAssertTrue(app.staticTexts["Your face"].exists, "S3.1: the sheet is not titled 'Your face'")
            let animals = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'face-option-'"))
            XCTAssertEqual(animals.count, 8, "S3.1: the sheet does not offer eight animals")
            let choose = app.element("face-choose-photo")
            XCTAssertTrue(choose.exists && choose.label == "Choose a photo", "S3.1: no 'Choose a photo'")
        }

        recorder.step("S3.2", "upload a library photo") {
            uploadLibraryPhoto(app, step: "S3.2")
            let label = toastLabel(app, step: "S3.2", timeout: 20)
            XCTAssertTrue(
                label.contains("Profile photo updated."),
                "S3.2: the toast read '\(label)', not 'Profile photo updated.'")
            XCTAssertEqual(
                app.buttons["profile-photo-picker"].label, withPhotoLabel, "S3.2: the avatar shows no stored photo")
        }
    }

    static func survivesRelaunch(_ app: XCUIApplication, recorder: JourneyRecorder) throws {
        let name = try newName()
        let avatar = app.buttons["profile-photo-picker"]

        recorder.step("P5", "start from S1's name and S3's photo") {
            openProfile(app, step: "P5")
            if displayName(app) != name {
                saveName(app, name, step: "P5")
                XCTAssertTrue(
                    app.staticTexts[name].waitForExistence(timeout: 15), "P5: the header does not show '\(name)'")
            }
            if avatar.label != withPhotoLabel {
                avatar.tap()
                uploadLibraryPhoto(app, step: "P5")
                _ = toastLabel(app, step: "P5", timeout: 20)
            }
        }

        recorder.step("S4.1", "relaunch and open Profile") {
            app.terminate()
            app.launch()
            XCTAssertTrue(app.tab("Profile").waitForExistence(timeout: 30), "S4.1: no tab bar after the relaunch")
            openProfile(app, step: "S4.1")
            XCTAssertTrue(
                app.staticTexts[name].waitForExistence(timeout: 30),
                "S4.1: the header reads '\(displayName(app))', not '\(name)', after the relaunch")
            XCTAssertEqual(avatar.label, withPhotoLabel, "S4.1: the avatar shows no stored photo after the relaunch")
        }
    }

    static func photoRateLimit(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let picker = app.buttons["profile-photo-picker"]

        recorder.step("S5.1", "pick faces until the rate limit answers") {
            openProfile(app, step: "S5.1")
            var labels: [String] = []
            for _ in 0..<8 where !labels.contains(where: { $0.contains(rateLimitCopy) }) {
                XCTAssertTrue(
                    picker.waitForExistence(timeout: 10) && picker.isEnabled, "S5.1: the avatar is not tappable")
                picker.tap()
                let fox = app.buttons["face-option-fox"]
                XCTAssertTrue(fox.waitForExistence(timeout: 5), "S5.1: no fox on the face sheet")
                fox.tap()
                labels.append(toastLabel(app, step: "S5.1", timeout: 30))
            }
            XCTAssertTrue(
                labels.contains(where: { $0.contains(rateLimitCopy) }),
                "S5.1: no rate-limit toast in 8 picks, toasts were \(labels)")
        }
    }

    static func nudgeBanner(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let banner = app.element("onboarding-nudge")

        recorder.step("S6.1", "see the nudge on Profile") {
            openProfile(app, step: "S6.1")
            XCTAssertTrue(banner.waitForExistence(timeout: 15), "S6.1: no nudge banner on Profile")
            XCTAssertTrue(app.staticTexts[nudgeCopy].exists, "S6.1: the banner does not read '\(nudgeCopy)'")
        }

        recorder.step("S6.2", "see the nudge on Home") {
            app.tab("Home").tap()
            XCTAssertTrue(banner.waitForExistence(timeout: 10), "S6.2: no nudge banner on Home")
        }

        recorder.step("S6.3", "open the placeholder link screen") {
            app.buttons["onboarding-nudge-open"].tap()
            XCTAssertTrue(
                app.navigationBars["Connect X"].waitForExistence(timeout: 5),
                "S6.3: the 'Connect X' sheet did not show")
            app.buttons["Done"].tap()
            XCTAssertTrue(
                app.navigationBars["Connect X"].waitForNonExistence(timeout: 5),
                "S6.3: Done did not close the sheet")
        }

        recorder.step("S6.4", "close the banner") {
            app.buttons["onboarding-nudge-close"].tap()
            XCTAssertTrue(banner.waitForNonExistence(timeout: 5), "S6.4: the banner stayed after close")
        }

        recorder.step("S6.5", "the banner stays closed on Profile") {
            openProfile(app, step: "S6.5")
            XCTAssertFalse(banner.exists, "S6.5: the closed banner came back on Profile")
        }
    }

    static func noBannerWhenComplete(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let banner = app.element("onboarding-nudge")

        recorder.step("S7.1", "no banner on Profile") {
            openProfile(app, step: "S7.1")
            XCTAssertFalse(banner.exists, "S7.1: a nudge banner showed for a completed account")
        }

        recorder.step("S7.2", "no banner on Home") {
            app.tab("Home").tap()
            XCTAssertFalse(banner.waitForExistence(timeout: 5), "S7.2: a nudge banner showed on Home")
        }
    }

    static func openHandleEditor(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S8.1", "tap the handle") {
            openProfile(app, step: "S8.1")
            let row = app.element("profile-handle")
            let handle = String(row.label.dropFirst())
            row.tap()
            let field = app.textFields["handle-step-field"]
            XCTAssertTrue(field.waitForExistence(timeout: 5), "S8.1: the handle editor did not open")
            XCTAssertEqual(field.value as? String, handle, "S8.1: the handle field does not hold '\(handle)'")
        }
    }
}
