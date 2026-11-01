import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class AssetInYourCabalsModelTests: XCTestCase {
    func testTheHeldReadIsOneCallAndNeverReadsAPot() async throws {
        let transport = inYourCabalsTransport(held: [heldJSON(["c-1", "c-2"])], feed: [openPage(["p-1"])])
        let model = makeInYourCabalsModel(transport: transport, hints: FakeHintStream())

        await model.loadInYourCabals()

        let paths = await transport.sent.compactMap(\.path)
        XCTAssertEqual(paths.filter { $0.hasPrefix("/v1/me/held") }.count, 1)
        XCTAssertFalse(paths.contains { $0.contains("/pot") }, "\(paths)")
        XCTAssertEqual(model.heldRows.map(\.cabalID), ["c-1", "c-2"])
        XCTAssertEqual(model.positions.map(\.cabalID), ["c-1", "c-2"])
        XCTAssertEqual(model.positions.map(\.canVote), [true, false])
    }

    func testTheOpenFeedAsksForOpenProposalsOnTheStockAndPagesToTheEnd() async throws {
        let transport = inYourCabalsTransport(
            held: [heldJSON([])],
            feed: [openPage(["p-1"], next: "C1")], later: [openPage(["p-2"], next: "C2"), openPage(["p-3"])])
        let model = makeInYourCabalsModel(transport: transport, hints: FakeHintStream())

        await model.loadInYourCabals()

        let feeds = await transport.sent.compactMap(\.path).filter { $0.hasPrefix("/v1/feed") }
        XCTAssertEqual(feeds.count, 3, "\(feeds)")
        for path in feeds {
            let query = URLComponents(string: path)?.queryItems ?? []
            XCTAssertEqual(query.first { $0.name == "status" }?.value, "open")
            XCTAssertEqual(query.first { $0.name == "kind" }?.value, "proposal")
            XCTAssertEqual(query.first { $0.name == "symbol" }?.value, "NVDAx")
        }
        XCTAssertEqual(model.openProposals.map(\.proposalID), ["p-1", "p-2", "p-3"])
    }

    func testAnOpenProposalSurvivesFiveNewerClosedPostsOnTheSameStock() async throws {
        let transport = inYourCabalsTransport(held: [heldJSON([])], feed: [openPage(["older-open"])])
        let model = makeInYourCabalsModel(transport: transport, hints: FakeHintStream())

        await model.loadInYourCabals()

        XCTAssertEqual(model.openProposals.map(\.proposalID), ["older-open"])
    }

    func testAFeedHintReloadsTheOpenProposalsAndTheHeldRead() async throws {
        let transport = inYourCabalsTransport(
            held: [heldJSON(["c-1"]), heldJSON(["c-1"])], feed: [openPage(["p-1"]), openPage(["p-1", "p-2"])])
        let hints = FakeHintStream()
        let model = makeInYourCabalsModel(transport: transport, hints: hints)
        await model.loadInYourCabals()
        let observing = Task { await model.observe() }
        await hints.waitForSubscribers(3)

        await hints.send(.changed(.global, what: "feed", id: "1"))
        let reloaded = await waitUntil {
            let held = await transport.sent.compactMap(\.path).filter { $0.hasPrefix("/v1/me/held") }.count
            return model.openProposals.count == 2 && held == 2
        }

        observing.cancel()
        XCTAssertTrue(reloaded)
    }

    func testAnActivityHintForAHeldCabalReloadsTheHeldRead() async throws {
        let transport = inYourCabalsTransport(
            held: [heldJSON(["c-1"]), heldJSON(["c-1", "c-2"])], feed: [openPage([])])
        let hints = FakeHintStream()
        let model = makeInYourCabalsModel(transport: transport, hints: hints)
        await model.loadInYourCabals()
        let observing = Task { await model.observe() }
        await hints.waitForSubscribers(3)

        await hints.send(.changed(.cabal("c-1"), what: "activity_changed", id: "1"))
        let reloaded = await waitUntil { model.heldRows.count == 2 }

        observing.cancel()
        XCTAssertTrue(reloaded)
    }

    private func makeInYourCabalsModel(transport: StubTransport, hints: FakeHintStream) -> AssetDetailClientModel {
        let api = APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport)
        return AssetDetailClientModel(api: api, symbol: "NVDAx", hints: hints)
    }

    private func inYourCabalsTransport(held: [String], feed: [String], later: [String] = []) -> StubTransport {
        let base = "/v1/feed?kind=proposal&symbol=NVDAx&scope=mine&status=open"
        var routes: [String: [StubTransport.Reply]] = [
            "/v1/me/held?symbol=NVDAx": held.map { .json(.ok, $0) },
            "/v1/me/cabals": Array(repeating: .json(.ok, cabalsJSON), count: max(held.count, 1)),
            "\(base)&limit=20": feed.map { .json(.ok, $0) },
        ]
        for (index, page) in later.enumerated() {
            routes["\(base)&cursor=C\(index + 1)&limit=20"] = [.json(.ok, page)]
        }
        return StubTransport(routes: routes)
    }

    private var cabalsJSON: String {
        let cabal = { (id: String, vote: Bool) in
            #"{"id":"\#(id)","name":"Cabal \#(id)","picture_url":null,"status":"active","role":"member","can_vote":\#(vote),"#
                + #""member_count":2,"joined_at":"2026-09-30T12:00:00Z","pending_request_count":0,"unread_count":0}"#
        }
        return "[\(cabal("c-1", true)),\(cabal("c-2", false))]"
    }

    private func heldJSON(_ ids: [String]) -> String {
        let rows = ids.enumerated().map { index, id in
            #"{"cabal":{"id":"\#(id)","name":"Cabal \#(id)","picture_url":null},"logo_url":null,"holding":{"#
                + #""symbol":"NVDAx","display_name":"NVIDIA","kind":"equity","units":"38.0000","#
                + #""token_amount":38000000,"price_micros":220000000,"value_micros":\#(8_000_000_000 - index),"#
                + #""weight_bps":4200,"cost_basis_micros":7333333333,"pnl_micros":1026666667,"#
                + #""avg_entry_micros":"1","first_bought_at":null}}"#
        }
        return #"{"total_value_micros":0,"cabal_count":\#(ids.count),"prices_as_of":"2026-10-03T15:00:00Z","#
            + #""holdings":[\#(rows.joined(separator: ","))]}"#
    }

    private func openPage(_ ids: [String], next: String? = nil) -> String {
        let items = ids.map { id -> Components.Schemas.FeedItem in
            var item = Components.Schemas.FeedItem.samples[0]
            item.id = "item-\(id)"
            item.proposalId = id
            item.expiresAt = Date().addingTimeInterval(4 * 3600 + 30)
            return item
        }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let page = Components.Schemas.FeedPage(items: items, nextCursor: next)
        return String(decoding: (try? encoder.encode(page)) ?? Data(), as: UTF8.self)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<2000 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }
}
