import MonacoAPI
import MonacoTestSupport
import XCTest

@testable import MonacoCore

final class ClosedProposalsModelTests: XCTestCase {
    @MainActor
    func testItStartsLoadingAndReadsTheClosedFilterAcrossCabals() async throws {
        let transport = Self.transport(
            first: Self.page([Self.proposal("a", cabal: "c1"), Self.proposal("b", cabal: "c2")]))
        let model = Self.model(transport)
        XCTAssertEqual(model.phase, .loading)

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.pager.items.map(\.id), ["a", "b"])
        XCTAssertEqual(model.pager.items.map(\.cabalName), ["Cabal c1", "Cabal c2"])
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertTrue(paths.first?.hasPrefix("/v1/me/proposals") == true, "\(paths)")
        XCTAssertTrue(paths.first?.contains("filter=closed") == true, "\(paths)")
    }

    @MainActor
    func testNoClosedProposalsIsLoadedAndEmpty() async throws {
        let model = Self.model(Self.transport(first: Self.page([])))
        await model.load()
        XCTAssertEqual(model.phase, .loaded)
        XCTAssertTrue(model.pager.items.isEmpty)
    }

    @MainActor
    func testAFailedFirstLoadIsFailedAndRetryLoads() async throws {
        let transport = StubTransport(scripted: [
            .failure(URLError(.notConnectedToInternet)), .json(.ok, Self.page([Self.proposal("a", cabal: "c1")])),
            .json(.ok, "{}"), .json(.ok, "{}"),
        ])
        let model = Self.model(transport)

        await model.load()
        XCTAssertEqual(model.phase, .failed)

        await model.load()
        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.pager.items.map(\.id), ["a"])
    }

    @MainActor
    func testRowsStayWhenALaterPageFails() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page([Self.proposal("a", cabal: "c1")], next: "more")), .json(.ok, "{}"), .json(.ok, "{}"),
            .failure(URLError(.notConnectedToInternet)),
        ])
        let model = Self.model(transport)
        await model.load()
        XCTAssertTrue(model.canLoadMore)

        await model.loadMore()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.pager.items.map(\.id), ["a"])
    }

    @MainActor
    func testLoadMoreFollowsTheCursorAndStopsAtTheLastPage() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page([Self.proposal("a", cabal: "c1")], next: "more")), .json(.ok, "{}"), .json(.ok, "{}"),
            .json(.ok, Self.page([Self.proposal("b", cabal: "c1")])),
        ])
        let model = Self.model(transport)
        await model.load()

        await model.loadMore()

        XCTAssertEqual(model.pager.items.map(\.id), ["a", "b"])
        XCTAssertFalse(model.canLoadMore)
        let paths = await transport.sent.compactMap(\.path)
        XCTAssertTrue(paths.contains { $0.contains("cursor=more") }, "\(paths)")
    }

    @MainActor
    func testItNamesEachProposerFromTheirCabalsMembers() async throws {
        let model = Self.model(Self.transport(first: Self.page([Self.proposal("a", cabal: "c1")])))
        await model.load()

        let summary = try XCTUnwrap(model.pager.items.first)
        XCTAssertEqual(model.proposerName(of: summary), "Jordan")
        XCTAssertEqual(model.assets["AAPLx"]?.displayName, "Apple")
    }

    @MainActor
    func testAProposerWhoIsNotListedIsAMember() async throws {
        let model = Self.model(Self.transport(first: Self.page([Self.proposal("a", cabal: "c1", proposer: "gone")])))
        await model.load()

        let summary = try XCTUnwrap(model.pager.items.first)
        XCTAssertEqual(model.proposerName(of: summary), "Member")
    }

    @MainActor
    func testRefreshPicksUpAProposalThatJustClosed() async throws {
        let transport = StubTransport(scripted: [
            .json(.ok, Self.page([Self.proposal("a", cabal: "c1")])), .json(.ok, "{}"), .json(.ok, "{}"),
            .json(.ok, Self.page([Self.proposal("b", cabal: "c1"), Self.proposal("a", cabal: "c1")])),
        ])
        let model = Self.model(transport)
        await model.load()

        await model.refresh()

        XCTAssertEqual(model.pager.items.map(\.id), ["b", "a"])
    }

    @MainActor
    func testAHintBeforeTheListWasOpenedReadsNothing() async throws {
        let transport = Self.transport(first: Self.page([Self.proposal("a", cabal: "c1")]))
        let model = Self.model(transport)

        await model.refreshIfStarted()

        XCTAssertEqual(model.phase, .loading)
        let sent = await transport.sent
        XCTAssertTrue(sent.isEmpty)
    }

    @MainActor
    private static func model(_ transport: StubTransport) -> ClosedProposalsModel {
        ClosedProposalsModel(
            repository: ProposalsRepository(
                api: APIClient(
                    serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)),
            hints: FakeHintStream())
    }

    private static func transport(first page: String) -> StubTransport {
        StubTransport(routes: [
            "/v1/me/proposals?filter=closed": [.json(.ok, page)],
            "/v1/cabals/c1": [.json(.ok, cabalBody)],
            "/v1/cabals/c2": [.json(.ok, cabalBody)],
            "/v1/assets/AAPLx": [.json(.ok, assetBody)],
        ])
    }

    private static func page(_ proposals: [String], next: String? = nil) -> String {
        #"{"proposals":[\#(proposals.joined(separator: ","))],"next_cursor":\#(next.map { "\"\($0)\"" } ?? "null")}"#
    }

    private static func proposal(_ id: String, cabal: String, proposer: String = "u") -> String {
        #"{"id":"\#(id)","cabal_id":"\#(cabal)","cabal_name":"Cabal \#(cabal)","proposer_id":"\#(proposer)","kind":"buy","symbol":"AAPLx","usdc_micros":1,"token_amount":null,"quote_out_amount":1,"thesis":null,"status":"executed","status_reason":null,"status_message":null,"expires_at":"2026-01-01T00:00:00Z","created_at":"2025-01-01T00:00:00Z","tally":{"yes":1,"no":0,"voters":1,"needed":1},"my_ballot":"yes","can_vote":false}"#
    }

    private static let cabalBody =
        #"{"id":"c","name":"Cabal","picture_url":null,"status":"active","rules":{"join_mode":"request","voter_mode":"all","threshold":"majority","proposal_expiry_seconds":86400,"slippage_bps":100,"show_sides":false,"show_sides_since":null,"max_trade_bps":null,"who_can_propose":"voters"},"creator":{"user_id":"u","handle":"jordan","display_name":"Jordan","photo_url":null},"created_at":"2026-09-30T12:00:00Z","open_proposal_count":0,"voter_count":1,"member_count":1,"members":[{"user_id":"u","handle":"jordan","display_name":"Jordan","photo_url":null,"role":"creator","can_vote":true,"joined_at":"2026-01-01T00:00:00Z"}],"me":{"role":"creator","can_vote":true,"can_propose":true},"my_access_request":null,"invite_code":null,"treasury_address":"treasury"}"#

    private static let assetBody =
        #"{"id":"01890a5d-ac96-774b-bcce-b302099a8070","symbol":"AAPLx","display_name":"Apple","issuer":"xstocks","kind":"equity","logo_url":null,"price_micros":null,"price_as_of":null,"change_bps":null,"sparkline_micros":null,"quotable":true,"session":{"state":"open","continuous":false,"holiday":"","early_close":false,"next_state":null,"next_transition":null},"decimals":8,"ui_multiplier":{"num":1,"den":1},"tradable":true,"other_listings":[],"attribution":"test","price_poll_interval_seconds":120}"#
}
