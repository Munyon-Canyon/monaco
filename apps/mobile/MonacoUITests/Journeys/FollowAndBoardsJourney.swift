import XCTest

enum FollowAndBoardsJourney {
    static let id = "social/follow-and-boards"
    static let version = 3

    struct Seed {
        let meID: String
        let meName: String
        let memberID: String
        let memberName: String

        static func handedOff() throws -> Seed {
            Seed(
                meID: try JourneyHandoff.read("meID"),
                meName: try JourneyHandoff.read("meName"),
                memberID: try JourneyHandoff.read("memberID"),
                memberName: try JourneyHandoff.read("memberName"))
        }
    }

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func waitUntil(_ timeout: TimeInterval, _ condition: () -> Bool) -> Bool {
        FirstRunJourney.waitUntil(timeout, condition)
    }

    static func row(_ app: XCUIApplication, _ userID: String) -> XCUIElement {
        app.element("leaderboard-row-\(userID)")
    }

    static func showsReturn(_ element: XCUIElement) -> Bool {
        element.exists && element.label.contains("percent") && !element.label.contains("not ranked")
    }

    static func openBoard(_ app: XCUIApplication, step: String) {
        app.tab("Home").tap()
        app.popToRoot()
        let home = app.element("home-leaderboard-row")
        app.scrollIntoReach(home)
        XCTAssertTrue(home.waitForExistence(timeout: 15), "\(step): no home-leaderboard-row on Home within 15 s")
        home.tap()
        XCTAssertTrue(
            app.element("leaderboard-page").waitForExistence(timeout: 15),
            "\(step): the Home row did not open leaderboard-page")
        XCTAssertTrue(
            app.element("leaderboard-tab-people").waitForExistence(timeout: 10),
            "\(step): no leaderboard-tab-people")
        app.element("leaderboard-tab-people").tap()
    }

    static func openFollowing(_ app: XCUIApplication, step: String) {
        app.tab("Profile").tap()
        app.popToRoot()
        let link = app.element("profile-find-friends")
        app.scrollIntoReach(link)
        XCTAssertTrue(link.waitForExistence(timeout: 15), "\(step): no 'Find friends' row on Profile")
        link.tap()
        let following = app.element("friends-tab-following")
        XCTAssertTrue(following.waitForExistence(timeout: 10), "\(step): no Following segment on Find friends")
        following.tap()
    }

    static func label(_ app: XCUIApplication, _ identifier: String) -> String {
        let element = app.element(identifier)
        return element.exists ? element.label : ""
    }

    static func count(_ app: XCUIApplication, _ identifier: String) -> String {
        label(app, identifier).replacingOccurrences(of: ",", with: "")
    }

    static func openMember(_ app: XCUIApplication, seed: Seed, step: String) {
        let member = row(app, seed.memberID)
        app.scrollIntoReach(member)
        XCTAssertTrue(
            member.waitForExistence(timeout: 30),
            "\(step): no leaderboard row for \(seed.memberName) on the People board within 30 s")
        app.scrollIntoReach(member)
        member.tap()
        XCTAssertTrue(
            app.element("user-profile-header").waitForExistence(timeout: 15),
            "\(step): \(seed.memberName)'s profile did not show within 15 s")
    }

    static func bothRank(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S1.1", "Leaderboard C People ranks A and B") {
            openBoard(app, step: "S1.1")
            let me = row(app, seed.meID)
            let member = row(app, seed.memberID)
            app.scrollIntoReach(member)
            XCTAssertTrue(
                waitUntil(30) { showsReturn(me) && showsReturn(member) },
                "S1.1: rows for A and B with a rate did not show within 30 s: A reads '\(me.label)', B reads '\(member.label)'"
            )
        }
    }

    static func followingWithNoFollows(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S2.1", "Following with no follows is empty") {
            openFollowing(app, step: "S2.1")
            XCTAssertTrue(
                app.element("friends-following-empty").waitForExistence(timeout: 10),
                "S2.1: Following does not show the empty state within 10 s")
            XCTAssertTrue(
                app.staticTexts["You're not following anyone yet"].firstMatch.exists,
                "S2.1: no \"You're not following anyone yet\"")
        }
    }

    static func followFromBoard(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S3.1", "the member's profile shows Follow and no followers") {
            openBoard(app, step: "S3.1")
            openMember(app, seed: seed, step: "S3.1")
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("user-profile-name"), containing: seed.memberName, timeout: 10),
                "S3.1: the profile name reads '\(label(app, "user-profile-name"))', not \(seed.memberName)")
            XCTAssertTrue(
                label(app, "user-profile-handle").hasPrefix("@"),
                "S3.1: the handle reads '\(label(app, "user-profile-handle"))', not an @handle")
            XCTAssertEqual(
                count(app, "user-profile-followers"), "0 followers",
                "S3.1: the followers link does not read \"0 followers\"")
            XCTAssertEqual(
                count(app, "user-profile-following"), "0 following",
                "S3.1: the following link does not read \"0 following\"")
            XCTAssertEqual(
                label(app, "user-profile-follow"), "Follow", "S3.1: the button does not read \"Follow\"")
        }

        recorder.step("S3.2", "Follow turns into Following and the count goes to 1") {
            app.buttons["user-profile-follow"].tap()
            XCTAssertTrue(
                waitUntil(5) {
                    label(app, "user-profile-follow") == "Following"
                        && count(app, "user-profile-followers") == "1 follower"
                },
                "S3.2: after Follow the button reads '\(label(app, "user-profile-follow"))' and the followers link '\(count(app, "user-profile-followers"))', not \"Following\" and \"1 follower\""
            )
            XCTAssertEqual(
                count(app, "user-profile-following"), "0 following",
                "S3.2: the following link does not read \"0 following\"")
        }

        recorder.step("S3.3", "Following lists the member") {
            openFollowing(app, step: "S3.3")
            let listed = app.element("friends-following-row-\(seed.memberID)")
            XCTAssertTrue(
                listed.waitForExistence(timeout: 15),
                "S3.3: no row for \(seed.memberName) under Following within 15 s")
            XCTAssertTrue(
                listed.label.contains(seed.memberName),
                "S3.3: the row reads '\(listed.label)', not \(seed.memberName)")
            XCTAssertFalse(
                app.element("friends-following-empty").exists, "S3.3: Following still shows the empty state")
        }
    }

    static func followerSeesIt(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S4.1", "the Profile tab reads one follower") {
            app.tab("Profile").tap()
            XCTAssertTrue(
                waitUntil(15) {
                    count(app, "profile-followers") == "1 follower" && count(app, "profile-following") == "0 following"
                },
                "S4.1: Profile reads '\(count(app, "profile-followers"))' and '\(count(app, "profile-following"))', not \"1 follower\" and \"0 following\""
            )
        }

        recorder.step("S4.2", "Followers lists the follower, who opens to their profile") {
            app.element("profile-followers").tap()
            XCTAssertTrue(
                app.navigationBars["Followers"].waitForExistence(timeout: 10),
                "S4.2: the \"Followers\" screen did not show within 10 s")
            XCTAssertTrue(
                app.element("follow-list-row-\(seed.meID)").waitForExistence(timeout: 15),
                "S4.2: no row for \(seed.meName) in Followers within 15 s")
            XCTAssertTrue(
                waitUntil(15) { label(app, "follow-list-follow-\(seed.meID)") == "Follow" },
                "S4.2: the button on \(seed.meName)'s row reads '\(label(app, "follow-list-follow-\(seed.meID)"))', not \"Follow\""
            )
            app.element("follow-list-open-\(seed.meID)").tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("user-profile-name"), containing: seed.meName, timeout: 15),
                "S4.2: tapping \(seed.meName)'s row did not open their profile within 15 s")
        }
    }

    static func unfollow(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S5.1", "Following turns back into Follow and the count goes to 0") {
            openBoard(app, step: "S5.1")
            openMember(app, seed: seed, step: "S5.1")
            XCTAssertTrue(
                waitUntil(15) { label(app, "user-profile-follow") == "Following" },
                "S5.1: the button reads '\(label(app, "user-profile-follow"))', not \"Following\"")
            app.buttons["user-profile-follow"].tap()
            XCTAssertTrue(
                waitUntil(5) {
                    label(app, "user-profile-follow") == "Follow"
                        && count(app, "user-profile-followers") == "0 followers"
                },
                "S5.1: after Following the button reads '\(label(app, "user-profile-follow"))' and the followers link '\(count(app, "user-profile-followers"))', not \"Follow\" and \"0 followers\""
            )
            XCTAssertEqual(
                count(app, "user-profile-following"), "0 following",
                "S5.1: the following link does not read \"0 following\"")
        }

        recorder.step("S5.2", "Following is empty again") {
            openFollowing(app, step: "S5.2")
            XCTAssertTrue(
                app.element("friends-following-empty").waitForExistence(timeout: 10),
                "S5.2: Following does not show the empty state within 10 s")
        }
    }
}
