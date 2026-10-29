import XCTest

nonisolated final class MoneyBestPriceTradeJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = MoneyBestPriceTradeJourney.recorder()
        let refundAddress = try MoneyWithdrawJourney.refundAddress()

        try session.scenario("S1") {
            let cabal = try JourneyHandoff.read("cabalName")
            try session.act(as: "A")
            MoneyBestPriceTradeJourney.fillThePot(app, cabal: cabal, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-funded")
        }

        try session.scenario("S2") {
            let cabal = try JourneyHandoff.read("cabalName")
            try session.act(as: "A")
            MoneyBestPriceTradeJourney.propose(app, cabal: cabal, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-proposal-sent")
        }

        try session.scenario("S3") {
            try session.act(as: "A")
            MoneyBestPriceTradeJourney.voteThrough(app, recorder: recorder)
            attachScreenshot(of: app, named: "S3-A-bought")
        }

        try session.scenario("S4") {
            let cabal = try JourneyHandoff.read("cabalName")
            try session.act(as: "A")
            MoneyBestPriceTradeJourney.seeTheTrade(app, cabal: cabal, recorder: recorder)
            attachScreenshot(of: app, named: "S4-A-trade")
        }

        try session.scenario("S5") {
            let cabal = try JourneyHandoff.read("cabalName")
            try session.act(as: "A")
            MoneyBestPriceTradeJourney.refund(app, cabal: cabal, to: refundAddress, recorder: recorder)
            attachScreenshot(of: app, named: "S5-A-withdrawn")
        }
    }
}
