import XCTest

nonisolated final class TalkItOverJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let run = try JourneyRun.id()
        let recorder = TalkItOverJourney.recorder()
        func switchTo(_ actor: String) throws {
            TalkItOverJourney.leaveScreens(app)
            try session.act(as: actor)
        }

        try session.scenario("S1") {
            try switchTo("A")
            TalkItOverJourney.anEmptyChat(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-empty")
        }

        try session.scenario("S2") {
            try switchTo("A")
            TalkItOverJourney.aSendsGm(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-sent")
            try switchTo("B")
            let sender = try session.account("A").name
            TalkItOverJourney.bReceivesGm(app, run: run, sender: sender, recorder: recorder)
            attachScreenshot(of: app, named: "S2-B-received")
        }

        try session.scenario("S3") {
            try switchTo("B")
            TalkItOverJourney.bRepliesInThread(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S3-B-thread")
            try switchTo("A")
            TalkItOverJourney.aSeesReply(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S3-A-replies")
        }

        try session.scenario("S4") {
            try switchTo("A")
            TalkItOverJourney.aDeletesTypo(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S4-A-deleted")
        }

        try session.scenario("S5") {
            try switchTo("A")
            TalkItOverJourney.aSendsTwo(app, run: run, recorder: recorder)
            try switchTo("B")
            TalkItOverJourney.bSeesBadge(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S5-B-badge")
            TalkItOverJourney.bOpensUnreadChat(app, run: run, recorder: recorder)
            TalkItOverJourney.bClearsUnread(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S5-B-cleared")
        }
    }
}
