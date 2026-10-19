import XCTest

enum ProfileEditJourney {
    static let id = "profile/edit"
    static let version = 4

    static let emptyNameCopy = "Display name is required."
    static let savedCopy = "Profile saved."
    static let photoLabel = "Edit profile photo"
    static let maxNameLength = 40

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

    static func openEditProfile(_ app: XCUIApplication, step: String) -> XCUIElement {
        let edit = app.buttons["profile-edit-button"]
        XCTAssertTrue(edit.waitForExistence(timeout: 10), "\(step): no name block on the Profile card")
        edit.tap()
        let field = app.textFields["profile-name-field"]
        XCTAssertTrue(field.waitForExistence(timeout: 5), "\(step): the name field did not show")
        XCTAssertTrue(app.navigationBars["Edit profile"].exists, "\(step): the screen is not titled 'Edit profile'")
        return field
    }

    static func closeEditProfile(_ app: XCUIApplication) {
        app.navigationBars["Edit profile"].buttons.firstMatch.tap()
    }

    static func clear(_ field: XCUIElement) {
        field.tap()
        field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: maxNameLength))
    }

    static func saveName(_ app: XCUIApplication, _ name: String, step: String) {
        let field = openEditProfile(app, step: step)
        clear(field)
        field.typeText(name)
        let save = app.buttons["profile-name-save"]
        XCTAssertTrue(save.isEnabled, "\(step): Save changes stayed disabled for '\(name)'")
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

    static func changeName(_ app: XCUIApplication, recorder: JourneyRecorder) throws {
        let name = try newName()
        var shown = ""

        recorder.step("S1.1", "see the Profile card") {
            openProfile(app, step: "S1.1")
            let avatar = app.buttons["profile-photo-picker"]
            XCTAssertTrue(avatar.exists, "S1.1: no avatar on the Profile card")
            XCTAssertEqual(avatar.frame.width, 64, accuracy: 1, "S1.1: the avatar is not 64 pt wide")
            XCTAssertEqual(avatar.label, photoLabel, "S1.1: the avatar is not labelled '\(photoLabel)'")
            shown = displayName(app)
            XCTAssertFalse(shown.isEmpty, "S1.1: no display name on the Profile card")
            let handle = app.element("profile-handle")
            XCTAssertTrue(
                handle.exists && handle.label.hasPrefix("@"), "S1.1: the handle does not read '@' and the handle")
            let since = app.staticTexts["profile-member-since"].label
            XCTAssertNotNil(
                since.range(of: #"^Joined \S+ \d{4}$"#, options: .regularExpression),
                "S1.1: the card reads '\(since)', not 'Joined' with a month and year")
        }

        recorder.step("S1.2", "open Edit profile") {
            _ = openEditProfile(app, step: "S1.2")
            XCTAssertFalse(
                app.buttons["profile-name-save"].isEnabled, "S1.2: Save changes is enabled before anything changed")
            closeEditProfile(app)
        }

        recorder.step("S1.3", "save the new name") {
            saveName(app, name, step: "S1.3")
            let toast = waitForToast(app, step: "S1.3", timeout: 10)
            XCTAssertTrue(
                toast.label.contains(savedCopy), "S1.3: the toast read '\(toast.label)', not '\(savedCopy)'")
            XCTAssertTrue(
                app.element("edit-profile-saved").waitForExistence(timeout: 5), "S1.3: no 'Profile saved.' notice")
            closeEditProfile(app)
            XCTAssertTrue(
                app.staticTexts[name].waitForExistence(timeout: 10),
                "S1.3: the Profile card does not show '\(name)'")
        }
    }

    static func emptyNameRefused(_ app: XCUIApplication, recorder: JourneyRecorder) {
        var before = ""
        let field = app.textFields["profile-name-field"]

        recorder.step("S2.1", "open Edit profile") {
            openProfile(app, step: "S2.1")
            before = displayName(app)
            _ = openEditProfile(app, step: "S2.1")
        }

        recorder.step("S2.2", "clear the name") {
            clear(field)
            let reported = XCTNSPredicateExpectation(
                predicate: NSPredicate(format: "value == %@", emptyNameCopy), object: field)
            XCTAssertEqual(
                XCTWaiter().wait(for: [reported], timeout: 2), .completed,
                "S2.2: the name field does not report '\(emptyNameCopy)'")
            XCTAssertFalse(
                app.buttons["profile-name-save"].isEnabled, "S2.2: Save changes is enabled for an empty name")
        }

        recorder.step("S2.3", "back out") {
            closeEditProfile(app)
            XCTAssertTrue(field.waitForNonExistence(timeout: 5), "S2.3: back did not leave Edit profile")
            XCTAssertEqual(displayName(app), before, "S2.3: the card no longer shows '\(before)'")
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
        }
    }

    static func survivesRelaunch(_ app: XCUIApplication, recorder: JourneyRecorder) throws {
        let name = try newName()

        recorder.step("P5", "start from S1's name") {
            openProfile(app, step: "P5")
            if displayName(app) != name {
                saveName(app, name, step: "P5")
                _ = toastLabel(app, step: "P5", timeout: 15)
                closeEditProfile(app)
                XCTAssertTrue(
                    app.staticTexts[name].waitForExistence(timeout: 15), "P5: the card does not show '\(name)'")
            }
        }

        recorder.step("S4.1", "relaunch and open Profile") {
            app.terminate()
            app.launch()
            XCTAssertTrue(app.tab("Profile").waitForExistence(timeout: 30), "S4.1: no tab bar after the relaunch")
            openProfile(app, step: "S4.1")
            XCTAssertTrue(
                app.staticTexts[name].waitForExistence(timeout: 30),
                "S4.1: the card reads '\(displayName(app))', not '\(name)', after the relaunch")
            XCTAssertTrue(app.buttons["profile-photo-picker"].exists, "S4.1: no avatar after the relaunch")
        }
    }
}
