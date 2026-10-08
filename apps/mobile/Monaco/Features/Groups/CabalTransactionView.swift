import MonacoCore
import SwiftUI

struct CabalTransactionView: View {
    let cabalID: String
    let transactionID: String

    private enum Resolution: Equatable {
        case loading
        case found(ActivityRow)
        case missing
        case forbidden
        case failed
    }

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var resolution = Resolution.loading
    @State private var model: CabalActivityModel?

    var body: some View {
        ScrollView {
            content
                .padding(.top, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .navigationTitle("Transaction")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            await resolve()
            await model?.observe()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
        .cabalActivityToasts(model, in: toasts)
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
            EmptyState(title: "This transaction doesn't exist.")
                .accessibilityIdentifier("cabal-txn-missing")
        case .forbidden:
            EmptyState(title: "Only members of this cabal can see this transaction.")
                .accessibilityIdentifier("cabal-txn-forbidden")
        case .failed:
            MonacoErrorRow(thing: "this transaction", identifier: "cabal-txn-error") {
                Task { await resolve() }
            }
        case .found(let row):
            receipt(row)
        }
    }

    private func receipt(_ row: ActivityRow) -> some View {
        let swap = row.kind.isSwap ? model?.openSwap : nil
        let solscanURL = swap?.solscanURL ?? row.solscanURL
        return VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    SunkenGlyphMark(systemImage: row.glyph)
                    Text(row.title)
                        .font(MonacoTheme.Typo.rowTitle)
                        .foregroundStyle(MonacoTheme.ink)
                }
                if let amount = swap?.amount ?? row.amount {
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
                ReceiptLine(label: "Status", value: .words(swap?.statusLabel ?? row.status.receiptLabel))
                    .accessibilityIdentifier("cabal-txn-status")
                if let reason = swap?.failureMessage {
                    ReceiptLine(label: "Why", value: .words(reason))
                        .accessibilityIdentifier("cabal-txn-failure")
                }
                if let assetLine = swap?.assetLine ?? row.assetLine {
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
                ReceiptLine(label: "When", value: .data(row.fullDate), isLast: solscanURL == nil)
                    .accessibilityIdentifier("cabal-txn-when")
                if let url = solscanURL {
                    solscanRow(url)
                }
            }
            if swap?.retryable == true {
                Button(CabalActivityCopy.retrySwap) {
                    Task {
                        await model?.retrySwap(row)
                        await model?.loadSwap(id: row.id)
                    }
                }
                .buttonStyle(.monacoPrimary)
                .padding(.horizontal, MonacoTheme.Space.m)
                .accessibilityIdentifier("cabal-txn-retry")
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
        let model =
            model
            ?? CabalActivityModel(
                cabalID: cabalID, api: environment.api, hints: environment.hints, clock: Date.init)
        self.model = model
        let lookup = await model.lookup(id: transactionID)
        if case .found(let row) = lookup, row.kind.isSwap { await model.loadSwap(id: row.id) }
        guard !Task.isCancelled else { return }
        resolution =
            switch lookup {
            case .found(let row): .found(row)
            case .notFound: .missing
            case .forbidden: .forbidden
            case .failed: .failed
            }
    }
}
