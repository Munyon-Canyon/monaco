import XCTest

enum CabalsEditRulesJourney {
    static let id = "cabals/edit-rules"
    static let version = 1

    static let memberKey = "member-id"
    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func seededName(run: String) -> String { "QA rules \(run)" }
    static func newName(run: String) -> String { "QA renamed \(run)" }

    static func rule(_ app: XCUIApplication, _ row: String) -> XCUIElement {
        app.element("cabal-rules-\(row)")
    }

    static func assertRule(
        _ app: XCUIApplication, _ row: String, reads text: String, step: String, timeout: TimeInterval = 10
    ) {
        XCTAssertTrue(
            JoinJourney.waitForLabel(rule(app, row), containing: text, timeout: timeout),
            "\(step): cabal-rules-\(row) does not read \(text): \(rule(app, row).label)"
        )
    }

    static func creatorEdits(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = newName(run: run)
        let field = app.textFields["edit-cabal-name"]

        recorder.step("S1.1", "open the cabal as its creator") {
            JoinJourney.openBySearch(app, seededName(run: run), step: "S1.1")
        }

        recorder.step("S1.2", "open details and read the rules") {
            app.buttons["cabal-details-button"].tap()
            app.scrollIntoReach(app.element("cabal-rules"))
            XCTAssertTrue(app.element("cabal-rules").waitForExistence(timeout: 10), "S1.2: no Rules within 10 s")
            assertRule(app, "join", reads: "Anyone", step: "S1.2")
            assertRule(app, "voters", reads: "Every member", step: "S1.2")
            assertRule(app, "threshold", reads: "Majority", step: "S1.2")
            assertRule(app, "expiry", reads: "1 day", step: "S1.2")
            JoinJourney.snap(app, "S1-A-rules-before")
        }

        recorder.step("S1.3", "open the editor from a rule row") {
            rule(app, "threshold").tap()
            XCTAssertTrue(field.waitForExistence(timeout: 10), "S1.3: the Edit cabal screen did not show within 10 s")
            XCTAssertTrue(app.navigationBars["Edit cabal"].exists, "S1.3: the editor is not titled Edit cabal")
            XCTAssertEqual(field.value as? String, seededName(run: run), "S1.3: the name field")
            XCTAssertEqual(
                app.element("edit-cabal-rules-footer").label,
                "Rule changes apply to new proposals. Open votes keep their rules.",
                "S1.3: the rules footer"
            )
        }

        recorder.step("S1.4", "rename the cabal") {
            field.tap()
            let old = (field.value as? String) ?? ""
            field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: old.count))
            field.typeText(name)
            XCTAssertTrue(app.buttons["edit-cabal-save"].isEnabled, "S1.4: Save is not enabled after a rename")
        }

        recorder.step("S1.5", "pick Everyone agrees and 1 week") {
            let threshold = app.element("edit-rule-threshold")
            app.scrollIntoReach(threshold)
            threshold.buttons["Everyone agrees"].tap()
            let expiry = app.element("edit-rule-expiry")
            app.scrollIntoReach(expiry)
            expiry.buttons["1 week"].tap()
            XCTAssertTrue(
                threshold.staticTexts["Passes only if every voter says yes."].waitForExistence(timeout: 2),
                "S1.5: the To pass caption did not change"
            )
            XCTAssertTrue(
                expiry.staticTexts["A vote that hasn't passed closes after 1 week."].waitForExistence(timeout: 2),
                "S1.5: the Votes stay open caption did not change"
            )
        }

        recorder.step("S1.6", "save") {
            app.buttons["edit-cabal-save"].tap()
            JoinJourney.waitForToast(app, "Cabal updated.", step: "S1.6")
        }
    }

    static func creatorPicksVoters(
        _ app: XCUIApplication, run: String, member: String, recorder: JourneyRecorder
    ) throws {
        let memberID = try JourneyHandoff.read(memberKey)
        let name = newName(run: run)
        let field = app.textFields["edit-cabal-name"]

        recorder.step("S1.7", "open Who votes") {
            let voters = app.buttons["edit-cabal-voters"]
            app.scrollIntoReach(voters)
            voters.tap()
            XCTAssertTrue(app.buttons["voters-pick"].waitForExistence(timeout: 10), "S1.7: Who votes did not show")
            XCTAssertTrue(app.buttons["voters-everyone"].isSelected, "S1.7: Everyone is not selected")
        }

        recorder.step("S1.8", "pick B as a voter") {
            app.buttons["voters-pick"].tap()
            let row = app.buttons["voters-member-\(memberID)"]
            XCTAssertTrue(row.waitForExistence(timeout: 10), "S1.8: no row for \(member) within 10 s")
            XCTAssertTrue(row.label.contains(member), "S1.8: the voter row does not name \(member)")
            row.tap()
            XCTAssertTrue(row.isSelected, "S1.8: \(member) is not selected")
            XCTAssertTrue(app.buttons["voters-save"].isEnabled, "S1.8: Save is not enabled")
        }

        recorder.step("S1.9", "save the voters") {
            app.buttons["voters-save"].tap()
            JoinJourney.waitForToast(app, "Voters updated.", step: "S1.9")
        }

        recorder.step("S1.10", "go back to the rules") {
            app.navigationBars.buttons.firstMatch.tap()
            XCTAssertTrue(field.waitForExistence(timeout: 10), "S1.10: the editor did not come back")
            app.navigationBars.buttons.firstMatch.tap()
            assertRule(app, "threshold", reads: "Everyone agrees", step: "S1.10")
            assertRule(app, "expiry", reads: "1 week", step: "S1.10")
            assertRule(app, "voters", reads: member, step: "S1.10")
            JoinJourney.snap(app, "S1-A-rules-after")
        }

        recorder.step("S1.11", "close details and see the new name") {
            app.buttons["cabal-details-done"].tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("cabal-header-name"), containing: name, timeout: 10),
                "S1.11: the hero does not read \(name): \(app.element("cabal-header-name").label)"
            )
        }
    }
}
