import MonacoAPI
import MonacoCore
import OSLog
import SwiftUI

enum HomeFeedPreviewSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomeFeedPreview(source: .yourCabals, header: "From your cabals", firstUseOnly: false)
    }
}

enum HomeEveryonePreviewSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomeFeedPreview(source: .everyone, header: "From everyone", firstUseOnly: true)
    }
}

private struct HomeFeedPreview: View {
    let source: HomeFeedPreviewModel.Source
    let header: String
    let firstUseOnly: Bool

    private static let log = Logger(subsystem: "so.monaco.app", category: "home-feed-preview")

    @Environment(AppEnvironment.self) private var environment
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @State private var model: HomeFeedPreviewModel?

    private var waitsForFirstUse: Bool {
        guard firstUseOnly, let portfolio else { return false }
        return portfolio.summary?.isFirstUse != true
    }

    private var items: [Components.Schemas.FeedItem] {
        guard case .loaded(let items) = model?.state else { return [] }
        return items
    }

    var body: some View {
        let rows = items.map { FeedPreviewRowFacts($0, source: source, now: Date()) }
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader(header, trailing: "See all") {
                environment.navigator.selectedTab = .feed
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoGroupedList {
                ForEach(rows) { row in
                    NavigationLink(value: row.destination.route) {
                        HomeFeedPreviewRow(row: row, isLast: row.id == rows.last?.id)
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("home-feed-preview-row-\(row.id)")
                }
            }
        }
        .homeSlotHidden(rows.isEmpty || waitsForFirstUse)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(source == .yourCabals ? "home-feed-preview" : "home-feed-everyone")
        .task(id: waitsForFirstUse) {
            guard !waitsForFirstUse else { return }
            let model = preparedModel()
            refresh?.register("home-feed-preview-\(header)") { await model.load() }
            await model.load()
            if case .failed(let error) = model.state {
                Self.log.error("home feed preview failed: \(String(describing: error), privacy: .public)")
            }
            await model.observe()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
    }

    private func preparedModel() -> HomeFeedPreviewModel {
        if let model { return model }
        let created = HomeFeedPreviewModel(
            api: environment.api, hints: environment.hints, clock: ContinuousClock(), source: source)
        model = created
        return created
    }
}

private struct HomeFeedPreviewRow: View {
    let row: FeedPreviewRowFacts
    let isLast: Bool

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var layout: MonacoRowLayout { MonacoRowLayout(dynamicTypeSize: dynamicTypeSize) }

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            lead
                .frame(width: MonacoRowLayout.baseMarkSize, height: MonacoRowLayout.baseMarkSize)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(row.title)
                    .font(MonacoTheme.Typo.rowTitle)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(layout.titleLineLimit)
                    .truncationMode(.tail)
                subtitle
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .lineLimit(layout.subtitleLineLimit)
                    .truncationMode(.tail)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            trailing
                .layoutPriority(1)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.s)
        .frame(minHeight: MonacoRowLayout.minHeight)
        .contentShape(Rectangle())
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule()
                    .padding(.leading, layout.separatorLeadingInset)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(row.spoken)
    }

    private var subtitle: Text {
        guard row.replies > 0 else { return Text(row.subtitle) }
        return Text("\(row.subtitle) · \(Image(systemName: "bubble.left")) \(row.replies)")
    }

    @ViewBuilder private var lead: some View {
        switch row.lead {
        case .person(let name, let photoURL, let userID):
            MonacoAvatar(photoURL: photoURL, displayName: name, size: MonacoRowLayout.baseMarkSize, seed: userID)
        case .cabal(let name, let pictureURL, let cabalID):
            CabalMark(
                groupId: cabalID ?? name, name: name, size: MonacoRowLayout.baseMarkSize, pictureUrl: pictureURL)
        case .asset(let symbol, let name, let logoURL, let isPreIPO):
            StockMark(
                symbol: symbol, displayName: name, assetKind: isPreIPO ? .preIpo : .stock,
                size: MonacoRowLayout.baseMarkSize, logoURL: logoURL.flatMap { URL(string: $0) })
        }
    }

    @ViewBuilder private var trailing: some View {
        VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
            switch row.trailing {
            case .split(let yes, let no):
                Text("✓ \(yes) · ✕ \(no)")
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
            case .amount(let dollars, _):
                if let dollars { figure(dollars) }
                change
            case .symbol(let symbol, _):
                figure(symbol)
                change
            case .label(let text):
                Text(text)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
            case nil:
                EmptyView()
            }
        }
    }

    private func figure(_ text: String) -> some View {
        Text(text)
            .font(MonacoTheme.Typo.headline)
            .foregroundStyle(MonacoTheme.primaryText)
    }

    @ViewBuilder private var change: some View {
        if let text = row.trailing?.returnText {
            Text(text)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.signed(text))
        }
    }
}
