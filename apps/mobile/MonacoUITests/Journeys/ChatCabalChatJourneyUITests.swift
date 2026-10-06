import XCTest

nonisolated final class ChatCabalChatJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let run = try JourneyRun.id()
        let recorder = ChatCabalChatJourney.recorder()

        try session.scenario("S1") {
            let sender = try session.act(as: "A")
            ChatCabalChatJourney.aSends(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-sent")
            try session.act(as: "B")
            ChatCabalChatJourney.bReceives(app, run: run, sender: sender.name, recorder: recorder)
            attachScreenshot(of: app, named: "S1-B-received")
        }
    }
}
