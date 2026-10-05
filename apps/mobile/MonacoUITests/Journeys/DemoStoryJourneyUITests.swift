import XCTest

nonisolated final class DemoStoryJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func signedIn() throws -> (XCUIApplication, String) {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        return (app, run)
    }

    @MainActor
    func testS1Phase1AStartsACabalAndSharesTheCode() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        try DemoStoryJourney.startAndShareTheCode(app, as: account, run: run, recorder: DemoStoryJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-invite-code")
    }

    @MainActor
    func testS1Phase2BJoinsWithTheCode() throws {
        let (app, run) = try signedIn()
        try DemoStoryJourney.friendJoins(app, run: run, recorder: DemoStoryJourney.recorder())
        attachScreenshot(of: app, named: "S1-B-joined")
    }

    @MainActor
    func testS2Phase1BFundsThePot() throws {
        let (app, run) = try signedIn()
        DemoStoryJourney.fundThePot(app, run: run, recorder: DemoStoryJourney.recorder())
        attachScreenshot(of: app, named: "S2-B-funded")
    }

    @MainActor
    func testS3Phase1BBrowsesStocks() throws {
        let (app, _) = try signedIn()
        DemoStoryJourney.browseStocks(app, recorder: DemoStoryJourney.recorder())
        attachScreenshot(of: app, named: "S3-B-pre-ipo")
    }

    @MainActor
    func testS4Phase1BProposesABuy() throws {
        let (app, run) = try signedIn()
        DemoStoryJourney.proposeABuy(app, run: run, recorder: DemoStoryJourney.recorder())
        attachScreenshot(of: app, named: "S4-B-proposed")
    }

    @MainActor
    func testS4Phase2AVotesYes() throws {
        let (app, _) = try signedIn()
        DemoStoryJourney.voteAndBuy(app, recorder: DemoStoryJourney.recorder())
        attachScreenshot(of: app, named: "S4-A-bought")
    }

    @MainActor
    func testS5Phase1BSaysItInTheChat() throws {
        let (app, run) = try signedIn()
        DemoStoryJourney.chatFirst(app, run: run, recorder: DemoStoryJourney.recorder())
        attachScreenshot(of: app, named: "S5-B-chat")
    }

    @MainActor
    func testS5Phase2AReplies() throws {
        let (app, run) = try signedIn()
        DemoStoryJourney.chatReply(app, run: run, recorder: DemoStoryJourney.recorder())
        attachScreenshot(of: app, named: "S5-A-chat")
    }

    @MainActor
    func testS6Phase1AAddsATradingBot() throws {
        let (app, run) = try signedIn()
        DemoStoryJourney.addABot(app, run: run, recorder: DemoStoryJourney.recorder())
        attachScreenshot(of: app, named: "S6-A-bot-proposed")
    }

    @MainActor
    func testS6Phase2BConnectsTheBot() throws {
        let (app, run) = try signedIn()
        DemoStoryJourney.connectTheBot(app, run: run, recorder: DemoStoryJourney.recorder())
        attachScreenshot(of: app, named: "S6-B-bot")
    }

    @MainActor
    func testS7Phase1ACashesOut() throws {
        let (app, run) = try signedIn()
        DemoStoryJourney.cashOut(app, run: run, recorder: DemoStoryJourney.recorder())
        attachScreenshot(of: app, named: "S7-A-cash-out")
    }

    @MainActor
    func testS8Phase1ASeesWhosUp() throws {
        let (app, _) = try signedIn()
        let friend = try JourneyAccount.load(actor: "B")
        DemoStoryJourney.seeWhosUp(app, friend: friend.name, recorder: DemoStoryJourney.recorder())
        attachScreenshot(of: app, named: "S8-A-board")
    }
}
