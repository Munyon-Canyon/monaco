import XCTest

nonisolated final class ChatCabalChatJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1ASends() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        try JourneyHandoff.write("senderName", account.name)
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        ChatCabalChatJourney.aSends(app, run: run, recorder: ChatCabalChatJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-sent")
    }

    @MainActor
    func testS1Phase2BReceives() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let sender = try JourneyHandoff.read("senderName")
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        ChatCabalChatJourney.bReceives(app, run: run, sender: sender, recorder: ChatCabalChatJourney.recorder())
        attachScreenshot(of: app, named: "S1-B-received")
    }
}
