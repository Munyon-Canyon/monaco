import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import Synchronization
import XCTest

@MainActor
final class AssetBestChainStoreTests: XCTestCase {
    func testOpeningAStockFetchesOnceAndKeepsTheAnswerUnderItsSymbol() async throws {
        let clock = BestChainClock(Date())
        let fetcher = BestChainFetcher(now: clock.now)
        let store = AssetBestChainStore(now: clock.read, fetch: fetcher.fetch)

        await store.refreshIfNeeded(symbol: "NVDAx")

        let symbols = await fetcher.symbols
        XCTAssertEqual(symbols, ["NVDAx"])
        let stored = try XCTUnwrap(store.bestChain(for: "NVDAx"))
        XCTAssertEqual(stored.network, .base)
        XCTAssertEqual(stored.quotedAt, clock.now)
        XCTAssertEqual(stored.headline, "Best price on Base")
        XCTAssertNil(store.bestChain(for: "AAPLx"))
    }

    func testReappearingUnderAMinuteKeepsTheAnswerAndAfterAMinuteRefreshesIt() async {
        let clock = BestChainClock(Date())
        let fetcher = BestChainFetcher(now: clock.now)
        let store = AssetBestChainStore(now: clock.read, fetch: fetcher.fetch)
        await store.refreshIfNeeded(symbol: "NVDAx")

        clock.advance(59)
        await store.refreshIfNeeded(symbol: "NVDAx")
        var count = await fetcher.symbols.count
        XCTAssertEqual(count, 1)
        XCTAssertNotNil(store.bestChain(for: "NVDAx"))

        clock.advance(1)
        XCTAssertNil(store.bestChain(for: "NVDAx"), "a minute-old answer is never used")
        await fetcher.quote(at: clock.now)
        await store.refreshIfNeeded(symbol: "NVDAx")
        count = await fetcher.symbols.count
        XCTAssertEqual(count, 2)
        XCTAssertEqual(store.bestChain(for: "NVDAx")?.quotedAt, clock.now)
    }

    func testAnAnswerTheServerQuotedLongAgoIsNotUsed() async {
        let clock = BestChainClock(Date())
        let fetcher = BestChainFetcher(now: clock.now.addingTimeInterval(-61))
        let store = AssetBestChainStore(now: clock.read, fetch: fetcher.fetch)

        await store.refreshIfNeeded(symbol: "NVDAx")

        XCTAssertNil(store.bestChain(for: "NVDAx"))
    }

    func testAFailedRefreshDropsAnOldAnswerSoTheAmountScreenFallsBack() async {
        let clock = BestChainClock(Date())
        let fetcher = BestChainFetcher(now: clock.now)
        let store = AssetBestChainStore(now: clock.read, fetch: fetcher.fetch)
        await store.refreshIfNeeded(symbol: "NVDAx")
        await fetcher.fail()

        clock.advance(61)
        await store.refreshIfNeeded(symbol: "NVDAx")

        XCTAssertNil(store.bestChain(for: "NVDAx"))
    }

    func testResetForgetsEveryAnswer() async {
        let clock = BestChainClock(Date())
        let store = AssetBestChainStore(now: clock.read, fetch: BestChainFetcher(now: clock.now).fetch)
        await store.refreshIfNeeded(symbol: "NVDAx")

        store.reset()

        XCTAssertNil(store.bestChain(for: "NVDAx"))
    }

    func testTheLiveStoreReadsTheBestChainEndpoint() async throws {
        let body = try String(
            decoding: JSONEncoder.monacoTest.encode(Components.Schemas.AssetBestChain.fixture(now: Date())),
            as: UTF8.self)
        let transport = StubTransport(.json(.ok, body))
        let store = AssetBestChainStore(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            now: { Date() })

        await store.refreshIfNeeded(symbol: "NVDAx")

        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/assets/NVDAx/best-chain"])
        XCTAssertEqual(store.bestChain(for: "NVDAx")?.netShareNanos, 4_012_000)
    }

    func testTheEstimateScalesEveryChainFromTheOneDollarProbe() async throws {
        let bestChain = try XCTUnwrap(AssetBestChain(.fixture(now: Date())))

        XCTAssertEqual(bestChain.shareNanos(forMicros: 250_000_000), 1_003_000_000)
        let scaled = try XCTUnwrap(bestChain.bestPrice(forMicros: 250_000_000))
        XCTAssertEqual(scaled.venues.map(\.netShareNanos), [1_003_000_000, 1_000_000_000])
        XCTAssertEqual(scaled.venues.map(\.netMicros), [250_000_000, 250_000_000])
        XCTAssertEqual(scaled.winner.network, .base)
    }
}

private final class BestChainClock: Sendable {
    private let current: Mutex<Date>

    init(_ start: Date) { current = Mutex(start) }

    var now: Date { current.withLock { $0 } }

    var read: @Sendable () -> Date { { [self] in now } }

    func advance(_ seconds: TimeInterval) { current.withLock { $0 = $0.addingTimeInterval(seconds) } }
}

actor BestChainFetcher {
    struct Failure: Error {}
    private(set) var symbols: [String] = []
    private var quotedAt: Date
    private var failing = false

    init(now: Date) { quotedAt = now }

    func quote(at date: Date) { quotedAt = date }
    func fail() { failing = true }

    nonisolated var fetch: @Sendable (String) async throws -> AssetBestChain {
        { [self] symbol in try await answer(symbol) }
    }

    private func answer(_ symbol: String) throws -> AssetBestChain {
        symbols.append(symbol)
        if failing { throw Failure() }
        guard let bestChain = AssetBestChain(.fixture(symbol: symbol, now: quotedAt)) else { throw Failure() }
        return bestChain
    }
}

extension JSONEncoder {
    fileprivate static var monacoTest: JSONEncoder {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        return encoder
    }
}

extension Components.Schemas.AssetBestChain {
    static func fixture(symbol: String = "NVDAx", now: Date) -> Self {
        func quote(_ id: String, network: Components.Schemas.Network, shares: String) -> Components.Schemas.RouteQuote {
            .init(
                listingId: id, issuer: "xstocks", network: network, venue: network == .solana ? .jupiter : .uniswap,
                netShareNanos: shares, netMicros: "1000000", costs: [], quotedAt: now,
                expiresAt: now.addingTimeInterval(30))
        }
        let base = "01928a6e-0000-7000-8000-000000000002"
        return .init(
            symbol: symbol, amountMicros: "1000000", network: .base, venue: .uniswap, listingId: base,
            netShareNanos: "4012000", quotedAt: now,
            priceComparison: .init(
                winnerListingId: base,
                quotes: [
                    quote(base, network: .base, shares: "4012000"),
                    quote("01928a6e-0000-7000-8000-000000000001", network: .solana, shares: "4000000"),
                ],
                unavailable: []))
    }
}
