import XCTest

enum CabalsApproveRequestJourney {
    static let id = "cabals/approve-request"
    static let version = 4

    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func askCabal(run: String) -> String { "QA ask \(run)" }
    static func approveCabal(run: String) -> String { "QA ok \(run)" }

    static func memberAsksAndCancels(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = askCabal(run: run)
        let join = app.buttons["cabal-join-button"]
        let requested = app.element("cabal-join-requested")

        recorder.step("S1.1", "open the request cabal as a non-member") {
            app.openCabal(named: name)
            XCTAssertTrue(
                JoinJourney.waitForLabel(join, containing: "Request to join", timeout: screenTimeout),
                "S1.1: no Request to join on \(name) within \(Int(screenTimeout)) s"
            )
        }

        recorder.step("S1.2", "ask to join") {
            join.tap()
            JoinJourney.waitForToast(app, "Request sent. You'll be in once the creator says yes.", step: "S1.2")
            XCTAssertTrue(
                JoinJourney.waitForLabel(requested, containing: "Request pending", timeout: 10),
                "S1.2: no Request pending within 10 s"
            )
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.buttons["cabal-join-cancel"], containing: "Cancel request", timeout: 2),
                "S1.2: no Cancel request next to Request sent"
            )
        }

        recorder.step("S1.3", "cancel the request") {
            app.buttons["cabal-join-cancel"].tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(join, containing: "Request to join", timeout: 10),
                "S1.3: Request to join did not come back within 10 s"
            )
            XCTAssertFalse(requested.exists, "S1.3: still reads Request pending after Cancel request")
        }

        recorder.step("S1.4", "ask again") {
            join.tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(requested, containing: "Request pending", timeout: 10),
                "S1.4: no Request pending within 10 s of asking again"
            )
            JoinJourney.snap(app, "S1-B-asked-again")
        }
    }

    static func creatorSeesRequest(
        _ app: XCUIApplication, cabal: String, member: String, step: String
    ) {
        app.openCabal(named: cabal)
        JoinJourney.openRequests(app, step: step)
        let rows = app.descendants(matching: .any).matching(identifier: "cabal-join-request-row")
        XCTAssertEqual(rows.count, 1, "\(step): expected one request row")
        XCTAssertTrue(JoinJourney.rowNames(rows.firstMatch, member), "\(step): the request does not name \(member)")
        XCTAssertTrue(
            JoinJourney.rowNames(rows.firstMatch, "Asked to join"),
            "\(step): the request row does not read Asked to join")
    }

    static func creatorDenies(_ app: XCUIApplication, run: String, member: String, recorder: JourneyRecorder) {
        recorder.step("S1.5", "open the cabal and see the request") {
            creatorSeesRequest(app, cabal: askCabal(run: run), member: member, step: "S1.5")
            XCTAssertTrue(
                app.buttons["cabal-join-approve"].firstMatch.exists, "S1.5: the request row has no Approve button")
            JoinJourney.snap(app, "S1-A-request")
        }

        recorder.step("S1.6", "deny it") {
            JoinJourney.decide(app, approve: false, step: "S1.6")
            JoinJourney.waitForToast(app, "Denied.", step: "S1.6")
            XCTAssertTrue(
                JoinJourney.waitForRequestsGone(app, timeout: 10),
                "S1.6: the request is still on the Members screen"
            )
            app.navigationBars.buttons.firstMatch.tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("cabal-member-count"), containing: "1 member", timeout: 5),
                "S1.6: the hero does not read 1 member: \(app.element("cabal-member-count").label)"
            )
        }
    }

    static func creatorApproves(_ app: XCUIApplication, run: String, member: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the cabal and see the seeded request") {
            creatorSeesRequest(app, cabal: approveCabal(run: run), member: member, step: "S2.1")
        }

        recorder.step("S2.2", "approve it") {
            JoinJourney.decide(app, approve: true, step: "S2.2")
            JoinJourney.waitForToast(app, "Approved.", step: "S2.2")
            XCTAssertTrue(
                JoinJourney.waitForRequestsGone(app, timeout: 10),
                "S2.2: the request is still on the Members screen"
            )
            app.navigationBars.buttons.firstMatch.tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("cabal-member-count"), containing: "2 members", timeout: 10),
                "S2.2: the hero does not read 2 members: \(app.element("cabal-member-count").label)"
            )
        }
    }
}
