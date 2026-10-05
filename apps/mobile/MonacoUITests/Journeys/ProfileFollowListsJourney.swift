import XCTest

enum ProfileFollowListsJourney {
    static let id = "profile/follow-lists"
    static let version = 1

    static let memberName = "Bartholomez"

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func waitUntil(_ timeout: TimeInterval, _ condition: () -> Bool) -> Bool {
        FirstRunJourney.waitUntil(timeout, condition)
    }

    static func count(_ label: String) -> Int? {
        label.split(whereSeparator: { !$0.isNumber }).first.flatMap { Int($0) }
    }

    static func openList(
        _ app: XCUIApplication, link: String, title: String, coming: String, steps: (String, String, String),
        recorder: JourneyRecorder
    ) {
        let (open, tapLink, read) = steps
        recorder.step(open, "open Profile and find the follow links") {
            app.tab("Profile").tap()
            let followers = app.element("profile-followers")
            let following = app.element("profile-following")
            XCTAssertTrue(followers.waitForExistence(timeout: 15), "\(open): no 'Followers' link on Profile")
            XCTAssertTrue(followers.label.contains("Followers"), "\(open): the link reads '\(followers.label)'")
            XCTAssertTrue(following.label.contains("Following"), "\(open): the link reads '\(following.label)'")
        }

        recorder.step(tapLink, "open \(title)") {
            app.element(link).tap()
            XCTAssertTrue(
                app.navigationBars[title].waitForExistence(timeout: 10), "\(tapLink): the '\(title)' list did not open")
        }

        recorder.step(read, "find the member in \(title)") {
            XCTAssertFalse(
                app.element("follow-list-coming").waitForExistence(timeout: 5),
                "\(read): the list shows '\(coming)' (#620)")
            XCTAssertTrue(
                app.staticTexts[memberName].firstMatch.waitForExistence(timeout: 15),
                "\(read): no row for '\(memberName)' (#620)")
        }
    }

    static func followers(_ app: XCUIApplication, recorder: JourneyRecorder) {
        openList(
            app, link: "profile-followers", title: "Followers", coming: "Followers show up here soon.",
            steps: ("S1.1", "S1.2", "S1.3"), recorder: recorder)
    }

    static func following(_ app: XCUIApplication, recorder: JourneyRecorder) {
        openList(
            app, link: "profile-following", title: "Following", coming: "People you follow show up here soon.",
            steps: ("S2.1", "S2.2", "S2.3"), recorder: recorder)
    }

    static func counts(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "read the counts") {
            app.tab("Profile").tap()
            let followers = app.element("profile-followers")
            let following = app.element("profile-following")
            XCTAssertTrue(followers.waitForExistence(timeout: 15), "S3.1: no 'Followers' link on Profile")
            XCTAssertTrue(
                waitUntil(15) { (count(followers.label) ?? 0) >= 1 },
                "S3.1: the link reads '\(followers.label)', not a count and 'Followers' (#620)")
            XCTAssertTrue(
                (count(following.label) ?? 0) >= 1,
                "S3.1: the link reads '\(following.label)', not a count and 'Following' (#620)")
        }
    }
}
