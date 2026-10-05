import XCTest

enum ProfileEditJourney {
    static let id = "profile/edit"
    static let version = 3

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
}
