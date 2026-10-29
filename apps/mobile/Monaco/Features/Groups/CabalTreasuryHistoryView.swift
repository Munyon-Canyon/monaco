import MonacoCore
import SwiftUI

enum CabalTreasuryHistoryCopy {
    static let title = CabalTreasuryRows.historyLabel
}

struct CabalTreasuryHistoryView: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: CabalActivityModel?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: CabalTreasuryMetrics.sectionGap) {
                CabalValueChart(cabalID: cabalID, topInset: MonacoTheme.Space.s)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    MonacoSectionHeader(CabalActivityCopy.header)
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                    CabalActivityContent(model: model, rows: model?.rows ?? [], skeletonRows: 6) {
                        Task { await model?.loadMore() }
                    }
                }
                .accessibilityElement(children: .contain)
                .accessibilityIdentifier("cabal-activity")
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .refreshable { await model?.refresh() }
        .monacoCanvas()
        .navigationTitle(CabalTreasuryHistoryCopy.title)
        .navigationBarTitleDisplayMode(.inline)
        .task {
            let model = preparedModel()
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
        .cabalActivityToasts(model, in: toasts)
        .accessibilityIdentifier("cabal-treasury-history-screen")
    }

    private func preparedModel() -> CabalActivityModel {
        if let model { return model }
        let created = CabalActivityModel(
            cabalID: cabalID, api: environment.api, hints: environment.hints, clock: Date.init)
        model = created
        return created
    }
}
