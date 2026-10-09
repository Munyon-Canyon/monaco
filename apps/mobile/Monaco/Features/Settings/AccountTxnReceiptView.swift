import MonacoCore
import SwiftUI

nonisolated struct AccountTxnReceiptRoute: AppRoute {
    let row: AccountActivityRow

    @MainActor func destination() -> some View {
        AccountTxnReceiptView(row: row)
    }
}

struct AccountTxnReceiptView: View {
    let row: AccountActivityRow

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                Text(row.amount)
                    .moneyFont(.large)
                    .foregroundStyle(MonacoTheme.ink)
                    .lineLimit(1)
                    .minimumScaleFactor(MoneyStyle.large.minimumScaleFactor)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .accessibilityIdentifier("account-txn-receipt-amount")
                MonacoGroupedList {
                    ReceiptLine(label: "Status", value: .words(row.status.receiptLabel))
                        .accessibilityIdentifier("account-txn-receipt-status")
                    ReceiptLine(
                        label: "When", value: .data(row.fullDate), isLast: row.cabal == nil && row.solscanURL == nil
                    )
                    .accessibilityIdentifier("account-txn-receipt-time")
                    if let cabal = row.cabal {
                        cabalRow(cabal)
                    }
                    if let url = row.solscanURL {
                        SolscanLinkRow(url: url, identifier: "account-txn-receipt-solscan")
                    }
                }
            }
            .padding(.top, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .navigationTitle(row.title)
        .navigationBarTitleDisplayMode(.inline)
    }

    private func cabalRow(_ cabal: AccountActivityRow.Cabal) -> some View {
        NavigationLink(value: AnyAppRoute(CabalRoute(id: cabal.id))) {
            ReceiptLine(label: "Cabal", value: .words(cabal.name), isLast: row.solscanURL == nil)
                .contentShape(Rectangle())
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("account-txn-receipt-cabal")
    }
}
