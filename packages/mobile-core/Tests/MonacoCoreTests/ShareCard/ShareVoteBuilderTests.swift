import MonacoAPI
import XCTest

@testable import MonacoCore

final class ShareVoteBuilderTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_790_000_000)
    private let crest = ShareCrest(cabalID: "cabal-1", name: "AC Capital", pictureURL: nil)

    private func vote() -> Components.Schemas.ProposalDetail {
        var detail = Components.Schemas.ProposalDetail.openNVDA(now: now)
        detail.proposer = .init(id: detail.proposerId, displayName: "Mia Chen", photoUrl: nil)
        detail.tally = .init(yes: 4, no: 1, voters: 8, needed: 5)
        return detail
    }

    private func spec(
        _ detail: Components.Schemas.ProposalDetail? = nil,
        eyes: ShareEyes = ShareEyes(pnlDollars: false, buySell: false)
    ) -> ShareCardSpec {
        let chart = (0..<4).map {
            ShareChartPoint(
                time: now.addingTimeInterval(TimeInterval($0 * 3_600 - 14_400)),
                priceMicros: Int64(215 + $0) * 1_000_000)
        }
        return ShareVoteBuilder.build(
            proposal: detail ?? vote(), crest: crest, chart: chart, code: "andres", eyes: eyes, now: now,
            timeZone: TimeZone(identifier: "UTC") ?? .gmt)
    }

    func testTheFrameSpec() {
        let spec = spec()

        XCTAssertEqual(spec.header.title, "AC Capital")
        XCTAssertEqual(spec.header.subtitle, "Mia proposed \u{00B7} ends in 4h")
        XCTAssertEqual(spec.header.mark, .crest(crest))
        XCTAssertEqual(spec.tickerLine.symbol, "NVDA")
        XCTAssertEqual(spec.tickerLine.detail, "Buy")
        XCTAssertEqual(spec.figure.main, "$400")
        XCTAssertEqual(spec.figure.context, "2% of treasury")
        XCTAssertEqual(
            spec.stats,
            [
                .init(label: "Price", value: "$220.00"), .init(label: "Votes", value: "4 yes \u{00B7} 1 no"),
                .init(label: "Voted", value: "5 of 8"),
            ])
        XCTAssertTrue(spec.chart.markers.isEmpty)
        XCTAssertEqual(spec.chart.points.count, 4)
        XCTAssertEqual(spec.replayTitle, "Replay the price")
        XCTAssertEqual(spec.code, "ANDRES")
    }

    func testASellReadsSell() {
        var detail = vote()
        detail.kind = .sell
        detail.usdcMicros = nil
        detail.tokenAmount = 182_000_000
        detail.quoteOutAmount = 400_000_000

        let spec = spec(detail)
        XCTAssertEqual(spec.tickerLine.detail, "Sell")
        XCTAssertEqual(spec.figure.main, "$400")
    }

    func testAMissingTreasuryShareDropsTheContext() {
        var detail = vote()
        detail.stats.treasuryShareBps = nil
        XCTAssertNil(spec(detail).figure.context)
    }

    func testAMissingLivePriceReadsADash() {
        var detail = vote()
        detail.stats.priceMicros = nil
        XCTAssertEqual(spec(detail).stats.first?.value, "\u{2014}")
    }

    func testTheEyesChangeNothingButTheCode() {
        let plain = spec()
        let loud = spec(eyes: ShareEyes(pnlDollars: true, buySell: true))

        XCTAssertEqual(plain.figure, loud.figure)
        XCTAssertEqual(plain.chart, loud.chart)
        XCTAssertNil(spec(eyes: ShareEyes(code: false)).code)
    }

    func testTheVoteLinkCarriesTheCabalAndTheProposal() throws {
        let code = try XCTUnwrap(ReferralCode("andres"))
        let url = ReferralLink.url(for: code, cabalID: "cabal-1", proposalID: "proposal-1")
        let items = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems

        XCTAssertEqual(url.path, "/r/andres")
        XCTAssertEqual(items?.first { $0.name == "c" }?.value, "cabal-1")
        XCTAssertEqual(items?.first { $0.name == "p" }?.value, "proposal-1")
    }
}
