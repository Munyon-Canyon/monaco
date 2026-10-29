import Foundation
import MonacoAPI
import MonacoGovernance
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class HomeCabalRowsTests: XCTestCase {
    private let acCapital = Components.Schemas.CabalRef.sampleACCapital.id
    private let techTitans = Components.Schemas.CabalRef.sampleTechTitans.id
    private let degens = "01890a5d-ac96-774b-bcce-b302099a805c"
    private let unfunded = "01890a5d-ac96-774b-bcce-b302099a805d"

    private func stake(
        _ id: String, _ name: String, value: Int64, pnl: Int64, returnBps: Int64? = nil, ownership: Int32,
        members: Int32
    ) -> Components.Schemas.PortfolioCabal {
        let net: Int64 = value - pnl
        return Components.Schemas.PortfolioCabal(
            cabal: Components.Schemas.CabalRef(id: id, name: name, pictureUrl: nil), valueMicros: value,
            shareUnits: value, netContributedMicros: net, pnlMicros: pnl, returnBps: returnBps, sliceBps: 0,
            ownershipBps: ownership, role: .member, canVote: true, potValueMicros: 0, memberCount: members,
            openVoteCount: 0)
    }

    private func portfolio(_ cabals: [Components.Schemas.PortfolioCabal]) -> PortfolioSummary {
        PortfolioSummary(
            Components.Schemas.MyPortfolio(
                cashMicros: 0, netWorthMicros: 0, totalValueMicros: 0, pnlMicros: 0, returnBps: nil, computedAt: nil,
                pricesAsOf: nil, cabals: cabals))
    }

    private var threeCabals: [Components.Schemas.PortfolioCabal] {
        [
            stake(
                acCapital, "AC Capital", value: 7_000_000_000, pnl: 314_220_000, returnBps: 470, ownership: 3_500,
                members: 8),
            stake(
                techTitans, "Tech Titans", value: 2_710_000_000, pnl: -46_860_000, returnBps: -170, ownership: 1_200,
                members: 6),
            stake(
                degens, "Degens", value: 1_872_000_000, pnl: 188_550_000, returnBps: 1_120, ownership: 800,
                members: 6),
        ]
    }

    private func content(_ state: HomeCabalsState) throws -> HomeCabalsContent {
        guard case .cabals(let content) = state else {
            XCTFail("expected cabals, got \(state)")
            throw CancellationError()
        }
        return content
    }

    private func invite(_ name: String = "Quant Kids", by inviter: String = "Sam") -> HomeInviteRow {
        HomeInviteRow(
            Components.Schemas.CabalInvite(
                requestId: "01890a5d-ac96-774b-bcce-b302099a8100",
                cabal: .init(id: "01890a5d-ac96-774b-bcce-b302099a8101", name: name, pictureUrl: nil, memberCount: 5),
                invitedBy: .init(
                    userId: "01890a5d-ac96-774b-bcce-b302099a8102", handle: nil, displayName: inviter, photoUrl: nil),
                expiresAt: Date(timeIntervalSince1970: 1_791_590_400)))
    }

    private func pending(_ id: String, cabalID: String) -> HomePendingVote {
        HomePendingVote(
            id: id, cabalID: cabalID, cabalName: "Cabal", kind: "buy", symbol: "NVDAx", assetName: "NVIDIA",
            usdcMicros: 400_000_000, tally: ProposalTally(yes: 4, no: 1, voters: 8, needed: 5), replyCount: 0,
            expiresAt: Date(timeIntervalSince1970: 1_800_000_000))
    }

    func testTheSubLineNamesPendingVotesWhenThereAreSome() async throws {
        let shown = try content(HomeCabalList.state(portfolio: portfolio(threeCabals), invites: []))
        XCTAssertEqual(shown.cabals[0].subtitle(votesText: "1 vote open"), "1 vote open · 35% yours")
        XCTAssertEqual(shown.cabals[0].subtitle(votesText: "2 votes open"), "2 votes open · 35% yours")
    }

    func testTheSubLineNamesMembersOtherwise() async throws {
        let shown = try content(HomeCabalList.state(portfolio: portfolio(threeCabals), invites: []))
        XCTAssertEqual(shown.cabals[1].subtitle(votesText: nil), "6 members · 12% yours")
        XCTAssertEqual(shown.cabals[2].subtitle(votesText: nil), "6 members · 8% yours")
    }

    func testRowsKeepTheReadsOrderAndFigures() async throws {
        let shown = try content(HomeCabalList.state(portfolio: portfolio(threeCabals), invites: []))
        XCTAssertEqual(shown.cabals.map(\.id), [acCapital, techTitans, degens])
        XCTAssertEqual(shown.cabals.map(\.valueMicros), [7_000_000_000, 2_710_000_000, 1_872_000_000])
        XCTAssertEqual(shown.cabals.map(\.returnBps), [470, -170, 1_120])
    }

    func testAJoinedButUnfundedMembershipIsARowWithZeroPercentYours() async throws {
        let zero = stake(unfunded, "Quiet Money", value: 0, pnl: 0, ownership: 0, members: 4)
        let shown = try content(HomeCabalList.state(portfolio: portfolio(threeCabals + [zero]), invites: []))
        let row = try XCTUnwrap(shown.cabals.first { $0.id == unfunded })
        XCTAssertEqual(row.subtitle(votesText: nil), "4 members · 0% yours")
        XCTAssertEqual(row.valueMicros, 0)
        XCTAssertNil(row.returnBps)
        XCTAssertFalse(row.isFunded)
    }

    func testAOneMemberCabalReadsOneMember() async throws {
        let solo = stake(unfunded, "Solo", value: 0, pnl: 0, ownership: 0, members: 1)
        let shown = try content(HomeCabalList.state(portfolio: portfolio([solo]), invites: []))
        XCTAssertEqual(shown.cabals[0].subtitle(votesText: nil), "1 member · 0% yours")
    }

    func testTheCountReadsCabalsThenInvited() async throws {
        let withInvite = try content(HomeCabalList.state(portfolio: portfolio(threeCabals), invites: [invite()]))
        XCTAssertEqual(withInvite.countText, "3 · 1 invited")
        let without = try content(HomeCabalList.state(portfolio: portfolio(threeCabals), invites: []))
        XCTAssertEqual(without.countText, "3")
    }

    func testHomeShowsAtMostThreeCabalsPlusEveryInvite() async throws {
        let four = stake(unfunded, "Fourth", value: 100_000_000, pnl: 0, ownership: 100, members: 3)
        let shown = try content(
            HomeCabalList.state(
                portfolio: portfolio(threeCabals + [four]), invites: [invite(), invite("Other", by: "Mia")]))
        XCTAssertEqual(shown.shown.map(\.id), [acCapital, techTitans, degens])
        XCTAssertEqual(shown.invites.count, 2)
        XCTAssertEqual(shown.countText, "4 · 2 invited")
    }

    func testAnInviteReadsInvitedByAndTheMembers() async {
        let row = invite()
        XCTAssertEqual(row.name, "Quant Kids")
        XCTAssertEqual(row.subtitle, "Invited by Sam · 5 members")
    }

    func testNoCabalsAndNoInvitesIsEmptyOnceTheInvitesAreRead() async {
        XCTAssertEqual(HomeCabalList.state(portfolio: portfolio([]), invites: []), .empty)
        XCTAssertEqual(HomeCabalList.state(portfolio: portfolio([]), invites: nil), .loading)
    }

    func testAnInviteAloneIsStillShown() async throws {
        let shown = try content(HomeCabalList.state(portfolio: portfolio([]), invites: [invite()]))
        XCTAssertEqual(shown.countText, "0 · 1 invited")
        XCTAssertTrue(shown.shown.isEmpty)
    }

    func testHomeVoteNeedsCountsRowsByCabalAndSpeaksThem() async {
        let needs = HomeVoteNeeds()
        let rows = [
            pending("p-1", cabalID: acCapital), pending("p-2", cabalID: techTitans),
            pending("p-3", cabalID: techTitans),
        ]
        needs.update(rows: rows, votedThisSession: ["p-3"])
        XCTAssertEqual(needs.text(for: acCapital), "1 vote open")
        XCTAssertEqual(needs.text(for: techTitans), "1 vote open")
        needs.update(rows: rows, votedThisSession: [])
        XCTAssertEqual(needs.text(for: techTitans), "2 votes open")
        XCTAssertNil(needs.text(for: degens))
    }

    func testALoadReadsThePortfolioAndTheInvitesOnceAndNeverTheOldCabalReads() async throws {
        let portfolioBody = try Self.json(
            Components.Schemas.MyPortfolio(
                cashMicros: 0, netWorthMicros: 0, totalValueMicros: 0, pnlMicros: 0, returnBps: nil, computedAt: nil,
                pricesAsOf: nil, cabals: threeCabals))
        let transport = StubTransport(routes: [
            "/v1/me/portfolio": [.json(.ok, portfolioBody)],
            "/v1/me/cabal-invites": [.json(.ok, try Self.json([Components.Schemas.CabalInvite.sample]))],
        ])
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        let portfolio = PortfolioModel(api: api, hints: FakeHintStream())
        let context = HomeCabalContextModel(api: api)

        await portfolio.load()
        await context.load()
        await context.load()
        let summary = try XCTUnwrap(portfolio.summary)
        let shown = try content(HomeCabalList.state(portfolio: summary, invites: context.invites))

        let paths = await transport.sent.compactMap { $0.path.flatMap { URLComponents(string: $0)?.path } }
        XCTAssertEqual(paths.sorted(), ["/v1/me/cabal-invites", "/v1/me/portfolio"])
        XCTAssertEqual(shown.countText, "3 · 1 invited")
    }

    private static func json<Value: Encodable>(_ value: Value) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }
}
