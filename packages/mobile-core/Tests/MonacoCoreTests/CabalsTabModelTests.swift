import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class CabalsTabModelTests: XCTestCase {
    func testTheGridHoldsEveryMembershipInPortfolioOrderAndTheInvitationApart() async throws {
        let model = CabalsTabModel(api: api(try Self.transport()))

        await model.load()

        let content = try XCTUnwrap(model.content)
        XCTAssertEqual(content.tiles.map(\.shortName), ["AC Capital", "Tech Titans", "Degens"])
        XCTAssertEqual(content.tiles.map(\.returnText), ["+4.7%", "−1.7%", "+11.2%"])
        XCTAssertEqual(content.invites.map(\.shortName), ["Quant Kids"])
        XCTAssertEqual(content.countText, "3 · 1 invited")
    }

    func testAJoinedButUnfundedCabalIsATileAndNotAnInvitation() async throws {
        var portfolio = Components.Schemas.MyPortfolio.sampleLoss
        portfolio.cabals[0].valueMicros = 0
        let model = CabalsTabModel(api: api(try Self.transport(portfolio: portfolio, invites: "[]")))

        await model.load()

        let content = try XCTUnwrap(model.content)
        XCTAssertEqual(content.tiles.count, 2)
        XCTAssertTrue(content.invites.isEmpty)
        XCTAssertEqual(content.tiles.map(\.isFunded), [false, false])
        XCTAssertEqual(content.tiles.map(\.returnText), ["0%", "0%"])
        XCTAssertEqual(content.countText, "2")
    }

    func testTheHintShowsFromTwoCabals() async throws {
        var portfolio = Components.Schemas.MyPortfolio.sampleOwnProfile
        portfolio.cabals = Array(portfolio.cabals.prefix(1))
        let one = CabalsTabModel(api: api(try Self.transport(portfolio: portfolio, invites: "[]")))
        await one.load()
        let two = CabalsTabModel(api: api(try Self.transport()))
        await two.load()

        XCTAssertEqual(one.content?.showsReorderHint, false)
        XCTAssertEqual(two.content?.showsReorderHint, true)
    }

    func testTheShareLineFormatsTheTreasuryAndOwnership() async throws {
        let model = CabalsTabModel(api: api(try Self.transport()))

        await model.load()

        let tiles = try XCTUnwrap(model.content?.tiles)
        XCTAssertEqual(tiles[0].shareLine, "$20,000 treasury · 35% yours")
        XCTAssertEqual(tiles[1].shareLine, "$22,580 treasury · 12% yours")
        XCTAssertEqual(tiles[1].valueText, "$2,710.00")
    }

    func testALoadedTileCarriesItsUnreadCountFromMyCabalsById() async throws {
        let ids = Components.Schemas.MyPortfolio.sampleOwnProfile.cabals.map(\.cabal.id)
        let transport = try Self.transport(cabals: Self.cabalsJSON(unread: [ids[1]: 2]))
        let model = CabalsTabModel(api: api(transport))

        await model.load()

        XCTAssertEqual(try XCTUnwrap(model.content).tiles.map(\.unreadCount), [0, 2, 0])
    }

    func testAFailingMyCabalsStillLoadsTilesWithNoUnread() async throws {
        let transport = try Self.transport(cabals: nil)
        let model = CabalsTabModel(api: api(transport))

        await model.load()

        XCTAssertEqual(try XCTUnwrap(model.content).tiles.map(\.unreadCount), [0, 0, 0])
    }

    func testTheUnreadCopyIsSilentAtZeroAndPluralFromTwo() async throws {
        XCTAssertNil(CabalsTabCopy.unread(0))
        XCTAssertEqual(CabalsTabCopy.unread(1), "1 unread message")
        XCTAssertEqual(CabalsTabCopy.unread(2), "2 unread messages")
    }

    func testTheTabReadsThePortfolioAndTheInvitesOnce() async throws {
        let transport = try Self.transport()
        let model = CabalsTabModel(api: api(transport))

        await model.load()

        let sent = await transport.sent
        let paths = sent.compactMap(\.path).sorted()
        XCTAssertEqual(paths, ["/v1/me/cabal-invites", "/v1/me/cabals", "/v1/me/portfolio"])
    }

    func testAnOrderAppliedLocallyMovesTheTiles() async throws {
        let model = CabalsTabModel(api: api(try Self.transport()))
        await model.load()
        let ids = try XCTUnwrap(model.content?.tiles.map(\.id))

        model.applyOrder([ids[2], ids[0], ids[1]])

        XCTAssertEqual(model.content?.tiles.map(\.shortName), ["Degens", "AC Capital", "Tech Titans"])
    }

    func testAFailedRefreshKeepsTheTilesAndRaisesAToast() async throws {
        let transport = try Self.transport(
            portfolioReplies: [try Self.portfolioReply(), .failure(URLError(.notConnectedToInternet))],
            inviteReplies: [.json(.ok, Self.quantKidsInvite), .json(.ok, Self.quantKidsInvite)])
        let model = CabalsTabModel(api: api(transport))
        await model.load()

        await model.load()

        XCTAssertEqual(model.content?.tiles.count, 3)
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertEqual(model.lastError.map(ToastCopy.message(for:)), "You're offline. Try again.")
    }

    func testAFirstLoadThatFailsIsAFailure() async throws {
        let transport = try Self.transport(
            portfolioReplies: [.failure(URLError(.notConnectedToInternet))], inviteReplies: [.json(.ok, "[]")])
        let model = CabalsTabModel(api: api(transport))

        await model.load()

        guard case .failed = model.state else {
            XCTFail("expected a failure, got \(model.state)")
            return
        }
        XCTAssertEqual(model.failureTick, 1)
    }

    func testACancelledFirstLoadDoesNotLeaveTheTabLoading() async throws {
        let transport = try Self.transport(portfolioReplies: [.hang], inviteReplies: [.hang])
        let model = CabalsTabModel(api: api(transport))
        let load = Task { await model.load() }
        let started = await waitUntil { await transport.sent.count >= 3 }
        XCTAssertTrue(started)

        load.cancel()
        await load.value

        guard case .idle = model.state else {
            XCTFail("expected the tab back at idle, got \(model.state)")
            return
        }
    }

    func testACancelledRefreshKeepsTheLoadedTiles() async throws {
        let transport = try Self.transport(
            portfolioReplies: [try Self.portfolioReply(), .hang],
            inviteReplies: [.json(.ok, Self.quantKidsInvite), .hang])
        let model = CabalsTabModel(api: api(transport))
        await model.load()
        let refresh = Task { await model.load() }
        let started = await waitUntil { await transport.sent.count >= 6 }
        XCTAssertTrue(started)

        refresh.cancel()
        await refresh.value

        XCTAssertEqual(model.content?.tiles.count, 3)
        XCTAssertEqual(model.failureTick, 0)
    }

    func testAnyUserHintReadsAgain() async throws {
        let transport = try Self.transport(
            portfolioReplies: [try Self.portfolioReply(), try Self.portfolioReply()],
            inviteReplies: [.json(.ok, Self.quantKidsInvite), .json(.ok, Self.quantKidsInvite)])
        let hints = FakeHintStream()
        let model = CabalsTabModel(api: api(transport))
        await model.load()
        let observer = Task { await model.observe(hints: hints) }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.user("me"), what: "cabals", id: "1"))

        let refreshed = await waitUntil { await transport.sent.count == 6 }
        XCTAssertTrue(refreshed)
    }

    func testPullToRefreshRunsTheLatestReloadOfEverySection() async {
        let refresh = ScreenRefresh()
        var runs: [String] = []
        refresh.register("list") { runs.append("stale list") }
        refresh.register("list") { runs.append("list") }
        refresh.register("chart") { runs.append("chart") }

        await refresh.run()

        XCTAssertEqual(runs.sorted(), ["chart", "list"])
    }

    private func api(_ transport: StubTransport) -> APIClient {
        APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }

    private static func portfolioReply(
        _ portfolio: Components.Schemas.MyPortfolio = .sampleOwnProfile
    ) throws -> StubTransport.Reply {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return .json(.ok, String(decoding: try encoder.encode(portfolio), as: UTF8.self))
    }

    private static func transport(
        portfolio: Components.Schemas.MyPortfolio = .sampleOwnProfile, invites: String? = nil,
        cabals: String? = "[]"
    ) throws -> StubTransport {
        try transport(
            portfolioReplies: [try portfolioReply(portfolio)], inviteReplies: [.json(.ok, invites ?? quantKidsInvite)],
            cabalsReplies: [cabals.map { .json(.ok, $0) } ?? .failure(URLError(.notConnectedToInternet))])
    }

    private static func transport(
        portfolioReplies: [StubTransport.Reply], inviteReplies: [StubTransport.Reply],
        cabalsReplies: [StubTransport.Reply] = Array(repeating: .json(.ok, "[]"), count: 4)
    ) throws -> StubTransport {
        StubTransport(routes: [
            "/v1/me/cabals": cabalsReplies,
            "/v1/me/portfolio": portfolioReplies,
            "/v1/me/cabal-invites": inviteReplies,
        ])
    }

    private static func cabalsJSON(unread: [String: Int]) -> String {
        let rows = unread.map { id, count in
            #"{"id":"\#(id)","name":"Cabal","picture_url":null,"role":"member","can_vote":true,"member_count":2,"#
                + #""joined_at":"2026-09-30T12:00:00Z","pending_request_count":0,"unread_count":\#(count)}"#
        }
        return "[\(rows.joined(separator: ","))]"
    }

    private static let quantKidsInvite = """
        [{"request_id":"01890a5d-ac96-774b-bcce-b302099a8059","cabal":{"id":"01890a5d-ac96-774b-bcce-b302099a805d","name":"Quant Kids","picture_url":null,"member_count":5},"invited_by":{"user_id":"01890a5d-ac96-774b-bcce-b302099a8058","handle":"kai","display_name":"Kai","photo_url":null},"expires_at":"2026-10-09T15:00:00Z"}]
        """
}
