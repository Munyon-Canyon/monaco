import XCTest

enum CabalsLeaveJourney {
    static let id = "cabals/leave"
    static let version = 1

    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func leaveCabal(run: String) -> String { "QA leave \(run)" }
    static func stayCabal(run: String) -> String { "QA stay \(run)" }
    static func sellCabal(run: String) -> String { "QA sell \(run)" }

    static func openDetails(_ app: XCUIApplication, reaching identifier: String, step: String) -> XCUIElement {
        app.buttons["cabal-details-button"].tap()
        let target = app.element(identifier)
        XCTAssertTrue(
            app.element("cabal-details-done").waitForExistence(timeout: 10),
            "\(step): the Cabal details sheet did not show within 10 s"
        )
        app.scrollIntoReach(target)
        XCTAssertTrue(target.waitForExistence(timeout: 10), "\(step): no \(identifier) in the details within 10 s")
        return target
    }

    static func memberLeaves(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = leaveCabal(run: run)

        recorder.step("S1.1", "open the cabal as a member") {
            JoinJourney.openBySearch(app, name, step: "S1.1")
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("cabal-member-count"), containing: "2 members", timeout: screenTimeout),
                "S1.1: the hero does not read 2 members: \(app.element("cabal-member-count").label)"
            )
        }

        recorder.step("S1.2", "open details and reach Leave cabal") {
            let leave = openDetails(app, reaching: "cabalLeaveButton", step: "S1.2")
            XCTAssertEqual(leave.label, "Leave cabal", "S1.2: the leave button's label")
        }

        recorder.step("S1.3", "tap Leave cabal") {
            app.buttons["cabalLeaveButton"].tap()
            XCTAssertTrue(
                app.staticTexts["Leave \(name)?"].waitForExistence(timeout: 5),
                "S1.3: no dialog titled Leave \(name)? within 5 s"
            )
            XCTAssertTrue(app.buttons["cabalLeaveConfirmButton"].exists, "S1.3: the dialog has no Leave cabal")
            JoinJourney.snap(app, "S1-B-leave-dialog")
        }

        recorder.step("S1.4", "confirm") {
            app.buttons["cabalLeaveConfirmButton"].tap()
            JoinJourney.waitForToast(app, "You left \(name).", step: "S1.4")
            XCTAssertTrue(
                app.element("cabals-root").waitForExistence(timeout: 10),
                "S1.4: the Cabals tab did not show within 10 s"
            )
        }

        recorder.step("S1.5", "search shows the cabal as one to join") {
            JoinJourney.search(app, for: name, step: "S1.5")
            let join = app.buttons.matching(
                NSPredicate(format: "identifier BEGINSWITH 'cabals-search-enter-' AND label == %@", "Join \(name)")
            ).firstMatch
            XCTAssertTrue(join.waitForExistence(timeout: 10), "S1.5: no Join \(name) in the results within 10 s")
        }
    }

    static func creatorStays(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the cabal as its creator") {
            JoinJourney.openBySearch(app, stayCabal(run: run), step: "S2.1")
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("cabal-member-count"), containing: "2 members", timeout: screenTimeout),
                "S2.1: the hero does not read 2 members: \(app.element("cabal-member-count").label)"
            )
        }

        recorder.step("S2.2", "details explain why the creator cannot leave") {
            let note = openDetails(app, reaching: "cabalLeaveCreatorNote", step: "S2.2")
            XCTAssertEqual(note.label, "You can leave once everyone else has left.", "S2.2: the creator note")
            XCTAssertFalse(app.buttons["cabalLeaveButton"].exists, "S2.2: the creator is offered Leave cabal")
        }
    }

    static func memberSellsAndLeaves(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = sellCabal(run: run)

        recorder.step("S3.1", "open the cabal as a member") {
            JoinJourney.openBySearch(app, name, step: "S3.1")
        }

        recorder.step("S3.2", "the leave dialog offers Sell and leave") {
            _ = openDetails(app, reaching: "cabalLeaveButton", step: "S3.2")
            app.buttons["cabalLeaveButton"].tap()
            XCTAssertTrue(
                app.staticTexts["Leave \(name)?"].waitForExistence(timeout: 5),
                "S3.2: no dialog titled Leave \(name)? within 5 s"
            )
            XCTAssertTrue(
                app.buttons["Sell and leave"].exists,
                "S3.2: the dialog has no Sell and leave (known failure, #657)"
            )
        }
    }
}
