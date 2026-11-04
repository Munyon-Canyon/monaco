import XCTest

enum CabalsEditRulesJourney {
    static let id = "cabals/edit-rules"
    static let version = 10

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
        let element = rule(app, row)
        let deadline = Date().addingTimeInterval(timeout)
        var read = false
        repeat {
            read = rowReads(element, text)
            if !read { RunLoop.current.run(until: Date().addingTimeInterval(0.2)) }
        } while !read && Date() < deadline
        XCTAssertTrue(read, "\(step): cabal-rules-\(row) does not read \(text): \(element.label)")
    }

    private static func rowReads(_ element: XCUIElement, _ text: String) -> Bool {
        guard element.exists else { return false }
        if element.label.contains(text) { return true }
        return element.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", text)).firstMatch.exists
    }

    static func holdYesOnCard(_ app: XCUIApplication, step: String, toast: String) {
        let yes = app.descendants(matching: .any)
            .matching(NSPredicate(format: "identifier BEGINSWITH 'cabal-open-proposal-yes-'")).firstMatch
        XCTAssertTrue(yes.waitForExistence(timeout: 15), "\(step): no Yes on the open proposal card")
        yes.tap()
        app.holdToCommit(yes)
        JoinJourney.waitForToast(app, toast, step: step)
    }

    static func creatorReadsRules(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the cabal as its creator") {
            app.openCabal(named: seededName(run: run))
        }

        recorder.step("S1.2", "read the Rules tab") {
            app.openCabalTab("Rules")
            XCTAssertTrue(app.element("cabal-rules").waitForExistence(timeout: 10), "S1.2: no Rules within 10 s")
            assertRule(app, "votesToPass", reads: "2 of 2", step: "S1.2")
            assertRule(app, "voteWindow", reads: "24 hours", step: "S1.2")
            assertRule(app, "maxPerTrade", reads: "No limit", step: "S1.2")
            assertRule(app, "whoCanPropose", reads: "Voters", step: "S1.2")
            assertRule(app, "cashOut", reads: "Any time", step: "S1.2")
            let sides = app.switches["cabal-rules-show-sides-toggle"]
            app.scrollIntoReach(sides)
            XCTAssertTrue(sides.waitForExistence(timeout: 10), "S1.2: no Show who voted what switch")
            XCTAssertEqual(sides.value as? String, "0", "S1.2: Show who voted what is not off")
            let footer = app.element("cabal-rules-footer")
            app.scrollIntoReach(footer)
            XCTAssertTrue(
                footer.label.hasPrefix("Everyone sees the count as it happens."),
                "S1.2: the footer reads '\(footer.label)'")
            XCTAssertTrue(
                footer.label.hasSuffix("Rules change by proposal."), "S1.2: the footer reads '\(footer.label)'")
            JoinJourney.snap(app, "S1-A-rules-before")
        }
    }

    static func creatorRenames(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = newName(run: run)
        let field = app.textFields["edit-cabal-name"]

        recorder.step("S1.3", "open Edit name and picture from the \u{2026} menu") {
            app.buttons["cabal-more-button"].tap()
            let edit = app.buttons["cabal-more-edit"]
            XCTAssertTrue(edit.waitForExistence(timeout: 5), "S1.3: the \u{2026} menu has no Edit name and picture")
            edit.tap()
            XCTAssertTrue(field.waitForExistence(timeout: 10), "S1.3: the editor did not show within 10 s")
        }

        recorder.step("S1.4", "rename the cabal") {
            field.tap()
            field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: seededName(run: run).count))
            field.typeText(name)
            let save = app.buttons["edit-cabal-save"]
            XCTAssertTrue(save.isEnabled, "S1.4: Save is not enabled after a rename")
            save.tap()
            JoinJourney.waitForToast(app, "Cabal updated.", step: "S1.4")
            XCTAssertTrue(
                app.waitForCabalTitle(name, timeout: 10),
                "S1.4: the cabal title does not read \(name)"
            )
        }
    }

    static func creatorProposesSides(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "turn on Show who voted what") {
            app.waitForToastGone()
            app.openCabalTab("Rules")
            let toggle = app.switches["cabal-rules-show-sides-toggle"]
            app.scrollIntoReach(toggle)
            XCTAssertTrue(toggle.waitForExistence(timeout: 10), "S2.1: no Show who voted what switch")
            toggle.tapSwitch()
            let words = app.element("cabal-rule-proposal-words")
            if !words.waitForExistence(timeout: 3) { toggle.tapSwitch() }
            XCTAssertTrue(words.waitForExistence(timeout: 10), "S2.1: the proposal sheet did not open")
            XCTAssertTrue(words.label.contains("on"), "S2.1: the sheet reads '\(words.label)'")
        }

        recorder.step("S2.2", "send the proposal") {
            app.buttons["cabal-rule-proposal-propose"].tap()
            JoinJourney.waitForToast(app, "Proposed. Your cabal votes on it.", step: "S2.2")
        }

        recorder.step("S2.3", "A holds Yes") {
            holdYesOnCard(app, step: "S2.3", toast: "Voted Yes.")
        }
    }

    static func memberVotes(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.4", "B holds Yes") {
            app.openCabal(named: newName(run: run))
            holdYesOnCard(app, step: "S2.4", toast: "Voted Yes.")
        }
    }

    static func creatorSeesSides(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.5", "the Show who voted what switch is on") {
            app.openCabal(named: newName(run: run))
            app.openCabalTab("Rules")
            XCTAssertTrue(app.element("cabal-rules").waitForExistence(timeout: 10), "S2.5: no Rules within 10 s")
            let sides = app.switches["cabal-rules-show-sides-toggle"]
            app.scrollIntoReach(sides)
            XCTAssertTrue(sides.waitForExistence(timeout: 10), "S2.5: no Show who voted what switch")
            XCTAssertTrue(
                FollowAndBoardsJourney.waitUntil(20) { sides.value as? String == "1" },
                "S2.5: Show who voted what is not on within 20 s")
            JoinJourney.snap(app, "S2-A-rules-sides-on")
        }
    }
}
