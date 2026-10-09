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
            StockRowSkeleton(rows: 1)
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
            MonacoGroupedList { cashRow(summary.cash) }
            note(Self.cashOnlyNote(canPropose: canPropose), id: "cabal-holdings-nothing-bought")
        case .invested:
            AllocationBar(legend: summary.legend, tint: .forGroupId(cabalID))
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.bottom, MonacoTheme.Space.xs)
            MonacoGroupedList {
                ForEach(summary.holdings) { row in
                    Button {
                        Haptics.selection()
                        open(AssetRoute(symbol: row.symbol))
                    } label: {
                        HoldingRow(row: row)
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("cabal-holding-\(row.ticker)")
                }
                cashRow(summary.cash)
            }
        }
    }

    private static func zeroPotNote(canPropose: Bool) -> String {
        canPropose ? "Fund, then propose the first buy." : "Nothing in the pot yet."
    }

    private static func cashOnlyNote(canPropose: Bool) -> String {
        canPropose ? "No stocks right now. Propose a buy to put the cash to work." : "No stocks right now."
    }

    private func cashRow(_ cash: String) -> some View {
        MonacoRow(title: "Cash", isLast: true) {
            StockMark(symbol: "USDC")
        } trailing: {
            Text(cash)
                .moneyFont(.row)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(1)
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("cabal-holdings-cash")
    }

    private func note(_ text: String, id: String) -> some View {
        Text(text)
            .font(MonacoTheme.Typo.callout)
            .foregroundStyle(MonacoTheme.muted)
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

private struct AllocationBar: View {
    let legend: [CabalPotSummary.Segment]
    let tint: MonacoTheme.CabalTint

    private static let gap: CGFloat = 2

    private var drawn: [CabalPotSummary.Segment] { legend.filter { $0.basisPoints > 0 } }

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            GeometryReader { geometry in
                let width = geometry.size.width - CGFloat(max(drawn.count - 1, 0)) * Self.gap
                HStack(spacing: Self.gap) {
                    ForEach(drawn) { segment in
                        Rectangle()
                            .fill(color(for: segment))
                            .frame(width: max(2, width * CGFloat(segment.basisPoints) / 10000))
                    }
                }
            }
            .frame(height: 8)
            .clipShape(Capsule())
            FlowingLegend(legend: legend, color: color(for:))
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Pot mix")
        .accessibilityValue(legend.map { "\($0.label) \($0.percent)" }.joined(separator: ", "))
        .accessibilityIdentifier("cabal-holdings-mix")
    }

    private func color(for segment: CabalPotSummary.Segment) -> Color {
        AllocationPalette.color(for: segment.swatch, tint: tint)
    }
}

enum AllocationPalette {
    static let opacities: [Double] = [1, 0.7, 0.5, 0.36, 0.26]

    static func opacity(forStep step: Int) -> Double {
        opacities[min(max(step, 0), opacities.count - 1)]
    }

    static func color(for swatch: CabalPotSummary.Swatch, tint: MonacoTheme.CabalTint) -> Color {
        switch swatch {
        case .cash: return MonacoTheme.cashFill
        case .stock(let step): return tint.fill.opacity(opacity(forStep: step))
        }
    }
}

private struct FlowingLegend: View {
    let legend: [CabalPotSummary.Segment]
    let color: (CabalPotSummary.Segment) -> Color

    var body: some View {
        ViewThatFits(in: .horizontal) {
            HStack(spacing: MonacoTheme.Space.sm) { entries }
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) { entries }
        }
    }

    private var entries: some View {
        ForEach(legend) { segment in
            HStack(spacing: MonacoTheme.Space.xs) {
                Circle()
                    .fill(color(segment))
                    .frame(width: 7, height: 7)
                Text("\(segment.label) \(segment.percent)")
                    .font(MonacoTheme.Typo.dataCaption)
                    .foregroundStyle(MonacoTheme.muted)
                    .lineLimit(1)
            }
        }
    }
}
