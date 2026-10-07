import XCTest

enum FollowAndBoardsJourney {
    static let id = "social/follow-and-boards"
    static let version = 1

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
        app.element("home-leaderboard-row-\(userID)")
    }

    static func ownRow(_ app: XCUIApplication, _ seed: Seed) -> XCUIElement {
        let listed = row(app, seed.meID)
        return listed.exists ? listed : app.element("home-leaderboard-me")
    }

    static func boardRows(_ app: XCUIApplication) -> XCUIElementQuery {
        app.descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH 'home-leaderboard-row-'"))
    }

    static func showsReturn(_ element: XCUIElement) -> Bool {
        element.exists && element.label.contains("percent") && !element.label.contains("no return yet")
    }

    static func openBoard(_ app: XCUIApplication, step: String) {
        app.tab("Home").tap()
        let header = app.staticTexts["Top investors"].firstMatch
        app.scrollIntoReach(header)
        XCTAssertTrue(header.waitForExistence(timeout: 15), "\(step): no \"Top investors\" on Home within 15 s")
    }

    static func choose(_ app: XCUIApplication, segment: String, step: String) {
        let filter = app.element("home-leaderboard-filter")
        app.scrollIntoReach(filter)
        XCTAssertTrue(filter.waitForExistence(timeout: 10), "\(step): no \"Everyone\" / \"Friends\" segment")
        let button = app.buttons[segment].firstMatch
        XCTAssertTrue(button.waitForExistence(timeout: 5), "\(step): no \"\(segment)\" segment")
        button.tap()
    }

    static func friendsEmpty(_ app: XCUIApplication, step: String) {
        XCTAssertTrue(
            app.element("home-leaderboard-friends-empty").waitForExistence(timeout: 10),
            "\(step): Friends does not show the empty state within 10 s")
        XCTAssertTrue(
            app.staticTexts["Follow people to see how they do."].firstMatch.exists,
            "\(step): no \"Follow people to see how they do.\"")
        XCTAssertTrue(
            app.buttons["home-leaderboard-find-friends"].exists,
            "\(step): no \"Find friends\" on the empty Friends board"
        )
    }

    static func label(_ app: XCUIApplication, _ identifier: String) -> String {
        let element = app.element(identifier)
        return element.exists ? element.label : ""
    }

    static func openMember(_ app: XCUIApplication, seed: Seed, step: String) {
        let member = row(app, seed.memberID)
        app.scrollIntoReach(member)
        XCTAssertTrue(
            member.waitForExistence(timeout: 30),
            "\(step): no row for \(seed.memberName) on Top investors within 30 s")
        app.scrollIntoReach(member)
        member.tap()
        XCTAssertTrue(
            app.element("user-profile-header").waitForExistence(timeout: 15),
            "\(step): \(seed.memberName)'s profile did not show within 15 s")
    }

    static func bothRank(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S1.1", "Top investors ranks A and B with a crown on first") {
            openBoard(app, step: "S1.1")
            let all = app.element("home-leaderboard-range-ALL")
            XCTAssertTrue(all.waitForExistence(timeout: 10), "S1.1: no \"All\" range chip")
            XCTAssertTrue(all.isSelected, "S1.1: \"All\" is not the selected range")
            let me = row(app, seed.meID)
            let member = row(app, seed.memberID)
            XCTAssertTrue(
                waitUntil(30) { showsReturn(me) && showsReturn(member) },
                "S1.1: rows for A and B with a return did not show within 30 s: A reads '\(me.label)', B reads '\(member.label)'"
            )
            let first = boardRows(app).element(boundBy: 0)
            XCTAssertTrue(
                first.label.hasPrefix("First,"),
                "S1.1: the first row reads '\(first.label)', not the crown")
        }
    }

    static func friendsWithNoFollows(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S2.1", "Friends with no follows is empty") {
            choose(app, segment: "Friends", step: "S2.1")
            friendsEmpty(app, step: "S2.1")
        }

        recorder.step("S2.2", "Everyone ranks again") {
            choose(app, segment: "Everyone", step: "S2.2")
            XCTAssertTrue(
                waitUntil(10) { row(app, seed.meID).exists && row(app, seed.memberID).exists },
                "S2.2: Everyone did not rank A and B again within 10 s")
            XCTAssertFalse(
                app.element("home-leaderboard-friends-empty").exists,
                "S2.2: Everyone still shows the Friends empty state")
        }
    }

    static func followFromBoard(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S3.1", "the member's profile shows Follow and no followers") {
            openMember(app, seed: seed, step: "S3.1")
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("user-profile-name"), containing: seed.memberName, timeout: 10),
                "S3.1: the profile name reads '\(label(app, "user-profile-name"))', not \(seed.memberName)")
            XCTAssertTrue(
                label(app, "user-profile-handle").hasPrefix("@"),
                "S3.1: the handle reads '\(label(app, "user-profile-handle"))', not an @handle")
            XCTAssertEqual(
                label(app, "user-profile-followers"), "0 Followers",
                "S3.1: the followers link does not read \"0 Followers\"")
            XCTAssertEqual(
                label(app, "user-profile-following"), "0 Following",
                "S3.1: the following link does not read \"0 Following\"")
            XCTAssertEqual(
                label(app, "user-profile-follow"), "Follow", "S3.1: the button does not read \"Follow\"")
        }

        recorder.step("S3.2", "Follow turns into Following and the count goes to 1") {
            app.buttons["user-profile-follow"].tap()
            XCTAssertTrue(
                waitUntil(5) {
                    label(app, "user-profile-follow") == "Following"
                        && label(app, "user-profile-followers") == "1 Followers"
                },
                "S3.2: after Follow the button reads '\(label(app, "user-profile-follow"))' and the followers link '\(label(app, "user-profile-followers"))', not \"Following\" and \"1 Followers\""
            )
            XCTAssertEqual(
                label(app, "user-profile-following"), "0 Following",
                "S3.2: the following link does not read \"0 Following\"")
        }

        recorder.step("S3.3", "Friends ranks the member and A's own row") {
            app.tapBack()
            choose(app, segment: "Friends", step: "S3.3")
            XCTAssertTrue(
                waitUntil(10) {
                    row(app, seed.memberID).exists && ownRow(app, seed).exists
                        && !app.element("home-leaderboard-friends-empty").exists
                },
                "S3.3: Friends did not show \(seed.memberName) and A's own row within 10 s")
            XCTAssertTrue(
                ownRow(app, seed).label.contains("You"),
                "S3.3: A's own row reads '\(ownRow(app, seed).label)', not labelled \"You\"")
        }
    }

    static func followerSeesIt(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S4.1", "the Profile tab reads one follower") {
            app.tab("Profile").tap()
            XCTAssertTrue(
                waitUntil(15) {
                    label(app, "profile-followers") == "1 Followers" && label(app, "profile-following") == "0 Following"
                },
                "S4.1: Profile reads '\(label(app, "profile-followers"))' and '\(label(app, "profile-following"))', not \"1 Followers\" and \"0 Following\""
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
                        && label(app, "user-profile-followers") == "0 Followers"
                },
                "S5.1: after Following the button reads '\(label(app, "user-profile-follow"))' and the followers link '\(label(app, "user-profile-followers"))', not \"Follow\" and \"0 Followers\""
            )
            XCTAssertEqual(
                label(app, "user-profile-following"), "0 Following",
                "S5.1: the following link does not read \"0 Following\"")
        }

        recorder.step("S5.2", "Friends is empty again") {
            app.tapBack()
            choose(app, segment: "Friends", step: "S5.2")
            friendsEmpty(app, step: "S5.2")
        }
    }
}
