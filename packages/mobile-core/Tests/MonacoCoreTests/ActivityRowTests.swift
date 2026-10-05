import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class ActivityRowTests: XCTestCase {
    private typealias Activity = Components.Schemas.CabalActivity

    private static let apple = Activity.AssetPayload(symbol: "AAPLx", name: "Apple")

    func testEachKindGetsItsTitleAndGlyph() throws {
        let expected: [(Activity.KindPayload, String, String)] = [
            (.buy, "Bought Apple", "arrow.down"),
            (.sell, "Sold Apple", "arrow.up"),
            (.fund, "Money added", "plus"),
            (.cashOut, "Cashed out", "arrow.down.left"),
        ]
        for (kind, title, glyph) in expected {
            let row = try makeRow(kind: kind, asset: Self.apple)
            XCTAssertEqual(row.title, title, "\(kind)")
            XCTAssertEqual(row.glyph, glyph, "\(kind)")
        }
    }

    func testATradeWithoutAKnownAssetSaysOnlyWhatHappened() throws {
        XCTAssertEqual(try makeRow(kind: .buy, asset: nil).title, "Bought")
        XCTAssertEqual(try makeRow(kind: .sell, asset: nil).title, "Sold")
        XCTAssertNil(try makeRow(kind: .buy, asset: nil).assetLine)
    }

    func testTheRowLabelsOnlyPendingAndFailedAndTheReceiptCallsConfirmedDone() throws {
        let rows = try [Activity.StatusPayload.pending, .confirmed, .failed].map { try makeRow(status: $0) }

        XCTAssertEqual(rows.map(\.status), [.pending, .confirmed, .failed])
        XCTAssertEqual(rows.map(\.status.rowLabel), ["Pending", nil, "Failed"])
        XCTAssertEqual(rows.map(\.status.receiptLabel), ["Pending", "Done", "Failed"])
    }

    func testTheAmountIsUnsignedAndMissingUntilKnown() throws {
        XCTAssertEqual(try makeRow(micros: 25_000_000).amount, "$25.00")
        XCTAssertEqual(try makeRow(micros: 1_234_560_000).amount, "$1,234.56")
        XCTAssertNil(try makeRow(micros: nil).amount)
    }

    func testTheAssetLineShowsTheTickerWithoutTheTrailingX() throws {
        XCTAssertEqual(try makeRow(asset: Self.apple).assetLine, "Apple · AAPL")
        XCTAssertEqual(
            try makeRow(asset: .init(symbol: "BRK.Bx", name: "Berkshire Hathaway")).assetLine,
            "Berkshire Hathaway · BRK.B")
    }

    func testTheActorIsTheDisplayNameOrTheHandleWhenTheNameIsEmpty() throws {
        let named = try makeRow(actor: .init(userId: "user-1", handle: "ana", displayName: "Ana"))
        let unnamed = try makeRow(actor: .init(userId: "user-2", handle: "bo", displayName: ""))
        let voted = try makeRow(actor: nil)

        XCTAssertEqual(named.actorName, "Ana")
        XCTAssertEqual(named.actorID, "user-1")
        XCTAssertEqual(unnamed.actorName, "@bo")
        XCTAssertNil(voted.actorName)
        XCTAssertNil(voted.actorID)
    }

    func testTheAgeShowsTheTimeThisYearAndTheYearOtherwise() throws {
        XCTAssertEqual(try makeRow(at: "2026-10-03T15:00:00Z").age, "Oct 3, 3:00 PM")
        XCTAssertEqual(try makeRow(at: "2025-10-03T15:00:00Z").age, "Oct 3, 2025")
        XCTAssertEqual(try makeRow(at: "2025-10-03T15:00:00Z").fullDate, "Oct 3, 2025 at 3:00 PM")
    }

    func testSolscanLinksTheTransactionOnlyOnceItWasSent() throws {
        XCTAssertEqual(try makeRow(signature: "abc123").solscanURL, URL(string: "https://solscan.io/tx/abc123"))
        XCTAssertNil(try makeRow(signature: nil).solscanURL)
    }

    func testOnlyAFailedBuyOrSellOffersRetry() throws {
        for kind in Activity.KindPayload.allCases {
            for status in Activity.StatusPayload.allCases {
                let row = try makeRow(kind: kind, status: status)
                XCTAssertEqual(
                    row.offersRetry, status == .failed && (kind == .buy || kind == .sell), "\(kind) \(status)")
            }
        }
    }

    func testTheSampleRowsCoverEveryKindAndStatus() {
        let samples = Activity.samples
        XCTAssertEqual(Set(samples.map(\.kind)), Set(Activity.KindPayload.allCases))
        XCTAssertEqual(Set(samples.map(\.status)), Set(Activity.StatusPayload.allCases))
        XCTAssertEqual(Set(samples.map(\.id)).count, samples.count)
    }

    private func makeRow(
        kind: Activity.KindPayload = .buy,
        status: Activity.StatusPayload = .confirmed,
        asset: Activity.AssetPayload? = ActivityRowTests.apple,
        micros: Int64? = 25_000_000,
        actor: Activity.ActorPayload? = nil,
        signature: String? = "signature-1",
        at occurred: String = "2026-10-03T15:00:00Z"
    ) throws -> ActivityRow {
        let activity = Activity(
            id: "activity-1", kind: kind, status: status, asset: asset, usdcMicros: micros, units: nil, actor: actor,
            txSignature: signature, occurredAt: try Self.date(occurred))
        return ActivityRow(
            activity, now: try Self.date("2026-10-04T12:00:00Z"),
            timeZone: try XCTUnwrap(TimeZone(secondsFromGMT: 0)))
    }

    private static func date(_ iso: String) throws -> Date {
        try XCTUnwrap(SharedFormatters.iso8601Date(from: iso))
    }
}
