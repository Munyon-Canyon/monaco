import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalHoldingsSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalPotModelHost(cabalID: context.cabalID, showsLogos: true) { model in
            CabalHoldingsLive(model: model, cabalID: context.cabalID)
        }
    }
}

private struct CabalHoldingsLive: View {
    let model: CabalPotModel?
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        CabalHoldingsSection(model: model, cabalID: cabalID) { route in
            environment.navigator.open(route, in: environment.navigator.selectedTab)
        }
    }
}

struct CabalHoldingsSection: View {
    let model: CabalPotModel?
    let cabalID: String
    let open: (any AppRoute) -> Void

    @Environment(ToastCenter.self) private var toasts
    @Environment(\.cabalModel) private var cabalModel

    private var canPropose: Bool { cabalModel?.cabal?.me?.canVote == true }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Holdings")
                .padding(.horizontal, MonacoTheme.Space.gutter)
            content
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .onChange(of: model?.toast) { _, message in
            guard let model, let message else { return }
            toasts.current = MonacoToast(message: message)
            model.dismissToast()
        }
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 3, markShape: .tile, hasTrailing: true)
                .accessibilityElement()
                .accessibilityLabel("Loading holdings")
                .accessibilityIdentifier("cabal-holdings-loading")
        case .failed:
            MonacoErrorRow(thing: "holdings", identifier: "cabal-holdings-failed") {
                Task { await model?.load() }
            }
        case .loaded(let summary):
            loaded(summary)
        }
    }

    @ViewBuilder
    private func loaded(_ summary: CabalPotSummary) -> some View {
        switch summary.state {
        case .zero:
            note(Self.zeroPotNote(canPropose: canPropose), id: "cabal-holdings-empty")
        case .cashOnly:
            MonacoGroupedList { cashRow(summary.cash, yield: summary.cashYield) }
            note(Self.cashOnlyNote(canPropose: canPropose), id: "cabal-holdings-nothing-bought")
        case .invested:
            AllocationBar(slices: Self.slices(summary.legend))
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.bottom, MonacoTheme.Space.xs)
                .accessibilityIdentifier("cabal-holdings-mix")
            MonacoGroupedList {
                ForEach(summary.holdings) { row in
                    Button {
                        Haptics.selection()
                        open(CabalHoldingRoute(cabalID: cabalID, symbol: row.symbol))
                    } label: {
                        HoldingRow(row: row)
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("cabal-holding-\(row.ticker)")
                }
                cashRow(summary.cash, yield: summary.cashYield)
            }
        }
    }

    static func slices(_ legend: [CabalPotSummary.Segment]) -> [AllocationBar.Slice] {
        legend.filter { $0.basisPoints > 0 }
            .map { AllocationBar.Slice(label: $0.label, fraction: Double($0.basisPoints) / 10_000, isCash: $0.isCash) }
    }

    private static func zeroPotNote(canPropose: Bool) -> String {
        canPropose ? "Fund, then propose the first buy." : "Nothing in the pot yet."
    }

    private static func cashOnlyNote(canPropose: Bool) -> String {
        canPropose ? "No stocks right now. Propose a buy to put the cash to work." : "No stocks right now."
    }

    private func cashRow(_ cash: String, yield: String?) -> some View {
        Button {
            Haptics.selection()
            open(GroupCashRoute(cabalID: cabalID, cabalName: cabalModel?.cabal?.name ?? ""))
        } label: {
            MonacoRow(title: "Cash", subtitle: "USDC", chevron: true, isLast: true) {
                SunkenGlyphMark(icon: .wallet)
            } trailing: {
                Text(cash)
                    .moneyFont(.row)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(1)
                if let yield {
                    Text(yield)
                        .moneyFont(.caption)
                        .foregroundStyle(MonacoTheme.profit)
                        .lineLimit(1)
                        .accessibilityIdentifier("cabal-holdings-cash-yield")
                }
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("cabal-holdings-cash")
    }

    private func note(_ text: String, id: String) -> some View {
        Text(text)
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier(id)
    }
}

private struct HoldingRow: View {
    let row: CabalPotSummary.Row

    var body: some View {
        MarketStockRow(
            name: row.name, subtitle: "\(row.ticker) · \(row.detail)",
            mark: StockMark(symbol: row.symbol, displayName: row.name, logoURL: row.logoURL)
        ) {
            Text(row.value)
                .moneyFont(.row)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(1)
            PnLText(dollarPnl: row.gain, style: .caption)
        }
    }
}
