import MonacoCore
import SwiftUI

struct AccountTxnReceiptSheet: View {
    let row: AccountActivityRow
    let openCabal: (String) -> Void

    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
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
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Done") { dismiss() }
                        .accessibilityIdentifier("account-txn-receipt-done")
                }
            }
        }
        .presentationDetents([.medium])
    }

    private func cabalRow(_ cabal: AccountActivityRow.Cabal) -> some View {
        Button {
            openCabal(cabal.id)
        } label: {
            ReceiptLine(label: "Cabal", value: .words(cabal.name), isLast: row.solscanURL == nil)
                .contentShape(Rectangle())
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("account-txn-receipt-cabal")
    }
}
