import MonacoCore
import SwiftUI

struct AssetDetailClientView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    let symbol: String
    @State private var model: AssetDetailClientModel?

    var body: some View {
        Group {
            if let model {
                content(model)
            }
        }
        .navigationTitle(model?.detail?.name ?? AssetSymbolFormatter.display(symbol, kind: .stock))
        .navigationBarTitleDisplayMode(.inline)
        .task { await start() }
        .onChange(of: model?.failureTick) { _, _ in
            if let error = model?.lastError { toasts.show(error) }
        }
    }

    @ViewBuilder
    private func content(_ model: AssetDetailClientModel) -> some View {
        switch model.phase {
        case .idle where model.detail == nil, .loading where model.detail == nil:
            ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
        case .failed where model.detail == nil:
            EmptyState(title: "Could not load this stock", actionTitle: "Retry") {
                Task { await model.load() }
            }
        default:
            if let detail = model.detail {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    StockMark(symbol: detail.symbol, displayName: detail.name, assetKind: detail.kind, size: 64)
                    Text(detail.name).font(MonacoTheme.Typo.title)
                    Text(detail.ticker).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.muted)
                    if let price = detail.priceMicros { MoneyText(micros: price, style: .hero, voice: .market) }
                    if let change = detail.changeBasisPoints { PercentText(basisPoints: change, style: .row) }
                    Text(detail.attribution).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.muted)
                }
                .padding(MonacoTheme.Space.m)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                .monacoCanvas()
            }
        }
    }

    private func start() async {
        if model == nil { model = AssetDetailClientModel(api: environment.api, symbol: symbol) }
        await model?.load()
    }
}
