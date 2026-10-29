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

    func testTheFailedFixtureCarriesWhoDidNotVote() async throws {
        let detail = ProposalDetail(.failedTSLA(now: now))
        let missing = detail.voters.filter { !$0.voted }
        XCTAssertEqual(missing.count, 1)
        XCTAssertNil(missing.first?.ballot)
        let members = Components.Schemas.Cabal.sampleWithMembers(role: "member").members.map(ProposalMember.init)
        let names = members.filter { member in missing.contains { $0.id == member.id } }.map(\.name)
        XCTAssertEqual(names, ["Priya"])
        XCTAssertEqual(
            ProposalResultCopy.notVotedRow(names: names, status: detail.summary.status), "Priya did not vote")
    }

    func testAVoterWithoutAnExplicitFlagReadsFromItsBallot() {
        XCTAssertTrue(ProposalVoter(id: "a", ballot: "yes").voted)
        XCTAssertFalse(ProposalVoter(id: "b", ballot: nil).voted)
        XCTAssertTrue(ProposalVoter(id: "c", ballot: nil, voted: true).voted)
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
            (.preIpoSpaceX(now: now), .spaceX, ["$185.00", "≈ 2.70", "2.2%", "Fri 5:53 AM"]),
        ]
        for (fixture, asset, values) in cases {
            let model = ProposalDetailModel(
                sample: fixture, members: Components.Schemas.Cabal.sampleWithMembers(role: "member").members,
                asset: asset, repository: ProposalsRepository(api: api), hints: FakeHintStream(), now: { now },
                calendar: utc)
            XCTAssertEqual(model.stats?.cells.map(\.value), values, "\(fixture.symbol) \(fixture.status)")
        }
    }

    func testTheSpaceXAssetDetailMapsItsPreIpoFactsAndEligibility() async throws {
        let asset = ProposalAsset(.spaceX)
        XCTAssertEqual(asset.kind, .preIpo)
        XCTAssertTrue(asset.tradable)
        XCTAssertEqual(asset.preIpo?.eligibility, .eligible)
        XCTAssertEqual(asset.preIpo?.profile?.estimateText, "$185.00")
        var research = Components.Schemas.AssetDetail.spaceX
        research.tradable = false
        XCTAssertFalse(ProposalAsset(research).tradable)
        XCTAssertEqual(ProposalAsset(research).preIpo?.eligibility, .researchOnly)
        XCTAssertNil(ProposalAsset(.nvda).preIpo)
    }

    func testTheNVDAAssetDetailMapsItsMarketStats() async throws {
        let stats = ProposalAsset(.nvda).marketStats
        XCTAssertEqual(stats.listingNetworks, [.solana])
        XCTAssertEqual(stats.priceSource, .jupiter)
        XCTAssertEqual(stats.priceAsOf, Date(timeIntervalSince1970: 1_772_596_200))
        XCTAssertEqual(stats.pollInterval, .seconds(120))
        XCTAssertFalse(stats.tradesContinuously)
    }

    func testARedactedFixtureMapsWithoutBallotsOrCastTimes() async throws {
        var fixture = Components.Schemas.ProposalDetail.filledNVDA(now: now)
        fixture.sidesVisible = false
        let viewer = fixture.voters[0].userId
        fixture.voters = fixture.voters.map { voter in
            var copy = voter
            if copy.userId != viewer {
                copy.choice = nil
                copy.castAt = nil
            }
            copy.photoUrl = "https://example.com/\(copy.userId).png"
            return copy
        }
        let detail = ProposalDetail(fixture)
        XCTAssertFalse(detail.sidesVisible)
        let others = detail.voters.filter { $0.id != viewer }
        XCTAssertTrue(others.allSatisfy { $0.castAt == nil && $0.ballot == nil })
        XCTAssertTrue(detail.voters.allSatisfy { !$0.displayName.isEmpty && $0.photoURL != nil })
        let sides = ProposalSides.make(voters: detail.voters, members: [], viewerID: viewer, sidesVisible: false)
        XCTAssertEqual(sides.yes.map(\.id), [viewer])
        XCTAssertEqual(sides.votedHidden.count, 1)
        XCTAssertEqual(sides.notVoted.count, 1)
    }

    func testARevealedFixtureCarriesEveryCastTimeAndOrdersByIt() async throws {
        var fixture = Components.Schemas.ProposalDetail.filledNVDA(now: now)
        fixture.voters[0].castAt = now.addingTimeInterval(-10)
        fixture.voters[1].castAt = now.addingTimeInterval(-20)
        let detail = ProposalDetail(fixture)
        XCTAssertTrue(detail.sidesVisible)
        XCTAssertTrue(detail.voters.filter(\.voted).allSatisfy { $0.castAt != nil })
        let sides = ProposalSides.make(voters: detail.voters, members: [], viewerID: "nobody", sidesVisible: true)
        XCTAssertEqual(sides.yes.map(\.id), [detail.voters[1].id, detail.voters[0].id])
    }
}
