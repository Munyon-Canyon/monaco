import XCTest

enum CabalsLeaveJourney {
    static let id = "cabals/leave"
    static let version = 6

    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func leaveCabal(run: String) -> String { "QA leave \(run)" }
    static func stayCabal(run: String) -> String { "QA stay \(run)" }

    static func openLeaveSheet(_ app: XCUIApplication, reaching identifier: String, step: String) -> XCUIElement {
        let more = app.buttons["cabal-more-button"]
        XCTAssertTrue(more.waitForExistence(timeout: 10), "\(step): no cabal-more-button within 10 s")
        more.tap()
        let leave = app.buttons["cabal-more-leave"]
        XCTAssertTrue(leave.waitForExistence(timeout: 5), "\(step): the \u{2026} menu has no Leave cabal")
        leave.tap()
        let target = app.element(identifier)
        XCTAssertTrue(
            target.waitForExistence(timeout: 10), "\(step): no \(identifier) in the Leave cabal sheet within 10 s")
        return target
    }

    static func tile(_ app: XCUIApplication, named name: String) -> XCUIElement {
        app.buttons.matching(
            NSPredicate(format: "identifier BEGINSWITH 'cabals-tile-' AND label CONTAINS %@", name)
        ).firstMatch
    }

    static func memberLeaves(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = leaveCabal(run: run)

        recorder.step("S1.1", "open the cabal as a member") {
            app.openCabal(named: name)
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("cabal-member-count"), containing: "2 members", timeout: screenTimeout),
                "S1.1: the hero does not read 2 members: \(app.element("cabal-member-count").label)"
            )
        }

        recorder.step("S1.2", "open the \u{2026} menu and reach Leave cabal") {
            let leave = openLeaveSheet(app, reaching: "cabalLeaveButton", step: "S1.2")
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
            app.buttons["cabalLeaveConfirmButton"].firstMatch.tap()
            JoinJourney.waitForToast(app, "You left \(name).", step: "S1.4")
            XCTAssertTrue(
                app.element("cabal-header-name").waitForNonExistence(timeout: 10),
                "S1.4: the cabal screen is still open")
        }

        recorder.step("S1.5", "the Cabals tab no longer has the tile") {
            app.tab("Cabals").tap()
            XCTAssertTrue(
                tile(app, named: name).waitForNonExistence(timeout: 10),
                "S1.5: the Cabals tab still has a tile for \(name)")
        }
    }

    static func creatorStays(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the cabal as its creator") {
            app.openCabal(named: stayCabal(run: run))
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("cabal-member-count"), containing: "2 members", timeout: screenTimeout),
                "S2.1: the hero does not read 2 members: \(app.element("cabal-member-count").label)"
            )
        }

        recorder.step("S2.2", "the \u{2026} menu explains why the creator cannot leave") {
            let note = openLeaveSheet(app, reaching: "cabalLeaveCreatorNote", step: "S2.2")
            XCTAssertEqual(note.label, "You can leave once everyone else has left.", "S2.2: the creator note")
            XCTAssertFalse(app.buttons["cabalLeaveButton"].exists, "S2.2: the creator is offered Leave cabal")
        }
    }
}
