import MonacoAPI
import MonacoCore
import SwiftUI

typealias StockSearch = @Sendable (String) async -> [Components.Schemas.AssetSummary]

private struct StockSearchKey: EnvironmentKey {
    static let defaultValue: StockSearch? = nil
}

extension EnvironmentValues {
    var stockSearch: StockSearch? {
        get { self[StockSearchKey.self] }
        set { self[StockSearchKey.self] = newValue }
    }
}

enum LiveStockSearch {
    static func make(_ api: APIClient) -> StockSearch {
        { query in
            let list = try? await api.read { client in
                try await client.getAssets(query: .init(q: query.isEmpty ? nil : query, limit: 20)).ok.body.json
            }
            return list?.assets ?? []
        }
    }
}

struct StockShareSheet: View {
    let search: StockSearch
    let onPick: (CommentAttachmentDraft) -> Void

    @State private var query = ""
    @State private var results: [Components.Schemas.AssetSummary] = []
    @Environment(\.dismiss) private var dismiss

    private static let debounce = Duration.milliseconds(300)

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            HStack {
                Text("Share a stock")
                    .font(MonacoTheme.Typo.section)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .accessibilityAddTraits(.isHeader)
                Spacer(minLength: 0)
                MonacoGlassButton(systemImage: "xmark", accessibilityLabel: "Close", style: .quiet, size: .compact) {
                    dismiss()
                }
                .accessibilityIdentifier("stock-sheet-close")
            }
            .frame(minHeight: 48)
            MonacoSearchField(placeholder: "Search stocks", text: $query)
                .accessibilityElement(children: .contain)
                .accessibilityIdentifier("stock-sheet-search")
            ScrollView {
                LazyVStack(spacing: 0) {
                    ForEach(results, id: \.id) { asset in
                        row(asset)
                    }
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.s)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .background(MonacoTheme.canvas.ignoresSafeArea())
        .presentationDetents([.medium])
        .presentationDragIndicator(.visible)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("stock-sheet")
        .task(id: query) {
            if !query.isEmpty { try? await Task.sleep(for: Self.debounce) }
            guard !Task.isCancelled else { return }
            let found = await search(query)
            guard !Task.isCancelled else { return }
            results = found
        }
    }

    private func row(_ summary: Components.Schemas.AssetSummary) -> some View {
        let asset = MarketMapping.asset(summary)
        let color = (asset.changeBasisPoints ?? 0) < 0 ? MonacoTheme.loss : MonacoTheme.profit
        return Button {
            onPick(.stock(assetID: summary.id))
            dismiss()
        } label: {
            MarketStockRow(
                name: asset.name, subtitle: asset.ticker,
                mark: StockMark(
                    symbol: asset.symbol, displayName: asset.name, assetKind: asset.kind, size: .row,
                    logoURL: asset.logoURL)
            ) {
                if asset.isDelayed { DelayedTag() }
                Text(asset.priceText)
                    .moneyFigure(.row)
                    .foregroundStyle(MonacoTheme.ink)
                    .lineLimit(1)
                if asset.kind != .preIpo {
                    Text(asset.changeText)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(color)
                }
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("stock-share-row-\(asset.ticker)")
    }
}
