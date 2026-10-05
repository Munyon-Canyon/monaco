import XCTest

enum ProfileFindFriendsJourney {
    static let id = "profile/find-friends"
    static let version = 1

    static let memberName = "Bartholomez"
    static let explainer =
        "See which of your contacts are already on Monaco. Only scrambled numbers leave your phone, never your address book."

    struct Member {
        let id: String
        let handle: String

        static func handedOff() throws -> Member {
            Member(id: try JourneyHandoff.read("memberID"), handle: try JourneyHandoff.read("memberHandle"))
        }
    }

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func waitUntil(_ timeout: TimeInterval, _ condition: () -> Bool) -> Bool {
        FirstRunJourney.waitUntil(timeout, condition)
    }

    static func openFindFriends(_ app: XCUIApplication, step: String) {
        app.tab("Profile").tap()
        let row = app.element("profile-find-friends")
        app.scrollIntoReach(row)
        XCTAssertTrue(row.waitForExistence(timeout: 15), "\(step): no 'Find friends' row on Profile")
        row.tap()
        XCTAssertTrue(
            app.navigationBars["Friends on Monaco"].waitForExistence(timeout: 10),
            "\(step): 'Friends on Monaco' did not open")
    }

    static func openSearch(_ app: XCUIApplication, step: String) -> XCUIElement {
        openFindFriends(app, step: step)
        let field = app.element("friends-search-field")
        XCTAssertTrue(field.waitForExistence(timeout: 10), "\(step): no search field on Find friends (#2142)")
        XCTAssertTrue(field.isEnabled, "\(step): the search field is disabled (#2142)")
        field.tap()
        return field
    }

    static func backOut(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S1.1", "find the Find friends row") {
            app.tab("Profile").tap()
            let row = app.element("profile-find-friends")
            app.scrollIntoReach(row)
            XCTAssertTrue(row.waitForExistence(timeout: 15), "S1.1: no 'Find friends' row on Profile")
            XCTAssertTrue(row.label.contains("Find friends"), "S1.1: the row reads '\(row.label)', not 'Find friends'")
        }

        recorder.step("S1.2", "open Find friends") {
            app.element("profile-find-friends").tap()
            XCTAssertTrue(
                app.navigationBars["Friends on Monaco"].waitForExistence(timeout: 10),
                "S1.2: 'Friends on Monaco' did not open")
            XCTAssertTrue(app.staticTexts[explainer].exists, "S1.2: the contacts explainer copy is missing")
        }

        recorder.step("S1.3", "back out with Not now") {
            app.element("friends-not-now").tap()
            XCTAssertTrue(
                app.element("profile-header").waitForExistence(timeout: 10),
                "S1.3: 'Not now' did not return to Profile")
        }
    }

    static func searchByHandle(_ app: XCUIApplication, member: Member, recorder: JourneyRecorder) {
        var field = app.element("friends-search-field")
        let result = app.element("friends-result-\(member.id)")
        let follow = app.buttons["friends-result-follow-\(member.id)"]

        recorder.step("S2.1", "open the search") {
            field = openSearch(app, step: "S2.1")
        }

        recorder.step("S2.2", "search by handle") {
            field.typeText(member.handle)
            XCTAssertTrue(result.waitForExistence(timeout: 15), "S2.2: no result for @\(member.handle) (#2142)")
            XCTAssertTrue(result.label.contains(memberName), "S2.2: the result reads '\(result.label)'")
            XCTAssertTrue(result.label.contains("@\(member.handle)"), "S2.2: the result has no '@\(member.handle)'")
        }

        recorder.step("S2.3", "follow from the result") {
            follow.tap()
            XCTAssertTrue(
                waitUntil(10) { follow.label == "Following" },
                "S2.3: the button reads '\(follow.label)', not 'Following' (#2142)")
        }
    }

    static func searchByName(_ app: XCUIApplication, member: Member, recorder: JourneyRecorder) {
        var field = app.element("friends-search-field")
        let result = app.element("friends-result-\(member.id)")

        recorder.step("S3.1", "open the search") {
            field = openSearch(app, step: "S3.1")
        }

        recorder.step("S3.2", "search by name") {
            field.typeText(memberName)
            XCTAssertTrue(result.waitForExistence(timeout: 15), "S3.2: no result for '\(memberName)' (#2142)")
            XCTAssertTrue(result.label.contains(memberName), "S3.2: the result reads '\(result.label)'")
        }
    }
}
