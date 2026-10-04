import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class AccountActivityRowTests: XCTestCase {
    private static let now = "2026-10-04T12:00:00Z"
    private static let sundayInvestors = Components.Schemas.UserTxn.CabalPayload(
        id: "cabal-1", name: "Sunday Investors")

    func testEachKindGetsItsTitleAndGlyph() throws {
        let expected: [(Components.Schemas.UserTxn.KindPayload, String, String)] = [
            (.deposit, "Deposit", "arrow.down.to.line"),
            (.withdrawal, "Withdrawal", "arrow.up.right"),
            (.fund, "Funded Sunday Investors", "plus"),
            (.cashOut, "Cashed out of Sunday Investors", "arrow.down.left"),
        ]
        for (kind, title, glyph) in expected {
            let row = try makeRow(kind: kind, cabal: Self.sundayInvestors)
            XCTAssertEqual(row.title, title, "\(kind)")
            XCTAssertEqual(row.glyph, glyph, "\(kind)")
        }
    }

    func testAFundOrCashOutWithoutACabalSaysACabal() throws {
        XCTAssertEqual(try makeRow(kind: .fund, cabal: nil).title, "Funded a cabal")
        XCTAssertEqual(try makeRow(kind: .cashOut, cabal: nil).title, "Cashed out of a cabal")
    }

    func testTheReceiptGetsTheCabalOnlyWhenThereIsOne() throws {
        let withCabal = try makeRow(kind: .fund, cabal: Self.sundayInvestors)
        XCTAssertEqual(withCabal.cabal?.id, "cabal-1")
        XCTAssertEqual(withCabal.cabal?.name, "Sunday Investors")
        XCTAssertNil(try makeRow(kind: .deposit, cabal: nil).cabal)
    }

    func testOnlyAPendingOrFailedRowNamesItsStatusInTheList() throws {
        let pending = try makeRow(status: .pending)
        let settled = try makeRow(status: .settled)
        let failed = try makeRow(status: .failed)

        XCTAssertEqual([pending.status, settled.status, failed.status], [.pending, .settled, .failed])
        XCTAssertEqual(pending.status.rowLabel, "Pending")
        XCTAssertNil(settled.status.rowLabel)
        XCTAssertEqual(failed.status.rowLabel, "Failed")
    }

    func testTheReceiptCallsASettledRowDone() throws {
        XCTAssertEqual(try makeRow(status: .pending).status.receiptLabel, "Pending")
        XCTAssertEqual(try makeRow(status: .settled).status.receiptLabel, "Done")
        XCTAssertEqual(try makeRow(status: .failed).status.receiptLabel, "Failed")
    }

    func testAmountsCarryTheirSignAndATypographicMinus() throws {
        XCTAssertEqual(try makeRow(micros: "25000000").amount, "+$25.00")
        XCTAssertEqual(try makeRow(micros: "-500000000").amount, "\u{2212}$500.00")
        XCTAssertEqual(try makeRow(micros: "-1234560000").amount, "\u{2212}$1,234.56")
        XCTAssertEqual(try makeRow(micros: "0").amount, "$0.00")
    }

    func testAnAmountThatIsNotAnIntegerFailsTheRowInsteadOfShowingZero() {
        for micros in ["", "abc", "1.5", "99999999999999999999"] {
            XCTAssertThrowsError(try makeRow(micros: micros), micros) { error in
                XCTAssertEqual(error as? APIError, .decoding("usdc_micros"))
            }
        }
    }

    func testTheDateShowsTheTimeThisYearAndTheYearOtherwise() throws {
        XCTAssertEqual(try makeRow(at: "2026-10-03T15:00:00Z").date, "Oct 3, 3:00 PM")
        XCTAssertEqual(try makeRow(at: "2026-01-01T00:05:00Z").date, "Jan 1, 12:05 AM")
        XCTAssertEqual(try makeRow(at: "2025-10-03T15:00:00Z").date, "Oct 3, 2025")
    }

    func testTheDateIsInTheDeviceTimeZone() throws {
        let eastern = try Self.zone(hours: -4)
        XCTAssertEqual(try makeRow(at: "2026-10-03T15:00:00Z", timeZone: eastern).date, "Oct 3, 11:00 AM")
    }

    func testTheYearIsJudgedInTheDeviceTimeZone() throws {
        let central = try Self.zone(hours: -5)
        XCTAssertEqual(try makeRow(at: "2026-01-01T02:00:00Z", timeZone: central).date, "Dec 31, 2025")
    }

    func testTheFullDateAlwaysHasTheYearAndTheTime() throws {
        XCTAssertEqual(try makeRow(at: "2026-10-03T15:00:00Z").fullDate, "Oct 3, 2026 at 3:00 PM")
        XCTAssertEqual(try makeRow(at: "2025-10-03T15:00:00Z").fullDate, "Oct 3, 2025 at 3:00 PM")
    }

    func testSolscanLinksTheTransactionOnlyOnceItWasSent() throws {
        XCTAssertEqual(try makeRow(signature: "abc123").solscanURL, URL(string: "https://solscan.io/tx/abc123"))
        XCTAssertNil(try makeRow(signature: nil).solscanURL)
    }

    func testTheSamplePagesMapToTheRowsTheQAScriptExpects() throws {
        let zone = try Self.zone(hours: 0)
        let now = try Self.date(Self.now)
        let first = try Components.Schemas.UserTxnPage.sampleFirst.items.map {
            try AccountActivityRow($0, now: now, timeZone: zone)
        }
        let second = try Components.Schemas.UserTxnPage.sampleSecond.items.map {
            try AccountActivityRow($0, now: now, timeZone: zone)
        }

        XCTAssertEqual(first.prefix(3).map(\.title), ["Deposit", "Funded QA pot", "Withdrawal"])
        XCTAssertEqual(first.prefix(3).map(\.amount), ["+$25.00", "\u{2212}$10.00", "\u{2212}$5.00"])
        XCTAssertEqual(first[2].status, .pending)
        XCTAssertTrue(second.contains { $0.date.hasSuffix(", 2025") })
        XCTAssertEqual(Set((first + second).map(\.id)).count, first.count + second.count)
        XCTAssertNotNil(Components.Schemas.UserTxnPage.sampleFirst.nextCursor)
        XCTAssertNil(Components.Schemas.UserTxnPage.sampleSecond.nextCursor)
        XCTAssertTrue(Components.Schemas.UserTxnPage.sampleEmpty.items.isEmpty)
    }

    private func makeRow(
        kind: Components.Schemas.UserTxn.KindPayload = .deposit,
        status: Components.Schemas.UserTxn.StatusPayload = .settled,
        micros: String = "25000000",
        cabal: Components.Schemas.UserTxn.CabalPayload? = nil,
        signature: String? = "signature-1",
        at created: String = "2026-10-03T15:00:00Z",
        timeZone: TimeZone? = nil
    ) throws -> AccountActivityRow {
        let txn = Components.Schemas.UserTxn(
            id: "txn-1", kind: kind, status: status, usdcMicros: micros, cabal: cabal, txSignature: signature,
            createdAt: try Self.date(created))
        return try AccountActivityRow(
            txn, now: try Self.date(Self.now), timeZone: try timeZone ?? Self.zone(hours: 0))
    }

    private static func date(_ iso: String) throws -> Date {
        try XCTUnwrap(SharedFormatters.iso8601Date(from: iso))
    }

    private static func zone(hours: Int) throws -> TimeZone {
        try XCTUnwrap(TimeZone(secondsFromGMT: hours * 3600))
    }
}
