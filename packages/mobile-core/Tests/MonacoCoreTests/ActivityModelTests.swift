import Foundation
import HTTPTypes
import MonacoAPI
import MonacoCore
import MonacoTestSupport
import XCTest

@MainActor
final class ActivityModelTests: XCTestCase {
    private typealias Activity = Components.Schemas.CabalActivity
    private typealias Page = Components.Schemas.CabalActivityPage

    private static let cabalID = "cabal-1"
    private static let base = "/v1/cabals/cabal-1/activity"

    func testLoadShowsTheNewestFiveAndNoSeeAllWhenThatIsEverything() async throws {
        let (model, transport, _) = try make([.page(Self.page(1...5, next: nil))])

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.firstFive.count, 5)
        XCTAssertFalse(model.hasMore)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["\(Self.base)?limit=30"])
    }

    func testMoreThanFiveRowsOrANextCursorMeansThereIsMore() async throws {
        let (six, _, _) = try make([.page(Self.page(1...6, next: nil))])
        let (cursor, _, _) = try make([.page(Self.page(1...3, next: "next"))])

        await six.load()
        await cursor.load()

        XCTAssertEqual(six.firstFive.map(\.id), Self.page(1...5, next: nil).items.map(\.id))
        XCTAssertEqual(six.rows.count, 6)
        XCTAssertTrue(six.hasMore)
        XCTAssertTrue(cursor.hasMore)
    }

    func testLoadMoreReadsTheNextPage() async throws {
        let (model, transport, _) = try make([.page(Self.page(1...3, next: "c2")), .page(Self.page(4...5, next: nil))])
        await model.load()

        await model.loadMore()

        XCTAssertEqual(model.rows.count, 5)
        XCTAssertFalse(model.hasMore)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["\(Self.base)?limit=30", "\(Self.base)?limit=30&cursor=c2"])
    }

    func testANonMemberIsHiddenAndNeverSeesTheError() async throws {
        let (model, _, _) = try make([.notMember])

        await model.load()

        XCTAssertEqual(model.phase, .hidden)
        XCTAssertEqual(model.failureTick, 0)
        XCTAssertNil(model.lastError)
    }

    func testAnEmptyCabalIsEmpty() async throws {
        let (model, _, _) = try make([.page(.sampleEmpty)])

        await model.load()

        XCTAssertEqual(model.phase, .empty)
        XCTAssertFalse(model.hasMore)
    }

    func testAFailedFirstLoadIsTheErrorStateWithoutAToast() async throws {
        let (model, _, _) = try make([.failure(.notConnectedToInternet), .page(.sampleFirst)])

        await model.load()

        guard case .failed(.transport) = model.phase else {
            return XCTFail("want a transport failure, got \(model.phase)")
        }
        XCTAssertEqual(model.failureTick, 0)

        await model.load()

        XCTAssertEqual(model.phase, .loaded)
    }

    func testAFailedReloadKeepsTheRowsAndTicksTheToast() async throws {
        let (model, _, _) = try make([.page(.sampleFirst), .failure(.networkConnectionLost)])
        await model.load()

        await model.refresh()

        XCTAssertEqual(model.phase, .loaded)
        XCTAssertEqual(model.rows.count, Page.sampleFirst.items.count)
        XCTAssertEqual(model.failureTick, 1)
        XCTAssertEqual(model.lastError.map(ToastCopy.message(for:)), "You're offline. Try again.")
    }

    func testFindReturnsALoadedRowWithoutAsking() async throws {
        let (model, transport, _) = try make([.page(.sampleFirst)])
        await model.load()

        let row = await model.find(id: Activity.sampleFund.id)

        XCTAssertEqual(row?.title, "Money added")
        let count = await transport.sent.count
        XCTAssertEqual(count, 1)
    }

    func testFindPagesOnUntilItFindsTheRow() async throws {
        let (model, transport, _) = try make([
            .page(Self.page(1...2, next: "c2")), .page(Self.page(3...4, next: "c3")),
        ])

        let row = await model.find(id: Self.page(4...4, next: nil).items[0].id)

        XCTAssertNotNil(row)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["\(Self.base)?limit=100", "\(Self.base)?limit=100&cursor=c2"])
    }

    func testFindReturnsNilWhenTheCursorEnds() async throws {
        let (model, transport, _) = try make([.page(Self.page(1...2, next: "c2")), .page(Self.page(3...4, next: nil))])

        let row = await model.find(id: "missing")

        XCTAssertNil(row)
        let count = await transport.sent.count
        XCTAssertEqual(count, 2)
    }

    func testFindReturnsNilWhenTheReadFails() async throws {
        let (model, _, _) = try make([.failure(.notConnectedToInternet)])

        let row = await model.find(id: "missing")

        XCTAssertNil(row)
    }

    func testAnActivityHintReloadsTheFirstPageAndAnotherCabalsDoesNot() async throws {
        let (model, transport, hints) = try make([
            .page(Self.page(2...3, next: nil)), .page(Self.page(1...3, next: nil)),
        ])
        await model.load()
        let observer = Task { await model.observe() }
        addTeardownBlock { observer.cancel() }
        let subscribed = await waitUntil { await hints.subscriberCount == 2 }
        XCTAssertTrue(subscribed)

        await hints.send(.changed(.cabal("cabal-2"), what: "activity_changed", id: "1"))
        for _ in 0..<50 { await Task.yield() }
        let afterOther = await transport.sent.count
        XCTAssertEqual(afterOther, 1)

        await hints.send(.changed(.cabal(Self.cabalID), what: "swap_updated", id: "2"))

        let reloaded = await waitUntil { model.rows.count == 3 }
        XCTAssertTrue(reloaded)
    }

    func testRetryMarksTheRowPendingAndSaysSo() async throws {
        let failed = Self.buy(1, .failed)
        let (model, transport, _) = try make([.page(Page(items: [failed], nextCursor: nil)), .accepted(failed.id)])
        await model.load()

        await model.retrySwap(try XCTUnwrap(model.rows.first))

        XCTAssertEqual(model.rows.first?.status, .pending)
        XCTAssertEqual(model.rows.first?.offersRetry, false)
        XCTAssertEqual(model.toast?.message, "Retrying the trade")
        let sent = await transport.sent
        XCTAssertEqual(sent.last?.path, "/v1/swaps/\(failed.id)/retry")
        XCTAssertEqual(sent.last?.method, .post)
        let keyName = try XCTUnwrap(HTTPField.Name(IdempotentSubmission.keyHeader))
        XCTAssertNotNil(sent.last?.headerFields[keyName])
    }

    func testARefusedRetryShowsTheServersMessageAndKeepsTheRowFailed() async throws {
        let failed = Self.buy(1, .failed)
        let (model, _, _) = try make([.page(Page(items: [failed], nextCursor: nil)), .notRetryable])
        await model.load()

        await model.retrySwap(try XCTUnwrap(model.rows.first))

        XCTAssertEqual(model.rows.first?.status, .failed)
        XCTAssertEqual(model.rows.first?.offersRetry, true)
        XCTAssertEqual(model.toast?.message, "This trade can't be retried.")
        XCTAssertEqual(model.toast?.isSuccess, false)
    }

    func testTheRetriedTradeConfirmingSaysBoughtAndDropsRetryFromTheOldRow() async throws {
        let failed = Self.buy(2, .failed)
        let (model, _, _) = try make([
            .page(Page(items: [failed], nextCursor: nil)), .accepted(failed.id),
            .page(Page(items: [Self.buy(1, .pending), failed], nextCursor: nil)),
            .page(Page(items: [Self.buy(1, .confirmed), failed], nextCursor: nil)),
        ])
        await model.load()
        await model.retrySwap(try XCTUnwrap(model.rows.first))

        await model.refresh()
        XCTAssertEqual(model.toast?.message, "Retrying the trade")
        XCTAssertEqual(model.rows.map(\.status), [.pending, .failed])

        await model.refresh()
        XCTAssertEqual(model.toast?.message, "Bought. Holdings updated")
        XCTAssertEqual(model.rows.map(\.status), [.confirmed, .failed])
        XCTAssertEqual(model.rows.last?.offersRetry, false)
    }

    func testTheRetriedTradeFailingAgainSaysTryLater() async throws {
        let failed = Self.buy(2, .failed)
        let (model, _, _) = try make([
            .page(Page(items: [failed], nextCursor: nil)), .accepted(failed.id),
            .page(Page(items: [Self.buy(1, .failed), failed], nextCursor: nil)),
        ])
        await model.load()
        await model.retrySwap(try XCTUnwrap(model.rows.first))

        await model.refresh()

        XCTAssertEqual(model.toast?.message, "It didn't go through again. Try later")
        XCTAssertEqual(model.rows.map(\.offersRetry), [true, false])
    }

    func testTheSwapReceiptOffersRetryOnlyWhenTheServerSaysRetryable() async throws {
        let (model, transport, _) = try make([.swap(retryable: true), .swap(retryable: false)])

        await model.loadSwap(id: "swap-1")
        XCTAssertEqual(model.openSwap?.retryable, true)
        XCTAssertEqual(model.openSwap?.failureMessage, "The trade did not go through.")
        XCTAssertEqual(model.openSwap?.assetLine, "Apple · AAPL")

        await model.loadSwap(id: "swap-1")
        XCTAssertEqual(model.openSwap?.retryable, false)
        let paths = await transport.sent.map(\.path)
        XCTAssertEqual(paths, ["/v1/swaps/swap-1", "/v1/swaps/swap-1"])
    }

    private static func buy(_ number: Int, _ status: Activity.StatusPayload) -> Activity {
        Activity.sample(
            number, kind: .buy, status: status, asset: .init(symbol: "AAPLx", name: "Apple"), micros: 25_000_000,
            actor: nil)
    }

    private enum Script {
        case page(Page)
        case notMember
        case notRetryable
        case accepted(String)
        case swap(retryable: Bool)
        case failure(URLError.Code)

        func reply() throws -> StubTransport.Reply {
            switch self {
            case .page(let page):
                let encoder = JSONEncoder()
                encoder.dateEncodingStrategy = .iso8601
                return .json(.ok, String(decoding: try encoder.encode(page), as: UTF8.self))
            case .notMember:
                let body =
                    #"{"type":"about:blank","title":"Error","status":403,"code":"not_cabal_member","#
                    + #""message":"You are not a member of this cabal.","#
                    + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
                return .response(status: .forbidden, contentType: "application/problem+json", body: Data(body.utf8))
            case .notRetryable:
                let body =
                    #"{"type":"about:blank","title":"Error","status":409,"code":"swap_not_retryable","#
                    + #""message":"This trade can't be retried.","#
                    + #""trace_id":"00000000000000000000000000000000","retryable":false}"#
                return .response(status: .conflict, contentType: "application/problem+json", body: Data(body.utf8))
            case .accepted(let id):
                return .json(.accepted, #"{"swap_id":"\#(id)","status":"retry_requested"}"#)
            case .swap(let retryable):
                return .json(
                    .ok,
                    #"{"id":"swap-1","cabal_id":"cabal-1","source":{"kind":"proposal","id":"p-1"},"action":"buy","#
                        + #""symbol":"AAPLx","asset_name":"Apple","token_decimals":8,"usdc_micros":25000000,"#
                        + #""token_amount":null,"status":"failed","failure_code":"jupiter_failed","#
                        + #""failure_message":"The trade did not go through.","tx_signature":null,"#
                        + #""created_at":"2026-10-03T15:00:00Z","confirmed_at":null,"retryable":\#(retryable)}"#)
            case .failure(let code):
                return .failure(URLError(code))
            }
        }
    }

    private static func page(_ numbers: ClosedRange<Int>, next: String?) -> Page {
        Page(
            items: numbers.map {
                Activity.sample($0, kind: .fund, status: .confirmed, asset: nil, micros: 1_000_000, actor: nil)
            }, nextCursor: next)
    }

    private func make(_ script: [Script]) throws -> (CabalActivityModel, StubTransport, FakeHintStream) {
        let transport = StubTransport(scripted: try script.map { try $0.reply() })
        let hints = FakeHintStream()
        let now = Date(timeIntervalSince1970: 1_791_100_800)
        let model = CabalActivityModel(
            cabalID: Self.cabalID,
            api: APIClient(serverURL: testServerURL, tokens: StubTokenProvider(token: "token-1"), transport: transport),
            hints: hints, clock: { now })
        return (model, transport, hints)
    }

    private func waitUntil(_ predicate: @escaping () async -> Bool) async -> Bool {
        for _ in 0..<500 {
            if await predicate() { return true }
            await Task.yield()
        }
        return await predicate()
    }
}
