import XCTest

nonisolated final class AgentsTradingBotJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1AOpenBot() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        AgentsTradingBotJourney.openBot(app, run: run, recorder: AgentsTradingBotJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-bot")
    }

    @MainActor
    func testS2Phase1AProposeBot() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        AgentsTradingBotJourney.proposeBot(app, run: run, recorder: AgentsTradingBotJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-proposed")
    }
}
