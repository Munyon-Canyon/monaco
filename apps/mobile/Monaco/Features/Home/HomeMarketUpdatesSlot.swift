import MonacoCore
import SwiftUI

enum HomeMarketUpdatesSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomeMarketUpdates()
    }
}

private struct HomeMarketUpdates: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(\.homeReads) private var reads
    @State private var stocks: StocksTabModel?

    var body: some View {
        let gate = HomeGate(portfolio: portfolio, reads: reads, balance: environment.balance.state)
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Market updates", trailing: "Markets") {
                environment.navigator.selectedTab = .stocks
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoGroupedList {
                mover
                news
            }
        }
        .homeSlotHidden(!gate.shows { $0 != .empty })
        .task {
            let model = preparedModel()
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { stocks?.setVisible($0) }
    }

    @ViewBuilder private var mover: some View {
        switch stocks?.phase ?? .loading {
        case .idle, .loading:
            MonacoRowSkeleton(rows: 1, markShape: .tile)
                .accessibilityIdentifier("home-market-loading")
        case .failed:
            EmptyView()
        case .loaded:
            if let asset = MarketMover.top(in: stocks?.rows ?? []) {
                moverRow(asset)
            }
        }
    }

    private func moverRow(_ asset: MarketAsset) -> some View {
        let tone = PnLTone(dollarPnl: asset.changeText)
        let color = tone == .loss ? MonacoTheme.loss : MonacoTheme.profit
        return NavigationLink(value: AnyAppRoute(AssetRoute(symbol: asset.symbol, seed: AssetSeed(asset)))) {
            HomeMarketRow(isLast: false) {
                StockMark(
                    symbol: asset.symbol, displayName: asset.name, assetKind: asset.kind, size: .row,
                    logoURL: asset.logoURL)
            } title: {
                Text("\(asset.name) · ") + Text(asset.changeText).foregroundStyle(color)
            } subtitle: {
                "24h token move"
            } trailing: {
                Image(monaco: .propose)
                    .foregroundStyle(color)
                    .accessibilityHidden(true)
            }
        }
        .buttonStyle(.monacoRow)
        .accessibilityIdentifier("home-market-mover")
    }

    private var news: some View {
        HomeMarketRow(isLast: true) {
            SunkenGlyphMark(icon: .news)
        } title: {
            Text("News")
        } subtitle: {
            "News opens soon."
        } trailing: {
            EmptyView()
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("home-news-soon")
    }

    private func preparedModel() -> StocksTabModel {
        if let stocks { return stocks }
        let created = StocksTabModel(api: environment.api, hints: environment.hints, clock: ContinuousClock())
        stocks = created
        return created
    }
}

private struct HomeMarketRow<Leading: View, Trailing: View>: View {
    let isLast: Bool
    @ViewBuilder let leading: Leading
    let title: Text
    let subtitle: String
    @ViewBuilder let trailing: Trailing

    init(
        isLast: Bool, @ViewBuilder leading: () -> Leading, @ViewBuilder title: () -> Text,
        subtitle: () -> String, @ViewBuilder trailing: () -> Trailing
    ) {
        self.isLast = isLast
        self.leading = leading()
        self.title = title()
        self.subtitle = subtitle()
        self.trailing = trailing()
    }

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            leading
                .frame(width: MonacoRowLayout.baseMarkSize, height: MonacoRowLayout.baseMarkSize)
            VStack(alignment: .leading, spacing: 0) {
                title
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.primaryText)
                Text(subtitle)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            trailing
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.sm)
        .frame(minHeight: MonacoRowLayout.minHeight)
        .contentShape(Rectangle())
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule().padding(.horizontal, MonacoTheme.Space.gutter)
            }
        }
    }
}
