import Foundation
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class AccountActivityModelTests: XCTestCase {
    private typealias Page = Components.Schemas.UserTxnPage

    func testLoadReadsTheFirstPageOnce() async throws {
        let (model, transport, _) = try make([.page(.sampleFirst)])

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.rows.map(\.id), Page.sampleFirst.items.map(\.id))
        XCTAssertFalse(model.isLoadingMore)
        let sent = await transport.sent
        XCTAssertEqual(sent.map(\.method), [.get])
        XCTAssertEqual(sent.map(\.path), ["/v1/me/txns?limit=30"])
    }

    func testLoadMoreReadsTheNextPageWithTheCursorFromTheFirst() async throws {
        let (model, transport, _) = try make([.page(.sampleFirst), .page(.sampleSecond)])
        await model.load()

        await model.loadMore()

        XCTAssertEqual(model.rows.map(\.id), (Page.sampleFirst.items + Page.sampleSecond.items).map(\.id))
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/me/txns?limit=30", "/v1/me/txns?limit=30&cursor=\(Page.sampleCursor)"])
    }

    func testThereIsNoThirdCallAfterTheLastPage() async throws {
        let (model, transport, _) = try make([.page(.sampleFirst), .page(.sampleSecond)])
        await model.load()
        await model.loadMore()

        await model.loadMore()

        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
        XCTAssertNil(model.toast)
    }

    func testABalanceChangedHintReloadsTheFirstPageAndAPingHintDoesNot() async throws {
        let newest = Components.Schemas.UserTxn(
            id: "txn-new", kind: .deposit, status: .settled, usdcMicros: "7000000", cabal: nil,
            txSignature: "signature-new", createdAt: Page.sampleFirst.items[0].createdAt.addingTimeInterval(3600))
        let refreshed = Page(items: [newest] + Page.sampleFirst.items, nextCursor: Page.sampleFirst.nextCursor)
        let (model, transport, hints) = try make([.page(.sampleFirst), .page(refreshed)])
        await model.load()
        let observer = await observing(model, hints)
        addTeardownBlock { observer.cancel() }

        await hints.send(.changed(.user("me"), what: "ping_echoed", id: "1"))

        for _ in 0..<50 { await Task.yield() }
        let afterPing = await transport.sent.count
        XCTAssertEqual(afterPing, 1)
        XCTAssertEqual(model.rows.count, Page.sampleFirst.items.count)

        await hints.send(.changed(.user("me"), what: "balance_changed", id: "2"))

        let reloaded = await waitUntil { model.rows.first?.id == "txn-new" }
        XCTAssertTrue(reloaded)
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
        XCTAssertEqual(model.rows.count, Page.sampleFirst.items.count + 1)
    }

    func testAResyncReloadsTheFirstPage() async throws {
        let (model, transport, hints) = try make([.page(.sampleFirst), .page(.sampleFirst)])
        await model.load()
        let observer = await observing(model, hints)
        addTeardownBlock { observer.cancel() }

        await hints.send(.resync)

        let reloaded = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(reloaded)
    }

    func testAHiddenScreenReloadsOnceWhenItBecomesVisible() async throws {
        let (model, transport, hints) = try make([.page(.sampleFirst), .page(.sampleFirst)])
        await model.load()
        let observer = await observing(model, hints)
        addTeardownBlock { observer.cancel() }

        model.setVisible(false)
        await hints.send(.changed(.user("me"), what: "balance_changed", id: "1"))
        for _ in 0..<50 { await Task.yield() }
        let whileHidden = await transport.sent.count
        XCTAssertEqual(whileHidden, 1)

        model.setVisible(true)
        let reloaded = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(reloaded)
    }

    func testAFailedReloadKeepsTheRowsAndToasts() async throws {
        let (model, _, _) = try make([.page(.sampleFirst), .failure(.notConnectedToInternet)])
        await model.load()

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.rows.map(\.id), Page.sampleFirst.items.map(\.id))
        XCTAssertEqual(model.toast, "You're offline. Try again.")
        model.dismissToast()
        XCTAssertNil(model.toast)
    }

    func testAFailedNextPageKeepsTheRowsAndToasts() async throws {
        let (model, _, _) = try make([.page(.sampleFirst), .failure(.networkConnectionLost)])
        await model.load()

        await model.loadMore()

        XCTAssertEqual(model.rows.map(\.id), Page.sampleFirst.items.map(\.id))
        XCTAssertFalse(model.isLoadingMore)
        XCTAssertEqual(model.toast, "You're offline. Try again.")
    }

    func testLoadingAgainWithRowsShownKeepsTheLoadedPages() async throws {
        let (model, transport, _) = try make([.page(.sampleFirst), .page(.sampleSecond), .page(.sampleFirst)])
        await model.load()
        await model.loadMore()

        await model.load()

        XCTAssertEqual(model.rows.map(\.id), (Page.sampleFirst.items + Page.sampleSecond.items).map(\.id))
        let count = await transport.sent.count
        XCTAssertEqual(count, 3)
    }

    func testLeavingTheScreenDuringAReloadShowsNoToast() async throws {
        let (model, transport, _) = try make([.page(.sampleFirst), .hang])
        await model.load()

        let reload = Task { await model.load() }
        let started = await waitUntil { await transport.sent.count == 2 }
        XCTAssertTrue(started)
        reload.cancel()
        await reload.value

        XCTAssertNil(model.toast)
        XCTAssertEqual(model.phase, .loaded)
    }

    func testAFailedFirstLoadShowsTheFailureWithoutAToastAndTheNextLoadRecovers() async throws {
        let (model, _, _) = try make([.failure(.notConnectedToInternet), .page(.sampleFirst)])

        await model.load()

        guard case .failed(.transport) = model.phase else {
            return XCTFail("want a transport failure, got \(model.phase)")
        }
        XCTAssertNil(model.toast)

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
    }

    func testAnEmptyPageIsEmpty() async throws {
        let (model, _, _) = try make([.page(.sampleEmpty)])

        await model.load()

        XCTAssertEqual(model.phase, .empty)
        XCTAssertTrue(model.rows.isEmpty)
    }

    func testAnUnknownKindFailsThePageInsteadOfGuessing() async throws {
        let unknown =
            #"{"items":[{"id":"txn-1","kind":"refund","status":"settled","usdc_micros":"1","cabal":null,"#
            + #""tx_signature":null,"created_at":"2026-10-03T15:00:00Z"}],"next_cursor":null}"#
        let (model, _, _) = try make([.json(unknown)])

        await model.load()

        guard case .failed(.decoding) = model.phase else {
            return XCTFail("want a decoding failure, got \(model.phase)")
        }
        XCTAssertTrue(model.rows.isEmpty)
    }

    func testTheYearRuleReadsTheInjectedClock() async throws {
        let (sameYear, _, _) = try make([.page(.sampleFirst)], now: Self.date("2026-10-04T12:00:00Z"))
        let (nextYear, _, _) = try make([.page(.sampleFirst)], now: Self.date("2027-01-15T12:00:00Z"))

        await sameYear.load()
        await nextYear.load()

        XCTAssertFalse(try XCTUnwrap(sameYear.rows.first).date.hasSuffix(", 2026"))
        XCTAssertTrue(try XCTUnwrap(nextYear.rows.first).date.hasSuffix(", 2026"))
    }

    func testThePreviewServesTheSamplePagesByCursor() async throws {
        let now = Self.now
        let model = AccountActivityModel.preview(empty: false, clock: { now })

        await model.load()
        await model.loadMore()

        XCTAssertEqual(model.rows.map(\.id), (Page.sampleFirst.items + Page.sampleSecond.items).map(\.id))
        XCTAssertNil(model.toast)
    }

    func testThePreviewCanBeEmpty() async throws {
        let now = Self.now
        let model = AccountActivityModel.preview(empty: true, clock: { now })

        await model.load()

        XCTAssertEqual(model.phase, .empty)
    }

    private static let now = Date(timeIntervalSince1970: 1_791_100_800)

    private static func date(_ iso: String) throws -> Date {
        try XCTUnwrap(SharedFormatters.iso8601Date(from: iso))
    }

    private enum Script {
        case page(Page)
        case json(String)
        case failure(URLError.Code)
        case hang

        func reply() throws -> StubTransport.Reply {
            switch self {
            case .page(let page):
                let encoder = JSONEncoder()
                encoder.dateEncodingStrategy = .iso8601
                return .json(.ok, String(decoding: try encoder.encode(page), as: UTF8.self))
            case .json(let body):
                return .json(.ok, body)
            case .failure(let code):
                return .failure(URLError(code))
            case .hang:
                return .hang
            }
        }
    }

    private func make(
        _ script: [Script], now: Date = AccountActivityModelTests.now
    ) throws -> (AccountActivityModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: try script.map { try $0.reply() })
        let hints = FakeHintStream()
        let model = AccountActivityModel(
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints, clock: { now })
        return (model, transport, hints)
    }

    private func observing(_ model: AccountActivityModel, _ hints: FakeHintStream) async -> Task<Void, Never> {
        let observer = Task { await model.observe() }
        let subscribed = await waitUntil { await hints.subscriberCount == 1 }
        XCTAssertTrue(subscribed)
        return observer
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }
}
