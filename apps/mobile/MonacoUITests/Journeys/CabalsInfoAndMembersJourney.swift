import XCTest

enum CabalsInfoAndMembersJourney {
    static let id = "cabals/info-and-members"
    static let version = 1

    static let screenTimeout: TimeInterval = 15

    struct Seed {
        let cabalName: String
        let creatorID: String
        let memberID: String
        let memberName: String

        static func handedOff() throws -> Seed {
            Seed(
                cabalName: try JourneyHandoff.read("cabalName"),
                creatorID: try JourneyHandoff.read("creatorID"),
                memberID: try JourneyHandoff.read("memberID"),
                memberName: try JourneyHandoff.read("memberName"))
        }
    }

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func waitForLabel(_ element: XCUIElement, containing text: String, timeout: TimeInterval) -> Bool {
        JoinJourney.waitForLabel(element, containing: text, timeout: timeout)
    }

    static func memberRow(_ app: XCUIApplication, label predicate: String, _ argument: String) -> XCUIElement {
        app.buttons.matching(
            NSPredicate(format: "identifier == 'members-list' AND label \(predicate) %@", argument)
        ).firstMatch
    }

    static func helper(_ app: XCUIApplication) -> XCUIElement {
        app.staticTexts.matching(NSPredicate(format: "label CONTAINS 'eligible voter'")).firstMatch
    }

    static func informationAndMembers(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the cabal") {
            JoinJourney.openBySearch(app, seed.cabalName, step: "S1.1")
        }

        recorder.step("S1.2", "tap the information round") {
            app.element("cabal-details-button").tap()
            XCTAssertTrue(
                app.navigationBars["Cabal information"].waitForExistence(timeout: screenTimeout),
                "S1.2: Cabal information did not open")
            let row = app.element("cabal-members-row")
            XCTAssertTrue(row.waitForExistence(timeout: screenTimeout), "S1.2: no Members & voters row")
            XCTAssertTrue(row.label.contains("Members & voters"), "S1.2: the row reads '\(row.label)'")
            XCTAssertTrue(row.label.contains("2 members"), "S1.2: the row reads '\(row.label)', not '2 members'")
        }

        recorder.step("S1.3", "open Members & voters") {
            app.element("cabal-members-row").tap()
            XCTAssertTrue(
                app.navigationBars["Members & voters"].waitForExistence(timeout: screenTimeout),
                "S1.3: Members & voters did not open")
            for tab in ["members", "voters", "requests"] {
                XCTAssertTrue(
                    app.element("members-tab-\(tab)").waitForExistence(timeout: 10), "S1.3: no members-tab-\(tab)")
            }
            XCTAssertTrue(
                waitForLabel(helper(app), containing: "2 members · 1 eligible voter", timeout: 10),
                "S1.3: no helper reading '2 members · 1 eligible voter'")
        }

        recorder.step("S1.4", "the member list names the roles") {
            let creator = memberRow(app, label: "ENDSWITH", "Admin")
            let member = memberRow(app, label: "BEGINSWITH", seed.memberName)
            XCTAssertTrue(creator.waitForExistence(timeout: 10), "S1.4: no row for the creator")
            XCTAssertTrue(creator.label.contains("Admin"), "S1.4: the creator's row reads '\(creator.label)'")
            XCTAssertTrue(member.waitForExistence(timeout: 10), "S1.4: no row for \(seed.memberName)")
            XCTAssertTrue(member.label.contains("Member"), "S1.4: the member's row reads '\(member.label)'")
            XCTAssertTrue(
                member.label.contains(seed.memberName), "S1.4: the member's row reads '\(member.label)'")
        }
    }

    static func pickAVoter(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        let creatorRow = app.element("voters-member-\(seed.creatorID)")
        let memberRow = app.element("voters-member-\(seed.memberID)")
        let save = app.buttons["edit-cabal-voters-save"]

        recorder.step("S2.1", "read the Voters tab") {
            app.element("members-tab-voters").tap()
            XCTAssertTrue(creatorRow.waitForExistence(timeout: 10), "S2.1: no voter row for the creator")
            XCTAssertTrue(creatorRow.label.contains("Voter"), "S2.1: the creator's row reads '\(creatorRow.label)'")
            XCTAssertTrue(
                waitForLabel(memberRow, containing: "Not voting", timeout: 10),
                "S2.1: the member's row reads '\(memberRow.label)', not 'Not voting'")
            let choose = app.buttons["members-choose-voters"]
            XCTAssertTrue(choose.waitForExistence(timeout: 10), "S2.1: no Choose voters button")
            XCTAssertEqual(choose.label, "Choose voters", "S2.1: the button reads '\(choose.label)'")
        }

        recorder.step("S2.2", "open Eligible voters") {
            app.buttons["members-choose-voters"].tap()
            let checklist = app.element("edit-cabal-voters")
            XCTAssertTrue(checklist.waitForExistence(timeout: 10), "S2.2: the Eligible voters sheet did not open")
            XCTAssertTrue(save.waitForExistence(timeout: 10), "S2.2: no Save voters button")
            XCTAssertFalse(save.isEnabled, "S2.2: Save voters is enabled before any change")
        }

        recorder.step("S2.3", "tick the member") {
            let row = app.element("edit-cabal-voters").descendants(matching: .any)
                .matching(identifier: "voters-member-\(seed.memberID)").firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 10), "S2.3: no row for \(seed.memberName) in the sheet")
            row.tap()
            XCTAssertTrue(
                ProfileOverviewJourney.waitUntil(5) { save.isEnabled },
                "S2.3: Save voters is still disabled after ticking \(seed.memberName)")
        }

        recorder.step("S2.4", "save the voters") {
            save.tap()
            JoinJourney.waitForToast(app, "Voters saved.", step: "S2.4")
            XCTAssertTrue(
                app.element("edit-cabal-voters").waitForNonExistence(timeout: 10),
                "S2.4: the Eligible voters sheet did not close")
            XCTAssertTrue(
                waitForLabel(memberRow, containing: "Voter", timeout: 10) && !memberRow.label.contains("Not voting"),
                "S2.4: the member's row reads '\(memberRow.label)', not 'Voter'")
            XCTAssertTrue(
                waitForLabel(helper(app), containing: "2 members · 2 eligible voters", timeout: 10),
                "S2.4: no helper reading '2 members · 2 eligible voters'")
        }
    }
}
