import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class ProposalDetailMappingTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_792_662_780)
    private let utc: Calendar = {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(secondsFromGMT: 0) ?? .current
        return calendar
    }()

    func testTheStatsSurviveTheRepositoryConversion() async throws {
        let detail = ProposalDetail(.filledNVDA(now: now))
        XCTAssertEqual(detail.stats.priceMicros, 220_000_000)
        XCTAssertEqual(detail.stats.sharesAtomics, 182_000_000)
        XCTAssertTrue(detail.stats.sharesExact)
        XCTAssertEqual(detail.stats.tokenDecimals, 8)
        XCTAssertEqual(detail.stats.treasuryShareBps, 200)
        XCTAssertEqual(detail.stats.sinceBps, 30)
        XCTAssertEqual(detail.stats.closesAt, now.addingTimeInterval(14_400))
        XCTAssertEqual(detail.stats.closedAt, now.addingTimeInterval(-720))
        XCTAssertEqual(detail.replyCount, 6)
        XCTAssertNil(detail.myVote)
    }

    func testAStatsBlockWithOnlyItsRequiredFieldsMapsToNilFigures() async throws {
        let detail = ProposalDetail(.sample())
        XCTAssertNil(detail.stats.priceMicros)
        XCTAssertNil(detail.stats.sharesAtomics)
        XCTAssertFalse(detail.stats.sharesExact)
        XCTAssertNil(detail.stats.sinceBps)
    }

    func testTheCallersPlaceInTheVoteSurvivesTheConversion() async throws {
        var fixture = Components.Schemas.ProposalDetail.openNVDA(now: now)
        fixture.myVote = .init(order: 6, of: 8, yesOrder: 5, noOrder: 0, decided: true)
        XCTAssertEqual(
            ProposalDetail(fixture).myVote, ProposalMyVote(order: 6, of: 8, yesOrder: 5, noOrder: 0, decided: true))
    }

    func testTheSwapKeepsItsFeeAndFillTime() async throws {
        let swap = try XCTUnwrap(ProposalDetail(.filledNVDA(now: now)).summary.swap)
        XCTAssertEqual(swap.feeMicros, 400_000)
        XCTAssertEqual(swap.filledAt, now.addingTimeInterval(-720))
        XCTAssertEqual(swap.priceMicros, 219_400_000)
        let unfilled = try XCTUnwrap(ProposalDetail(.failedSwap(retryable: true)).summary.swap)
        XCTAssertNil(unfilled.feeMicros)
        XCTAssertNil(unfilled.filledAt)
    }

    func testTheModelBuiltFromAFixtureYieldsTheCells() async throws {
        let transport = StubTransport(.failure(URLError(.notConnectedToInternet)))
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        let now = now
        let cases: [(Components.Schemas.ProposalDetail, Components.Schemas.AssetDetail, [String])] = [
            (.openNVDA(now: now), .nvda, ["$220.00", "≈ 1.82", "2%", "1:53 PM"]),
            (.passedNVDA(now: now), .nvda, ["$220.00", "≈ 1.82", "2%", "9:41 AM"]),
            (.filledNVDA(now: now), .nvda, ["$219.40", "1.82", "$220.00", "+0.3%"]),
            (.filledMETA(now: now), .nvda, ["$716.52", "0.42", "$724.40", "+1.1%"]),
            (.failedTSLA(now: now), .nvda, ["$412.55", "2.2", "4%", "Oct 18"]),
            (.preIpoSpaceX(now: now), .spaceX, ["$185.00", "≈ 2.7", "2.2%", "Fri 5:53 AM"]),
        ]
        for (fixture, asset, values) in cases {
            let model = ProposalDetailModel(
                sample: fixture, members: Components.Schemas.Cabal.sampleWithMembers(role: "member").members,
                asset: asset, repository: ProposalsRepository(api: api), hints: FakeHintStream(), now: { now },
                calendar: utc)
            XCTAssertEqual(model.stats?.cells.map(\.value), values, "\(fixture.symbol) \(fixture.status)")
        }
    }
}
