import XCTest

nonisolated final class MoneyDepositJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = MoneyDepositJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "A")
            MoneyDepositJourney.copySolana(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-how-it-works")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            MoneyDepositJourney.copyBase(app, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-base-copied")
        }

        try session.scenario("S3") {
            try session.act(as: "A")
            MoneyDepositJourney.openFromProfile(app, recorder: recorder)
            attachScreenshot(of: app, named: "S3-A-profile-deposit")
        }
    }
}
