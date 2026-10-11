import Foundation
import MonacoAPI
import MonacoTestClock
import MonacoTestSupport
import XCTest

@testable import MonacoCore

@MainActor
final class BestPriceLiveTests: XCTestCase {
    private let quotedAt = Date()

    func testAFetchReplacesTheStoredComparison() async {
        let fetches = Fetches([.success(live(winnerShares: "40500000"))])
        let model = LiveBestPriceModel(sleep: { try await TestClock().sleep(for: $0) }, fetch: fetches.next)

        XCTAssertEqual(model.row(for: openBuy, decimals: 8, now: quotedAt)?.caption, "Price when proposed")
        model.poll(cabalID: "cabal-1")
        await waitUntil { model.latest != nil }

        let row = model.row(for: openBuy, decimals: 8, now: quotedAt)
        XCTAssertEqual(row?.caption, "Updated just now")
        XCTAssertEqual(row?.headline, "Best price across 1 venue")
        XCTAssertEqual(row?.venues?.winner.netShareNanos, 40_500_000)
        let cabals = await fetches.cabals
        XCTAssertEqual(cabals, ["cabal-1"])
    }

    func testTheTimerRefreshesEvery15Seconds() async {
        let clock = TestClock()
        let fetches = Fetches([.success(live(winnerShares: "40500000")), .success(live(winnerShares: "40700000"))])
        let model = LiveBestPriceModel(sleep: { try await clock.sleep(for: $0) }, fetch: fetches.next)

        model.poll(cabalID: "cabal-1")
        _ = await clock.state.until { $0.pending == 1 }
        XCTAssertEqual(clock.state.current.requested.last, .seconds(15))
        XCTAssertEqual(model.latest?.comparison?.quotes.first?.netShareNanos, "40500000")

        clock.advance(by: .seconds(15))
        await waitUntil { model.latest?.comparison?.quotes.first?.netShareNanos == "40700000" }
        let count = await fetches.count
        XCTAssertEqual(count, 2)
    }

    func testHidingTheScreenStopsTheTimer() async {
        let clock = TestClock()
        let fetches = Fetches([.success(live(winnerShares: "40500000"))])
        let model = LiveBestPriceModel(sleep: { try await clock.sleep(for: $0) }, fetch: fetches.next)
        model.poll(cabalID: "cabal-1")
        _ = await clock.state.until { $0.pending == 1 }

        model.poll(cabalID: nil)
        _ = await clock.state.until { $0.pending == 0 }
        clock.advance(by: .seconds(60))
        for _ in 0..<100 { await Task.yield() }

        XCTAssertFalse(model.isPolling)
        let count = await fetches.count
        XCTAssertEqual(count, 1)
    }

    func testAFailedFetchKeepsTheLastComparisonWithItsAge() async {
        let clock = TestClock()
        let fetches = Fetches([.success(live(winnerShares: "40500000")), .failure(URLError(.timedOut))])
        let model = LiveBestPriceModel(sleep: { try await clock.sleep(for: $0) }, fetch: fetches.next)
        model.poll(cabalID: "cabal-1")
        _ = await clock.state.until { $0.pending == 1 }

        clock.advance(by: .seconds(15))
        _ = await clock.state.until { $0.pending == 1 && $0.requested.count == 2 }

        let count = await fetches.count
        XCTAssertEqual(count, 2)
        let row = model.row(for: openBuy, decimals: 8, now: quotedAt.addingTimeInterval(20))
        XCTAssertEqual(row?.caption, "Updated 20s ago")
        XCTAssertEqual(row?.venues?.winner.netShareNanos, 40_500_000)
    }

    func testAComparisonOlderThan60SecondsFallsBackToTheStoredOneAsWhenProposed() async throws {
        let fetches = Fetches([.success(live(winnerShares: "40500000"))])
        let model = LiveBestPriceModel(sleep: { try await TestClock().sleep(for: $0) }, fetch: fetches.next)
        model.poll(cabalID: "cabal-1")
        await waitUntil { model.latest != nil }

        XCTAssertEqual(
            model.row(for: openBuy, decimals: 8, now: quotedAt.addingTimeInterval(60))?.caption, "Updated 60s ago")
        let stale = model.row(for: openBuy, decimals: 8, now: quotedAt.addingTimeInterval(61))
        XCTAssertEqual(stale, BestPriceRow(try XCTUnwrap(openBuy.bestPrice), caption: "Price when proposed"))
        XCTAssertEqual(stale?.headline, "Best price across 3 venues")
    }

    func testWithNoRouteTheSolanaQuoteShowsWithNoVenues() async {
        let noRoute = LivePriceComparison(.init(quoteOutAmount: 10_000_000, quotedAt: quotedAt))
        let model = LiveBestPriceModel(
            sleep: { try await TestClock().sleep(for: $0) }, fetch: Fetches([.success(noRoute)]).next)
        model.poll(cabalID: "cabal-1")
        await waitUntil { model.latest != nil }

        let row = model.row(for: openBuy, decimals: 8, now: quotedAt.addingTimeInterval(3))
        XCTAssertEqual(row?.figure, "$10.00 a share")
        XCTAssertEqual(row?.headline, "Solana")
        XCTAssertEqual(row?.caption, "Updated just now")
        XCTAssertNil(row?.venues)
    }

    func testPullToRefreshFetchesWithoutWaitingForTheTimer() async {
        let clock = TestClock()
        let fetches = Fetches([.success(live(winnerShares: "40500000")), .success(live(winnerShares: "40900000"))])
        let model = LiveBestPriceModel(sleep: { try await clock.sleep(for: $0) }, fetch: fetches.next)
        model.poll(cabalID: "cabal-1")
        _ = await clock.state.until { $0.pending == 1 }

        await model.refresh()

        XCTAssertEqual(model.latest?.comparison?.quotes.first?.netShareNanos, "40900000")
    }

    func testAnOlderResponseLandingAfterANewerOneKeepsTheNewer() async {
        let held = HeldFetch()
        let older = live(winnerShares: "40500000", at: quotedAt.addingTimeInterval(-15))
        let newer = live(winnerShares: "40900000")
        let model = LiveBestPriceModel(
            sleep: { try await TestClock().sleep(for: $0) }, fetch: { _ in try await held.fetch() })
        model.poll(cabalID: "cabal-1")
        await held.waitForCalls(1)

        let pull = Task { await model.refresh() }
        await held.waitForCalls(2)
        await held.resolve(call: 1, with: newer)
        await pull.value
        await held.resolve(call: 0, with: older)
        for _ in 0..<100 { await Task.yield() }

        XCTAssertEqual(model.latest, newer)
    }

    func testClosedAndSellProposalsShowTheStoredComparisonAndNeverPoll() async throws {
        for detail in [Self.detail(status: .executed, kind: .buy), Self.detail(status: .open, kind: .sell)] {
            let transport = StubTransport(routes: ["/v1/proposals/proposal-1": [.json(.ok, try Self.json(detail))]])
            let model = detailModel(transport)
            model.setVisible(true)
            await model.load()

            let summary = try XCTUnwrap(model.summary)
            XCTAssertFalse(model.livePrice.isPolling)
            XCTAssertEqual(
                model.livePrice.row(for: summary, decimals: 8, now: quotedAt),
                BestPriceRow(try XCTUnwrap(summary.bestPrice)))
            let paths = await transport.sent.compactMap(\.path)
            XCTAssertFalse(paths.contains { $0.hasSuffix("/price-comparison") })
        }
    }

    func testAnOpenBuyDetailPollsTheLiveComparisonOnlyWhileVisible() async throws {
        let detail = Self.detail(status: .open, kind: .buy)
        let path = "/v1/cabals/\(detail.cabalId)/proposals/proposal-1/price-comparison"
        let comparison = Components.Schemas.ProposalPriceComparison(
            priceComparison: BestPrice.sampleComparison(now: quotedAt), quoteOutAmount: 14_250_000,
            quotedAt: quotedAt)
        let transport = StubTransport(routes: [
            "/v1/proposals/proposal-1": [.json(.ok, try Self.json(detail))],
            path: [.json(.ok, try Self.json(comparison))],
        ])
        let model = detailModel(transport)
        await model.load()
        XCTAssertFalse(model.livePrice.isPolling)

        model.setVisible(true)
        await transport.waitForRequest(path: path)
        await waitUntil { model.livePrice.latest != nil }
        XCTAssertEqual(model.livePrice.latest?.quoteOutAmount, 14_250_000)

        model.setVisible(false)
        XCTAssertFalse(model.livePrice.isPolling)
    }

    private var openBuy: ProposalSummary {
        var proposal = Components.Schemas.Proposal.sample(kind: .buy)
        proposal.priceComparison = BestPrice.sampleComparison(now: quotedAt)
        return ProposalSummary(proposal)
    }

    private func live(winnerShares: String, at time: Date? = nil) -> LivePriceComparison {
        let quotedAt = time ?? quotedAt
        let quote = Components.Schemas.RouteQuote(
            listingId: "listing-sol", issuer: "xstocks", network: .solana, venue: .jupiter,
            netShareNanos: winnerShares, netMicros: "10000000", costs: [], quotedAt: quotedAt,
            expiresAt: quotedAt.addingTimeInterval(30))
        return LivePriceComparison(
            .init(
                priceComparison: .init(winnerListingId: "listing-sol", quotes: [quote], unavailable: []),
                quoteOutAmount: 40_500_000, quotedAt: quotedAt))
    }

    private func detailModel(_ transport: StubTransport) -> ProposalDetailModel {
        let api = APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token"), transport: transport)
        return ProposalDetailModel(
            id: "proposal-1", cabalID: "", repository: ProposalsRepository(api: api), hints: FakeHintStream(),
            sleep: { try await TestClock().sleep(for: $0) })
    }

    private static func detail(
        status: Components.Schemas.ProposalStatus, kind: Components.Schemas.ProposalKind
    ) -> Components.Schemas.ProposalDetail {
        var detail = Components.Schemas.ProposalDetail.sample(status: status, kind: kind)
        detail.priceComparison = BestPrice.sampleComparison(now: Date())
        return detail
    }

    private static func json(_ value: some Encodable) throws -> String {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    private func waitUntil(_ predicate: () -> Bool) async {
        for _ in 0..<500 where !predicate() { await Task.yield() }
        XCTAssertTrue(predicate())
    }
}

private actor Fetches {
    private var replies: [Result<LivePriceComparison, any Error>]
    private(set) var cabals: [String] = []
    var count: Int { cabals.count }

    init(_ replies: [Result<LivePriceComparison, any Error>]) {
        self.replies = replies
    }

    nonisolated var next: @Sendable (String) async throws -> LivePriceComparison {
        { cabalID in try await self.take(cabalID) }
    }

    private func take(_ cabalID: String) throws -> LivePriceComparison {
        cabals.append(cabalID)
        guard !replies.isEmpty else { throw URLError(.cancelled) }
        return try replies.removeFirst().get()
    }
}

private actor HeldFetch {
    private var pending: [CheckedContinuation<LivePriceComparison, any Error>?] = []
    private var waiters: [(count: Int, continuation: CheckedContinuation<Void, Never>)] = []

    func fetch() async throws -> LivePriceComparison {
        try await withCheckedThrowingContinuation { continuation in
            pending.append(continuation)
            let ready = waiters.filter { $0.count <= pending.count }
            waiters.removeAll { $0.count <= pending.count }
            for waiter in ready { waiter.continuation.resume() }
        }
    }

    func waitForCalls(_ count: Int) async {
        if pending.count >= count { return }
        await withCheckedContinuation { waiters.append((count, $0)) }
    }

    func resolve(call index: Int, with value: LivePriceComparison) {
        pending[index]?.resume(returning: value)
        pending[index] = nil
    }
}
