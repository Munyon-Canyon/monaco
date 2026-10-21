import MonacoAPI
import MonacoCore
import SwiftUI
import Testing
import UIKit

@testable import Monaco

@MainActor
struct AccountActivityViewTests {
    private static let phone = CGSize(width: 390, height: CGFloat.greatestFiniteMagnitude)
    private static let longCabal = Components.Schemas.UserTxn.CabalPayload(
        id: "cabal-long", name: "The Sunday Investors Club of Greater Metropolitan Lisbon")

    private static func row(
        kind: Components.Schemas.UserTxn.KindPayload, status: Components.Schemas.UserTxn.StatusPayload,
        micros: String, cabal: Components.Schemas.UserTxn.CabalPayload? = nil
    ) throws -> AccountActivityRow {
        let created = Date(timeIntervalSince1970: 1_790_996_400)
        let txn = Components.Schemas.UserTxn(
            id: "\(kind)-\(status)", kind: kind, status: status, usdcMicros: micros, cabal: cabal,
            txSignature: nil, createdAt: created)
        return try AccountActivityRow(
            txn, now: created, timeZone: TimeZone(identifier: "UTC") ?? .current,
            locale: Locale(identifier: "en_US"))
    }

    private static func fitted(_ row: AccountActivityRow, at size: DynamicTypeSize) -> CGSize {
        let view = AccountActivityRowView(row: row, isLast: true)
            .environment(\.dynamicTypeSize, size)
        return UIHostingController(rootView: view).sizeThatFits(in: phone)
    }

    @Test func aPendingWithdrawalKeepsItsAmountAndNamesItsStatus() throws {
        let pending = try Self.row(kind: .withdrawal, status: .pending, micros: "-100000000")
        let visible = AccountActivityRowView.visibleText(for: pending)
        #expect(visible == ["Withdrawal", pending.subtitle, "−$100.00", "Pending"])
        #expect(pending.amount.hasPrefix("−"))
    }

    @Test func aFailedCashOutKeepsItsAmountDirectionAndDestination() throws {
        let failed = try Self.row(kind: .cashOut, status: .failed, micros: "250000000", cabal: Self.longCabal)
        let visible = AccountActivityRowView.visibleText(for: failed)
        #expect(visible.contains("+$250.00"))
        #expect(visible.contains("Failed"))
        #expect(visible.contains("Cash-out not completed"))
        #expect(visible.contains { $0.hasPrefix("To cash") })
    }

    @Test func aSettledDepositShowsNoStatus() throws {
        let settled = try Self.row(kind: .deposit, status: .settled, micros: "100000000")
        #expect(AccountActivityRowView.visibleText(for: settled) == ["Deposit received", settled.subtitle, "+$100.00"])
    }

    @Test func twoPendingTransfersCanBeToldApartWithoutOpeningThem() throws {
        let first = try Self.row(kind: .withdrawal, status: .pending, micros: "-100000000")
        let second = try Self.row(kind: .withdrawal, status: .pending, micros: "-2500000000")
        #expect(AccountActivityRowView.spokenLabel(for: first) != AccountActivityRowView.spokenLabel(for: second))
    }

    @Test(arguments: [AccountActivityRow.Status.pending, .settled, .failed])
    func theSpokenLabelIsExactlyWhatTheRowShows(status: AccountActivityRow.Status) throws {
        let payload: Components.Schemas.UserTxn.StatusPayload =
            switch status {
            case .pending: .pending
            case .settled: .settled
            case .failed: .failed
            }
        let row = try Self.row(kind: .fund, status: payload, micros: "-250000000", cabal: Self.longCabal)
        let spoken = AccountActivityRowView.spokenLabel(for: row)
        #expect(spoken == AccountActivityRowView.visibleText(for: row).joined(separator: ", "))
        #expect(spoken.contains(row.amount))
        if let label = status.rowLabel { #expect(spoken.hasSuffix(label)) }
    }

    @Test(arguments: [DynamicTypeSize.large, .xxxLarge, .accessibility3, .accessibility5])
    func aPendingRowWithALongDestinationKeepsItsWidthAtEveryTextSize(size: DynamicTypeSize) throws {
        let row = try Self.row(kind: .fund, status: .pending, micros: "-250000000", cabal: Self.longCabal)
        let fitted = Self.fitted(row, at: size)
        #expect(fitted.width <= Self.phone.width + 1, "row is \(fitted.width)pt wide at \(size)")
        #expect(fitted.height >= MonacoRowLayout.minHeight)
    }

    @Test func theStackedRowIsTallerThanTheInlineRowBecauseTheFiguresDropBelowTheLabels() throws {
        let row = try Self.row(kind: .fund, status: .failed, micros: "-250000000", cabal: Self.longCabal)
        #expect(Self.fitted(row, at: .accessibility5).height > Self.fitted(row, at: .large).height)
    }

    @Test func aPendingRowIsTallerThanASettledRowBecauseTheStatusSitsBeneathTheAmount() throws {
        let pending = try Self.row(kind: .deposit, status: .pending, micros: "100000000")
        let settled = try Self.row(kind: .deposit, status: .settled, micros: "100000000")
        #expect(Self.fitted(pending, at: .accessibility5).height > Self.fitted(settled, at: .accessibility5).height)
    }
}
