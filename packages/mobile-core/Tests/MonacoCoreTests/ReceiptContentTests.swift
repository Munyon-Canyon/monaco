import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class ReceiptContentTests: XCTestCase {
    private static let cabal = Components.Schemas.UserTxn.CabalPayload(id: "cabal-1", name: "Big Ballers")

    func testEachKindNamesItsFromAndToEnds() throws {
        let expected: [(Components.Schemas.UserTxn.KindPayload, String, String)] = [
            (.deposit, "External wallet", "Your cash"),
            (.withdrawal, "Your cash", "External wallet"),
            (.fund, "Your cash", "Big Ballers"),
            (.cashOut, "Big Ballers", "Your cash"),
        ]
        for (kind, from, to) in expected {
            let content = ReceiptContent(try row(kind: kind))
            XCTAssertEqual(value(content, .from), from, "\(kind)")
            XCTAssertEqual(value(content, .to), to, "\(kind)")
            XCTAssertEqual(value(content, .asset), "USDC", "\(kind)")
        }
    }

    func testKeyRowsRunInTheDesignOrderAndEndWithTheRecordID() throws {
        let content = ReceiptContent(try row(kind: .deposit))

        XCTAssertEqual(
            content.keyRows.map(\.field), [.from, .to, .asset, .requestedAmount, .when, .status, .recordID])
        XCTAssertEqual(value(content, .recordID), "txn-1")
        XCTAssertEqual(value(content, .requestedAmount), "+$25.00")
    }

    func testSettledPendingAndFailedMapToTheirBadgeToneNoticeAndStatusRow() throws {
        let expected:
            [(Components.Schemas.UserTxn.StatusPayload, ReceiptContent.Status, String, ReceiptContent.Tone, String)] = [
                (.settled, .completed, "Completed", .brand, "Done"),
                (.pending, .pending, "Processing", .warn, "Pending"),
                (.failed, .failed, "Not completed", .negative, "Not completed"),
            ]
        for (payload, status, badge, tone, rowValue) in expected {
            let content = ReceiptContent(try row(kind: .deposit, status: payload))
            XCTAssertEqual(content.status, status)
            XCTAssertEqual(content.badgeText, badge)
            XCTAssertEqual(content.badgeTone, tone)
            XCTAssertEqual(value(content, .status), rowValue)
        }
    }

    func testOnlyAnUnfinishedReceiptCarriesANotice() throws {
        XCTAssertNil(ReceiptContent(try row(kind: .deposit, status: .settled)).notice)
        XCTAssertEqual(
            ReceiptContent(try row(kind: .deposit, status: .pending)).notice?.message,
            "The transfer is processing. The funds are not confirmed at the destination.")
        let failed = ReceiptContent(try row(kind: .deposit, status: .failed)).notice
        XCTAssertEqual(failed?.tone, .negative)
        XCTAssertEqual(
            failed?.message,
            "This operation did not complete. Check the latest record before submitting another request.")
    }

    func testTradeAndUnknownStatusesUseTheirOwnCopy() {
        XCTAssertEqual(ReceiptContent.Status.pending.badgeText(isPersonal: false), "Execution pending")
        XCTAssertEqual(
            ReceiptContent.Status.pending.notice(isPersonal: false)?.message,
            "The vote passed. Execution has not been confirmed.")
        XCTAssertEqual(ReceiptContent.Status.unknown.badgeText(isPersonal: true), "Status unconfirmed")
        XCTAssertEqual(ReceiptContent.Status.unknown.tone, .warn)
        XCTAssertEqual(ReceiptContent.Status.unknown.rowValue, "Unknown")
        XCTAssertEqual(
            ReceiptContent.Status.unknown.notice(isPersonal: true)?.message,
            "We couldn’t confirm the latest status. This is not proof of failure.")
    }

    func testOnlyAFundOrCashOutReceiptOpensItsCabal() throws {
        XCTAssertEqual(ReceiptContent(try row(kind: .fund)).cabal?.id, "cabal-1")
        XCTAssertEqual(ReceiptContent(try row(kind: .cashOut)).cabal?.name, "Big Ballers")
        XCTAssertNil(ReceiptContent(try row(kind: .deposit)).cabal)
        XCTAssertNil(ReceiptContent(try row(kind: .withdrawal)).cabal)
    }

    func testTheReceiptTitleFollowsTheStatusForEveryKind() throws {
        let statuses: [(Components.Schemas.UserTxn.StatusPayload, ReceiptContent.Status)] = [
            (.settled, .completed), (.pending, .pending), (.failed, .failed),
        ]
        let titles: [Components.Schemas.UserTxn.KindPayload: [ReceiptContent.Status: String]] = [
            .deposit: [
                .completed: "Deposit received", .pending: "Deposit pending", .failed: "Deposit not completed",
                .unknown: "Checking deposit status",
            ],
            .cashOut: [
                .completed: "Cashed out of Big Ballers", .pending: "Cash-out pending",
                .failed: "Cash-out not completed", .unknown: "Checking cash-out status",
            ],
        ]
        for (kind, byStatus) in titles {
            for (payload, status) in statuses {
                XCTAssertEqual(ReceiptContent(try row(kind: kind, status: payload)).eyebrow, byStatus[status])
            }
            XCTAssertEqual(ReceiptContent(try row(kind: kind), status: .unknown).eyebrow, byStatus[.unknown])
        }
    }

    func testAFailedOrUnknownReceiptNeverReadsReceivedOrCashedOut() throws {
        let kinds: [Components.Schemas.UserTxn.KindPayload] = [.deposit, .cashOut]
        for kind in kinds {
            let failed = ReceiptContent(try row(kind: kind, status: .failed))
            let unknown = ReceiptContent(try row(kind: kind), status: .unknown)
            for content in [failed, unknown] {
                XCTAssertFalse(content.eyebrow.contains("received"), content.eyebrow)
                XCTAssertFalse(content.eyebrow.hasPrefix("Cashed out"), content.eyebrow)
                XCTAssertEqual(content.amount, "+$25.00")
            }
            XCTAssertEqual(unknown.badgeText, "Status unconfirmed")
            XCTAssertEqual(failed.badgeText, "Not completed")
        }
    }

    func testAnUnsettledAmountIsLabelledExpectedOrRequested() throws {
        XCTAssertNil(ReceiptContent(try row(kind: .deposit, status: .settled)).amountCaption)
        XCTAssertEqual(ReceiptContent(try row(kind: .deposit, status: .pending)).amountCaption, "Expected amount")
        XCTAssertEqual(ReceiptContent(try row(kind: .deposit, status: .failed)).amountCaption, "Requested amount")
        XCTAssertEqual(ReceiptContent(try row(kind: .deposit), status: .unknown).amountCaption, "Requested amount")
        XCTAssertNotNil(ReceiptContent(try row(kind: .deposit), status: .unknown).notice)
        XCTAssertEqual(
            value(ReceiptContent(try row(kind: .deposit, status: .failed)), .requestedAmount), "+$25.00")
    }

    func testTheExplorerNeedsASignature() throws {
        XCTAssertTrue(ReceiptContent(try row(kind: .deposit, signature: "abc")).canOpenExplorer)
        XCTAssertFalse(ReceiptContent(try row(kind: .deposit, signature: nil)).canOpenExplorer)
    }

    func testAFundReceiptCarriesItsEyebrowAmountAndPersonalMark() throws {
        let content = ReceiptContent(try row(kind: .fund, micros: "-250000000"))

        XCTAssertEqual(content.eyebrow, "Funding Big Ballers")
        XCTAssertEqual(content.amount, "\u{2212}$250.00")
        XCTAssertEqual(content.mark, .glyph("person.3"))
        XCTAssertEqual(content.related, .accountActivity)
        XCTAssertTrue(content.isPersonal)
    }

    private func value(_ content: ReceiptContent, _ field: ReceiptContent.Field) -> String? {
        content.keyRows.first { $0.field == field }?.value
    }

    private func row(
        kind: Components.Schemas.UserTxn.KindPayload,
        status: Components.Schemas.UserTxn.StatusPayload = .settled,
        micros: String = "25000000",
        signature: String? = "signature-1"
    ) throws -> AccountActivityRow {
        let created = try XCTUnwrap(SharedFormatters.iso8601Date(from: "2026-10-03T15:00:00Z"))
        let txn = Components.Schemas.UserTxn(
            id: "txn-1", kind: kind, status: status, usdcMicros: micros, cabal: Self.cabal, txSignature: signature,
            createdAt: created)
        return try AccountActivityRow(txn, now: created, locale: Locale(identifier: "en_US_POSIX"))
    }
}
