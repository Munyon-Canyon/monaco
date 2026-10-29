import XCTest

enum CabalsInfoAndMembersJourney {
    static let id = "cabals/info-and-members"
    static let version = 2

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

    static func membersTab(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the cabal") {
            app.openCabal(named: seed.cabalName)
        }

        recorder.step("S1.2", "read the Members tab") {
            app.openCabalTab("Members")
            let tab = app.element("cabal-tab-members")
            XCTAssertTrue(
                waitForLabel(tab, containing: "2", timeout: 10), "S1.2: the Members tab reads '\(tab.label)', not 2")
            XCTAssertTrue(app.element("cabal-members").waitForExistence(timeout: 10), "S1.2: no Members tab body")
            let creator = app.element("cabal-member-\(seed.creatorID)")
            let member = app.element("cabal-member-\(seed.memberID)")
            XCTAssertTrue(creator.waitForExistence(timeout: 10), "S1.2: no row for the creator")
            XCTAssertTrue(creator.label.contains("Admin"), "S1.2: the creator's row reads '\(creator.label)'")
            XCTAssertTrue(creator.label.contains("you"), "S1.2: the creator's row reads '\(creator.label)'")
            XCTAssertTrue(member.waitForExistence(timeout: 10), "S1.2: no row for \(seed.memberName)")
            XCTAssertTrue(
                member.label.contains(seed.memberName), "S1.2: the member's row reads '\(member.label)'")
            XCTAssertTrue(
                member.label.contains("No votes this month"),
                "S1.2: the member's row reads '\(member.label)', not the empty record")
            XCTAssertTrue(
                app.element("cabal-members-footer").label.contains("Make a member a voter by proposal."),
                "S1.2: the Members footer")
        }

        recorder.step("S1.3", "See all opens Members all") {
            let seeAll = app.element("cabal-members-see-all")
            app.scrollIntoReach(seeAll)
            seeAll.tap()
            XCTAssertTrue(
                app.navigationBars["Members"].waitForExistence(timeout: screenTimeout),
                "S1.3: Members all did not open")
            XCTAssertTrue(
                app.element("members-list").waitForExistence(timeout: 10), "S1.3: no members-list")
            app.tapBack()
        }
    }

    static func makeAVoter(_ app: XCUIApplication, seed: Seed, recorder: JourneyRecorder) {
        recorder.step("S2.1", "read the Voters tab") {
            app.openCabalTab("Voters")
            XCTAssertTrue(app.element("cabal-voters").waitForExistence(timeout: 10), "S2.1: no Voters tab body")
            XCTAssertTrue(
                app.element("cabal-member-\(seed.creatorID)").waitForExistence(timeout: 10),
                "S2.1: the creator is not under Voters")
            XCTAssertFalse(
                app.element("cabal-member-\(seed.memberID)").exists,
                "S2.1: \(seed.memberName) is under Voters before the proposal")
        }

        recorder.step("S2.2", "open Make a voter from the member's row") {
            let member = app.element("cabal-member-\(seed.memberID)")
            XCTAssertTrue(member.waitForExistence(timeout: 10), "S2.2: no row for \(seed.memberName)")
            member.press(forDuration: 1)
            let action = app.buttons["cabal-voters-propose-voter"]
            XCTAssertTrue(action.waitForExistence(timeout: 5), "S2.2: no Propose making a voter action")
            action.tap()
            let words = app.element("cabal-rule-proposal-words")
            XCTAssertTrue(words.waitForExistence(timeout: 10), "S2.2: the proposal sheet did not open")
            XCTAssertTrue(
                words.label.contains("Make \(seed.memberName) a voter"), "S2.2: the sheet reads '\(words.label)'")
        }

        recorder.step("S2.3", "send the proposal") {
            app.buttons["cabal-rule-proposal-propose"].tap()
            JoinJourney.waitForToast(app, "Proposed. Your cabal votes on it.", step: "S2.3")
        }

        recorder.step("S2.4", "hold Yes on the open proposal") {
            let row = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'cabal-open-proposal-'"))
                .firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 15), "S2.4: no open proposal row")
            row.tap()
            app.pickAndHold("yes")
            app.tapBack()
        }

        recorder.step("S2.5", "the member shows under Voters") {
            app.openCabalTab("Voters")
            XCTAssertTrue(
                app.element("cabal-member-\(seed.memberID)").waitForExistence(timeout: 20),
                "S2.5: \(seed.memberName) is not under Voters after the proposal passed")
        }
    }
}
