import XCTest

enum ProfileInviteJourney {
    static let id = "profile/invite"
    static let version = 1

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func openInvite(_ app: XCUIApplication, step: String) {
        ProfileOverviewJourney.openProfile(app, step: step)
        let row = app.element("profile-invite-row")
        app.scrollIntoReach(row)
        XCTAssertTrue(row.waitForExistence(timeout: 10), "\(step): no Invite friends row on Profile")
        XCTAssertTrue(row.label.contains("Invite friends"), "\(step): the row reads '\(row.label)'")
        row.tap()
        XCTAssertTrue(
            app.element("invite-link").waitForExistence(timeout: 15),
            "\(step): the invite link did not show within 15 s")
    }

    static func copyLink(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open Profile") {
            ProfileOverviewJourney.openProfile(app, step: "S1.1")
        }

        recorder.step("S1.2", "tap Invite friends") {
            let row = app.element("profile-invite-row")
            app.scrollIntoReach(row)
            XCTAssertTrue(row.waitForExistence(timeout: 10), "S1.2: no Invite friends row on Profile")
            XCTAssertTrue(row.label.contains("Invite friends"), "S1.2: the row reads '\(row.label)'")
            row.tap()
        }

        recorder.step("S1.3", "read the invite link") {
            let link = app.element("invite-link")
            XCTAssertTrue(link.waitForExistence(timeout: 15), "S1.3: the invite link did not show within 15 s")
            XCTAssertTrue(app.navigationBars["Invite friends"].exists, "S1.3: the screen is not titled Invite friends")
            XCTAssertTrue(link.label.contains("/r/"), "S1.3: the invite link reads '\(link.label)'")
        }

        recorder.step("S1.4", "copy the link") {
            app.buttons["invite-copy"].tap()
            JoinJourney.waitForToast(app, "Link copied.", step: "S1.4")
        }
    }

    static func shareLink(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open Invite friends") {
            openInvite(app, step: "S2.1")
        }

        recorder.step("S2.2", "open the share sheet") {
            let share = app.buttons["invite-share"]
            XCTAssertTrue(share.waitForExistence(timeout: 5), "S2.2: no Share button")
            XCTAssertTrue(share.label.contains("Share"), "S2.2: the share button reads '\(share.label)'")
            share.tap()
            XCTAssertTrue(
                app.otherElements["ActivityListView"].waitForExistence(timeout: 5),
                "S2.2: the share sheet did not open within 5 s")
        }

        recorder.step("S2.3", "close the share sheet") {
            let sheet = app.otherElements["ActivityListView"]
            let close = sheet.buttons["Close"]
            if close.exists {
                close.tap()
            } else {
                sheet.swipeDown(velocity: .fast)
            }
            XCTAssertTrue(
                app.element("invite-link").waitForExistence(timeout: 5),
                "S2.3: the invite link did not come back after the share sheet closed")
        }
    }
}
