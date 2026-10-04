import XCTest

nonisolated final class CabalsEditRulesJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1EditsTheRules() throws {
        let account = try JourneyAccount.load()
        let member = try JourneyAccount.load(actor: "B")
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        let recorder = CabalsEditRulesJourney.recorder()
        CabalsEditRulesJourney.creatorEdits(app, run: run, recorder: recorder)
        try CabalsEditRulesJourney.creatorPicksVoters(app, run: run, member: member.name, recorder: recorder)
        attachScreenshot(of: app, named: "S1-A-edited")
    }
}
