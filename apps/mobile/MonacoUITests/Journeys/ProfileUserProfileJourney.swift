import XCTest

enum ProfileUserProfileJourney {
    static let id = "profile/user-profile"
    static let version = 1

    static let memberName = "Bartholomez"

    struct Seed {
        let cabalID: String
        let cabalName: String
        let meID: String
        let memberID: String

        static func handedOff() throws -> Seed {
            Seed(
                cabalID: try JourneyHandoff.read("cabalID"),
                cabalName: try JourneyHandoff.read("cabalName"),
                meID: try JourneyHandoff.read("meID"),
                memberID: try JourneyHandoff.read("memberID"))
        }
    }

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func waitUntil(_ timeout: TimeInterval, _ condition: () -> Bool) -> Bool {
        FirstRunJourney.waitUntil(timeout, condition)
    }

    static func openCabal(_ app: XCUIApplication, seed: Seed, step: String) {
        app.tab("Profile").tap()
        let row = app.element("cabal-row-\(seed.cabalID)")
        app.scrollIntoReach(row)
        XCTAssertTrue(row.waitForExistence(timeout: 15), "\(step): no row for '\(seed.cabalName)' on Profile")
        row.tap()
        XCTAssertTrue(
            app.element("cabal-details-button").waitForExistence(timeout: 15), "\(step): the cabal screen did not show")
    }

    static func openMember(_ app: XCUIApplication, userID: String, step: String) {
        let row = app.element("cabal-member-\(userID)")
        app.scrollIntoReach(row)
        XCTAssertTrue(row.waitForExistence(timeout: 15), "\(step): no member board row for \(userID)")
        row.tap()
        XCTAssertTrue(
            app.element("user-profile-header").waitForExistence(timeout: 15), "\(step): the profile did not show")
    }

    static func openProfile(_ app: XCUIApplication, seed: Seed, userID: String, step: String) {
        openCabal(app, seed: seed, step: step)
        openMember(app, userID: userID, step: step)
    }

    static func header(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the shared cabal from Profile") {
            openCabal(app, seed: seed, step: "S1.1")
        }

        recorder.step("S1.2", "open the member's profile from the member board") {
            openMember(app, userID: seed.memberID, step: "S1.2")
        }

        recorder.step("S1.3", "read the header") {
            let name = app.element("user-profile-name")
            XCTAssertTrue(
                waitUntil(10) { name.label == memberName },
                "S1.3: the name reads '\(name.label)', not '\(memberName)'")
            XCTAssertTrue(
                app.element("user-profile-followers").label.contains("Followers"), "S1.3: no 'Followers' link")
            XCTAssertTrue(
                app.element("user-profile-following").label.contains("Following"), "S1.3: no 'Following' link")
        }

        recorder.step("S1.4", "find the more menu") {
            let more = app.element("user-profile-more")
            XCTAssertTrue(more.waitForExistence(timeout: 5), "S1.4: no '…' menu on another member's profile")
            XCTAssertEqual(more.label, "More", "S1.4: the menu is labelled '\(more.label)', not 'More'")
        }
    }

    static func follow(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        let button = app.buttons["user-profile-follow"]

        recorder.step("S2.1", "open the member's profile") {
            openProfile(app, seed: seed, userID: seed.memberID, step: "S2.1")
            XCTAssertTrue(
                waitUntil(15) { button.exists && button.label == "Follow" },
                "S2.1: the button reads '\(button.label)', not 'Follow'")
        }

        recorder.step("S2.2", "follow") {
            button.tap()
            XCTAssertTrue(
                waitUntil(10) { button.label == "Following" && button.isEnabled },
                "S2.2: the button reads '\(button.label)', not 'Following'")
        }

        recorder.step("S2.3", "unfollow") {
            button.tap()
            XCTAssertTrue(
                waitUntil(10) { button.label == "Follow" && button.isEnabled },
                "S2.3: the button reads '\(button.label)', not 'Follow'")
        }
    }

    static func sharedCabals(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        let shared = app.staticTexts["Cabals you share"]
        let row = app.buttons.containing(NSPredicate(format: "label CONTAINS %@", seed.cabalName)).firstMatch

        recorder.step("S3.1", "open the member's profile") {
            openProfile(app, seed: seed, userID: seed.memberID, step: "S3.1")
        }

        recorder.step("S3.2", "read Cabals you share") {
            app.scrollIntoReach(shared)
            XCTAssertTrue(shared.waitForExistence(timeout: 15), "S3.2: no 'Cabals you share' header")
            XCTAssertFalse(
                app.element("user-profile-shared-coming").exists,
                "S3.2: 'Shared cabals show up here soon.' shows (#2145)")
            XCTAssertTrue(row.waitForExistence(timeout: 15), "S3.2: no row for '\(seed.cabalName)' (#2145)")
        }

        recorder.step("S3.3", "open the shared cabal") {
            row.tap()
            XCTAssertTrue(
                app.element("cabal-details-button").waitForExistence(timeout: 15),
                "S3.3: the shared cabal did not open (#2145)")
        }
    }

    static func reportAndBlock(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S4.1", "open the member's profile") {
            openProfile(app, seed: seed, userID: seed.memberID, step: "S4.1")
            XCTAssertTrue(app.element("user-profile-more").waitForExistence(timeout: 15), "S4.1: no '…' menu")
        }

        recorder.step("S4.2", "open the menu") {
            app.element("user-profile-more").tap()
            let report = app.buttons["Report"]
            let block = app.buttons["Block"]
            XCTAssertTrue(report.waitForExistence(timeout: 5), "S4.2: the menu has no 'Report'")
            XCTAssertTrue(block.exists, "S4.2: the menu has no 'Block'")
            XCTAssertTrue(report.isEnabled, "S4.2: 'Report' is disabled (#2144, #2145)")
            XCTAssertTrue(block.isEnabled, "S4.2: 'Block' is disabled (#2144, #2145)")
        }
    }

    static func ownRow(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S5.1", "open your own row") {
            openProfile(app, seed: seed, userID: seed.meID, step: "S5.1")
            XCTAssertFalse(
                app.element("user-profile-follow").exists, "S5.1: your own profile shows a Follow button")
            XCTAssertFalse(app.element("user-profile-more").exists, "S5.1: your own profile shows the '…' menu")
        }
    }
}
