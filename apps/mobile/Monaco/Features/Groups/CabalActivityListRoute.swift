import MonacoCore
import SwiftUI

nonisolated struct CabalActivityListRoute: AppRoute {
    let cabalID: String

    @MainActor func destination() -> some View {
        CabalActivityListView(cabalID: cabalID)
    }
}

private struct CabalActivityListView: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: CabalActivityModel?

    var body: some View {
        ScrollView {
            CabalActivityContent(model: model, rows: model?.rows ?? [], skeletonRows: 6) {
                Task { await model?.loadMore() }
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .refreshable { await model?.refresh() }
        .monacoCanvas()
        .navigationTitle(CabalActivityCopy.header)
        .navigationBarTitleDisplayMode(.inline)
        .task {
            let model = preparedModel()
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
        .cabalActivityToasts(model, in: toasts)
    }

    private func preparedModel() -> CabalActivityModel {
        if let model { return model }
        let created = CabalActivityModel(
            cabalID: cabalID, api: environment.api, hints: environment.hints, clock: Date.init)
        model = created
        return created
    }
}
