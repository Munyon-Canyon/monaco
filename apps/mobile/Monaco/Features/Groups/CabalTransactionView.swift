import MonacoCore
import SwiftUI

struct CabalTransactionView: View {
    let cabalID: String
    let transactionID: String

    private enum Resolution: Equatable {
        case loading
        case found(ActivityRow)
        case missing
    }

    @Environment(AppEnvironment.self) private var environment
    @State private var resolution = Resolution.loading

    var body: some View {
        ScrollView {
            content
                .padding(.top, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .navigationTitle("Transaction")
        .navigationBarTitleDisplayMode(.inline)
        .task { await resolve() }
    }

    @ViewBuilder private var content: some View {
        switch resolution {
        case .loading:
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    SkeletonBlock(width: 180, height: 20)
                    SkeletonBlock(width: 140, height: 36)
                }
                .padding(.horizontal, MonacoTheme.Space.m)
                BoardRowSkeleton(rows: 4)
            }
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Loading this transaction")
            .accessibilityIdentifier("cabal-txn-loading")
        case .missing:
            EmptyState(title: "Couldn't load this transaction.", actionTitle: "Try again") {
                Task { await resolve() }
            }
            .accessibilityIdentifier("cabal-txn-error")
        case .found(let row):
            receipt(row)
        }
    }

    private func receipt(_ row: ActivityRow) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    SunkenGlyphMark(systemImage: row.glyph)
                    Text(row.title)
                        .font(MonacoTheme.Typo.rowTitle)
                        .foregroundStyle(MonacoTheme.ink)
                }
                if let amount = row.amount {
                    Text(amount)
                        .moneyFont(.large)
                        .foregroundStyle(MonacoTheme.ink)
                        .lineLimit(1)
                        .minimumScaleFactor(MoneyStyle.large.minimumScaleFactor)
                        .accessibilityIdentifier("cabal-txn-amount")
                }
            }
            .padding(.horizontal, MonacoTheme.Space.m)
            MonacoGroupedList {
                ReceiptLine(label: "Status", value: .words(row.status.receiptLabel))
                    .accessibilityIdentifier("cabal-txn-status")
                if let assetLine = row.assetLine {
                    ReceiptLine(label: "Asset", value: .words(assetLine))
                        .accessibilityIdentifier("cabal-txn-asset")
                }
                if let name = row.actorName, let actorID = row.actorID {
                    NavigationLink(
                        value: AnyAppRoute(
                            UserProfileRoute(
                                userID: actorID,
                                preview: UserPreview(displayName: name, handle: row.actorHandle, photoURL: nil)
                            ))
                    ) {
                        ReceiptLine(label: "By", value: .words(name))
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("cabal-txn-actor")
                }
                ReceiptLine(label: "When", value: .data(row.fullDate), isLast: row.solscanURL == nil)
                    .accessibilityIdentifier("cabal-txn-when")
                if let url = row.solscanURL {
                    solscanRow(url)
                }
            }
        }
    }

    private func solscanRow(_ url: URL) -> some View {
        Link(destination: url) {
            HStack(spacing: MonacoTheme.Space.sm) {
                Text("View on Solscan")
                    .font(MonacoTheme.Typo.bodyStrong)
                    .foregroundStyle(MonacoTheme.ink)
                Spacer(minLength: MonacoTheme.Space.sm)
                Image(systemName: "arrow.up.right")
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(MonacoTheme.tertiaryText)
                    .accessibilityHidden(true)
            }
            .padding(.horizontal, MonacoTheme.Space.m)
            .frame(minHeight: 52)
            .contentShape(Rectangle())
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("cabal-txn-solscan")
    }

    private func resolve() async {
        resolution = .loading
        let model = CabalActivityModel(
            cabalID: cabalID, api: environment.api, hints: environment.hints, clock: Date.init)
        let row = await model.find(id: transactionID)
        guard !Task.isCancelled else { return }
        resolution = row.map(Resolution.found) ?? .missing
    }
}
